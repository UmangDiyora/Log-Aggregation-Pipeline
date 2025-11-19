package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// AgentConfig represents the agent configuration
type AgentConfig struct {
	// Agent settings
	Agent AgentSettings `yaml:"agent"`

	// Input sources
	Inputs []InputConfig `yaml:"inputs"`

	// Processors to apply to logs
	Processors []ProcessorConfig `yaml:"processors,omitempty"`

	// Output configuration
	Output OutputConfig `yaml:"output"`

	// Buffer configuration
	Buffer BufferConfig `yaml:"buffer,omitempty"`
}

// AgentSettings contains agent-level settings
type AgentSettings struct {
	// ID is the agent identifier
	ID string `yaml:"id"`

	// Name is a human-readable agent name
	Name string `yaml:"name,omitempty"`

	// Tags are custom tags for this agent
	Tags []string `yaml:"tags,omitempty"`

	// HeartbeatInterval is how often to send heartbeat
	HeartbeatInterval time.Duration `yaml:"heartbeat_interval,omitempty"`

	// LogLevel is the agent's logging level
	LogLevel string `yaml:"log_level,omitempty"`
}

// InputConfig represents an input source configuration
type InputConfig struct {
	// Type is the input type (file, syslog, docker, kubernetes, http)
	Type string `yaml:"type"`

	// Name is an optional name for this input
	Name string `yaml:"name,omitempty"`

	// Enabled indicates if this input is enabled
	Enabled bool `yaml:"enabled"`

	// File input settings
	Paths   []string `yaml:"paths,omitempty"`
	Exclude []string `yaml:"exclude,omitempty"`

	// Multiline settings
	Multiline *MultilineConfig `yaml:"multiline,omitempty"`

	// Syslog settings
	Address  string `yaml:"address,omitempty"`
	Protocol string `yaml:"protocol,omitempty"`

	// Docker settings
	Endpoint   string   `yaml:"endpoint,omitempty"`
	Containers []string `yaml:"containers,omitempty"`

	// Kubernetes settings
	Namespace string            `yaml:"namespace,omitempty"`
	Labels    map[string]string `yaml:"labels,omitempty"`

	// HTTP settings
	ListenAddress string `yaml:"listen_address,omitempty"`
	TLS           bool   `yaml:"tls,omitempty"`

	// Additional configuration
	Config map[string]interface{} `yaml:"config,omitempty"`
}

// MultilineConfig represents multiline log configuration
type MultilineConfig struct {
	// Pattern is the regex pattern to match
	Pattern string `yaml:"pattern"`

	// Negate inverts the pattern matching
	Negate bool `yaml:"negate"`

	// Match specifies when to combine lines (before, after)
	Match string `yaml:"match"`

	// MaxLines is the maximum lines to combine
	MaxLines int `yaml:"max_lines,omitempty"`

	// Timeout is the maximum time to wait for continuation
	Timeout time.Duration `yaml:"timeout,omitempty"`
}

// ProcessorConfig represents a processor configuration
type ProcessorConfig struct {
	// Type is the processor type
	Type string `yaml:"type"`

	// Condition for applying this processor
	Condition string `yaml:"condition,omitempty"`

	// Configuration for the processor
	Config map[string]interface{} `yaml:"config,omitempty"`
}

// OutputConfig represents output configuration
type OutputConfig struct {
	// Type is the output type (grpc, http, kafka)
	Type string `yaml:"type"`

	// Hosts are the server endpoints
	Hosts []string `yaml:"hosts"`

	// Compression type (none, gzip, snappy, lz4)
	Compression string `yaml:"compression,omitempty"`

	// BatchSize is the number of logs per batch
	BatchSize int `yaml:"batch_size,omitempty"`

	// BatchTimeout is how long to wait before sending
	BatchTimeout time.Duration `yaml:"batch_timeout,omitempty"`

	// MaxRetries for failed sends
	MaxRetries int `yaml:"max_retries,omitempty"`

	// TLS settings
	TLS TLSConfig `yaml:"tls,omitempty"`

	// Authentication
	APIKey string `yaml:"api_key,omitempty"`

	// Additional configuration
	Config map[string]interface{} `yaml:"config,omitempty"`
}

// BufferConfig represents buffer configuration
type BufferConfig struct {
	// Type is the buffer type (memory, disk)
	Type string `yaml:"type"`

	// Size is the buffer size (bytes for disk, entries for memory)
	Size string `yaml:"size"`

	// Path is the directory for disk buffer
	Path string `yaml:"path,omitempty"`

	// FlushInterval for disk buffer
	FlushInterval time.Duration `yaml:"flush_interval,omitempty"`
}

