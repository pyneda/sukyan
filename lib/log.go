package lib

import (
	"io"
	"os"
	"runtime"

	"github.com/mattn/go-colorable"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	LogTimeFormat = "2006-01-02T15:04:05.000"
)

func ZeroConsoleLog() {
	// zerolog.TimeFieldFormat = LogTimeFormat
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	sysType := runtime.GOOS

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, NoColor: false, TimeFormat: LogTimeFormat})

	if sysType == "windows" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: colorable.NewColorableStdout(), TimeFormat: LogTimeFormat})
	}
}

// ZeroConsoleAndFileLog
func ZeroConsoleAndFileLog(filename string) {
	// zerolog.TimeFieldFormat = LogTimeFormat
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	sysType := runtime.GOOS

	var logFile *os.File
	var err error
	logFile, err = os.OpenFile(filename, os.O_WRONLY|os.O_APPEND, 0666)

	if !LocalFileExists(filename) {
		logFile, err = os.Create(filename)
	} else {
		logFile, err = os.OpenFile(filename, os.O_WRONLY|os.O_APPEND, 0666)
	}
	if err != nil {
		log.Error().Err(err).Msg("Error setting up log config")
	}

	var consoleLog zerolog.ConsoleWriter = zerolog.ConsoleWriter{Out: os.Stdout, NoColor: false, TimeFormat: LogTimeFormat}
	if sysType == "windows" {
		consoleLog = zerolog.ConsoleWriter{Out: colorable.NewColorableStdout(), TimeFormat: LogTimeFormat}
	}

	var writers []io.Writer
	writers = append(writers, logFile)
	writers = append(writers, consoleLog)
	mw := io.MultiWriter(writers...)

	log.Logger = zerolog.New(mw).With().Timestamp().Logger()
}
