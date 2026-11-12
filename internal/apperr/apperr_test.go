package apperr

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExitCodeMapping(t *testing.T) {
	assert.Equal(t, 0, ExitCode(nil))
	assert.Equal(t, 1, ExitCode(User("bad")))
	assert.Equal(t, 2, ExitCode(System("boom")))
	assert.Equal(t, 2, ExitCode(errors.New("raw")))
}

func TestUserMessage(t *testing.T) {
	err := User("title is required")
	assert.Equal(t, "title is required", Message(err))
	assert.True(t, IsUser(err))
}

func TestSystemMessage(t *testing.T) {
	err := System("database is locked")
	assert.Equal(t, "database is locked", Message(err))
	assert.False(t, IsUser(err))
}

func TestUnexpectedWrapping(t *testing.T) {
	err := Unexpected(errors.New("disk on fire"))
	require.Error(t, err)
	assert.Equal(t, "unexpected failure: disk on fire", Message(err))
	assert.Equal(t, 2, ExitCode(err))
}

func TestUnexpectedPreservesTyped(t *testing.T) {
	err := User("bad")
	assert.Equal(t, err, Unexpected(err))
}

func TestClassifyCobraErrors(t *testing.T) {
	cases := []string{
		`unknown command "bogus" for "todo"`,
		"unknown flag: --bogus",
		`invalid argument "x" for "todo add"`,
		"required flag(s) \"title\" not set",
		"flag needs an argument: --db",
	}
	for _, msg := range cases {
		t.Run(msg, func(t *testing.T) {
			err := ClassifyCobra(errors.New(msg))
			assert.True(t, IsUser(err))
			assert.Equal(t, 1, ExitCode(err))
			assert.Equal(t, msg, Message(err))
		})
	}
}

func TestClassifyCobraTruncatesSuggestions(t *testing.T) {
	err := ClassifyCobra(errors.New("unknown command \"bogus\" for \"todo\"\n\nDid you mean this?\n\tbook"))
	assert.Equal(t, `unknown command "bogus" for "todo"`, Message(err))
}

func TestClassifyCobraCancelled(t *testing.T) {
	err := ClassifyCobra(fmt.Errorf("wrapped: %w", context.Canceled))
	assert.Equal(t, 2, ExitCode(err))
	assert.Equal(t, "interrupted.", Message(err))
}

func TestClassifyCobraPreservesTyped(t *testing.T) {
	original := User("nope")
	assert.Equal(t, original, ClassifyCobra(original))
}
