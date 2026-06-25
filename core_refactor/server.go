package core_refactor

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// serverConn 封装与上游目标服务器之间的连接。
type serverConn struct {
	raw     net.Conn
	reader  *bufio.Reader
	tlsConn *tls.Conn
	isTLS   bool
	host    string // 原始请求 Host（不含端口）
}

// dialServer 建立到目标服务器的 TCP 连接；若指定了上游代理，则先连接到代理。
func dialServer(target string, proxy Proxy, timeout time.Duration) (*serverConn, error) {
	addr := target
	if !strings.Contains(addr, ":") {
		addr += ":80"
	}

	dialAddr := addr
	if !proxy.IsDirect() {
		dialAddr = proxy.Host
	}

	conn, err := net.DialTimeout("tcp", dialAddr, timeout)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", dialAddr, err)
	}

	sc := &serverConn{
		raw:    conn,
		reader: bufio.NewReader(conn),
		host:   target,
	}

	if !proxy.IsDirect() {
		if err := sc.connectViaProxy(addr, proxy); err != nil {
			sc.Close()
			return nil, err
		}
	}

	return sc, nil
}

func (s *serverConn) connectViaProxy(targetAddr string, proxy Proxy) error {
	if !strings.Contains(targetAddr, ":") {
		targetAddr += ":443"
	}
	req := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", targetAddr, targetAddr)
	if auth := proxy.BasicAuth(); auth != "" {
		req += "Proxy-Authorization: " + auth + "\r\n"
	}
	req += "\r\n"

	if _, err := s.raw.Write([]byte(req)); err != nil {
		return fmt.Errorf("write CONNECT: %w", err)
	}

	resp, err := http.ReadResponse(s.reader, nil)
	if err != nil {
		return fmt.Errorf("read CONNECT response: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("proxy CONNECT failed: %s", resp.Status)
	}
	return nil
}

func (s *serverConn) upgradeTLS(targetAddr string) error {
	if !strings.Contains(targetAddr, ":") {
		targetAddr += ":443"
	}
	hostname, _, _ := net.SplitHostPort(targetAddr)
	if hostname == "" {
		hostname = targetAddr
	}

	tlsConn := tls.Client(s.raw, &tls.Config{
		ServerName:         hostname,
		InsecureSkipVerify: true,
	})
	if err := tlsConn.Handshake(); err != nil {
		return fmt.Errorf("tls handshake: %w", err)
	}

	s.tlsConn = tlsConn
	s.isTLS = true
	s.reader = bufio.NewReader(tlsConn)
	return nil
}

func (s *serverConn) ReadResponse(req *http.Request) (*http.Response, error) {
	return http.ReadResponse(s.reader, req)
}

func (s *serverConn) Read(p []byte) (int, error) {
	if s.isTLS {
		return s.tlsConn.Read(p)
	}
	return s.raw.Read(p)
}

func (s *serverConn) Write(p []byte) (int, error) {
	if s.isTLS {
		return s.tlsConn.Write(p)
	}
	return s.raw.Write(p)
}

func (s *serverConn) Close() error {
	if s.isTLS && s.tlsConn != nil {
		s.tlsConn.Close()
	}
	return s.raw.Close()
}

var _ io.Reader = (*serverConn)(nil)
var _ io.Writer = (*serverConn)(nil)
