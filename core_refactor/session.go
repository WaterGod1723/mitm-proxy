package core_refactor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

// session 对应一个客户端连接的生命周期，负责请求串行读取、响应顺序写入、
// 上游连接复用以及 WebSocket 隧道。
type session struct {
	mitm    *MITM
	client  *clientConn
	servers map[string]*serverConn
	writeCh chan func() error
	mu      sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
}

func (s *session) writeLoop() {
	for fn := range s.writeCh {
		if err := fn(); err != nil {
			s.mitm.logf("write to client error: %v", err)
			s.cancel()
			return
		}
	}
}

func (s *session) submit(fn func() error) {
	select {
	case s.writeCh <- fn:
	case <-s.ctx.Done():
	}
}

func (s *session) close() {
	s.mu.Lock()
	for _, srv := range s.servers {
		srv.Close()
	}
	s.mu.Unlock()
	s.client.Close()
	close(s.writeCh)
}

func (s *session) handleRequest(req *http.Request, isWS bool) {
	defer req.Body.Close()

	// 缓存小请求体，便于上游复用连接失效时安全重试。
	if !isWS && req.ContentLength >= 0 && req.ContentLength < 1<<18 {
		if req.ContentLength > 0 {
			body, err := io.ReadAll(req.Body)
			if err != nil {
				s.mitm.logf("read request body error: %v", err)
				s.submit(func() error {
					w := NewResponseWriter(s.client)
					w.WriteHeader(http.StatusBadGateway)
					w.Header().Set("Content-Type", "text/plain; charset=utf-8")
					_, werr := w.Write([]byte(err.Error()))
					return werr
				})
				return
			}
			req.Body = io.NopCloser(bytes.NewReader(body))
		}
	}

	if s.mitm.mustManageRequest(req) {
		s.handleManage(req)
		return
	}

	if s.mitm.htmlInjector != nil {
		EnableCompressionHint(req)
	}

	if req.URL.Scheme == "" {
		if s.client.isTLS {
			req.URL.Scheme = "https"
		} else {
			req.URL.Scheme = "http"
		}
	}

	if s.mitm.requestHandler != nil {
		if resp := s.mitm.requestHandler(req); resp != nil {
			s.submit(func() error {
				defer resp.Body.Close()
				return resp.Write(s.client)
			})
			return
		}
	}

	resp, err := s.forward(req, isWS)
	if err != nil {
		s.mitm.logf("forward request error: %v", err)
		s.submit(func() error {
			w := NewResponseWriter(s.client)
			w.WriteHeader(http.StatusBadGateway)
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, werr := w.Write([]byte(err.Error()))
			return werr
		})
		return
	}

	if s.mitm.responseHandler != nil {
		if replaced := s.mitm.responseHandler(resp); replaced != nil {
			resp.Body.Close()
			resp = replaced
		}
	}

	if s.mitm.htmlInjector != nil {
		if err := s.mitm.htmlInjector.Inject(resp); err != nil {
			s.mitm.logf("html inject error: %v", err)
		}
	}

	if isWS {
		s.handleWebSocket(req, resp)
		return
	}

	s.submit(func() error {
		defer resp.Body.Close()
		if err := resp.Write(s.client); err != nil {
			return err
		}
		s.mitm.logln(s.client.RemoteAddr(), req.URL, resp.Status)
		return nil
	})
}

func (s *session) forward(req *http.Request, isWS bool) (*http.Response, error) {
	proxy := s.mitm.proxyFunc(req)
	srv, err := s.getServer(req, proxy)
	if err != nil {
		return nil, err
	}

	if err := s.writeRequest(req, srv); err != nil {
		s.removeServer(req.Host)
		// 上游复用连接可能已被对端关闭，重试一次。
		srv, err = s.getServer(req, proxy)
		if err != nil {
			return nil, err
		}
		if err := s.writeRequest(req, srv); err != nil {
			s.removeServer(req.Host)
			return nil, err
		}
	}

	resp, err := srv.ReadResponse(req)
	if err != nil {
		s.removeServer(req.Host)
		// 上游复用连接可能已被对端关闭，重试一次。
		srv, err = s.getServer(req, proxy)
		if err != nil {
			return nil, err
		}
		if err := s.writeRequest(req, srv); err != nil {
			s.removeServer(req.Host)
			return nil, err
		}
		resp, err = srv.ReadResponse(req)
		if err != nil {
			s.removeServer(req.Host)
			return nil, err
		}
	}
	return resp, nil
}

