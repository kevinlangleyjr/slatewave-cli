package activator

import (
	"strings"
	"testing"

	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
)

// FuzzTOMLImportRewrite feeds arbitrary content + entry pairs through
// the line-based TOML rewriter. Properties:
//
//  1. No panics. Real users have weird configs (mixed line endings,
//     unicode in comments, half-finished arrays from a botched edit);
//     the activator must survive any input without taking down the CLI.
//
//  2. "changed" must agree with the actual diff: changed=true ↔ output
//     differs; changed=false ↔ output equals input.
//
//  3. Idempotency: running the rewrite twice must yield the same result
//     as running it once. The activator runs on every install; it
//     mustn't keep mutating the file when the entry is already there.
func FuzzTOMLImportRewrite(f *testing.F) {
	f.Add("", "~/.config/alacritty/themes/slatewave.toml")
	f.Add("[general]\nimport = []\n", "~/foo.toml")
	f.Add("[general]\nimport = [\"~/x.toml\"]\n", "~/foo.toml")
	f.Add("[general]\nimport = [\"~/foo.toml\"]\n", "~/foo.toml") // already present
	f.Add("# comment\n[general]\nfoo = 1\n", "~/foo.toml")

	f.Fuzz(func(t *testing.T, content, entry string) {
		if entry == "" {
			t.Skip("empty entry")
		}

		out1, changed1, err := tomlImportRewrite(content, entry)
		if err != nil {
			t.Skip("rewrite returned error — not a panic, fine")
		}
		if changed1 && out1 == content {
			t.Errorf("changed=true but output unchanged\ninput:  %q\noutput: %q", content, out1)
		}
		if !changed1 && out1 != content {
			t.Errorf("changed=false but output differs\ninput:  %q\noutput: %q", content, out1)
		}

		out2, changed2, err := tomlImportRewrite(out1, entry)
		if err != nil {
			t.Skip("second rewrite errored — fine, just no panic")
		}
		if changed2 {
			t.Errorf("not idempotent: second rewrite reported changed\nafter 1: %q\nafter 2: %q", out1, out2)
		}
		if out2 != out1 {
			t.Errorf("not idempotent: output diverged\nafter 1: %q\nafter 2: %q", out1, out2)
		}
	})
}

// FuzzYAMLSetRewrite fuzzes the YAML depth-2 rewriter against a fixed
// pair set (color.when=auto, color.theme=custom — the lsd activation
// shape). Fuzzing the pairs themselves would conflate two layers of
// bug; pinning them lets the fuzzer focus on edge cases in user content.
func FuzzYAMLSetRewrite(f *testing.F) {
	f.Add("")
	f.Add("color:\n  when: never\n  theme: default\n")
	f.Add("color:\n  when: auto\n")
	f.Add("# comment only\n")
	f.Add("classic: true\n")

	pairs := []manifest.YAMLPair{
		{Path: "color.when", Value: "auto"},
		{Path: "color.theme", Value: "custom"},
	}

	f.Fuzz(func(t *testing.T, content string) {
		out1, changed1, err := yamlSetRewrite(content, pairs)
		if err != nil {
			t.Skip("rewrite errored")
		}
		if changed1 && out1 == content {
			t.Errorf("changed=true but output unchanged\ninput:  %q\noutput: %q", content, out1)
		}
		if !changed1 && out1 != content {
			t.Errorf("changed=false but output differs\ninput:  %q\noutput: %q", content, out1)
		}

		out2, changed2, err := yamlSetRewrite(out1, pairs)
		if err != nil {
			t.Skip("second rewrite errored")
		}
		// Second pass on the already-set state must not see any change —
		// every requested key is at its desired value.
		if changed2 {
			t.Errorf("not idempotent: second rewrite changed\nafter 1: %q\nafter 2: %q", out1, out2)
		}
		// Also sanity-check that the output contains every value we set.
		for _, p := range pairs {
			if !strings.Contains(out1, p.Value) {
				t.Errorf("set %q=%q but value missing from output:\n%s", p.Path, p.Value, out1)
			}
		}
	})
}
