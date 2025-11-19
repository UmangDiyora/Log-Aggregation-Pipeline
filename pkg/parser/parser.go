package parser

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/UmangDiyora/logpipeline/pkg/models"
)

// Parser is the interface for log parsers
type Parser interface {
	// Parse parses a log entry
	Parse(entry *models.LogEntry) error

	// Name returns the parser name
	Name() string
}

// Config holds parser configuration
type Config struct {
	// Type is the parser type
	Type string

	// Fields to extract
	Fields map[string]string

	// Patterns for regex/grok parsing
	Patterns map[string]string

	// TimeFormat for timestamp parsing
	TimeFormat string

	// TimeField is the field containing the timestamp
	TimeField string
}

// JSONParser parses JSON logs
type JSONParser struct {
	config *Config
}

// NewJSONParser creates a new JSON parser
func NewJSONParser(config *Config) *JSONParser {
	return &JSONParser{config: config}
}

// Parse parses a JSON log entry
func (p *JSONParser) Parse(entry *models.LogEntry) error {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(entry.Raw), &data); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Extract fields
	for key, value := range data {
		entry.AddField(key, value)
	}

	// Extract message
	if msg, ok := data["message"].(string); ok {
		entry.Message = msg
	} else if msg, ok := data["msg"].(string); ok {
		entry.Message = msg
	}

	// Extract level
	if level, ok := data["level"].(string); ok {
		entry.Level = p.parseLevel(level)
	}

	// Extract timestamp
	if p.config != nil && p.config.TimeField != "" {
		if ts, ok := data[p.config.TimeField]; ok {
			if timestamp, err := p.parseTimestamp(ts); err == nil {
				entry.Timestamp = timestamp
			}
		}
	}

	return nil
}

// Name returns the parser name
func (p *JSONParser) Name() string {
	return "json"
}

// parseLevel converts string to LogLevel
func (p *JSONParser) parseLevel(level string) models.LogLevel {
	level = strings.ToUpper(level)
	switch level {
	case "DEBUG", "DBG", "TRACE":
		return models.LogLevelDebug
	case "INFO", "INFORMATION":
		return models.LogLevelInfo
	case "WARN", "WARNING":
		return models.LogLevelWarn
	case "ERROR", "ERR":
		return models.LogLevelError
	case "FATAL", "CRITICAL", "PANIC":
		return models.LogLevelFatal
	default:
		return models.LogLevelInfo
	}
}

// parseTimestamp parses a timestamp value
func (p *JSONParser) parseTimestamp(value interface{}) (time.Time, error) {
	switch v := value.(type) {
	case string:
		if p.config != nil && p.config.TimeFormat != "" {
			return time.Parse(p.config.TimeFormat, v)
		}
		// Try common formats
		formats := []string{
			time.RFC3339,
			time.RFC3339Nano,
			"2006-01-02T15:04:05.000Z07:00",
			"2006-01-02 15:04:05",
		}
		for _, format := range formats {
			if t, err := time.Parse(format, v); err == nil {
				return t, nil
			}
		}
		return time.Time{}, fmt.Errorf("failed to parse timestamp")
	case float64:
		// Unix timestamp
		return time.Unix(int64(v), 0), nil
	default:
		return time.Time{}, fmt.Errorf("unsupported timestamp type")
	}
}

// RegexParser parses logs using regular expressions
type RegexParser struct {
	config  *Config
	pattern *regexp.Regexp
}

// NewRegexParser creates a new regex parser
func NewRegexParser(config *Config) (*RegexParser, error) {
	if config == nil || config.Patterns == nil {
		return nil, fmt.Errorf("regex pattern is required")
	}

	patternStr, ok := config.Patterns["pattern"]
	if !ok {
		return nil, fmt.Errorf("pattern not found in config")
	}

	pattern, err := regexp.Compile(patternStr)
	if err != nil {
		return nil, fmt.Errorf("failed to compile pattern: %w", err)
	}

	return &RegexParser{
		config:  config,
		pattern: pattern,
	}, nil
}

