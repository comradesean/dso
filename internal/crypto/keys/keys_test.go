package keys

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestPEMRoundTrip(t *testing.T) {
	priv, err := Generate(2048)
	if err != nil {
		t.Fatal(err)
	}
	privPEM := PrivatePEM(priv)
	pubPEM := PublicPEM(&priv.PublicKey)

	if !strings.Contains(string(pubPEM), "BEGIN RSA PUBLIC KEY") {
		t.Errorf("public PEM is not PKCS#1 form:\n%s", pubPEM)
	}

	gotPriv, err := ParsePrivatePEM(privPEM)
	if err != nil {
		t.Fatal(err)
	}
	if gotPriv.D.Cmp(priv.D) != 0 {
		t.Error("private key did not round-trip")
	}
	gotPub, err := ParsePublicPEM(pubPEM)
	if err != nil {
		t.Fatal(err)
	}
	if gotPub.N.Cmp(priv.N) != 0 || gotPub.E != priv.E {
		t.Error("public key did not round-trip")
	}
}

func TestLoadOrGenerate(t *testing.T) {
	dir := t.TempDir()
	privPath := filepath.Join(dir, "server.private.pem")
	pubPath := filepath.Join(dir, "server.public.pem")

	first, err := LoadOrGenerate(privPath, pubPath, 2048)
	if err != nil {
		t.Fatal(err)
	}
	// Second call must load the same key, not generate a new one.
	second, err := LoadOrGenerate(privPath, pubPath, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if first.N.Cmp(second.N) != 0 {
		t.Fatal("LoadOrGenerate regenerated the key on the second call")
	}
	if !bytes.Equal(PublicPEM(&first.PublicKey), PublicPEM(&second.PublicKey)) {
		t.Fatal("public key changed between loads")
	}
}
