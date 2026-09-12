package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Popup classes match the shared branchctl convention: fixed large surfaces
// for scrollable forms, automatic medium surfaces for finite confirmations.
type popupSize int

const (
	popupSmall popupSize = iota
	popupMedium
	popupLarge
)

type popupFit int

const (
	fitFixed popupFit = iota
	fitAuto
)

type popupWidth int

const (
	widthClass popupWidth = iota
	widthAuto
)

type popupOpts struct {
	Size         popupSize
	Fit          popupFit
	Width        popupWidth
	Title        string
	Danger       bool
	Body, Footer []string
}

func (m appModel) popupDims(size popupSize) (int, int) {
	w, h, floor := m.width*2/3, m.height*2/3, 78
	if size == popupSmall {
		w, h, floor = m.width/3, m.height/2, 50
	}
	if size == popupMedium {
		w, h, floor = m.width/2, m.height/2, 60
	}
	w = min(120, max(floor, w))
	w = min(w, m.width-2)
	h = max(12, h)
	h = min(h, m.height)
	return max(1, w), max(1, h)
}

func (m appModel) popupInnerW(size popupSize) int { w, _ := m.popupDims(size); return max(1, w-4) }
func popupWrap(lines []string, width int) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			out = append(out, "")
			continue
		}
		out = append(out, strings.Split(ansi.Wrap(line, width, ""), "\n")...)
	}
	return out
}
func popupPad(line string, width int) string {
	line = ansi.Truncate(line, width, "…")
	return line + strings.Repeat(" ", max(0, width-ansi.StringWidth(line)))
}
func popupBox(m appModel, options popupOpts) string {
	width, height := m.popupDims(options.Size)
	inner := width - 4
	body := popupWrap(options.Body, inner)
	footer := popupWrap(options.Footer, inner)
	rows := height - 2 - len(footer)
	if len(footer) > 0 {
		rows--
	}
	rows = max(1, rows)
	if len(body) > rows {
		body = append(body[:max(0, rows-1)], dimStyle.Render("… more lines"))
	}
	if options.Fit == fitFixed {
		for len(body) < rows {
			body = append(body, "")
		}
	}
	if len(footer) > 0 {
		body = append(body, "")
		body = append(body, footer...)
	}
	for index := range body {
		body[index] = popupPad(body[index], inner)
	}
	title := options.Title
	if options.Danger {
		title = "⚠  " + title
	}
	boxHeight := len(body) + 2
	if options.Fit == fitFixed {
		boxHeight = height
	}
	return panel(title, strings.Join(body, "\n"), width, boxHeight, true)
}
