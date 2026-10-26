package logger

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Field aliases for zap fields
type Field = zapcore.Field

// Helper functions for creating fields
var (
	// String creates a field with a string value
	String = zap.String
	// Int creates a field with an int value
	Int = zap.Int
	// Int64 creates a field with an int64 value
	Int64 = zap.Int64
	// Float64 creates a field with a float64 value
	Float64 = zap.Float64
	// Bool creates a field with a bool value
	Bool = zap.Bool
	// Time creates a field with a time.Time value
	Time = zap.Time
	// Duration creates a field with a time.Duration value
	Duration = zap.Duration
	// Error creates a field with an error value
	Error = zap.Error
	// Any creates a field with any value
	Any = zap.Any
)

// Logger is a wrapper around zap.Logger
type Logger struct {
	*zap.Logger
}

// Config represents logger configuration
type Config struct {
	Level  string // debug, info, warn, error
	Format string // json, console

	// File, when set, sends the log to a rotating file as well as to stdout.
	// Upstream logs to stdout only, which leaves the size of whatever it is
	// redirected into entirely unbounded. See rotating.go.
	File      string
	MaxSizeMB int // ceiling per file; 0 means 100
	MaxFiles  int // files kept, oldest deleted first; 0 means 7
}

// Custom level encoder that adds colors for console output
func coloredLevelEncoder(level zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	switch level {
	case zapcore.ErrorLevel:
		enc.AppendString("\033[1;31m" + level.String() + "\033[0m") // Bold Red
	case zapcore.WarnLevel:
		enc.AppendString("\033[1;33m" + level.String() + "\033[0m") // Bold Yellow
	case zapcore.InfoLevel:
		enc.AppendString("\033[1;36m" + level.String() + "\033[0m") // Bold Cyan
	case zapcore.DebugLevel:
		enc.AppendString("\033[1;37m" + level.String() + "\033[0m") // Bold White
	default:
		enc.AppendString(level.String())
	}
}

// Custom name encoder that truncates or pads the logger name to a fixed width
func fixedWidthNameEncoder(loggerName string, enc zapcore.PrimitiveArrayEncoder) {
	// Extract the last component of the logger name for shorter display
	parts := strings.Split(loggerName, ".")
	displayName := parts[len(parts)-1]

	// Limit to 15 chars and pad with spaces to align columns
	if len(displayName) > 15 {
		displayName = displayName[:15]
	} else if len(displayName) < 15 {
		displayName = displayName + strings.Repeat(" ", 15-len(displayName))
	}

	enc.AppendString(displayName)
}

// New creates a new logger with the given configuration
func New(config Config) (*Logger, error) {
	// Parse log level
	level, err := parseLogLevel(config.Level)
	if err != nil {
		return nil, err
	}

	// Create encoder config
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
	}

	// Set encoding options based on format
	if config.Format == "console" {
		encoderConfig.EncodeLevel = coloredLevelEncoder
		encoderConfig.EncodeName = fixedWidthNameEncoder
	} else {
		encoderConfig.EncodeLevel = zapcore.LowercaseLevelEncoder
		encoderConfig.EncodeName = zapcore.FullNameEncoder
	}

	// Only include caller info for debug level
	// For other levels, omit the caller info
	if level != zapcore.DebugLevel {
		encoderConfig.CallerKey = zapcore.OmitKey
	} else {
		encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
	}

	// Create encoder based on format
	var encoder zapcore.Encoder
	switch config.Format {
	case "json":
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	case "console":
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	default:
		return nil, fmt.Errorf("unsupported log format: %s", config.Format)
	}

	// An atomic level so the threshold can be raised or lowered while the server
	// runs. Turning debug on to look at something, then off again, should not need
	// a restart -- a restart drops the audio stream and the ADS-B history window.
	atomic := zap.NewAtomicLevelAt(level)
	setLevel(atomic)

	// Stdout always: a server run from a terminal must still print. The file is
	// added beside it, never instead of it.
	sink := zapcore.AddSync(os.Stdout)
	if config.File != "" {
		rot, err := newRotatingFile(config.File, config.MaxSizeMB, config.MaxFiles)
		if err != nil {
			return nil, err
		}
		sink = zapcore.NewMultiWriteSyncer(sink, zapcore.AddSync(rot))
		currentFile.Store(rot)
	}

	// Create core
	core := zapcore.NewCore(
		encoder,
		sink,
		atomic,
	)

	// Create logger options
	opts := []zap.Option{
		zap.AddStacktrace(zapcore.ErrorLevel),
	}

	// Only add caller info for debug level
	if level == zapcore.DebugLevel {
		opts = append(opts, zap.AddCaller(), zap.AddCallerSkip(1))
	}

	// Create logger
	logger := zap.New(core, opts...)

	return &Logger{Logger: logger}, nil
}

// parseLogLevel parses the log level string
func parseLogLevel(level string) (zapcore.Level, error) {
	switch level {
	case "debug":
		return zapcore.DebugLevel, nil
	case "info":
		return zapcore.InfoLevel, nil
	case "warn":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	default:
		return zapcore.InfoLevel, fmt.Errorf("unsupported log level: %s", level)
	}
}

// With returns a logger with the given fields
func (l *Logger) With(fields ...zapcore.Field) *Logger {
	return &Logger{Logger: l.Logger.With(fields...)}
}

// Named returns a logger with the given name
func (l *Logger) Named(name string) *Logger {
	return &Logger{Logger: l.Logger.Named(name)}
}

// The process-wide log level, held atomically so it can be changed at runtime.
// Package scope because every Logger built by New shares one core threshold, and a
// caller changing the level means "make the server quieter", not "make this one
// logger quieter".
var currentLevel atomic.Pointer[zap.AtomicLevel]

func setLevel(l zap.AtomicLevel) { currentLevel.Store(&l) }

// Level reports the threshold in force, as it is written in the configuration.
func Level() string {
	if l := currentLevel.Load(); l != nil {
		return l.Level().String()
	}
	return ""
}

// SetLevel changes the threshold in force. The caller-visible names are those of
// the configuration file, so a value read from Level can be handed back unchanged.
func SetLevel(name string) error {
	l := currentLevel.Load()
	if l == nil {
		return fmt.Errorf("no logger has been created yet")
	}
	parsed, err := parseLogLevel(name)
	if err != nil {
		return err
	}
	l.SetLevel(parsed)
	return nil
}

// The file the server's log is written to, for the launcher's health
// (docs-fr/36): the launcher rebuilt its name from the configuration and the
// date, and missed a file cut by size. The last logger built with a file.
var currentFile atomic.Pointer[rotatingFile]

// File is the log file being written now -- the day's, or its next part once
// it outgrew its ceiling -- or "" when the log goes to stdout alone.
func File() string {
	if r := currentFile.Load(); r != nil {
		return r.current()
	}
	return ""
}
