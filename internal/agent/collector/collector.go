package collector

import (
	"context"

	"github.com/UmangDiyora/logpipeline/pkg/models"
)

// Collector represents a log collector interface
type Collector interface {
	// Start starts the collector
	Start(ctx context.Context) error

	// Stop stops the collector
	Stop() error

	// Output returns a channel that emits collected log entries
	Output() <-chan *models.LogEntry

	// Name returns the collector name
	Name() string

	// Type returns the collector type
	Type() string

	// Status returns the collector status
	Status() *models.CollectorStatus
}

// BaseCollector provides common functionality for collectors
type BaseCollector struct {
	name         string
	collectorType string
	output       chan *models.LogEntry
	ctx          context.Context
	cancel       context.CancelFunc
	status       *models.CollectorStatus
}

// NewBaseCollector creates a new base collector
func NewBaseCollector(name, collectorType string, bufferSize int) *BaseCollector {
	ctx, cancel := context.WithCancel(context.Background())

	return &BaseCollector{
		name:          name,
		collectorType: collectorType,
		output:        make(chan *models.LogEntry, bufferSize),
		ctx:           ctx,
		cancel:        cancel,
		status: &models.CollectorStatus{
			Name:   name,
			Type:   collectorType,
			Status: models.AgentStatusHealthy,
		},
	}
}

// Output returns the output channel
func (bc *BaseCollector) Output() <-chan *models.LogEntry {
	return bc.output
}

// Name returns the collector name
func (bc *BaseCollector) Name() string {
	return bc.name
}

// Type returns the collector type
func (bc *BaseCollector) Type() string {
	return bc.collectorType
}

// Status returns the collector status
func (bc *BaseCollector) Status() *models.CollectorStatus {
	return bc.status
}

// Emit sends a log entry to the output channel
func (bc *BaseCollector) Emit(entry *models.LogEntry) error {
	select {
	case bc.output <- entry:
		bc.status.LogsCollected++
		bc.status.BytesCollected += uint64(len(entry.Raw))
		return nil
	case <-bc.ctx.Done():
		return context.Canceled
	default:
		return models.ErrBufferFull
	}
}

// Close closes the base collector
func (bc *BaseCollector) Close() {
	bc.cancel()
	close(bc.output)
}
