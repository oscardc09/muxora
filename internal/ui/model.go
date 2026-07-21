package ui

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/creack/pty"
	"github.com/local/muxora/internal/config"
	"github.com/local/muxora/internal/recording"
	"github.com/local/muxora/internal/sshclient"
)

// mode determina qué componente posee el teclado. Esta máquina de estados evita
// que una tecla destinada a SSH active accidentalmente una acción del catálogo.
type mode int

const (
	modeList mode = iota
	modeSearch
	modeForm
	modeGroupForm
	modeDelete
	modeHelp
	modeSession
	modeRecordingForm
)

type field struct {
	label string
	value string
}

type focus int

const (
	focusGroups focus = iota
	focusHosts
)

// model es la única fuente de verdad de la aplicación Bubble Tea.
type model struct {
	store          *config.Store
	cfg            config.Config
	visible        []config.Host
	groups         []string
	groupCursor    int
	selectedGroup  string
	focus          focus
	cursor         int
	query          string
	mode           mode
	width, height  int
	status         string
	fields         []field
	fieldIndex     int
	editingID      string
	editingGroupID string
	deletingGroup  bool
	sessions       []*sshSession
	activeSession  int
	nextSessionID  int
}

// sshSession aísla proceso, PTY, salida y selección de cada pestaña.
type sshSession struct {
	id        int
	host      config.Host
	pty       *os.File
	cmd       *exec.Cmd
	terminal  terminalBuffer
	running   bool
	status    string
	recorder  *recording.Recorder
	logPath   string
	selecting bool
	selStart  textPoint
	selEnd    textPoint
}

type textPoint struct{ line, col int }

var (
	cyan       = lipgloss.Color("#38BDF8")
	blue       = lipgloss.Color("#2563EB")
	muted      = lipgloss.Color("#64748B")
	green      = lipgloss.Color("#22C55E")
	red        = lipgloss.Color("#F43F5E")
	panelStyle = lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(lipgloss.Color("#475569")).Padding(0, 1)
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(cyan)
	keyStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F8FAFC")).Background(blue).Padding(0, 1)
	dimStyle   = lipgloss.NewStyle().Foreground(muted)
	valueStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#E2E8F0"))
)

// Run inicia la TUI en alternate screen y activa eventos de mouse por celda.
func Run(store *config.Store, cfg config.Config) error {
	m := newModel(store, cfg)
	_, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}

func newModel(store *config.Store, cfg config.Config) model {
	m := model{store: store, cfg: cfg, status: "Listo · mouse activo", focus: focusHosts}
	m.filter()
	return m
}

func (m model) Init() tea.Cmd { return nil }

type sessionStartedMsg struct {
	id   int
	file *os.File
	cmd  *exec.Cmd
	host config.Host
	err  error
}
type sessionOutputMsg struct {
	id   int
	data []byte
	err  error
}
type clipboardMsg struct{ err error }

// Update enruta mensajes síncronos de UI y resultados asíncronos de PTY.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizeSessions()
	case sessionStartedMsg:
		session := m.sessionByID(msg.id)
		if session == nil {
			if msg.file != nil {
				_ = msg.file.Close()
			}
			if msg.cmd != nil && msg.cmd.Process != nil {
				_ = msg.cmd.Process.Kill()
				go func(cmd *exec.Cmd) { _ = cmd.Wait() }(msg.cmd)
			}
			break
		}
		if msg.err != nil {
			session.running, session.status = false, "Error: "+msg.err.Error()
			session.terminal.Write([]byte(session.status))
			if session.recorder != nil {
				_, _ = session.recorder.Write([]byte(session.status + "\n"))
				_ = session.recorder.Close()
				session.recorder = nil
			}
			m.status = session.status
			break
		}
		session.pty, session.cmd, session.running, session.status = msg.file, msg.cmd, true, "Conectada"
		m.mode, m.status = modeSession, "Sesión activa · Ctrl+] vuelve al catálogo"
		m.resizeSessions()
		return m, readSession(msg.id, msg.file)
	case sessionOutputMsg:
		session := m.sessionByID(msg.id)
		if session == nil {
			break
		}
		if len(msg.data) > 0 {
			session.terminal.Write(msg.data)
			if session.recorder != nil {
				if _, err := session.recorder.Write(msg.data); err != nil {
					m.status = "Error de recording: " + err.Error()
				}
			}
		}
		if msg.err != nil {
			m.finishSession(msg.id, msg.err)
			break
		}
		return m, readSession(msg.id, session.pty)
	case clipboardMsg:
		if msg.err != nil {
			m.status = "No se pudo copiar: " + msg.err.Error()
		} else {
			m.status = "Texto SSH copiado al portapapeles"
		}
	case tea.MouseMsg:
		return m.updateMouse(tea.MouseEvent(msg))
	case tea.KeyMsg:
		if m.mode == modeSession {
			return m, m.updateSession(msg)
		}
		if m.mode == modeRecordingForm {
			m.updateRecordingForm(msg)
			return m, nil
		}
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		switch m.mode {
		case modeSearch:
			m.updateSearch(msg)
		case modeForm:
			m.updateForm(msg)
		case modeGroupForm:
			m.updateGroupForm(msg)
		case modeDelete:
			m.updateDelete(msg)
		case modeHelp:
			m.mode = modeList
		default:
			return m.updateList(msg)
		}
	}
	return m, nil
}

func (m model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "?":
		m.mode = modeHelp
	case "s":
		if len(m.sessions) > 0 {
			m.mode = modeSession
			return m, nil
		}
	case "/":
		m.mode = modeSearch
	case "tab":
		if m.focus == focusHosts {
			m.focus = focusGroups
		} else {
			m.focus = focusHosts
		}
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "g", "home":
		if m.focus == focusGroups {
			m.groupCursor = 0
			m.chooseGroup()
		} else {
			m.cursor = 0
		}
	case "G", "end":
		if m.focus == focusGroups {
			m.groupCursor = max(0, len(m.groups)-1)
			m.chooseGroup()
		} else {
			m.cursor = max(0, len(m.visible)-1)
		}
	case "a":
		if m.focus == focusGroups {
			m.openGroupForm(config.Group{})
		} else {
			m.openForm(config.Host{})
		}
	case "e":
		if m.focus == focusGroups {
			if group, ok := m.selectedEditableGroup(); ok {
				m.editingGroupID = group.ID
				m.openGroupForm(group)
			}
		} else if h, ok := m.selected(); ok {
			m.editingID = h.ID
			m.openForm(h)
		}
	case "d":
		if m.focus == focusGroups {
			if _, ok := m.selectedEditableGroup(); ok {
				m.deletingGroup, m.mode = true, modeDelete
			}
		} else if _, ok := m.selected(); ok {
			m.deletingGroup, m.mode = false, modeDelete
		}
	case "f":
		m.toggleFavorite()
	case "r":
		if cfg, err := m.store.LoadOrCreate(); err != nil {
			m.status = "No se pudo recargar: " + err.Error()
		} else {
			m.cfg, m.status = cfg, "Configuración recargada"
			m.filter()
		}
	case "enter":
		if h, ok := m.selected(); ok {
			m.status = "Conectando a " + h.ID
			return m, m.addSession(h)
		}
	}
	return m, nil
}

