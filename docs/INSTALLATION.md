# Instalación y distribución

## Opción 1: ejecutar la demo desde el repositorio

```bash
cd /ruta/al/repositorio/muxora
make check
./bin/muxora
```

Esto es apropiado para desarrollo, pero obliga a recordar la ruta del repositorio.

## Opción 2: instalar como comando global con Go

```bash
cd /ruta/al/repositorio/muxora
make install
```

El binario queda en:

```bash
$(go env GOPATH)/bin/muxora
```

La ubicación exacta depende de tu instalación de Go; consúltala con `go env GOPATH`.

Añade una sola vez esta línea a `~/.zshrc`:

```bash
export PATH="$(go env GOPATH)/bin:$PATH"
```

Después:

```bash
source ~/.zshrc
muxora --version
muxora
```

Esta es la forma más simple de obtener una experiencia similar a `lazygit`: escribir solamente el nombre desde cualquier carpeta.

## Opción 3: instalar en `~/.local/bin`

```bash
cd /ruta/al/repositorio/muxora
make install-local
```

Asegura que `~/.local/bin` esté en el PATH:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

También existe un instalador local:

```bash
./scripts/install.sh
```

No requiere `sudo`, no modifica `/usr/local` y reemplaza únicamente `~/.local/bin/muxora`.

## Configuración del usuario

Al ejecutar sin `--config`, Muxora usa:

```text
~/Library/Application Support/muxora/config.yaml
```

### Migración desde ERES-E

En el primer inicio, si Muxora todavía no tiene configuración pero existe:

```text
~/Library/Application Support/eres-e/config.yaml
```

el archivo se copia automáticamente a la ruta nueva con permisos `0600`. El original no se borra y funciona como respaldo. Si Muxora ya tiene un archivo propio, la migración no lo sobrescribe.

El archivo `config.example.yaml` es sólo una muestra del repositorio. Para uso diario ejecuta:

```bash
muxora
```

## Builds con versión

```bash
make build VERSION=0.1.0
./bin/muxora --version
```

Make inserta versión, commit y fecha mediante `-ldflags`. Para un release reproducible:

```bash
VERSION=0.1.0
make clean
make check VERSION="$VERSION"
```

## Distribución como Lazygit

Lazygit se instala cómodamente porque publica versiones, binarios por plataforma y fórmulas de gestores de paquetes. Para que Muxora llegue a ese nivel hacen falta estos pasos:

1. Publicar el código en un repositorio Git remoto.
2. Cambiar `module github.com/local/muxora` por la ruta real, por ejemplo `github.com/USUARIO/muxora`.
3. Crear tags semánticos: `v0.1.0`, `v0.2.0`, etc.
4. Configurar CI para ejecutar `make check`.
5. Generar archivos para `darwin/amd64`, `darwin/arm64`, `linux/amd64` y `linux/arm64`.
6. Publicar checksums SHA-256 en cada GitHub Release.
7. Crear una fórmula Homebrew que descargue el archivo correspondiente y verifique su checksum.

Una vez publicado el módulo, usuarios con Go podrán instalarlo así:

```bash
go install github.com/USUARIO/muxora/cmd/muxora@latest
```

Con una fórmula Homebrew publicada, la experiencia final sería:

```bash
brew install USUARIO/tap/muxora
muxora
```

No se debe anunciar `go install ...@latest` ni Homebrew hasta sustituir `USUARIO`, publicar el repositorio y disponer de releases verificables.

## Desinstalación

Según el método elegido, elimina sólo el binario correspondiente:

```bash
rm "$(go env GOPATH)/bin/muxora"
```

o:

```bash
rm "$HOME/.local/bin/muxora"
```

La configuración se conserva. Si realmente deseas borrarla también:

```bash
rm "$HOME/Library/Application Support/muxora/config.yaml"
```
