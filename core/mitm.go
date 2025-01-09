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