func (m *model) updateSearch(msg tea.KeyMsg) {
	switch msg.String() {
	case "esc", "enter":
		m.mode = modeList
	case "ctrl+u":
		m.query = ""
	case "backspace":
		_, size := utf8.DecodeLastRuneInString(m.query)
		if size > 0 {
			m.query = m.query[:len(m.query)-size]
		}
	default:
		if len(msg.Runes) > 0 {
			m.query += string(msg.Runes)
		}
	}
	m.filter()
}

func (m *model) openForm(h config.Host) {
	port := ""
	if h.Port != 0 {
		port = strconv.Itoa(h.Port)
	}
	m.fields = []field{
		{"ID", h.ID}, {"Nombre", h.Name}, {"Dirección", h.Address},
		{"Usuario", h.User}, {"Puerto", port}, {"Grupos (coma)", strings.Join(h.Groups, ", ")},
	}
	m.fieldIndex = 0
	m.mode = modeForm
}

func (m *model) openGroupForm(group config.Group) {
	m.fields = []field{{"ID", group.ID}, {"Nombre", group.Name}, {"Símbolo", group.Symbol}, {"Color", group.Color}}
	m.fieldIndex = 0
	m.mode = modeGroupForm
}

func (m *model) updateForm(msg tea.KeyMsg) {
	switch msg.String() {
	case "esc":
		m.mode, m.editingID = modeList, ""
	case "tab", "down":
		m.fieldIndex = (m.fieldIndex + 1) % len(m.fields)
	case "shift+tab", "up":
		m.fieldIndex = (m.fieldIndex - 1 + len(m.fields)) % len(m.fields)
	case "ctrl+s", "enter":
		m.saveForm()
	case "ctrl+u":
		m.fields[m.fieldIndex].value = ""
	case "backspace":
		value := m.fields[m.fieldIndex].value
		_, size := utf8.DecodeLastRuneInString(value)
		if size > 0 {
			m.fields[m.fieldIndex].value = value[:len(value)-size]
		}
	default:
		if len(msg.Runes) > 0 {
			m.fields[m.fieldIndex].value += string(msg.Runes)
		}
	}
}

func (m *model) updateGroupForm(msg tea.KeyMsg) {
	switch msg.String() {
	case "esc":
		m.mode, m.editingGroupID = modeList, ""
	case "tab", "down":
		m.fieldIndex = (m.fieldIndex + 1) % len(m.fields)
	case "shift+tab", "up":
		m.fieldIndex = (m.fieldIndex - 1 + len(m.fields)) % len(m.fields)
	case "ctrl+s", "enter":
		m.saveGroupForm()
	case "ctrl+u":
		m.fields[m.fieldIndex].value = ""
	case "backspace":
		value := m.fields[m.fieldIndex].value
		_, size := utf8.DecodeLastRuneInString(value)
		if size > 0 {
			m.fields[m.fieldIndex].value = value[:len(value)-size]
		}
	default:
		if len(msg.Runes) > 0 {
			m.fields[m.fieldIndex].value += string(msg.Runes)
		}
	}
}

func (m *model) saveGroupForm() {
	group := config.Group{ID: strings.TrimSpace(m.fields[0].value), Name: strings.TrimSpace(m.fields[1].value), Symbol: strings.TrimSpace(m.fields[2].value), Color: strings.TrimSpace(m.fields[3].value)}
	if group.Symbol == "" {
		group.Symbol = "●"
	}
	if group.Color == "" {
		group.Color = "#38BDF8"
	}
	next := m.cfg
	next.Groups = append([]config.Group{}, m.cfg.Groups...)
	if m.editingGroupID == "" {
		next.Groups = append(next.Groups, group)
	} else {
		for i := range next.Groups {
			if next.Groups[i].ID == m.editingGroupID {
				next.Groups[i] = group
			}
		}
		if group.ID != m.editingGroupID {
			next.Hosts = append([]config.Host{}, m.cfg.Hosts...)
			for i := range next.Hosts {
				for j := range next.Hosts[i].Groups {
					if next.Hosts[i].Groups[j] == m.editingGroupID {
						next.Hosts[i].Groups[j] = group.ID
					}
				}
			}
		}
	}
	if err := m.store.Save(next); err != nil {
		m.status = "No se guardó: " + err.Error()
		return
	}
	m.cfg, m.mode, m.editingGroupID = next, modeList, ""
	m.selectedGroup, m.status = group.ID, "Grupo guardado"
	m.filter()
}

func (m *model) selectedEditableGroup() (config.Group, bool) {
	if m.selectedGroup == "Todos" || m.selectedGroup == "★ Favoritos" {
		m.status = "Este grupo del sistema no se puede editar"
		return config.Group{}, false
	}
	for _, group := range m.cfg.Groups {
		if group.ID == m.selectedGroup {
			return group, true
		}
	}
	// Convierte una etiqueta heredada en grupo editable al abrir el formulario.
	return config.Group{ID: m.selectedGroup, Name: m.selectedGroup, Symbol: "●", Color: "#38BDF8"}, true
}

func (m *model) saveForm() {
	port := 0
	var err error
	if strings.TrimSpace(m.fields[4].value) != "" {
		port, err = strconv.Atoi(strings.TrimSpace(m.fields[4].value))
		if err != nil {
			m.status = "El puerto debe ser un número"
			return
		}
	}
	groups := splitGroups(m.fields[5].value)
	h := config.Host{
		ID: strings.TrimSpace(m.fields[0].value), Name: strings.TrimSpace(m.fields[1].value),
		Address: strings.TrimSpace(m.fields[2].value), User: strings.TrimSpace(m.fields[3].value),
		Port: port, Groups: groups,
	}
	next := m.cfg
	if m.editingID == "" {
		next.Hosts = append(append([]config.Host{}, m.cfg.Hosts...), h)
	} else {
		next.Hosts = append([]config.Host{}, m.cfg.Hosts...)
		for i := range next.Hosts {
			if next.Hosts[i].ID == m.editingID {
				h.Favorite = next.Hosts[i].Favorite
				h.IdentityFile = next.Hosts[i].IdentityFile
				next.Hosts[i] = h
			}
		}
	}
	if err := m.store.Save(next); err != nil {
		m.status = "No se guardó: " + err.Error()
		return
	}
	m.cfg, m.mode, m.editingID = next, modeList, ""
	m.status = "Host guardado"
	m.filter()
}

