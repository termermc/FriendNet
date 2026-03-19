package fsys

import (
	"path/filepath"
	"testing"
)

func TestStrictReplacer_ReplacesWindowsForbiddenASCII(t *testing.T) {
	in := `a\b/c:d*e?f"g<h>i|j`
	want := `a＼b／c꞉d∗e？f＂g‹h›i｜j`

	got := StrictReplacer.ReplaceFilename(in)
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestStrictReplacer_ReplacesControlCharsAndDEL(t *testing.T) {
	in := "A" + string(rune(0x01)) + string(rune(0x1F)) + string(rune(0x7F)) + "Z"
	want := "A␁␟␡Z"

	got := StrictReplacer.ReplaceFilename(in)
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestStrictReplacer_TrimsTrailingSpaceAndDot(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"abc.", "abc"},
		{"abc..", "abc"},
		{"abc ", "abc"},
		{"abc  ", "abc"},
		{"abc . .", "abc"}, // ends with space/dot sequence
		{".", "_"},         // becomes empty after trim => "_"
		{" ", "_"},         // becomes empty after trim => "_"
		{"....  .. ", "_"}, // becomes empty after trim => "_"
	}

	for _, tc := range cases {
		got := StrictReplacer.ReplaceFilename(tc.in)
		if got != tc.want {
			t.Fatalf("in %q: got %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestStrictReplacer_EmptyBecomesUnderscore(t *testing.T) {
	got := StrictReplacer.ReplaceFilename("")
	if got != "_" {
		t.Fatalf("got %q, want %q", got, "_")
	}
}

func TestStrictReplacer_PreservesUnicodeThatIsAllowed(t *testing.T) {
	in := "Résumé—東京📄"
	got := StrictReplacer.ReplaceFilename(in)
	if got != in {
		t.Fatalf("got %q, want %q", got, in)
	}
}

func TestStrictReplacer_WindowsReservedDeviceNames(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"CON", "_CON"},
		{"con", "_con"},
		{"Con", "_Con"},
		{"NUL", "_NUL"},
		{"COM1", "_COM1"},
		{"LPT9", "_LPT9"},

		// Reserved even with extensions
		{"con.txt", "_con.txt"},
		{"Lpt1.prn", "_Lpt1.prn"},

		// Windows trims trailing dots/spaces before checking
		{"con.", "_con"},
		{"con..", "_con"},
		{"con ", "_con"},
		{"con .", "_con"},
		{"LPT1..txt.. ", "_LPT1..txt"}, // trim => "LPT1..txt" base "LPT1" reserved
	}

	for _, tc := range cases {
		got := StrictReplacer.ReplaceFilename(tc.in)
		if got != tc.want {
			t.Fatalf("in %q: got %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestStrictReplacer_DoesNotPrefixNonReservedSimilarNames(t *testing.T) {
	cases := []string{
		"CONSOLE",
		"NULL",
		"COM10", // only COM1..COM9
		"LPT0",  // only LPT1..LPT9
		"AUXILIARY",
		"PRNTER",
		"conman.txt", // base isn't exactly CON
	}

	for _, in := range cases {
		got := StrictReplacer.ReplaceFilename(in)
		if got != in {
			t.Fatalf("in %q: got %q, want %q", in, got, in)
		}
	}
}

func TestStrictReplacer_ReplacePath_ReplacesEachNonEmptySegment(t *testing.T) {
	sep := string(filepath.Separator)

	// Build an OS-native path. We include characters that StrictReplacer replaces
	// within segments.
	in := "a" + sep + "b:c*" + sep + "d?.txt"

	want := "a" + sep + "b꞉c∗" + sep + "d？.txt"

	got := StrictReplacer.ReplacePath(in)
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestStrictReplacer_ReplacePath_DoesNotReplaceEmptySegments(t *testing.T) {
	sep := string(filepath.Separator)

	// Leading, double, and trailing separators create empty segments with Split.
	// ReplacePath now skips empty parts, so structure should be preserved.
	in := sep + "a" + sep + sep + "b" + sep
	want := in

	got := StrictReplacer.ReplacePath(in)
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
