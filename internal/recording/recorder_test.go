package recording

import (
	"os"
	"strings"
	"testing"
)

func TestRecorderCreatesPrivatePlainTextTranscript(t *testing.T) {
	recorder, err := Start(t.TempDir(), Session{HostID: "router-1", Name: "Router de prueba", Address: "192.0.2.10", User: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	path := recorder.Path()
	if _, err := recorder.Write([]byte("\r\n\r\n\x1b[2J\x1b[32mAPIC#\x1b[0m show controller\r\nFabric: Lab\r\n")); err != nil {
		t.Fatal(err)
	}
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "APIC# show controller\nFabric: Lab") {
		t.Fatalf("transcripción inesperada: %q", text)
	}
	if strings.Contains(text, "\x1b[") {
		t.Fatal("el log contiene secuencias ANSI")
	}
	if strings.Contains(text, "\n\n\n\nAPIC#") {
		t.Fatal("el log conservó filas vacías antes del primer prompt")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("permisos=%o; esperado 600", info.Mode().Perm())
	}
}

func TestRecorderHandlesCarriageReturnRedraw(t *testing.T) {
	recorder, err := Start(t.TempDir(), Session{HostID: "switch", Name: "Switch"})
	if err != nil {
		t.Fatal(err)
	}
	path := recorder.Path()
	_, _ = recorder.Write([]byte("%                 \r\x1b[2KSwitch# show version\r\n"))
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "%   ") || !strings.Contains(string(data), "Switch# show version") {
		t.Fatalf("redibujado incorrecto: %q", data)
	}
}

func TestRecorderPreservesPromptAcrossCRThenEnterCRLF(t *testing.T) {
	recorder, err := Start(t.TempDir(), Session{HostID: "apic", Name: "APIC"})
	if err != nil {
		t.Fatal(err)
	}
	path := recorder.Path()
	_, _ = recorder.Write([]byte("router-1# \r"))
	_, _ = recorder.Write([]byte("\r\nrouter-1# "))
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "router-1# \nrouter-1# ") {
		t.Fatalf("recording perdió el prompt: %q", data)
	}
}

func TestRecorderCollapsesOnlyBlankBeforePrompt(t *testing.T) {
	recorder, err := Start(t.TempDir(), Session{HostID: "apic", Name: "APIC"})
	if err != nil {
		t.Fatal(err)
	}
	path := recorder.Path()
	_, _ = recorder.Write([]byte("result-a\r\n\r\nresult-b\r\n\r\nrouter-1#"))
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "result-a\n\nresult-b\nrouter-1#") {
		t.Fatalf("normalización inesperada: %q", text)
	}
}

func TestSafeNameNeverEscapesDirectory(t *testing.T) {
	if got := safeName("../../prod server"); strings.Contains(got, "/") || got == "" {
		t.Fatalf("nombre inseguro: %q", got)
	}
}
