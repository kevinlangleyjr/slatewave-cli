package manifest

import "testing"

// All tests rely on the embedded manifest set having the slugs
// referenced (bat, btop, vscode, alacritty). Adding/removing those
// slugs would break this file — that's intentional, since the embedded
// set is part of the test surface.

func TestSuggestSlug_FindsClosestForOneCharTypo(t *testing.T) {
	cases := []struct {
		typo string
		want string
	}{
		{"btap", "btop"},           // one-char swap
		{"vsccode", "vscode"},      // one extra char
		{"alecritty", "alacritty"}, // one swap mid-word
		{"deltta", "delta"},        // one extra char in short slug
	}
	for _, c := range cases {
		got := SuggestSlug(c.typo)
		if got != c.want {
			t.Errorf("SuggestSlug(%q) = %q, want %q", c.typo, got, c.want)
		}
	}
}

func TestSuggestSlug_ExactMatchReturnsItself(t *testing.T) {
	if got := SuggestSlug("bat"); got != "bat" {
		t.Errorf("SuggestSlug exact match = %q, want bat", got)
	}
}

func TestSuggestSlug_FarOffReturnsEmpty(t *testing.T) {
	// Distance well past the threshold — should not suggest anything.
	if got := SuggestSlug("xyzzy-thunderdome"); got != "" {
		t.Errorf("SuggestSlug far-off = %q, want empty", got)
	}
}

func TestSuggestSlug_EmptyInputReturnsEmpty(t *testing.T) {
	if got := SuggestSlug(""); got != "" {
		t.Errorf("SuggestSlug empty = %q, want empty", got)
	}
}

func TestSuggestSlug_ShortInputUsesTighterThreshold(t *testing.T) {
	// "cat" is distance 1 from "bat" — but with the tightened threshold
	// for slugs ≤ 5 chars, distance 1 should still match (we want
	// transpositions caught; we just don't want totally unrelated short
	// words to map to existing slugs).
	if got := SuggestSlug("cat"); got != "bat" {
		t.Errorf("short typo `cat` should still map to bat: got %q", got)
	}
	// Distance 3 from a 4-char slug should NOT suggest anything since
	// the tightened threshold is 2 for short typos. "btop" is the
	// nearest 4-char slug; pick a 4-char input distance 3 away.
	if got := SuggestSlug("xxxx"); got != "" {
		t.Errorf("4-char unrelated input shouldn't match (threshold tightened): got %q", got)
	}
}

// ----- levenshtein helper -----

func TestLevenshtein_Basics(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "abc", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"kitten", "sitting", 3}, // canonical example
		{"flaw", "lawn", 2},
		{"btap", "btop", 1},
	}
	for _, c := range cases {
		got := levenshtein(c.a, c.b)
		if got != c.want {
			t.Errorf("levenshtein(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestLevenshtein_Symmetric(t *testing.T) {
	// Edit distance is symmetric — d(a,b) == d(b,a). Pin it to catch
	// off-by-one bugs in the row-rotation logic.
	pairs := [][2]string{
		{"bat", "btop"},
		{"vscode", "vsccode"},
		{"alacritty", "alecritty"},
	}
	for _, p := range pairs {
		ab := levenshtein(p[0], p[1])
		ba := levenshtein(p[1], p[0])
		if ab != ba {
			t.Errorf("levenshtein(%q,%q)=%d but (%q,%q)=%d", p[0], p[1], ab, p[1], p[0], ba)
		}
	}
}