func (m *model) move(delta int) {
	if m.focus == focusGroups {
		m.groupCursor = min(max(0, m.groupCursor+delta), max(0, len(m.groups)-1))
		m.chooseGroup()
		return
	}
	m.cursor = min(max(0, m.cursor+delta), max(0, len(m.visible)-1))
}

func (m *model) chooseGroup() {
	if m.groupCursor >= 0 && m.groupCursor < len(m.groups) {
		m.selectedGroup = m.groups[m.groupCursor]
		m.cursor = 0
		m.filter()
	}
}

func (m model) updateMouse(event tea.MouseEvent) (tea.Model, tea.Cmd) {
	if m.mode == modeRecordingForm {
		if event.Button == tea.MouseButtonLeft && event.Action == tea.MouseActionPress {
			switch event.Y {
			case 6:
				if directory, err := recording.DefaultDirectory(); err == nil {
					m.fields[0].value = directory
				}
			case 8:
				if home, err := os.UserHomeDir(); err == nil {
					m.fields[0].value = filepath.Join(home, "Documents", "Muxora Logs")
				}
			case 10:
				if home, err := os.UserHomeDir(); err == nil {
					m.fields[0].value = filepath.Join(home, "Desktop")
				}
			}
		}
		return m, nil
	}
	if m.mode == modeSession {
		if event.Button == tea.MouseButtonLeft && event.Action == tea.MouseActionPress && event.Y == 2 {
			if index := m.sessionTabAt(event.X); index >= 0 {
				m.activeSession = index
			}
			return m, nil
		}
		session := m.active()
		if session == nil {
			return m, nil
		}
		point, inside := m.sessionTextPoint(event.X, event.Y)
		if event.Button == tea.MouseButtonLeft && event.Action == tea.MouseActionPress && inside {
			session.selecting, session.selStart, session.selEnd = true, point, point
			m.status = "Seleccionando texto SSH…"
			return m, nil
		}
		if event.Action == tea.MouseActionMotion && session.selecting {
			session.selEnd = point
			return m, nil
		}
		if event.Action == tea.MouseActionRelease && session.selecting {
			session.selEnd, session.selecting = point, false
			selected := session.terminal.SelectedText(session.selStart, session.selEnd)
			if selected == "" {
				m.status = "Selección vacía"
				return m, nil
			}
			m.status = "Copiando selección…"
			return m, copyToClipboard(selected)
		}
		return m, nil
	}
	if m.mode != modeList && m.mode != modeSearch {
		return m, nil
	}
	if event.Button == tea.MouseButtonWheelUp {
		m.focus = focusHosts
		m.move(-1)
		return m, nil
	}
	if event.Button == tea.MouseButtonWheelDown {
		m.focus = focusHosts
		m.move(1)
		return m, nil
	}
	if event.Button != tea.MouseButtonLeft || event.Action != tea.MouseActionPress {
		return m, nil
	}

	w := max(60, m.width)
	leftW := max(25, w*34/100)
	if event.X >= leftW || event.Y < 1 {
		return m, nil
	}
	contentH := max(7, max(18, m.height)-7)
	groupH := min(9, max(4, contentH/2))
	groupFirstY := 4
	hostTop := 1 + groupH + 2
	hostFirstY := hostTop + 3
	if event.Y >= groupFirstY && event.Y < groupFirstY+len(m.groups) {
		m.focus = focusGroups
		m.groupCursor = event.Y - groupFirstY
		m.chooseGroup()
		return m, nil
	}
	if event.Y >= hostFirstY && event.Y < hostFirstY+len(m.visible) {
		clicked := event.Y - hostFirstY
		alreadySelected := m.focus == focusHosts && clicked == m.cursor
		m.focus, m.cursor = focusHosts, clicked
		if alreadySelected {
			h := m.visible[m.cursor]
			m.status = "Conectando a " + h.ID
			return m, m.addSession(h)
		}
	}
	return m, nil
}

func (m *model) updateDelete(msg tea.KeyMsg) {
	switch msg.String() {
	case "y", "Y":
		if m.deletingGroup {
			m.deleteSelectedGroup()
			return
		}
		h, ok := m.selected()
		if !ok {
			m.mode = modeList
			return
		}
		next := m.cfg
		next.Hosts = make([]config.Host, 0, len(m.cfg.Hosts)-1)
		for _, item := range m.cfg.Hosts {
			if item.ID != h.ID {
				next.Hosts = append(next.Hosts, item)
			}
		}
		if err := m.store.Save(next); err != nil {
			m.status = err.Error()
		} else {
			m.cfg, m.status = next, "Host eliminado"
		}
		m.mode = modeList
		m.filter()
	case "n", "N", "esc":
		m.mode, m.deletingGroup = modeList, false
	}
}

func (m *model) deleteSelectedGroup() {
	group, ok := m.selectedEditableGroup()
	if !ok {
		m.mode = modeList
		return
	}
	next := m.cfg
	next.Groups = make([]config.Group, 0, len(m.cfg.Groups))
	for _, item := range m.cfg.Groups {
		if item.ID != group.ID {
			next.Groups = append(next.Groups, item)
		}
	}
	next.Hosts = append([]config.Host{}, m.cfg.Hosts...)
	for i := range next.Hosts {
		filtered := next.Hosts[i].Groups[:0]
		for _, id := range next.Hosts[i].Groups {
			if id != group.ID {
				filtered = append(filtered, id)
			}
		}
		next.Hosts[i].Groups = filtered
	}
	if err := m.store.Save(next); err != nil {
		m.status = err.Error()
	} else {
		m.cfg, m.status = next, "Grupo eliminado"
	}
	m.selectedGroup, m.groupCursor, m.deletingGroup, m.mode = "Todos", 0, false, modeList
	m.filter()
}

