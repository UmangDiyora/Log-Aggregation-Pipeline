package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/UmangDiyora/logpipeline/internal/agent/buffer"
	"github.com/UmangDiyora/logpipeline/internal/agent/collector"
	"github.com/UmangDiyora/logpipeline/internal/agent/shipper"
	"github.com/UmangDiyora/logpipeline/pkg/config"
)

var (
	configFile = flag.String("config", "configs/agent.yaml", "Path to configuration file")
	version    = "1.0.0"
)

func main() {
	flag.Parse()

	fmt.Printf("LogPipeline Agent v%s\n", version)
	fmt.Printf("Loading configuration from: %s\n", *configFile)

	// Load configuration
	cfg, err := config.LoadAgentConfig(*configFile)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		fmt.Println("Using default configuration...")
		cfg = defaultConfig()
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create buffer
	bufferConfig := &buffer.Config{
		Type:          cfg.Buffer.Type,
		MaxSize:       parseSize(cfg.Buffer.Size),
		Path:          cfg.Buffer.Path,
		FlushInterval: cfg.Buffer.FlushInterval,
		MaxBatchSize:  1000,
	}

	buf, err := buffer.New(bufferConfig)
	if err != nil {
		fmt.Printf("Fatal: failed to create buffer: %v\n", err)
		os.Exit(1)
	}
	defer buf.Close()

	// Create shipper
	shipperConfig := &shipper.Config{
		Endpoints:    cfg.Output.Hosts,
		Compression:  cfg.Output.Compression,
		BatchSize:    cfg.Output.BatchSize,
		BatchTimeout: cfg.Output.BatchTimeout,
		MaxRetries:   cfg.Output.MaxRetries,
		APIKey:       cfg.Output.APIKey,
	}

	ship, err := shipper.New(shipperConfig, cfg.Agent.ID)
	if err != nil {
		fmt.Printf("Fatal: failed to create shipper: %v\n", err)
		os.Exit(1)
	}
	defer ship.Close()

	// Create collectors
	collectors := make([]collector.Collector, 0)

	for _, input := range cfg.Inputs {
		if !input.Enabled {
			continue
		}

		var coll collector.Collector
		switch input.Type {
		case "file":
			fileConfig := &collector.FileCollectorConfig{
				Paths:      input.Paths,
				Exclude:    input.Exclude,
				Source:     input.Name,
				Host:       cfg.Agent.ID,
				BufferSize: 64 * 1024,
			}
			coll, err = collector.NewFileCollector(input.Name, fileConfig)

		case "syslog":
			syslogConfig := &collector.SyslogCollectorConfig{
				Address:  input.Address,
				Protocol: input.Protocol,
				Source:   input.Name,
				Host:     cfg.Agent.ID,
			}
			coll, err = collector.NewSyslogCollector(input.Name, syslogConfig)

		case "http":
			httpConfig := &collector.HTTPCollectorConfig{
				ListenAddress: input.ListenAddress,
				TLSEnabled:    input.TLS,
				Source:        input.Name,
				Host:          cfg.Agent.ID,
			}
			coll, err = collector.NewHTTPCollector(input.Name, httpConfig)

		default:
			fmt.Printf("Warning: unsupported input type: %s\n", input.Type)
			continue
		}

		if err != nil {
			fmt.Printf("Warning: failed to create collector %s: %v\n", input.Name, err)
			continue
		}

		collectors = append(collectors, coll)
	}

	if len(collectors) == 0 {
		fmt.Println("Fatal: no collectors configured")
		os.Exit(1)
	}

	// Start collectors
	fmt.Printf("Starting %d collector(s)...\n", len(collectors))
	for _, coll := range collectors {
		if err := coll.Start(ctx); err != nil {
			fmt.Printf("Warning: failed to start collector %s: %v\n", coll.Name(), err)
			continue
		}
		fmt.Printf("  ✓ %s (%s) started\n", coll.Name(), coll.Type())

		// Start goroutine to forward logs from collector to shipper
		go func(c collector.Collector) {
			for entry := range c.Output() {
				if err := ship.Ship(entry); err != nil {
					fmt.Printf("Warning: failed to ship log: %v\n", err)
				}
			}
		}(coll)
	}

	fmt.Println("\nAgent started successfully!")
	fmt.Printf("Agent ID: %s\n", cfg.Agent.ID)
	fmt.Printf("Shipping to: %v\n", cfg.Output.Hosts)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	fmt.Println("\nShutdown signal received, stopping agent...")

	// Stop collectors
	for _, coll := range collectors {
		if err := coll.Stop(); err != nil {
			fmt.Printf("Error stopping collector %s: %v\n", coll.Name(), err)
		}
	}

	fmt.Println("Agent stopped gracefully")
}

func defaultConfig() *config.AgentConfig {
	return &config.AgentConfig{
		Agent: config.AgentSettings{
			ID:                "default-agent",
			HeartbeatInterval: 30 * time.Second,
		},
		Inputs: []config.InputConfig{
			{
				Type:    "file",
				Name:    "system-logs",
				Enabled: true,
				Paths:   []string{"/var/log/*.log"},
			},
		},
		Output: config.OutputConfig{
			Type:         "grpc",
			Hosts:        []string{"http://localhost:8080"},
			Compression:  "gzip",
			BatchSize:    1000,
			BatchTimeout: 5 * time.Second,
			MaxRetries:   3,
		},
		Buffer: config.BufferConfig{
			Type: "memory",
			Size: "10000",
			Path: "/tmp/logagent",
		},
	}
}

func parseSize(size string) int64 {
	// Simple size parser (for production, use a proper parser)
	var value int64
	fmt.Sscanf(size, "%d", &value)
	return value
}
