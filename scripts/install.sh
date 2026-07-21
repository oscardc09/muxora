#!/bin/sh
set -eu

# Instalador local y sin privilegios. Compila desde la raíz del repositorio y
# coloca un único binario en ~/.local/bin, una convención compatible con macOS
# y Linux cuando esa carpeta forma parte de PATH.
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
PROJECT_DIR=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
INSTALL_DIR=${MUXORA_INSTALL_DIR:-"$HOME/.local/bin"}
VERSION=${MUXORA_VERSION:-dev}
BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)
COMMIT=$(git -C "$PROJECT_DIR" rev-parse --short HEAD 2>/dev/null || printf none)
TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_DIR"' EXIT HUP INT TERM

if ! command -v go >/dev/null 2>&1; then
    printf '%s\n' "Error: Go no está instalado. Consulta docs/INSTALLATION.md." >&2
    exit 1
fi

mkdir -p "$INSTALL_DIR"
cd "$PROJECT_DIR"
go test ./...
go build -ldflags "-s -w -X main.version=$VERSION -X main.commit=$COMMIT -X main.date=$BUILD_DATE" -o "$TEMP_DIR/muxora" ./cmd/muxora
install -m 0755 "$TEMP_DIR/muxora" "$INSTALL_DIR/muxora"

printf '%s\n' "Muxora instalado en $INSTALL_DIR/muxora"
case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *) printf '%s\n' "Añade al PATH: export PATH=\"$INSTALL_DIR:\$PATH\"" ;;
esac
