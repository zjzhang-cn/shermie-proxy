package Core

import (
	"crypto/tls"
	"os"
	"sync"
	"testing"
)

func setupCacheTest(t *testing.T) {
	t.Helper()
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	os.Chdir(tmpDir)
	t.Cleanup(func() { os.Chdir(origDir) })

	cert := NewCertificate()
	err := cert.Init()
	if err != nil {
		t.Fatalf("Init() error: %v", err)
	}
}

func TestGetCertificate_Basic(t *testing.T) {
	setupCacheTest(t)
	storage := NewStorage()

	cert, err := storage.GetCertificate("example.com", "443")
	if err != nil {
		t.Fatalf("GetCertificate() error: %v", err)
	}
	if cert == nil {
		t.Fatal("certificate should not be nil")
	}
	if _, ok := cert.(tls.Certificate); !ok {
		t.Error("result should be tls.Certificate")
	}
}

func TestGetCertificate_SameHostReturnsCached(t *testing.T) {
	setupCacheTest(t)
	storage := NewStorage()

	cert1, err := storage.GetCertificate("example.com", "443")
	if err != nil {
		t.Fatal(err)
	}
	cert2, err := storage.GetCertificate("example.com", "443")
	if err != nil {
		t.Fatal(err)
	}

	// 两次获取应返回相同的证书（同一指针）
	c1 := cert1.(tls.Certificate)
	c2 := cert2.(tls.Certificate)
	if len(c1.Certificate) == 0 || len(c2.Certificate) == 0 {
		t.Error("certificates should not be empty")
	}
}

func TestGetCertificate_DifferentHosts(t *testing.T) {
	setupCacheTest(t)
	storage := NewStorage()

	cert1, err := storage.GetCertificate("a.com", "443")
	if err != nil {
		t.Fatal(err)
	}
	cert2, err := storage.GetCertificate("b.com", "443")
	if err != nil {
		t.Fatal(err)
	}

	c1 := cert1.(tls.Certificate)
	c2 := cert2.(tls.Certificate)
	if len(c1.Certificate) == 0 || len(c2.Certificate) == 0 {
		t.Error("certificates should not be empty")
	}
}

func TestGetCertificate_ConcurrentSameHost(t *testing.T) {
	setupCacheTest(t)
	storage := NewStorage()

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)
	results := make([]interface{}, goroutines)
	errors := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			cert, err := storage.GetCertificate("concurrent.com", "443")
			results[idx] = cert
			errors[idx] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errors {
		if err != nil {
			t.Errorf("goroutine %d: GetCertificate() error: %v", i, err)
		}
	}
	for i, cert := range results {
		if cert == nil {
			t.Errorf("goroutine %d: certificate is nil", i)
		}
		if _, ok := cert.(tls.Certificate); !ok {
			t.Errorf("goroutine %d: result is not tls.Certificate", i)
		}
	}
}

func TestGetCertificate_ConcurrentDifferentHosts(t *testing.T) {
	setupCacheTest(t)
	storage := NewStorage()

	hosts := []string{"a.com", "b.com", "c.com", "d.com", "e.com"}
	var wg sync.WaitGroup
	wg.Add(len(hosts))
	errors := make([]error, len(hosts))

	for i, host := range hosts {
		go func(idx int, h string) {
			defer wg.Done()
			cert, err := storage.GetCertificate(h, "443")
			if err != nil {
				errors[idx] = err
				return
			}
			if cert == nil {
				errors[idx] = err
			}
		}(i, host)
	}
	wg.Wait()

	for i, err := range errors {
		if err != nil {
			t.Errorf("host %s: error: %v", hosts[i], err)
		}
	}
}

func TestGetCertificate_HostWithPort(t *testing.T) {
	setupCacheTest(t)
	storage := NewStorage()

	// 传入带端口的 hostname
	cert, err := storage.GetCertificate("example.com:8443", "8443")
	if err != nil {
		t.Fatalf("GetCertificate() error: %v", err)
	}
	if cert == nil {
		t.Fatal("certificate should not be nil")
	}
}

func TestNewStorage(t *testing.T) {
	s := NewStorage()
	if s == nil {
		t.Fatal("NewStorage() returned nil")
	}
	if s.lock == nil {
		t.Error("lock should not be nil")
	}
	if s.mapping == nil {
		t.Error("mapping should not be nil")
	}
}
