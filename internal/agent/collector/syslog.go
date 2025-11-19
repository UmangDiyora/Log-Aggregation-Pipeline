package collector

import (
	"bufio"
	"context"
	"crypto/md5"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"time"

	"github.com/UmangDiyora/logpipeline/pkg/models"
)

// SyslogCollectorConfig holds syslog collector configuration
type SyslogCollectorConfig struct {
	// Address to listen on (e.g., "0.0.0.0:514")
	Address string

	// Protocol (udp or tcp)
	Protocol string

	// Source identifier
	Source string

	// Host identifier
	Host string
}

// SyslogCollector collects logs via syslog
type SyslogCollector struct {
	*BaseCollector
	config   *SyslogCollectorConfig
	listener net.Listener
	conn     *net.UDPConn
}

// NewSyslogCollector creates a new syslog collector
func NewSyslogCollector(name string, config *SyslogCollectorConfig) (*SyslogCollector, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}

	if config.Protocol == "" {
		config.Protocol = "udp"
	}

	base := NewBaseCollector(name, "syslog", 1000)

	sc := &SyslogCollector{
		BaseCollector: base,
		config:        config,
	}

	return sc, nil
}

// Start starts the syslog collector
func (sc *SyslogCollector) Start(ctx context.Context) error {
	var err error

	switch sc.config.Protocol {
	case "udp":
		err = sc.startUDP()
	case "tcp":
		err = sc.startTCP()
	default:
		return fmt.Errorf("unsupported protocol: %s", sc.config.Protocol)
	}

	return err
}

// Stop stops the syslog collector
func (sc *SyslogCollector) Stop() error {
	sc.Close()

	if sc.listener != nil {
		sc.listener.Close()
	}

	if sc.conn != nil {
		sc.conn.Close()
	}

	return nil
}

// startUDP starts a UDP listener
func (sc *SyslogCollector) startUDP() error {
	addr, err := net.ResolveUDPAddr("udp", sc.config.Address)
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP: %w", err)
	}

	sc.conn = conn

	go sc.handleUDP()

	return nil
}

// startTCP starts a TCP listener
func (sc *SyslogCollector) startTCP() error {
	listener, err := net.Listen("tcp", sc.config.Address)
	if err != nil {
		return fmt.Errorf("failed to listen on TCP: %w", err)
	}

	sc.listener = listener

	go sc.handleTCP()

	return nil
}

// handleUDP handles UDP connections
func (sc *SyslogCollector) handleUDP() {
	buffer := make([]byte, 65536)

	for {
		select {
		case <-sc.ctx.Done():
			return
		default:
			n, _, err := sc.conn.ReadFromUDP(buffer)
			if err != nil {
				sc.status.ErrorCount++
				sc.status.LastError = err.Error()
				continue
			}

			message := string(buffer[:n])
			entry := sc.parseSyslog(message)
			sc.Emit(entry)
		}
	}
}

// handleTCP handles TCP connections
func (sc *SyslogCollector) handleTCP() {
	for {
		select {
		case <-sc.ctx.Done():
			return
		default:
			conn, err := sc.listener.Accept()
			if err != nil {
				sc.status.ErrorCount++
				sc.status.LastError = err.Error()
				continue
			}

			go sc.handleConnection(conn)
		}
	}
}

// handleConnection handles a single TCP connection
func (sc *SyslogCollector) handleConnection(conn net.Conn) {
	defer conn.Close()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		select {
		case <-sc.ctx.Done():
			return
		default:
			line := scanner.Text()
			entry := sc.parseSyslog(line)
			sc.Emit(entry)
		}
	}

	if err := scanner.Err(); err != nil {
		sc.status.ErrorCount++
		sc.status.LastError = err.Error()
	}
}

// parseSyslog parses a syslog message (RFC3164 format)
func (sc *SyslogCollector) parseSyslog(message string) *models.LogEntry {
	entry := models.NewLogEntry()
	entry.ID = sc.generateID(message)
	entry.Raw = message
	entry.Source = sc.config.Source
	entry.Host = sc.config.Host
	entry.Timestamp = time.Now()

	// Try to parse RFC3164 format: <PRI>TIMESTAMP HOSTNAME MESSAGE
	re := regexp.MustCompile(`^<(\d+)>(\w+\s+\d+\s+\d+:\d+:\d+)\s+(\S+)\s+(.+)$`)
	matches := re.FindStringSubmatch(message)

	if len(matches) == 5 {
		// Parse priority
		priority, _ := strconv.Atoi(matches[1])
		entry.Level = sc.priorityToLevel(priority)

		// Parse timestamp
		if ts, err := time.Parse("Jan 2 15:04:05", matches[2]); err == nil {
			// Use current year
			now := time.Now()
			entry.Timestamp = time.Date(now.Year(), ts.Month(), ts.Day(), ts.Hour(), ts.Minute(), ts.Second(), 0, time.Local)
		}

		// Host
		entry.AddField("syslog_host", matches[3])

		// Message
		entry.Message = matches[4]
	} else {
		// Fallback: use raw message
		entry.Message = message
		entry.Level = models.LogLevelInfo
	}

	return entry
}

// priorityToLevel converts syslog priority to log level
func (sc *SyslogCollector) priorityToLevel(priority int) models.LogLevel {
	severity := priority & 0x07

	switch severity {
	case 0, 1, 2: // Emergency, Alert, Critical
		return models.LogLevelFatal
	case 3: // Error
		return models.LogLevelError
	case 4: // Warning
		return models.LogLevelWarn
	case 5, 6: // Notice, Informational
		return models.LogLevelInfo
	case 7: // Debug
		return models.LogLevelDebug
	default:
		return models.LogLevelInfo
	}
}

// generateID generates a unique ID for a log entry
func (sc *SyslogCollector) generateID(message string) string {
	hash := md5.Sum([]byte(fmt.Sprintf("%s:%d:%s", sc.config.Source, time.Now().UnixNano(), message)))
	return fmt.Sprintf("%x", hash)
}
