package core

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	isTls   bool
	conn    *net.Conn
	tlsConn *tls.Conn
	reader  *bufio.Reader
}

func (client *Client) ReadRequest() (*http.Request, error) {
	return http.ReadRequest(client.reader)
}

func (client *Client) Write(b []byte) (n int, err error) {
	if client.isTls {
		return client.tlsConn.Write(b)
	} else {
		return (*client.conn).Write(b)
	}
}

func (client *Client) Read(b []byte) (n int, err error) {
	if client.isTls {
		return client.tlsConn.Read(b)
	} else {
		return (*client.conn).Read(b)
	}
}

type Server struct {
	isTls   bool
	conn    *net.Conn
	tlsConn *tls.Conn
	reader  *bufio.Reader
}

func (server *Server) ReadResponse(req *http.Request) (*http.Response, error) {
	return http.ReadResponse(server.reader, req)
}

func (server *Server) Write(b []byte) (n int, err error) {
	if server.isTls {
		return server.tlsConn.Write(b)
	} else {
		return (*server.conn).Write(b)
	}
}

func (server *Server) Read(b []byte) (n int, err error) {
	if server.isTls {
		return server.tlsConn.Read(b)
	} else {
		return (*server.conn).Read(b)
	}
}

type Intermediary struct {
	client        Client
	clientWriteCh chan func()
	server        map[string]*Server
}

func (inter *Intermediary) ReadRequest(handleRequestFn func(req *http.Request) error) {
	(*inter.client.conn).SetDeadline(time.Now().Add(time.Second * 60))
	for {
		request, err := inter.client.ReadRequest()
		if err != nil {
			if nErr, ok := err.(net.Error); err != io.EOF && (ok && !nErr.Timeout()) {
				log.Println(err)
			}
			return
		}
		(*inter.client.conn).SetDeadline(time.Now().Add(time.Second * 60))

		err = handleRequestFn(request)
		if err != nil {
			return
		}
	}
}

func (inter *Intermediary) DoRequest(request *MyRequest, isRetry bool) (*http.Response, error) {
	server, err := inter.connectServer(request, isRetry)
	if err != nil {
		return nil, err
	}

	rc := request.raw.Clone(context.TODO())
	err = rc.Write(server)
	rc.Body.Close()
	if err != nil {
		if !isRetry {
			// 重试
			return inter.DoRequest(request, true)
		}
		return nil, err
	}

	resp, err := server.ReadResponse(request.raw)
	if err != nil && !isRetry {
		return inter.DoRequest(request, true)
	}
	return resp, err
}

func (inter *Intermediary) connectServer(request *MyRequest, isConnClosed bool) (*Server, error) {
	target := request.raw.Host
	server := inter.server[target]

	isServerTls := false
	if request.raw.URL.Scheme == "https" {
		isServerTls = true
	} else if request.raw.URL.Scheme == "http" {
		isServerTls = false
	} else if request.raw.URL.Scheme == "" {
		isServerTls = inter.client.isTls
	}

	if strings.LastIndex(target, ":") < 0 {
		if isServerTls {
			target += ":443"
		} else {
			target += ":80"
		}
	}

	createConn := func() error {
		serverConn, err := net.Dial("tcp", target)
		if err != nil {
			return err
		}
		server = &Server{
			conn:   &serverConn,
			reader: bufio.NewReader(serverConn),
		}
		inter.server[request.raw.Host] = server
		return nil
	}

	proxyPath := request.proxy[1]
	if proxyPath != "" {
		target = proxyPath
	}

	if server == nil || isConnClosed {
		err := createConn()
		if err != nil {
			return nil, err
		}
	}

	if isServerTls && !server.isTls {
		err := inter.UpgradeServer2TLS(request)
		if err != nil {
			return nil, err
		}
	}

	return server, nil
}

func (inter *Intermediary) UpgradeClient2Tls(host string) error {
	response := "HTTP/1.1 200 Connection Established\r\n\r\n"
	if _, err := (*inter.client.conn).Write([]byte(response)); err != nil {
		log.Printf("Error sending CONNECT response: %v", err)
		return err
	}

	_host, _, err := net.SplitHostPort(host)
	if err != nil {
		_host = host
	}
	cert, err := SignHost([]string{_host})
	if err != nil {
		return err
	}

	inter.client.tlsConn = tls.Server(*inter.client.conn, &tls.Config{
		Certificates: []tls.Certificate{
			cert,
		},
		InsecureSkipVerify: true,
	})

	inter.client.isTls = true
	inter.client.reader = bufio.NewReader(inter.client.tlsConn)
	return nil
}

func (inter *Intermediary) UpgradeServer2TLS(req *MyRequest) error {
	server := inter.server[req.raw.Host]
	targetAddr := req.raw.Host
	proxyAddr := req.proxy[1]

	if !strings.Contains(targetAddr, ":") {
		targetAddr += ":443"
	}
	// 如果需要通过代理连接
	if proxyAddr != "" {
		// 发送 CONNECT 请求到代理服务器
		auth := strings.Join(req.proxy[2:], ":")
		basicAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))

		// 构造 CONNECT 请求
		connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\nProxy-Authorization: %s\r\n\r\n", targetAddr, targetAddr, basicAuth)
		if _, err := (*server.conn).Write([]byte(connectReq)); err != nil {
			return fmt.Errorf("failed to send CONNECT request: %v", err)
		}

		resp, err := server.ReadResponse(nil)
		if err != nil {
			return fmt.Errorf("failed to read CONNECT response: %v", err)
		}
		defer resp.Body.Close()

		// 检查代理服务器是否返回 200 Connection Established
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("proxy returned non-200 status: %s", resp.Status)
		}
	}

	// 将连接升级为 TLS 连接
	server.tlsConn = tls.Client(*server.conn, &tls.Config{
		ServerName:         strings.Split(targetAddr, ":")[0], // 设置 SNI
		InsecureSkipVerify: true,
	})
	server.isTls = true

	// 进行 TLS 握手
	if err := server.tlsConn.Handshake(); err != nil {
		return fmt.Errorf("TLS handshake failed: %v", err)
	}
	server.reader.Reset(*server.conn)
	server.reader = bufio.NewReader(server.tlsConn)

	return nil
}
