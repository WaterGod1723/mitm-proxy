package internal

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/WaterGod1723/mitm-proxy/core_refactor"
	lua "github.com/yuin/gopher-lua"
)

// NewRequestHandler 创建基于 Lua 的请求重写处理器。
// Lua 返回 bodyFilePath 时直接短路响应；否则修改 req 的协议、Host、Path 和 Header。
func NewRequestHandler(pool *Pool, logger *log.Logger) func(*http.Request) *http.Response {
	return func(req *http.Request) *http.Response {
		L, err := pool.Get()
		if err != nil {
			logger.Printf("lua pool get error: %v", err)
			return nil
		}
		defer SafePut(pool, L, logger)

		headersTbl := GoHeadersToLua(L, req.Header)
		fn := L.GetGlobal("GoRequest")
		if fn == lua.LNil {
			logger.Println("lua function GoRequest not found")
			return nil
		}

		err = L.CallByParam(lua.P{Fn: fn, NRet: 5, Protect: true},
			lua.LString(req.URL.Scheme),
			lua.LString(req.Host),
			lua.LString(req.URL.Path),
			headersTbl,
		)
		if err != nil {
			logger.Printf("GoRequest error: %v", err)
			return nil
		}

		protocol := L.ToString(-5)
		host := L.ToString(-4)
		path := L.ToString(-3)
		bodyFilePath := L.ToString(-2)

		// 解析 Lua 返回的 headers（完整替换）。
		var newHeaders http.Header
		if tbl, ok := L.Get(-1).(*lua.LTable); ok {
			newHeaders = LuaTableToGoHeaders(tbl)
		}
		L.Pop(5)

		// 直接返回本地文件作为响应；此时 headers 作为响应头写入。
		if bodyFilePath != "" {
			return buildFileResponse(req, bodyFilePath, newHeaders, logger)
		}

		// 应用 headers 到请求。
		if len(newHeaders) > 0 {
			req.Header = newHeaders
		}

		// 重写目标地址。
		if protocol != "" {
			req.URL.Scheme = protocol
		}
		if host != "" {
			req.URL.Host = host
			req.Host = host
		}
		if path != "" {
			req.URL.Path = path
		}
		return nil
	}
}

// buildFileResponse 将本地文件构造为 HTTP 响应。
func buildFileResponse(req *http.Request, path string, respHeaders http.Header, logger *log.Logger) *http.Response {
	data, err := os.ReadFile(path)
	if err != nil {
		logger.Printf("read mock file %q error: %v", path, err)
		return errorResponse(req, http.StatusNotFound, fmt.Sprintf("mock file not found: %s", path))
	}

	if respHeaders == nil {
		respHeaders = http.Header{}
	}
	if respHeaders.Get("Content-Type") == "" {
		respHeaders.Set("Content-Type", "application/json; charset=utf-8")
	}

	return &http.Response{
		StatusCode:    http.StatusOK,
		ProtoMajor:    1,
		ProtoMinor:    1,
		Request:       req,
		Header:        respHeaders,
		Body:          io.NopCloser(bytes.NewReader(data)),
		ContentLength: int64(len(data)),
	}
}

// errorResponse 构造一个短路的错误响应。
func errorResponse(req *http.Request, code int, msg string) *http.Response {
	return &http.Response{
		StatusCode:    code,
		ProtoMajor:    1,
		ProtoMinor:    1,
		Request:       req,
		Header:        http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}},
		Body:          io.NopCloser(bytes.NewReader([]byte(msg))),
		ContentLength: int64(len(msg)),
	}
}

// NewProxySelector 创建基于 Lua 的代理选择器。
func NewProxySelector(pool *Pool, logger *log.Logger) core_refactor.ProxyFunc {
	return func(req *http.Request) core_refactor.Proxy {
		host := strings.Split(req.Host, ":")[0]

		L, err := pool.Get()
		if err != nil {
			logger.Printf("lua pool get error: %v", err)
			return core_refactor.Proxy{}
		}
		defer SafePut(pool, L, logger)

		s, err := CallLuaString(L, "GoProxy", lua.LString(host))
		if err != nil {
			logger.Printf("GoProxy error: %v", err)
			return core_refactor.Proxy{}
		}

		p, err := core_refactor.ParseProxyURL(s)
		if err != nil {
			logger.Printf("parse proxy url %q error: %v", s, err)
			return core_refactor.Proxy{}
		}
		return p
	}
}

// NewHTMLInjector 创建基于 Lua 的 HTML 注入器。
func NewHTMLInjector(pool *Pool, logger *log.Logger) func(*http.Response) string {
	return func(resp *http.Response) string {
		host := ""
		if resp.Request != nil {
			host = strings.Split(resp.Request.Host, ":")[0]
		}

		L, err := pool.Get()
		if err != nil {
			logger.Printf("lua pool get error: %v", err)
			return ""
		}
		defer SafePut(pool, L, logger)

		path, err := CallLuaString(L, "GoInject", lua.LString(host))
		if err != nil {
			logger.Printf("GoInject error: %v", err)
			return ""
		}
		if path == "" {
			return ""
		}

		data, err := os.ReadFile(path)
		if err != nil {
			logger.Printf("read inject file %q error: %v", path, err)
			return ""
		}
		return string(data)
	}
}

// NewResponseHandler 预留的响应后处理钩子，当前直接透传。
func NewResponseHandler(_ *Pool, _ *log.Logger) func(*http.Response) *http.Response {
	return func(resp *http.Response) *http.Response {
		return nil
	}
}
