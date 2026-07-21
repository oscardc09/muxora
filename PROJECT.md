# Contexto del proyecto Muxora

Este archivo permite retomar el proyecto en otra conversación o por otro desarrollador. Debe actualizarse cuando cambie la arquitectura o se añada una función importante.

## Objetivo

Muxora es una TUI en Go para catalogar hosts y trabajar con múltiples sesiones SSH dentro del mismo programa. Su experiencia visual está inspirada en Lazygit, LazyVim y Tabby.

## Estado de la demo

- Catálogo YAML de hosts y grupos.
- CRUD de hosts y grupos dentro de la TUI.
- Símbolos Unicode y colores por grupo.
- Búsqueda, favoritos y filtrado por grupos.
- Navegación mediante teclado y mouse.
- Sesiones OpenSSH embebidas mediante PTY.
- Varias sesiones simultáneas en pestañas.
- Sesiones en segundo plano al volver al catálogo.
- Selección limitada al contenido SSH y copia mediante `pbcopy` en macOS.
- Manejo de ANSI básico, CRLF y redibujado de prompts con retorno de carro.
- CLI para validar, listar, agregar, eliminar y conectar hosts.
- Migración automática y no destructiva desde la configuración anterior de ERES-E.
- Recording manual con selector visual de destino o automático por sesión, con texto plano, metadatos, permisos privados y exclusión de pulsaciones crudas.

## Decisiones importantes

1. Se invoca `/usr/bin/ssh` en lugar de implementar el protocolo SSH. Esto conserva `ssh-agent`, `known_hosts`, ProxyJump y la configuración del usuario.
2. Nunca se guarda una contraseña en YAML.
3. `identity_file` continúa en el esquema por compatibilidad, pero no se muestra en el formulario normal.
4. Cada sesión posee su propio proceso, PTY, búfer y estado; los mensajes asíncronos incluyen un ID para impedir que se mezcle la salida.
5. La configuración se guarda con permisos `0600` y reemplazo atómico.
6. La selección de texto es interna porque una selección nativa copiaría las filas completas de los tres paneles.

## Archivos principales

| Ruta | Responsabilidad |
|---|---|
| `cmd/muxora/main.go` | CLI, flags, carga inicial y lanzamiento de la TUI |
| `internal/config/config.go` | Esquema YAML, validación y Store atómico |
| `internal/sshclient/ssh.go` | Construcción de argumentos y ejecución directa de OpenSSH |
| `internal/ui/model.go` | Estado, eventos, vistas, PTY, pestañas y selección |
| `config.example.yaml` | Ejemplo editable de configuración |
| `docs/ARCHITECTURE.md` | Explicación técnica del flujo completo |
| `docs/INSTALLATION.md` | Instalación para usuarios y estrategia de releases |
| `docs/DEPENDENCIES.md` | Dependencias directas y por qué existen |

## Próximos pasos recomendados

1. Separar `internal/ui/model.go` en `model`, `update`, `views`, `session` y `terminal` sin cambiar comportamiento.
2. Incorporar un emulador VT completo para aplicaciones remotas de pantalla completa como Vim, `top` o `less`.
3. Añadir scrollback manual y búsqueda dentro de la salida SSH.
4. Implementar SFTP como adaptador separado.
5. Publicar el repositorio, automatizar releases y crear una fórmula Homebrew.

## Cómo retomar el trabajo

En una conversación nueva indica:

```text
Continúa el proyecto Muxora ubicado en esta carpeta.
Lee PROJECT.md, docs/ARCHITECTURE.md y README.md antes de modificar código.
Ejecuta make check después de los cambios.
```