// TLSConfig represents TLS configuration
type TLSConfig struct {
	// Enabled indicates if TLS is enabled
	Enabled bool `yaml:"enabled"`

	// CertFile is the path to the certificate file
	CertFile string `yaml:"cert_file,omitempty"`

	// KeyFile is the path to the key file
	KeyFile string `yaml:"key_file,omitempty"`

	// CAFile is the path to the CA certificate
	CAFile string `yaml:"ca_file,omitempty"`

	// InsecureSkipVerify skips certificate verification
	InsecureSkipVerify bool `yaml:"insecure_skip_verify,omitempty"`
}

// ServerConfig represents the server configuration
type ServerConfig struct {
	// Server settings
	Server ServerSettings `yaml:"server"`

	// Storage configuration
	Storage StorageConfig `yaml:"storage"`

	// Index configuration
	Index IndexConfig `yaml:"index"`

	// Pipelines configuration
	Pipelines []PipelineConfig `yaml:"pipelines,omitempty"`

	// Metrics settings
	Metrics MetricsConfig `yaml:"metrics,omitempty"`

	// Alerts configuration
	Alerts []AlertConfig `yaml:"alerts,omitempty"`
}

// ServerSettings contains server-level settings
type ServerSettings struct {
	// GRPCPort is the gRPC server port
	GRPCPort int `yaml:"grpc_port"`

	// HTTPPort is the HTTP server port
	HTTPPort int `yaml:"http_port"`

	// LogLevel is the server's logging level
	LogLevel string `yaml:"log_level,omitempty"`
}

// StorageConfig represents storage configuration
type StorageConfig struct {
	// Type is the storage backend (badger, boltdb)
	Type string `yaml:"type"`

	// Path is the storage directory
	Path string `yaml:"path"`

	// Retention is how long to keep logs
	Retention string `yaml:"retention,omitempty"`

	// CompactionInterval for storage optimization
	CompactionInterval time.Duration `yaml:"compaction_interval,omitempty"`
}

// IndexConfig represents index configuration
type IndexConfig struct {
	// Type is the index type (bleve, custom)
	Type string `yaml:"type"`

	// Path is the index directory
	Path string `yaml:"path"`

	// RefreshInterval for index updates
	RefreshInterval time.Duration `yaml:"refresh_interval,omitempty"`
}

// PipelineConfig represents a processing pipeline
type PipelineConfig struct {
	// Name is the pipeline name
	Name string `yaml:"name"`

	// Filter determines which logs enter this pipeline
	Filter string `yaml:"filter,omitempty"`

	// Parser to use
	Parser string `yaml:"parser,omitempty"`

	// Processors to apply
	Processors []ProcessorConfig `yaml:"processors,omitempty"`
}

// MetricsConfig represents metrics configuration
type MetricsConfig struct {
	// Enabled indicates if metrics are enabled
	Enabled bool `yaml:"enabled"`

	// Port is the metrics server port
	Port int `yaml:"port,omitempty"`

	// Path is the metrics endpoint path
	Path string `yaml:"path,omitempty"`
}

// AlertConfig represents an alert rule
type AlertConfig struct {
	// Name is the alert name
	Name string `yaml:"name"`

	// Query is the search query
	Query string `yaml:"query"`

	// Window is the time window
	Window string `yaml:"window"`

	// Threshold for triggering alert
	Threshold int `yaml:"threshold"`

	// Channels to notify
	Channels []string `yaml:"channels"`
}

// LoadAgentConfig loads agent configuration from a file
func LoadAgentConfig(path string) (*AgentConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config AgentConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Set defaults
	if config.Agent.HeartbeatInterval == 0 {
		config.Agent.HeartbeatInterval = 30 * time.Second
	}

	if config.Output.BatchSize == 0 {
		config.Output.BatchSize = 1000
	}

	if config.Output.BatchTimeout == 0 {
		config.Output.BatchTimeout = 5 * time.Second
	}

	if config.Output.MaxRetries == 0 {
		config.Output.MaxRetries = 3
	}

	return &config, nil
}

// LoadServerConfig loads server configuration from a file
func LoadServerConfig(path string) (*ServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config ServerConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Set defaults
	if config.Server.GRPCPort == 0 {
		config.Server.GRPCPort = 9090
	}

	if config.Server.HTTPPort == 0 {
		config.Server.HTTPPort = 8080
	}

	if config.Metrics.Enabled && config.Metrics.Port == 0 {
		config.Metrics.Port = 2112
	}

	return &config, nil
}
