package core_refactor

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
)

// Proxy 描述一个上游代理服务器。空值表示直连。
type Proxy struct {
	Scheme   string
	Host     string
	Username string
	Password string
}

// ProxyFunc 根据请求决定使用哪个上游代理。
type ProxyFunc func(req *http.Request) Proxy

// DirectProxy 返回空代理，表示直连目标服务器。
func DirectProxy(_ *http.Request) Proxy { return Proxy{} }

// IsDirect 判断该代理是否为空（直连）。
func (p Proxy) IsDirect() bool {
	return p.Host == ""
}

// BasicAuth 返回代理 Basic 认证头内容（含 "Basic " 前缀）。
func (p Proxy) BasicAuth() string {
	if p.Username == "" && p.Password == "" {
		return ""
	}
	auth := p.Username + ":" + p.Password
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
}

// Address 返回用于 TCP 拨号的地址，若为空则返回直连标记。
func (p Proxy) Address() string {
	if p.Host == "" {
		return ""
	}
	return p.Host
}

// ParseProxyURL 将代理 URL 字符串解析为 Proxy。
func ParseProxyURL(s string) (Proxy, error) {
	if s == "" {
		return Proxy{}, nil
	}
	u, err := url.Parse(s)
	if err != nil {
		return Proxy{}, fmt.Errorf("parse proxy url: %w", err)
	}
	p := Proxy{
		Scheme: u.Scheme,
		Host:   u.Host,
	}
	if u.User != nil {
		p.Username = u.User.Username()
		p.Password, _ = u.User.Password()
	}
	return p, nil
}