func (m *model) toggleFavorite() {
	h, ok := m.selected()
	if !ok {
		return
	}
	for i := range m.cfg.Hosts {
		if m.cfg.Hosts[i].ID == h.ID {
			m.cfg.Hosts[i].Favorite = !m.cfg.Hosts[i].Favorite
		}
	}
	if err := m.store.Save(m.cfg); err != nil {
		m.status = err.Error()
	} else {
		m.status = "Favorito actualizado"
	}
	m.filter()
}

func (m *model) selected() (config.Host, bool) {
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return config.Host{}, false
	}
	return m.visible[m.cursor], true
}

func (m *model) filter() {
	groupSet := map[string]struct{}{}
	for _, group := range m.cfg.Groups {
		groupSet[group.ID] = struct{}{}
	}
	for _, h := range m.cfg.Hosts {
		for _, group := range h.Groups {
			if strings.TrimSpace(group) != "" {
				groupSet[group] = struct{}{}
			}
		}
	}
	m.groups = []string{"Todos", "★ Favoritos"}
	var namedGroups []string
	for group := range groupSet {
		namedGroups = append(namedGroups, group)
	}
	sort.Strings(namedGroups)
	m.groups = append(m.groups, namedGroups...)
	if m.selectedGroup == "" {
		m.selectedGroup = "Todos"
	}
	foundGroup := false
	for i, group := range m.groups {
		if group == m.selectedGroup {
			m.groupCursor, foundGroup = i, true
		}
	}
	if !foundGroup {
		m.selectedGroup, m.groupCursor = "Todos", 0
	}

	q := strings.ToLower(strings.TrimSpace(m.query))
	m.visible = m.visible[:0]
	for _, h := range m.cfg.Hosts {
		if m.selectedGroup == "★ Favoritos" && !h.Favorite {
			continue
		}
		if m.selectedGroup != "Todos" && m.selectedGroup != "★ Favoritos" && !contains(h.Groups, m.selectedGroup) {
			continue
		}
		haystack := strings.ToLower(h.ID + " " + h.Name + " " + h.Address + " " + strings.Join(h.Groups, " "))
		if q == "" || strings.Contains(haystack, q) {
			m.visible = append(m.visible, h)
		}
	}
	sort.SliceStable(m.visible, func(i, j int) bool { return m.visible[i].Favorite && !m.visible[j].Favorite })
	if m.cursor >= len(m.visible) {
		m.cursor = max(0, len(m.visible)-1)
	}
}

