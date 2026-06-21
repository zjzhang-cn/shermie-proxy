package Core

import "testing"

func TestByteToInt(t *testing.T) {
	socks := &ProxySocks5{}
	tests := []struct {
		name  string
		input []byte
		want  int32
	}{
		{"port 80", []byte{0x00, 0x50}, 80},
		{"port 443", []byte{0x01, 0xBB}, 443},
		{"port 8080", []byte{0x1F, 0x90}, 8080},
		{"port 0", []byte{0x00, 0x00}, 0},
		{"port 65535", []byte{0xFF, 0xFF}, 65535},
		{"port 1", []byte{0x00, 0x01}, 1},
		{"port 256", []byte{0x01, 0x00}, 256},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := socks.ByteToInt(tt.input)
			if got != tt.want {
				t.Errorf("ByteToInt(%v) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestIpV4(t *testing.T) {
	socks := &ProxySocks5{}
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"valid IPv4", "192.168.1.1", true},
		{"localhost IPv4", "127.0.0.1", true},
		{"zero IPv4", "0.0.0.0", true},
		{"broadcast", "255.255.255.255", true},
		{"valid IPv6", "::1", false},
		{"valid IPv6 full", "2001:db8::1", false},
		{"invalid", "not-an-ip", false},
		{"empty", "", false},
		{"domain", "example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := socks.IpV4(tt.ip)
			if got != tt.want {
				t.Errorf("IpV4(%q) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestIpV6(t *testing.T) {
	socks := &ProxySocks5{}
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"valid IPv6 loopback", "::1", true},
		{"valid IPv6 full", "2001:db8::1", true},
		{"valid IPv6 expanded", "2001:0db8:0000:0000:0000:0000:0000:0001", true},
		{"valid IPv4", "192.168.1.1", false},
		{"invalid", "not-an-ip", false},
		{"empty", "", false},
		{"domain", "example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := socks.IpV6(tt.ip)
			if got != tt.want {
				t.Errorf("IpV6(%q) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestNewProxySocket(t *testing.T) {
	s := NewProxySocket()
	if s == nil {
		t.Fatal("NewProxySocket() returned nil")
	}
}
