// Package hotkey registers one global capture-mark key that works while the
// game has focus, and acts only on the single combination you chose. On
// Windows (hotkey_windows.go) the system delivers only that combination. On
// Linux (hotkey_linux.go) the kernel delivers every key event, and the code
// drops all but the modifiers and the chosen key; that file explains the
// permission it needs. This file parses and formats the key combination.
package hotkey

import (
	"fmt"
	"strings"
)

// Modifier bits. They are the Windows MOD_* values, and both platforms use
// them to describe a combination.
const (
	modAlt   = 0x0001
	modCtrl  = 0x0002
	modShift = 0x0004
	modWin   = 0x0008
)

// Spec is a key combination: modifiers plus a virtual key.
type Spec struct {
	Mods uint32
	VK   uint32
}

type keyDef struct {
	name string
	vk   uint32
	free bool // usable without a modifier (does not interfere with typing)
}

var keys = buildKeys()

func buildKeys() []keyDef {
	var k []keyDef
	for i := 1; i <= 24; i++ {
		k = append(k, keyDef{fmt.Sprintf("F%d", i), uint32(0x6F + i), true})
	}
	for c := 'A'; c <= 'Z'; c++ {
		k = append(k, keyDef{string(c), uint32(c), false})
	}
	for c := '0'; c <= '9'; c++ {
		k = append(k, keyDef{string(c), uint32(c), false})
	}
	for i := 0; i <= 9; i++ {
		k = append(k, keyDef{fmt.Sprintf("Num%d", i), uint32(0x60 + i), true})
	}
	return append(k,
		keyDef{"Space", 0x20, false},
		keyDef{"Insert", 0x2D, true}, keyDef{"Delete", 0x2E, true},
		keyDef{"Home", 0x24, true}, keyDef{"End", 0x23, true},
		keyDef{"PageUp", 0x21, true}, keyDef{"PageDown", 0x22, true},
		keyDef{"Pause", 0x13, true}, keyDef{"ScrollLock", 0x91, true},
	)
}

// Parse reads "F8", "Ctrl+Alt+M", "Shift+Num5"... (case-insensitive). A typing
// key (letter, number, space) needs at least one modifier, otherwise the
// shortcut would steal that key from the whole system.
func Parse(s string) (Spec, error) {
	var sp Spec
	var key *keyDef
	for _, part := range strings.Split(s, "+") {
		p := strings.TrimSpace(part)
		switch strings.ToLower(p) {
		case "":
			return Spec{}, fmt.Errorf("invalid hotkey %q", s)
		case "ctrl", "control":
			sp.Mods |= modCtrl
		case "alt":
			sp.Mods |= modAlt
		case "shift":
			sp.Mods |= modShift
		case "win", "meta":
			sp.Mods |= modWin
		default:
			if key != nil {
				return Spec{}, fmt.Errorf("invalid hotkey %q: more than one key", s)
			}
			for i := range keys {
				if strings.EqualFold(keys[i].name, p) {
					key = &keys[i]
					break
				}
			}
			if key == nil {
				return Spec{}, fmt.Errorf("unknown key %q", p)
			}
		}
	}
	if key == nil {
		return Spec{}, fmt.Errorf("invalid hotkey %q: no key", s)
	}
	if sp.Mods == 0 && !key.free {
		return Spec{}, fmt.Errorf("%s alone would block typing: add Ctrl, Alt or Shift", key.name)
	}
	sp.VK = key.vk
	return sp, nil
}

// String returns the canonical form ("Ctrl+Alt+F8").
func (s Spec) String() string {
	var parts []string
	if s.Mods&modCtrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if s.Mods&modAlt != 0 {
		parts = append(parts, "Alt")
	}
	if s.Mods&modShift != 0 {
		parts = append(parts, "Shift")
	}
	if s.Mods&modWin != 0 {
		parts = append(parts, "Win")
	}
	for _, k := range keys {
		if k.vk == s.VK {
			return strings.Join(append(parts, k.name), "+")
		}
	}
	return strings.Join(append(parts, fmt.Sprintf("0x%X", s.VK)), "+")
}
