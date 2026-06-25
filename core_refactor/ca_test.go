package core_refactor

import (
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
)

func writeTestCAFiles(t *testing.T) (certPath, keyPath string) {
	t.Helper()
	cert, key, err := generateRootCA()
	if err != nil {
		t.Fatalf("generate root ca: %v", err)
	}

	dir := t.TempDir()
	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")

	if err := os.WriteFile(certPath, pemEncodeCert(cert.Raw), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pemEncodeECKey(key), 0600); err != nil {
		t.Fatal(err)
	}
	return certPath, keyPath
}

func TestLoadCA(t *testing.T) {
	certPath, keyPath := writeTestCAFiles(t)
	ca, err := LoadCA(certPath, keyPath)
	if err != nil {
		t.Fatalf("LoadCA failed: %v", err)
	}
	if ca.cert == nil || ca.key == nil {
		t.Fatal("CA cert or key is nil")
	}
}

func TestLoadCAMissing(t *testing.T) {
	_, err := LoadCA("/nonexistent/cert.pem", "/nonexistent/key.pem")
	if err == nil {
		t.Fatal("expected error for missing CA files")
	}
}

func TestSignHostCache(t *testing.T) {
	certPath, keyPath := writeTestCAFiles(t)
	ca, err := LoadCA(certPath, keyPath)
	if err != nil {
		t.Fatalf("LoadCA failed: %v", err)
	}

	cert1, err := ca.SignHost([]string{"example.com"})
	if err != nil {
		t.Fatalf("SignHost failed: %v", err)
	}
	cert2, err := ca.SignHost([]string{"example.com"})
	if err != nil {
		t.Fatalf("SignHost failed: %v", err)
	}
	if &cert1 == &cert2 {
		t.Fatal("expected different certificate objects")
	}
	if string(cert1.Certificate[0]) != string(cert2.Certificate[0]) {
		t.Fatal("expected cached certificate bytes to be equal")
	}
}

func TestSignHostIPAndDNS(t *testing.T) {
	certPath, keyPath := writeTestCAFiles(t)
	ca, err := LoadCA(certPath, keyPath)
	if err != nil {
		t.Fatalf("LoadCA failed: %v", err)
	}

	cert, err := ca.SignHost([]string{"example.com", "192.168.1.1"})
	if err != nil {
		t.Fatalf("SignHost failed: %v", err)
	}

	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	if len(x509Cert.DNSNames) != 1 || x509Cert.DNSNames[0] != "example.com" {
		t.Fatalf("unexpected DNS names: %v", x509Cert.DNSNames)
	}
	if len(x509Cert.IPAddresses) != 1 {
		t.Fatalf("unexpected IP addresses: %v", x509Cert.IPAddresses)
	}
}
