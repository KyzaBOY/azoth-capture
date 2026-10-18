package hotkey

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	procRegisterHotKey   = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey = user32.NewProc("UnregisterHotKey")
	procGetMessageW      = user32.NewProc("GetMessageW")
	procPeekMessageW     = user32.NewProc("PeekMessageW")
	procPostThreadMsgW   = user32.NewProc("PostThreadMessageW")
)

const (
	wmHotkey      = 0x0312
	wmQuit        = 0x0012
	wmUser        = 0x0400
	pmNoRemove    = 0x0000
	modNoRepeat   = 0x4000 // segurar a tecla não repete o atalho
	errHotkeyBusy = 1409   // ERROR_HOTKEY_ALREADY_REGISTERED
)

type point struct{ X, Y int32 }

type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

// Listener é um atalho registrado, com a thread de mensagens própria dele
// (RegisterHotKey liga o atalho à thread que o registrou, e a fila dessa
// thread precisa de um loop de GetMessage).
type Listener struct {
	tid  uint32
	done chan struct{}
	once sync.Once
}

// Register registra o atalho e chama onPress(instante) a cada aperto. O instante
// é tomado na própria thread de mensagens, antes de qualquer outro trabalho.
// onPress roda nessa thread: tem que ser rápido e não pode chamar Close.
func Register(spec Spec, onPress func(at time.Time)) (*Listener, error) {
	type ready struct {
		tid uint32
		err error
	}
	rdy := make(chan ready, 1)
	l := &Listener{done: make(chan struct{})}

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(l.done)

		tid := windows.GetCurrentThreadId()
		var m msg
		// força a criação da fila de mensagens da thread antes de avisar que está pronta
		procPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, wmUser, wmUser, pmNoRemove)

		r, _, e := procRegisterHotKey.Call(0, 1, uintptr(spec.Mods|modNoRepeat), uintptr(spec.VK))
		if r == 0 {
			if errno, ok := e.(windows.Errno); ok && errno == errHotkeyBusy {
				rdy <- ready{err: fmt.Errorf("%s is already used by another program", spec)}
			} else {
				rdy <- ready{err: fmt.Errorf("could not register %s: %v", spec, e)}
			}
			return
		}
		defer procUnregisterHotKey.Call(0, 1)
		rdy <- ready{tid: tid}

		for {
			r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(r) <= 0 { // WM_QUIT (0) ou erro (-1)
				return
			}
			if m.Message == wmHotkey {
				onPress(time.Now())
			}
		}
	}()

	res := <-rdy
	if res.err != nil {
		<-l.done
		return nil, res.err
	}
	l.tid = res.tid
	return l, nil
}

// Close desfaz o atalho e encerra a thread. Pode ser chamado mais de uma vez.
func (l *Listener) Close() error {
	var err error
	l.once.Do(func() {
		r, _, e := procPostThreadMsgW.Call(uintptr(l.tid), wmQuit, 0, 0)
		if r == 0 {
			err = errors.New("could not stop the hotkey thread: " + e.Error())
			return
		}
		<-l.done
	})
	return err
}
