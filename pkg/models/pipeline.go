package models

import (
	"time"
)

// PipelineStatus represents the current status of a pipeline
type PipelineStatus string

const (
	PipelineStatusRunning PipelineStatus = "RUNNING"
	PipelineStatusStopped PipelineStatus = "STOPPED"
	PipelineStatusError   PipelineStatus = "ERROR"
)

// Pipeline represents a log processing pipeline configuration
type Pipeline struct {
	// ID is the unique identifier for this pipeline
	ID string `json:"id" yaml:"id"`

	// Name is a human-readable name for the pipeline
	Name string `json:"name" yaml:"name"`

	// Description provides details about the pipeline's purpose
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// Input defines the input source configuration
	Input InputConfig `json:"input" yaml:"input"`

	// Processors is a list of processing stages to apply
	Processors []ProcessorConfig `json:"processors,omitempty" yaml:"processors,omitempty"`

	// Output defines where processed logs should be sent
	Output OutputConfig `json:"output" yaml:"output"`

	// Status is the current operational status
	Status PipelineStatus `json:"status"`

	// Enabled indicates if the pipeline is enabled
	Enabled bool `json:"enabled" yaml:"enabled"`

	// CreatedAt is when the pipeline was created
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the pipeline was last modified
	UpdatedAt time.Time `json:"updated_at"`

	// Metrics contains runtime statistics
	Metrics PipelineMetrics `json:"metrics,omitempty"`
}

// InputConfig represents the input source configuration
type InputConfig struct {
	// Type specifies the input type (file, syslog, docker, etc.)
	Type string `json:"type" yaml:"type"`

	// Filter is an optional filter expression
	Filter string `json:"filter,omitempty" yaml:"filter,omitempty"`

	// Config contains type-specific configuration
	Config map[string]interface{} `json:"config,omitempty" yaml:"config,omitempty"`
}

// ProcessorConfig represents a single processing stage
type ProcessorConfig struct {
	// Type specifies the processor type (parse, transform, enrich, etc.)
	Type string `json:"type" yaml:"type"`

	// Condition is an optional condition for applying this processor
	Condition string `json:"condition,omitempty" yaml:"condition,omitempty"`

	// Config contains processor-specific configuration
	Config map[string]interface{} `json:"config,omitempty" yaml:"config,omitempty"`
}

// OutputConfig represents the output destination configuration
type OutputConfig struct {
	// Type specifies the output type (storage, forward, file, etc.)
	Type string `json:"type" yaml:"type"`

	// Config contains output-specific configuration
	Config map[string]interface{} `json:"config,omitempty" yaml:"config,omitempty"`
}

// PipelineMetrics contains runtime statistics for a pipeline
type PipelineMetrics struct {
	// LogsProcessed is the total number of logs processed
	LogsProcessed uint64 `json:"logs_processed"`

	// LogsFailed is the number of logs that failed processing
	LogsFailed uint64 `json:"logs_failed"`

	// BytesProcessed is the total bytes processed
	BytesProcessed uint64 `json:"bytes_processed"`

	// AverageLatency is the average processing latency in milliseconds
	AverageLatency float64 `json:"average_latency_ms"`

	// LastProcessedAt is when the last log was processed
	LastProcessedAt time.Time `json:"last_processed_at,omitempty"`
}

// NewPipeline creates a new pipeline with default values
func NewPipeline(name string) *Pipeline {
	now := time.Now()
	return &Pipeline{
		Name:       name,
		Status:     PipelineStatusStopped,
		Enabled:    true,
		CreatedAt:  now,
		UpdatedAt:  now,
		Processors: make([]ProcessorConfig, 0),
		Metrics:    PipelineMetrics{},
	}
}

// IsRunning returns true if the pipeline is currently running
func (p *Pipeline) IsRunning() bool {
	return p.Status == PipelineStatusRunning
}

// IsStopped returns true if the pipeline is stopped
func (p *Pipeline) IsStopped() bool {
	return p.Status == PipelineStatusStopped
}

// HasError returns true if the pipeline is in error state
func (p *Pipeline) HasError() bool {
	return p.Status == PipelineStatusError
}

// AddProcessor adds a processor to the pipeline
func (p *Pipeline) AddProcessor(processor ProcessorConfig) {
	p.Processors = append(p.Processors, processor)
	p.UpdatedAt = time.Now()
}

// UpdateMetrics updates the pipeline metrics
func (p *Pipeline) UpdateMetrics(processed, failed uint64, bytes uint64, latency float64) {
	p.Metrics.LogsProcessed += processed
	p.Metrics.LogsFailed += failed
	p.Metrics.BytesProcessed += bytes

	// Calculate moving average for latency
	if p.Metrics.AverageLatency == 0 {
		p.Metrics.AverageLatency = latency
	} else {
		p.Metrics.AverageLatency = (p.Metrics.AverageLatency + latency) / 2
	}

	p.Metrics.LastProcessedAt = time.Now()
}
