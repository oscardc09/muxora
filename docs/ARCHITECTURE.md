# Arquitectura de Muxora

## Flujo de inicio

```text
main
 ├─ interpreta flags y comandos
 ├─ resuelve la ruta del YAML
 ├─ Store.LoadOrCreate
 │   ├─ crea configuración inicial si no existe
 │   ├─ rechaza campos YAML desconocidos
 │   └─ valida versión, IDs, puertos y colores
 └─ ui.Run
     └─ Bubble Tea ejecuta Init → Update ↔ View
```

`cmd/muxora/main.go` mantiene la CLI pequeña. Los comandos no interactivos usan los mismos tipos y el mismo `Store` que la TUI, evitando dos implementaciones de persistencia.

## Modelo de configuración

`Config` es la raíz serializada:

- `Version`: versión del contrato YAML, actualmente `1`.
- `Defaults`: puerto, usuario, timeout e identidad opcional heredados por hosts.
- `Groups`: entidades visuales con ID, nombre, símbolo y color.
- `Hosts`: destinos SSH y asociaciones por ID de grupo.
- `Settings`: preferencias preparadas para crecer sin alterar hosts.

`Store.Save` valida antes de escribir. Crea un archivo temporal en el mismo directorio, aplica `0600`, escribe y finalmente usa `os.Rename`. Al estar en el mismo filesystem, el reemplazo evita dejar un YAML parcial si el proceso falla.

## Arquitectura Bubble Tea

### Estado

`model` contiene:

- Configuración y Store.
- Filtros, cursores y panel enfocado.
- Modal activo y campos de formularios.
- Dimensiones actuales de la ventana.
- Lista de sesiones y pestaña activa.
- Mensajes de estado para el usuario.

No existe estado global mutable. Bubble Tea entrega mensajes secuencialmente a `Update`, que produce el siguiente modelo y opcionalmente un `tea.Cmd` asíncrono.

### Modos

| Modo | Propósito |
|---|---|
| `modeList` | Catálogo y navegación normal |
| `modeSearch` | Captura texto para filtrar |
| `modeForm` | Alta o edición de host |
| `modeGroupForm` | Alta o edición de grupo |
| `modeDelete` | Confirmación destructiva |
| `modeHelp` | Atajos contextuales |
| `modeSession` | Entrada dirigida al PTY SSH activo |

El modo decide cómo interpretar una tecla. Por ejemplo, `j` mueve el cursor en catálogo, pero se envía literalmente al servidor dentro de una sesión.

## Sesiones SSH y concurrencia

Cada `sshSession` mantiene:

- ID interno monotónico.
- Host que originó la sesión.
- Proceso `ssh`.
- Descriptor PTY.
- Búfer de salida independiente.
- Estado de ejecución.
- Selección de texto propia.

Al conectar, `startSession` ejecuta OpenSSH directamente, sin shell, mediante `pty.StartWithSize`. `readSession` bloquea fuera de `Update` como un `tea.Cmd`. Cuando llegan bytes devuelve `sessionOutputMsg{id, data}`; el ID permite encontrar el búfer correcto aunque existan varias lecturas concurrentes.

Después de procesar un bloque, `Update` programa la siguiente lectura sólo para esa sesión. Cerrar una pestaña cierra su PTY, termina su proceso y elimina únicamente esa entrada del slice.

## Entrada y resize

`keyBytes` traduce eventos Bubble Tea a bytes de terminal:

- `Enter` → `CR`.
- Flechas → secuencias CSI.
- `Ctrl+C` → ETX para interrumpir el comando remoto.
- Runas normales → UTF-8.

`sessionSize` calcula filas y columnas reales del panel 3. Ante `WindowSizeMsg`, `resizeSessions` aplica el nuevo tamaño a todos los PTY vivos.

## Búfer y parser de terminal

`terminalBuffer` no es todavía un emulador VT completo. Su objetivo actual es una shell y CLIs de equipos de red:

- Elimina secuencias ANSI CSI y OSC para que no rompan el renderer principal.
- Conserva saltos CRLF.
- Interpreta CR sin LF como redibujado desde la columna cero.
- Procesa backspace y tabs.
- Limita memoria retenida.
- Presenta las últimas líneas que caben en el panel.

Aplicaciones remotas que dependen de posicionamiento arbitrario del cursor, como Vim o `top`, requieren integrar un emulador VT con una matriz de celdas.

## Selección y portapapeles

La selección nativa del emulador local incluiría los tres paneles porque comparten filas físicas. Por eso `updateMouse` convierte coordenadas globales en `textPoint{line, col}` relativo al panel 3. `SelectedText` extrae únicamente el fragmento del búfer SSH y `copyToClipboard` lo envía a `pbcopy`.

## Recording de sesiones

`internal/recording` recibe exclusivamente los bytes de salida del PTY. Mantiene un parser de streaming separado para eliminar CSI/OSC, resolver CRLF, redibujados con CR y backspace antes de escribir texto plano. No recibe eventos de teclado, evitando registrar directamente contraseñas.

Cada `sshSession` puede poseer un `Recorder`. `Ctrl+R` abre `modeRecordingForm`, un selector contextual que conserva activa la conexión mientras el usuario elige la carpeta. La confirmación llama a `startRecordingAt`; `finishSession` y `closeActiveSession` garantizan el cierre del archivo. La opción `settings.record_sessions` continúa iniciando el recorder automáticamente con la carpeta configurada. Los archivos se agrupan por fecha, usan nombres saneados y permisos `0600`.

## Extensiones futuras

Las funciones nuevas deben aislarse tras paquetes o adaptadores:

- `internal/sftp`: operaciones y transferencias.
- `internal/tunnel`: local, remote y dynamic forwarding.
- `internal/vault`: Keychain, Secret Service o credential manager.
- `internal/terminal`: emulación VT completa extraída de `ui`.
- `internal/session`: ciclo de vida extraído de `ui`.

La regla principal es que la vista no debe conocer detalles del protocolo y el protocolo no debe importar estilos visuales.
