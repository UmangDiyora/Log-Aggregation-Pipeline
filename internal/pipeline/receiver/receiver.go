package receiver

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/UmangDiyora/logpipeline/pkg/models"
)

// Config holds receiver configuration
type Config struct {
	// HTTPAddr is the HTTP server address
	HTTPAddr string

	// MaxBatchSize is the maximum batch size to accept
	MaxBatchSize int

	// RateLimit is the max requests per second per agent
	RateLimit int

	// Timeout for requests
	Timeout time.Duration

	// Authentication
	RequireAuth bool
	APIKeys     map[string]string
}

// DefaultConfig returns default receiver configuration
func DefaultConfig() *Config {
	return &Config{
		HTTPAddr:     ":8080",
		MaxBatchSize: 10000,
		RateLimit:    1000,
		Timeout:      30 * time.Second,
		RequireAuth:  false,
		APIKeys:      make(map[string]string),
	}
}

// Receiver receives logs from agents
type Receiver struct {
	config     *Config
	server     *http.Server
	output     chan *models.LogEntry
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	stats      Stats
	rateLimits map[string]*rateLimiter
	rlMu       sync.RWMutex
}

// Stats holds receiver statistics
type Stats struct {
	RequestsReceived uint64
	LogsReceived     uint64
	BytesReceived    uint64
	Errors           uint64
	LastReceived     time.Time
}

// rateLimiter implements a simple token bucket rate limiter
type rateLimiter struct {
	tokens    int
	maxTokens int
	lastRefill time.Time
	mu        sync.Mutex
}

// New creates a new receiver
func New(config *Config, output chan *models.LogEntry) (*Receiver, error) {
	if config == nil {
		config = DefaultConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	r := &Receiver{
		config:     config,
		output:     output,
		ctx:        ctx,
		cancel:     cancel,
		rateLimits: make(map[string]*rateLimiter),
	}

	return r, nil
}

// Start starts the receiver
func (r *Receiver) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/logs/ingest", r.handleIngest)
	mux.HandleFunc("/api/v1/health", r.handleHealth)
	mux.HandleFunc("/api/v1/stats", r.handleStats)

	r.server = &http.Server{
		Addr:         r.config.HTTPAddr,
		Handler:      r.timeoutMiddleware(r.authMiddleware(mux)),
		ReadTimeout:  r.config.Timeout,
		WriteTimeout: r.config.Timeout,
	}

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		if err := r.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("receiver server error: %v\n", err)
		}
	}()

	return nil
}

// Stop stops the receiver
func (r *Receiver) Stop() error {
	r.cancel()

	if r.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		r.server.Shutdown(ctx)
	}

	r.wg.Wait()
	return nil
}

// authMiddleware adds authentication middleware
func (r *Receiver) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !r.config.RequireAuth {
			next.ServeHTTP(w, req)
			return
		}

		authHeader := req.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			atomic.AddUint64(&r.stats.Errors, 1)
			return
		}

		// Extract Bearer token
		var token string
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		}

		// Validate token
		valid := false
		for _, apiKey := range r.config.APIKeys {
			if token == apiKey {
				valid = true
				break
			}
		}

		if !valid {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			atomic.AddUint64(&r.stats.Errors, 1)
			return
		}

		next.ServeHTTP(w, req)
	})
}

// timeoutMiddleware adds timeout middleware
func (r *Receiver) timeoutMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx, cancel := context.WithTimeout(req.Context(), r.config.Timeout)
		defer cancel()

		req = req.WithContext(ctx)
		next.ServeHTTP(w, req)
	})
}

