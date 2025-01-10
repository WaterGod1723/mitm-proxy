package util

import (
	"fmt"
	"io"
	"net/http"
)

type ResponseWriter struct {
	writer io.Writer
	header http.Header
	status int
}

func NewResponseWriter(w io.Writer) *ResponseWriter {
	return &ResponseWriter{
		writer: w,
	}
}

func (w *ResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *ResponseWriter) SetStatus(statusCode int) {
	w.status = statusCode
}

func (w *ResponseWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK // 默认状态码
	}

	// 写入状态行
	statusLine := fmt.Sprintf("HTTP/1.1 %d %s\r\n", w.status, http.StatusText(w.status))
	if _, err := w.writer.Write([]byte(statusLine)); err != nil {
		return 0, err
	}

	if len(data) > 0 && w.header.Get("Content-type") == "" {
		w.header.Set("Content-type", "text/plain")
	}

	// 写入头部
	if w.header != nil {
		// 如果未设置 Content-Length 且未启用分块传输，则根据数据大小决定
		if w.header.Get("Content-Length") == "" && w.header.Get("Transfer-Encoding") != "chunked" {
			if len(data) < 1024 { // 小数据直接使用 Content-Length
				w.header.Set("Content-Length", fmt.Sprintf("%d", len(data)))
			} else { // 大数据使用分块传输
				w.header.Set("Transfer-Encoding", "chunked")
			}
		}

		for key, values := range w.header {
			for _, value := range values {
				headerLine := fmt.Sprintf("%s: %s\r\n", key, value)
				if _, err := w.writer.Write([]byte(headerLine)); err != nil {
					return 0, err
				}
			}
		}
	}

	// 写入空行
	if _, err := w.writer.Write([]byte("\r\n")); err != nil {
		return 0, err
	}

	// 写入响应体
	if w.header.Get("Transfer-Encoding") == "chunked" {
		// 分块传输
		chunkHeader := fmt.Sprintf("%x\r\n", len(data)) // 块大小
		if _, err := w.writer.Write([]byte(chunkHeader)); err != nil {
			return 0, err
		}
		if _, err := w.writer.Write(data); err != nil {
			return 0, err
		}
		if _, err := w.writer.Write([]byte("\r\n")); err != nil {
			return 0, err
		}
		// 结束块
		if _, err := w.writer.Write([]byte("0\r\n\r\n")); err != nil {
			return 0, err
		}
	} else {
		// 普通传输
		if _, err := w.writer.Write(data); err != nil {
			return 0, err
		}
	}

	return len(data), nil
}
