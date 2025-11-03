package logger

// NewLogger retains the previous constructor name for coloured loggers.
func NewLogger(options ...Option) *ColoredLogger {
	return NewColoredLogger(options...)
}
