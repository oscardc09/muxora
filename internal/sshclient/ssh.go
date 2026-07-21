package sshclient

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/local/muxora/internal/config"
)

// Args construye los argumentos de OpenSSH aplicando primero valores del Host
// y después Defaults. Devuelve un slice para ejecución directa, no una cadena
// destinada a una shell.
func Args(host config.Host, defaults config.Defaults) []string {
	user := first(host.User, defaults.User)
	port := host.Port
	if port == 0 {
		port = defaults.Port
	}
	identity := first(host.IdentityFile, defaults.IdentityFile)
	timeout := defaults.ConnectTimeoutSecs
	if timeout == 0 {
		timeout = 10
	}

	args := []string{"-o", "ConnectTimeout=" + strconv.Itoa(timeout)}
	if port != 0 {
		args = append(args, "-p", strconv.Itoa(port))
	}
	if identity != "" {
		args = append(args, "-i", expandHome(identity))
	}
	target := host.Address
	if user != "" {
		target = user + "@" + target
	}
	return append(args, target)
}

// Connect entrega stdin/stdout/stderr actuales a OpenSSH. Es utilizado por el
// subcomando no interactivo `muxora connect`; la TUI utiliza Args dentro de PTY.
func Connect(host config.Host, defaults config.Defaults) error {
	path, err := exec.LookPath("ssh")
	if err != nil {
		return fmt.Errorf("OpenSSH no está instalado: %w", err)
	}
	cmd := exec.Command(path, Args(host, defaults)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return home + "/" + strings.TrimPrefix(path, "~/")
		}
	}
	return path
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
