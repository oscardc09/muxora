package sshclient

import (
	"slices"
	"testing"

	"github.com/local/muxora/internal/config"
)

func TestArgsOverridesDefaults(t *testing.T) {
	host := config.Host{Address: "server.example.com", User: "deploy", Port: 2222, IdentityFile: "/keys/prod"}
	defaults := config.Defaults{User: "root", Port: 22, ConnectTimeoutSecs: 15}
	got := Args(host, defaults)
	want := []string{"-o", "ConnectTimeout=15", "-p", "2222", "-i", "/keys/prod", "deploy@server.example.com"}
	if !slices.Equal(got, want) {
		t.Fatalf("Args() = %#v; esperado %#v", got, want)
	}
}
