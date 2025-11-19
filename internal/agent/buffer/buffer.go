package buffer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/UmangDiyora/logpipeline/pkg/models"
)

// Buffer represents a log buffer interface
type Buffer interface {
	// Add adds a log entry to the buffer
	Add(entry *models.LogEntry) error

	// AddBatch adds multiple log entries to the buffer
	AddBatch(entries []*models.LogEntry) error

	// Get retrieves log entries from the buffer (up to limit)
	Get(limit int) ([]*models.LogEntry, error)

	// Remove removes log entries from the buffer by their IDs
	Remove(ids []string) error

	// Size returns the current number of entries in the buffer
	Size() int

	// Close closes the buffer and releases resources
	Close() error
}

// Config holds buffer configuration
type Config struct {
	// Type is the buffer type ("memory" or "disk")
	Type string

	// MaxSize is the maximum size in bytes (for disk) or entries (for memory)
	MaxSize int64

	// Path is the directory path for disk buffer
	Path string

	// FlushInterval is how often to flush the buffer to disk
	FlushInterval time.Duration

	// MaxBatchSize is the maximum entries per batch
	MaxBatchSize int
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Type:          "memory",
		MaxSize:       1024 * 1024 * 100, // 100MB
		Path:          "/var/lib/logagent/buffer",
		FlushInterval: 5 * time.Second,
		MaxBatchSize:  1000,
	}
}

// MemoryBuffer is an in-memory ring buffer with disk overflow
type MemoryBuffer struct {
	mu         sync.RWMutex
	entries    []*models.LogEntry
	maxEntries int
	overflowed bool
	diskPath   string
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewMemoryBuffer creates a new memory buffer
func NewMemoryBuffer(maxEntries int, diskPath string) *MemoryBuffer {
	ctx, cancel := context.WithCancel(context.Background())
	return &MemoryBuffer{
		entries:    make([]*models.LogEntry, 0, maxEntries),
		maxEntries: maxEntries,
		diskPath:   diskPath,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Add adds a log entry to the buffer
func (mb *MemoryBuffer) Add(entry *models.LogEntry) error {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	// Check if buffer is full
	if len(mb.entries) >= mb.maxEntries {
		// Spill to disk
		if err := mb.spillToDisk(); err != nil {
			return fmt.Errorf("failed to spill to disk: %w", err)
		}
	}

	mb.entries = append(mb.entries, entry)
	return nil
}

// AddBatch adds multiple entries to the buffer
func (mb *MemoryBuffer) AddBatch(entries []*models.LogEntry) error {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	for _, entry := range entries {
		if len(mb.entries) >= mb.maxEntries {
			if err := mb.spillToDisk(); err != nil {
				return fmt.Errorf("failed to spill to disk: %w", err)
			}
		}
		mb.entries = append(mb.entries, entry)
	}

	return nil
}

// Get retrieves entries from the buffer
func (mb *MemoryBuffer) Get(limit int) ([]*models.LogEntry, error) {
	mb.mu.RLock()
	defer mb.mu.RUnlock()

	if limit > len(mb.entries) {
		limit = len(mb.entries)
	}

	result := make([]*models.LogEntry, limit)
	copy(result, mb.entries[:limit])

	return result, nil
}

// Remove removes entries from the buffer
func (mb *MemoryBuffer) Remove(ids []string) error {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	idMap := make(map[string]bool)
	for _, id := range ids {
		idMap[id] = true
	}

	filtered := make([]*models.LogEntry, 0, len(mb.entries))
	for _, entry := range mb.entries {
		if !idMap[entry.ID] {
			filtered = append(filtered, entry)
		}
	}

	mb.entries = filtered
	return nil
}

// Size returns the number of entries in the buffer
func (mb *MemoryBuffer) Size() int {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	return len(mb.entries)
}

// Close closes the buffer
func (mb *MemoryBuffer) Close() error {
	mb.cancel()
	return nil
}

// spillToDisk writes entries to disk when buffer is full
func (mb *MemoryBuffer) spillToDisk() error {
	if len(mb.entries) == 0 {
		return nil
	}

	// Create overflow directory
	overflowDir := filepath.Join(mb.diskPath, "overflow")
	if err := os.MkdirAll(overflowDir, 0755); err != nil {
		return err
	}

	// Write to timestamped file
	filename := filepath.Join(overflowDir, fmt.Sprintf("overflow_%d.json", time.Now().UnixNano()))
	data, err := json.Marshal(mb.entries)
	if err != nil {
		return err
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return err
	}

	mb.overflowed = true
	mb.entries = mb.entries[:0] // Clear the buffer

	return nil
}

// DiskBuffer is a disk-based WAL buffer
type DiskBuffer struct {
	mu           sync.RWMutex
	path         string
	currentFile  *os.File
	maxSize      int64
	currentSize  int64
	entries      []*models.LogEntry
	ctx          context.Context
	cancel       context.CancelFunc
	flushTicker  *time.Ticker
	maxBatchSize int
}

// NewDiskBuffer creates a new disk-based buffer
func NewDiskBuffer(path string, maxSize int64, flushInterval time.Duration, maxBatchSize int) (*DiskBuffer, error) {
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create buffer directory: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	db := &DiskBuffer{
		path:         path,
		maxSize:      maxSize,
		entries:      make([]*models.LogEntry, 0, maxBatchSize),
		ctx:          ctx,
		cancel:       cancel,
		flushTicker:  time.NewTicker(flushInterval),
		maxBatchSize: maxBatchSize,
	}

	// Start background flusher
	go db.backgroundFlush()

	return db, nil
}

// Add adds a log entry to the buffer
func (db *DiskBuffer) Add(entry *models.LogEntry) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	db.entries = append(db.entries, entry)

	// Flush if batch is full
	if len(db.entries) >= db.maxBatchSize {
		return db.flush()
	}

	return nil
}

// AddBatch adds multiple entries
func (db *DiskBuffer) AddBatch(entries []*models.LogEntry) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	db.entries = append(db.entries, entries...)

	if len(db.entries) >= db.maxBatchSize {
		return db.flush()
	}

	return nil
}

// Get retrieves entries from the buffer
func (db *DiskBuffer) Get(limit int) ([]*models.LogEntry, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if limit > len(db.entries) {
		limit = len(db.entries)
	}

	result := make([]*models.LogEntry, limit)
	copy(result, db.entries[:limit])

	return result, nil
}

// Remove removes entries from the buffer
func (db *DiskBuffer) Remove(ids []string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	idMap := make(map[string]bool)
	for _, id := range ids {
		idMap[id] = true
	}

	filtered := make([]*models.LogEntry, 0, len(db.entries))
	for _, entry := range db.entries {
		if !idMap[entry.ID] {
			filtered = append(filtered, entry)
		}
	}

	db.entries = filtered
	return nil
}

// Size returns the number of entries
func (db *DiskBuffer) Size() int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return len(db.entries)
}

