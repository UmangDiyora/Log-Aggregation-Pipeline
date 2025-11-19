package collector

import (
	"context"
	"crypto/md5"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/UmangDiyora/logpipeline/pkg/models"
)

// HTTPCollectorConfig holds HTTP collector configuration
type HTTPCollectorConfig struct {
	// ListenAddress is the address to listen on
	ListenAddress string

	// TLSEnabled indicates if TLS should be used
	TLSEnabled bool

	// TLSCert is the path to the TLS certificate
	TLSCert string

	// TLSKey is the path to the TLS key
	TLSKey string

	// AuthToken for authentication (optional)
	AuthToken string

	// Source identifier
	Source string

	// Host identifier
	Host string
}

// HTTPCollector collects logs via HTTP endpoint
type HTTPCollector struct {
	*BaseCollector
	config *HTTPCollectorConfig
	server *http.Server
}

// NewHTTPCollector creates a new HTTP collector
func NewHTTPCollector(name string, config *HTTPCollectorConfig) (*HTTPCollector, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}

	if config.ListenAddress == "" {
		config.ListenAddress = ":8081"
	}

	base := NewBaseCollector(name, "http", 1000)

	hc := &HTTPCollector{
		BaseCollector: base,
		config:        config,
	}

	return hc, nil
}

// Start starts the HTTP collector
func (hc *HTTPCollector) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ingest", hc.handleIngest)
	mux.HandleFunc("/health", hc.handleHealth)

	hc.server = &http.Server{
		Addr:    hc.config.ListenAddress,
		Handler: mux,
	}

	if hc.config.TLSEnabled {
		hc.server.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	go func() {
		var err error
		if hc.config.TLSEnabled {
			err = hc.server.ListenAndServeTLS(hc.config.TLSCert, hc.config.TLSKey)
		} else {
			err = hc.server.ListenAndServe()
		}

		if err != nil && err != http.ErrServerClosed {
			hc.status.ErrorCount++
			hc.status.LastError = err.Error()
		}
	}()

	return nil
}

// Stop stops the HTTP collector
func (hc *HTTPCollector) Stop() error {
	hc.Close()

	if hc.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return hc.server.Shutdown(ctx)
	}

	return nil
}

// handleIngest handles log ingestion requests
func (hc *HTTPCollector) handleIngest(w http.ResponseWriter, r *http.Request) {
	// Check authentication if enabled
	if hc.config.AuthToken != "" {
		authHeader := r.Header.Get("Authorization")
		expectedAuth := "Bearer " + hc.config.AuthToken
		if authHeader != expectedAuth {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Only accept POST
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		hc.status.ErrorCount++
		return
	}
	defer r.Body.Close()

	// Parse based on content type
	contentType := r.Header.Get("Content-Type")

	switch contentType {
	case "application/json":
		hc.handleJSONIngest(w, body)
	case "text/plain", "":
		hc.handleTextIngest(w, body)
	default:
		http.Error(w, "Unsupported content type", http.StatusBadRequest)
		return
	}
}

// handleJSONIngest handles JSON log ingestion
func (hc *HTTPCollector) handleJSONIngest(w http.ResponseWriter, body []byte) {
	// Try to parse as array of log entries
	var entries []*models.LogEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		// Try single entry
		var entry models.LogEntry
		if err := json.Unmarshal(body, &entry); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			hc.status.ErrorCount++
			return
		}
		entries = []*models.LogEntry{&entry}
	}

	// Emit entries
	for _, entry := range entries {
		if entry.ID == "" {
			entry.ID = hc.generateID(entry.Raw)
		}
		if entry.Source == "" {
			entry.Source = hc.config.Source
		}
		if entry.Host == "" {
			entry.Host = hc.config.Host
		}
		if entry.Timestamp.IsZero() {
			entry.Timestamp = time.Now()
		}

		hc.Emit(entry)
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "ok",
		"received": len(entries),
	})
}

// handleTextIngest handles plain text log ingestion
func (hc *HTTPCollector) handleTextIngest(w http.ResponseWriter, body []byte) {
	message := string(body)

	entry := models.NewLogEntry()
	entry.ID = hc.generateID(message)
	entry.Raw = message
	entry.Message = message
	entry.Source = hc.config.Source
	entry.Host = hc.config.Host
	entry.Level = models.LogLevelInfo
	entry.Timestamp = time.Now()

	hc.Emit(entry)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

// handleHealth handles health check requests
func (hc *HTTPCollector) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"status": "healthy",
		"collector": map[string]interface{}{
			"name":           hc.Name(),
			"type":           hc.Type(),
			"logs_collected": hc.status.LogsCollected,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// generateID generates a unique ID for a log entry
func (hc *HTTPCollector) generateID(message string) string {
	hash := md5.Sum([]byte(fmt.Sprintf("%s:%d:%s", hc.config.Source, time.Now().UnixNano(), message)))
	return fmt.Sprintf("%x", hash)
}
