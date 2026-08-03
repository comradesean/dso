// Package frpgcipher provides the concrete ciphers used by the Frpg2 message
// streams: the RSA pairing on the login/auth handshake and (later) the AES-CWC
// framings for the authenticated TCP and UDP streams. Each cipher satisfies the
// message.Cipher interface (Encrypt/Decrypt of a message body).
package frpgcipher

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"

	"github.com/sstreight/dso/internal/crypto/x931"
)

// rsaServerEncrypt encrypts server->client bodies with a raw private-key op over
// X9.31 padding.
type rsaServerEncrypt struct{ priv *rsa.PrivateKey }

func (c rsaServerEncrypt) Encrypt(pt []byte) ([]byte, error) { return x931.PrivateEncrypt(c.priv, pt) }
func (c rsaServerEncrypt) Decrypt(ct []byte) ([]byte, error) {
	// Server decrypts inbound with OAEP (private key).
	return rsa.DecryptOAEP(sha1.New(), rand.Reader, c.priv, ct, nil)
}

// NewRSAServer returns the encrypt and decrypt ciphers for the server side of a
// login/auth stream: outbound X9.31 (private-encrypt), inbound OAEP
// (private-decrypt).
func NewRSAServer(priv *rsa.PrivateKey) (enc, dec cipher) {
	c := rsaServerEncrypt{priv: priv}
	return c, c
}

// rsaClient is the mirror side used by the emulator and tests: outbound OAEP
// (public-encrypt), inbound X9.31 (public-decrypt).
type rsaClient struct{ pub *rsa.PublicKey }

func (c rsaClient) Encrypt(pt []byte) ([]byte, error) {
	return rsa.EncryptOAEP(sha1.New(), rand.Reader, c.pub, pt, nil)
}
func (c rsaClient) Decrypt(ct []byte) ([]byte, error) { return x931.PublicDecrypt(c.pub, ct) }

// NewRSAClient returns the encrypt and decrypt ciphers for the client side.
func NewRSAClient(pub *rsa.PublicKey) (enc, dec cipher) {
	c := rsaClient{pub: pub}
	return c, c
}

// cipher mirrors message.Cipher without importing that package (avoiding a
// dependency cycle); the interface is satisfied structurally.
type cipher interface {
	Encrypt(plaintext []byte) ([]byte, error)
	Decrypt(ciphertext []byte) ([]byte, error)
}
