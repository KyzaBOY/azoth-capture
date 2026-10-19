package hotkey

// vkToEvdev maps each key this app allows (named in spec.go, stored as a Windows
// virtual-key code) to its Linux evdev keycode (linux/input-event-codes.h). It
// is built from the same key groups spec.go uses, in the same order, so the two
// files stay in step.
var vkToEvdev = buildVKToEvdev()

func buildVKToEvdev() map[uint32]uint16 {
	m := make(map[uint32]uint16)

	// Function keys F1..F24. VK = 0x6F + i (spec.go).
	fKeys := []uint16{
		59, 60, 61, 62, 63, 64, 65, 66, 67, 68, // F1..F10
		87, 88, // F11, F12
		183, 184, 185, 186, 187, 188, 189, 190, 191, 192, 193, 194, // F13..F24
	}
	for i, code := range fKeys {
		m[uint32(0x6F+(i+1))] = code
	}

	// Letters A..Z. VK = 'A' + offset.
	letters := []uint16{
		30, 48, 46, 32, 18, 33, 34, 35, 23, 36, 37, 38, 50, // A..M
		49, 24, 25, 16, 19, 31, 20, 22, 47, 17, 45, 21, 44, // N..Z
	}
	for i, code := range letters {
		m[uint32('A'+i)] = code
	}

	// Digits 0..9 on the top row. VK = '0' + digit.
	digits := []uint16{11, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	for d, code := range digits {
		m[uint32('0'+d)] = code
	}

	// Numpad Num0..Num9. VK = 0x60 + i (spec.go).
	numpad := []uint16{82, 79, 80, 81, 75, 76, 77, 71, 72, 73}
	for i, code := range numpad {
		m[uint32(0x60+i)] = code
	}

	// Named keys that spec.go also allows.
	specials := map[uint32]uint16{
		0x20: 57,  // Space
		0x2D: 110, // Insert
		0x2E: 111, // Delete
		0x24: 102, // Home
		0x23: 107, // End
		0x21: 104, // PageUp
		0x22: 109, // PageDown
		0x13: 119, // Pause
		0x91: 70,  // ScrollLock
	}
	for vk, code := range specials {
		m[vk] = code
	}

	return m
}
