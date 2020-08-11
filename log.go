package main

import (
	"io"

	gommonlog "github.com/labstack/gommon/log"
	"github.com/sirupsen/logrus"
)

// Logger defines our implementation of Labstack Echo's logger API.
type Logger struct {
	Entry *logrus.Entry
}

// Output returns the output writer associated with the logger.
func (l Logger) Output() io.Writer {
	return l.Entry.Logger.Out
}

// SetOutput sets the output writer associated with the logger.
func (l Logger) SetOutput(writer io.Writer) {
	l.Entry.Logger.SetOutput(writer)
}

// Prefix returns the prefix associated with the logger. This is a no-op.
func (l Logger) Prefix() string {
	return ""
}

// SetPrefix sets the prefix associated with the logger. This is a no-op.
func (l Logger) SetPrefix(_ string) {
}

// Level returns the level associated with the logger. The most restrictive logging level supported by
// labstack/echo's Logger interface is ERROR. The least restrictive logging level that it supports is
// DEBUG.
func (l Logger) Level() gommonlog.Lvl {
	if l.Entry.Level == logrus.PanicLevel {
		return gommonlog.ERROR
	} else if l.Entry.Logger.Level == logrus.FatalLevel {
		return gommonlog.ERROR
	} else if l.Entry.Logger.Level == logrus.ErrorLevel {
		return gommonlog.ERROR
	} else if l.Entry.Logger.Level == logrus.WarnLevel {
		return gommonlog.WARN
	} else if l.Entry.Logger.Level == logrus.InfoLevel {
		return gommonlog.INFO
	} else {
		return gommonlog.DEBUG
	}
}

// SetLevel sets the level associated with the logger. The most restrictive logging level supported by
// labstack/echo's Logger interface is ERROR. The least restrictive logging level that it supports is
// DEBUG.
func (l Logger) SetLevel(lvl gommonlog.Lvl) {
	if lvl == gommonlog.ERROR {
		l.Entry.Logger.SetLevel(logrus.ErrorLevel)
	} else if lvl == gommonlog.WARN {
		l.Entry.Logger.SetLevel(logrus.WarnLevel)
	} else if lvl == gommonlog.INFO {
		l.Entry.Logger.SetLevel(logrus.InfoLevel)
	} else {
		l.Entry.Logger.SetLevel(logrus.DebugLevel)
	}
}

// SetHeader adds header fields to log messages. This is a no-op.
func (l Logger) SetHeader(_ string) {
}

// Print prints a log message.
func (l Logger) Print(args ...interface{}) {
	l.Entry.Print(args...)
}

// Printf prints a formatted log message.
func (l Logger) Printf(format string, args ...interface{}) {
	l.Entry.Printf(format, args...)
}

// Printj prints a JSON log message. This is currently a no-op.
func (l Logger) Printj(_ gommonlog.JSON) {
}

// Debug prints a log message at DEBUG level.
func (l Logger) Debug(args ...interface{}) {
	l.Entry.Debug(args...)
}

// Debugf prints a formatted log message at DEBUG level.
func (l Logger) Debugf(format string, args ...interface{}) {
	l.Entry.Debugf(format, args...)
}

// Debugj prints a JSON log message at DEBUG level. This is currently a no-op.
func (l Logger) Debugj(_ gommonlog.JSON) {
}

// Info prints a log message at INFO level.
func (l Logger) Info(args ...interface{}) {
	l.Entry.Info(args...)
}

// Infof prints a formatted log message at INFO level.
func (l Logger) Infof(format string, args ...interface{}) {
	l.Entry.Infof(format, args...)
}

// Infoj prints a JSON log message at INFO level. This is currently a no-op.
func (l Logger) Infoj(_ gommonlog.JSON) {
}

// Warn prints a log message at WARN level.
func (l Logger) Warn(args ...interface{}) {
	l.Entry.Warn(args...)
}

// Warnf prints a formatted log message at WARN level.
func (l Logger) Warnf(format string, args ...interface{}) {
	l.Entry.Warnf(format, args...)
}

// Warnj prints a JSON log satement at WARN level. This is currently a no-op.
func (l Logger) Warnj(_ gommonlog.JSON) {
}

// Error prints a log message at ERROR level.
func (l Logger) Error(args ...interface{}) {
	l.Entry.Error(args...)
}

// Errorf prints a formatted log message at ERROR level.
func (l Logger) Errorf(format string, args ...interface{}) {
	l.Entry.Errorf(format, args...)
}

// Errorj prints a JSON log message at ERROR level. This is currently a no-op.
func (l Logger) Errorj(_ gommonlog.JSON) {
}

// Panic prints a log message at PANIC level and exits.
func (l Logger) Panic(args ...interface{}) {
	l.Entry.Panic(args...)
}

// Panicf prints a formatted log message at PANIC level and exits.
func (l Logger) Panicf(format string, args ...interface{}) {
	l.Entry.Panicf(format, args...)
}

// Panicj prints a JSON log message at PANIC level and exits. This is currently a no-op.
func (l Logger) Panicj(_ gommonlog.JSON) {
}

// Fatal prints a log message at FATAL level and exits.
func (l Logger) Fatal(args ...interface{}) {
	l.Entry.Fatal(args...)
}

// Fatalf prints a formatted log message at FATAL level and exits.
func (l Logger) Fatalf(format string, args ...interface{}) {
	l.Entry.Fatalf(format, args...)
}

// Fatalj prints a JSON log message at FATAL level and exits. this is currently a no-op.
func (l Logger) Fatalj(_ gommonlog.JSON) {
}
