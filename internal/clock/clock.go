// Package clock provides the time abstraction used by date-sensitive
// commands. Production code uses System; tests use Fake for deterministic
// results.
package clock

import (
	"sync"
	"time"
)

// DateLayout is the calendar-date layout used for stored due dates and JSON
// date fields.
const DateLayout = "2006-01-02"

// Clock exposes the current instant and the local calendar day.
type Clock interface {
	// Now returns the current instant.
	Now() time.Time
	// Today returns the start of the current local calendar day.
	Today() time.Time
}

// System is the production clock.
type System struct{}

// Now returns the current instant in the local timezone.
func (System) Now() time.Time { return time.Now() }

// Today returns the start of the current local calendar day.
func (System) Today() time.Time { return StartOfDay(time.Now()) }

// StartOfDay truncates t to midnight in t's location.
func StartOfDay(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
}

// TodayIn returns the start of the calendar day containing t, in t's
// location. It is exported so tests and helpers can reason about a fixed
// instant without a Clock.
func TodayIn(t time.Time) time.Time { return StartOfDay(t) }

// Date formats a calendar date as YYYY-MM-DD.
func Date(t time.Time) string { return t.Format(DateLayout) }

// Timestamp formats an instant as RFC3339 in UTC.
func Timestamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// Fake is a controllable clock for tests.
type Fake struct {
	mu  sync.Mutex
	now time.Time
}

// NewFake returns a Fake fixed at now.
func NewFake(now time.Time) *Fake { return &Fake{now: now} }

// Now returns the fake instant.
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// Today returns the start of the fake instant's local calendar day.
func (f *Fake) Today() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return StartOfDay(f.now)
}

// Set replaces the fake instant.
func (f *Fake) Set(now time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = now
}

// Advance moves the fake instant by d.
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}
