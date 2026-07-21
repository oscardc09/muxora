package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/local/muxora/internal/config"
	"github.com/local/muxora/internal/recording"
	"github.com/local/muxora/internal/sshclient"
	"github.com/local/muxora/internal/ui"
)

// Estas variables se reemplazan durante el build mediante -ldflags. Sus valores
// por defecto permiten ejecutar `go run` sin un proceso de release.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	configPath := flag.String("config", "", "ruta del archivo de configuración")
	showVersion := flag.Bool("version", false, "mostrar versión y salir")
	flag.Parse()
	if *showVersion {
		fmt.Printf("muxora %s (commit %s, built %s)\n", version, commit, date)
		return
	}

	path, err := config.ResolvePath(*configPath)
	fatalIf(err)
	store := config.NewStore(path)
	cfg, err := store.LoadOrCreate()
	fatalIf(err)

	args := flag.Args()
	if len(args) == 1 && args[0] == "logs" {
		dir, err := recording.ResolveDirectory(cfg.Settings.LogDirectory)
		fatalIf(err)
		fmt.Println(dir)
		return
	}
	if len(args) >= 1 && args[0] == "host" {
		handleHostCommand(store, &cfg, args[1:])
		return
	}
	if len(args) >= 2 && args[0] == "connect" {
		host, ok := cfg.FindHost(args[1])
		if !ok {
			fatalIf(fmt.Errorf("host %q no encontrado", args[1]))
		}
		fatalIf(sshclient.Connect(host, cfg.Defaults))
		return
	}
	if len(args) == 1 && args[0] == "validate" {
		fmt.Printf("Configuración válida: %s (%d hosts)\n", path, len(cfg.Hosts))
		return
	}
	if len(args) != 0 {
		fmt.Fprintln(os.Stderr, "uso: muxora [--config ruta] [validate|logs|connect HOST|host add|host remove|host list]")
		os.Exit(2)
	}

	fatalIf(ui.Run(store, cfg))
}

func handleHostCommand(store *config.Store, cfg *config.Config, args []string) {
	if len(args) == 1 && args[0] == "list" {
		for _, h := range cfg.Hosts {
			fmt.Printf("%-18s %-24s %s\n", h.ID, h.Address, h.Name)
		}
		return
	}
	if len(args) >= 4 && args[0] == "add" {
		host := config.Host{ID: args[1], Name: args[2], Address: args[3]}
		if len(args) >= 5 {
			host.User = args[4]
		}
		if _, exists := cfg.FindHost(host.ID); exists {
			fatalIf(fmt.Errorf("ya existe el host %q", host.ID))
		}
		cfg.Hosts = append(cfg.Hosts, host)
		fatalIf(store.Save(*cfg))
		fmt.Printf("Host %q agregado a %s\n", host.ID, store.Path())
		return
	}
	if len(args) == 2 && args[0] == "remove" {
		for i, h := range cfg.Hosts {
			if h.ID == args[1] {
				cfg.Hosts = append(cfg.Hosts[:i], cfg.Hosts[i+1:]...)
				fatalIf(store.Save(*cfg))
				fmt.Printf("Host %q eliminado\n", args[1])
				return
			}
		}
		fatalIf(fmt.Errorf("host %q no encontrado", args[1]))
	}
	fmt.Fprintln(os.Stderr, "uso:")
	fmt.Fprintln(os.Stderr, "  muxora host list")
	fmt.Fprintln(os.Stderr, "  muxora host add ID NOMBRE DIRECCION [USUARIO]")
	fmt.Fprintln(os.Stderr, "  muxora host remove ID")
	os.Exit(2)
}

func fatalIf(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
