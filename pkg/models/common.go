package models

import (
	"errors"
	"time"
)

// Common errors
var (
	ErrInvalidLogEntry    = errors.New("invalid log entry")
	ErrInvalidPipeline    = errors.New("invalid pipeline configuration")
	ErrPipelineNotFound   = errors.New("pipeline not found")
	ErrAgentNotFound      = errors.New("agent not found")
	ErrInvalidConfig      = errors.New("invalid configuration")
	ErrParseFailure       = errors.New("failed to parse log")
	ErrStorageFailure     = errors.New("storage operation failed")
	ErrIndexFailure       = errors.New("indexing operation failed")
	ErrQueryFailure       = errors.New("query execution failed")
	ErrConnectionFailure  = errors.New("connection failed")
	ErrAuthenticationFail = errors.New("authentication failed")
	ErrAuthorizationFail  = errors.New("authorization failed")
	ErrBufferFull         = errors.New("buffer is full")
	ErrTimeout            = errors.New("operation timed out")
)

// TimeRange represents a time-based query range
type TimeRange struct {
	// Start is the beginning of the time range
	Start time.Time `json:"start"`

	// End is the end of the time range
	End time.Time `json:"end"`
}

// NewTimeRange creates a new time range
func NewTimeRange(start, end time.Time) TimeRange {
	return TimeRange{
		Start: start,
		End:   end,
	}
}

// NewTimeRangeRelative creates a time range relative to now
func NewTimeRangeRelative(duration time.Duration) TimeRange {
	now := time.Now()
	return TimeRange{
		Start: now.Add(-duration),
		End:   now,
	}
}

// Contains checks if a timestamp falls within the time range
func (tr TimeRange) Contains(t time.Time) bool {
	return !t.Before(tr.Start) && !t.After(tr.End)
}

// Duration returns the duration of the time range
func (tr TimeRange) Duration() time.Duration {
	return tr.End.Sub(tr.Start)
}

// SearchQuery represents a log search query
type SearchQuery struct {
	// Query is the search query string
	Query string `json:"query"`

	// TimeRange is the time range to search
	TimeRange TimeRange `json:"time_range"`

	// Filters are additional filters to apply
	Filters map[string]interface{} `json:"filters,omitempty"`

	// Limit is the maximum number of results to return
	Limit int `json:"limit"`

	// Offset is the number of results to skip
	Offset int `json:"offset"`

	// SortBy specifies the field to sort by
	SortBy string `json:"sort_by,omitempty"`

	// SortOrder is either "asc" or "desc"
	SortOrder string `json:"sort_order,omitempty"`

	// Fields specifies which fields to return (empty means all)
	Fields []string `json:"fields,omitempty"`
}

// NewSearchQuery creates a new search query with defaults
func NewSearchQuery(query string) *SearchQuery {
	return &SearchQuery{
		Query:     query,
		TimeRange: NewTimeRangeRelative(24 * time.Hour), // Last 24 hours by default
		Limit:     100,
		Offset:    0,
		SortOrder: "desc",
		SortBy:    "timestamp",
		Filters:   make(map[string]interface{}),
	}
}

// SearchResult represents the result of a search query
type SearchResult struct {
	// Hits are the log entries that matched the query
	Hits []*LogEntry `json:"hits"`

	// Total is the total number of matching logs
	Total int64 `json:"total"`

	// Took is how long the search took in milliseconds
	Took int64 `json:"took_ms"`

	// TimedOut indicates if the search timed out
	TimedOut bool `json:"timed_out"`

	// Aggregations contains aggregation results if requested
	Aggregations map[string]interface{} `json:"aggregations,omitempty"`
}

// Batch represents a batch of log entries
type Batch struct {
	// Entries are the log entries in this batch
	Entries []*LogEntry `json:"entries"`

	// ID is a unique identifier for this batch
	ID string `json:"id"`

	// Source is the source agent ID
	Source string `json:"source"`

	// Timestamp is when the batch was created
	Timestamp time.Time `json:"timestamp"`

	// Compressed indicates if the batch is compressed
	Compressed bool `json:"compressed"`

	// CompressionType is the compression algorithm used
	CompressionType string `json:"compression_type,omitempty"`
}

// NewBatch creates a new batch
func NewBatch(source string) *Batch {
	return &Batch{
		Entries:   make([]*LogEntry, 0),
		Source:    source,
		Timestamp: time.Now(),
	}
}

// Add adds a log entry to the batch
func (b *Batch) Add(entry *LogEntry) {
	b.Entries = append(b.Entries, entry)
}

// Size returns the number of entries in the batch
func (b *Batch) Size() int {
	return len(b.Entries)
}

// IsEmpty returns true if the batch has no entries
func (b *Batch) IsEmpty() bool {
	return len(b.Entries) == 0
}

// Clear removes all entries from the batch
func (b *Batch) Clear() {
	b.Entries = make([]*LogEntry, 0)
}

// HealthStatus represents the health status of a component
type HealthStatus struct {
	// Status is the overall health status
	Status string `json:"status"`

	// Message provides details about the health status
	Message string `json:"message,omitempty"`

	// Checks contains individual health check results
	Checks map[string]CheckResult `json:"checks,omitempty"`

	// Timestamp is when the health check was performed
	Timestamp time.Time `json:"timestamp"`
}

// CheckResult represents the result of a single health check
type CheckResult struct {
	// Status is the check status (healthy, unhealthy, degraded)
	Status string `json:"status"`

	// Message provides details about the check
	Message string `json:"message,omitempty"`

	// Value is an optional metric value
	Value interface{} `json:"value,omitempty"`
}

// NewHealthStatus creates a new health status
func NewHealthStatus() *HealthStatus {
	return &HealthStatus{
		Status:    "healthy",
		Checks:    make(map[string]CheckResult),
		Timestamp: time.Now(),
	}
}

// AddCheck adds a health check result
func (h *HealthStatus) AddCheck(name string, result CheckResult) {
	if h.Checks == nil {
		h.Checks = make(map[string]CheckResult)
	}
	h.Checks[name] = result
}

// IsHealthy returns true if the overall status is healthy
func (h *HealthStatus) IsHealthy() bool {
	return h.Status == "healthy"
}
