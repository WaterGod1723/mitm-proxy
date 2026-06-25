package core_refactor

import (
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// HTMLInjector 用于在 HTML 响应 </body> 前注入内容。
type HTMLInjector struct {
	fn func(*http.Response) string
}

// NewHTMLInjector 构造注入器。
func NewHTMLInjector(fn func(*http.Response) string) *HTMLInjector {
	return &HTMLInjector{fn: fn}
}

// ShouldInject 判断是否需要注入（仅 text/html）。
func (h *HTMLInjector) ShouldInject(resp *http.Response) bool {
	ct := resp.Header.Get("Content-Type")
	return strings.Contains(ct, "text/html")
}

// Inject 执行注入并返回新的响应体；若响应不是 HTML 或没有 </body> 则返回原响应体。
func (h *HTMLInjector) Inject(resp *http.Response) error {
	if h == nil || h.fn == nil {
		return nil
	}
	if !h.ShouldInject(resp) {
		return nil
	}

	bodyStr, err := decompressBody(resp)
	if err != nil {
		return err
	}

	resp.Header.Del("Content-Security-Policy")
	resp.Header.Del("Content-Encoding")

	idx := strings.LastIndex(bodyStr, "</body>")
	if idx == -1 {
		resp.Body = io.NopCloser(strings.NewReader(bodyStr))
		resp.ContentLength = int64(len(bodyStr))
		return nil
	}

	injected := bodyStr[:idx] + h.fn(resp) + bodyStr[idx:]
	resp.Body = io.NopCloser(strings.NewReader(injected))
	resp.ContentLength = int64(len(injected))
	resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(injected)))
	return nil
}

func decompressBody(resp *http.Response) (string, error) {
	encoding := strings.ToLower(resp.Header.Get("Content-Encoding"))
	var reader io.ReadCloser
	var err error
	switch encoding {
	case "gzip":
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return "", fmt.Errorf("gzip reader: %w", err)
		}
		defer reader.Close()
	case "deflate":
		reader, err = zlib.NewReader(resp.Body)
		if err != nil {
			return "", fmt.Errorf("zlib reader: %w", err)
		}
		defer reader.Close()
	default:
		reader = resp.Body
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}
	return string(body), nil
}

// EnableCompressionHint 当启用 HTML 注入时，强制客户端接受 gzip/deflate，便于后续解压缩。
func EnableCompressionHint(req *http.Request) {
	req.Header.Set("Accept-Encoding", "gzip, deflate")
}
