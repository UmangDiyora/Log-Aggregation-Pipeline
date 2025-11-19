package models

import (
	"time"
)

// LogLevel represents the severity level of a log entry
type LogLevel string

const (
	LogLevelDebug LogLevel = "DEBUG"
	LogLevelInfo  LogLevel = "INFO"
	LogLevelWarn  LogLevel = "WARN"
	LogLevelError LogLevel = "ERROR"
	LogLevelFatal LogLevel = "FATAL"
)

// LogEntry represents a single log entry in the system
type LogEntry struct {
	// ID is the unique identifier for this log entry
	ID string `json:"id"`

	// Timestamp is when the log was generated
	Timestamp time.Time `json:"timestamp"`

	// Level is the severity level of the log
	Level LogLevel `json:"level"`

	// Message is the main log message
	Message string `json:"message"`

	// Source is the identifier of the log source
	Source string `json:"source"`

	// Host is the hostname where the log was generated
	Host string `json:"host"`

	// Service is the name of the service that generated the log
	Service string `json:"service"`

	// Fields contains structured data extracted from or added to the log
	Fields map[string]interface{} `json:"fields,omitempty"`

	// Tags are metadata tags associated with this log entry
	Tags []string `json:"tags,omitempty"`

	// Raw is the original, unparsed log line
	Raw string `json:"raw"`
}

// NewLogEntry creates a new log entry with default values
func NewLogEntry() *LogEntry {
	return &LogEntry{
		Timestamp: time.Now(),
		Fields:    make(map[string]interface{}),
		Tags:      make([]string, 0),
	}
}

// AddField adds or updates a field in the log entry
func (l *LogEntry) AddField(key string, value interface{}) {
	if l.Fields == nil {
		l.Fields = make(map[string]interface{})
	}
	l.Fields[key] = value
}

// GetField retrieves a field value from the log entry
func (l *LogEntry) GetField(key string) (interface{}, bool) {
	if l.Fields == nil {
		return nil, false
	}
	val, ok := l.Fields[key]
	return val, ok
}

// AddTag adds a tag to the log entry
func (l *LogEntry) AddTag(tag string) {
	if l.Tags == nil {
		l.Tags = make([]string, 0)
	}
	l.Tags = append(l.Tags, tag)
}

// HasTag checks if the log entry has a specific tag
func (l *LogEntry) HasTag(tag string) bool {
	for _, t := range l.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// Clone creates a deep copy of the log entry
func (l *LogEntry) Clone() *LogEntry {
	clone := &LogEntry{
		ID:        l.ID,
		Timestamp: l.Timestamp,
		Level:     l.Level,
		Message:   l.Message,
		Source:    l.Source,
		Host:      l.Host,
		Service:   l.Service,
		Raw:       l.Raw,
		Fields:    make(map[string]interface{}),
		Tags:      make([]string, len(l.Tags)),
	}

	// Deep copy fields
	for k, v := range l.Fields {
		clone.Fields[k] = v
	}

	// Copy tags
	copy(clone.Tags, l.Tags)

	return clone
}
