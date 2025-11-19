package processor

import (
	"fmt"
	"strings"

	"github.com/UmangDiyora/logpipeline/pkg/models"
)

// Processor is the interface for log processors
type Processor interface {
	// Process processes a log entry
	Process(entry *models.LogEntry) error

	// Name returns the processor name
	Name() string
}

// Config holds processor configuration
type Config struct {
	// Type is the processor type
	Type string

	// Fields to process
	Fields map[string]interface{}

	// Condition for processing
	Condition string
}

// AddFieldsProcessor adds fields to log entries
type AddFieldsProcessor struct {
	fields map[string]interface{}
}

// NewAddFieldsProcessor creates a new add fields processor
func NewAddFieldsProcessor(fields map[string]interface{}) *AddFieldsProcessor {
	return &AddFieldsProcessor{
		fields: fields,
	}
}

// Process adds fields to the log entry
func (p *AddFieldsProcessor) Process(entry *models.LogEntry) error {
	for key, value := range p.fields {
		entry.AddField(key, value)
	}
	return nil
}

// Name returns the processor name
func (p *AddFieldsProcessor) Name() string {
	return "add_fields"
}

// RenameFieldsProcessor renames fields in log entries
type RenameFieldsProcessor struct {
	mapping map[string]string
}

// NewRenameFieldsProcessor creates a new rename fields processor
func NewRenameFieldsProcessor(mapping map[string]string) *RenameFieldsProcessor {
	return &RenameFieldsProcessor{
		mapping: mapping,
	}
}

// Process renames fields in the log entry
func (p *RenameFieldsProcessor) Process(entry *models.LogEntry) error {
	for oldName, newName := range p.mapping {
		if value, exists := entry.GetField(oldName); exists {
			entry.AddField(newName, value)
			// Remove old field
			delete(entry.Fields, oldName)
		}
	}
	return nil
}

// Name returns the processor name
func (p *RenameFieldsProcessor) Name() string {
	return "rename_fields"
}

// DropFieldsProcessor removes fields from log entries
type DropFieldsProcessor struct {
	fields []string
}

// NewDropFieldsProcessor creates a new drop fields processor
func NewDropFieldsProcessor(fields []string) *DropFieldsProcessor {
	return &DropFieldsProcessor{
		fields: fields,
	}
}

// Process removes fields from the log entry
func (p *DropFieldsProcessor) Process(entry *models.LogEntry) error {
	for _, field := range p.fields {
		delete(entry.Fields, field)
	}
	return nil
}

// Name returns the processor name
func (p *DropFieldsProcessor) Name() string {
	return "drop_fields"
}

// LowercaseProcessor converts field values to lowercase
type LowercaseProcessor struct {
	fields []string
}

// NewLowercaseProcessor creates a new lowercase processor
func NewLowercaseProcessor(fields []string) *LowercaseProcessor {
	return &LowercaseProcessor{
		fields: fields,
	}
}

// Process converts specified fields to lowercase
func (p *LowercaseProcessor) Process(entry *models.LogEntry) error {
	for _, field := range p.fields {
		if value, exists := entry.GetField(field); exists {
			if strValue, ok := value.(string); ok {
				entry.AddField(field, strings.ToLower(strValue))
			}
		}
	}
	return nil
}

// Name returns the processor name
func (p *LowercaseProcessor) Name() string {
	return "lowercase"
}

// TrimProcessor trims whitespace from field values
type TrimProcessor struct {
	fields []string
}

// NewTrimProcessor creates a new trim processor
func NewTrimProcessor(fields []string) *TrimProcessor {
	return &TrimProcessor{
		fields: fields,
	}
}

// Process trims whitespace from specified fields
func (p *TrimProcessor) Process(entry *models.LogEntry) error {
	for _, field := range p.fields {
		if value, exists := entry.GetField(field); exists {
			if strValue, ok := value.(string); ok {
				entry.AddField(field, strings.TrimSpace(strValue))
			}
		}
	}
	return nil
}

// Name returns the processor name
func (p *TrimProcessor) Name() string {
	return "trim"
}

// FilterProcessor filters log entries based on conditions
type FilterProcessor struct {
	dropIfMatch bool
	field       string
	pattern     string
}

// NewFilterProcessor creates a new filter processor
func NewFilterProcessor(field, pattern string, dropIfMatch bool) *FilterProcessor {
	return &FilterProcessor{
		field:       field,
		pattern:     pattern,
		dropIfMatch: dropIfMatch,
	}
}

// Process filters the log entry
func (p *FilterProcessor) Process(entry *models.LogEntry) error {
	if value, exists := entry.GetField(p.field); exists {
		if strValue, ok := value.(string); ok {
			matches := strings.Contains(strValue, p.pattern)
			if (p.dropIfMatch && matches) || (!p.dropIfMatch && !matches) {
				return fmt.Errorf("entry filtered out")
			}
		}
	}
	return nil
}

// Name returns the processor name
func (p *FilterProcessor) Name() string {
	return "filter"
}

// New creates a new processor based on configuration
func New(config *Config) (Processor, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}

	switch config.Type {
	case "add_fields":
		return NewAddFieldsProcessor(config.Fields), nil

	case "rename_fields":
		mapping := make(map[string]string)
		for k, v := range config.Fields {
			if strVal, ok := v.(string); ok {
				mapping[k] = strVal
			}
		}
		return NewRenameFieldsProcessor(mapping), nil

	case "drop_fields":
		fields := make([]string, 0)
		if fieldList, ok := config.Fields["fields"].([]interface{}); ok {
			for _, f := range fieldList {
				if strField, ok := f.(string); ok {
					fields = append(fields, strField)
				}
			}
		}
		return NewDropFieldsProcessor(fields), nil

	case "lowercase":
		fields := make([]string, 0)
		if fieldList, ok := config.Fields["fields"].([]interface{}); ok {
			for _, f := range fieldList {
				if strField, ok := f.(string); ok {
					fields = append(fields, strField)
				}
			}
		}
		return NewLowercaseProcessor(fields), nil

	case "trim":
		fields := make([]string, 0)
		if fieldList, ok := config.Fields["fields"].([]interface{}); ok {
			for _, f := range fieldList {
				if strField, ok := f.(string); ok {
					fields = append(fields, strField)
				}
			}
		}
		return NewTrimProcessor(fields), nil

	default:
		return nil, fmt.Errorf("unsupported processor type: %s", config.Type)
	}
}
