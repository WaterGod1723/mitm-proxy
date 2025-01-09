package core

import (
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

var certCache = make(map[string]tls.Certificate)
var certCacheMu sync.Mutex

// 根证书和私钥的全局变量
var caCert *x509.Certificate
var caKey interface{} // 修改为接口类型以支持多种私钥类型

// Init 函数在包导入时自动加载根证书和私钥
func init() {
	var err error
	caCert, caKey, err = loadCA("./cert/cert.pem", "./cert/key.pem")
	if err != nil {
		panic(fmt.Sprintf("Failed to load CA: %v", err))
	}
}

// loadCA 从文件加载根证书和私钥
func loadCA(caCertPath, caKeyPath string) (*x509.Certificate, interface{}, error) {
	caCertPEM, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read CA cert: %v", err)
	}

	caKeyPEM, err := os.ReadFile(caKeyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read CA key: %v", err)
	}

	// 解析 CA 证书
	caBlock, _ := pem.Decode(caCertPEM)
	if caBlock == nil {
		return nil, nil, fmt.Errorf("failed to parse CA cert PEM")
	}
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse CA certificate: %v", err)
	}

	// 解析 CA 私钥
	keyBlock, _ := pem.Decode(caKeyPEM)
	if keyBlock == nil {
		return nil, nil, fmt.Errorf("failed to parse CA key PEM")
	}

	var caKey interface{}
	if keyBlock.Type == "PRIVATE KEY" { // PKCS#8 格式
		caKey, err = x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	} else if keyBlock.Type == "EC PRIVATE KEY" { // EC 私钥
		caKey, err = x509.ParseECPrivateKey(keyBlock.Bytes)
	} else if keyBlock.Type == "RSA PRIVATE KEY" { // RSA 私钥
		caKey, err = x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	} else {
		return nil, nil, fmt.Errorf("unsupported private key type: %s", keyBlock.Type)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse CA private key: %v", err)
	}

	return caCert, caKey, nil
}

// GenerateTLSCertificate 生成新证书并签名，支持自动识别域名和IP地址
func SignHost(hosts []string) (tls.Certificate, error) {
	if hostCert, ok := certCache[hosts[0]]; ok {
		return hostCert, nil
	}
	// 生成新的私钥
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to generate private key: %v", err)
	}

	// 创建新证书模板
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName:   "PYJ",
			Organization: []string{"PYJ"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0), // 1 年有效期
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// 根据输入的主机名判断是IP还是域名
	for _, host := range hosts {
		if ip := net.ParseIP(host); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, host)
		}
	}

	// 使用 CA 证书和密钥签署新证书
	certBytes, err := x509.CreateCertificate(rand.Reader, template, caCert, &priv.PublicKey, caKey)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to create certificate: %v", err)
	}

	// 将新证书和私钥打包为 tls.Certificate
	tlsCert := tls.Certificate{
		Certificate: [][]byte{certBytes},
		PrivateKey:  priv,
	}

	certCacheMu.Lock()
	certCache[hosts[0]] = tlsCert
	certCacheMu.Unlock()

	return tlsCert, nil
}