// Close closes the buffer
func (db *DiskBuffer) Close() error {
	db.cancel()
	db.flushTicker.Stop()

	db.mu.Lock()
	defer db.mu.Unlock()

	// Final flush
	if err := db.flush(); err != nil {
		return err
	}

	if db.currentFile != nil {
		return db.currentFile.Close()
	}

	return nil
}

// flush writes entries to disk
func (db *DiskBuffer) flush() error {
	if len(db.entries) == 0 {
		return nil
	}

	filename := filepath.Join(db.path, fmt.Sprintf("buffer_%d.json", time.Now().UnixNano()))
	data, err := json.MarshalIndent(db.entries, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return err
	}

	db.currentSize += int64(len(data))
	db.entries = db.entries[:0] // Clear entries after flush

	// Cleanup old files if size exceeds limit
	if db.currentSize > db.maxSize {
		db.cleanup()
	}

	return nil
}

// backgroundFlush periodically flushes the buffer
func (db *DiskBuffer) backgroundFlush() {
	for {
		select {
		case <-db.ctx.Done():
			return
		case <-db.flushTicker.C:
			db.mu.Lock()
			db.flush()
			db.mu.Unlock()
		}
	}
}

// cleanup removes old buffer files
func (db *DiskBuffer) cleanup() {
	files, err := filepath.Glob(filepath.Join(db.path, "buffer_*.json"))
	if err != nil {
		return
	}

	// Remove oldest files first
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}

		os.Remove(file)
		db.currentSize -= info.Size()

		if db.currentSize <= db.maxSize {
			break
		}
	}
}

// New creates a new buffer based on configuration
func New(config *Config) (Buffer, error) {
	if config == nil {
		config = DefaultConfig()
	}

	switch config.Type {
	case "memory":
		maxEntries := int(config.MaxSize)
		if maxEntries <= 0 {
			maxEntries = 10000
		}
		return NewMemoryBuffer(maxEntries, config.Path), nil

	case "disk":
		return NewDiskBuffer(config.Path, config.MaxSize, config.FlushInterval, config.MaxBatchSize)

	default:
		return nil, fmt.Errorf("unsupported buffer type: %s", config.Type)
	}
}
