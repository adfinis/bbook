package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// returns a logger writing JSON records through contextHandler into buf.
func newBufLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(contextHandler{slog.NewJSONHandler(buf, nil)})
}

func logLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &m), "log line %q", buf.String())
	return m
}

func TestContextWithAttrsAppearInOutput(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	ctx := ContextWith(context.Background(), slog.String("request_id", "abc123"))
	newBufLogger(&buf).InfoContext(ctx, "hello")
	m := logLine(t, &buf)
	assert.Equal(t, "abc123", m["request_id"])
	assert.Equal(t, "hello", m["msg"])
}

func TestContextWithChildDoesNotMutateSibling(t *testing.T) {
	t.Parallel()
	parent := ContextWith(context.Background(), slog.String("base", "yes"))
	child1 := ContextWith(parent, slog.String("child", "one"))
	child2 := ContextWith(parent, slog.String("child", "two"))

	var buf1, buf2 bytes.Buffer
	newBufLogger(&buf1).InfoContext(child1, "m1")
	newBufLogger(&buf2).InfoContext(child2, "m2")

	m1 := logLine(t, &buf1)
	m2 := logLine(t, &buf2)
	assert.Equal(t, "yes", m1["base"])
	assert.Equal(t, "one", m1["child"])
	assert.Equal(t, "yes", m2["base"])
	assert.Equal(t, "two", m2["child"])
}

func TestSetupLevels(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	ctx := context.Background()

	Setup("json", "debug")
	assert.True(t, slog.Default().Enabled(ctx, slog.LevelDebug), "debug should be enabled after Setup with level debug")

	Setup("text", "warn")
	assert.False(t, slog.Default().Enabled(ctx, slog.LevelInfo), "info should be disabled after Setup with level warn")
	assert.True(t, slog.Default().Enabled(ctx, slog.LevelWarn), "warn should be enabled after Setup with level warn")
}
