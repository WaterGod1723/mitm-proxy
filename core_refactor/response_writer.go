package core_refactor

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
)

// ResponseWriter 实现了标准 http.ResponseWriter 接口，用于直接向客户端连接写响应。
// 与原实现不同，这里将 header 写入延迟到第一次 Write，从而可以自动计算 Content-Length。
type ResponseWriter struct {
	writer        io.Writer
	header        http.Header
	status        int
	headerWritten bool
}

// NewResponseWriter 构造一个 ResponseWriter。
func NewResponseWriter(w io.Writer) *ResponseWriter {
	return &ResponseWriter{
		writer: w,
		header: make(http.Header),
		status: http.StatusOK,
	}
}

// Header 返回可修改的响应头。
func (w *ResponseWriter) Header() http.Header {
	return w.header
}

// WriteHeader 设置响应状态码；真正的头刷新会延迟到 Write。
func (w *ResponseWriter) WriteHeader(statusCode int) {
	if w.headerWritten {
		return
	}
	w.status = statusCode
}

// Write 写入响应体，并在首次调用时自动刷新状态行与头部。
func (w *ResponseWriter) Write(p []byte) (int, error) {
	if !w.headerWritten {
		if w.header.Get("Content-Length") == "" && w.header.Get("Transfer-Encoding") == "" {
			w.header.Set("Content-Length", fmt.Sprintf("%d", len(p)))
		}
		if len(p) > 0 && w.header.Get("Content-Type") == "" {
			w.header.Set("Content-Type", "text/plain; charset=utf-8")
		}
		fmt.Fprintf(w.writer, "HTTP/1.1 %d %s\r\n", w.status, http.StatusText(w.status))
		w.header.Write(w.writer)
		w.writer.Write([]byte("\r\n"))
		w.headerWritten = true
	}
	return w.writer.Write(p)
}

// Flush 实现 http.Flusher 接口，如果底层 writer 支持则刷新。
func (w *ResponseWriter) Flush() {
	if f, ok := w.writer.(http.Flusher); ok {
		f.Flush()
	} else if f, ok := w.writer.(*bufio.Writer); ok {
		f.Flush()
	}
}
