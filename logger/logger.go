package logger

import (
	"fmt"
	"os"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/op/go-logging"
)

var (
	logger      *logging.Logger
	logBufferMu sync.RWMutex
	logBuffer   []struct {
		time  string
		level logging.Level
		log   string
	}
	logBufferBytes int
)

const (
	logBufferMaxEntries = 10240
	logBufferMaxBytes   = 4 * 1024 * 1024
	logEntryMaxBytes    = 16 * 1024
)

func InitLogger(level logging.Level) {
	newLogger := logging.MustGetLogger("kwor")
	var err error
	var backend logging.Backend
	var format logging.Formatter

	backend, err = logging.NewSyslogBackend("")
	if err != nil {
		fmt.Println("Unable to use syslog: " + err.Error())
		backend = logging.NewLogBackend(os.Stderr, "", 0)
	}
	if err != nil {
		format = logging.MustStringFormatter(`%{time:2006/01/02 15:04:05} %{level} - %{message}`)
	} else {
		format = logging.MustStringFormatter(`%{level} - %{message}`)
	}

	backendFormatter := logging.NewBackendFormatter(backend, format)
	backendLeveled := logging.AddModuleLevel(backendFormatter)
	backendLeveled.SetLevel(level, "kwor")
	newLogger.SetBackend(backendLeveled)

	logger = newLogger
}

func GetLogger() *logging.Logger {
	return logger
}

func Debug(args ...interface{}) {
	if logger != nil {
		logger.Debug(args...)
	}
	addToBuffer("DEBUG", fmt.Sprint(args...))
}

func Debugf(format string, args ...interface{}) {
	if logger != nil {
		logger.Debugf(format, args...)
	}
	addToBuffer("DEBUG", fmt.Sprintf(format, args...))
}

func Info(args ...interface{}) {
	if logger != nil {
		logger.Info(args...)
	}
	addToBuffer("INFO", fmt.Sprint(args...))
}

func Infof(format string, args ...interface{}) {
	if logger != nil {
		logger.Infof(format, args...)
	}
	addToBuffer("INFO", fmt.Sprintf(format, args...))
}

func Warning(args ...interface{}) {
	if logger != nil {
		logger.Warning(args...)
	}
	addToBuffer("WARNING", fmt.Sprint(args...))
}

func Warningf(format string, args ...interface{}) {
	if logger != nil {
		logger.Warningf(format, args...)
	}
	addToBuffer("WARNING", fmt.Sprintf(format, args...))
}

func Error(args ...interface{}) {
	if logger != nil {
		logger.Error(args...)
	}
	addToBuffer("ERROR", fmt.Sprint(args...))
}

func Errorf(format string, args ...interface{}) {
	if logger != nil {
		logger.Errorf(format, args...)
	}
	addToBuffer("ERROR", fmt.Sprintf(format, args...))
}

func addToBuffer(level string, newLog string) {
	newLog = truncateLogEntry(newLog, logEntryMaxBytes)
	t := time.Now()
	logLevel, _ := logging.LogLevel(level)
	entry := struct {
		time  string
		level logging.Level
		log   string
	}{
		time:  t.Format("2006/01/02 15:04:05"),
		level: logLevel,
		log:   newLog,
	}

	logBufferMu.Lock()
	defer logBufferMu.Unlock()
	for len(logBuffer) >= logBufferMaxEntries ||
		(logBufferBytes+len(entry.log) > logBufferMaxBytes && len(logBuffer) > 0) {
		logBufferBytes -= len(logBuffer[0].log)
		logBuffer = logBuffer[1:]
	}
	logBuffer = append(logBuffer, entry)
	logBufferBytes += len(entry.log)
}

func truncateLogEntry(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	const suffix = "..."
	if maxBytes <= len(suffix) {
		return suffix[:maxBytes]
	}
	cut := maxBytes - len(suffix)
	for cut > 0 && !utf8.ValidString(value[:cut]) {
		cut--
	}
	return value[:cut] + suffix
}

func GetLogs(c int, level string) []string {
	if c <= 0 {
		return []string{}
	}
	if c > logBufferMaxEntries {
		c = logBufferMaxEntries
	}
	logLevel, _ := logging.LogLevel(level)

	logBufferMu.RLock()
	defer logBufferMu.RUnlock()
	if c > len(logBuffer) {
		c = len(logBuffer)
	}
	output := make([]string, 0, c)
	for i := len(logBuffer) - 1; i >= 0 && len(output) < c; i-- {
		if logBuffer[i].level <= logLevel {
			output = append(output, fmt.Sprintf("%s %s - %s", logBuffer[i].time, logBuffer[i].level, logBuffer[i].log))
		}
	}
	return output
}
