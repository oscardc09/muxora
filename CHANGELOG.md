# Changelog

Los cambios relevantes de Muxora se documentan aquí. El proyecto sigue [Semantic Versioning](https://semver.org/lang/es/) y el formato de [Keep a Changelog](https://keepachangelog.com/es-ES/1.1.0/).

## [Unreleased]

### Añadido

- Preparación del repositorio para contribuciones públicas, con licencia MIT, política de seguridad y CI.
- Selector visual para elegir el destino de recordings manuales.

## [0.4.0] - 2026-07-19

### Añadido

- Recording manual o automático por sesión, con transcripciones sanitizadas y permisos `0600`.
- Comando `muxora logs` y configuración de directorio de logs.
- Pestañas para múltiples sesiones SSH dentro del panel principal.
- Selección de texto con mouse limitada a la salida SSH.
- CRUD de hosts y grupos, símbolos y colores desde la TUI.

### Corregido

- Conservación de prompts de equipos de red cuando el PTY entrega secuencias `CR`, `CR`, `LF` separadas.
- Ajuste de líneas largas a la anchura de la terminal sin mezclar el contenido de otros paneles.

### Cambiado

- El proyecto, binario, configuración y documentación pasaron de ERES-E a Muxora.
- La llave SSH dejó de mostrarse en el formulario normal; OpenSSH resuelve identidades implícitamente.
