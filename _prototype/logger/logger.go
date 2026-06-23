package logger

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var Log zerolog.Logger

func InitLogger() {
	logDir := filepath.Join(os.TempDir(), "goshareit")
	_ = os.MkdirAll(logDir, 0755)

	logPath := filepath.Join(logDir, "goshareit.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		panic("Unable to open log file: " + err.Error())
	}

	consoleWriter := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	multi := zerolog.MultiLevelWriter(consoleWriter, logFile)

	Log = zerolog.New(multi).With().Timestamp().Logger()

	SetLogLevel("INFO")

	log.Info().Msg("GoShareIt logging initialized")
}

func CloseLogger() {
	log.Info().Msg("Shutting down GoShareIt logger")
}

func SetLogLevel(level string) {
	level = strings.ToUpper(level)
	switch level {
	case "DEBUG":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "INFO":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "WARN":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "ERROR":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	case "FATAL":
		zerolog.SetGlobalLevel(zerolog.FatalLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
	log.Debug().Str("logLevel", level).Msg("Log level set")
}

func Info(msg string) {
	Log.Info().Msg(msg)
}

func Debug(msg string) {
	Log.Debug().Msg(msg)
}

func Warn(msg string) {
	Log.Warn().Msg(msg)
}

func Error(msg string) {
	Log.Error().Msg(msg)
}

func Fatal(msg string) {
	Log.Fatal().Msg(msg)
}
