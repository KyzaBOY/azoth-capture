// This file is the Linux implementation of Register/Close. It reads key events
// straight from the kernel input layer (/dev/input/event*), so the
// capture-mark key works on both X11 and Wayland and while the game has focus.
//
// Be clear about what that means: unlike Windows RegisterHotKey, the kernel
// delivers EVERY key event of each device to this process. The code keeps only
// the modifier state and the moment the chosen key goes down; every other event
// is dropped on the spot and never stored, logged or sent.
//
// Reading input devices needs permission, and most desktops do not give it to
// the user by default: the user must be in the "input" group, which lets every
// program that user runs read the keyboard. When no device can be read,
// Register returns an error and the app falls back to the on-screen Mark
// button, as it does on Windows when the key is already taken.
package hotkey

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Kernel input-event constants (linux/input-event-codes.h).
const (
	evKey = 0x01 // an event that carries a key state

	keyRelease = 0 // value 0 = key up
	keyPress   = 1 // value 1 = key down (value 2 is auto-repeat, ignored)

	// One input_event on a 64-bit kernel: struct timeval (16 bytes) + type
	// (u16) + code (u16) + value (s32).
	eventSize   = 24
	offsetType  = 16
	offsetCode  = 18
	offsetValue = 20
)

// modifierKeys maps the evdev keycodes of the modifier keys to the MOD_* bit we
// track for them. Left and right variants both count.
var modifierKeys = map[uint16]uint32{
	29: modCtrl, 97: modCtrl, // left/right control
	42: modShift, 54: modShift, // left/right shift
	56: modAlt, 100: modAlt, // left/right alt (right alt is AltGr)
	125: modWin, 126: modWin, // left/right meta (Super)
}

// Listener watches the input devices for one combination and stops cleanly on
// Close.
type Listener struct {
	files   []*os.File
	wg      sync.WaitGroup
	once    sync.Once
	mu      sync.Mutex
	modsNow uint32
}

// Register starts watching for spec and calls onPress(moment) on each press of
// the combination. The moment is taken the instant the key event is read.
func Register(spec Spec, onPress func(at time.Time)) (*Listener, error) {
	target, ok := vkToEvdev[spec.VK]
	if !ok {
		return nil, fmt.Errorf("%s cannot be used as a global key on Linux", spec)
	}

	paths, err := filepath.Glob("/dev/input/event*")
	if err != nil {
		return nil, err
	}

	l := &Listener{}
	var openErr error
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			openErr = err
			continue
		}
		l.files = append(l.files, f)
	}
	if len(l.files) == 0 {
		if openErr == nil {
			openErr = fmt.Errorf("no input devices found at /dev/input/event*")
		}
		return nil, fmt.Errorf("cannot read the keyboard (%v), so the global key is off; use the Mark button, or see docs/LINUX.md to enable it", openErr)
	}

	for _, f := range l.files {
		l.wg.Add(1)
		go l.watch(f, spec.Mods, target, onPress)
	}
	return l, nil
}

// watch reads one device until Close shuts it down. It keeps the running
// modifier state and fires onPress when the target key goes down with exactly
// the required modifiers held.
func (l *Listener) watch(f *os.File, wantMods uint32, target uint16, onPress func(at time.Time)) {
	defer l.wg.Done()
	buf := make([]byte, eventSize)
	for {
		if err := readFull(f, buf); err != nil {
			return // device closed (Close) or read error: stop quietly
		}
		etype := binary.LittleEndian.Uint16(buf[offsetType:])
		if etype != evKey {
			continue
		}
		code := binary.LittleEndian.Uint16(buf[offsetCode:])
		value := int32(binary.LittleEndian.Uint32(buf[offsetValue:]))

		if bit, isMod := modifierKeys[code]; isMod {
			l.mu.Lock()
			if value == keyRelease {
				l.modsNow &^= bit
			} else {
				l.modsNow |= bit
			}
			l.mu.Unlock()
			continue
		}

		if code == target && value == keyPress {
			at := time.Now()
			l.mu.Lock()
			held := l.modsNow
			l.mu.Unlock()
			if held == wantMods {
				onPress(at)
			}
		}
	}
}

// Close stops watching and releases the devices. It is safe to call more than
// once.
func (l *Listener) Close() error {
	l.once.Do(func() {
		for _, f := range l.files {
			f.Close() // unblocks the reading goroutine
		}
		l.wg.Wait()
	})
	return nil
}

// readFull fills buf completely, or returns the first error. A short read
// across event boundaries is reassembled.
func readFull(f *os.File, buf []byte) error {
	got := 0
	for got < len(buf) {
		n, err := f.Read(buf[got:])
		if n > 0 {
			got += n
		}
		if err != nil {
			return err
		}
	}
	return nil
}
