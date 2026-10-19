// Package hooks finds, inside NewWorld.exe, the three functions the capture
// agent hooks: the DTLS read/write plaintext (SSL_read, SSL_write) and the TLS
// key-log callback (nss_keylog_int).
//
// Why this exists: the upstream capture scripts (Aeternum-World) hardcode the
// addresses of those functions for ONE game build. Every game patch moves them
// (build 1.460 shifted all three by +0x1640) and a hook placed in the middle of
// an instruction crashes the game. Instead of trusting fixed addresses we look
// the functions up by their machine code, and refuse to start if we cannot
// find each of them exactly once.
//
// The signatures were cut from the build the old addresses were valid for, with
// every rip-relative displacement and relative call/branch target replaced by a
// wildcard, so they keep matching when a patch only shifts code around. If a
// patch actually changes these functions the lookup fails closed (see Find).
package hooks

import (
	"bytes"
	"debug/pe"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// imageBase is the preferred base the capture agent rebases from
// (BASE_IDA in _common.js). Addresses returned by Find are relative to it.
const imageBase = 0x140000000

// Build identifies one game executable, for the activity log and bug reports.
type Build struct {
	Version string // e.g. 1.460.6033.6024067 (file version resource; "unknown" on Linux)
	SHA256  string
}

// Addrs are the virtual addresses (at imageBase) of the hooked functions.
type Addrs struct {
	SSLRead  uint64
	SSLWrite uint64
	Keylog   uint64
}

// Signature text: hex bytes, "??" = wildcard. The first 64+ bytes of each
// function, cut at an instruction boundary. Do not edit by hand: regenerate
// from a known-good build (see docs/HOOKS.md).
const (
	sigSSLRead = "B8 38 00 00 00 E8 ?? ?? ?? ?? 48 2B E0 45 85 C0 79 ?? BA DF 00 00 00 C7 44 24 20 F3 06 00 00 " +
		"4C 8D 0D ?? ?? ?? ?? 8D 48 DC 44 8D 42 30 E8 ?? ?? ?? ?? B8 FF FF FF FF 48 83 C4 38 C3 4D 63 C0 4C 8D 4C 24 58"
	sigSSLWrite = "B8 38 00 00 00 E8 ?? ?? ?? ?? 48 2B E0 45 85 C0 79 ?? BA D0 00 00 00 C7 44 24 20 B2 07 00 00 " +
		"4C 8D 0D ?? ?? ?? ?? 8D 48 DC 44 8D 42 3F E8 ?? ?? ?? ?? B8 FF FF FF FF 48 83 C4 38 C3 4D 63 C0 4C 8D 4C 24 58"
	sigKeylog = "40 57 41 55 41 56 41 57 B8 48 00 00 00 E8 ?? ?? ?? ?? 48 2B E0 48 8B 82 98 05 00 00 4D 8B F1 4D 8B F8 " +
		"4C 8B EA 48 8B F9 48 83 B8 88 03 00 00 00 75 ?? B8 01 00 00 00 48 83 C4 48 41 5F 41 5E 41 5D"
)

type signature struct {
	name string
	pat  []byte
	wild []bool // wild[i] = byte i is a wildcard
}

func parseSig(name, text string) signature {
	f := strings.Fields(text)
	s := signature{name: name, pat: make([]byte, len(f)), wild: make([]bool, len(f))}
	for i, tok := range f {
		if tok == "??" {
			s.wild[i] = true
			continue
		}
		v, err := strconv.ParseUint(tok, 16, 8)
		if err != nil {
			panic(fmt.Sprintf("hooks: bad signature %s token %q", name, tok))
		}
		s.pat[i] = byte(v)
	}
	return s
}

// anchor is the first run of concrete bytes: the part we search for quickly
// before checking the rest of the pattern.
func (s signature) anchor() (off int, run []byte) {
	for off < len(s.wild) && s.wild[off] {
		off++
	}
	end := off
	for end < len(s.wild) && !s.wild[end] {
		end++
	}
	return off, s.pat[off:end]
}

// find returns every offset in text where the signature matches.
func (s signature) find(text []byte) []int {
	aoff, anchor := s.anchor()
	var hits []int
	for from := 0; ; {
		p := bytes.Index(text[from:], anchor)
		if p < 0 {
			break
		}
		p += from
		from = p + 1
		start := p - aoff
		if start < 0 || start+len(s.pat) > len(text) {
			continue
		}
		ok := true
		for i, w := range s.wild {
			if !w && text[start+i] != s.pat[i] {
				ok = false
				break
			}
		}
		if ok {
			hits = append(hits, start)
		}
	}
	return hits
}

// resolve looks up the three functions in a code section that starts at
// virtual address textVA. Each one must be found exactly once.
func resolve(text []byte, textVA uint64) (Addrs, error) {
	var out Addrs
	targets := []struct {
		sig signature
		dst *uint64
	}{
		{parseSig("SSL_read", sigSSLRead), &out.SSLRead},
		{parseSig("SSL_write", sigSSLWrite), &out.SSLWrite},
		{parseSig("nss_keylog_int", sigKeylog), &out.Keylog},
	}
	var problems []string
	for _, t := range targets {
		hits := t.sig.find(text)
		switch len(hits) {
		case 1:
			*t.dst = textVA + uint64(hits[0])
		case 0:
			problems = append(problems, t.sig.name+": not found")
		default:
			problems = append(problems, fmt.Sprintf("%s: %d candidates (ambiguous)", t.sig.name, len(hits)))
		}
	}
	if len(problems) > 0 {
		return Addrs{}, errors.New(strings.Join(problems, "; "))
	}
	return out, nil
}

// Find opens the game executable and resolves the hook addresses. It returns
// an error, and the caller must NOT launch the game with hooks, if the build is
// not one we can place the hooks in with certainty.
func Find(exePath string) (Addrs, error) {
	text, textVA, err := readText(exePath)
	if err != nil {
		return Addrs{}, err
	}
	return resolve(text, textVA)
}

// PrologueLen is how many bytes at the start of each hooked function are
// compared with the running game's memory before any hook is placed. It equals
// what the agent's nwHexdump reads, so the two sides compare like with like.
const PrologueLen = 16

// Prologues are the first PrologueLen bytes of each hooked function as stored
// in the executable file, written the way the agent's nwHexdump writes them
// ("B8 38 00 ..."): upper-case hex pairs separated by spaces.
type Prologues struct {
	SSLRead  string
	SSLWrite string
	Keylog   string
}

// ReadPrologues reads the file bytes at the resolved hook addresses. When the
// capture attaches to a game that is already running, these let the agent
// check that the code in memory is the code we scanned, and refuse to hook if
// it is not, instead of crashing the game.
func ReadPrologues(exePath string, a Addrs) (Prologues, error) {
	text, textVA, err := readText(exePath)
	if err != nil {
		return Prologues{}, err
	}
	var p Prologues
	for _, site := range []struct {
		name string
		va   uint64
		dst  *string
	}{
		{"SSL_read", a.SSLRead, &p.SSLRead},
		{"SSL_write", a.SSLWrite, &p.SSLWrite},
		{"nss_keylog_int", a.Keylog, &p.Keylog},
	} {
		if *site.dst, err = prologue(text, textVA, site.va); err != nil {
			return Prologues{}, fmt.Errorf("%s: %w", site.name, err)
		}
	}
	return p, nil
}

// prologue formats the PrologueLen bytes at virtual address va of a code
// section that starts at textVA.
func prologue(text []byte, textVA, va uint64) (string, error) {
	if va < textVA || va-textVA+PrologueLen > uint64(len(text)) {
		return "", fmt.Errorf("address 0x%X is outside the code section", va)
	}
	off := va - textVA
	hexBytes := make([]string, PrologueLen)
	for i, b := range text[off : off+PrologueLen] {
		hexBytes[i] = fmt.Sprintf("%02X", b)
	}
	return strings.Join(hexBytes, " "), nil
}

// readText returns the .text section of a 64-bit game executable and the
// virtual address it starts at, after checking the image base the capture
// agent assumes.
func readText(exePath string) (text []byte, textVA uint64, err error) {
	f, err := pe.Open(exePath)
	if err != nil {
		return nil, 0, fmt.Errorf("not a readable Windows executable: %w", err)
	}
	defer f.Close()

	oh, ok := f.OptionalHeader.(*pe.OptionalHeader64)
	if !ok {
		return nil, 0, errors.New("not a 64-bit executable")
	}
	if oh.ImageBase != imageBase {
		return nil, 0, fmt.Errorf("unexpected image base 0x%X (the capture agent assumes 0x%X)", oh.ImageBase, uint64(imageBase))
	}
	sec := f.Section(".text")
	if sec == nil {
		return nil, 0, errors.New("no .text section")
	}
	text, err = sec.Data()
	if err != nil {
		return nil, 0, fmt.Errorf("reading .text: %w", err)
	}
	return text, uint64(oh.ImageBase) + uint64(sec.VirtualAddress), nil
}
