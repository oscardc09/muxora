# Publicar Muxora en GitHub

Esta guía comienza cuando el proyecto local ya pasa `make check`. Ninguno de estos pasos ha sido ejecutado contra un servicio remoto automáticamente.

## 1. Elegir la ruta definitiva del módulo

Antes del primer commit público, sustituye el marcador del archivo `go.mod`:

```text
module github.com/local/muxora
```

por la cuenta u organización real:

```text
module github.com/USUARIO/muxora
```

Actualiza también `USUARIO` en `README.md`, `CONTRIBUTING.md` y `docs/INSTALLATION.md`. Después ejecuta:

```bash
go mod tidy
make check
```

No se ha reemplazado ese valor en esta preparación porque el nombre de usuario de GitHub no debe inventarse.

## 2. Revisar antes de publicar

```bash
git status --short
git diff --check
rg -n --hidden "/Users/|BEGIN .*PRIVATE KEY|password:|passwd:|token:" \
  -g '!bin/**' -g '!work/**' .
make check
```

Comprueba manualmente que no existan hosts, usuarios, dominios, rutas personales, configuraciones ni logs reales. `.gitignore` excluye los destinos habituales, pero no sustituye esta revisión.

## 3. Crear el primer commit local

```bash
git add .
git status
git commit -m "feat: initial Muxora release"
```

Revisa siempre `git status` antes de confirmar. El directorio local ya usa la rama `main` cuando fue inicializado con `git init -b main`.

## 4. Crear y conectar el repositorio

Crea en GitHub un repositorio vacío llamado `muxora`, sin generar README, licencia ni `.gitignore`, porque esos archivos ya existen localmente. Luego:

```bash
git remote add origin git@github.com:USUARIO/muxora.git
git push -u origin main
```

Puedes usar HTTPS en lugar de SSH si lo prefieres. Verifica el remoto antes del push con `git remote -v`.

## 5. Configuración recomendada en GitHub

- Habilita **Private vulnerability reporting**.
- Protege `main` y exige que el check **Go checks** termine correctamente.
- Exige revisión antes de fusionar pull requests cuando haya más colaboradores.
- Desactiva los pushes forzados y el borrado de la rama principal.
- Mantén el repositorio privado durante la primera revisión si aún hay dudas sobre datos sensibles.

El workflow `.github/workflows/ci.yml` comprueba formato, integridad de módulos, `go vet`, pruebas con detector de carreras y compilación.

## 6. Crear un release

Actualiza `CHANGELOG.md`, confirma que la versión sea coherente y crea un tag anotado:

```bash
git tag -a v0.4.0 -m "Muxora v0.4.0"
git push origin v0.4.0
```

El CI actual valida el código, pero todavía no publica binarios. Antes de ofrecer Homebrew conviene incorporar un workflow de releases que genere artefactos para macOS y Linux, firme o publique checksums SHA-256 y adjunte todo al release. No anuncies `go install ...@latest` hasta que la ruta real del módulo esté publicada.
