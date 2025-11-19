package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/UmangDiyora/logpipeline/internal/pipeline"
	"github.com/UmangDiyora/logpipeline/internal/pipeline/receiver"
	"github.com/UmangDiyora/logpipeline/internal/query"
	"github.com/UmangDiyora/logpipeline/internal/storage"
	"github.com/UmangDiyora/logpipeline/pkg/config"
	"github.com/UmangDiyora/logpipeline/pkg/models"
)

var (
	configFile = flag.String("config", "configs/server.yaml", "Path to configuration file")
	version    = "1.0.0"
)

func main() {
	flag.Parse()

	fmt.Printf("LogPipeline Server v%s\n", version)
	fmt.Printf("Loading configuration from: %s\n", *configFile)

	// Load configuration
	cfg, err := config.LoadServerConfig(*configFile)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		fmt.Println("Using default configuration...")
		cfg = defaultConfig()
	}

	// Create storage
	storageConfig := &storage.Config{
		Path:              cfg.Storage.Path,
		RetentionDays:     30,
		PartitionInterval: cfg.Storage.CompactionInterval,
		SyncWrites:        false,
	}

	store, err := storage.New(storageConfig)
	if err != nil {
		fmt.Printf("Fatal: failed to create storage: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	fmt.Printf("Storage initialized at: %s\n", cfg.Storage.Path)

	// Create query engine
	queryEngine := query.NewEngine(store, query.DefaultConfig())
	fmt.Println("Query engine initialized")

	// Create channels
	receiverOutput := make(chan *models.LogEntry, 10000)
	pipelineOutput := make(chan *models.LogEntry, 10000)

	// Create receiver
	receiverConfig := &receiver.Config{
		HTTPAddr:     fmt.Sprintf(":%d", cfg.Server.HTTPPort),
		MaxBatchSize: 10000,
		RateLimit:    1000,
	}

	recv, err := receiver.New(receiverConfig, receiverOutput)
	if err != nil {
		fmt.Printf("Fatal: failed to create receiver: %v\n", err)
		os.Exit(1)
	}

	if err := recv.Start(); err != nil {
		fmt.Printf("Fatal: failed to start receiver: %v\n", err)
		os.Exit(1)
	}
	defer recv.Stop()

	fmt.Printf("HTTP receiver listening on port %d\n", cfg.Server.HTTPPort)

	// Create pipelines
	pipelines := make([]*pipeline.Pipeline, 0)
	for _, pipeConfig := range cfg.Pipelines {
		pipelineConfig := &pipeline.Config{
			ID:      pipeConfig.Name,
			Name:    pipeConfig.Name,
			Workers: 4,
		}

		pipe, err := pipeline.New(pipelineConfig, receiverOutput, pipelineOutput)
		if err != nil {
			fmt.Printf("Warning: failed to create pipeline %s: %v\n", pipeConfig.Name, err)
			continue
		}

		pipelines = append(pipelines, pipe)
		fmt.Printf("Pipeline '%s' initialized\n", pipeConfig.Name)
	}

	// Start storage writer
	go func() {
		for entry := range pipelineOutput {
			if err := store.Write(entry); err != nil {
				fmt.Printf("Error writing to storage: %v\n", err)
			}
		}
	}()

	fmt.Println("\nServer started successfully!")
	fmt.Printf("HTTP API: http://localhost:%d\n", cfg.Server.HTTPPort)
	fmt.Printf("Ingestion endpoint: http://localhost:%d/api/v1/logs/ingest\n", cfg.Server.HTTPPort)
	fmt.Printf("Health endpoint: http://localhost:%d/api/v1/health\n", cfg.Server.HTTPPort)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	fmt.Println("\nShutdown signal received, stopping server...")

	// Stop pipelines
	for _, pipe := range pipelines {
		if err := pipe.Stop(); err != nil {
			fmt.Printf("Error stopping pipeline: %v\n", err)
		}
	}

	// Print statistics
	stats := queryEngine.Stats()
	fmt.Printf("\nFinal statistics:\n")
	fmt.Printf("  Total entries: %v\n", stats["total_entries"])
	fmt.Printf("  Cache size: %v\n", stats["cache_size"])

	fmt.Println("Server stopped gracefully")
}

func defaultConfig() *config.ServerConfig {
	return &config.ServerConfig{
		Server: config.ServerSettings{
			GRPCPort: 9090,
			HTTPPort: 8080,
			LogLevel: "info",
		},
		Storage: config.StorageConfig{
			Type: "file",
			Path: "/tmp/logpipeline/data",
		},
		Index: config.IndexConfig{
			Type: "memory",
			Path: "/tmp/logpipeline/index",
		},
		Pipelines: []config.PipelineConfig{
			{
				Name:   "default",
				Parser: "json",
			},
		},
		Metrics: config.MetricsConfig{
			Enabled: true,
			Port:    2112,
		},
	}
}
