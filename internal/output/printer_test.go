package output

import (
	"io"
	"os"
	"strings"
	"testing"
)

// compile-time guard: *os.File satisfies the descriptor interface used by
// terminal detection.
var _ fdWriter = (*os.File)(nil)

func TestJSONSingleDocument(t *testing.T) {
	var out, errw strings.Builder
	p := New(Options{Out: &out, Err: &errw, JSON: true})
	if err := p.WriteJSON(map[string]any{"ok": true}); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	if got, want := out.String(), "{\"ok\":true}\n"; got != want {
		t.Fatalf("JSON output = %q, want %q", got, want)
	}
	if errw.Len() != 0 {
		t.Fatalf("stderr should stay empty, got %q", errw.String())
	}
}

func TestJSONHTMLNotEscaped(t *testing.T) {
	var out strings.Builder
	p := New(Options{Out: &out, JSON: true})
	if err := p.WriteJSON(map[string]string{"text": "<a> & <b>"}); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	if got, want := out.String(), "{\"text\":\"<a> & <b>\"}\n"; got != want {
		t.Fatalf("JSON output = %q, want %q", got, want)
	}
}

func TestJSONNullAndEmptyArrays(t *testing.T) {
	var out strings.Builder
	type payload struct {
		Due  *string  `json:"due"`
		Tags []string `json:"tags"`
	}
	p := New(Options{Out: &out, JSON: true})
	if err := p.WriteJSON(payload{Tags: []string{}}); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	if got, want := out.String(), "{\"due\":null,\"tags\":[]}\n"; got != want {
		t.Fatalf("JSON output = %q, want %q", got, want)
	}
}

func TestLineOutput(t *testing.T) {
	var out strings.Builder
	p := New(Options{Out: &out})
	p.Line("hello %s", "world")
	if got, want := out.String(), "hello world\n"; got != want {
		t.Fatalf("line = %q, want %q", got, want)
	}
}

func TestNoteGoesToStderr(t *testing.T) {
	var out, errw strings.Builder
	p := New(Options{Out: &out, Err: &errw})
	p.Note("heads up")
	if got, want := errw.String(), "note: heads up\n"; got != want {
		t.Fatalf("note = %q, want %q", got, want)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout should stay empty, got %q", out.String())
	}
}

func TestErrorGoesToStderr(t *testing.T) {
	var out, errw strings.Builder
	p := New(Options{Out: &out, Err: &errw})
	p.Error("bad input")
	if got, want := errw.String(), "error: bad input\n"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout should stay empty, got %q", out.String())
	}
}

func TestColorDisabledWhenNotTTY(t *testing.T) {
	var out strings.Builder
	p := New(Options{Out: &out})
	if p.ColorEnabled() {
		t.Fatal("color should be disabled for a non-TTY writer")
	}
	if got, want := p.Red("x"), "x"; got != want {
		t.Fatalf("styled = %q, want %q", got, want)
	}
}

func TestNoColorFlag(t *testing.T) {
	var out strings.Builder
	p := New(Options{
		Out:        &out,
		IsTerminal: func(io.Writer) bool { return true },
		NoColor:    true,
	})
	if p.ColorEnabled() {
		t.Fatal("--no-color must disable styling")
	}
	if got, want := p.Bold("x"), "x"; got != want {
		t.Fatalf("styled = %q, want %q", got, want)
	}
}

func TestColorWhenTTY(t *testing.T) {
	var out strings.Builder
	p := New(Options{
		Out:        &out,
		IsTerminal: func(io.Writer) bool { return true },
	})
	if !p.ColorEnabled() {
		t.Fatal("color should be enabled for a TTY")
	}
	if got, want := p.Red("x"), ansiRed+"x"+ansiReset; got != want {
		t.Fatalf("styled = %q, want %q", got, want)
	}
}

func TestTableAlignment(t *testing.T) {
	var out strings.Builder
	p := New(Options{Out: &out})
	p.Table(
		[]string{"ID", "TITLE", "TAGS"},
		[][]string{
			{"1", "short", "a"},
			{"22", "a longer title", "b, c"},
		},
	)
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %q", len(lines), out.String())
	}
	// The TITLE column must start at the same offset on every row.
	titleOffset := strings.Index(lines[1], "short")
	if titleOffset < 0 || strings.Index(lines[2], "a longer title") != titleOffset {
		t.Fatalf("columns are not aligned:\n%s", out.String())
	}
	if strings.Index(lines[0], "TITLE") != titleOffset {
		t.Fatalf("header is not aligned:\n%s", out.String())
	}
}
