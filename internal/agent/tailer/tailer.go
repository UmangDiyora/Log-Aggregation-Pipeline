package tailer

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Config holds the configuration for the file tailer
type Config struct {
	// BufferSize is the read buffer size in bytes
	BufferSize int

	// StateFile is the path to store file positions
	StateFile string

	// PollInterval is how often to check for file changes
	PollInterval time.Duration

	// BatchSize is the number of lines to batch before sending
	BatchSize int

	// BatchTimeout is max time to wait before flushing a batch
	BatchTimeout time.Duration
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		BufferSize:   64 * 1024, // 64KB
		StateFile:    "/var/lib/logagent/state.json",
		PollInterval: 1 * time.Second,
		BatchSize:    100,
		BatchTimeout: 5 * time.Second,
	}
}

// FileState tracks the state of a file being tailed
type FileState struct {
	Path   string      `json:"path"`
	Offset int64       `json:"offset"`
	Inode  uint64      `json:"inode"`
	Device uint64      `json:"device"`
	Size   int64       `json:"size"`
	ModTime time.Time  `json:"mod_time"`
}

// FileTailer tails multiple files concurrently
type FileTailer struct {
	config   *Config
	watcher  *fsnotify.Watcher
	states   map[string]*FileState
	statesMu sync.RWMutex
	files    map[string]*os.File
	filesMu  sync.RWMutex
	output   chan<- string
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// New creates a new FileTailer
func New(config *Config, output chan<- string) (*FileTailer, error) {
	if config == nil {
		config = DefaultConfig()
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	ft := &FileTailer{
		config:  config,
		watcher: watcher,
		states:  make(map[string]*FileState),
		files:   make(map[string]*os.File),
		output:  output,
		ctx:     ctx,
		cancel:  cancel,
	}

	// Load previous state
	if err := ft.loadState(); err != nil {
		// Log but don't fail on state load error
		fmt.Printf("warning: failed to load state: %v\n", err)
	}

	return ft, nil
}

// AddFile adds a file to be tailed
func (ft *FileTailer) AddFile(path string) error {
	// Resolve symlinks
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		resolvedPath = path // Use original path if can't resolve
	}

	ft.statesMu.Lock()
	defer ft.statesMu.Unlock()

	// Check if already tracking
	if _, exists := ft.states[resolvedPath]; exists {
		return nil
	}

	// Get file info
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return fmt.Errorf("failed to stat file %s: %w", resolvedPath, err)
	}

	// Get inode information
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("failed to get file stat for %s", resolvedPath)
	}

	// Initialize state if not exists
	state, exists := ft.states[resolvedPath]
	if !exists {
		state = &FileState{
			Path:    resolvedPath,
			Offset:  0,
			Inode:   stat.Ino,
			Device:  uint64(stat.Dev),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		}
		ft.states[resolvedPath] = state
	}

	// Add to watcher
	if err := ft.watcher.Add(resolvedPath); err != nil {
		return fmt.Errorf("failed to watch file %s: %w", resolvedPath, err)
	}

	// Start tailing this file
	ft.wg.Add(1)
	go ft.tailFile(resolvedPath)

	return nil
}

// RemoveFile stops tailing a file
func (ft *FileTailer) RemoveFile(path string) error {
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		resolvedPath = path
	}

	ft.statesMu.Lock()
	delete(ft.states, resolvedPath)
	ft.statesMu.Unlock()

	ft.filesMu.Lock()
	if f, exists := ft.files[resolvedPath]; exists {
		f.Close()
		delete(ft.files, resolvedPath)
	}
	ft.filesMu.Unlock()

	return ft.watcher.Remove(resolvedPath)
}

// Start begins tailing files
func (ft *FileTailer) Start() error {
	ft.wg.Add(1)
	go ft.watchEvents()
	return nil
}

// Stop stops the file tailer
func (ft *FileTailer) Stop() error {
	ft.cancel()

	// Close all files
	ft.filesMu.Lock()
	for _, f := range ft.files {
		f.Close()
	}
	ft.filesMu.Unlock()

	// Close watcher
	ft.watcher.Close()

	// Wait for goroutines
	ft.wg.Wait()

	// Save state
	return ft.saveState()
}

