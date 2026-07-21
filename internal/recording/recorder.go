package recording

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Session identifica la conexión dentro del encabezado de una transcripción.
type Session struct {
	HostID  string
	Name    string
	Address string
	User    string
}

// Recorder convierte salida PTY con ANSI en texto plano y la escribe con
// permisos privados. Es seguro invocarlo desde el loop secuencial de Bubble Tea
// y protege el cierre ante futuras escrituras concurrentes.
type Recorder struct {
	mu           sync.Mutex
	file         *os.File
	path         string
	line         []rune
	lineStart    int
	escape       int
	pendingCR    bool
	hasContent   bool
	pendingBlank int
	closed       bool
}

// DefaultDirectory devuelve el directorio estándar de logs de Muxora.
func DefaultDirectory() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "muxora", "logs"), nil
}

// ResolveDirectory expande ~/ o usa DefaultDirectory cuando value está vacío.
func ResolveDirectory(value string) (string, error) {
	if value == "" {
		return DefaultDirectory()
	}
	if value == "~" || strings.HasPrefix(value, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, strings.TrimPrefix(value, "~/")), nil
	}
	return filepath.Clean(value), nil
}

// Start crea una transcripción con encabezado y permisos 0600.
func Start(directory string, session Session) (*Recorder, error) {
	dir, err := ResolveDirectory(directory)
	if err != nil {
		return nil, err
	}
	dayDir := filepath.Join(dir, time.Now().Format("2006-01-02"))
	if err := os.MkdirAll(dayDir, 0700); err != nil {
		return nil, err
	}
	name := fmt.Sprintf("%s_%s_%s.log", time.Now().Format("150405.000000000"), safeName(session.HostID), safeName(session.Name))
	path := filepath.Join(dayDir, name)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	r := &Recorder{file: file, path: path}
	header := fmt.Sprintf("# Muxora SSH session log\n# Host: %s (%s)\n# Address: %s\n# User: %s\n# Started: %s\n# Security: PTY output only; raw keystrokes are not recorded.\n\n", session.Name, session.HostID, session.Address, session.User, time.Now().Format(time.RFC3339))
	if _, err := file.WriteString(header); err != nil {
		file.Close()
		return nil, err
	}
	return r, nil
}

// Path devuelve el archivo de la transcripción.
func (r *Recorder) Path() string { return r.path }

// Write limpia ANSI/OSC y conserva el comportamiento de CRLF y redibujado.
func (r *Recorder) Write(data []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, os.ErrClosed
	}
	for _, ch := range []rune(string(data)) {
		if err := r.writeRune(ch); err != nil {
			return 0, err
		}
	}
	return len(data), nil
}

func (r *Recorder) writeRune(ch rune) error {
	if r.pendingCR {
		if ch == '\r' {
			return nil
		}
		r.pendingCR = false
		if ch == '\n' {
			return r.flushLine(true)
		}
		r.line = r.line[:0]
	}
	switch r.escape {
	case 1:
		if ch == '[' {
			r.escape = 2
		} else if ch == ']' {
			r.escape = 3
		} else {
			r.escape = 0
		}
		return nil
	case 2:
		if ch >= '@' && ch <= '~' {
			r.escape = 0
		}
		return nil
	case 3:
		if ch == '\a' {
			r.escape = 0
		} else if ch == '\x1b' {
			r.escape = 4
		}
		return nil
	case 4:
		if ch == '\\' {
			r.escape = 0
		} else {
			r.escape = 3
		}
		return nil
	}
	switch ch {
	case '\x1b':
		r.escape = 1
	case '\r':
		r.pendingCR = true
	case '\n':
		return r.flushLine(true)
	case '\b':
		if len(r.line) > 0 {
			r.line = r.line[:len(r.line)-1]
		}
	case '\t':
		r.line = append(r.line, ' ', ' ', ' ', ' ')
	default:
		if ch >= 0x20 {
			r.line = append(r.line, ch)
		}
	}
	return nil
}

func (r *Recorder) flushLine(newline bool) error {
	// Algunos equipos limpian la pantalla con varias filas vacías antes del
	// prompt. Se omiten sólo hasta encontrar el primer contenido real.
	if len(r.line) == 0 && !r.hasContent {
		return nil
	}
	if len(r.line) == 0 {
		if newline {
			r.pendingBlank++
		}
		return nil
	}
	if r.pendingBlank > 0 {
		if !looksLikePrompt(r.line) {
			if _, err := r.file.WriteString(strings.Repeat("\n", r.pendingBlank)); err != nil {
				return err
			}
		}
		r.pendingBlank = 0
	}
	if len(r.line) > 0 {
		if _, err := r.file.WriteString(string(r.line)); err != nil {
			return err
		}
		r.hasContent = true
	}
	if newline {
		if _, err := r.file.WriteString("\n"); err != nil {
			return err
		}
	}
	r.line = r.line[:0]
	return nil
}

func looksLikePrompt(line []rune) bool {
	value := strings.TrimSpace(string(line))
	if value == "" || len([]rune(value)) > 160 {
		return false
	}
	last := value[len(value)-1]
	return last == '#' || last == '>' || last == '$' || last == '%'
}

// Close termina la transcripción y añade hora de finalización.
func (r *Recorder) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	if len(r.line) > 0 {
		if err := r.flushLine(true); err != nil {
			r.file.Close()
			return err
		}
	}
	if _, err := r.file.WriteString("\n# Ended: " + time.Now().Format(time.RFC3339) + "\n"); err != nil {
		r.file.Close()
		return err
	}
	return r.file.Close()
}

func safeName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "session"
	}
	var b strings.Builder
	for _, ch := range value {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			b.WriteRune(ch)
		} else {
			b.WriteRune('-')
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		return "session"
	}
	return result
}
