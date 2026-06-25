package core_refactor

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func startTestMITM(t *testing.T, setup func(*MITM), opts ...Option) string {
	t.Helper()
	certPath, keyPath := writeTestCAFiles(t)
	opts = append([]Option{WithCAPath(certPath, keyPath), WithLogger(nil)}, opts...)
	m, err := New(opts...)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if setup != nil {
		setup(m)
	}

	go func() {
		if err := m.Start("127.0.0.1:0"); err != nil {
			t.Logf("Start returned: %v", err)
		}
	}()

	var addr string
	for i := 0; i < 100; i++ {
		if a := m.ListenAddr(); a != nil {
			addr = a.String()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if addr == "" {
		t.Fatal("mitm did not start")
	}

	t.Cleanup(func() { _ = m.Stop() })
	return addr
}

func TestMITMManageEndpoint(t *testing.T) {
	addr := startTestMITM(t, func(m *MITM) {
		m.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("pong"))
		})
	})

	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	_, _ = fmt.Fprintf(conn, "GET /ping HTTP/1.1\r\nHost: 127.0.0.1:%s\r\n\r\n", strings.Split(addr, ":")[1])
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := ioReadAll(resp.Body)
	if string(body) != "pong" {
		t.Fatalf("body = %q, want pong", string(body))
	}
}

func TestMITMOptionsRequest(t *testing.T) {
	addr := startTestMITM(t, nil)

	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	_, _ = fmt.Fprintf(conn, "OPTIONS /ping HTTP/1.1\r\nHost: 127.0.0.1:%s\r\n\r\n", strings.Split(addr, ":")[1])
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("missing CORS header")
	}
}

// TestMITMRetryOnStaleUpstreamConnection 验证当上联复用连接被对端关闭时，
// 代理会自动重试新连接，而不是直接返回 502。
// TestMITMStopClosesActiveConnections 验证存在活跃客户端连接时，Stop 仍能及时退出。
func TestMITMStopClosesActiveConnections(t *testing.T) {
	certPath, keyPath := writeTestCAFiles(t)
	m, err := New(WithCAPath(certPath, keyPath), WithLogger(nil))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	go func() {
		if err := m.Start("127.0.0.1:0"); err != nil {
			t.Logf("Start returned: %v", err)
		}
	}()

	var addr string
	for i := 0; i < 100; i++ {
		if a := m.ListenAddr(); a != nil {
			addr = a.String()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if addr == "" {
		t.Fatal("mitm did not start")
	}

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	stopDone := make(chan error, 1)
	go func() {
		stopDone <- m.Stop()
	}()

	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatalf("Stop failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return with active connection")
	}

	// 确认活跃连接已被关闭。
	buf := make([]byte, 1)
	conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, err = conn.Read(buf)
	if err == nil {
		t.Fatal("expected active connection to be closed by Stop")
	}
}

func TestMITMRetryOnStaleUpstreamConnection(t *testing.T) {
	// 上游服务器：每个连接只处理一个请求，然后关闭，模拟 keep-alive 连接被服务端关闭。
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen upstream: %v", err)
	}
	defer ln.Close()

	var requestCount int
	var mu sync.Mutex

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				req, err := http.ReadRequest(bufio.NewReader(conn))
				if err != nil {
					return
				}
				io.Copy(io.Discard, req.Body)
				mu.Lock()
				requestCount++
				mu.Unlock()
				fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
			}(c)
		}
	}()

	addr := startTestMITM(t, nil)
	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	for i := 0; i < 2; i++ {
		_, err = fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: %s\r\n\r\n", ln.Addr().String())
		if err != nil {
			t.Fatalf("write request %d: %v", i, err)
		}
		resp, err := http.ReadResponse(reader, nil)
		if err != nil {
			t.Fatalf("read response %d: %v", i, err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("response %d status = %d, want 200", i, resp.StatusCode)
		}
	}

	mu.Lock()
	count := requestCount
	mu.Unlock()
	if count != 2 {
		t.Fatalf("upstream request count = %d, want 2", count)
	}
}
