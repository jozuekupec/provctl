package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"provctl/internal/fsbrowse"
)

// pathEntry includes the synthetic parent row. It is data rather than render
// chrome so filtering and cursor movement always refer to the same rows.
type pathEntry struct {
	fsbrowse.Entry
	parent bool
}

func settingPathMode(index int) (fsbrowse.Mode, bool) {
	if index < 0 || index >= len(settingFields) {
		return fsbrowse.Dirs, false
	}
	switch settingFields[index].section + "." + settingFields[index].key {
	case "paths.vhosts", "paths.backups", "paths.acme_challenge", "apache.sites_available", "apache.sites_enabled":
		return fsbrowse.Dirs, true
	case "mariadb.defaults_file", "users.shell":
		return fsbrowse.All, true
	default:
		return fsbrowse.Dirs, false
	}
}

func (m appModel) openPathPickerForSetting() (appModel, tea.Cmd) {
	m = m.persistSettingInput()
	field := m.activeSetting()
	mode, ok := settingPathMode(field)
	if !ok {
		return m, nil
	}
	input := newFilter().input
	input.Prompt = ""
	input.SetValue(m.settings.values[field])
	m.pathPicker = pathPickerState{
		open: true, field: field, mode: mode, loading: true,
		generation: m.pathPicker.generation + 1, filter: newFilter(), pathInput: input,
	}
	m.status = ""
	return m, m.listPathCmd(m.settings.values[field], m.pathPicker.generation)
}

func (m appModel) listPathCmd(path string, generation uint64) tea.Cmd {
	return func() tea.Msg {
		if m.deps.BrowsePath == nil {
			return pathListedMsg{err: context.Canceled, generation: generation}
		}
		dir, entries, err := m.deps.BrowsePath(context.Background(), path, m.pathPicker.mode)
		return pathListedMsg{dir: dir, entries: entries, err: err, generation: generation}
	}
}

func (m appModel) gotoPath(path string) (appModel, tea.Cmd) {
	m.pathPicker.generation++
	m.pathPicker.loading, m.pathPicker.err = true, ""
	m.pathPicker.entries, m.pathPicker.cursor, m.pathPicker.scroll = nil, 0, 0
	m.pathPicker.filter.input.SetValue("")
	m.pathPicker.filter.active = false
	return m, m.listPathCmd(path, m.pathPicker.generation)
}

func (m appModel) visiblePathEntries() []pathEntry {
	query := strings.ToLower(m.pathPicker.filter.query())
	if query == "" {
		return m.pathPicker.entries
	}
	entries := make([]pathEntry, 0, len(m.pathPicker.entries))
	for _, entry := range m.pathPicker.entries {
		if strings.Contains(strings.ToLower(entry.Name), query) {
			entries = append(entries, entry)
		}
	}
	return entries
}

func (m appModel) selectedPathEntry() (pathEntry, bool) {
	entries := m.visiblePathEntries()
	if m.pathPicker.cursor < 0 || m.pathPicker.cursor >= len(entries) {
		return pathEntry{}, false
	}
	return entries[m.pathPicker.cursor], true
}

func (m appModel) pathEntryPath(entry pathEntry) string {
	return filepath.Join(m.pathPicker.dir, entry.Name)
}

func (m appModel) pathPickerRows() int {
	_, height := m.popupDims(popupLarge)
	return max(1, height-7)
}

func (m appModel) followPathCursor() appModel {
	rows, count := m.pathPickerRows(), len(m.visiblePathEntries())
	if count <= rows {
		m.pathPicker.scroll = 0
		return m
	}
	maxScroll := max(0, count-rows+1)
	if m.pathPicker.cursor < m.pathPicker.scroll+1 {
		m.pathPicker.scroll = max(0, m.pathPicker.cursor-1)
	}
	if m.pathPicker.cursor > m.pathPicker.scroll+rows-2 {
		m.pathPicker.scroll = min(maxScroll, m.pathPicker.cursor-rows+2)
	}
	return m
}

func (m appModel) acceptPath(path string) appModel {
	absolute, err := filepath.Abs(path)
	if err != nil {
		m.pathPicker.err = "resolve selected path: " + err.Error()
		return m
	}
	m.settings.values[m.pathPicker.field] = absolute
	m.pathPicker = pathPickerState{}
	return m.focusSettingInput()
}

