package output

import (
	"io"

	"golang.org/x/term"
)

// ANSI escape sequences used by the printer.
const (
	ansiReset  = "\x1b[0m"
	ansiRed    = "\x1b[31m"
	ansiYellow = "\x1b[33m"
	ansiBold   = "\x1b[1m"
)

// colorEnabled resolves the color policy: --no-color always wins, otherwise
// ANSI styling is used only when stdout is a terminal.
func colorEnabled(w io.Writer, noColor bool, isTerminal func(io.Writer) bool) bool {
	if noColor {
		return false
	}
	if isTerminal != nil {
		return isTerminal(w)
	}
	return defaultIsTerminal(w)
}

type fdWriter interface{ Fd() uintptr }

func defaultIsTerminal(w io.Writer) bool {
	f, ok := w.(fdWriter)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// Red wraps s in red ANSI styling when color is enabled.
func (p *Printer) Red(s string) string { return p.style(ansiRed, s) }

// Yellow wraps s in yellow ANSI styling when color is enabled.
func (p *Printer) Yellow(s string) string { return p.style(ansiYellow, s) }

// Bold wraps s in bold ANSI styling when color is enabled.
func (p *Printer) Bold(s string) string { return p.style(ansiBold, s) }

// BoldYellow wraps s in bold yellow ANSI styling when color is enabled.
func (p *Printer) BoldYellow(s string) string {
	if !p.color {
		return s
	}
	return ansiBold + ansiYellow + s + ansiReset
}

func (p *Printer) style(code, s string) string {
	if !p.color {
		return s
	}
	return code + s + ansiReset
}
