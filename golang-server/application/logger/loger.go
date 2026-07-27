package logger

import (
	"fmt"
	"os"
	"strings"
	"time"
	"context"

	"exiro.ai/config"
	"github.com/go-chi/traceid"
	"github.com/rs/zerolog"
)

type TracingHook struct{}

func (h TracingHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	ctx := e.GetCtx()
	if ctx != nil {
		trace := traceid.FromContext(ctx)
		if trace != "" {
			e.Str("trace_id", trace)
		}
	}
}

// var Default = NewLogger()

func NewLogger(ctx context.Context) zerolog.Logger {
	if config.Ctx(ctx).ProductionMode {
		logger := zerolog.New(os.Stdout).With().Timestamp().Caller().Logger().Level(zerolog.InfoLevel)
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
		logger = logger.Hook(TracingHook{})
		return logger
	}

	// Set up zerolog with colored output
	output := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}

	output.FormatLevel = func(i any) string {
		level := strings.ToUpper(fmt.Sprintf("%s", i))
		var colorStart, colorEnd string
		switch level {
		case "INFO":
			colorStart = "\x1b[32m" // Green
		case "DEBUG":
			colorStart = "\x1b[34m" // Blue
		case "ERROR", "FATAL":
			colorStart = "\x1b[31m" // Red
		case "WARN":
			colorStart = "\x1b[33m" // Yellow
		default:
			colorStart = ""
		}
		colorEnd = "\x1b[0m"

		return fmt.Sprintf("| %s%-6s%s|", colorStart, level, colorEnd)
	}

	output.FormatMessage = func(i any) string {
		return fmt.Sprintf("[ %s ]", i)
	}
	output.FormatFieldName = func(i any) string {
		return fmt.Sprintf("%s:", i)
	}
	output.FormatFieldValue = func(i any) string {
		return strings.ToUpper(fmt.Sprintf("%s", i))
	}

	logger := zerolog.New(output).With().
		Timestamp().
		Caller().
		Logger()

	logger = logger.Hook(TracingHook{})

	return logger
}
