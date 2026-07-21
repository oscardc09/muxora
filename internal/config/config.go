package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config es la raíz del documento YAML. Version permite evolucionar el formato
// explícitamente sin interpretar configuraciones antiguas de forma ambigua.
type Config struct {
	Version  int      `yaml:"version"`
	Defaults Defaults `yaml:"defaults,omitempty"`
	Groups   []Group  `yaml:"groups,omitempty"`
	Hosts    []Host   `yaml:"hosts"`
	Settings Settings `yaml:"settings,omitempty"`
}

// Group describe una agrupación visual. Los hosts guardan Group.ID, no Name,
// para permitir cambiar el texto presentado sin romper asociaciones.
type Group struct {
	ID     string `yaml:"id"`
	Name   string `yaml:"name"`
	Symbol string `yaml:"symbol,omitempty"`
	Color  string `yaml:"color,omitempty"`
}

// Defaults contiene valores heredados cuando un Host no define el suyo.
type Defaults struct {
	User               string `yaml:"user,omitempty"`
	Port               int    `yaml:"port,omitempty"`
	IdentityFile       string `yaml:"identity_file,omitempty"`
	ConnectTimeoutSecs int    `yaml:"connect_timeout_seconds,omitempty"`
}

// Host representa un destino SSH persistente. IdentityFile es opcional y se
// mantiene principalmente para casos avanzados; OpenSSH resuelve agent y llaves
// estándar automáticamente cuando se omite.
type Host struct {
	ID           string   `yaml:"id"`
	Name         string   `yaml:"name"`
	Address      string   `yaml:"address"`
	User         string   `yaml:"user,omitempty"`
	Port         int      `yaml:"port,omitempty"`
	IdentityFile string   `yaml:"identity_file,omitempty"`
	Groups       []string `yaml:"groups,omitempty"`
	Favorite     bool     `yaml:"favorite,omitempty"`
}

// Settings reserva preferencias de interfaz independientes del catálogo.
type Settings struct {
	Theme             string `yaml:"theme,omitempty"`
	ConfirmBeforeExit bool   `yaml:"confirm_before_exit,omitempty"`
	RecordSessions    bool   `yaml:"record_sessions,omitempty"`
	LogDirectory      string `yaml:"log_directory,omitempty"`
}

// FindHost busca por el identificador estable utilizado por CLI y TUI.
func (c Config) FindHost(id string) (Host, bool) {
	for _, h := range c.Hosts {
		if h.ID == id {
			return h, true
		}
	}
	return Host{}, false
}

// Validate comprueba el contrato completo antes de usar o persistir Config.
func (c Config) Validate() error {
	if c.Version != 1 {
		return fmt.Errorf("versión %d no soportada; se esperaba 1", c.Version)
	}
	seen := make(map[string]struct{}, len(c.Hosts))
	seenGroups := make(map[string]struct{}, len(c.Groups))
	for i, group := range c.Groups {
		if strings.TrimSpace(group.ID) == "" || strings.TrimSpace(group.Name) == "" {
			return fmt.Errorf("groups[%d]: id y name son obligatorios", i)
		}
		if _, exists := seenGroups[group.ID]; exists {
			return fmt.Errorf("id de grupo duplicado: %q", group.ID)
		}
		seenGroups[group.ID] = struct{}{}
		if group.Color != "" && !validHexColor(group.Color) {
			return fmt.Errorf("grupo %q: color inválido; usa #RRGGBB", group.ID)
		}
	}
	for i, h := range c.Hosts {
		if strings.TrimSpace(h.ID) == "" || strings.TrimSpace(h.Name) == "" || strings.TrimSpace(h.Address) == "" {
			return fmt.Errorf("hosts[%d]: id, name y address son obligatorios", i)
		}
		if _, exists := seen[h.ID]; exists {
			return fmt.Errorf("id de host duplicado: %q", h.ID)
		}
		seen[h.ID] = struct{}{}
		if h.Port < 0 || h.Port > 65535 {
			return fmt.Errorf("host %q: puerto inválido", h.ID)
		}
	}
	return nil
}

func validHexColor(value string) bool {
	if len(value) != 7 || value[0] != '#' {
		return false
	}
	for _, r := range value[1:] {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

// Default devuelve una configuración mínima válida y segura.
func Default() Config {
	return Config{Version: 1, Defaults: Defaults{Port: 22, ConnectTimeoutSecs: 10}, Hosts: []Host{}, Settings: Settings{Theme: "dark", ConfirmBeforeExit: true}}
}

// ResolvePath usa la ruta explícita o el directorio de configuración estándar
// del sistema operativo.
func ResolvePath(explicit string) (string, error) {
	if explicit != "" {
		return expandHome(explicit)
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	target := filepath.Join(dir, "muxora", "config.yaml")
	legacy := filepath.Join(dir, "eres-e", "config.yaml")
	if err := migrateLegacyConfig(legacy, target); err != nil {
		return "", err
	}
	return target, nil
}

// migrateLegacyConfig copia una sola vez la configuración de ERES-E. Nunca
// sobrescribe un archivo Muxora existente y conserva el original como respaldo.
func migrateLegacyConfig(legacy, target string) error {
	if _, err := os.Stat(target); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := os.ReadFile(legacy)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("leer configuración anterior: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".migration-*.yaml")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, target); err != nil {
		return err
	}
	return nil
}

func expandHome(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
	}
	return filepath.Clean(path), nil
}

// Store encapsula una ruta y centraliza todas las operaciones de persistencia.
type Store struct{ path string }

// NewStore crea un Store para path sin acceder todavía al filesystem.
func NewStore(path string) *Store { return &Store{path: path} }

// Path devuelve la ruta resuelta utilizada por el Store.
func (s *Store) Path() string { return s.path }

// LoadOrCreate carga YAML estricto o crea Default si el archivo no existe.
func (s *Store) LoadOrCreate() (Config, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		cfg := Default()
		return cfg, s.Save(cfg)
	}
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("leer YAML: %w", err)
	}
	return cfg, cfg.Validate()
}

// Save valida y reemplaza el YAML atómicamente con permisos 0600. El temporal
// se crea junto al destino para que os.Rename permanezca en el mismo filesystem.
func (s *Store) Save(cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".config-*.yaml")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.path)
}
