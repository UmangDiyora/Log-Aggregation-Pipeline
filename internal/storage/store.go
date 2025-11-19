package storage

import (
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/UmangDiyora/logpipeline/pkg/models"
)

// Store is the interface for log storage
type Store interface {
	// Write writes a log entry
	Write(entry *models.LogEntry) error

	// WriteBatch writes multiple log entries
	WriteBatch(entries []*models.LogEntry) error

	// Query queries log entries
	Query(query *models.SearchQuery) (*models.SearchResult, error)

	// Get retrieves a log entry by ID
	Get(id string) (*models.LogEntry, error)

	// Delete deletes log entries older than the retention period
	Delete(before time.Time) error

	// Close closes the store
	Close() error

	// Stats returns storage statistics
	Stats() StoreStats
}

// StoreStats holds storage statistics
type StoreStats struct {
	TotalEntries uint64
	TotalSize    uint64
	OldestEntry  time.Time
	NewestEntry  time.Time
}

// Config holds storage configuration
type Config struct {
	// Path is the storage directory
	Path string

	// RetentionDays is how many days to keep logs
	RetentionDays int

	// PartitionInterval is how often to create new partitions
	PartitionInterval time.Duration

	// SyncWrites enables synchronous writes
	SyncWrites bool
}

// DefaultConfig returns default storage configuration
func DefaultConfig() *Config {
	return &Config{
		Path:              "/var/lib/logpipeline/data",
		RetentionDays:     30,
		PartitionInterval: 24 * time.Hour,
		SyncWrites:        false,
	}
}

// FileStore implements a file-based log store with time partitioning
type FileStore struct {
	config     *Config
	mu         sync.RWMutex
	partitions map[string]*partition
	stats      StoreStats
	index      *memoryIndex
}

// partition represents a time-based partition
type partition struct {
	path      string
	startTime time.Time
	endTime   time.Time
	file      *os.File
	encoder   *gob.Encoder
	mu        sync.Mutex
	count     int
}

// memoryIndex is a simple in-memory index
type memoryIndex struct {
	mu      sync.RWMutex
	entries map[string]*indexEntry
}

type indexEntry struct {
	id        string
	partition string
	offset    int64
	timestamp time.Time
}

// NewFileStore creates a new file-based store
func NewFileStore(config *Config) (*FileStore, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Create storage directory
	if err := os.MkdirAll(config.Path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	fs := &FileStore{
		config:     config,
		partitions: make(map[string]*partition),
		index: &memoryIndex{
			entries: make(map[string]*indexEntry),
		},
	}

	// Load existing partitions
	if err := fs.loadPartitions(); err != nil {
		return nil, fmt.Errorf("failed to load partitions: %w", err)
	}

	return fs, nil
}

// Write writes a log entry
func (fs *FileStore) Write(entry *models.LogEntry) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	// Get or create partition
	part, err := fs.getPartition(entry.Timestamp)
	if err != nil {
		return err
	}

	// Write to partition
	part.mu.Lock()
	defer part.mu.Unlock()

	offset := int64(0)
	if part.file != nil {
		if info, err := part.file.Stat(); err == nil {
			offset = info.Size()
		}
	}

	if err := part.encoder.Encode(entry); err != nil {
		return fmt.Errorf("failed to encode entry: %w", err)
	}

	part.count++

	// Update index
	fs.index.add(&indexEntry{
		id:        entry.ID,
		partition: part.path,
		offset:    offset,
		timestamp: entry.Timestamp,
	})

	// Update stats
	fs.stats.TotalEntries++
	if fs.stats.OldestEntry.IsZero() || entry.Timestamp.Before(fs.stats.OldestEntry) {
		fs.stats.OldestEntry = entry.Timestamp
	}
	if entry.Timestamp.After(fs.stats.NewestEntry) {
		fs.stats.NewestEntry = entry.Timestamp
	}

	return nil
}

// WriteBatch writes multiple log entries
func (fs *FileStore) WriteBatch(entries []*models.LogEntry) error {
	for _, entry := range entries {
		if err := fs.Write(entry); err != nil {
			return err
		}
	}
	return nil
}

// Query queries log entries
func (fs *FileStore) Query(query *models.SearchQuery) (*models.SearchResult, error) {
	start := time.Now()
	result := &models.SearchResult{
		Hits: make([]*models.LogEntry, 0),
	}

	fs.mu.RLock()
	defer fs.mu.RUnlock()

	// Find relevant partitions
	partitions := fs.findPartitions(query.TimeRange)

	// Search each partition
	for _, part := range partitions {
		entries, err := fs.readPartition(part, query)
		if err != nil {
			continue
		}
		result.Hits = append(result.Hits, entries...)
		if len(result.Hits) >= query.Limit {
			result.Hits = result.Hits[:query.Limit]
			break
		}
	}

	result.Total = int64(len(result.Hits))
	result.Took = time.Since(start).Milliseconds()

	return result, nil
}

