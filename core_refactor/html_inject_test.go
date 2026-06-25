package core_refactor

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestHTMLInjector(t *testing.T) {
	injector := NewHTMLInjector(func(resp *http.Response) string {
		return "<script>injected</script>"
	})

	resp := &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
		Body:       io.NopCloser(strings.NewReader("<html><body></body></html>")),
	}

	if err := injector.Inject(resp); err != nil {
		t.Fatalf("inject: %v", err)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "<script>injected</script>") {
		t.Fatalf("body not injected: %q", string(body))
	}
	if cl := resp.Header.Get("Content-Length"); cl == "" {
		t.Fatal("expected Content-Length after injection")
	}
}

func TestHTMLInjectorNonHTML(t *testing.T) {
	injector := NewHTMLInjector(func(resp *http.Response) string {
		return "<script>injected</script>"
	})

	original := `{"ok":true}`
	resp := &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(original)),
	}

	if err := injector.Inject(resp); err != nil {
		t.Fatalf("inject: %v", err)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != original {
		t.Fatalf("body should remain unchanged for non-html: %q", string(body))
	}
}
