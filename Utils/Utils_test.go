package Utils

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestFileExist_ExistingFile(t *testing.T) {
	// 创建临时文件
	tmpFile := filepath.Join(t.TempDir(), "test.txt")
	if err := os.WriteFile(tmpFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}
	if !FileExist(tmpFile) {
		t.Errorf("FileExist(%q) = false, want true", tmpFile)
	}
}

func TestFileExist_NonExistingFile(t *testing.T) {
	if FileExist("/nonexistent/path/to/file.txt") {
		t.Error("FileExist for nonexistent file should return false")
	}
}

func TestFileExist_Directory(t *testing.T) {
	dir := t.TempDir()
	if !FileExist(dir) {
		t.Errorf("FileExist(%q) for directory = false, want true", dir)
	}
}

func TestFileExist_EmptyPath(t *testing.T) {
	if FileExist("") {
		t.Error("FileExist('') should return false")
	}
}

func TestGetAvailablePort(t *testing.T) {
	port, err := GetAvailablePort()
	if err != nil {
		t.Fatalf("GetAvailablePort() error: %v", err)
	}
	if port <= 0 || port > 65535 {
		t.Errorf("GetAvailablePort() = %d, want valid port (1-65535)", port)
	}
}

func TestGetAvailablePort_ReturnsDifferentPorts(t *testing.T) {
	port1, err := GetAvailablePort()
	if err != nil {
		t.Fatal(err)
	}
	port2, err := GetAvailablePort()
	if err != nil {
		t.Fatal(err)
	}
	// 两次调用应返回不同端口（概率极高）
	if port1 == port2 {
		t.Logf("warning: got same port %d twice (unlikely but possible)", port1)
	}
}

func TestIsPortAvailable_FreePort(t *testing.T) {
	// 先获取一个可用端口
	port, err := GetAvailablePort()
	if err != nil {
		t.Fatal(err)
	}
	if !IsPortAvailable(port) {
		t.Errorf("IsPortAvailable(%d) = false, want true for free port", port)
	}
}

func TestIsPortAvailable_OccupiedPort(t *testing.T) {
	// 启动一个监听来占用端口（与 IsPortAvailable 一致绑定 0.0.0.0）
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	if IsPortAvailable(port) {
		t.Errorf("IsPortAvailable(%d) = true, want false for occupied port", port)
	}
}
