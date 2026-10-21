package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Backend performs package actions. Interactive commands run with the terminal
// released so existing prompts keep working.
type Backend interface {
	Load() ([]Package, error)
	Details(id string) (string, error)
	Diff(id string) (string, error)
	Install(id string) error
	Update(id string) error
	UpdateAll() error
	Remove(id string) error
	Pin(id string) error
	Unpin(id string) error
	Sync() error
}

type mode int

const (
	modeList mode = iota
	modeDetail
	modeDiff
	modeConfirm
	modeHelp
)

type model struct {
	backend  Backend
	packages []Package
	filter   Filter
	sort     Sort
	cursor   int
	query    string
	search   textinput.Model
	searchOn bool
	width    int
	height   int
	status   string
	page     viewport.Model
	mode     mode
	confirm  string
	action   func() error
	ready    bool
}

type loadedMsg struct {
	packages []Package
	err      error
}

type contentMsg struct {
	mode mode
	body string
	err  error
}

type doneMsg struct {
	status string
}

type runFunc func() error

func (f runFunc) Run() error { return f() }

func (f runFunc) SetStdin(io.Reader)  {}
func (f runFunc) SetStdout(io.Writer) {}
func (f runFunc) SetStderr(io.Writer) {}

// Run starts the package browser.
func Run(backend Backend) error {
	m := newModel(backend)
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func newModel(backend Backend) model {
	search := textinput.New()
	search.Placeholder = "filter packages"
	search.CharLimit = 128
	search.Prompt = "/ "
	return model{
		backend: backend,
		filter:  FilterInstalled,
		sort:    SortName,
		search:  search,
		width:   80,
		height:  24,
		status:  "loading packages…",
		page:    viewport.New(80, 16),
	}
}

func (m model) Init() tea.Cmd {
	return m.load
}

func (m model) load() tea.Msg {
	packages, err := m.backend.Load()
	return loadedMsg{packages: packages, err: err}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.page.Width = msg.Width
		m.page.Height = max(1, msg.Height-2)
		return m, nil
	case loadedMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
		} else {
			m.keepCursor(msg.packages)
			m.status = fmt.Sprintf("%d packages", len(m.rows()))
		}
		m.ready = true
		return m, nil
	case contentMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
		}
		m.mode = msg.mode
		m.page.SetContent(msg.body)
		m.page.GotoTop()
		return m, nil
	case doneMsg:
		m.status = msg.status
		m.mode = modeList
		return m, m.load
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.searchOn && m.mode == modeList {
		switch msg.String() {
		case "esc":
			m.searchOn = false
			m.search.Blur()
			return m, nil
		case "enter":
			m.searchOn = false
			m.query = m.search.Value()
			m.search.Blur()
			m.cursor = 0
			m.status = fmt.Sprintf("%d packages", len(m.rows()))
			return m, nil
		}
		var cmd tea.Cmd
		m.search, cmd = m.search.Update(msg)
		m.query = m.search.Value()
		m.cursor = 0
		return m, cmd
	}

	if m.mode == modeDetail || m.mode == modeDiff || m.mode == modeHelp {
		if msg.String() == "esc" || msg.String() == "q" {
			m.mode = modeList
			return m, nil
		}
		var cmd tea.Cmd
		m.page, cmd = m.page.Update(msg)
		return m, cmd
	}

	if m.mode == modeConfirm {
		switch msg.String() {
		case "y", "Y", "enter":
			fn := m.action
			label := m.confirm
			m.mode = modeList
			m.action = nil
			if fn == nil {
				return m, nil
			}
			return m.run(label, fn)
		case "n", "N", "esc", "q":
			m.mode = modeList
			m.action = nil
			m.status = "cancelled"
			return m, nil
		}
		return m, nil
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		rows := m.rows()
		if len(rows) > 0 {
			m.cursor = len(rows) - 1
		}
	case "f":
		m.filter = m.filter.Next()
		m.cursor = 0
		m.status = fmt.Sprintf("filter: %s (%d)", m.filter, len(m.rows()))
	case "o":
		m.sort = m.sort.Next()
		m.status = "sort: " + m.sort.String()
	case "/":
		m.searchOn = true
		m.search.Focus()
		return m, textinput.Blink
	case "esc":
		if m.query != "" {
			m.query = ""
			m.search.SetValue("")
			m.cursor = 0
			m.status = "cleared search"
		}
	case "enter":
		return m.openContent(modeDetail)
	case "d":
		return m.openContent(modeDiff)
	case "i":
		if pkg, ok := m.selected(); ok {
			id := pkg.ID
			return m.run("install "+id, func() error { return m.backend.Install(id) })
		}
	case "u":
		if pkg, ok := m.selected(); ok {
			if !pkg.Installed {
				m.status = pkg.ID + " is not installed"
				return m, nil
			}
			id := pkg.ID
			return m.run("update "+id, func() error { return m.backend.Update(id) })
		}
	case "U":
		return m.ask("update all unpinned packages", func() error { return m.backend.UpdateAll() })
	case "x":
		if pkg, ok := m.selected(); ok {
			if !pkg.Installed {
				m.status = pkg.ID + " is not installed"
				return m, nil
			}
			id := pkg.ID
			return m.ask("remove "+id, func() error { return m.backend.Remove(id) })
		}
	case "p":
		if pkg, ok := m.selected(); ok {
			if !pkg.Installed {
				m.status = "install " + pkg.ID + " before pinning, or use nvpm pin id@version"
				return m, nil
			}
			id := pkg.ID
			return m.run("pin "+id, func() error { return m.backend.Pin(id) })
		}
	case "P":
		if pkg, ok := m.selected(); ok {
			id := pkg.ID
			return m.run("unpin "+id, func() error { return m.backend.Unpin(id) })
		}
	case "s":
		return m.ask("sync packages from the lock file", func() error { return m.backend.Sync() })
	case "r":
		m.status = "refreshing…"
		return m, m.load
	case "?":
		m.mode = modeHelp
		m.page.SetContent(helpText)
		m.page.GotoTop()
	}
	return m, nil
}

