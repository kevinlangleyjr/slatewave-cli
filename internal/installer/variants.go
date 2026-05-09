package installer

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/mod/semver"

	"github.com/kevinlangleyjr/slatewave-cli/internal/manifest"
	"github.com/kevinlangleyjr/slatewave-cli/internal/shell"
)

// constraintRE captures the operator (group 1) and the X.Y.Z version
// (group 2) of a WhenVersion expression. Operator is one of
// `<`, `<=`, `>`, `>=`, `=`, or empty (bare version → exact match).
// Trailing/leading whitespace is tolerated by callers via TrimSpace.
var constraintRE = regexp.MustCompile(`^(<=|>=|<|>|=)?\s*([0-9]+\.[0-9]+\.[0-9]+)$`)

// matchConstraint reports whether version satisfies expr. version must
// be X.Y.Z (no leading "v"); golang.org/x/mod/semver requires the "v"
// prefix and ensureV adds it before comparison.
func matchConstraint(expr, version string) (bool, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return false, fmt.Errorf("empty version constraint")
	}
	m := constraintRE.FindStringSubmatch(expr)
	if m == nil {
		return false, fmt.Errorf("invalid version constraint %q (want one of <X.Y.Z, <=X.Y.Z, >X.Y.Z, >=X.Y.Z, =X.Y.Z, or bare X.Y.Z)", expr)
	}
	op, want := m[1], m[2]
	got := ensureV(version)
	bound := ensureV(want)
	if !semver.IsValid(got) {
		return false, fmt.Errorf("detected version %q is not valid semver", version)
	}
	cmp := semver.Compare(got, bound)
	switch op {
	case "<":
		return cmp < 0, nil
	case "<=":
		return cmp <= 0, nil
	case ">":
		return cmp > 0, nil
	case ">=":
		return cmp >= 0, nil
	case "=", "":
		return cmp == 0, nil
	default:
		return false, fmt.Errorf("invalid version constraint %q", expr)
	}
}

func ensureV(v string) string {
	if strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}

// resolveVariant returns the first variant whose WhenVersion matches
// version, or nil when none match. Declaration order wins on overlap —
// matching the manifest author's intent that the variants list is read
// top-to-bottom.
func resolveVariant(variants []manifest.InstallVariant, version string) (*manifest.InstallVariant, error) {
	for i := range variants {
		match, err := matchConstraint(variants[i].WhenVersion, version)
		if err != nil {
			return nil, fmt.Errorf("install.variants[%d]: %w", i, err)
		}
		if match {
			return &variants[i], nil
		}
	}
	return nil, nil
}

// detectVersion runs Theme.DetectCommand and applies Theme.VersionRegex
// to capture the installed version. Returns ("", nil) when VersionRegex
// is unset — the caller treats that as "no variant logic" and skips the
// resolve step. A regex miss returns an error rather than falling back
// to defaults: silently shipping the default URL on a host whose version
// the manifest specifically calls out would re-introduce the bug we're
// trying to fix.
func detectVersion(t manifest.Theme) (string, error) {
	if t.Theme.VersionRegex == "" {
		return "", nil
	}
	re, err := regexp.Compile(t.Theme.VersionRegex)
	if err != nil {
		return "", fmt.Errorf("compile version_regex %q: %w", t.Theme.VersionRegex, err)
	}
	cmd := manifest.DetectCommandFor(t)
	if cmd == "" {
		return "", fmt.Errorf("version_regex set but no detect_command")
	}
	ctx, cancel := context.WithTimeout(context.Background(), detectTimeout)
	defer cancel()
	out, err := shell.Run(ctx, cmd)
	if err != nil {
		return "", fmt.Errorf("run detect_command %q: %w (output: %s)", cmd, err, strings.TrimSpace(string(out)))
	}
	m := re.FindStringSubmatch(string(out))
	if len(m) < 2 {
		return "", fmt.Errorf("version_regex %q did not capture a version from %q output: %s", t.Theme.VersionRegex, cmd, strings.TrimSpace(string(out)))
	}
	return m[1], nil
}

// applyVariant returns a copy of in with v's non-empty fields layered
// on top. When v sets Files, the single-file fields (URL/Dest) are
// cleared so the install dispatch sees a clean multi-file shape — the
// inverse case is left to the caller (a variant that overrides URL on
// a parent that uses Files would result in both being set, which
// curlFiles surfaces as "pick one"). v == nil returns in unchanged.
func applyVariant(in manifest.Install, v *manifest.InstallVariant) manifest.Install {
	if v == nil {
		return in
	}
	if len(v.Files) > 0 {
		in.Files = v.Files
		in.URL = ""
		in.Dest = ""
		return in
	}
	if v.URL != "" {
		in.URL = v.URL
	}
	if v.Dest != "" {
		in.Dest = v.Dest
	}
	return in
}