// View compone paneles responsivos sin realizar I/O ni modificar estado.
func (m model) View() string {
	w := max(60, m.width)
	h := max(18, m.height)
	leftW := max(25, w*34/100)
	rightW := max(30, w-leftW-1)
	contentH := max(7, h-7)
	groupH := min(9, max(4, contentH/2))
	hostH := max(3, contentH-groupH-2)
	header := m.header(w)
	groupBorder := lipgloss.Color("#475569")
	hostBorder := lipgloss.Color("#475569")
	if m.focus == focusGroups {
		groupBorder = cyan
	} else {
		hostBorder = cyan
	}
	groups := panelStyle.BorderForeground(groupBorder).Width(leftW - 4).Height(groupH).Render(m.groupPanel(groupH))
	hosts := panelStyle.BorderForeground(hostBorder).Width(leftW - 4).Height(hostH).Render(m.hostPanel(hostH))
	left := lipgloss.JoinVertical(lipgloss.Left, groups, hosts)
	rightContent := m.detailPanel()
	borderColor := lipgloss.Color("#334155")
	switch m.mode {
	case modeForm:
		rightContent, borderColor = m.formPanel(rightW-4), cyan
	case modeGroupForm:
		rightContent, borderColor = m.groupFormPanel(), cyan
	case modeDelete:
		rightContent, borderColor = m.deletePanel(), red
	case modeHelp:
		rightContent, borderColor = m.helpPanel(rightW-4), cyan
	case modeSession:
		rightContent, borderColor = m.sessionPanel(rightW-4, contentH), green
	case modeRecordingForm:
		rightContent, borderColor = m.recordingFormPanel(), red
	}
	right := panelStyle.BorderForeground(borderColor).Width(rightW - 4).Height(contentH).Render(rightContent)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	footer := m.footer(w)
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (m model) header(width int) string {
	brand := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0F172A")).Background(cyan).Padding(0, 2).Render("Muxora")
	title := titleStyle.Render("  SSH workspace")
	count := dimStyle.Render(fmt.Sprintf("%d hosts", len(m.visible)))
	space := max(1, width-lipgloss.Width(brand)-lipgloss.Width(title)-lipgloss.Width(count)-1)
	return brand + title + strings.Repeat(" ", space) + count
}

func (m model) hostPanel(height int) string {
	var lines []string
	title := titleStyle.Render("2 Hosts")
	if m.mode == modeSearch {
		title += "  " + lipgloss.NewStyle().Foreground(green).Render("/ "+m.query+"█")
	} else if m.query != "" {
		title += dimStyle.Render("  / " + m.query)
	}
	lines = append(lines, title, "")
	available := max(1, height-2)
	innerWidth := max(12, m.width*34/100-6)
	start := 0
	if m.cursor >= available {
		start = m.cursor - available + 1
	}
	for i := start; i < len(m.visible) && len(lines)-2 < available; i++ {
		h := m.visible[i]
		star := "  "
		if h.Favorite {
			star = "★ "
		}
		line := star + truncate(h.Name, max(4, innerWidth-4))
		if i == m.cursor {
			line = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0F172A")).Background(cyan).Width(innerWidth).Render("› " + line)
		} else {
			line = "  " + line
		}
		lines = append(lines, line)
	}
	if len(m.visible) == 0 {
		lines = append(lines, dimStyle.Render("  Sin hosts. Presiona a para agregar."))
	}
	return strings.Join(lines, "\n")
}

func (m model) groupPanel(height int) string {
	lines := []string{titleStyle.Render("1 Grupos"), ""}
	available := max(1, height-2)
	start := 0
	if m.groupCursor >= available {
		start = m.groupCursor - available + 1
	}
	innerWidth := max(12, m.width*34/100-6)
	for i := start; i < len(m.groups) && len(lines)-2 < available; i++ {
		groupID := m.groups[i]
		count := m.groupCount(groupID)
		name, symbol, color := m.groupPresentation(groupID)
		label := symbol + " " + name
		line := fmt.Sprintf("%-*s %d", max(4, innerWidth-6), truncate(label, max(4, innerWidth-6)), count)
		line = lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(line)
		if i == m.groupCursor {
			line = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0F172A")).Background(cyan).Width(innerWidth).Render("› " + line)
		} else {
			line = "  " + line
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m model) groupPresentation(id string) (name, symbol, color string) {
	if id == "Todos" {
		return "Todos", "◉", "#38BDF8"
	}
	if id == "★ Favoritos" {
		return "Favoritos", "★", "#FBBF24"
	}
	for _, group := range m.cfg.Groups {
		if group.ID == id {
			name, symbol, color = group.Name, group.Symbol, group.Color
			if symbol == "" {
				symbol = "●"
			}
			if color == "" {
				color = "#94A3B8"
			}
			return
		}
	}
	return id, "●", "#94A3B8"
}

func (m model) groupCount(group string) int {
	if group == "Todos" {
		return len(m.cfg.Hosts)
	}
	count := 0
	for _, h := range m.cfg.Hosts {
		if group == "★ Favoritos" && h.Favorite {
			count++
		}
		if group != "★ Favoritos" && contains(h.Groups, group) {
			count++
		}
	}
	return count
}

func (m model) detailPanel() string {
	h, ok := m.selected()
	if !ok {
		return titleStyle.Render("3 Detalle") + "\n\n" + dimStyle.Render("Selecciona o agrega un host")
	}
	port := h.Port
	if port == 0 {
		port = m.cfg.Defaults.Port
	}
	user := h.User
	if user == "" {
		user = m.cfg.Defaults.User
	}
	rows := [][2]string{{"Nombre", h.Name}, {"ID", h.ID}, {"Dirección", h.Address}, {"Usuario", fallback(user, "—")}, {"Puerto", strconv.Itoa(port)}, {"Autenticación", "Automática · OpenSSH/agent"}, {"Grupos", fallback(strings.Join(h.Groups, ", "), "—")}, {"Favorito", map[bool]string{true: "Sí", false: "No"}[h.Favorite]}}
	var b strings.Builder
	b.WriteString(titleStyle.Render("3 Detalle") + "\n\n")
	if len(m.sessions) > 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(green).Render(fmt.Sprintf("▣ %d sesiones abiertas · s para volver", len(m.sessions))) + "\n\n")
	}
	for _, row := range rows {
		b.WriteString(dimStyle.Width(15).Render(row[0]) + valueStyle.Render(row[1]) + "\n")
	}
	b.WriteString("\n" + lipgloss.NewStyle().Foreground(green).Render("● listo para conectar"))
	return b.String()
}

func (m model) footer(width int) string {
	if m.mode == modeRecordingForm {
		return "\n" + keyStyle.Render("enter") + " iniciar REC  " + keyStyle.Render("ctrl+d") + " predeterminado  " + keyStyle.Render("ctrl+o") + " Documents  " + keyStyle.Render("esc") + " cancelar\n" + lipgloss.NewStyle().Foreground(red).Render("● Selecciona el destino del recording")
	}
	if m.mode == modeSession {
		return "\n" + keyStyle.Render("ctrl+←/→") + " pestaña  " + keyStyle.Render("ctrl+r") + " REC  " + keyStyle.Render("ctrl+w") + " cerrar  " + keyStyle.Render("ctrl+]") + " catálogo\n" + lipgloss.NewStyle().Foreground(green).Render("● "+m.status)
	}
	entity := "host"
	if m.focus == focusGroups {
		entity = "grupo"
	}
	keys := []string{keyStyle.Render("enter") + " conectar", keyStyle.Render("a") + " agregar", keyStyle.Render("e") + " editar", keyStyle.Render("d") + " borrar", keyStyle.Render("/") + " buscar", keyStyle.Render("?") + " ayuda", keyStyle.Render("q") + " salir"}
	if width < 100 {
		keys = []string{keyStyle.Render("tab") + " panel", keyStyle.Render("a") + " nuevo " + entity, keyStyle.Render("s") + " sesiones", keyStyle.Render("e") + " editar", keyStyle.Render("d") + " borrar", keyStyle.Render("?")}
	}
	bar := strings.Join(keys, "  ")
	status := lipgloss.NewStyle().Foreground(lipgloss.Color("#CBD5E1")).Render("● " + truncate(m.status, max(20, width-4)))
	return "\n" + bar + "\n" + status
}

func (m model) sessionPanel(width, height int) string {
	session := m.active()
	if session == nil {
		return titleStyle.Render("3 Terminal") + "\n\n" + dimStyle.Render("No hay sesiones abiertas")
	}
	title := m.sessionTabs(width)
	indicator := "○"
	if session.running {
		indicator = "●"
	}
	rec := ""
	if session.recorder != nil {
		rec = " · " + lipgloss.NewStyle().Bold(true).Foreground(red).Render("● REC")
	}
	remote := dimStyle.Render(indicator+" "+session.host.ID+" · "+session.host.Address+" · "+session.status) + rec
	rows := max(3, height-4)
	output := session.terminal.Render(rows, max(20, width-2), session.selStart, session.selEnd)
	if output == "" {
		output = dimStyle.Render("Iniciando OpenSSH…")
	}
	return title + "\n" + remote + "\n\n" + lipgloss.NewStyle().Width(max(20, width-2)).Render(output)
}

func (m model) sessionTabs(width int) string {
	var rendered string
	for i, session := range m.sessions {
		label := fmt.Sprintf("%d:%s", i+1, truncate(session.host.Name, 12))
		style := dimStyle
		if i == m.activeSession {
			style = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0F172A")).Background(green).Padding(0, 1)
		} else {
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("#CBD5E1")).Background(lipgloss.Color("#1E293B")).Padding(0, 1)
		}
		tab := style.Render(label)
		candidate := tab
		if rendered != "" {
			candidate = rendered + " " + tab
		}
		if lipgloss.Width(candidate) > width {
			break
		}
		rendered = candidate
	}
	return rendered
}

func (m model) sessionTabAt(x int) int {
	w := max(60, m.width)
	leftW := max(25, w*34/100)
	position := leftW + 2
	for i, session := range m.sessions {
		labelWidth := lipgloss.Width(fmt.Sprintf("%d:%s", i+1, truncate(session.host.Name, 12))) + 2
		if x >= position && x < position+labelWidth {
			return i
		}
		position += labelWidth + 1
	}
	return -1
}

func (m model) sessionTextPoint(x, y int) (textPoint, bool) {
	session := m.active()
	if session == nil {
		return textPoint{}, false
	}
	w, h := max(60, m.width), max(18, m.height)
	leftW := max(25, w*34/100)
	outputX, outputY := leftW+2, 5
	rows := max(3, max(7, h-7)-4)
	lines, offset := session.terminal.visibleLines(rows)
	row := min(max(0, y-outputY), max(0, len(lines)-1))
	col := max(0, x-outputX)
	if len(lines) > 0 {
		col = min(col, len([]rune(lines[row])))
	}
	inside := x >= outputX && x < w-2 && y >= outputY && y < outputY+rows
	return textPoint{line: offset + row, col: col}, inside
}

