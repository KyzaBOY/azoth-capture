// Package hotkey registra um atalho GLOBAL do Windows com RegisterHotKey, que
// funciona com o jogo em foco. É só isso: sem hook de teclado de baixo nível,
// sem ler tecla de outro processo. O Windows entrega a mensagem WM_HOTKEY pra
// uma thread nossa quando (e só quando) a combinação registrada é apertada.
package hotkey

import (
	"fmt"
	"strings"
)

// Modificadores do Windows (MOD_*).
const (
	modAlt   = 0x0001
	modCtrl  = 0x0002
	modShift = 0x0004
	modWin   = 0x0008
)

// Spec é uma combinação: modificadores + tecla virtual.
type Spec struct {
	Mods uint32
	VK   uint32
}

type keyDef struct {
	name string
	vk   uint32
	free bool // pode ser usada sem modificador (não atrapalha a digitação)
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

// Parse lê "F8", "Ctrl+Alt+M", "Shift+Num5"... (sem diferenciar maiúsculas).
// Tecla de digitação (letra, número, espaço) exige pelo menos um modificador,
// senão o atalho roubaria a tecla do sistema inteiro.
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

// String devolve a forma canônica ("Ctrl+Alt+F8").
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
