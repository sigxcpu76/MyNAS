package certutil

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"testing"
)

func TestEnsureTLSConfig_GenerateAndLoad(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "nas-cert-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tlsCfg, caPEM, err := EnsureTLSConfig(tempDir, "", "")
	if err != nil {
		t.Fatalf("EnsureTLSConfig failed: %v", err)
	}

	if tlsCfg == nil || len(tlsCfg.Certificates) == 0 {
		t.Fatalf("expected certificates in tls.Config, got empty")
	}

	if len(caPEM) == 0 {
		t.Fatalf("expected non-empty caPEM")
	}

	// Verify CA cert structure
	caBlock, _ := pem.Decode(caPEM)
	if caBlock == nil {
		t.Fatalf("failed to decode CA PEM")
	}
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		t.Fatalf("failed to parse CA certificate: %v", err)
	}
	if !caCert.IsCA {
		t.Errorf("expected CA certificate to have IsCA=true")
	}

	// Re-run EnsureTLSConfig to test reloading existing certificates
	tlsCfg2, caPEM2, err := EnsureTLSConfig(tempDir, "", "")
	if err != nil {
		t.Fatalf("reloading existing certificates failed: %v", err)
	}
	if tlsCfg2 == nil || len(tlsCfg2.Certificates) == 0 {
		t.Fatalf("expected certificates on reload")
	}
	if string(caPEM) != string(caPEM2) {
		t.Errorf("expected reloaded CA PEM to match original")
	}

	// Test custom cert file paths
	customCertPath := tempDir + "/custom.crt"
	customKeyPath := tempDir + "/custom.key"
	keyBytes, _ := os.ReadFile(tempDir + "/server.key")
	certBytes, _ := os.ReadFile(tempDir + "/server.crt")
	_ = os.WriteFile(customCertPath, certBytes, 0644)
	_ = os.WriteFile(customKeyPath, keyBytes, 0600)

	customCfg, _, err := EnsureTLSConfig(tempDir, customCertPath, customKeyPath)
	if err != nil {
		t.Fatalf("EnsureTLSConfig with custom files failed: %v", err)
	}
	if len(customCfg.Certificates) == 0 {
		t.Errorf("expected loaded custom certificates")
	}
}
