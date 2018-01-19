package api

// LogLevel defines the types of log level available on the api
type LogLevel string

// Logs levels available
const (
	LogLevelDebug LogLevel = "DEBUG"
	LogLevelInfo  LogLevel = "INFO"
	LogLevelWarn  LogLevel = "WARNING"
	LogLevelError LogLevel = "ERROR"
	LogLevelFatal LogLevel = "FATAL"
)

// LogEntry represents an activity record
type LogEntry struct {
	MrID         string                 `json:"mrId"`
	SpanID       string                 `json:"spanId,omitempty"`
	TraceID      string                 `json:"traceId,omitempty"`
	CreationDate int64                  `json:"creationDate"`
	RecordDate   int64                  `json:"recordDate"`
	Level        LogLevel               `json:"level,omitempty"`
	Message      string                 `json:"message,omitempty"`
	Properties   map[string]interface{} `json:"properties,omitempty"`
}
