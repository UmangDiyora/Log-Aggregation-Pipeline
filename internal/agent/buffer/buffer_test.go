package buffer

import (
	"testing"

	"github.com/UmangDiyora/logpipeline/pkg/models"
)

func TestMemoryBuffer(t *testing.T) {
	buf := NewMemoryBuffer(100, "/tmp/test-buffer")
	defer buf.Close()

	// Test Add
	entry := models.NewLogEntry()
	entry.ID = "test-1"
	entry.Message = "test message"

	if err := buf.Add(entry); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Test Size
	if buf.Size() != 1 {
		t.Errorf("Expected size 1, got %d", buf.Size())
	}

	// Test Get
	entries, err := buf.Get(10)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(entries))
	}

	if entries[0].ID != "test-1" {
		t.Errorf("Expected ID test-1, got %s", entries[0].ID)
	}

	// Test Remove
	if err := buf.Remove([]string{"test-1"}); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	if buf.Size() != 0 {
		t.Errorf("Expected size 0 after remove, got %d", buf.Size())
	}
}

func TestMemoryBufferBatch(t *testing.T) {
	buf := NewMemoryBuffer(100, "/tmp/test-buffer")
	defer buf.Close()

	entries := make([]*models.LogEntry, 10)
	for i := 0; i < 10; i++ {
		entries[i] = models.NewLogEntry()
		entries[i].ID = string(rune('a' + i))
	}

	if err := buf.AddBatch(entries); err != nil {
		t.Fatalf("AddBatch failed: %v", err)
	}

	if buf.Size() != 10 {
		t.Errorf("Expected size 10, got %d", buf.Size())
	}
}

func BenchmarkMemoryBufferAdd(b *testing.B) {
	buf := NewMemoryBuffer(100000, "/tmp/bench-buffer")
	defer buf.Close()

	entry := models.NewLogEntry()
	entry.Message = "benchmark test"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Add(entry)
	}
}

func BenchmarkMemoryBufferGet(b *testing.B) {
	buf := NewMemoryBuffer(100000, "/tmp/bench-buffer")
	defer buf.Close()

	// Pre-populate
	for i := 0; i < 1000; i++ {
		entry := models.NewLogEntry()
		buf.Add(entry)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Get(100)
	}
}
