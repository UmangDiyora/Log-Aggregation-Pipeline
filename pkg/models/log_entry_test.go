package models

import (
	"testing"
)

func TestNewLogEntry(t *testing.T) {
	entry := NewLogEntry()

	if entry == nil {
		t.Fatal("NewLogEntry returned nil")
	}

	if entry.Fields == nil {
		t.Error("Fields map is nil")
	}

	if entry.Tags == nil {
		t.Error("Tags slice is nil")
	}

	if entry.Timestamp.IsZero() {
		t.Error("Timestamp was not set")
	}
}

func TestAddField(t *testing.T) {
	entry := NewLogEntry()

	entry.AddField("key1", "value1")
	entry.AddField("key2", 123)

	if val, ok := entry.GetField("key1"); !ok || val != "value1" {
		t.Errorf("Expected key1=value1, got %v", val)
	}

	if val, ok := entry.GetField("key2"); !ok || val != 123 {
		t.Errorf("Expected key2=123, got %v", val)
	}
}

func TestAddTag(t *testing.T) {
	entry := NewLogEntry()

	entry.AddTag("tag1")
	entry.AddTag("tag2")

	if !entry.HasTag("tag1") {
		t.Error("Expected tag1 to exist")
	}

	if !entry.HasTag("tag2") {
		t.Error("Expected tag2 to exist")
	}

	if entry.HasTag("tag3") {
		t.Error("tag3 should not exist")
	}
}

func TestClone(t *testing.T) {
	original := NewLogEntry()
	original.ID = "test-id"
	original.Message = "test message"
	original.Level = LogLevelError
	original.AddField("key", "value")
	original.AddTag("tag")

	clone := original.Clone()

	if clone.ID != original.ID {
		t.Error("ID not cloned correctly")
	}

	if clone.Message != original.Message {
		t.Error("Message not cloned correctly")
	}

	if clone.Level != original.Level {
		t.Error("Level not cloned correctly")
	}

	if val, ok := clone.GetField("key"); !ok || val != "value" {
		t.Error("Fields not cloned correctly")
	}

	if !clone.HasTag("tag") {
		t.Error("Tags not cloned correctly")
	}

	// Modify clone and ensure original is not affected
	clone.AddField("key", "modified")
	if val, _ := original.GetField("key"); val == "modified" {
		t.Error("Clone modification affected original")
	}
}

func TestLogLevels(t *testing.T) {
	levels := []LogLevel{
		LogLevelDebug,
		LogLevelInfo,
		LogLevelWarn,
		LogLevelError,
		LogLevelFatal,
	}

	for _, level := range levels {
		entry := NewLogEntry()
		entry.Level = level

		if entry.Level != level {
			t.Errorf("Level not set correctly: expected %s, got %s", level, entry.Level)
		}
	}
}

func BenchmarkNewLogEntry(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NewLogEntry()
	}
}

func BenchmarkAddField(b *testing.B) {
	entry := NewLogEntry()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		entry.AddField("key", "value")
	}
}

func BenchmarkClone(b *testing.B) {
	entry := NewLogEntry()
	entry.AddField("key1", "value1")
	entry.AddField("key2", "value2")
	entry.AddTag("tag1")
	entry.AddTag("tag2")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry.Clone()
	}
}
