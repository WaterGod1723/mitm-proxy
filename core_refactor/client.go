package core_refactor

import (
	"bufio"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"time"
)

// clientConn 封装与浏览器/客户端之间的连接。
type clientConn struct {
	raw     net.Conn
	reader  *bufio.Reader
	tlsConn *tls.Conn
	isTLS   bool
}

func newClientConn(conn net.Conn) *clientConn {
	return &clientConn{
		raw:    conn,
		reader: bufio.NewReader(conn),
	}
}

func (c *clientConn) ReadRequest() (*http.Request, error) {
	return http.ReadRequest(c.reader)
}

func (c *clientConn) Read(p []byte) (int, error) {
	if c.isTLS {
		return c.tlsConn.Read(p)
	}
	return c.raw.Read(p)
}

func (c *clientConn) Write(p []byte) (int, error) {
	if c.isTLS {
		return c.tlsConn.Write(p)
	}
	return c.raw.Write(p)
}

func (c *clientConn) RemoteAddr() net.Addr {
	return c.raw.RemoteAddr()
}

func (c *clientConn) SetDeadline(t time.Time) error {
	if c.isTLS {
		return c.tlsConn.SetDeadline(t)
	}
	return c.raw.SetDeadline(t)
}

func (c *clientConn) Close() error {
	if c.isTLS && c.tlsConn != nil {
		c.tlsConn.Close()
	}
	return c.raw.Close()
}

func (c *clientConn) upgradeTLS(ca *CA, host string) error {
	if _, err := c.raw.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		return err
	}

	hostname, _, err := net.SplitHostPort(host)
	if err != nil {
		hostname = host
	}

	cert, err := ca.SignHost([]string{hostname})
	if err != nil {
		return err
	}

	tlsConn := tls.Server(c.raw, &tls.Config{
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true,
	})
	if err := tlsConn.Handshake(); err != nil {
		return err
	}

	c.tlsConn = tlsConn
	c.isTLS = true
	c.reader = bufio.NewReader(tlsConn)
	return nil
}

var _ io.Reader = (*clientConn)(nil)
var _ io.Writer = (*clientConn)(nil)
