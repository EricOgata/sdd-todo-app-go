package buildinfo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVersionStringTagged(t *testing.T) {
	original := Version
	t.Cleanup(func() { Version = original })
	Version = "v0.1.0"
	assert.Equal(t, "todo v0.1.0 (schema 1)", String())
	assert.Equal(t, "todo v0.2.0 (schema 1)", Format("v0.2.0"))
}

func TestVersionStringDev(t *testing.T) {
	original := Version
	t.Cleanup(func() { Version = original })
	Version = "dev"
	assert.Equal(t, "todo dev (schema 1)", String())
}

func TestSchemaVersion(t *testing.T) {
	assert.Equal(t, 1, SchemaVersion)
}
