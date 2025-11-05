package logger

import "context"

type traceKey struct{}

// TraceContext captures request-scoped identifiers for log correlation.
type TraceContext struct {
	TraceID   string
	RequestID string
	UserID    string
}

// ContextWithTrace returns a derived context carrying the provided trace metadata.
func ContextWithTrace(ctx context.Context, trace TraceContext) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return context.WithValue(ctx, traceKey{}, trace)
}

// TraceFromContext extracts a TraceContext from ctx.
func TraceFromContext(ctx context.Context) TraceContext {
	if ctx == nil {
		return TraceContext{}
	}
	if trace, ok := ctx.Value(traceKey{}).(TraceContext); ok {
		return trace
	}
	return TraceContext{}
}

func traceFieldsFromContext(ctx context.Context) []Field {
	trace := TraceFromContext(ctx)
	return trace.fields()
}

func (t TraceContext) fields() []Field {
	var fields []Field
	if t.TraceID != "" {
		fields = append(fields, String("trace_id", t.TraceID))
	}
	if t.RequestID != "" {
		fields = append(fields, String("request_id", t.RequestID))
	}
	if t.UserID != "" {
		fields = append(fields, String("user_id", t.UserID))
	}
	return fields
}
