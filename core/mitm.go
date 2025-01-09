package core

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"mitm/util"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

var mapPool = sync.Pool{
	New: func() interface{} {
		return make(map[string]*Server)
	},
}

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
	client Client
	server map[string]*Server
}

// [协议，代理地址，账号，密码]
type ProxyArray = [4]string

type ResponseWriteFunc func(w io.Writer) error

type Container struct {
	inters map[int]*Intermediary
	count  int
	uid    int
	mu     sync.Mutex

	processProxy    func(req *http.Request) ProxyArray
	processRequest  func(req *http.Request) ResponseWriteFunc
	processResponse func(resp *http.Response) ResponseWriteFunc
}

type MyRequest struct {
	raw   *http.Request
	proxy ProxyArray
}

func NewMITM() *Container {
	return &Container{
		inters: make(map[int]*Intermediary),
		count:  0,
		uid:    0,
		mu:     sync.Mutex{},
	}
}

func (c *Container) Start(addr string) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Println("some error", err)
		time.Sleep(time.Second * 10)
		return
	}
	fmt.Println("server on: http://localhost:8003", listener)
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Fatal(err)
		}

		go c.addIntermediary(&conn)
	}
}

func (c *Container) addIntermediary(clientConn *net.Conn) {
	c.mu.Lock()
	c.count++
	c.uid++
	id := c.uid
	inter := Intermediary{
		client: Client{
			conn:   clientConn,
			reader: bufio.NewReader(*clientConn),
		},
		server: mapPool.Get().(map[string]*Server),
	}
	c.inters[id] = &inter
	c.mu.Unlock()
	log.Printf("conn++: %d\n", c.count)

	defer func() {
		c.mu.Lock()
		delete(c.inters, id)
		c.count--
		log.Printf("conn--: %d\n", c.count)
		c.mu.Unlock()

		for key, server := range inter.server {
			(*server.conn).Close()
			delete(inter.server, key)
		}
		mapPool.Put(inter.server)
		(*clientConn).Close()
	}()

	inter.ReadRequest(func(req *http.Request) error {
		if req.Method == http.MethodConnect {
			err := inter.UpgradeClient2Tls(req.Host)
			return err
		}

		if c.processRequest != nil {
			writeFn := c.processRequest(req)
			if writeFn != nil {
				return writeFn(&inter.client)
			}
		}

		mReq := &MyRequest{
			raw: req,
		}
		if c.processProxy != nil {
			mReq.proxy = c.processProxy(req)
		}

		resp, err := inter.DoRequest(mReq, false)
		if err != nil {
			ww := util.NewResponseWriter(&inter.client)
			ww.SetStatus(http.StatusBadGateway)
			ww.Header().Set("Content-Type", "text/plain; charset=utf-8")
			ww.Write([]byte(err.Error()))
			return err
		}

		if c.processResponse != nil {
			writeFn := c.processResponse(resp)
			if writeFn != nil {
				return writeFn(&inter.client)
			}
		}

		err = resp.Write(&inter.client)
		if err != nil {
			log.Println("write to client error", err)
		}
		log.Println((*inter.client.conn).RemoteAddr(), req.URL, resp.Status)

		if isWebSocketRequest(req) {
			server := inter.server[req.Host]
			s := ""
			if server.isTls {
				s = "s"
			}
			if resp.StatusCode == http.StatusSwitchingProtocols {
				log.Printf("websocket connected: ws%s://%s\n", s, req.Host)
				go io.Copy(&inter.client, server)
				io.Copy(server, &inter.client)
			} else {
				log.Printf("websocket connect error: ws%s://%s\n", s, req.Host)
			}
		}

		return nil
	})
}

func (c *Container) SetProxy(fn func(req *http.Request) ProxyArray) *Container {
	c.processProxy = fn
	return c
}

func (c *Container) ProcessRequest(fn func(req *http.Request) ResponseWriteFunc) *Container {
	c.processRequest = fn
	return c
}

func (c *Container) ProcessResponse(fn func(resp *http.Response) ResponseWriteFunc) *Container {
	c.processResponse = fn
	return c
}

func (inter *Intermediary) ReadRequest(handleRequestFn func(req *http.Request) error) {
	(*inter.client.conn).SetDeadline(time.Now().Add(time.Second * 60))
	for {
		request, err := inter.client.ReadRequest()
		if err != nil {
			log.Println(err)
			return
		}
		(*inter.client.conn).SetDeadline(time.Now().Add(time.Second * 60))

		err = handleRequestFn(request)
		if err != nil {
			return
		}
	}
}

func (inter *Intermediary) DoRequest(request *MyRequest, isConnClosed bool) (*http.Response, error) {
	server, err := inter.connectTarget(request, isConnClosed)
	if err != nil {
		return nil, err
	}

	err = request.raw.Write(server)
	if err != nil {
		if err == net.ErrClosed {
			// 重试
			return inter.DoRequest(request, true)
		}
		return nil, err
	}
	return server.ReadResponse(request.raw)
}

func (inter *Intermediary) connectTarget(request *MyRequest, isConnClosed bool) (*Server, error) {
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
