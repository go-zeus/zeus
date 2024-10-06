package log

import (
	l "log"
)

func Debug(format string, args ...any) {
	l.Printf(format, args...)
}

func Info(format string, args ...any) {
	l.Printf(format, args...)
}

func Error(format string, args ...any) {
	l.Printf(format, args...)
}

func Panic(format string, args ...any) {
	l.Printf(format, args...)
}

func Fatal(v ...interface{}) {
	l.Fatal(v...)
}
