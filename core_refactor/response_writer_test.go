package core_refactor

import (
	"bufio"
	"bytes"
	"net/http"
	"strings"
	"testing"
)

func TestResponseWriter(t *testing.T) {
	var buf bytes.Buffer
	w := NewResponseWriter(&buf)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if _, err := w.Write([]byte(`{"ok":true}`)); err != nil {
		t.Fatalf("write: %v", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(&buf), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status code = %d, want %d", resp.StatusCode, http.StatusCreated)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q, want application/json", ct)
	}
	body, _ := ioReadAll(resp.Body)
	if string(body) != `{"ok":true}` {
		t.Fatalf("body = %q", string(body))
	}
}

func TestResponseWriterImplicitOK(t *testing.T) {
	var buf bytes.Buffer
	w := NewResponseWriter(&buf)
	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(&buf), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestResponseWriterStatusText(t *testing.T) {
	var buf bytes.Buffer
	w := NewResponseWriter(&buf)
	w.WriteHeader(http.StatusNotFound)
	w.Write(nil)
	line := buf.String()
	if !strings.HasPrefix(line, "HTTP/1.1 404 Not Found") {
		t.Fatalf("unexpected status line: %q", line)
	}
}

func ioReadAll(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	return buf.Bytes(), err
}
