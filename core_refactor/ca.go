package core_refactor

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"sync"
	"time"
)

// CA 封装根证书与动态签名能力，替代原全局 init() 加载模式。
// 调用方可以显式指定证书路径，失败时返回 error 而不是 panic。
type CA struct {
	cert  *x509.Certificate
	key   crypto.PrivateKey
	cache map[string]tls.Certificate
	mu    sync.RWMutex
}

// LoadCA 从 PEM 文件加载根证书与私钥。
func LoadCA(certPath, keyPath string) (*CA, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("read CA cert %q: %w", certPath, err)
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read CA key %q: %w", keyPath, err)
	}

	cert, key, err := parseCA(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}

	return &CA{
		cert:  cert,
		key:   key,
		cache: make(map[string]tls.Certificate),
	}, nil
}

func parseCA(certPEM, keyPEM []byte) (*x509.Certificate, crypto.PrivateKey, error) {
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		return nil, nil, fmt.Errorf("parse CA cert PEM failed")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("parse CA certificate: %w", err)
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, nil, fmt.Errorf("parse CA key PEM failed")
	}

	var key crypto.PrivateKey
	switch keyBlock.Type {
	case "PRIVATE KEY":
		key, err = x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	case "EC PRIVATE KEY":
		key, err = x509.ParseECPrivateKey(keyBlock.Bytes)
	case "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	default:
		return nil, nil, fmt.Errorf("unsupported private key type: %s", keyBlock.Type)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("parse CA private key: %w", err)
	}
	return cert, key, nil
}

// SignHost 为给定主机列表签发 TLS 证书；相同首主机名会命中缓存。
func (ca *CA) SignHost(hosts []string) (tls.Certificate, error) {
	if len(hosts) == 0 {
		return tls.Certificate{}, fmt.Errorf("no hosts provided")
	}
	cacheKey := hosts[0]

	ca.mu.RLock()
	if c, ok := ca.cache[cacheKey]; ok {
		ca.mu.RUnlock()
		return c, nil
	}
	ca.mu.RUnlock()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("generate private key: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName:   hosts[0],
			Organization: []string{"mitm-proxy"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &priv.PublicKey, ca.key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("create certificate: %w", err)
	}

	tlsCert := tls.Certificate{
		Certificate: [][]byte{certBytes},
		PrivateKey:  priv,
	}

	ca.mu.Lock()
	ca.cache[cacheKey] = tlsCert
	ca.mu.Unlock()
	return tlsCert, nil
}
