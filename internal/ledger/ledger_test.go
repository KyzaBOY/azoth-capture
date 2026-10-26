package ledger

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// rec builds one ledger record.
func rec(ts int64, ssl uint64, payload string) []byte {
	b := make([]byte, HeaderSize, HeaderSize+len(payload))
	binary.LittleEndian.PutUint32(b, Magic)
	binary.LittleEndian.PutUint64(b[offTime:], uint64(ts))
	binary.LittleEndian.PutUint64(b[offSSL:], ssl)
	binary.LittleEndian.PutUint32(b[offLen:], uint32(len(payload)))
	return append(b, payload...)
}

func writeLedger(t *testing.T, recs ...[]byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ledger.bin")
	if err := os.WriteFile(p, bytes.Join(recs, nil), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestScanAndSplitByConnection(t *testing.T) {
	a1, a2, b1 := rec(100, 0xA, "hello"), rec(200, 0xA, "x"), rec(300, 0xB, "second login")
	cut := rec(400, 0xB, "lost")[:HeaderSize+2] // the game was killed mid-write
	path := writeLedger(t, a1, a2, b1, cut)

	conns, trailing, err := Scan(path)
	if err != nil {
		t.Fatal(err)
	}
	if trailing != int64(len(cut)) {
		t.Errorf("trailing = %d, want %d", trailing, len(cut))
	}
	want := []Connection{
		{SSL: 0xA, FirstMs: 100, LastMs: 200, Records: 2, Bytes: int64(len(a1) + len(a2))},
		{SSL: 0xB, FirstMs: 300, LastMs: 300, Records: 1, Bytes: int64(len(b1))},
	}
	if len(conns) != 2 || conns[0] != want[0] || conns[1] != want[1] {
		t.Fatalf("connections = %+v", conns)
	}
	if !Sequential(conns) {
		t.Error("two logins one after the other are sequential")
	}

	dir := t.TempDir()
	outs := []string{filepath.Join(dir, "1.bin"), filepath.Join(dir, "2.bin")}
	if err := Split(path, conns, outs); err != nil {
		t.Fatal(err)
	}
	for i, wantBytes := range [][]byte{append(append([]byte{}, a1...), a2...), b1} {
		got, err := os.ReadFile(outs[i])
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, wantBytes) {
			t.Errorf("part %d = %d bytes, want the connection's records unchanged", i+1, len(got))
		}
	}
}

func TestSequentialRejectsOverlap(t *testing.T) {
	if Sequential([]Connection{{FirstMs: 0, LastMs: 500}, {FirstMs: 300, LastMs: 900}}) {
		t.Error("overlapping connections were called sequential")
	}
}

func TestScanRejectsDamage(t *testing.T) {
	bad := rec(1, 1, "ok")
	bad[0] = 0
	if _, _, err := Scan(writeLedger(t, rec(1, 1, "ok"), bad)); err == nil {
		t.Error("a record without the magic number was accepted")
	}
}
