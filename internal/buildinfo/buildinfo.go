// Package buildinfo carries build-time metadata and the supported schema
// version.
package buildinfo

import "fmt"

// Version is injected at build time with
//
//	-ldflags "-X todo/internal/buildinfo.Version=v0.1.0"
//
// An untagged build reports "dev".
var Version = "dev"

// SchemaVersion is the highest database schema version this binary supports.
const SchemaVersion = 1

// Format renders the documented version line for a version value.
func Format(version string) string {
	if version == "" {
		version = Version
	}
	return fmt.Sprintf("todo %s (schema %d)", version, SchemaVersion)
}

// String renders the documented version line for the compiled-in version.
func String() string { return Format(Version) }
