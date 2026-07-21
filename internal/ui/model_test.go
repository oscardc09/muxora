package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/local/muxora/internal/config"
)

func TestFormCreatesAndPersistsHost(t *testing.T) {
	store := config.NewStore(filepath.Join(t.TempDir(), "config.yaml"))
	cfg := config.Default()
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	m := newModel(store, cfg)
	m.openForm(config.Host{})
	m.fields[0].value = "staging"
	m.fields[1].value = "Staging"
	m.fields[2].value = "staging.example.com"
	m.fields[3].value = "deploy"
	m.fields[4].value = "2222"
	m.fields[5].value = "web, pruebas"
	m.saveForm()

	loaded, err := store.LoadOrCreate()
	if err != nil {
		t.Fatal(err)
	}
	host, ok := loaded.FindHost("staging")
	if !ok || host.Port != 2222 || len(host.Groups) != 2 {
		t.Fatalf("host persistido inesperado: %#v", host)
	}
}

func TestFormDoesNotPersistInvalidPort(t *testing.T) {
	store := config.NewStore(filepath.Join(t.TempDir(), "config.yaml"))
	m := newModel(store, config.Default())
	m.openForm(config.Host{})
	m.fields[0].value = "bad"
	m.fields[1].value = "Bad"
	m.fields[2].value = "bad.example.com"
	m.fields[4].value = "not-a-port"
	m.saveForm()
	if len(m.cfg.Hosts) != 0 || m.mode != modeForm {
		t.Fatal("un formulario inválido no debe guardarse ni cerrarse")
	}
}

func TestMouseSelectsGroupAndHost(t *testing.T) {
	cfg := config.Default()
	cfg.Hosts = []config.Host{
		{ID: "one", Name: "One", Address: "one", Groups: []string{"lab"}},
		{ID: "two", Name: "Two", Address: "two", Groups: []string{"prod"}},
	}
	m := newModel(config.NewStore(filepath.Join(t.TempDir(), "config.yaml")), cfg)
	m.width, m.height = 80, 24

	updated, _ := m.updateMouse(tea.MouseEvent{X: 5, Y: 6, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(model)
	if m.selectedGroup != "lab" || len(m.visible) != 1 {
		t.Fatalf("clic de grupo inesperado: grupo=%q hosts=%d", m.selectedGroup, len(m.visible))
	}

	updated, _ = m.updateMouse(tea.MouseEvent{X: 5, Y: 14, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = updated.(model)
	if m.focus != focusHosts || m.cursor != 0 {
		t.Fatalf("clic de host inesperado: focus=%d cursor=%d", m.focus, m.cursor)
	}
}

func TestGroupFormCreatesStyledGroup(t *testing.T) {
	store := config.NewStore(filepath.Join(t.TempDir(), "config.yaml"))
	cfg := config.Default()
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	m := newModel(store, cfg)
	m.openGroupForm(config.Group{})
	m.fields[0].value = "network"
	m.fields[1].value = "Red"
	m.fields[2].value = "⬢"
	m.fields[3].value = "#22C55E"
	m.saveGroupForm()
	loaded, err := store.LoadOrCreate()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Groups) != 1 || loaded.Groups[0].Symbol != "⬢" || loaded.Groups[0].Color != "#22C55E" {
		t.Fatalf("grupo inesperado: %#v", loaded.Groups)
	}
}

func TestToggleRecordingIsIndependentPerSession(t *testing.T) {
	logDir := t.TempDir()
	cfg := config.Default()
	cfg.Settings.LogDirectory = logDir
	m := newModel(config.NewStore(filepath.Join(t.TempDir(), "config.yaml")), cfg)
	m.sessions = []*sshSession{
		{id: 1, host: config.Host{ID: "one", Name: "One", Address: "one.example"}},
		{id: 2, host: config.Host{ID: "two", Name: "Two", Address: "two.example"}},
	}
	m.activeSession = 0
	m.toggleRecording()
	if m.sessions[0].recorder == nil || m.sessions[1].recorder != nil {
		t.Fatal("el recording debe pertenecer sólo a la pestaña activa")
	}
	_, _ = m.sessions[0].recorder.Write([]byte("one# show version\r\n"))
	path := m.sessions[0].logPath
	m.toggleRecording()
	if m.sessions[0].recorder != nil {
		t.Fatal("el recorder debe cerrarse al alternar")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "one# show version") {
		t.Fatalf("log inesperado: %q", data)
	}
}

func TestRecordingFormSavesToSelectedDirectory(t *testing.T) {
	target := t.TempDir()
	m := newModel(config.NewStore(filepath.Join(t.TempDir(), "config.yaml")), config.Default())
	m.sessions = []*sshSession{{id: 1, host: config.Host{ID: "router", Name: "Router", Address: "192.0.2.10"}}}
	m.activeSession, m.mode = 0, modeSession
	m.openRecordingForm()
	if m.mode != modeRecordingForm {
		t.Fatal("el selector de recording no se abrió")
	}
	m.fields[0].value = target
	m.updateRecordingForm(tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeSession || m.sessions[0].recorder == nil {
		t.Fatal("el recording no inició al confirmar")
	}
	if !strings.HasPrefix(m.sessions[0].logPath, target+string(os.PathSeparator)) {
		t.Fatalf("ruta inesperada: %s", m.sessions[0].logPath)
	}
	m.stopRecording(m.sessions[0])
}

func TestRecordingFormCanBeCancelled(t *testing.T) {
	m := newModel(config.NewStore(filepath.Join(t.TempDir(), "config.yaml")), config.Default())
	m.sessions = []*sshSession{{id: 1, host: config.Host{ID: "one", Name: "One"}}}
	m.activeSession, m.mode = 0, modeSession
	m.openRecordingForm()
	m.updateRecordingForm(tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeSession || m.sessions[0].recorder != nil {
		t.Fatal("cancelar no debe iniciar recording")
	}
}
