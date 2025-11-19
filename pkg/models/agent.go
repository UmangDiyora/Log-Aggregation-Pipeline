package models

import (
	"time"
)

// AgentStatus represents the current status of an agent or collector
type AgentStatus string

const (
	AgentStatusHealthy   AgentStatus = "HEALTHY"
	AgentStatusUnhealthy AgentStatus = "UNHEALTHY"
	AgentStatusOffline   AgentStatus = "OFFLINE"
)

// Agent represents a log collection agent
type Agent struct {
	// ID is the unique identifier for this agent
	ID string `json:"id"`

	// Hostname is the hostname of the machine running the agent
	Hostname string `json:"hostname"`

	// IP is the IP address of the agent
	IP string `json:"ip"`

	// Version is the agent software version
	Version string `json:"version"`

	// LastHeartbeat is the timestamp of the last heartbeat received
	LastHeartbeat time.Time `json:"last_heartbeat"`

	// Status is the current health status of the agent
	Status AgentStatus `json:"status"`

	// Resources contains current resource usage information
	Resources ResourceUsage `json:"resources"`

	// CollectorStatus contains the status of each collector
	CollectorStatus map[string]CollectorStatus `json:"collector_status"`

	// Labels are custom labels attached to this agent
	Labels map[string]string `json:"labels,omitempty"`

	// RegisteredAt is when the agent first registered
	RegisteredAt time.Time `json:"registered_at"`

	// Config contains agent configuration metadata
	Config AgentConfig `json:"config,omitempty"`
}

// ResourceUsage represents resource consumption metrics
type ResourceUsage struct {
	// CPUPercent is the CPU usage percentage (0-100)
	CPUPercent float64 `json:"cpu_percent"`

	// MemoryMB is the memory usage in megabytes
	MemoryMB float64 `json:"memory_mb"`

	// MemoryPercent is the memory usage percentage (0-100)
	MemoryPercent float64 `json:"memory_percent"`

	// DiskUsageMB is the disk space used in megabytes
	DiskUsageMB float64 `json:"disk_usage_mb"`

	// NetworkBytesSent is the total bytes sent over network
	NetworkBytesSent uint64 `json:"network_bytes_sent"`

	// NetworkBytesRecv is the total bytes received over network
	NetworkBytesRecv uint64 `json:"network_bytes_recv"`

	// Goroutines is the number of active goroutines
	Goroutines int `json:"goroutines"`

	// UpdatedAt is when these metrics were last updated
	UpdatedAt time.Time `json:"updated_at"`
}

// CollectorStatus represents the status of a specific collector
type CollectorStatus struct {
	// Name is the collector name
	Name string `json:"name"`

	// Type is the collector type (file, docker, syslog, etc.)
	Type string `json:"type"`

	// Status is the current status
	Status AgentStatus `json:"status"`

	// LogsCollected is the total number of logs collected
	LogsCollected uint64 `json:"logs_collected"`

	// BytesCollected is the total bytes collected
	BytesCollected uint64 `json:"bytes_collected"`

	// ErrorCount is the number of errors encountered
	ErrorCount uint64 `json:"error_count"`

	// LastError is the last error message (if any)
	LastError string `json:"last_error,omitempty"`

	// LastActive is when this collector last processed a log
	LastActive time.Time `json:"last_active"`
}

// AgentConfig contains agent configuration metadata
type AgentConfig struct {
	// BufferType is the type of buffer used (memory, disk)
	BufferType string `json:"buffer_type"`

	// BufferSizeMB is the buffer size in megabytes
	BufferSizeMB int `json:"buffer_size_mb"`

	// Compression indicates if compression is enabled
	Compression bool `json:"compression"`

	// CompressionType is the compression algorithm (gzip, snappy, lz4)
	CompressionType string `json:"compression_type,omitempty"`

	// BatchSize is the number of logs per batch
	BatchSize int `json:"batch_size"`

	// FlushInterval is how often to flush the buffer
	FlushInterval time.Duration `json:"flush_interval"`

	// ServerEndpoints are the pipeline server endpoints
	ServerEndpoints []string `json:"server_endpoints"`
}

// NewAgent creates a new agent instance
func NewAgent(id, hostname, ip string) *Agent {
	now := time.Now()
	return &Agent{
		ID:              id,
		Hostname:        hostname,
		IP:              ip,
		LastHeartbeat:   now,
		Status:          AgentStatusHealthy,
		CollectorStatus: make(map[string]CollectorStatus),
		Labels:          make(map[string]string),
		RegisteredAt:    now,
		Resources: ResourceUsage{
			UpdatedAt: now,
		},
	}
}

// IsHealthy returns true if the agent is healthy
func (a *Agent) IsHealthy() bool {
	return a.Status == AgentStatusHealthy
}

// IsOffline returns true if the agent hasn't sent heartbeat recently
func (a *Agent) IsOffline(timeout time.Duration) bool {
	return time.Since(a.LastHeartbeat) > timeout
}

// UpdateHeartbeat updates the agent's last heartbeat timestamp
func (a *Agent) UpdateHeartbeat() {
	a.LastHeartbeat = time.Now()
	if a.Status == AgentStatusOffline {
		a.Status = AgentStatusHealthy
	}
}

// UpdateResources updates the agent's resource usage metrics
func (a *Agent) UpdateResources(cpu, memory, memoryPercent, disk float64, goroutines int) {
	a.Resources.CPUPercent = cpu
	a.Resources.MemoryMB = memory
	a.Resources.MemoryPercent = memoryPercent
	a.Resources.DiskUsageMB = disk
	a.Resources.Goroutines = goroutines
	a.Resources.UpdatedAt = time.Now()
}

// UpdateCollectorStatus updates the status of a specific collector
func (a *Agent) UpdateCollectorStatus(name string, status CollectorStatus) {
	if a.CollectorStatus == nil {
		a.CollectorStatus = make(map[string]CollectorStatus)
	}
	a.CollectorStatus[name] = status
}

// AddLabel adds or updates a label
func (a *Agent) AddLabel(key, value string) {
	if a.Labels == nil {
		a.Labels = make(map[string]string)
	}
	a.Labels[key] = value
}

// GetLabel retrieves a label value
func (a *Agent) GetLabel(key string) (string, bool) {
	if a.Labels == nil {
		return "", false
	}
	val, ok := a.Labels[key]
	return val, ok
}

// TotalLogsCollected returns the total number of logs collected across all collectors
func (a *Agent) TotalLogsCollected() uint64 {
	var total uint64
	for _, status := range a.CollectorStatus {
		total += status.LogsCollected
	}
	return total
}

// TotalBytesCollected returns the total bytes collected across all collectors
func (a *Agent) TotalBytesCollected() uint64 {
	var total uint64
	for _, status := range a.CollectorStatus {
		total += status.BytesCollected
	}
	return total
}
