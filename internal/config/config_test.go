package config

import (
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.yaml")
	store := NewStore(path)
	want := Default()
	want.Hosts = []Host{{ID: "prod", Name: "Producción", Address: "prod.example.com", User: "deploy"}}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadOrCreate()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hosts) != 1 || got.Hosts[0].ID != "prod" {
		t.Fatalf("configuración inesperada: %#v", got)
	}
}

func TestValidateRejectsDuplicateID(t *testing.T) {
	cfg := Default()
	cfg.Hosts = []Host{
		{ID: "same", Name: "Uno", Address: "one"},
		{ID: "same", Name: "Dos", Address: "two"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("se esperaba un error por ID duplicado")
	}
}

func TestValidateGroupColor(t *testing.T) {
	cfg := Default()
	cfg.Groups = []Group{{ID: "prod", Name: "Producción", Symbol: "◆", Color: "red"}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("se esperaba un error por color inválido")
	}
	cfg.Groups[0].Color = "#F43F5E"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("color hexadecimal válido rechazado: %v", err)
	}
}

func TestMigrateLegacyConfigWithoutOverwritingTarget(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "eres-e", "config.yaml")
	target := filepath.Join(root, "muxora", "config.yaml")
	legacyStore := NewStore(legacy)
	cfg := Default()
	cfg.Hosts = []Host{{ID: "legacy", Name: "Anterior", Address: "old.example"}}
	if err := legacyStore.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacyConfig(legacy, target); err != nil {
		t.Fatal(err)
	}
	loaded, err := NewStore(target).LoadOrCreate()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.FindHost("legacy"); !ok {
		t.Fatal("el host anterior no fue migrado")
	}

	loaded.Hosts = append(loaded.Hosts, Host{ID: "new", Name: "Nuevo", Address: "new.example"})
	if err := NewStore(target).Save(loaded); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacyConfig(legacy, target); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewStore(target).LoadOrCreate()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.FindHost("new"); !ok {
		t.Fatal("la migración sobrescribió Muxora")
	}
}
