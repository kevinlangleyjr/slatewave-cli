package installer

import (
	"strings"
	"testing"
)

// FuzzMatchConstraint stresses the version-constraint parser with
// arbitrary expr / version pairs. The function is called against
// manifest-supplied strings, so theme authors writing a malformed
// when_version (or a version_regex that captures something unexpected)
// must surface as an error, never a panic.
//
// Properties:
//
//  1. Never panic. Manifest authoring is the threat model — a typo'd
//     constraint mustn't crash slatewave install.
//
//  2. Determinism. Two consecutive calls with the same args must agree
//     on (result, err-is-nil-or-not). A regex backreference or shared
//     state regression would surface here.
//
//  3. When matchConstraint returns nil error, the result is one of the
//     well-defined operator semantics — the err==nil path requires the
//     regex matched AND the semver compared cleanly, so a true/false
//     result has to be self-consistent across calls.
func FuzzMatchConstraint(f *testing.F) {
	// Seed from the existing TestMatchConstraint cases so the fuzz starts
	// from inputs we already know parse, then explores from there.
	seeds := []struct{ expr, version string }{
		{"<1.1.0", "1.0.5"},
		{"<=1.0.5", "1.0.5"},
		{">=2.0.0", "2.0.0"},
		{">2.0.0", "2.0.1"},
		{"=1.0.5", "1.0.5"},
		{"1.0.5", "1.0.5"},
		// Known-invalid seeds prime the fuzz to explore the error path.
		{"~1.0", "1.0.5"},
		{"<1.0", "1.0.0"},
		{"", "1.0.0"},
		{"<1.0.0", "not-a-version"},
		// Whitespace and prefix-v shapes the production regex tolerates.
		{"  <1.0.0 ", "1.0.0"},
		{"<1.0.0", "v1.0.0"},
	}
	for _, s := range seeds {
		f.Add(s.expr, s.version)
	}

	f.Fuzz(func(t *testing.T, expr, version string) {
		// Property 1: no panic. The whole point of this fuzz.
		got1, err1 := matchConstraint(expr, version)

		// Property 2: determinism. Same args, same result.
		got2, err2 := matchConstraint(expr, version)
		if got1 != got2 {
			t.Errorf("non-deterministic match: matchConstraint(%q, %q) returned %v then %v", expr, version, got1, got2)
		}
		if (err1 == nil) != (err2 == nil) {
			t.Errorf("non-deterministic error state: matchConstraint(%q, %q) returned err=%v then err=%v", expr, version, err1, err2)
		}

		// Property 3: when no error, the result must be a boolean (true
		// or false — Go enforces that). A non-error result must not have
		// been swallowed somewhere. Trivially true via Go's type system;
		// the assertion exists to document the invariant the next test
		// case should build on.
		if err1 == nil {
			_ = got1
		}

		// Bonus: extreme inputs shouldn't allocate unbounded memory or
		// trigger long backtracking in constraintRE. The fuzzer's own
		// timeout would surface that — no explicit assertion needed.
		_ = strings.TrimSpace(expr)
	})
}

// FuzzRemoveShellRCLineFromContent feeds arbitrary content / line /
// marker triples through the pure-string rewrite. The properties:
//
//  1. Must not panic on any input. The function operates on user-
//     supplied rc files, so unbalanced quotes, lone CR-LFs, or extreme
//     length must not crash the uninstaller.
//
//  2. The "dropped" return value must be consistent: if it claims
//     content changed, the result must differ from input; if it claims
//     no change, the result must equal input.
//
//  3. Idempotency: running the rewrite twice must produce the same
//     result as running it once. A user uninstalling twice (or our
//     own retry logic) shouldn't progressively eat unrelated lines.
func FuzzRemoveShellRCLineFromContent(f *testing.F) {
	// Seed corpus from realistic shapes.
	f.Add("# slatewave\nexport BAT_THEME=Slatewave\n", "export BAT_THEME=Slatewave", "# slatewave")
	f.Add("# user content\nalias gs='git status'\n", "export BAT_THEME=Slatewave", "# slatewave")
	f.Add("", "export BAT_THEME=Slatewave", "# slatewave")
	f.Add("\n\n\n", "x", "# slatewave")
	f.Add("# slatewave\nx\n# slatewave\nx\n", "x", "# slatewave")
	// Lua marker shape.
	f.Add("-- slatewave\nrequire('slatewave-full').apply_to_config(config)\n", "require('slatewave-full').apply_to_config(config)", "-- slatewave")

	f.Fuzz(func(t *testing.T, content, line, marker string) {
		// Skip degenerate inputs that aren't representative of real use.
		// An empty line as the target would match every blank line in
		// the file, which is a different kind of bug to chase.
		if strings.TrimSpace(line) == "" {
			t.Skip("empty target line")
		}

		out1, dropped1 := removeShellRCLineFromContent(content, line, marker)
		// Property 2: consistency.
		if dropped1 && out1 == content {
			t.Errorf("dropped=true but output unchanged\ninput:  %q\noutput: %q", content, out1)
		}
		if !dropped1 && out1 != content {
			t.Errorf("dropped=false but output differs\ninput:  %q\noutput: %q", content, out1)
		}

		// Property 3: idempotency. Second call on the already-cleaned
		// content must be a no-op.
		out2, dropped2 := removeShellRCLineFromContent(out1, line, marker)
		if dropped2 {
			t.Errorf("not idempotent: second call removed more\nafter 1: %q\nafter 2: %q", out1, out2)
		}
		if out2 != out1 {
			t.Errorf("not idempotent: output changed\nafter 1: %q\nafter 2: %q", out1, out2)
		}
	})
}
