// Package keys handles the server's RSA key pair: generation on first run,
// loading, and PKCS#1 PEM (de)serialization. The Frpg2 client expects the
// server's public key in PKCS#1 form ("-----BEGIN RSA PUBLIC KEY-----"), which
// is the exact text patched into the client to point it at this server.
package keys

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	privatePEMType = "RSA PRIVATE KEY"
	publicPEMType  = "RSA PUBLIC KEY"
	defaultBits    = 2048
)

// Generate creates a new RSA key pair with the given modulus size (public
// exponent 65537, matching the reference server).
func Generate(bits int) (*rsa.PrivateKey, error) {
	if bits == 0 {
		bits = defaultBits
	}
	return rsa.GenerateKey(rand.Reader, bits)
}

// PrivatePEM encodes a private key as PKCS#1 PEM.
func PrivatePEM(priv *rsa.PrivateKey) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  privatePEMType,
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})
}

// PublicPEM encodes a public key as PKCS#1 PEM.
func PublicPEM(pub *rsa.PublicKey) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  publicPEMType,
		Bytes: x509.MarshalPKCS1PublicKey(pub),
	})
}

// ParsePrivatePEM decodes a PKCS#1 private key PEM.
func ParsePrivatePEM(data []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil || block.Type != privatePEMType {
		return nil, errors.New("keys: not a PKCS#1 RSA private key PEM")
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

// ParsePublicPEM decodes a PKCS#1 public key PEM.
func ParsePublicPEM(data []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(data)
	if block == nil || block.Type != publicPEMType {
		return nil, errors.New("keys: not a PKCS#1 RSA public key PEM")
	}
	return x509.ParsePKCS1PublicKey(block.Bytes)
}

// LoadOrGenerate loads the key pair from privatePath, generating and persisting
// a new one (both halves) if the private key file does not yet exist. The
// private file is written with 0600 permissions.
func LoadOrGenerate(privatePath, publicPath string, bits int) (*rsa.PrivateKey, error) {
	if data, err := os.ReadFile(privatePath); err == nil {
		return ParsePrivatePEM(data)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("keys: reading %s: %w", privatePath, err)
	}

	priv, err := Generate(bits)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(privatePath), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(privatePath, PrivatePEM(priv), 0o600); err != nil {
		return nil, fmt.Errorf("keys: writing private key: %w", err)
	}
	if publicPath != "" {
		if err := os.WriteFile(publicPath, PublicPEM(&priv.PublicKey), 0o644); err != nil {
			return nil, fmt.Errorf("keys: writing public key: %w", err)
		}
	}
	return priv, nil
}
