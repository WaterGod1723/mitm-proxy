package core

import (
	"bufio"
	"compress/gzip"
	"compress/zlib"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"log"
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
	insertHTMLFn    func(resp *http.Response) error
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
	fmt.Println("server on: http://localhost:8003")
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

		if c.insertHTMLFn != nil {
			req.Header.Set("accept-encoding", "gzip, deflate")
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
			ww := NewResponseWriter(&inter.client)
			ww.SetStatus(http.StatusBadGateway)
			ww.Header().Set("Content-Type", "text/plain; charset=utf-8")
			ww.Write([]byte(err.Error()))
			return err
		}

		if c.insertHTMLFn != nil {
			err := c.insertHTMLFn(resp)
			if err != nil {
				log.Println("insert html error:", err)
			}
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
			return err
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

// 设置代理
func (c *Container) SetProxy(fn func(req *http.Request) ProxyArray) *Container {
	c.processProxy = fn
	return c
}

// 设置请求的预处理
func (c *Container) ProcessRequest(fn func(req *http.Request) ResponseWriteFunc) *Container {
	c.processRequest = fn
	return c
}

// 设置响应的处理
func (c *Container) ProcessResponse(fn func(resp *http.Response) ResponseWriteFunc) *Container {
	c.processResponse = fn
	return c
}

// html元素注入，注入html元素的时候会默认将请求头中的accept-encoding修改为gzip, deflate;
func (c *Container) InsertHTMLToHTMLBody(htmlFn func(resp *http.Response) string) {
	decompressBody := func(resp *http.Response) (string, error) {
		var reader io.ReadCloser
		var err error
		// 根据 Content-Encoding 头选择解压缩方式
		encoding := resp.Header.Get("Content-Encoding")
		switch strings.ToLower(encoding) {
		case "gzip":
			reader, err = gzip.NewReader(resp.Body)
			if err != nil {
				return "", fmt.Errorf("failed to create gzip reader: %v", err)
			}
			defer reader.Close()
		case "deflate":
			reader, err = zlib.NewReader(resp.Body)
			if err != nil {
				return "", fmt.Errorf("failed to create zlib reader: %v", err)
			}
			defer reader.Close()
		default:
			reader = resp.Body // 未压缩，直接使用原响应体
		}

		// 读取解压缩后的数据
		bodyBytes, err := io.ReadAll(reader)
		if err != nil {
			return "", fmt.Errorf("failed to read response body: %v", err)
		}

		// 删除 Content-Encoding 头
		resp.Header.Del("Content-Encoding")

		return string(bodyBytes), nil
	}

	c.insertHTMLFn = func(resp *http.Response) error {
		// 检查 Content-Type 是否为 HTML
		contentType := resp.Header.Get("Content-Type")
		if !strings.Contains(contentType, "text/html") {
			return nil // 不是 HTML，直接返回原响应
		}

		// 在 </body> 标签前插入指定字符串
		bodyStr, err := decompressBody(resp)
		if err != nil {
			return err
		}
		bodyTagIndex := strings.LastIndex(bodyStr, "</body>")
		if bodyTagIndex == -1 {
			// 构建新的响应体
			newBody := io.NopCloser(strings.NewReader(bodyStr))
			resp.Body = newBody
			// 如果没有找到 </body> 标签，直接返回原响应
			return nil
		}

		// 构建新的响应体
		newBodyStr := bodyStr[:bodyTagIndex] + htmlFn(resp) + bodyStr[bodyTagIndex:]
		newBody := io.NopCloser(strings.NewReader(newBodyStr))

		// 更新响应体
		resp.Body = newBody
		resp.ContentLength = int64(len(newBodyStr))
		resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(newBodyStr)))

		return nil
	}
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

func (inter *Intermediary) DoRequest(request *MyRequest, isRetry bool) (*http.Response, error) {
	server, err := inter.connectTarget(request, isRetry)
	if err != nil {
		return nil, err
	}

	err = request.raw.Write(server)
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

func isWebSocketRequest(r *http.Request) bool {
	// 检查请求头中的 Connection 和 Upgrade 字段
	connection := r.Header.Get("Connection")
	upgrade := r.Header.Get("Upgrade")
	if !strings.Contains(strings.ToLower(connection), "upgrade") || !strings.EqualFold(upgrade, "websocket") {
		return false
	}

	// 检查是否有 Sec-WebSocket-Key 和 Sec-WebSocket-Version 头
	if r.Header.Get("Sec-WebSocket-Key") == "" || r.Header.Get("Sec-WebSocket-Version") == "" {
		return false
	}

	// 如果符合所有条件，返回 true
	return true
}
