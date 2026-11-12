package testutil

import (
	"testing"
	"time"

	"todo/internal/clock"
)

// FixedClock returns a fake clock fixed at now.
func FixedClock(t *testing.T, now time.Time) *clock.Fake {
	t.Helper()
	return clock.NewFake(now)
}
