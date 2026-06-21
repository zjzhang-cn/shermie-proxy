package Core

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"os"
	"testing"
)

func setupTestCert(t *testing.T) {
	t.Helper()
	// 使用临时目录生成根证书，避免污染项目目录
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

func TestGenerateKeyPair(t *testing.T) {
	cert := NewCertificate()
	key, err := cert.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error: %v", err)
	}
	if key == nil {
		t.Fatal("GenerateKeyPair() returned nil key")
	}
	if key.N.BitLen() != 2048 {
		t.Errorf("key bit length = %d, want 2048", key.N.BitLen())
	}
}

func TestGenerateKeyPair_Uniqueness(t *testing.T) {
	cert := NewCertificate()
	key1, _ := cert.GenerateKeyPair()
	key2, _ := cert.GenerateKeyPair()
	if key1.N.Cmp(key2.N) == 0 {
		t.Error("two generated keys should have different moduli")
	}
}

func TestCertificate_Init(t *testing.T) {
	setupTestCert(t)
	if Cert == nil {
		t.Fatal("Cert global should be set after Init()")
	}
	if Cert.RootCa == nil {
		t.Error("RootCa should not be nil")
	}
	if Cert.RootKey == nil {
		t.Error("RootKey should not be nil")
	}
	if len(Cert.RootCaStr) == 0 {
		t.Error("RootCaStr should not be empty")
	}
	if len(Cert.RootKeyStr) == 0 {
		t.Error("RootKeyStr should not be empty")
	}
}

func TestGeneratePem_Domain(t *testing.T) {
	setupTestCert(t)

	certPEM, keyPEM, err := Cert.GeneratePem("example.com")
	if err != nil {
		t.Fatalf("GeneratePem() error: %v", err)
	}
	if len(certPEM) == 0 {
		t.Error("cert PEM should not be empty")
	}
	if len(keyPEM) == 0 {
		t.Error("key PEM should not be empty")
	}

	// 验证证书可以解析
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("failed to decode cert PEM")
	}
	parsedCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("failed to parse certificate: %v", err)
	}
	if parsedCert.Subject.CommonName != "example.com" {
		t.Errorf("CommonName = %q, want %q", parsedCert.Subject.CommonName, "example.com")
	}
	if len(parsedCert.DNSNames) == 0 || parsedCert.DNSNames[0] != "example.com" {
		t.Errorf("DNSNames = %v, want [example.com]", parsedCert.DNSNames)
	}

	// 验证私钥可以解析
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		t.Fatal("failed to decode key PEM")
	}
	_, err = x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		t.Fatalf("failed to parse private key: %v", err)
	}

	// 验证证书和私钥配对有效
	_, err = tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("X509KeyPair() error: %v", err)
	}
}

func TestGeneratePem_IPAddress(t *testing.T) {
	setupTestCert(t)

	certPEM, keyPEM, err := Cert.GeneratePem("192.168.1.1")
	if err != nil {
		t.Fatalf("GeneratePem() error: %v", err)
	}

	block, _ := pem.Decode(certPEM)
	parsedCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("failed to parse certificate: %v", err)
	}
	if len(parsedCert.IPAddresses) == 0 {
		t.Error("certificate should have IP addresses for IP host")
	}
	if len(parsedCert.DNSNames) != 0 {
		t.Errorf("DNSNames should be empty for IP host, got %v", parsedCert.DNSNames)
	}

	_, err = tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("X509KeyPair() error: %v", err)
	}
}

func TestGeneratePem_UniqueSerialNumbers(t *testing.T) {
	setupTestCert(t)

	certPEM1, _, _ := Cert.GeneratePem("a.com")
	certPEM2, _, _ := Cert.GeneratePem("b.com")

	block1, _ := pem.Decode(certPEM1)
	block2, _ := pem.Decode(certPEM2)
	c1, _ := x509.ParseCertificate(block1.Bytes)
	c2, _ := x509.ParseCertificate(block2.Bytes)

	if c1.SerialNumber.Cmp(c2.SerialNumber) == 0 {
		t.Error("two certificates should have different serial numbers")
	}
}

func TestNewCertificate(t *testing.T) {
	c := NewCertificate()
	if c == nil {
		t.Fatal("NewCertificate() returned nil")
	}
	if c.RootKey != nil {
		t.Error("RootKey should be nil before Init")
	}
	if c.RootCa != nil {
		t.Error("RootCa should be nil before Init")
	}
}
