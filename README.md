# Go Project Starter

> **Transform your API specifications into production-ready microservices in minutes, not days.**

A powerful, opinionated microservice scaffolding tool that generates complete, production-grade Go services from YAML configuration files. Built for developers who need to rapidly build business services with REST APIs, gRPC, background workers, and event-driven architectures—without sacrificing code quality or flexibility.

[![Go Version](https://img.shields.io/badge/Go-1.24.4+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE.txt)
[![GitHub Stars](https://img.shields.io/github/stars/Educentr/go-project-starter?style=flat&logo=github)](https://github.com/Educentr/go-project-starter/stargazers)
[![Code Generation](https://img.shields.io/badge/Generated%20Code-8K%2B%20lines-blue?style=flat)](#)
[![Templates](https://img.shields.io/badge/Templates-78+-green?style=flat)](#)

---

## Why Go Project Starter?

Building a production-ready microservice from scratch typically takes **2-3 weeks** of repetitive boilerplate work. Go Project Starter **reduces this to 5 minutes** by generating 100% of your infrastructure code from a single YAML configuration file.

**The game-changer?** Unlike traditional scaffolding tools that generate code once and leave you on your own, Go Project Starter uses intelligent disclaimer markers to separate generated code from your custom business logic. This means you can **regenerate your entire project** as your API evolves without losing your manual changes.

---

## Quick Start

### Installation

```bash
go install github.com/Educentr/go-project-starter/cmd/go-project-starter@latest
```

### Generate Your First Service

**1. Create a configuration file (`config.yaml`):**

```yaml
main:
  name: myservice
  logger: zerolog
  use_active_record: true

git:
  repo: github.com/myorg/myservice
  module_path: github.com/myorg/myservice

rest:
  - name: api
    path: [./api/openapi.yaml]
    generator_type: ogen
    port: 8080
    version: v1

  - name: system
    generator_type: template
    generator_template: sys
    port: 9090
    version: v1

applications:
  - name: server
    transport: [api, system]
```

**2. Run the generator:**

```bash
go-project-starter --config=config.yaml
```

**3. Start developing:**

```bash
cd myservice
make docker-up    # Start dependencies (Postgres, Redis, etc.)
make generate     # Generate code from OpenAPI specs
make run          # Start your service
```

**Access your service:**
- REST API: `http://localhost:8080`
- System endpoints: `http://localhost:9090` (health, metrics, pprof)

---

## Key Features

### Intelligent Code Preservation

Regenerate your entire project structure as your APIs evolve—add new endpoints, modify data models, update dependencies—without losing a single line of your custom business logic.

```go
// ==========================================
// GENERATED CODE - DO NOT EDIT ABOVE THIS LINE
// ==========================================

func (h *Handler) CreateUser(ctx context.Context, req *CreateUserRequest) (*User, error) {
    // Your custom business logic here - survives regeneration!
}
```

### Multi-Protocol Support

- **REST APIs** (OpenAPI 3.0 via [ogen](https://github.com/ogen-go/ogen))
- **gRPC Services** (Protocol Buffers v3)
- **Kafka Consumers** (event-driven architecture)
- **Background Workers** (Telegram bots, daemons)

### Application-Based Scaling

One codebase, multiple deployment profiles:

```yaml
applications:
  - name: gateway
    transport: [rest_api, grpc_users]

  - name: workers
    workers: [telegram_bot, kafka_consumer]
```

### Production-Ready Infrastructure

Every generated project includes:
- Docker & Docker Compose (multi-stage builds, ~50MB images)
- GitHub Actions CI/CD
- Traefik reverse proxy
- Prometheus metrics & Grafana dashboards
- Health checks & graceful shutdown
- Structured logging (zerolog)

---

## What You Get

```
myservice/                    # ~50 files, ~8,000 lines of production-ready code
├── cmd/server/              # Application entry point with graceful shutdown
├── internal/
│   ├── app/                 # Transport handlers & workers (your code goes here)
│   └── pkg/                 # Business logic & repositories
├── pkg/                     # Runtime libraries (middleware, logging, metrics)
├── docker-compose.yaml      # Local dev env (Postgres, Redis, Traefik)
├── Dockerfile               # Multi-stage build (50MB final image)
├── Makefile                 # 40+ targets (build, test, lint, deploy)
└── .github/workflows/       # CI/CD (test, build, push to registry)
```

---

## Documentation

| Section | Description |
|---------|-------------|
| [Quick Start](docs/quick-start.md) | Installation and first project in 5 minutes |
| [Architecture](docs/architecture.md) | Three-layer design, Applications, Drivers |
| [Configuration](docs/configuration.md) | Complete YAML configuration guide |
| [Features](docs/features.md) | All generator capabilities |
| [Examples](docs/examples.md) | Ready-to-use configurations |
| [Advanced Topics](docs/advanced.md) | Drivers, OnlineConf, monitoring, Grafana |
| [Comparison](docs/comparison.md) | vs Goa, go-zero, Sponge, grpc-gateway |
| [Contributing](docs/contributing.md) | How to contribute |

### Local Documentation

```bash
docker build -f docs/Dockerfile -t mkdocs-app .
docker run -p 8000:8000 -v $(pwd):/docs mkdocs-app
```

Then open http://localhost:8000

---

## License

This project is licensed under the MIT License - see the [LICENSE.txt](LICENSE.txt) file for details.

---

## Acknowledgments

Built with these amazing open-source projects:

- [ogen](https://github.com/ogen-go/ogen) - OpenAPI 3.0 code generation
- [go-activerecord](https://github.com/Educentr/go-activerecord) - Database ORM and migrations
- [zerolog](https://github.com/rs/zerolog) - Structured logging
- [Prometheus](https://prometheus.io/) - Monitoring and metrics
- [Traefik](https://traefik.io/) - Reverse proxy and load balancer

---

## Getting Help & Support

- [Documentation](docs/index.md) - Complete guides and API references
- [GitHub Discussions](https://github.com/Educentr/go-project-starter/discussions) - Ask questions, share ideas
- [Issue Tracker](https://github.com/Educentr/go-project-starter/issues) - Report bugs, request features

---

<div align="center">

**Stop writing boilerplate. Start building features.**

```bash
go install github.com/Educentr/go-project-starter/cmd/go-project-starter@latest
```

**[Get Started](docs/quick-start.md)** • **[View Examples](docs/examples.md)** • **[Read Docs](docs/index.md)**

</div>
