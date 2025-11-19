package query

import (
	"fmt"
	"sync"
	"time"

	"github.com/UmangDiyora/logpipeline/internal/storage"
	"github.com/UmangDiyora/logpipeline/pkg/models"
)

// Engine handles log queries
type Engine struct {
	store storage.Store
	cache *queryCache
	mu    sync.RWMutex
}

// Config holds query engine configuration
type Config struct {
	// CacheSize is the maximum number of cached queries
	CacheSize int

	// CacheTTL is how long to cache query results
	CacheTTL time.Duration

	// MaxResults is the maximum results per query
	MaxResults int
}

// DefaultConfig returns default query engine configuration
func DefaultConfig() *Config {
	return &Config{
		CacheSize:  1000,
		CacheTTL:   5 * time.Minute,
		MaxResults: 10000,
	}
}

// queryCache implements a simple query result cache
type queryCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	maxSize int
	ttl     time.Duration
}

type cacheEntry struct {
	result    *models.SearchResult
	timestamp time.Time
}

// NewEngine creates a new query engine
func NewEngine(store storage.Store, config *Config) *Engine {
	if config == nil {
		config = DefaultConfig()
	}

	return &Engine{
		store: store,
		cache: &queryCache{
			entries: make(map[string]*cacheEntry),
			maxSize: config.CacheSize,
			ttl:     config.CacheTTL,
		},
	}
}

// Query executes a search query
func (e *Engine) Query(query *models.SearchQuery) (*models.SearchResult, error) {
	// Validate query
	if err := e.validateQuery(query); err != nil {
		return nil, err
	}

	// Check cache
	cacheKey := e.queryCacheKey(query)
	if result := e.cache.get(cacheKey); result != nil {
		return result, nil
	}

	// Execute query
	result, err := e.store.Query(query)
	if err != nil {
		return nil, err
	}

	// Apply sorting
	e.sortResults(result, query)

	// Apply pagination
	e.paginateResults(result, query)

	// Cache result
	e.cache.set(cacheKey, result)

	return result, nil
}

// Get retrieves a single log entry by ID
func (e *Engine) Get(id string) (*models.LogEntry, error) {
	return e.store.Get(id)
}

// Aggregate performs aggregations on log data
func (e *Engine) Aggregate(query *models.SearchQuery, aggType string, field string) (map[string]interface{}, error) {
	result, err := e.store.Query(query)
	if err != nil {
		return nil, err
	}

	switch aggType {
	case "count":
		return map[string]interface{}{
			"count": result.Total,
		}, nil

	case "terms":
		return e.termsAggregation(result.Hits, field), nil

	case "date_histogram":
		return e.dateHistogram(result.Hits, field), nil

	default:
		return nil, fmt.Errorf("unsupported aggregation type: %s", aggType)
	}
}

// termsAggregation performs a terms aggregation
func (e *Engine) termsAggregation(entries []*models.LogEntry, field string) map[string]interface{} {
	counts := make(map[string]int)

	for _, entry := range entries {
		var value string
		switch field {
		case "level":
			value = string(entry.Level)
		case "source":
			value = entry.Source
		case "host":
			value = entry.Host
		case "service":
			value = entry.Service
		default:
			if v, ok := entry.GetField(field); ok {
				value = fmt.Sprintf("%v", v)
			}
		}

		if value != "" {
			counts[value]++
		}
	}

	buckets := make([]map[string]interface{}, 0, len(counts))
	for key, count := range counts {
		buckets = append(buckets, map[string]interface{}{
			"key":   key,
			"count": count,
		})
	}

	return map[string]interface{}{
		"buckets": buckets,
	}
}

