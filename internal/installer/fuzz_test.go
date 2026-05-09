package installer

import (
	"strings"
	"testing"
)

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
