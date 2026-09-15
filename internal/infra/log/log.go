// Package log monta o registrador estruturado da aplicação.
package log

import (
	"io"
	"log/slog"
)

// New devolve um registrador de texto com o nível pedido.
//
// Texto e não JSON: os registos desta aplicação são lidos por uma pessoa a
// depurar no terminal, não por um agregador.
func New(nivel string, w io.Writer) *slog.Logger {
	var lvl slog.Level
	switch nivel {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: lvl}))
}
