package collector

import (
	"context"
	"crypto/md5"
	"fmt"
	"path/filepath"
	"time"

	"github.com/UmangDiyora/logpipeline/internal/agent/tailer"
	"github.com/UmangDiyora/logpipeline/pkg/models"
)

// FileCollectorConfig holds file collector configuration
type FileCollectorConfig struct {
	// Paths to tail (supports glob patterns)
	Paths []string

	// Exclude patterns
	Exclude []string

	// BufferSize for reading
	BufferSize int

	// StateFile for tracking positions
	StateFile string

	// Source identifier
	Source string

	// Host identifier
	Host string
}

// FileCollector collects logs from files
type FileCollector struct {
	*BaseCollector
	config *FileCollectorConfig
	tailer *tailer.FileTailer
	lines  chan string
}

// NewFileCollector creates a new file collector
func NewFileCollector(name string, config *FileCollectorConfig) (*FileCollector, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}

	base := NewBaseCollector(name, "file", 1000)

	lines := make(chan string, 1000)

	tailerConfig := &tailer.Config{
		BufferSize:   config.BufferSize,
		StateFile:    config.StateFile,
		PollInterval: 1 * time.Second,
		BatchSize:    100,
		BatchTimeout: 5 * time.Second,
	}

	if tailerConfig.BufferSize == 0 {
		tailerConfig.BufferSize = 64 * 1024
	}

	if tailerConfig.StateFile == "" {
		tailerConfig.StateFile = "/var/lib/logagent/filestate.json"
	}

	ft, err := tailer.New(tailerConfig, lines)
	if err != nil {
		return nil, fmt.Errorf("failed to create tailer: %w", err)
	}

	fc := &FileCollector{
		BaseCollector: base,
		config:        config,
		tailer:        ft,
		lines:         lines,
	}

	return fc, nil
}

// Start starts the file collector
func (fc *FileCollector) Start(ctx context.Context) error {
	// Add files to tailer
	for _, pattern := range fc.config.Paths {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			fc.status.ErrorCount++
			fc.status.LastError = fmt.Sprintf("failed to glob pattern %s: %v", pattern, err)
			continue
		}

		for _, match := range matches {
			// Check if excluded
			excluded := false
			for _, excludePattern := range fc.config.Exclude {
				if matched, _ := filepath.Match(excludePattern, filepath.Base(match)); matched {
					excluded = true
					break
				}
			}

			if !excluded {
				if err := fc.tailer.AddFile(match); err != nil {
					fc.status.ErrorCount++
					fc.status.LastError = fmt.Sprintf("failed to add file %s: %v", match, err)
				}
			}
		}
	}

	// Start tailer
	if err := fc.tailer.Start(); err != nil {
		return fmt.Errorf("failed to start tailer: %w", err)
	}

	// Start processing lines
	go fc.processLines()

	return nil
}

// Stop stops the file collector
func (fc *FileCollector) Stop() error {
	fc.Close()
	return fc.tailer.Stop()
}

// processLines converts raw lines to log entries
func (fc *FileCollector) processLines() {
	for {
		select {
		case <-fc.ctx.Done():
			return
		case line, ok := <-fc.lines:
			if !ok {
				return
			}

			entry := fc.lineToLogEntry(line)
			fc.Emit(entry)
		}
	}
}

// lineToLogEntry converts a line to a log entry
func (fc *FileCollector) lineToLogEntry(line string) *models.LogEntry {
	entry := models.NewLogEntry()
	entry.ID = fc.generateID(line)
	entry.Raw = line
	entry.Message = line
	entry.Source = fc.config.Source
	entry.Host = fc.config.Host
	entry.Level = models.LogLevelInfo
	entry.Timestamp = time.Now()

	return entry
}

// generateID generates a unique ID for a log entry
func (fc *FileCollector) generateID(line string) string {
	hash := md5.Sum([]byte(fmt.Sprintf("%s:%d:%s", fc.config.Source, time.Now().UnixNano(), line)))
	return fmt.Sprintf("%x", hash)
}
