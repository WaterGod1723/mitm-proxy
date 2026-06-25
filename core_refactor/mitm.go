package core_refactor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	defaultDialTimeout = 10 * time.Second
	defaultIdleTimeout = 60 * time.Second
	defaultCertPath    = "./cert/cert.pem"
	defaultKeyPath     = "./cert/key.pem"
)

// MITM 是重构后的中间人代理核心。
type MITM struct {
	ca *CA

	proxyFunc       ProxyFunc
	requestHandler  func(*http.Request) *http.Response
	responseHandler func(*http.Response) *http.Response
	htmlInjector    *HTMLInjector

	manageRouter map[string]http.HandlerFunc
	logger       *log.Logger

	dialTimeout time.Duration
	idleTimeout time.Duration
	certPath    string
	keyPath     string

	listener   net.Listener
	listenPort string
	mu         sync.Mutex
	closed     bool
	conns      sync.WaitGroup
	clients    map[net.Conn]struct{}
	clientsMu  sync.Mutex
	localIPs   map[string]struct{}
}

// New 构造并初始化 MITM 代理。若未指定 CA，会尝试从默认路径加载。
func New(opts ...Option) (*MITM, error) {
	m := &MITM{
		proxyFunc:    DirectProxy,
		manageRouter: make(map[string]http.HandlerFunc),
		dialTimeout:  defaultDialTimeout,
		idleTimeout:  defaultIdleTimeout,
		certPath:     defaultCertPath,
		keyPath:      defaultKeyPath,
		clients:      make(map[net.Conn]struct{}),
	}
	for _, opt := range opts {
		opt(m)
	}

	if m.logger == nil {
		m.logger = log.New(io.Discard, "", 0)
	}

	if m.ca == nil {
		ca, err := LoadCA(m.certPath, m.keyPath)
		if err != nil {
			return nil, fmt.Errorf("load CA: %w", err)
		}
		m.ca = ca
	}

	ips, err := localIPs()
	if err != nil {
		return nil, fmt.Errorf("get local ips: %w", err)
	}
	m.localIPs = ips
	return m, nil
}

// HandleFunc 注册本地管理接口，仅对监听地址生效。
func (m *MITM) HandleFunc(pattern string, h http.HandlerFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.manageRouter[pattern] = h
}

// Start 阻塞地启动代理监听。
func (m *MITM) Start(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		ln.Close()
		return errors.New("mitm already stopped")
	}
	m.listener = ln
	_, m.listenPort, _ = net.SplitHostPort(ln.Addr().String())
	m.mu.Unlock()

	m.logger.Printf("mitm-proxy listening on %s", ln.Addr())

	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			m.logger.Printf("accept error: %v", err)
			continue
		}
		m.conns.Add(1)
		m.clientsMu.Lock()
		m.clients[conn] = struct{}{}
		m.clientsMu.Unlock()
		go func(c net.Conn) {
			defer func() {
				m.clientsMu.Lock()
				delete(m.clients, c)
				m.clientsMu.Unlock()
				m.conns.Done()
			}()
			m.serve(c)
		}(conn)
	}
}

// Stop 停止监听并等待所有连接处理完毕。
func (m *MITM) Stop() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	ln := m.listener
	m.mu.Unlock()

	if ln != nil {
		if err := ln.Close(); err != nil {
			return err
		}
	}

	// 强制关闭所有活跃客户端连接，避免 goroutine 在 I/O 上阻塞导致程序无法退出。
	m.clientsMu.Lock()
	for c := range m.clients {
		c.Close()
	}
	m.clientsMu.Unlock()

	m.conns.Wait()
	return nil
}

func (m *MITM) serve(conn net.Conn) {
	defer conn.Close()

	client := newClientConn(conn)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sess := &session{
		mitm:    m,
		client:  client,
		servers: make(map[string]*serverConn),
		writeCh: make(chan func() error, 8),
		ctx:     ctx,
		cancel:  cancel,
	}

	go sess.writeLoop()
	defer sess.close()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := client.SetDeadline(time.Now().Add(m.idleTimeout)); err != nil {
			m.logf("set deadline error: %v", err)
			return
		}

		req, err := client.ReadRequest()
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				var netErr net.Error
				if !(errors.As(err, &netErr) && netErr.Timeout()) {
					m.logf("read request error: %v", err)
				}
			}
			return
		}

		if req.Method == http.MethodConnect {
			if err := client.upgradeTLS(m.ca, req.Host); err != nil {
				m.logf("upgrade client tls error: %v", err)
				return
			}
			continue
		}

		isWS := isWebSocketRequest(req)
		sess.handleRequest(req, isWS)

		if isWS {
			// WebSocket 升级为隧道后不再复用该连接读取后续请求
			return
		}
	}
}

func (m *MITM) logf(format string, v ...interface{}) {
	m.logger.Printf(format, v...)
}

func (m *MITM) logln(v ...interface{}) {
	m.logger.Println(v...)
}

// mustManageRequest 判断请求是否应路由到本地管理接口。
func (m *MITM) mustManageRequest(req *http.Request) bool {
	host, port := hostPort(req.Host, false)
	_, ok := m.localIPs[host]
	if !ok {
		return false
	}
	m.mu.Lock()
	listenPort := m.listenPort
	m.mu.Unlock()
	return port == listenPort
}

// ListenAddr 返回当前监听地址；未启动时返回 nil。
func (m *MITM) ListenAddr() net.Addr {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.listener == nil {
		return nil
	}
	return m.listener.Addr()
}

// SetLogOutput 兼容旧接口：设置日志输出到指定 writer；传入 nil 关闭日志。
func (m *MITM) SetLogOutput(w *os.File) {
	if w == nil {
		m.logger = log.New(io.Discard, "", 0)
	} else {
		m.logger = log.New(w, "[mitm] ", log.LstdFlags)
	}
}
