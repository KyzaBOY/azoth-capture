package hooks

import (
	"os"
	"testing"
)

func TestPrologueFormatsLikeTheAgent(t *testing.T) {
	const textVA = 0x140001000
	text := make([]byte, 64)
	for i := range text {
		text[i] = byte(i)
	}
	text[20] = 0xAB

	got, err := prologue(text, textVA, textVA+16)
	if err != nil {
		t.Fatal(err)
	}
	// Upper-case pairs separated by single spaces: what nwHexdump produces.
	want := "10 11 12 13 AB 15 16 17 18 19 1A 1B 1C 1D 1E 1F"
	if got != want {
		t.Errorf("prologue = %q, want %q", got, want)
	}
}

func TestPrologueRejectsAddressesOutsideText(t *testing.T) {
	const textVA = 0x140001000
	text := make([]byte, 64)
	for _, va := range []uint64{textVA - 1, textVA + 64 - PrologueLen + 1, 0} {
		if _, err := prologue(text, textVA, va); err == nil {
			t.Errorf("address %#X: expected an error", va)
		}
	}
	if _, err := prologue(text, textVA, textVA+64-PrologueLen); err != nil {
		t.Errorf("last valid address rejected: %v", err)
	}
}

// Runs against a real game install when AZOTH_NW_EXE points at NewWorld.exe
// (the exe is Amazon's and is not in this repo, so CI skips this).
func TestRealGamePrologues(t *testing.T) {
	exe := os.Getenv("AZOTH_NW_EXE")
	if exe == "" {
		t.Skip("set AZOTH_NW_EXE to the path of NewWorld.exe to run this")
	}
	b, err := Identify(exe)
	if err != nil {
		t.Fatal(err)
	}
	a, err := Find(exe)
	if err != nil {
		t.Fatal(err)
	}
	p, err := ReadPrologues(exe, a)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("SSL_read  %s\nSSL_write %s\nkeylog    %s", p.SSLRead, p.SSLWrite, p.Keylog)

	// Build 1.460.6033: these are also the bytes the attach helper found in the
	// running game's MEMORY during live captures ("hook sites match the game in
	// memory"), so the file bytes we compare against are the bytes the running
	// game has.
	if b.SHA256 == "30fe6461f929ba44178376a72f0765dc5da28ccba20e5006a60a229babd74033" {
		want := Prologues{
			SSLRead:  "B8 38 00 00 00 E8 C6 50 1B 00 48 2B E0 45 85 C0",
			SSLWrite: "B8 38 00 00 00 E8 86 4A 1B 00 48 2B E0 45 85 C0",
			Keylog:   "40 57 41 55 41 56 41 57 B8 48 00 00 00 E8 FE 44",
		}
		if p != want {
			t.Fatalf("build 1.460.6033 prologues:\n got  %+v\n want %+v", p, want)
		}
	}
}
