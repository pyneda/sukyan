package lib

import (
	"io"
	"os"
	"runtime"

	"github.com/mattn/go-colorable"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

const (
	LogTimeFormat = "2006-01-02T15:04:05.000"
)

func ZeroConsoleLog() zerolog.Logger {
	// zerolog.TimeFieldFormat = LogTimeFormat
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	sysType := runtime.GOOS

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, NoColor: false, TimeFormat: LogTimeFormat})

	if sysType == "windows" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: colorable.NewColorableStdout(), TimeFormat: LogTimeFormat})
	}
	return log.Logger
}

// ZeroConsoleAndFileLog
func ZeroConsoleAndFileLog() zerolog.Logger {
	// zerolog.TimeFieldFormat = LogTimeFormat
	filename := viper.GetString("logging.file.path")
	if filename == "" {
		filename = "sukyan.log"
	}
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	sysType := runtime.GOOS

	logFile, err := os.OpenFile(filename, os.O_WRONLY|os.O_APPEND, 0666)

	if !LocalFileExists(filename) {
		logFile, err = os.Create(filename)
	} else {
		logFile, err = os.OpenFile(filename, os.O_WRONLY|os.O_APPEND, 0666)
	}
	if err != nil {
		log.Error().Err(err).Msg("Error setting up log config")
	}

	// var consoleLog zerolog.ConsoleWriter = zerolog.ConsoleWriter{Out: os.Stdout, NoColor: false, TimeFormat: LogTimeFormat}
	// if sysType == "windows" {
	// 	consoleLog = zerolog.ConsoleWriter{Out: colorable.NewColorableStdout(), TimeFormat: LogTimeFormat}
	// }
	var writers []io.Writer

	if viper.GetString("logging.console.format") == "pretty" {
		var consoleLog zerolog.ConsoleWriter
		if sysType == "windows" {
			consoleLog = zerolog.ConsoleWriter{Out: colorable.NewColorableStdout(), TimeFormat: LogTimeFormat}
		} else {
			consoleLog = zerolog.ConsoleWriter{Out: os.Stdout, NoColor: false, TimeFormat: LogTimeFormat}

		}
		writers = append(writers, consoleLog)
	} else {
		writers = append(writers, os.Stdout)
	}

	if viper.GetBool("logging.file.enabled") {
		writers = append(writers, logFile)
	}
	mw := io.MultiWriter(writers...)
	logger := zerolog.New(mw).With().Timestamp().Logger()
	log.Logger = logger
	return logger

}