func (s *session) getServer(req *http.Request, proxy Proxy) (*serverConn, error) {
	targetHost := req.Host
	targetPort := targetPort(req, s.client.isTLS)

	s.mu.Lock()
	defer s.mu.Unlock()

	key := req.Host
	srv := s.servers[key]
	if srv != nil {
		return srv, nil
	}

	target := targetHost
	if _, _, err := net.SplitHostPort(targetHost); err != nil {
		target = net.JoinHostPort(targetHost, targetPort)
	}

	srv, err := dialServer(target, proxy, s.mitm.dialTimeout)
	if err != nil {
		return nil, err
	}

	if req.URL.Scheme == "https" || (req.URL.Scheme == "" && s.client.isTLS) {
		if err := srv.upgradeTLS(target); err != nil {
			srv.Close()
			return nil, err
		}
	}

	s.servers[key] = srv
	return srv, nil
}

func (s *session) removeServer(host string) {
	s.mu.Lock()
	srv := s.servers[host]
	delete(s.servers, host)
	s.mu.Unlock()
	if srv != nil {
		srv.Close()
	}
}

func (s *session) writeRequest(req *http.Request, srv *serverConn) error {
	out := req.Clone(context.Background())
	defer out.Body.Close()
	return out.Write(srv)
}

func (s *session) handleWebSocket(req *http.Request, resp *http.Response) {
	err := func() error {
		defer resp.Body.Close()
		if err := resp.Write(s.client); err != nil {
			return err
		}
		if resp.StatusCode != http.StatusSwitchingProtocols {
			return fmt.Errorf("websocket upgrade failed: %s", resp.Status)
		}
		return nil
	}()
	if err != nil {
		s.mitm.logf("websocket handshake error: %v", err)
		return
	}

	_ = s.client.SetDeadline(time.Time{})
	srv := s.servers[req.Host]
	if srv == nil {
		s.mitm.logf("websocket server connection missing")
		return
	}

	s.mitm.logf("websocket tunnel: %s", req.Host)
	var wg sync.WaitGroup
	wg.Add(2)
	var closeOnce sync.Once
	closeBoth := func() {
		closeOnce.Do(func() {
			s.client.Close()
			srv.Close()
		})
	}
	go func() {
		defer wg.Done()
		_, err := io.Copy(s.client, srv)
		if err != nil {
			s.mitm.logf("websocket copy server->client error: %v", err)
		}
		closeBoth()
	}()
	go func() {
		defer wg.Done()
		_, err := io.Copy(srv, s.client)
		if err != nil {
			s.mitm.logf("websocket copy client->server error: %v", err)
		}
		closeBoth()
	}()
	wg.Wait()
}

func (s *session) handleManage(req *http.Request) {
	w := NewResponseWriter(s.client)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Max-Age", "86400")
	w.Header().Set("Access-Control-Allow-Credentials", "true")

	remoteIP, _, _ := net.SplitHostPort(s.client.RemoteAddr().String())
	if ip := net.ParseIP(remoteIP); ip != nil && !ip.IsLoopback() {
		w.WriteHeader(http.StatusNotFound)
		s.submit(func() error { _, err := w.Write([]byte("404 not found")); return err })
		return
	}

	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		s.submit(func() error { _, err := w.Write(nil); return err })
		return
	}

	s.mitm.mu.Lock()
	h := s.mitm.manageRouter[req.URL.Path]
	s.mitm.mu.Unlock()

	if h != nil {
		s.submit(func() error { h(w, req); return nil })
	} else {
		w.WriteHeader(http.StatusNoContent)
		s.submit(func() error { _, err := w.Write(nil); return err })
	}
}
