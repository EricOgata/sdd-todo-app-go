package cli

import (
	"todo/internal/buildinfo"
)

type versionDocument struct {
	Version       string `json:"version"`
	SchemaVersion int    `json:"schema_version"`
}

// versionValue reports the version string used in output.
func (s *rootState) versionValue() string {
	if s.opts.Version != "" {
		return s.opts.Version
	}
	return buildinfo.Version
}

// printVersion writes the text or JSON version document. It never touches the
// database.
func (s *rootState) printVersion(jsonMode bool) error {
	if jsonMode {
		if err := s.printer.WriteJSON(versionDocument{
			Version:       s.versionValue(),
			SchemaVersion: s.schemaVersion(),
		}); err != nil {
			return err
		}
		return errStop
	}
	s.printer.Line("%s", buildinfo.Format(s.versionValue()))
	return errStop
}
