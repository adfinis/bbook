package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

type ctxKey struct{}

// ContextWith carries attrs that the default logger adds to every record logged with this context.
func ContextWith(ctx context.Context, attrs ...slog.Attr) context.Context {
	existing, _ := ctx.Value(ctxKey{}).([]slog.Attr)
	return context.WithValue(ctx, ctxKey{}, append(existing[:len(existing):len(existing)], attrs...))
}

type contextHandler struct{ slog.Handler }

func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if attrs, ok := ctx.Value(ctxKey{}).([]slog.Attr); ok {
		r.AddAttrs(attrs...)
	}
	return h.Handler.Handle(ctx, r)
}

// Setup installs the default logger. format is "json" or "text"; level is one of
// debug/info/warn/error (empty or unknown keeps the info default).
func Setup(format, level string) {
	opts := &slog.HandlerOptions{Level: parseLevel(level)}
	var base slog.Handler
	if strings.EqualFold(format, "json") {
		base = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		base = slog.NewTextHandler(os.Stderr, opts)
	}
	slog.SetDefault(slog.New(contextHandler{base}))
}

func parseLevel(level string) slog.Level {
	var l slog.Level // zero value is LevelInfo
	_ = l.UnmarshalText([]byte(level))
	return l
}
