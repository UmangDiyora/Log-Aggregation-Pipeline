package parser

import (
	"testing"

	"github.com/UmangDiyora/logpipeline/pkg/models"
)

func TestJSONParser(t *testing.T) {
	parser := NewJSONParser(nil)

	entry := models.NewLogEntry()
	entry.Raw = `{"level":"ERROR","message":"test error","timestamp":"2024-01-01T12:00:00Z","custom":"value"}`

	if err := parser.Parse(entry); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if entry.Message != "test error" {
		t.Errorf("Expected message 'test error', got '%s'", entry.Message)
	}

	if entry.Level != models.LogLevelError {
		t.Errorf("Expected level ERROR, got %s", entry.Level)
	}

	if val, ok := entry.GetField("custom"); !ok || val != "value" {
		t.Errorf("Custom field not parsed correctly")
	}
}

func TestJSONParserInvalidJSON(t *testing.T) {
	parser := NewJSONParser(nil)

	entry := models.NewLogEntry()
	entry.Raw = `{invalid json}`

	if err := parser.Parse(entry); err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestRegexParser(t *testing.T) {
	config := &Config{
		Patterns: map[string]string{
			"pattern": `^(?P<timestamp>\S+) (?P<level>\w+) (?P<message>.+)$`,
		},
	}

	parser, err := NewRegexParser(config)
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}

	entry := models.NewLogEntry()
	entry.Raw = "2024-01-01T12:00:00Z ERROR something went wrong"

	if err := parser.Parse(entry); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if entry.Message != "something went wrong" {
		t.Errorf("Expected message 'something went wrong', got '%s'", entry.Message)
	}

	if entry.Level != models.LogLevelError {
		t.Errorf("Expected level ERROR, got %s", entry.Level)
	}
}

func TestNginxParser(t *testing.T) {
	parser := NewNginxParser()

	entry := models.NewLogEntry()
	entry.Raw = `192.168.1.1 - user1 [01/Jan/2024:12:00:00 +0000] "GET /api/users HTTP/1.1" 200 1234 "https://example.com" "Mozilla/5.0"`

	if err := parser.Parse(entry); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if val, ok := entry.GetField("remote_addr"); !ok || val != "192.168.1.1" {
		t.Error("Remote address not parsed correctly")
	}

	if val, ok := entry.GetField("request_method"); !ok || val != "GET" {
		t.Error("Request method not parsed correctly")
	}

	if val, ok := entry.GetField("status"); !ok || val != "200" {
		t.Error("Status not parsed correctly")
	}

	if entry.Level != models.LogLevelInfo {
		t.Errorf("Expected level INFO for 200 status, got %s", entry.Level)
	}
}

func TestNginxParserErrorStatus(t *testing.T) {
	parser := NewNginxParser()

	entry := models.NewLogEntry()
	entry.Raw = `192.168.1.1 - user1 [01/Jan/2024:12:00:00 +0000] "GET /api/error HTTP/1.1" 500 1234 "-" "Mozilla/5.0"`

	if err := parser.Parse(entry); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if entry.Level != models.LogLevelError {
		t.Errorf("Expected level ERROR for 500 status, got %s", entry.Level)
	}
}

func BenchmarkJSONParser(b *testing.B) {
	parser := NewJSONParser(nil)
	entry := models.NewLogEntry()
	entry.Raw = `{"level":"INFO","message":"benchmark test"}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser.Parse(entry)
	}
}

func BenchmarkNginxParser(b *testing.B) {
	parser := NewNginxParser()
	entry := models.NewLogEntry()
	entry.Raw = `192.168.1.1 - - [01/Jan/2024:12:00:00 +0000] "GET / HTTP/1.1" 200 1234 "-" "Mozilla/5.0"`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser.Parse(entry)
	}
}
