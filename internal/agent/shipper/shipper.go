package shipper

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/UmangDiyora/logpipeline/pkg/models"
)

// Config holds shipper configuration
type Config struct {
	// Endpoints are the server endpoints to send logs to
	Endpoints []string

	// Compression type (none, gzip, snappy, lz4)
	Compression string

	// BatchSize is the number of logs to send per request
	BatchSize int

	// BatchTimeout is how long to wait before sending incomplete batch
	BatchTimeout time.Duration

	// MaxRetries is the maximum number of retry attempts
	MaxRetries int

	// RetryBackoff is the initial backoff duration
	RetryBackoff time.Duration

	// Timeout is the HTTP request timeout
	Timeout time.Duration

	// TLS configuration
	TLSEnabled bool
	TLSConfig  *tls.Config

	// Authentication
	APIKey string

	// Circuit breaker settings
	FailureThreshold int
	SuccessThreshold int
	OpenTimeout      time.Duration
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Endpoints:        []string{"http://localhost:9090"},
		Compression:      "gzip",
		BatchSize:        1000,
		BatchTimeout:     5 * time.Second,
		MaxRetries:       3,
		RetryBackoff:     1 * time.Second,
		Timeout:          30 * time.Second,
		TLSEnabled:       false,
		FailureThreshold: 5,
		SuccessThreshold: 2,
		OpenTimeout:      30 * time.Second,
	}
}

// Shipper sends logs to the pipeline server
type Shipper struct {
	config     *Config
	client     *http.Client
	batch      *models.Batch
	batchMu    sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	inputChan  chan *models.LogEntry
	endpoints  []*endpoint
	endpointMu sync.RWMutex
}

// endpoint represents a server endpoint with circuit breaker
type endpoint struct {
	url            string
	state          circuitState
	failures       int
	successes      int
	lastFailure    time.Time
	consecutiveFail int
	mu             sync.RWMutex
}

type circuitState int

const (
	circuitClosed circuitState = iota
	circuitOpen
	circuitHalfOpen
)

// New creates a new shipper
func New(config *Config, agentID string) (*Shipper, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Create HTTP client
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}

	if config.TLSEnabled && config.TLSConfig != nil {
		transport.TLSClientConfig = config.TLSConfig
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   config.Timeout,
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Initialize endpoints
	endpoints := make([]*endpoint, len(config.Endpoints))
	for i, url := range config.Endpoints {
		endpoints[i] = &endpoint{
			url:   url,
			state: circuitClosed,
		}
	}

	shipper := &Shipper{
		config:    config,
		client:    client,
		batch:     models.NewBatch(agentID),
		ctx:       ctx,
		cancel:    cancel,
		inputChan: make(chan *models.LogEntry, config.BatchSize*2),
		endpoints: endpoints,
	}

	// Start batch processor
	shipper.wg.Add(1)
	go shipper.processBatches()

	return shipper, nil
}

// Ship sends a log entry
func (s *Shipper) Ship(entry *models.LogEntry) error {
	select {
	case s.inputChan <- entry:
		return nil
	case <-s.ctx.Done():
		return fmt.Errorf("shipper is closed")
	default:
		return models.ErrBufferFull
	}
}

// ShipBatch sends a batch of log entries
func (s *Shipper) ShipBatch(entries []*models.LogEntry) error {
	for _, entry := range entries {
		if err := s.Ship(entry); err != nil {
			return err
		}
	}
	return nil
}

// processBatches processes incoming log entries and sends batches
func (s *Shipper) processBatches() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.BatchTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			// Send remaining batch
			s.sendCurrentBatch()
			return

		case entry := <-s.inputChan:
			s.batchMu.Lock()
			s.batch.Add(entry)
			shouldSend := s.batch.Size() >= s.config.BatchSize
			s.batchMu.Unlock()

			if shouldSend {
				s.sendCurrentBatch()
			}

		case <-ticker.C:
			s.sendCurrentBatch()
		}
	}
}

// sendCurrentBatch sends the current batch to the server
func (s *Shipper) sendCurrentBatch() {
	s.batchMu.Lock()
	if s.batch.IsEmpty() {
		s.batchMu.Unlock()
		return
	}

	// Take ownership of current batch
	batch := s.batch
	s.batch = models.NewBatch(batch.Source)
	s.batchMu.Unlock()

	// Send with retry
	if err := s.sendWithRetry(batch); err != nil {
		fmt.Printf("error sending batch: %v\n", err)
	}
}

