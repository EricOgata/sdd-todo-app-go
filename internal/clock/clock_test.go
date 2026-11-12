package clock

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTodayUsesLocalDate(t *testing.T) {
	loc := time.FixedZone("UTC-05", -5*60*60)
	instant := time.Date(2026, 11, 11, 23, 30, 0, 0, loc)
	today := TodayIn(instant)
	assert.Equal(t, time.Date(2026, 11, 11, 0, 0, 0, 0, loc), today)
	assert.Equal(t, "2026-11-11", Date(today))
}

func TestFakeClock(t *testing.T) {
	loc := time.FixedZone("UTC+02", 2*60*60)
	instant := time.Date(2026, 11, 12, 1, 15, 0, 0, loc)
	f := NewFake(instant)
	assert.Equal(t, instant, f.Now())
	assert.Equal(t, time.Date(2026, 11, 12, 0, 0, 0, 0, loc), f.Today())

	f.Advance(24 * time.Hour)
	assert.Equal(t, time.Date(2026, 11, 13, 1, 15, 0, 0, loc), f.Now())
	assert.Equal(t, "2026-11-13", Date(f.Today()))
}

func TestUTCForTimestamps(t *testing.T) {
	loc := time.FixedZone("UTC+02", 2*60*60)
	instant := time.Date(2026, 11, 12, 1, 15, 0, 0, loc)
	assert.Equal(t, "2026-11-11T23:15:00Z", Timestamp(instant))
}
