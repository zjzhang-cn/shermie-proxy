package Core

import "testing"

func TestIsHttpMethod(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  bool
	}{
		// 标准 HTTP 方法
		{"GET", []byte("GET /index.html HTTP/1.1"), true},
		{"POST", []byte("POST /api/data HTTP/1.1"), true},
		{"PUT", []byte("PUT /api/resource HTTP/1.1"), true},
		{"DELETE", []byte("DELETE /api/resource HTTP/1.1"), true},
		{"OPTIONS", []byte("OPTIONS * HTTP/1.1"), true},
		{"HEAD", []byte("HEAD /index.html HTTP/1.1"), true},
		{"CONNECT", []byte("CONNECT example.com:443 HTTP/1.1"), true},
		{"PATCH", []byte("PATCH /api/resource HTTP/1.1"), true},
		{"TRACE", []byte("TRACE / HTTP/1.1"), true},

		// 非 HTTP 数据
		{"SOCKS5 version byte", []byte{0x05, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, false},
		{"random binary", []byte{0xFF, 0xFE, 0xFD, 0xFC, 0xFB, 0xFA, 0xF9, 0xF8, 0xF7}, false},
		{"empty", []byte{}, false},
		{"short input", []byte("GE"), false},

		// 方法名存在但没有尾部空格（不完整）
		{"GET without space", []byte("GETX /path HTTP/1.1"), false},
		{"POST without space", []byte("POSTX /path HTTP/1.1"), false},

		// 小写方法（HTTP 方法是大写的）
		{"lowercase get", []byte("get /path HTTP/1.1"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isHttpMethod(tt.input)
			if got != tt.want {
				t.Errorf("isHttpMethod(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsHttpMethod_DistinguishesPostAndPut(t *testing.T) {
	postData := []byte("POST /api HTTP/1.1\r\n")
	putData := []byte("PUT /api HTTP/1.1\r\n")

	if !isHttpMethod(postData) {
		t.Error("POST should be recognized as HTTP method")
	}
	if !isHttpMethod(putData) {
		t.Error("PUT should be recognized as HTTP method")
	}
}

func TestNewProxyServer(t *testing.T) {
	s := NewProxyServer("8080", true, "proxy:8080", "target:9090", "192.168.1.1")
	if s.port != "8080" {
		t.Errorf("port = %q, want %q", s.port, "8080")
	}
	if s.nagle != true {
		t.Errorf("nagle = %v, want true", s.nagle)
	}
	if s.proxy != "proxy:8080" {
		t.Errorf("proxy = %q, want %q", s.proxy, "proxy:8080")
	}
	if s.to != "target:9090" {
		t.Errorf("to = %q, want %q", s.to, "target:9090")
	}
	if s.network != "192.168.1.1" {
		t.Errorf("network = %q, want %q", s.network, "192.168.1.1")
	}
	if s.dns == nil {
		t.Error("dns resolver should not be nil")
	}
}
