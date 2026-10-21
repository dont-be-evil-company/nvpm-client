package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type fakeBackend struct {
	installed string
	pinned    string
	updated   string
}

func (f *fakeBackend) Load() ([]Package, error) {
	pinned := f.pinned == "npm:eslint"
	return []Package{
		{ID: "npm:eslint", Name: "eslint", Provider: "npm", Installed: true, Version: "8.0.0", Available: "9.0.0", Outdated: true, Pinned: pinned},
		{ID: "pypi:black", Name: "black", Provider: "pypi", Installed: false, Available: "24.0.0"},
	}, nil
}

func (f *fakeBackend) Details(id string) (string, error) { return "details " + id, nil }
func (f *fakeBackend) Diff(id string) (string, error)    { return "diff " + id, nil }
func (f *fakeBackend) Install(id string) error           { f.installed = id; return nil }
func (f *fakeBackend) Update(id string) error            { f.updated = id; return nil }
func (f *fakeBackend) UpdateAll() error                  { f.updated = "all"; return nil }
func (f *fakeBackend) Remove(string) error               { return nil }
func (f *fakeBackend) Pin(id string) error               { f.pinned = id; return nil }
func (f *fakeBackend) Unpin(string) error                { f.pinned = ""; return nil }
func (f *fakeBackend) Sync() error                       { return nil }

func TestModelFilterAndDetails(t *testing.T) {
	backend := &fakeBackend{}
	m := newModel(backend)
	loaded := m.Init()()
	next, _ := m.Update(loaded)
	m = next.(model)
	if len(m.rows()) != 1 || m.rows()[0].ID != "npm:eslint" {
		t.Fatalf("default filter rows=%v", m.rows())
	}

	next, _ = m.Update(key("f"))
	m = next.(model)
	next, _ = m.Update(key("f"))
	m = next.(model)
	next, _ = m.Update(key("f"))
	m = next.(model)
	if m.filter != FilterAll || len(m.rows()) != 2 {
		t.Fatalf("filter=%s rows=%d", m.filter, len(m.rows()))
	}

	next, cmd := m.Update(key("enter"))
	m = next.(model)
	if cmd == nil {
		t.Fatal("expected details command")
	}
	msg := cmd()
	next, _ = m.Update(msg)
	m = next.(model)
	if m.mode != modeDetail || !strings.Contains(m.page.View(), "details npm:eslint") {
		t.Fatalf("mode=%v view=%q", m.mode, m.page.View())
	}
}

func key(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}
