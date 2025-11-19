package pipeline

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/UmangDiyora/logpipeline/internal/pipeline/processor"
	"github.com/UmangDiyora/logpipeline/pkg/models"
	"github.com/UmangDiyora/logpipeline/pkg/parser"
)

// Pipeline represents a log processing pipeline
type Pipeline struct {
	id         string
	name       string
	parser     parser.Parser
	processors []processor.Processor
	input      <-chan *models.LogEntry
	output     chan<- *models.LogEntry
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	stats      PipelineStats
	statsMu    sync.RWMutex
}

// PipelineStats holds pipeline statistics
type PipelineStats struct {
	Processed      uint64
	Failed         uint64
	Dropped        uint64
	AverageLatency time.Duration
	LastProcessed  time.Time
}

// Config holds pipeline configuration
type Config struct {
	ID         string
	Name       string
	Parser     *parser.Config
	Processors []processor.Config
	Workers    int
}

// New creates a new pipeline
func New(config *Config, input <-chan *models.LogEntry, output chan<- *models.LogEntry) (*Pipeline, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}

	ctx, cancel := context.WithCancel(context.Background())

	p := &Pipeline{
		id:     config.ID,
		name:   config.Name,
		input:  input,
		output: output,
		ctx:    ctx,
		cancel: cancel,
	}

	// Initialize parser
	if config.Parser != nil {
		var err error
		p.parser, err = parser.New(config.Parser)
		if err != nil {
			return nil, fmt.Errorf("failed to create parser: %w", err)
		}
	}

	// Initialize processors
	p.processors = make([]processor.Processor, 0, len(config.Processors))
	for _, procConfig := range config.Processors {
		proc, err := processor.New(&procConfig)
		if err != nil {
			fmt.Printf("warning: failed to create processor: %v\n", err)
			continue
		}
		p.processors = append(p.processors, proc)
	}

	// Set default workers
	if config.Workers == 0 {
		config.Workers = 4
	}

	// Start workers
	for i := 0; i < config.Workers; i++ {
		p.wg.Add(1)
		go p.worker()
	}

	return p, nil
}

// worker processes log entries
func (p *Pipeline) worker() {
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return

		case entry, ok := <-p.input:
			if !ok {
				return
			}

			start := time.Now()

			// Process entry
			if err := p.processEntry(entry); err != nil {
				p.recordFailure()
				continue
			}

			// Send to output
			select {
			case p.output <- entry:
				p.recordSuccess(time.Since(start))
			case <-p.ctx.Done():
				return
			default:
				// Output full, drop entry
				p.recordDrop()
			}
		}
	}
}

// processEntry processes a single log entry
func (p *Pipeline) processEntry(entry *models.LogEntry) error {
	// Apply parser
	if p.parser != nil {
		if err := p.parser.Parse(entry); err != nil {
			return fmt.Errorf("parser error: %w", err)
		}
	}

	// Apply processors
	for _, proc := range p.processors {
		if err := proc.Process(entry); err != nil {
			return fmt.Errorf("processor %s error: %w", proc.Name(), err)
		}
	}

	return nil
}

// recordSuccess records successful processing
func (p *Pipeline) recordSuccess(latency time.Duration) {
	p.statsMu.Lock()
	defer p.statsMu.Unlock()

	p.stats.Processed++
	p.stats.LastProcessed = time.Now()

	// Update average latency
	if p.stats.AverageLatency == 0 {
		p.stats.AverageLatency = latency
	} else {
		p.stats.AverageLatency = (p.stats.AverageLatency + latency) / 2
	}
}

// recordFailure records failed processing
func (p *Pipeline) recordFailure() {
	p.statsMu.Lock()
	defer p.statsMu.Unlock()
	p.stats.Failed++
}

// recordDrop records dropped entry
func (p *Pipeline) recordDrop() {
	p.statsMu.Lock()
	defer p.statsMu.Unlock()
	p.stats.Dropped++
}

// GetStats returns pipeline statistics
func (p *Pipeline) GetStats() PipelineStats {
	p.statsMu.RLock()
	defer p.statsMu.RUnlock()
	return p.stats
}

// ID returns the pipeline ID
func (p *Pipeline) ID() string {
	return p.id
}

// Name returns the pipeline name
func (p *Pipeline) Name() string {
	return p.name
}

// Stop stops the pipeline
func (p *Pipeline) Stop() error {
	p.cancel()
	p.wg.Wait()
	return nil
}
