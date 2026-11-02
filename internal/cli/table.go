package cli

import (
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/ardacoskun/codehop/internal/picker"
)

type style string

const (
	styleNone    style = ""
	styleBold    style = "1"
	styleDim     style = "2"
	styleRed     style = "31"
	styleGreen   style = "32"
	styleYellow  style = "33"
	styleCyan    style = "36"
	styleMagenta style = "35"
)

type cell struct {
	text  string
	style style
}

// table aligns columns by visible width, so color codes do not break
// alignment the way they do with text/tabwriter.
type table struct {
	w     io.Writer
	color bool
	rows  [][]cell
}

func newTable(w io.Writer, color bool) *table {
	return &table{w: w, color: color}
}

func (t *table) row(cells ...cell) {
	t.rows = append(t.rows, cells)
}

func (t *table) flush() {
	var widths []int
	for _, r := range t.rows {
		for i, c := range r {
			if i >= len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], utf8.RuneCountInString(c.text))
		}
	}
	var b strings.Builder
	for _, r := range t.rows {
		b.Reset()
		for i, c := range r {
			if i > 0 {
				b.WriteString("  ")
			}
			b.WriteString(t.paint(c))
			if i < len(r)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(c.text)))
			}
		}
		b.WriteByte('\n')
		_, _ = io.WriteString(t.w, b.String())
	}
	t.rows = nil
}

func (t *table) paint(c cell) string {
	if !t.color || c.style == styleNone {
		return c.text
	}
	return "\x1b[" + string(c.style) + "m" + c.text + "\x1b[0m"
}

// colorEnabled follows the NO_COLOR convention and only colors terminals.
func colorEnabled(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	return ok && picker.IsTerminal(f)
}