// tailFile tails a single file
func (ft *FileTailer) tailFile(path string) {
	defer ft.wg.Done()

	// Open file
	f, err := os.Open(path)
	if err != nil {
		fmt.Printf("error opening file %s: %v\n", path, err)
		return
	}

	ft.filesMu.Lock()
	ft.files[path] = f
	ft.filesMu.Unlock()

	defer func() {
		f.Close()
		ft.filesMu.Lock()
		delete(ft.files, path)
		ft.filesMu.Unlock()
	}()

	// Seek to last known position
	ft.statesMu.RLock()
	state := ft.states[path]
	offset := state.Offset
	ft.statesMu.RUnlock()

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		fmt.Printf("error seeking file %s: %v\n", path, err)
		return
	}

	reader := bufio.NewReaderSize(f, ft.config.BufferSize)
	batch := make([]string, 0, ft.config.BatchSize)
	batchTimer := time.NewTimer(ft.config.BatchTimeout)
	defer batchTimer.Stop()

	// Track consecutive EOF errors
	eofCount := 0

	for {
		select {
		case <-ft.ctx.Done():
			// Flush remaining batch
			if len(batch) > 0 {
				ft.sendBatch(batch)
			}
			return

		case <-batchTimer.C:
			// Timeout reached, flush batch
			if len(batch) > 0 {
				ft.sendBatch(batch)
				batch = batch[:0]
			}
			batchTimer.Reset(ft.config.BatchTimeout)

		default:
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					// End of file, flush batch if we have data
					if len(batch) > 0 {
						ft.sendBatch(batch)
						batch = batch[:0]
						batchTimer.Reset(ft.config.BatchTimeout)
					}

					// Wait before retrying
					eofCount++
					if eofCount > 3 {
						// After multiple EOFs, wait longer
						time.Sleep(ft.config.PollInterval)
					} else {
						time.Sleep(ft.config.PollInterval / 10)
					}
					continue
				}
				// Other errors are fatal
				fmt.Printf("error reading file %s: %v\n", path, err)
				return
			}

			// Reset EOF counter on successful read
			eofCount = 0

			// Update offset
			offset += int64(len(line))
			ft.updateOffset(path, offset)

			// Add to batch
			batch = append(batch, line)

			// Send batch if full
			if len(batch) >= ft.config.BatchSize {
				ft.sendBatch(batch)
				batch = batch[:0]
				batchTimer.Reset(ft.config.BatchTimeout)
			}
		}
	}
}

// sendBatch sends a batch of lines to the output channel
func (ft *FileTailer) sendBatch(batch []string) {
	for _, line := range batch {
		select {
		case ft.output <- line:
		case <-ft.ctx.Done():
			return
		default:
			// Output channel full, implement backpressure
			select {
			case ft.output <- line:
			case <-ft.ctx.Done():
				return
			case <-time.After(5 * time.Second):
				fmt.Println("warning: output channel blocked, dropping line")
			}
		}
	}
}

// watchEvents watches for file system events
func (ft *FileTailer) watchEvents() {
	defer ft.wg.Done()

	for {
		select {
		case <-ft.ctx.Done():
			return

		case event, ok := <-ft.watcher.Events:
			if !ok {
				return
			}

			ft.handleEvent(event)

		case err, ok := <-ft.watcher.Errors:
			if !ok {
				return
			}
			fmt.Printf("watcher error: %v\n", err)
		}
	}
}

// handleEvent handles a file system event
func (ft *FileTailer) handleEvent(event fsnotify.Event) {
	switch {
	case event.Has(fsnotify.Write):
		// File was written to, tailer will pick it up

	case event.Has(fsnotify.Rename), event.Has(fsnotify.Remove):
		// File was rotated or removed
		ft.handleRotation(event.Name)

	case event.Has(fsnotify.Create):
		// New file created (possibly after rotation)
		ft.AddFile(event.Name)
	}
}

// handleRotation handles file rotation
func (ft *FileTailer) handleRotation(path string) {
	ft.filesMu.Lock()
	if f, exists := ft.files[path]; exists {
		f.Close()
		delete(ft.files, path)
	}
	ft.filesMu.Unlock()

	// Reset state for this file
	ft.statesMu.Lock()
	if state, exists := ft.states[path]; exists {
		state.Offset = 0
	}
	ft.statesMu.Unlock()

	// File might be recreated, try to add it again
	time.Sleep(100 * time.Millisecond)
	ft.AddFile(path)
}

// updateOffset updates the file offset in state
func (ft *FileTailer) updateOffset(path string, offset int64) {
	ft.statesMu.Lock()
	defer ft.statesMu.Unlock()

	if state, exists := ft.states[path]; exists {
		state.Offset = offset
	}
}

// loadState loads the file states from disk
func (ft *FileTailer) loadState() error {
	data, err := os.ReadFile(ft.config.StateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No state file yet
		}
		return err
	}

	ft.statesMu.Lock()
	defer ft.statesMu.Unlock()

	return json.Unmarshal(data, &ft.states)
}

// saveState saves the file states to disk
func (ft *FileTailer) saveState() error {
	ft.statesMu.RLock()
	defer ft.statesMu.RUnlock()

	data, err := json.MarshalIndent(ft.states, "", "  ")
	if err != nil {
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(ft.config.StateFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(ft.config.StateFile, data, 0644)
}
