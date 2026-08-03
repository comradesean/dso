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
	"math/big"
	"os"
	"path/filepath"
)

const (
	privatePEMType = "RSA PRIVATE KEY"
	publicPEMType  = "RSA PUBLIC KEY"
	defaultBits    = 2048
)

// Generate creates a new RSA key pair with the given modulus size (public
// exponent 65537, matching the reference PC server).
func Generate(bits int) (*rsa.PrivateKey, error) {
	if bits == 0 {
		bits = defaultBits
	}
	return rsa.GenerateKey(rand.Reader, bits)
}

// GenerateWithExponent creates an RSA key pair with a specific public exponent.
// The standard library only supports e=65537; e=3 is needed to match the
// Dark Souls 2 PS3 client, whose embedded keys use exponent 3 (so our
// replacement public key is byte-length-identical for an in-place patch).
func GenerateWithExponent(bits, e int) (*rsa.PrivateKey, error) {
	if bits == 0 {
		bits = defaultBits
	}
	if e == 0 || e == 65537 {
		return Generate(bits)
	}
	return generateSmallExponent(bits, e)
}

func generateSmallExponent(bits, e int) (*rsa.PrivateKey, error) {
	eBig := big.NewInt(int64(e))
	one := big.NewInt(1)
	for attempt := 0; attempt < 1000; attempt++ {
		p, err := rand.Prime(rand.Reader, bits/2)
		if err != nil {
			return nil, err
		}
		q, err := rand.Prime(rand.Reader, bits/2)
		if err != nil {
			return nil, err
		}
		if p.Cmp(q) == 0 {
			continue
		}
		p1 := new(big.Int).Sub(p, one)
		q1 := new(big.Int).Sub(q, one)
		// e must be coprime to (p-1) and (q-1).
		if new(big.Int).GCD(nil, nil, eBig, p1).Cmp(one) != 0 {
			continue
		}
		if new(big.Int).GCD(nil, nil, eBig, q1).Cmp(one) != 0 {
			continue
		}
		n := new(big.Int).Mul(p, q)
		if n.BitLen() != bits {
			continue
		}
		// d = e^-1 mod lcm(p-1, q-1).
		g := new(big.Int).GCD(nil, nil, p1, q1)
		lcm := new(big.Int).Div(new(big.Int).Mul(p1, q1), g)
		d := new(big.Int).ModInverse(eBig, lcm)
		if d == nil {
			continue
		}
		key := &rsa.PrivateKey{
			PublicKey: rsa.PublicKey{N: n, E: e},
			D:         d,
			Primes:    []*big.Int{p, q},
		}
		key.Precompute()
		if err := key.Validate(); err != nil {
			continue
		}
		return key, nil
	}
	return nil, errors.New("keys: failed to generate key with requested exponent")
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
