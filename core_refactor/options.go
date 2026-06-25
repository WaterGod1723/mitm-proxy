package core_refactor

import (
	"log"
	"net/http"
	"time"
)

// Option 用于配置 MITM 代理。
type Option func(*MITM)

// WithLogger 设置日志输出器；nil 表示关闭日志。
func WithLogger(l *log.Logger) Option {
	return func(m *MITM) {
		m.logger = l
	}
}

// WithCA 指定根证书签名器；不指定时 MITM 启动会从默认路径加载。
func WithCA(ca *CA) Option {
	return func(m *MITM) {
		m.ca = ca
	}
}

// WithRequestHandler 设置请求预处理钩子。
func WithRequestHandler(fn func(*http.Request) *http.Response) Option {
	return func(m *MITM) {
		m.requestHandler = fn
	}
}

// WithResponseHandler 设置响应后处理钩子。
func WithResponseHandler(fn func(*http.Response) *http.Response) Option {
	return func(m *MITM) {
		m.responseHandler = fn
	}
}

// WithHTMLInjector 设置 HTML 注入器；当返回值非空时会在 </body> 前插入。
func WithHTMLInjector(fn func(*http.Response) string) Option {
	return func(m *MITM) {
		m.htmlInjector = NewHTMLInjector(fn)
	}
}

// WithProxy 设置上游代理选择器。
func WithProxy(fn ProxyFunc) Option {
	return func(m *MITM) {
		m.proxyFunc = fn
	}
}

// WithDialTimeout 设置连接上游服务器的超时时间。
func WithDialTimeout(d time.Duration) Option {
	return func(m *MITM) {
		m.dialTimeout = d
	}
}

// WithIdleTimeout 设置客户端连接空闲超时时间。
func WithIdleTimeout(d time.Duration) Option {
	return func(m *MITM) {
		m.idleTimeout = d
	}
}

// WithCAPath 显式指定根证书路径。
func WithCAPath(certPath, keyPath string) Option {
	return func(m *MITM) {
		m.certPath = certPath
		m.keyPath = keyPath
	}
}
