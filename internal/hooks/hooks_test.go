package hooks

import (
	"math/rand"
	"strings"
	"testing"
)

// plant writes the concrete bytes of sig at off; wildcard positions get fill.
func plant(buf []byte, off int, s signature, fill byte) {
	for i, w := range s.wild {
		if w {
			buf[off+i] = fill
		} else {
			buf[off+i] = s.pat[i]
		}
	}
}

func noise(n int) []byte {
	b := make([]byte, n)
	rand.New(rand.NewSource(1)).Read(b)
	return b
}

func TestResolveFindsAllThreeWhateverTheDisplacements(t *testing.T) {
	const textVA = 0x140001000
	for _, fill := range []byte{0x00, 0xAB, 0xFF} {
		buf := noise(1 << 20)
		plant(buf, 0x1000, parseSig("r", sigSSLRead), fill)
		plant(buf, 0x1640, parseSig("w", sigSSLWrite), fill)
		plant(buf, 0x2000, parseSig("k", sigKeylog), fill)

		got, err := resolve(buf, textVA)
		if err != nil {
			t.Fatalf("fill %#x: %v", fill, err)
		}
		want := Addrs{SSLRead: textVA + 0x1000, SSLWrite: textVA + 0x1640, Keylog: textVA + 0x2000}
		if got != want {
			t.Fatalf("fill %#x: got %+v, want %+v", fill, got, want)
		}
	}
}

func TestResolveRefusesWhenAFunctionIsMissing(t *testing.T) {
	buf := noise(1 << 20)
	plant(buf, 0x1000, parseSig("r", sigSSLRead), 0)
	plant(buf, 0x1640, parseSig("w", sigSSLWrite), 0)
	// keylog never planted: the game build changed that function.
	_, err := resolve(buf, 0x140001000)
	if err == nil || !strings.Contains(err.Error(), "nss_keylog_int: not found") {
		t.Fatalf("want a 'not found' error for nss_keylog_int, got %v", err)
	}
}

func TestResolveRefusesWhenAmbiguous(t *testing.T) {
	buf := noise(1 << 20)
	plant(buf, 0x1000, parseSig("r", sigSSLRead), 0)
	plant(buf, 0x5000, parseSig("r", sigSSLRead), 0)
	plant(buf, 0x1640, parseSig("w", sigSSLWrite), 0)
	plant(buf, 0x2000, parseSig("k", sigKeylog), 0)
	_, err := resolve(buf, 0x140001000)
	if err == nil || !strings.Contains(err.Error(), "SSL_read: 2 candidates") {
		t.Fatalf("want an 'ambiguous' error for SSL_read, got %v", err)
	}
}

// SSL_read and SSL_write share a 22-byte prefix. A signature must never match
// the other one's code, or we would hook read as write.
func TestReadAndWriteSignaturesAreDistinct(t *testing.T) {
	buf := noise(1 << 20)
	plant(buf, 0x1000, parseSig("w", sigSSLWrite), 0)
	if hits := parseSig("r", sigSSLRead).find(buf); len(hits) != 0 {
		t.Fatalf("SSL_read signature matched SSL_write code at %v", hits)
	}
}

func TestSignaturesParse(t *testing.T) {
	for name, s := range map[string]string{"read": sigSSLRead, "write": sigSSLWrite, "keylog": sigKeylog} {
		p := parseSig(name, s)
		if len(p.pat) < 64 {
			t.Errorf("%s: signature is only %d bytes", name, len(p.pat))
		}
		if _, anchor := p.anchor(); len(anchor) < 5 {
			t.Errorf("%s: anchor of %d bytes is too short to search quickly", name, len(anchor))
		}
	}
}
