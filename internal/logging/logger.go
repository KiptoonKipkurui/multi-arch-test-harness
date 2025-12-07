package logging

import (
	"log/slog"
	"os"
)

var Logger *slog.Logger

func Init() {
	// Text logs with key-value pairs
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	Logger = slog.New(handler)
}
