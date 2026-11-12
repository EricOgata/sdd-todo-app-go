// Package output implements the shared output contract: plain lines, aligned
// tables, JSON documents, notes, errors, and ANSI color policy.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// Options configures a Printer.
type Options struct {
	// Out receives program output. It is the only stream scripts read.
	Out io.Writer
	// Err receives diagnostics, notes, and errors.
	Err io.Writer
	// JSON selects machine-readable output for successful commands.
	JSON bool
	// NoColor disables ANSI styling regardless of the terminal state.
	NoColor bool
	// IsTerminal overrides terminal detection. Tests use it to simulate a TTY.
	// A nil value falls back to a descriptor-based check.
	IsTerminal func(io.Writer) bool
}

// Printer is the single output surface for a command invocation.
type Printer struct {
	out   io.Writer
	err   io.Writer
	json  bool
	color bool
	isTTY func(io.Writer) bool
}

// New builds a Printer and resolves the color policy.
func New(opts Options) *Printer {
	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	errw := opts.Err
	if errw == nil {
		errw = io.Discard
	}
	return &Printer{
		out:   out,
		err:   errw,
		json:  opts.JSON,
		color: colorEnabled(out, opts.NoColor, opts.IsTerminal),
		isTTY: opts.IsTerminal,
	}
}

// JSONMode reports whether the invocation requested JSON output.
func (p *Printer) JSONMode() bool { return p.json }

// ColorEnabled reports whether ANSI styling is active.
func (p *Printer) ColorEnabled() bool { return p.color }

// Out returns the stdout writer.
func (p *Printer) Out() io.Writer { return p.out }

// ErrWriter returns the stderr writer.
func (p *Printer) ErrWriter() io.Writer { return p.err }

// Line writes one formatted line to stdout.
func (p *Printer) Line(format string, args ...any) {
	fmt.Fprintf(p.out, format+"\n", args...)
}

// Blank writes an empty line to stdout.
func (p *Printer) Blank() { fmt.Fprintln(p.out) }

// WriteJSON writes exactly one compact JSON document followed by a newline.
// HTML escaping is disabled so <, >, and & stay literal.
func (p *Printer) WriteJSON(v any) error {
	enc := json.NewEncoder(p.out)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// Note writes a non-fatal diagnostic to stderr. It does not change the exit
// code.
func (p *Printer) Note(format string, args ...any) {
	fmt.Fprintf(p.err, "note: "+format+"\n", args...)
}

// Error writes the message that follows the "error: " prefix to stderr.
func (p *Printer) Error(msg string) {
	fmt.Fprintf(p.err, "error: %s\n", msg)
}

// Table writes an aligned table to stdout. Empty cells are the caller's
// responsibility.
func (p *Printer) Table(headers []string, rows [][]string) {
	w := tabwriter.NewWriter(p.out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(headers, "\t"))
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	_ = w.Flush()
}
