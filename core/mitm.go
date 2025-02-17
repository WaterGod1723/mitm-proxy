package core

import (
	"bufio"
	"compress/gzip"
	"compress/zlib"
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

type ResponseWriteFunc func(w *ResponseWriter) error

// [协议，代理地址，账号，密码]
type ProxyArray = [4]string

type Container struct {
	connectors   map[int]*Connector
	count        int
	uid          int
	mu           sync.Mutex
	manageRouter map[string]func(w *ResponseWriter, r *http.Request)
	port         string
	localIPs     map[string]struct{} // 防止请求形成环路

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
		connectors: make(map[int]*Connector),
		count:      0,
		uid:        0,
		mu:         sync.Mutex{},
	}
}

func (c *Container) Start(addr string) {
	var err error
	c.localIPs, err = getLocalIPs()
	if err != nil {
		log.Fatal(err)
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Println("some error", err)
		time.Sleep(time.Second * 10)
		return
	}

	c.port = strings.Split(addr, ":")[1]
	log.Println("server on http://localhost:" + c.port)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Fatal(err)
		}

		go c.addConnector(&conn)
	}
}

func (c *Container) addConnector(clientConn *net.Conn) {
	c.mu.Lock()
	c.count++
	c.uid++
	id := c.uid
	connector := Connector{
		client: Client{
			conn:   clientConn,
			reader: bufio.NewReader(*clientConn),
		},
		server:        mapPool.Get().(map[string]*Server),
		clientWriteCh: make(chan func(), 10),
	}
	go func() {
		for fn := range connector.clientWriteCh {
			fn()
		}
	}()
	c.connectors[id] = &connector
	c.mu.Unlock()
	log.Printf("conn++: %d\n", c.count)

	defer func() {
		c.mu.Lock()
		delete(c.connectors, id)
		c.count--
		log.Printf("conn--: %d\n", c.count)
		c.mu.Unlock()

		for key, server := range connector.server {
			(*server.conn).Close()
			delete(connector.server, key)
		}
		mapPool.Put(connector.server)
		(*clientConn).Close()
		close(connector.clientWriteCh)
	}()

	connector.ReadRequest(func(req *http.Request, isWs bool) {
		defer func() {
			if err := recover(); err != nil {
				log.Println(err)
			}
		}()
		defer req.Body.Close()

		hostPort := strings.Split(req.Host, ":")

		port := "80"
		if connector.client.isTls {
			port = "443"
		}

		if len(hostPort) > 1 {
			port = hostPort[1]
		}

		if _, ok := c.localIPs[hostPort[0]]; ok && port == c.port {
			ww := NewResponseWriter(&connector.client)
			ww.Header().Set("Access-Control-Allow-Origin", "*")
			ww.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE")
			ww.Header().Set("Access-Control-Allow-Headers", "*")
			ww.Header().Set("Access-Control-Max-Age", "86400")
			ww.Header().Set("Access-Control-Allow-Credentials", "true")

			if !(*connector.client.conn).RemoteAddr().(*net.TCPAddr).IP.IsLoopback() {
				ww.SetStatus(http.StatusNotFound)
				connector.clientWriteCh <- func() {
					ww.Write([]byte("404 not found"))
				}
				return
			}

			if req.Method == http.MethodOptions {
				ww.SetStatus(http.StatusNoContent)
				connector.clientWriteCh <- func() {
					ww.Write([]byte(""))
				}
				return
			}
			fn := c.manageRouter[req.URL.Path]
			if fn != nil {
				connector.clientWriteCh <- func() {
					fn(ww, req)
				}
			} else {
				ww.SetStatus(http.StatusNoContent)
				connector.clientWriteCh <- func() {
					ww.Write([]byte(""))
				}
			}
			return
		}

		if c.insertHTMLFn != nil {
			req.Header.Set("accept-encoding", "gzip, deflate")
		}

		if c.processRequest != nil {
			writeFn := c.processRequest(req)
			if writeFn != nil {
				connector.clientWriteCh <- func() {
					writeFn(NewResponseWriter(&connector.client))
				}
			}
		}

		mReq := &MyRequest{
			raw: req,
		}
		if c.processProxy != nil {
			mReq.proxy = c.processProxy(req)
		}

		resp, err := connector.DoRequest(mReq, false)
		if err != nil {
			ww := NewResponseWriter(&connector.client)
			ww.SetStatus(http.StatusBadGateway)
			ww.Header().Set("Content-Type", "text/plain; charset=utf-8")
			connector.clientWriteCh <- func() {
				ww.Write([]byte(err.Error()))
			}
			return
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
				connector.clientWriteCh <- func() {
					defer resp.Body.Close()
					writeFn(NewResponseWriter(&connector.client))
				}
				return
			}
		}

		w := func() {
			err = resp.Write(&connector.client)
			if err != nil {
				log.Println("write to client error", err)
				return
			}
			resp.Body.Close()

			if req.URL.Scheme == "" {
				if connector.client.isTls {
					req.URL.Scheme = "https"
				} else {
					req.URL.Scheme = "http"
				}
			}
			log.Println((*connector.client.conn).RemoteAddr(), req.URL, resp.Status)
		}
		if isWs {
			w()
			server := connector.server[req.Host]
			s := ""
			if server.isTls {
				s = "s"
			}
			if resp.StatusCode == http.StatusSwitchingProtocols {
				(*connector.client.conn).SetDeadline(time.Now().Add(time.Hour * 24))
				log.Printf("websocket connected: ws%s://%s\n", s, req.Host)
				go io.Copy(&connector.client, server)
				io.Copy(server, &connector.client)
			} else {
				log.Printf("websocket connect error: ws%s://%s\n", s, req.Host)
			}
		} else {
			connector.clientWriteCh <- w
		}

	})
}

// 处理host是localhost且监听端口是本服务的http请求，主要用于mitm服务管理
func (c *Container) HandleFunc(pattern string, handleFunc func(w *ResponseWriter, r *http.Request)) {
	if c.manageRouter == nil {
		c.manageRouter = make(map[string]func(w *ResponseWriter, r *http.Request))
	}
	c.manageRouter[pattern] = handleFunc
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
		resp.Header.Del("Content-Security-Policy")

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

func getLocalIPs() (map[string]struct{}, error) {
	ips := make(map[string]struct{})
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range interfaces {
		// 排除回环接口
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}

		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
				ips[ipnet.IP.String()] = struct{}{}
			}
		}
	}

	// 如果没有找到有效的 IP 地址，则返回错误
	if len(ips) == 0 {
		return nil, fmt.Errorf("could not find any valid non-loopback IP addresses")
	}

	ips["localhost"] = struct{}{}
	ips["127.0.0.1"] = struct{}{}

	return ips, nil
}