func (m model) openContent(kind mode) (tea.Model, tea.Cmd) {
	pkg, ok := m.selected()
	if !ok {
		m.status = "no package selected"
		return m, nil
	}
	id := pkg.ID
	m.status = "loading " + id + "…"
	return m, func() tea.Msg {
		var (
			body string
			err  error
		)
		if kind == modeDiff {
			body, err = m.backend.Diff(id)
		} else {
			body, err = m.backend.Details(id)
		}
		return contentMsg{mode: kind, body: body, err: err}
	}
}

func (m model) ask(label string, fn func() error) (tea.Model, tea.Cmd) {
	m.mode = modeConfirm
	m.confirm = label
	m.action = fn
	return m, nil
}

func (m model) run(label string, fn func() error) (tea.Model, tea.Cmd) {
	m.status = label + "…"
	return m, tea.Exec(runFunc(fn), func(err error) tea.Msg {
		if err != nil {
			return doneMsg{status: label + " failed: " + err.Error()}
		}
		return doneMsg{status: label + " finished"}
	})
}

func (m *model) move(delta int) {
	rows := m.rows()
	if len(rows) == 0 {
		m.cursor = 0
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(rows) {
		m.cursor = len(rows) - 1
	}
}

func (m *model) keepCursor(packages []Package) {
	prev := ""
	rows := m.rows()
	if m.cursor >= 0 && m.cursor < len(rows) {
		prev = rows[m.cursor].ID
	}
	m.packages = packages
	rows = m.rows()
	m.cursor = 0
	if prev == "" {
		return
	}
	for i, row := range rows {
		if row.ID == prev {
			m.cursor = i
			return
		}
	}
}

func (m model) rows() []Package {
	return Visible(m.packages, m.filter, m.query, m.sort)
}

func (m model) selected() (Package, bool) {
	rows := m.rows()
	if m.cursor < 0 || m.cursor >= len(rows) {
		return Package{}, false
	}
	return rows[m.cursor], true
}

func (m model) View() string {
	if m.mode == modeConfirm {
		return lipgloss.JoinVertical(lipgloss.Left,
			titleStyle.Render("nvpm"),
			"",
			m.confirm+"?",
			"",
			"y confirm    n cancel",
		)
	}
	if m.mode == modeDetail || m.mode == modeDiff || m.mode == modeHelp {
		title := "details"
		if m.mode == modeDiff {
			title = "diff"
		}
		if m.mode == modeHelp {
			title = "help"
		}
		return lipgloss.JoinVertical(lipgloss.Left,
			titleStyle.Render("nvpm "+title)+"  "+mutedStyle.Render("esc back"),
			m.page.View(),
		)
	}

	header := titleStyle.Render("nvpm") + "  " + m.filter.String() + "  sort:" + m.sort.String()
	if m.searchOn {
		header += "\n" + m.search.View()
	} else if m.query != "" {
		header += "\n" + mutedStyle.Render("/ "+m.query)
	}
	list := m.renderList()
	footer := mutedStyle.Render("j/k move  enter details  d diff  i install  u update  U all  x remove  p pin  P unpin  f filter  o sort  / search  r refresh  s sync  ? help  q quit")
	status := m.status
	return lipgloss.JoinVertical(lipgloss.Left, header, list, footer, status)
}

func (m model) renderList() string {
	rows := m.rows()
	if !m.ready {
		return "\n" + m.status
	}
	if len(rows) == 0 {
		return "\nNo packages match this view."
	}
	height := max(1, m.height-6)
	start := 0
	if m.cursor >= height {
		start = m.cursor - height + 1
	}
	end := start + height
	if end > len(rows) {
		end = len(rows)
	}
	var b strings.Builder
	for i := start; i < end; i++ {
		pkg := rows[i]
		marker := "  "
		style := lipgloss.NewStyle()
		if i == m.cursor {
			marker = "> "
			style = selectedStyle
		}
		line := fmt.Sprintf("%s%-36s  %-18s  %s", marker, clip(pkg.ID, 36), clip(versionCell(pkg), 18), availableCell(pkg))
		b.WriteString(style.Render(clip(line, max(10, m.width))) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func versionCell(pkg Package) string {
	if !pkg.Installed {
		return "-"
	}
	text := pkg.Version
	if text == "" {
		text = "installed"
	}
	if pkg.Pinned {
		text += " pinned"
	}
	return text
}

func availableCell(pkg Package) string {
	if pkg.Outdated {
		avail := pkg.Available
		if avail == "" {
			avail = "update"
		}
		return avail + " outdated"
	}
	if !pkg.Installed {
		if pkg.Available == "" {
			return "not installed"
		}
		return pkg.Available
	}
	return "up to date"
}

func clip(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	if n == 1 {
		return s[:1]
	}
	return s[:n-1] + "…"
}

var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	mutedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

const helpText = `nvpm tui

j / down        next package
k / up          previous package
g / home        first package
G / end         last package
enter           package details
d               diff since the installed version
i               install the selected package
u               update the selected package
U               update every unpinned package
x               remove the selected package
p               pin the installed version
P               unpin
f               cycle filter: installed, outdated, pinned, all
o               cycle sort: name, provider, status
/               search
esc             clear search or close a panel
r               refresh the package list and registry
s               sync packages from nvpm-lock.json
?               this help
q               quit

Pinned packages stay on their locked version. Update and update-all skip them
until you unpin, including nvpm up -A --force.
Install, update, remove, pin, and sync use the same prompts as the CLI.
`
