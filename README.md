# LogPipeline - Log Aggregation System

A production-ready log aggregation pipeline built entirely in Go, similar to ELK Stack but lightweight and optimized for performance.

## Features

- **Lightweight Agent**: Minimal resource footprint (<50MB memory, <5% CPU)
- **Multiple Input Sources**: Files, Syslog, Docker, Kubernetes, HTTP
- **Real-time Processing**: Stream logs with low latency
- **Powerful Search**: Full-text search with Lucene-like query syntax
- **Metrics Export**: Prometheus and StatsD support
- **Alerting**: Pattern-based alerts with multiple notification channels
- **Scalability**: Horizontal scaling with clustering support
- **Security**: TLS, authentication, and RBAC

## Architecture

```
Log Sources → Agent → Pipeline Server → Parser → Enrichment → Storage
                ↓                           ↓         ↓          ↓
            Buffer                      Indexer   Metrics    Query API
```

## Components

- **Agent**: Lightweight log collector deployed on hosts
- **Server**: Central processing and storage server
- **CLI**: Management command-line tool
- **UI**: Web-based dashboard for log exploration

## Project Structure

```
logpipeline/
├── cmd/                  # Main applications
│   ├── agent/           # Log collector agent
│   ├── server/          # Pipeline server
│   ├── cli/             # CLI tool
│   └── ui/              # Web UI server
├── internal/            # Private application code
│   ├── agent/           # Agent implementation
│   ├── pipeline/        # Processing pipeline
│   ├── storage/         # Storage backends
│   ├── query/           # Query engine
│   ├── metrics/         # Metrics generation
│   └── alert/           # Alerting system
├── pkg/                 # Public libraries
│   ├── models/          # Data models
│   ├── protocol/        # Wire protocols
│   ├── parser/          # Parser library
│   ├── compress/        # Compression utilities
│   └── config/          # Configuration
├── web/                 # Web UI assets
├── configs/             # Sample configurations
├── deployments/         # Deployment configs
└── tests/              # Test suites
```

## Performance Targets

- **Agent**: <50MB memory, <5% CPU
- **Ingest Rate**: 100,000 logs/second
- **Query Latency**: <100ms for recent data
- **Compression**: 10:1 ratio
- **Scalability**: Support 10,000+ agents

## Getting Started

### Prerequisites

- Go 1.21 or higher
- Docker (optional, for containerized deployment)
- Kubernetes (optional, for K8s deployment)

### Installation

```bash
# Clone the repository
git clone https://github.com/UmangDiyora/logpipeline.git
cd logpipeline

# Build all components
make build

# Or build specific components
make build-agent
make build-server
make build-cli
```

### Quick Start

```bash
# Start the server
./bin/logserver -config configs/server.yaml

# Start the agent
./bin/logagent -config configs/agent.yaml

# Query logs using CLI
./bin/logcli query "level:ERROR"
```

## Configuration

See the `configs/` directory for sample configuration files:
- `agent.yaml` - Agent configuration
- `server.yaml` - Server configuration

## Development Status

✅ **Completed Phases:**
- Phase 1: Project setup and core data models
- Phase 2: Agent implementation (buffer, shipper, collectors)
- Phase 3: Input plugins (file, syslog, HTTP)
- Phase 4: Pipeline server (receiver, parser engine)
- Phase 5: Storage layer (time-partitioned file storage)
- Phase 6: Query engine with caching and aggregations
- Phase 7: Main applications and configuration
- Phase 8: Testing and build verification

**Current Status:** All core components implemented and tested. Ready for deployment and further enhancement.

## Testing

Run tests:
```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Run benchmarks
make bench
```

Test coverage: ~45% overall, with critical paths well-tested.

## License

MIT License

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.
