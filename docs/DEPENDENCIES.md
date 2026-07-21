# Dependencias

## Requisitos del sistema

| Dependencia | Uso | Obligatoria en ejecución |
|---|---|---|
| OpenSSH `ssh` | Conexiones remotas, agent y known_hosts | Sí |
| `pbcopy` | Copiar selección SSH en macOS | Sólo para copiar |
| Go 1.26+ | Compilar, probar e instalar desde fuente | No después de compilar |
| Git | Versionado y metadatos de compilación | Sólo desarrollo |
| Make | Atajos reproducibles del proyecto | Opcional |

## Módulos Go directos

### `github.com/charmbracelet/bubbletea`

Framework TUI basado en Elm Architecture. Gestiona entrada de teclado y mouse, resize, alternate screen y el ciclo `Model → Update → View`.

### `github.com/charmbracelet/lipgloss`

Construye paneles, bordes, colores, padding y composición horizontal/vertical. Aunque inicialmente llegó como dependencia transitiva, Muxora lo importa directamente.

### `github.com/creack/pty`

Crea y redimensiona el pseudo-terminal donde se ejecuta OpenSSH. Es necesario para que el proceso remoto crea que dispone de una terminal interactiva.

### `gopkg.in/yaml.v3`

Decodifica y codifica YAML. Se usa con `KnownFields(true)` para detectar errores tipográficos en claves de configuración.

## Dependencias indirectas

Las entradas marcadas `// indirect` en `go.mod` son utilizadas por Bubble Tea y Lip Gloss para ANSI, colores, anchura Unicode, detección de terminal y lectura cancelable. No deben añadirse manualmente al código salvo que pasen a utilizarse directamente.

## Actualización segura

```bash
go list -m -u all
go get github.com/charmbracelet/bubbletea@VERSION
go get github.com/charmbracelet/lipgloss@VERSION
go get github.com/creack/pty@VERSION
go get gopkg.in/yaml.v3@VERSION
go mod tidy
make check
```

Nunca actualices todas las dependencias y publiques sin probar conexiones, resize, mouse, selección, pestañas y compatibilidad YAML.