// Parse parses a log entry using regex
func (p *RegexParser) Parse(entry *models.LogEntry) error {
	matches := p.pattern.FindStringSubmatch(entry.Raw)
	if matches == nil {
		return fmt.Errorf("pattern did not match")
	}

	names := p.pattern.SubexpNames()
	for i, name := range names {
		if i > 0 && i < len(matches) && name != "" {
			entry.AddField(name, matches[i])

			// Special handling for known fields
			switch name {
			case "message":
				entry.Message = matches[i]
			case "level":
				entry.Level = p.parseLevel(matches[i])
			case "timestamp":
				if ts, err := p.parseTimestamp(matches[i]); err == nil {
					entry.Timestamp = ts
				}
			}
		}
	}

	return nil
}

// Name returns the parser name
func (p *RegexParser) Name() string {
	return "regex"
}

// parseLevel converts string to LogLevel
func (p *RegexParser) parseLevel(level string) models.LogLevel {
	level = strings.ToUpper(level)
	switch level {
	case "DEBUG", "DBG":
		return models.LogLevelDebug
	case "INFO":
		return models.LogLevelInfo
	case "WARN", "WARNING":
		return models.LogLevelWarn
	case "ERROR", "ERR":
		return models.LogLevelError
	case "FATAL", "CRITICAL":
		return models.LogLevelFatal
	default:
		return models.LogLevelInfo
	}
}

// parseTimestamp parses a timestamp string
func (p *RegexParser) parseTimestamp(value string) (time.Time, error) {
	if p.config != nil && p.config.TimeFormat != "" {
		return time.Parse(p.config.TimeFormat, value)
	}

	// Try common formats
	formats := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006/01/02 15:04:05",
		"Jan 02 15:04:05",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, value); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("failed to parse timestamp")
}

// NginxParser parses Nginx access logs
type NginxParser struct{}

// NewNginxParser creates a new Nginx parser
func NewNginxParser() *NginxParser {
	return &NginxParser{}
}

// Parse parses an Nginx log entry
func (p *NginxParser) Parse(entry *models.LogEntry) error {
	// Nginx combined log format pattern
	pattern := regexp.MustCompile(`^(\S+) \S+ (\S+) \[([^\]]+)\] "(\S+) (\S+) (\S+)" (\d+) (\d+) "([^"]*)" "([^"]*)"`)

	matches := pattern.FindStringSubmatch(entry.Raw)
	if matches == nil || len(matches) < 11 {
		return fmt.Errorf("nginx pattern did not match")
	}

	entry.AddField("remote_addr", matches[1])
	entry.AddField("remote_user", matches[2])
	entry.AddField("time_local", matches[3])
	entry.AddField("request_method", matches[4])
	entry.AddField("request_path", matches[5])
	entry.AddField("request_protocol", matches[6])
	entry.AddField("status", matches[7])
	entry.AddField("body_bytes_sent", matches[8])
	entry.AddField("http_referer", matches[9])
	entry.AddField("http_user_agent", matches[10])

	entry.Message = fmt.Sprintf("%s %s %s - %s", matches[4], matches[5], matches[6], matches[7])

	// Parse timestamp
	if t, err := time.Parse("02/Jan/2006:15:04:05 -0700", matches[3]); err == nil {
		entry.Timestamp = t
	}

	// Set level based on status code
	if matches[7] >= "500" {
		entry.Level = models.LogLevelError
	} else if matches[7] >= "400" {
		entry.Level = models.LogLevelWarn
	} else {
		entry.Level = models.LogLevelInfo
	}

	return nil
}

// Name returns the parser name
func (p *NginxParser) Name() string {
	return "nginx"
}

// New creates a new parser based on configuration
func New(config *Config) (Parser, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}

	switch config.Type {
	case "json":
		return NewJSONParser(config), nil
	case "regex":
		return NewRegexParser(config)
	case "nginx":
		return NewNginxParser(), nil
	default:
		return nil, fmt.Errorf("unsupported parser type: %s", config.Type)
	}
}
