package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/local/muxora/internal/config"
)

func TestTerminalBufferPreservesCRLFAndStripsANSI(t *testing.T) {
	var buffer terminalBuffer
	buffer.Write([]byte("\x1b[32muser@host\x1b[0m\r\n$ echo hola\r\nhola\r\n"))
	got := buffer.LastLines(10)
	if !strings.Contains(got, "user@host\n$ echo hola\nhola") {
		t.Fatalf("salida de terminal inesperada: %q", got)
	}
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("la salida conserva secuencias ANSI: %q", got)
	}
}

func TestTerminalBufferCarriageReturnRedrawsPrompt(t *testing.T) {
	var buffer terminalBuffer
	buffer.Write([]byte("%                                                                 "))
	buffer.Write([]byte("\r\x1b[2K"))
	buffer.Write([]byte("router-1# show status\r"))
	buffer.Write([]byte("\nSystem Name : ExampleLab\r\n"))
	got := buffer.LastLines(10)
	if !strings.Contains(got, "router-1# show status\nSystem Name : ExampleLab") {
		t.Fatalf("prompt redibujado incorrectamente: %q", got)
	}
	if strings.Contains(got, "%   ") {
		t.Fatalf("se conservó el prompt anterior: %q", got)
	}
}

func TestTerminalBufferPreservesPromptAcrossCRThenEnterCRLF(t *testing.T) {
	var buffer terminalBuffer
	// Dos lecturas PTY: el prompt termina en CR y Enter llega después como CRLF.
	buffer.Write([]byte("router-1# \r"))
	buffer.Write([]byte("\r\n"))
	if got := buffer.LastLines(10); got != "router-1# \n" {
		t.Fatalf("el Enter borró el hostname: %q", got)
	}
	buffer.Write([]byte("router-1# "))
	if got := buffer.LastLines(10); got != "router-1# \nrouter-1# " {
		t.Fatalf("el siguiente prompt quedó separado: %q", got)
	}
}

func TestVisibleLinesPreservesNormalSSHLineFlow(t *testing.T) {
	var buffer terminalBuffer
	buffer.Write([]byte("router-1#\r\nrouter-1# show status\r\n\r\nresult"))
	lines, offset := buffer.visibleLines(20)
	if offset != 0 {
		t.Fatalf("offset=%d; esperado 0", offset)
	}
	got := strings.Join(lines, "\n")
	if got != "router-1#\nrouter-1# show status\n\nresult" {
		t.Fatalf("filas visibles inesperadas: %q", got)
	}
}

func TestTerminalBufferPreservesBlankLinesInsideCommandOutput(t *testing.T) {
	var buffer terminalBuffer
	buffer.Write([]byte("heading\r\n\r\nbody\r\n"))
	got := buffer.LastLines(20)
	if got != "heading\n\nbody\n" {
		t.Fatalf("se alteró la salida interna: %q", got)
	}
}

func TestSessionTabsSwitchAndCloseIndependently(t *testing.T) {
	m := newModel(config.NewStore(t.TempDir()+"/config.yaml"), config.Default())
	m.sessions = []*sshSession{
		{id: 1, host: config.Host{ID: "one", Name: "One"}, status: "Conectada"},
		{id: 2, host: config.Host{ID: "two", Name: "Two"}, status: "Conectada"},
	}
	m.activeSession, m.mode = 0, modeSession
	m.switchSession(1)
	if m.active().id != 2 {
		t.Fatalf("sesión activa inesperada: %d", m.active().id)
	}
	m.closeActiveSession()
	if len(m.sessions) != 1 || m.active().id != 1 || m.mode != modeSession {
		t.Fatalf("cierre de pestaña inesperado: %#v", m.sessions)
	}
	m.closeActiveSession()
	if len(m.sessions) != 0 || m.mode != modeList {
		t.Fatal("la última pestaña debe volver al catálogo")
	}
}

func TestSessionTabMouseHit(t *testing.T) {
	m := model{width: 100, sessions: []*sshSession{{host: config.Host{Name: "One"}}, {host: config.Host{Name: "Two"}}}}
	leftW := max(25, m.width*34/100)
	if got := m.sessionTabAt(leftW + 3); got != 0 {
		t.Fatalf("primera pestaña=%d", got)
	}
	if got := m.sessionTabAt(leftW + 3 + len("1:One") + 3); got != 1 {
		t.Fatalf("segunda pestaña=%d", got)
	}
}

func TestSessionKeyEncoding(t *testing.T) {
	cases := []struct {
		key  tea.KeyMsg
		want string
	}{
		{tea.KeyMsg{Type: tea.KeyEnter}, "\r"},
		{tea.KeyMsg{Type: tea.KeyUp}, "\x1b[A"},
		{tea.KeyMsg{Type: tea.KeyCtrlC}, "\x03"},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ñ")}, "ñ"},
	}
	for _, tc := range cases {
		if got := string(keyBytes(tc.key)); got != tc.want {
			t.Fatalf("keyBytes(%q)=%q, esperado %q", tc.key.String(), got, tc.want)
		}
	}
}

func TestTerminalSelectionOnlyReturnsSSHText(t *testing.T) {
	var buffer terminalBuffer
	buffer.Write([]byte("ID  Address\r\n1   192.0.2.10\r\n2   192.0.2.20"))
	got := buffer.SelectedText(textPoint{line: 0, col: 4}, textPoint{line: 1, col: 9})
	if got != "Address\n1   192.0." {
		t.Fatalf("selección inesperada: %q", got)
	}
	if strings.Contains(got, "┃") {
		t.Fatalf("la selección incluyó bordes de la TUI: %q", got)
	}
}

func TestSessionTextPointIsRelativeToTerminalPane(t *testing.T) {
	var terminal terminalBuffer
	terminal.Write([]byte("alpha\r\nbeta"))
	m := model{width: 100, height: 24, activeSession: 0, sessions: []*sshSession{{terminal: terminal}}}
	leftW := max(25, m.width*34/100)
	point, inside := m.sessionTextPoint(leftW+4, 5)
	if !inside || point.line != 0 || point.col != 2 {
		t.Fatalf("coordenada inesperada: %#v inside=%v", point, inside)
	}
}