// Get retrieves a log entry by ID
func (fs *FileStore) Get(id string) (*models.LogEntry, error) {
	fs.index.mu.RLock()
	idx, exists := fs.index.entries[id]
	fs.index.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("entry not found")
	}

	// Read from partition
	file, err := os.Open(idx.partition)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Seek to offset
	if _, err := file.Seek(idx.offset, 0); err != nil {
		return nil, err
	}

	decoder := gob.NewDecoder(file)
	var entry models.LogEntry
	if err := decoder.Decode(&entry); err != nil {
		return nil, err
	}

	return &entry, nil
}

// Delete deletes log entries older than the specified time
func (fs *FileStore) Delete(before time.Time) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	for key, part := range fs.partitions {
		if part.endTime.Before(before) {
			// Close and delete partition
			part.mu.Lock()
			if part.file != nil {
				part.file.Close()
			}
			os.Remove(part.path)
			part.mu.Unlock()

			delete(fs.partitions, key)
		}
	}

	return nil
}

// Close closes the store
func (fs *FileStore) Close() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	for _, part := range fs.partitions {
		part.mu.Lock()
		if part.file != nil {
			part.file.Close()
		}
		part.mu.Unlock()
	}

	return nil
}

// Stats returns storage statistics
func (fs *FileStore) Stats() StoreStats {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	return fs.stats
}

// getPartition gets or creates a partition for the given time
func (fs *FileStore) getPartition(t time.Time) (*partition, error) {
	// Round down to partition boundary
	partTime := t.Truncate(fs.config.PartitionInterval)
	key := partTime.Format("2006-01-02-15")

	if part, exists := fs.partitions[key]; exists {
		return part, nil
	}

	// Create new partition
	filename := filepath.Join(fs.config.Path, fmt.Sprintf("partition_%s.gob", key))
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	part := &partition{
		path:      filename,
		startTime: partTime,
		endTime:   partTime.Add(fs.config.PartitionInterval),
		file:      file,
		encoder:   gob.NewEncoder(file),
	}

	fs.partitions[key] = part
	return part, nil
}

// findPartitions finds partitions that overlap with the time range
func (fs *FileStore) findPartitions(timeRange models.TimeRange) []*partition {
	partitions := make([]*partition, 0)

	for _, part := range fs.partitions {
		if part.startTime.Before(timeRange.End) && part.endTime.After(timeRange.Start) {
			partitions = append(partitions, part)
		}
	}

	return partitions
}

// readPartition reads entries from a partition
func (fs *FileStore) readPartition(part *partition, query *models.SearchQuery) ([]*models.LogEntry, error) {
	file, err := os.Open(part.path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decoder := gob.NewDecoder(file)
	entries := make([]*models.LogEntry, 0)

	for {
		var entry models.LogEntry
		if err := decoder.Decode(&entry); err != nil {
			break
		}

		// Apply time range filter
		if !query.TimeRange.Contains(entry.Timestamp) {
			continue
		}

		// Simple text search in message
		if query.Query != "" && !containsIgnoreCase(entry.Message, query.Query) {
			continue
		}

		entries = append(entries, &entry)
	}

	return entries, nil
}

// loadPartitions loads existing partitions
func (fs *FileStore) loadPartitions() error {
	files, err := filepath.Glob(filepath.Join(fs.config.Path, "partition_*.gob"))
	if err != nil {
		return err
	}

	for _, filename := range files {
		// Parse partition time from filename
		base := filepath.Base(filename)
		// Extract time string from filename
		// Format: partition_2006-01-02-15.gob

		file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			continue
		}

		part := &partition{
			path:    filename,
			file:    file,
			encoder: gob.NewEncoder(file),
		}

		// Store partition
		fs.partitions[base] = part
	}

	return nil
}

// add adds an entry to the index
func (idx *memoryIndex) add(entry *indexEntry) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.entries[entry.id] = entry
}

// containsIgnoreCase checks if s contains substr (case-insensitive)
func containsIgnoreCase(s, substr string) bool {
	s = toLower(s)
	substr = toLower(substr)
	return contains(s, substr)
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func contains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// New creates a new store based on configuration
func New(config *Config) (Store, error) {
	return NewFileStore(config)
}
