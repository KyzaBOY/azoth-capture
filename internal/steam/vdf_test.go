//go:build linux

package steam

import "testing"

func TestVDFTokens(t *testing.T) {
	in := `
"libraryfolders"
{
	"0"
	{
		"path"		"/home/u/.local/share/Steam"
		"apps"
		{
			"1063730"	"12345"
		}
	}
}
`
	got := vdfTokens(in)
	want := []string{"libraryfolders", "0", "path", "/home/u/.local/share/Steam", "apps", "1063730", "12345"}
	if len(got) != len(want) {
		t.Fatalf("got %d tokens %v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("token %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestVDFTokensEscapedQuote(t *testing.T) {
	got := vdfTokens(`"name" "a\"b"`)
	if len(got) != 2 || got[1] != `a"b` {
		t.Fatalf("escaped quote not handled: %q", got)
	}
}
