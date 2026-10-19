package hotkey

import "testing"

// TestEveryKeyMapped makes sure the Linux keycode table covers every key the
// shared spec.go allows, so no allowed hotkey is unusable on Linux.
func TestEveryKeyMapped(t *testing.T) {
	for _, k := range keys {
		if _, ok := vkToEvdev[k.vk]; !ok {
			t.Errorf("key %s (VK 0x%X) has no Linux keycode", k.name, k.vk)
		}
	}
}

func TestKnownKeycodes(t *testing.T) {
	// VK code -> expected evdev keycode (name for the message).
	cases := []struct {
		name string
		vk   uint32
		want uint16
	}{
		{"F8", 0x77, 66}, // default hotkey
		{"A", 0x41, 30},
		{"0", 0x30, 11},
		{"Num5", 0x65, 76},
		{"Space", 0x20, 57},
	}
	for _, c := range cases {
		if got := vkToEvdev[c.vk]; got != c.want {
			t.Errorf("%s (VK 0x%X) -> evdev %d, want %d", c.name, c.vk, got, c.want)
		}
	}
}
