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
