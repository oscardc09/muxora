# Contribuir a Muxora

Gracias por ayudar a mejorar Muxora. Este proyecto prioriza compatibilidad con OpenSSH, configuraciones seguras y una experiencia TUI predecible.

## Preparar el entorno

Necesitas Go 1.26 o posterior, Git y un cliente OpenSSH disponible en el sistema.

```bash
git clone https://github.com/USUARIO/muxora.git
cd muxora
go mod download
make check
```

Sustituye `USUARIO` por la cuenta u organización donde se publique el proyecto.

## Flujo de trabajo

1. Crea una rama desde la rama principal.
2. Mantén cada cambio enfocado en una sola corrección o funcionalidad.
3. Formatea el código con `make fmt`.
4. Añade o actualiza pruebas para el comportamiento modificado.
5. Ejecuta `make check` antes de abrir un pull request.
6. Actualiza README, documentos y `CHANGELOG.md` cuando cambie el uso visible.

## Seguridad de los datos de prueba

No incluyas direcciones internas, nombres de equipos, usuarios reales, llaves SSH, contraseñas, archivos `known_hosts`, configuraciones personales ni recordings. Usa los rangos reservados `192.0.2.0/24`, `198.51.100.0/24` o `203.0.113.0/24` en ejemplos.

Los fallos de seguridad no deben abrirse como incidencias públicas; sigue [SECURITY.md](SECURITY.md).

## Pull requests

Describe el problema, la solución y cómo verificaste el cambio. Si afecta la terminal integrada, incluye el tipo de servidor o shell probado sin revelar datos de infraestructura. Los cambios grandes de arquitectura deben explicar compatibilidad, migración y riesgos.
