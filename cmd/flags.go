package cmd

import "github.com/spf13/pflag"

// flagBool reads a bool flag whose existence is a compile-time
// invariant — every name passed here is wired up via Flags().Bool in
// the command's init(). pflag.GetBool returns an error only when the
// flag isn't defined, which would be a code bug, so we drop it. Saves
// the noise of `v, _ := f.GetBool(...)` at every read site.
func flagBool(f *pflag.FlagSet, name string) bool {
	v, _ := f.GetBool(name)
	return v
}

// flagString is the GetString counterpart to flagBool.
func flagString(f *pflag.FlagSet, name string) string {
	v, _ := f.GetString(name)
	return v
}
