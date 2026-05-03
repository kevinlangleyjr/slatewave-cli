package manifest

// SuggestSlug finds the closest known slug to `typo` by Levenshtein
// distance and returns it for "did you mean X?" hints. Returns "" if
// no slug is within a useful threshold or if the manifest set fails
// to load — callers should treat empty as "no suggestion to offer."
//
// Threshold is distance ≤ 3 for normal-length slugs (catches most
// transpositions and one-or-two-key typos: btap→btop, alecritty→
// alacritty, vsccode→vscode) and ≤ 2 for short slugs (≤ 5 chars) so
// we don't suggest "bat" when the user typed "cat" — too aggressive.
func SuggestSlug(typo string) string {
	if typo == "" {
		return ""
	}
	all, err := LoadAll()
	if err != nil {
		return ""
	}

	threshold := 3
	if len(typo) <= 5 {
		threshold = 2
	}

	bestDist := threshold + 1
	bestSlug := ""
	for _, t := range all {
		d := levenshtein(typo, t.Theme.Slug)
		if d < bestDist {
			bestDist = d
			bestSlug = t.Theme.Slug
		}
	}
	if bestDist > threshold {
		return ""
	}
	return bestSlug
}

// levenshtein computes edit distance between two ASCII strings using
// Wagner-Fischer with two rolling rows. Slugs are always ASCII so we
// can index by byte without UTF-8 concerns.
func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}