func (m appModel) handlePathPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.pathPicker.confirm != "" {
		switch msg.String() {
		case "enter":
			return m.acceptPath(m.pathPicker.confirm), nil
		case "esc":
			m.pathPicker.confirm = ""
		}
		return m, nil
	}
	if m.pathPicker.enterPath {
		switch msg.String() {
		case "enter":
			m.pathPicker.enterPath = false
			m.pathPicker.pathInput.Blur()
			return m.gotoPath(strings.Trim(strings.TrimSpace(m.pathPicker.pathInput.Value()), "\"'"))
		case "esc":
			m.pathPicker.enterPath = false
			m.pathPicker.pathInput.Blur()
			return m, nil
		case "ctrl+c":
			return m, tea.Quit
		}
		input, command := m.pathPicker.pathInput.Update(msg)
		m.pathPicker.pathInput = input
		return m, command
	}
	if m.pathPicker.filter.active {
		if msg.String() == "alt+enter" {
			if entry, ok := m.selectedPathEntry(); ok && !entry.parent {
				m.pathPicker.confirm, m.pathPicker.confirmDir = m.pathEntryPath(entry), entry.Dir
			}
			return m, nil
		}
		switch msg.String() {
		case "enter":
			m.pathPicker.filter.active = false
			m.pathPicker.filter.input.Blur()
			return m, nil
		case "esc":
			m.pathPicker.filter.active = false
			m.pathPicker.filter.input.Blur()
			m.pathPicker.filter.input.SetValue("")
			m.pathPicker.cursor, m.pathPicker.scroll = 0, 0
			return m, nil
		}
		input, command := m.pathPicker.filter.input.Update(msg)
		m.pathPicker.filter.input = input
		m.pathPicker.cursor, m.pathPicker.scroll = 0, 0
		return m, command
	}

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		m.pathPicker.cursor = clamp(m.pathPicker.cursor-1, len(m.visiblePathEntries()))
		return m.followPathCursor(), nil
	case "down", "j":
		m.pathPicker.cursor = clamp(m.pathPicker.cursor+1, len(m.visiblePathEntries()))
		return m.followPathCursor(), nil
	case "enter":
		entry, ok := m.selectedPathEntry()
		if !ok {
			return m, nil
		}
		if entry.parent {
			parent, ok := fsbrowse.Parent(m.pathPicker.dir)
			if ok {
				return m.gotoPath(parent)
			}
			return m, nil
		}
		if entry.Dir {
			return m.gotoPath(m.pathEntryPath(entry))
		}
	case " ", "alt+enter":
		if entry, ok := m.selectedPathEntry(); ok && !entry.parent {
			m.pathPicker.confirm, m.pathPicker.confirmDir = m.pathEntryPath(entry), entry.Dir
		}
	case "/":
		m.pathPicker.filter.active = true
		m.pathPicker.filter.input.Focus()
	case "p":
		m.pathPicker.enterPath = true
		m.pathPicker.pathInput.SetValue(m.pathPicker.dir)
		m.pathPicker.pathInput.Focus()
	case "esc":
		if m.pathPicker.filter.query() != "" {
			m.pathPicker.filter.input.SetValue("")
			m.pathPicker.cursor, m.pathPicker.scroll = 0, 0
			return m, nil
		}
		m.pathPicker = pathPickerState{}
		return m.focusSettingInput(), nil
	}
	return m, nil
}

func (m appModel) pathPickerPopup() string {
	inner := m.popupInnerW(popupLarge)
	body := []string{dimStyle.Render(ansi.TruncateLeft(m.pathPicker.dir, inner, "…")), ""}
	switch {
	case m.pathPicker.loading:
		body = append(body, "Reading directory…")
	case m.pathPicker.err != "":
		body = append(body, "Error: "+m.pathPicker.err)
	default:
		entries := m.visiblePathEntries()
		lines := make([]string, len(entries))
		for index, entry := range entries {
			name := entry.Name
			if entry.Dir && !entry.parent {
				name += string(filepath.Separator)
			}
			marker := "  "
			if index == m.pathPicker.cursor {
				marker = "▸ "
			}
			lines[index] = marker + ansi.TruncateLeft(name, inner-2, "…")
		}
		if len(lines) == 0 {
			lines = []string{"Nothing here."}
		}
		body = append(body, popupWindow(lines, m.pathPickerRows(), m.pathPicker.scroll)...)
	}
	footer := m.pathPickerFooter()
	return popupBox(m, popupOpts{Size: popupLarge, Fit: fitFixed, Title: "Choose path", Body: body, Footer: []string{footer}})
}

func (m appModel) pathPickerFooter() string {
	switch {
	case m.pathPicker.enterPath:
		return filterLabelStyle.Render("Path:") + " " + m.pathPicker.pathInput.View() + dimStyle.Render(" · enter open · esc cancel")
	case m.pathPicker.filter.active:
		return filterLabelStyle.Render("Filter:") + " " + m.pathPicker.filter.input.View() + dimStyle.Render(" · alt+enter select · esc clear")
	case m.pathPicker.filter.query() != "":
		return filterLabelStyle.Render("/"+m.pathPicker.filter.query()) + " " + filterCountStyle.Render(fmt.Sprintf("%d/%d", len(m.visiblePathEntries()), len(m.pathPicker.entries))) + dimStyle.Render(" · esc clear")
	default:
		return dimStyle.Render("enter open · space select · p path · / filter · esc back")
	}
}

func (m appModel) pathPickerConfirmPopup() string {
	question := "Use this file?"
	if m.pathPicker.confirmDir {
		question = "Use this directory?"
	}
	return popupBox(m, popupOpts{Size: popupSmall, Fit: fitAuto, Title: question, Body: []string{m.pathPicker.confirm}, Footer: []string{dimStyle.Render("enter use · esc back")}})
}
