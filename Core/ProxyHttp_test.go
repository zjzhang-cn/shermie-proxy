package Core

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestRemoveHeader(t *testing.T) {
	proxy := &ProxyHttp{}

	t.Run("removes standard proxy headers", func(t *testing.T) {
		header := http.Header{
			"Keep-Alive":          []string{"timeout=5"},
			"Transfer-Encoding":   []string{"chunked"},
			"TE":                  []string{"trailers"},
			"Connection":          []string{"keep-alive"},
			"Trailer":             []string{"X-Trailer"},
			"Upgrade":             []string{"websocket"},
			"Proxy-Authorization": []string{"Basic xxx"},
			"Proxy-Authenticate":  []string{"Basic"},
			"Accept-Encoding":     []string{"gzip"},
			"Content-Type":        []string{"application/json"},
		}
		proxy.RemoveHeader(header)

		removedHeaders := []string{
			"Keep-Alive", "Transfer-Encoding", "TE", "Connection",
			"Trailer", "Upgrade", "Proxy-Authorization",
			"Proxy-Authenticate", "Accept-Encoding",
		}
		for _, h := range removedHeaders {
			if v := header.Get(h); v != "" {
				t.Errorf("header %q should be removed, got %q", h, v)
			}
		}
		// Content-Type should remain
		if header.Get("Content-Type") != "application/json" {
			t.Error("Content-Type should not be removed")
		}
	})

	t.Run("empty headers", func(t *testing.T) {
		header := http.Header{}
		proxy.RemoveHeader(header) // should not panic
	})

	t.Run("only non-removable headers", func(t *testing.T) {
		header := http.Header{
			"Content-Type":  []string{"text/html"},
			"X-Custom":      []string{"value"},
			"Authorization": []string{"Bearer token"},
		}
		proxy.RemoveHeader(header)
		if header.Get("Content-Type") != "text/html" {
			t.Error("Content-Type should remain")
		}
		if header.Get("X-Custom") != "value" {
			t.Error("X-Custom should remain")
		}
		if header.Get("Authorization") != "Bearer token" {
			t.Error("Authorization should remain")
		}
	})
}

func TestReadResponseBody_PlainText(t *testing.T) {
	proxy := &ProxyHttp{}
	body := "Hello, World!"
	resp := &http.Response{
		Header: http.Header{
			"Content-Type": []string{"text/plain"},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}
	data, err := proxy.ReadResponseBody(resp)
	if err != nil {
		t.Fatalf("ReadResponseBody() error: %v", err)
	}
	if string(data) != body {
		t.Errorf("got %q, want %q", string(data), body)
	}
}

func TestReadResponseBody_Gzip(t *testing.T) {
	proxy := &ProxyHttp{}
	original := "compressed content"

	// 构造 gzip 压缩数据
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	gzWriter.Write([]byte(original))
	gzWriter.Close()

	resp := &http.Response{
		Header: http.Header{
			"Content-Encoding": []string{"gzip"},
		},
		Body: io.NopCloser(&buf),
	}

	data, err := proxy.ReadResponseBody(resp)
	if err != nil {
		t.Fatalf("ReadResponseBody() error: %v", err)
	}
	if string(data) != original {
		t.Errorf("got %q, want %q", string(data), original)
	}
}

func TestReadResponseBody_GzipInvalid(t *testing.T) {
	proxy := &ProxyHttp{}
	resp := &http.Response{
		Header: http.Header{
			"Content-Encoding": []string{"gzip"},
		},
		Body: io.NopCloser(strings.NewReader("not gzip data")),
	}

	data, err := proxy.ReadResponseBody(resp)
	if err != nil {
		t.Fatalf("ReadResponseBody() should not return error for invalid gzip: %v", err)
	}
	// 函数返回空切片
	if len(data) != 0 {
		t.Errorf("expected empty body for invalid gzip, got %d bytes", len(data))
	}
}

func TestReadRequestBody_NilReader(t *testing.T) {
	proxy := &ProxyHttp{}
	data, err := proxy.ReadRequestBody(nil)
	if err != nil {
		t.Fatalf("ReadRequestBody(nil) error: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected empty body, got %d bytes", len(data))
	}
}

func TestReadRequestBody_WithData(t *testing.T) {
	proxy := &ProxyHttp{}
	body := "request body data"
	data, err := proxy.ReadRequestBody(strings.NewReader(body))
	if err != nil {
		t.Fatalf("ReadRequestBody() error: %v", err)
	}
	if string(data) != body {
		t.Errorf("got %q, want %q", string(data), body)
	}
}
