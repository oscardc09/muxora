// Package sshclient traduce un Host y sus Defaults a argumentos seguros de
// OpenSSH. No utiliza una shell, por lo que nombres y direcciones no se evalúan
// como comandos. La TUI reutiliza Args para iniciar ssh dentro de un PTY.
package sshclient