// sendWithRetry sends a batch with retry logic
func (s *Shipper) sendWithRetry(batch *models.Batch) error {
	var lastErr error

	for attempt := 0; attempt <= s.config.MaxRetries; attempt++ {
		// Select endpoint
		ep := s.selectEndpoint()
		if ep == nil {
			return fmt.Errorf("no available endpoints")
		}

		// Try to send
		err := s.send(ep, batch)
		if err == nil {
			s.recordSuccess(ep)
			return nil
		}

		lastErr = err
		s.recordFailure(ep)

		// Exponential backoff
		if attempt < s.config.MaxRetries {
			backoff := s.config.RetryBackoff * time.Duration(1<<uint(attempt))
			time.Sleep(backoff)
		}
	}

	return fmt.Errorf("failed after %d attempts: %w", s.config.MaxRetries, lastErr)
}

// send sends a batch to a specific endpoint
func (s *Shipper) send(ep *endpoint, batch *models.Batch) error {
	// Marshal batch
	data, err := json.Marshal(batch)
	if err != nil {
		return fmt.Errorf("failed to marshal batch: %w", err)
	}

	// Compress if enabled
	if s.config.Compression == "gzip" {
		compressed, err := s.compressGzip(data)
		if err != nil {
			return fmt.Errorf("failed to compress: %w", err)
		}
		data = compressed
		batch.Compressed = true
		batch.CompressionType = "gzip"
	}

	// Create request
	url := fmt.Sprintf("%s/api/v1/logs/ingest", ep.url)
	req, err := http.NewRequestWithContext(s.ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	if batch.Compressed {
		req.Header.Set("Content-Encoding", batch.CompressionType)
	}
	if s.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.config.APIKey)
	}

	// Send request
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// compressGzip compresses data using gzip
func (s *Shipper) compressGzip(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)

	if _, err := writer.Write(data); err != nil {
		return nil, err
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// selectEndpoint selects an available endpoint using round-robin with circuit breaker
func (s *Shipper) selectEndpoint() *endpoint {
	s.endpointMu.RLock()
	defer s.endpointMu.RUnlock()

	available := make([]*endpoint, 0)
	for _, ep := range s.endpoints {
		ep.mu.RLock()
		state := ep.state
		lastFailure := ep.lastFailure
		ep.mu.RUnlock()

		switch state {
		case circuitClosed:
			available = append(available, ep)
		case circuitHalfOpen:
			available = append(available, ep)
		case circuitOpen:
			// Check if enough time has passed to retry
			if time.Since(lastFailure) > s.config.OpenTimeout {
				ep.mu.Lock()
				ep.state = circuitHalfOpen
				ep.mu.Unlock()
				available = append(available, ep)
			}
		}
	}

	if len(available) == 0 {
		return nil
	}

	// Random selection from available endpoints
	return available[rand.Intn(len(available))]
}

// recordSuccess records a successful request
func (s *Shipper) recordSuccess(ep *endpoint) {
	ep.mu.Lock()
	defer ep.mu.Unlock()

	ep.successes++
	ep.consecutiveFail = 0

	if ep.state == circuitHalfOpen && ep.successes >= s.config.SuccessThreshold {
		ep.state = circuitClosed
		ep.failures = 0
		ep.successes = 0
	}
}

// recordFailure records a failed request
func (s *Shipper) recordFailure(ep *endpoint) {
	ep.mu.Lock()
	defer ep.mu.Unlock()

	ep.failures++
	ep.consecutiveFail++
	ep.lastFailure = time.Now()

	if ep.state == circuitHalfOpen {
		ep.state = circuitOpen
		ep.successes = 0
	} else if ep.consecutiveFail >= s.config.FailureThreshold {
		ep.state = circuitOpen
	}
}

// Stats returns shipper statistics
func (s *Shipper) Stats() map[string]interface{} {
	s.endpointMu.RLock()
	defer s.endpointMu.RUnlock()

	endpointStats := make([]map[string]interface{}, len(s.endpoints))
	for i, ep := range s.endpoints {
		ep.mu.RLock()
		endpointStats[i] = map[string]interface{}{
			"url":      ep.url,
			"state":    ep.state,
			"failures": ep.failures,
		}
		ep.mu.RUnlock()
	}

	s.batchMu.Lock()
	batchSize := s.batch.Size()
	s.batchMu.Unlock()

	return map[string]interface{}{
		"endpoints":        endpointStats,
		"current_batch_size": batchSize,
		"queue_size":       len(s.inputChan),
	}
}

// Close stops the shipper
func (s *Shipper) Close() error {
	s.cancel()
	s.wg.Wait()
	close(s.inputChan)
	return nil
}
