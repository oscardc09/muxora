# Política de seguridad

## Versiones soportadas

Mientras Muxora permanezca en etapa de demo, las correcciones de seguridad se aplican solamente a la versión más reciente de la rama principal.

## Reportar una vulnerabilidad

No publiques detalles sensibles en una incidencia. Cuando el repositorio esté en GitHub, habilita **Private vulnerability reporting** en `Settings > Security > Code security and analysis` y usa la opción **Report a vulnerability** del repositorio.

Incluye una descripción, pasos de reproducción, impacto, versión o commit afectado y, si existe, una mitigación. No adjuntes llaves, contraseñas, configuraciones ni logs reales. El responsable del repositorio deberá confirmar la recepción y coordinar la divulgación antes de hacer públicos los detalles.

## Datos sensibles

Muxora delega la autenticación en OpenSSH y no debe almacenar contraseñas. Sin embargo, la configuración puede revelar infraestructura y los recordings pueden contener secretos impresos por comandos remotos.

- Conserva `config.yaml` y los recordings fuera del repositorio.
- Mantén permisos `0600` en datos locales.
- Revisa los logs antes de compartirlos.
- Rota cualquier credencial expuesta accidentalmente; borrarla del último commit no la elimina del historial Git.
- Verifica cuidadosamente cambios en argumentos SSH, rutas de archivos, permisos, escape ANSI y manejo del PTY.

Si un secreto llega al historial público, revócalo primero y luego reescribe el historial siguiendo el procedimiento de tu proveedor Git.