// handleIngest handles log ingestion requests
func (r *Receiver) handleIngest(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	atomic.AddUint64(&r.stats.RequestsReceived, 1)

	// Check rate limit
	agentID := req.Header.Get("X-Agent-ID")
	if agentID == "" {
		agentID = req.RemoteAddr
	}

	if !r.checkRateLimit(agentID) {
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
		atomic.AddUint64(&r.stats.Errors, 1)
		return
	}

	// Read body
	var reader io.Reader = req.Body
	contentEncoding := req.Header.Get("Content-Encoding")

	if contentEncoding == "gzip" {
		gzReader, err := gzip.NewReader(req.Body)
		if err != nil {
			http.Error(w, "Failed to decompress", http.StatusBadRequest)
			atomic.AddUint64(&r.stats.Errors, 1)
			return
		}
		defer gzReader.Close()
		reader = gzReader
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		atomic.AddUint64(&r.stats.Errors, 1)
		return
	}

	atomic.AddUint64(&r.stats.BytesReceived, uint64(len(body)))

	// Parse batch
	var batch models.Batch
	if err := json.Unmarshal(body, &batch); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		atomic.AddUint64(&r.stats.Errors, 1)
		return
	}

	// Validate batch size
	if len(batch.Entries) > r.config.MaxBatchSize {
		http.Error(w, "Batch too large", http.StatusRequestEntityTooLarge)
		atomic.AddUint64(&r.stats.Errors, 1)
		return
	}

	// Send entries to output channel
	for _, entry := range batch.Entries {
		select {
		case r.output <- entry:
			atomic.AddUint64(&r.stats.LogsReceived, 1)
		case <-r.ctx.Done():
			http.Error(w, "Server shutting down", http.StatusServiceUnavailable)
			return
		default:
			http.Error(w, "Output buffer full", http.StatusServiceUnavailable)
			atomic.AddUint64(&r.stats.Errors, 1)
			return
		}
	}

	r.stats.LastReceived = time.Now()

	// Send response
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "ok",
		"received": len(batch.Entries),
	})
}

// handleHealth handles health check requests
func (r *Receiver) handleHealth(w http.ResponseWriter, req *http.Request) {
	health := map[string]interface{}{
		"status": "healthy",
		"time":   time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(health)
}

// handleStats handles statistics requests
func (r *Receiver) handleStats(w http.ResponseWriter, req *http.Request) {
	stats := map[string]interface{}{
		"requests_received": atomic.LoadUint64(&r.stats.RequestsReceived),
		"logs_received":     atomic.LoadUint64(&r.stats.LogsReceived),
		"bytes_received":    atomic.LoadUint64(&r.stats.BytesReceived),
		"errors":            atomic.LoadUint64(&r.stats.Errors),
		"last_received":     r.stats.LastReceived,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// checkRateLimit checks if the agent has exceeded the rate limit
func (r *Receiver) checkRateLimit(agentID string) bool {
	r.rlMu.Lock()
	defer r.rlMu.Unlock()

	rl, exists := r.rateLimits[agentID]
	if !exists {
		rl = &rateLimiter{
			tokens:    r.config.RateLimit,
			maxTokens: r.config.RateLimit,
			lastRefill: time.Now(),
		}
		r.rateLimits[agentID] = rl
	}

	return rl.allow()
}

// allow checks if a request is allowed
func (rl *rateLimiter) allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Refill tokens based on time elapsed
	now := time.Now()
	elapsed := now.Sub(rl.lastRefill)
	tokensToAdd := int(elapsed.Seconds())

	if tokensToAdd > 0 {
		rl.tokens += tokensToAdd
		if rl.tokens > rl.maxTokens {
			rl.tokens = rl.maxTokens
		}
		rl.lastRefill = now
	}

	// Check if we have tokens
	if rl.tokens > 0 {
		rl.tokens--
		return true
	}

	return false
}

// GetStats returns current statistics
func (r *Receiver) GetStats() Stats {
	return Stats{
		RequestsReceived: atomic.LoadUint64(&r.stats.RequestsReceived),
		LogsReceived:     atomic.LoadUint64(&r.stats.LogsReceived),
		BytesReceived:    atomic.LoadUint64(&r.stats.BytesReceived),
		Errors:           atomic.LoadUint64(&r.stats.Errors),
		LastReceived:     r.stats.LastReceived,
	}
}
