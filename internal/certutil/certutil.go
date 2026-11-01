package certutil

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// EnsureTLSConfig loads or generates a TLS certificate and private key, returning
// a configured *tls.Config and the Root CA PEM bytes for client trust installation.
func EnsureTLSConfig(certDir, customCertFile, customKeyFile string) (*tls.Config, []byte, error) {
	if customCertFile != "" && customKeyFile != "" {
		tlsCert, err := tls.LoadX509KeyPair(customCertFile, customKeyFile)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load custom TLS keypair: %w", err)
		}
		var caPEM []byte
		if caData, err := os.ReadFile(customCertFile); err == nil {
			caPEM = caData
		}
		tlsConfig := &tls.Config{
			Certificates: []tls.Certificate{tlsCert},
			MinVersion:   tls.VersionTLS12,
		}
		return tlsConfig, caPEM, nil
	}

	if err := os.MkdirAll(certDir, 0700); err != nil {
		return nil, nil, fmt.Errorf("failed to create cert directory %s: %w", certDir, err)
	}

	caCertPath := filepath.Join(certDir, "ca.crt")
	caKeyPath := filepath.Join(certDir, "ca.key")
	serverCertPath := filepath.Join(certDir, "server.crt")
	serverKeyPath := filepath.Join(certDir, "server.key")

	caCert, caKey, caPEM, err := loadOrCreateCA(caCertPath, caKeyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load or create CA: %w", err)
	}

	serverCertPEM, serverKeyPEM, err := loadOrCreateServerCert(serverCertPath, serverKeyPath, caCert, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load or create server cert: %w", err)
	}

	tlsCert, err := tls.X509KeyPair(serverCertPEM, serverKeyPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create tls certificate: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		MinVersion:   tls.VersionTLS12,
	}

	return tlsConfig, caPEM, nil
}

func loadOrCreateCA(certPath, keyPath string) (*x509.Certificate, *rsa.PrivateKey, []byte, error) {
	certData, err1 := os.ReadFile(certPath)
	keyData, err2 := os.ReadFile(keyPath)

	if err1 == nil && err2 == nil {
		block, _ := pem.Decode(certData)
		if block != nil {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err == nil && time.Now().Before(cert.NotAfter.Add(-24*time.Hour)) {
				keyBlock, _ := pem.Decode(keyData)
				if keyBlock != nil {
					key, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
					if err == nil {
						return cert, key, certData, nil
					}
					// Attempt PKCS8
					if pkcs8Key, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes); err == nil {
						if rsaKey, ok := pkcs8Key.(*rsa.PrivateKey); ok {
							return cert, rsaKey, certData, nil
						}
					}
				}
			}
		}
	}

	// Generate new CA
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to generate CA private key: %w", err)
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "nas"
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"NaN NAS"},
			CommonName:   fmt.Sprintf("NaN NAS Root CA (%s)", hostname),
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour), // 10 years
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	caBytes, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create CA certificate: %w", err)
	}

	caCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caBytes})
	caKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(caKey)})

	if err := os.WriteFile(certPath, caCertPEM, 0644); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to save CA certificate: %w", err)
	}
	if err := os.WriteFile(keyPath, caKeyPEM, 0600); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to save CA key: %w", err)
	}

	parsedCA, err := x509.ParseCertificate(caBytes)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to parse generated CA certificate: %w", err)
	}

	return parsedCA, caKey, caCertPEM, nil
}

func loadOrCreateServerCert(certPath, keyPath string, caCert *x509.Certificate, caKey *rsa.PrivateKey) ([]byte, []byte, error) {
	currentIPs := GetLocalIPs()
	currentDNS := GetDNSNames()

	certData, err1 := os.ReadFile(certPath)
	keyData, err2 := os.ReadFile(keyPath)

	if err1 == nil && err2 == nil {
		block, _ := pem.Decode(certData)
		if block != nil {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err == nil && time.Now().Before(cert.NotAfter.Add(-24*time.Hour)) {
				// Verify if existing cert covers current IPs
				if containsAllIPs(cert.IPAddresses, currentIPs) {
					return certData, keyData, nil
				}
			}
		}
	}

	// Generate new server key and certificate signed by CA
	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate server key: %w", err)
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "nas"
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	serverTemplate := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"NaN NAS"},
			CommonName:   hostname,
		},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(2 * 365 * 24 * time.Hour), // 2 years
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  currentIPs,
		DNSNames:     currentDNS,
	}

	serverBytes, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create server certificate: %w", err)
	}

	serverCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverBytes})
	serverKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(serverKey)})

	if err := os.WriteFile(certPath, serverCertPEM, 0644); err != nil {
		return nil, nil, fmt.Errorf("failed to save server certificate: %w", err)
	}
	if err := os.WriteFile(keyPath, serverKeyPEM, 0600); err != nil {
		return nil, nil, fmt.Errorf("failed to save server key: %w", err)
	}

	return serverCertPEM, serverKeyPEM, nil
}

// GetLocalIPs enumerates loopback and all active host unicast IPv4 and IPv6 addresses.
func GetLocalIPs() []net.IP {
	var ips []net.IP
	ips = append(ips, net.ParseIP("127.0.0.1"), net.ParseIP("::1"))

	seen := make(map[string]bool)
	seen["127.0.0.1"] = true
	seen["::1"] = true

	ifaces, err := net.Interfaces()
	if err != nil {
		return ips
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil && !seen[ip.String()] {
				seen[ip.String()] = true
				ips = append(ips, ip)
			}
		}
	}
	return ips
}

// GetDNSNames returns standard hostnames and local domains.
func GetDNSNames() []string {
	names := []string{"localhost"}
	hostname, err := os.Hostname()
	if err == nil && hostname != "" {
		names = append(names, hostname)
		if !strings.Contains(hostname, ".") {
			names = append(names, hostname+".local")
		}
	}
	return names
}

func containsAllIPs(existing []net.IP, target []net.IP) bool {
	existingMap := make(map[string]bool)
	for _, ip := range existing {
		existingMap[ip.String()] = true
	}
	for _, ip := range target {
		if !existingMap[ip.String()] {
			return false
		}
	}
	return true
}
