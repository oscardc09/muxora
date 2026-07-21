// Package config define el contrato YAML de Muxora y su persistencia.
//
// Store es la única pieza autorizada para leer y escribir la configuración.
// Valida campos desconocidos, crea el directorio con permisos privados y guarda
// de forma atómica mediante un archivo temporal seguido de rename.
package config
