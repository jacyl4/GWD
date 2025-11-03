package core

// Logger abstracts the logging methods used by the downloader packages.
type Logger interface {
	Debug(format string, args ...interface{})
	Info(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Success(format string, args ...interface{})
	Progress(operation string)
	ProgressDone(operation string)
}