func (m model) groupFormPanel() string {
	var b strings.Builder
	title := "Agregar grupo"
	if m.editingGroupID != "" {
		title = "Editar grupo: " + m.editingGroupID
	}
	b.WriteString(titleStyle.Render(title) + "\n\n")
	for i, f := range m.fields {
		cursor, style := "  ", valueStyle
		if i == m.fieldIndex {
			cursor, style = "› ", lipgloss.NewStyle().Bold(true).Foreground(cyan)
		}
		b.WriteString(cursor + dimStyle.Width(16).Render(f.label) + style.Render(f.value))
		if i == m.fieldIndex {
			b.WriteString(style.Render("█"))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n" + dimStyle.Render("Color: formato #RRGGBB · Símbolo: cualquier Unicode") + "\n\n")
	b.WriteString(keyStyle.Render("tab") + " campo  " + keyStyle.Render("enter/ctrl+s") + " guardar  " + keyStyle.Render("esc") + " cancelar")
	return b.String()
}

func (m model) recordingFormPanel() string {
	value := ""
	if len(m.fields) > 0 {
		value = m.fields[0].value
	}
	var b strings.Builder
	b.WriteString(titleStyle.Foreground(red).Render("● Nuevo recording") + "\n\n")
	b.WriteString(dimStyle.Render("Selecciona la carpeta donde se guardará esta sesión.") + "\n\n")
	b.WriteString(keyStyle.Render("Ctrl+D") + "  Muxora logs (predeterminado)\n\n")
	b.WriteString(keyStyle.Render("Ctrl+O") + "  Documents/Muxora Logs\n\n")
	b.WriteString(keyStyle.Render("Ctrl+T") + "  Desktop\n\n")
	b.WriteString(dimStyle.Render("Ruta personalizada") + "\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(cyan).Render("› "+value+"█") + "\n\n")
	b.WriteString(dimStyle.Render("La carpeta se creará si no existe. El nombre del archivo se genera con fecha, hora y host.") + "\n\n")
	b.WriteString(keyStyle.Render("Enter") + " iniciar  " + keyStyle.Render("Esc") + " cancelar")
	return b.String()
}

func (m model) formPanel(width int) string {
	var b strings.Builder
	title := "Agregar host"
	if m.editingID != "" {
		title = "Editar host: " + m.editingID
	}
	b.WriteString(titleStyle.Render(title) + "\n\n")
	for i, f := range m.fields {
		cursor := "  "
		style := valueStyle
		if i == m.fieldIndex {
			cursor = "› "
			style = lipgloss.NewStyle().Bold(true).Foreground(cyan)
		}
		b.WriteString(cursor + dimStyle.Width(16).Render(f.label) + style.Render(f.value))
		if i == m.fieldIndex {
			b.WriteString(style.Render("█"))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n" + keyStyle.Render("tab") + " campo  " + keyStyle.Render("enter/ctrl+s") + " guardar  " + keyStyle.Render("esc") + " cancelar")
	return b.String()
}

func (m model) deletePanel() string {
	if m.deletingGroup {
		group, _ := m.selectedEditableGroup()
		return titleStyle.Foreground(red).Render("Eliminar grupo") + "\n\n¿Eliminar " + valueStyle.Render(group.Name) + " y quitarlo de todos los hosts?\n\n" + keyStyle.Render("y") + " confirmar  " + keyStyle.Render("n/esc") + " cancelar"
	}
	h, _ := m.selected()
	text := titleStyle.Foreground(red).Render("Eliminar host") + "\n\n¿Eliminar " + valueStyle.Render(h.Name) + " (" + h.ID + ")?\n\n" + keyStyle.Render("y") + " confirmar  " + keyStyle.Render("n/esc") + " cancelar"
	return text
}

func (m model) helpPanel(width int) string {
	text := titleStyle.Render("Atajos") + "\n\n" +
		"tab         cambiar panel        clic  seleccionar/abrir\n" +
		"j/k, ↑/↓   mover selección      g/G   inicio/final\n" +
		"enter       conectar por SSH     /     buscar\n" +
		"s           volver a sesiones    ctrl+←/→ cambiar pestaña\n" +
		"a           agregar host         e     editar host\n" +
		"d           eliminar host        f     favorito\n" +
		"r           recargar YAML        q     salir\n\n" + dimStyle.Render("Presiona cualquier tecla para cerrar")
	return text
}

func splitGroups(value string) []string {
	var groups []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			groups = append(groups, item)
		}
	}
	return groups
}

func truncate(value string, width int) string {
	r := []rune(value)
	if len(r) <= width {
		return value
	}
	if width <= 1 {
		return string(r[:width])
	}
	return string(r[:width-1]) + "…"
}

func fallback(value, replacement string) string {
	if value == "" {
		return replacement
	}
	return value
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func sshCommand(host config.Host, defaults config.Defaults) *exec.Cmd {
	return exec.Command("ssh", sshclient.Args(host, defaults)...)
}

func startSession(id int, host config.Host, defaults config.Defaults, size *pty.Winsize) tea.Cmd {
	return func() tea.Msg {
		cmd := sshCommand(host, defaults)
		cmd.Env = append(os.Environ(), "TERM=xterm-256color")
		file, err := pty.StartWithSize(cmd, size)
		return sessionStartedMsg{id: id, file: file, cmd: cmd, host: host, err: err}
	}
}

func readSession(id int, file *os.File) tea.Cmd {
	return func() tea.Msg {
		if file == nil {
			return sessionOutputMsg{id: id, err: io.EOF}
		}
		buffer := make([]byte, 4096)
		n, err := file.Read(buffer)
		return sessionOutputMsg{id: id, data: append([]byte(nil), buffer[:n]...), err: err}
	}
}

func (m *model) sessionSize() *pty.Winsize {
	w := max(40, m.width)
	h := max(18, m.height)
	leftW := max(25, w*34/100)
	cols := max(20, w-leftW-7)
	rows := max(5, h-11)
	return &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)}
}

func (m *model) resizeSessions() {
	for _, session := range m.sessions {
		if session.pty != nil && session.running {
			_ = pty.Setsize(session.pty, m.sessionSize())
		}
	}
}

func (m *model) updateSession(key tea.KeyMsg) tea.Cmd {
	switch key.String() {
	case "ctrl+]":
		m.mode, m.status = modeList, "Sesiones en segundo plano: "+strconv.Itoa(len(m.sessions))
		return nil
	case "ctrl+w":
		m.closeActiveSession()
		return nil
	case "ctrl+left", "ctrl+p":
		m.switchSession(-1)
		return nil
	case "ctrl+right", "ctrl+n":
		m.switchSession(1)
		return nil
	case "ctrl+r":
		session := m.active()
		if session != nil && session.recorder != nil {
			m.stopRecording(session)
		} else {
			m.openRecordingForm()
		}
		return nil
	}
	session := m.active()
	if session == nil || session.pty == nil || !session.running {
		return nil
	}
	data := keyBytes(key)
	if len(data) > 0 {
		_, _ = session.pty.Write(data)
	}
	return nil
}

func (m *model) addSession(host config.Host) tea.Cmd {
	m.nextSessionID++
	session := &sshSession{id: m.nextSessionID, host: host, running: false, status: "Conectando…"}
	if m.cfg.Settings.RecordSessions {
		if err := m.startRecording(session); err != nil {
			m.status = "No se pudo iniciar recording: " + err.Error()
		}
	}
	m.sessions = append(m.sessions, session)
	m.activeSession, m.mode = len(m.sessions)-1, modeSession
	return startSession(session.id, host, m.cfg.Defaults, m.sessionSize())
}

func (m *model) openRecordingForm() {
	session := m.active()
	if session == nil {
		return
	}
	directory, err := recording.ResolveDirectory(m.cfg.Settings.LogDirectory)
	if err != nil {
		m.status = "No se pudo resolver logs: " + err.Error()
		return
	}
	m.fields = []field{{label: "Directorio", value: directory}}
	m.fieldIndex, m.mode = 0, modeRecordingForm
}

func (m *model) updateRecordingForm(key tea.KeyMsg) {
	if len(m.fields) == 0 {
		m.mode = modeSession
		return
	}
	switch key.String() {
	case "esc":
		m.mode = modeSession
	case "enter", "ctrl+s":
		session := m.active()
		if session == nil {
			m.mode = modeList
			return
		}
		directory := strings.TrimSpace(m.fields[0].value)
		if directory == "" {
			m.status = "Selecciona un directorio"
			return
		}
		if err := m.startRecordingAt(session, directory); err != nil {
			m.status = "No se pudo iniciar recording: " + err.Error()
			return
		}
		m.mode, m.status = modeSession, "Recording activo: "+session.logPath
	case "ctrl+d":
		if directory, err := recording.DefaultDirectory(); err == nil {
			m.fields[0].value = directory
		} else {
			m.status = err.Error()
		}
	case "ctrl+o":
		if home, err := os.UserHomeDir(); err == nil {
			m.fields[0].value = filepath.Join(home, "Documents", "Muxora Logs")
		} else {
			m.status = err.Error()
		}
	case "ctrl+t":
		if home, err := os.UserHomeDir(); err == nil {
			m.fields[0].value = filepath.Join(home, "Desktop")
		} else {
			m.status = err.Error()
		}
	case "ctrl+u":
		m.fields[0].value = ""
	case "backspace":
		value := m.fields[0].value
		_, size := utf8.DecodeLastRuneInString(value)
		if size > 0 {
			m.fields[0].value = value[:len(value)-size]
		}
	default:
		if len(key.Runes) > 0 {
			m.fields[0].value += string(key.Runes)
		}
	}
}

func (m *model) toggleRecording() {
	session := m.active()
	if session == nil {
		return
	}
	if session.recorder != nil {
		m.stopRecording(session)
		return
	}
	if err := m.startRecording(session); err != nil {
		m.status = "No se pudo iniciar recording: " + err.Error()
		return
	}
	m.status = "Recording activo: " + session.logPath
}

func (m *model) stopRecording(session *sshSession) {
	path := session.logPath
	if err := session.recorder.Close(); err != nil {
		m.status = "No se pudo cerrar el log: " + err.Error()
	} else {
		m.status = "Recording guardado: " + path
	}
	session.recorder = nil
}

func (m *model) startRecording(session *sshSession) error {
	return m.startRecordingAt(session, m.cfg.Settings.LogDirectory)
}

func (m *model) startRecordingAt(session *sshSession, directory string) error {
	user := session.host.User
	if user == "" {
		user = m.cfg.Defaults.User
	}
	recorder, err := recording.Start(directory, recording.Session{HostID: session.host.ID, Name: session.host.Name, Address: session.host.Address, User: user})
	if err != nil {
		return err
	}
	session.recorder, session.logPath = recorder, recorder.Path()
	return nil
}

func (m *model) active() *sshSession {
	if m.activeSession < 0 || m.activeSession >= len(m.sessions) {
		return nil
	}
	return m.sessions[m.activeSession]
}

func (m *model) sessionByID(id int) *sshSession {
	for _, session := range m.sessions {
		if session.id == id {
			return session
		}
	}
	return nil
}

func (m *model) switchSession(delta int) {
	if len(m.sessions) == 0 {
		m.mode = modeList
		return
	}
	m.activeSession = (m.activeSession + delta + len(m.sessions)) % len(m.sessions)
	m.status = "Sesión: " + m.sessions[m.activeSession].host.Name
}

func (m *model) finishSession(id int, err error) {
	session := m.sessionByID(id)
	if session == nil {
		return
	}
	if session.pty != nil {
		_ = session.pty.Close()
		session.pty = nil
	}
	if session.cmd != nil {
		go func(cmd *exec.Cmd) { _ = cmd.Wait() }(session.cmd)
	}
	if session.recorder != nil {
		_ = session.recorder.Close()
		session.recorder = nil
	}
	session.running = false
	if err != nil && err != io.EOF && !strings.Contains(err.Error(), "file already closed") {
		session.status = "Terminada: " + err.Error()
	} else {
		session.status = "Finalizada"
	}
	m.status = session.host.Name + " · " + session.status
}

func (m *model) closeActiveSession() {
	session := m.active()
	if session == nil {
		m.mode = modeList
		return
	}
	if session.pty != nil {
		_ = session.pty.Close()
	}
	if session.cmd != nil && session.cmd.ProcessState == nil && session.cmd.Process != nil {
		_ = session.cmd.Process.Kill()
		go func(cmd *exec.Cmd) { _ = cmd.Wait() }(session.cmd)
	}
	if session.recorder != nil {
		_ = session.recorder.Close()
		session.recorder = nil
	}
	m.sessions = append(m.sessions[:m.activeSession], m.sessions[m.activeSession+1:]...)
	if len(m.sessions) == 0 {
		m.activeSession, m.mode, m.status = 0, modeList, "Todas las sesiones fueron cerradas"
		return
	}
	if m.activeSession >= len(m.sessions) {
		m.activeSession = len(m.sessions) - 1
	}
	m.status = "Sesión: " + m.sessions[m.activeSession].host.Name
}

func keyBytes(key tea.KeyMsg) []byte {
	switch key.String() {
	case "enter":
		return []byte{'\r'}
	case "tab":
		return []byte{'\t'}
	case "backspace":
		return []byte{0x7f}
	case "esc":
		return []byte{0x1b}
	case "up":
		return []byte("\x1b[A")
	case "down":
		return []byte("\x1b[B")
	case "right":
		return []byte("\x1b[C")
	case "left":
		return []byte("\x1b[D")
	case "home":
		return []byte("\x1b[H")
	case "end":
		return []byte("\x1b[F")
	case "delete":
		return []byte("\x1b[3~")
	case "ctrl+c":
		return []byte{0x03}
	case "ctrl+d":
		return []byte{0x04}
	case "ctrl+z":
		return []byte{0x1a}
	case "ctrl+l":
		return []byte{0x0c}
	}
	if len(key.Runes) > 0 {
		return []byte(string(key.Runes))
	}
	return nil
}

func copyToClipboard(value string) tea.Cmd {
	return func() tea.Msg {
		path, err := exec.LookPath("pbcopy")
		if err != nil {
			return clipboardMsg{err: fmt.Errorf("pbcopy no está disponible: %w", err)}
		}
		cmd := exec.Command(path)
		cmd.Stdin = strings.NewReader(value)
		return clipboardMsg{err: cmd.Run()}
	}
}

// terminalBuffer conserva una representación lineal y saneada de la salida.
// No pretende sustituir todavía a un emulador VT con matriz de celdas.
type terminalBuffer struct {
	text      []rune
	lineStart int
	escape    int
	pendingCR bool
}

func (b *terminalBuffer) Reset() { b.text, b.lineStart, b.escape, b.pendingCR = nil, 0, 0, false }

func (b *terminalBuffer) Write(data []byte) {
	for _, r := range []rune(string(data)) {
		if b.pendingCR {
			// Algunos equipos dejan el prompt terminado en CR y, al recibir Enter,
			// comienzan el siguiente bloque con CRLF. CR+CR+LF equivale a un solo
			// salto y no debe borrar la línea que contiene el hostname.
			if r == '\r' {
				continue
			}
			b.pendingCR = false
			if r == '\n' {
				b.text = append(b.text, '\n')
				b.lineStart = len(b.text)
				continue
			}
			// CR sin LF: el dispositivo vuelve a la columna cero y redibuja la línea.
			b.text = b.text[:b.lineStart]
		}
		switch b.escape {
		case 1:
			if r == '[' {
				b.escape = 2
			} else if r == ']' {
				b.escape = 3
			} else {
				b.escape = 0
			}
			continue
		case 2:
			if r >= '@' && r <= '~' {
				b.escape = 0
			}
			continue
		case 3:
			if r == '\a' {
				b.escape = 0
			} else if r == '\x1b' {
				b.escape = 4
			}
			continue
		case 4:
			if r == '\\' {
				b.escape = 0
			} else {
				b.escape = 3
			}
			continue
		}
		switch r {
		case '\x1b':
			b.escape = 1
		case '\r':
			b.pendingCR = true
		case '\n':
			b.text = append(b.text, '\n')
			b.lineStart = len(b.text)
		case '\b':
			if len(b.text) > b.lineStart {
				b.text = b.text[:len(b.text)-1]
			}
		default:
			if r == '\t' {
				b.text = append(b.text, ' ', ' ', ' ', ' ')
			} else if r >= 0x20 {
				b.text = append(b.text, r)
			}
		}
	}
	if len(b.text) > 200000 {
		b.text = append([]rune(nil), b.text[len(b.text)-100000:]...)
		b.lineStart = 0
	}
}

func (b terminalBuffer) LastLines(count int) string {
	lines := strings.Split(string(b.text), "\n")
	if len(lines) > count {
		lines = lines[len(lines)-count:]
	}
	return strings.Join(lines, "\n")
}

func (b terminalBuffer) visibleLines(count int) ([]string, int) {
	lines := strings.Split(string(b.text), "\n")
	offset := 0
	if len(lines) > count {
		offset, lines = len(lines)-count, lines[len(lines)-count:]
	}
	return lines, offset
}

func normalizeSelection(a, z textPoint) (textPoint, textPoint) {
	if a.line > z.line || (a.line == z.line && a.col > z.col) {
		return z, a
	}
	return a, z
}

func (b terminalBuffer) SelectedText(a, z textPoint) string {
	a, z = normalizeSelection(a, z)
	lines := strings.Split(string(b.text), "\n")
	if len(lines) == 0 || a.line < 0 || a.line >= len(lines) {
		return ""
	}
	z.line = min(z.line, len(lines)-1)
	var selected []string
	for lineIndex := a.line; lineIndex <= z.line; lineIndex++ {
		runes := []rune(lines[lineIndex])
		start, end := 0, len(runes)
		if lineIndex == a.line {
			start = min(max(0, a.col), len(runes))
		}
		if lineIndex == z.line {
			end = min(max(0, z.col+1), len(runes))
		}
		if end < start {
			end = start
		}
		selected = append(selected, string(runes[start:end]))
	}
	return strings.Join(selected, "\n")
}

func (b terminalBuffer) Render(count, width int, a, z textPoint) string {
	lines, offset := b.visibleLines(count)
	a, z = normalizeSelection(a, z)
	selectionExists := a != z
	selectionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#0F172A")).Background(lipgloss.Color("#FBBF24"))
	for i, line := range lines {
		absolute := offset + i
		runes := []rune(line)
		if len(runes) > width {
			runes = runes[:width]
		}
		if !selectionExists || absolute < a.line || absolute > z.line {
			lines[i] = string(runes)
			continue
		}
		start, end := 0, len(runes)
		if absolute == a.line {
			start = min(max(0, a.col), len(runes))
		}
		if absolute == z.line {
			end = min(max(0, z.col+1), len(runes))
		}
		if end < start {
			end = start
		}
		lines[i] = string(runes[:start]) + selectionStyle.Render(string(runes[start:end])) + string(runes[end:])
	}
	return strings.Join(lines, "\n")
}