// dateHistogram performs a date histogram aggregation
func (e *Engine) dateHistogram(entries []*models.LogEntry, interval string) map[string]interface{} {
	buckets := make(map[string]int)

	var intervalDuration time.Duration
	switch interval {
	case "minute":
		intervalDuration = time.Minute
	case "hour":
		intervalDuration = time.Hour
	case "day":
		intervalDuration = 24 * time.Hour
	default:
		intervalDuration = time.Hour
	}

	for _, entry := range entries {
		bucket := entry.Timestamp.Truncate(intervalDuration).Format(time.RFC3339)
		buckets[bucket]++
	}

	result := make([]map[string]interface{}, 0, len(buckets))
	for bucket, count := range buckets {
		result = append(result, map[string]interface{}{
			"key":   bucket,
			"count": count,
		})
	}

	return map[string]interface{}{
		"buckets": result,
	}
}

// validateQuery validates a search query
func (e *Engine) validateQuery(query *models.SearchQuery) error {
	if query.Limit <= 0 {
		query.Limit = 100
	}

	if query.Limit > 10000 {
		query.Limit = 10000
	}

	if query.Offset < 0 {
		query.Offset = 0
	}

	if query.SortOrder == "" {
		query.SortOrder = "desc"
	}

	if query.SortBy == "" {
		query.SortBy = "timestamp"
	}

	return nil
}

// sortResults sorts query results
func (e *Engine) sortResults(result *models.SearchResult, query *models.SearchQuery) {
	// Simple bubble sort (for production, use sort.Slice)
	if query.SortBy == "timestamp" {
		ascending := query.SortOrder == "asc"
		for i := 0; i < len(result.Hits)-1; i++ {
			for j := i + 1; j < len(result.Hits); j++ {
				swap := false
				if ascending {
					swap = result.Hits[i].Timestamp.After(result.Hits[j].Timestamp)
				} else {
					swap = result.Hits[i].Timestamp.Before(result.Hits[j].Timestamp)
				}
				if swap {
					result.Hits[i], result.Hits[j] = result.Hits[j], result.Hits[i]
				}
			}
		}
	}
}

// paginateResults applies pagination to results
func (e *Engine) paginateResults(result *models.SearchResult, query *models.SearchQuery) {
	if query.Offset >= len(result.Hits) {
		result.Hits = make([]*models.LogEntry, 0)
		return
	}

	start := query.Offset
	end := start + query.Limit
	if end > len(result.Hits) {
		end = len(result.Hits)
	}

	result.Hits = result.Hits[start:end]
}

// queryCacheKey generates a cache key for a query
func (e *Engine) queryCacheKey(query *models.SearchQuery) string {
	return fmt.Sprintf("%s:%d:%d:%s:%s",
		query.Query,
		query.TimeRange.Start.Unix(),
		query.TimeRange.End.Unix(),
		query.SortBy,
		query.SortOrder,
	)
}

// get retrieves a cached query result
func (c *queryCache) get(key string) *models.SearchResult {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists {
		return nil
	}

	// Check if expired
	if time.Since(entry.timestamp) > c.ttl {
		return nil
	}

	return entry.result
}

// set caches a query result
func (c *queryCache) set(key string, result *models.SearchResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Simple eviction: if full, clear cache
	if len(c.entries) >= c.maxSize {
		c.entries = make(map[string]*cacheEntry)
	}

	c.entries[key] = &cacheEntry{
		result:    result,
		timestamp: time.Now(),
	}
}

// Clear clears the query cache
func (e *Engine) ClearCache() {
	e.cache.mu.Lock()
	defer e.cache.mu.Unlock()
	e.cache.entries = make(map[string]*cacheEntry)
}

// Stats returns query engine statistics
func (e *Engine) Stats() map[string]interface{} {
	e.cache.mu.RLock()
	defer e.cache.mu.RUnlock()

	storeStats := e.store.Stats()

	return map[string]interface{}{
		"cache_size":    len(e.cache.entries),
		"total_entries": storeStats.TotalEntries,
		"oldest_entry":  storeStats.OldestEntry,
		"newest_entry":  storeStats.NewestEntry,
	}
}
