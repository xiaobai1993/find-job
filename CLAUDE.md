# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a **Go backend developer interview preparation repository** focused on payment business direction. It contains structured technical notes, demo code, and interview questions organized by topic.

## Repository Architecture

The content is organized into numbered directories by knowledge domain:

| Directory | Purpose |
|-----------|---------|
| `00-go-version-features/` | Go version-specific features (1.18-1.23) with examples |
| `01-go-basics/` | Go language fundamentals: slice, map, string, interface, generics, error handling, defer, etc. |
| `02-go-concurrency/` | Concurrency primitives: goroutine, channel, sync package, context, patterns, race conditions |
| `03-go-runtime/` | Runtime internals: GMP scheduling, memory allocation, GC, pprof profiling |
| `04-database/` | MySQL, PostgreSQL, Redis, DynamoDB theory and best practices |
| `05-middleware/` | Message queues: Pulsar, Kafka, SQS |
| `06-distributed-system/` | Consensus algorithms, distributed locks, ID generation, transactions, circuit breaking, rate limiting |
| `07-microservice/` | gRPC, service discovery, API gateway, service mesh, observability |
| `08-system-design/` | Payment systems, order systems, reconciliation, idempotency, high concurrency design |
| `09-infra/` | Docker, Kubernetes, Terraform, ArgoCD, AWS, CI/CD |
| `10-algorithm/` | LeetCode problem solutions and patterns |
| `11-project-experience/` | STAR-formatted project experience documentation |
| `12-interview-qa/` | Interview questions with answers organized by topic |
| `reference/` | Reference materials |

## File Patterns

- **Theory files**: `XX-topic.md` - Markdown documentation with deep dives
- **Demo files**: `*-demo.go` - Runnable Go code demonstrating concepts
- **Test files**: `*_test.go` - Go test files
- **HTML visualizations**: `*.html` - Interactive visualizations (e.g., map internals)

## Common Development Commands

### Go Code Execution

```bash
# Run a Go demo file
go run path/to/file.go

# Run tests
go test ./...

# Run tests with race detection
go test -race ./...

# Run specific test file
go test path/to/file_test.go

# Run with Go version features
# Note: Repository uses Go 1.22.12, but demos may test 1.23 features
go run go1.23 ./path/to/file.go
```

### Code Quality

```bash
# Format Go code
go fmt ./...

# Vet code for issues
go vet ./...

# Run race detector on demo code
go run -race path/to/demo.go
```

## Key Conventions

1. **Each topic has both theory and code**: A markdown file for documentation and a `-demo.go` file for runnable examples
2. **Interview-focused content**: All material targets technical interview preparation, not production application development
3. **Payment domain emphasis**: System design focuses on payment, reconciliation, and order systems
4. **Go version coverage**: Examples cover Go 1.18 through 1.23 features

## Important Notes for Code Generation

1. When creating new demo files, follow the `*-demo.go` naming convention
2. Test files should be placed in the same directory as the code they test
3. Race condition examples should include comments explaining the issue and how to fix it
4. Include `go:build` directives for version-specific Go features when necessary

## Common Tasks

### Adding a New Topic

1. Create the markdown theory file: `XX-topic-name.md`
2. Create the demo file: `topic-name-demo.go` with runnable examples
3. If applicable, create test file: `topic_test.go`

### Running Example Code

```bash
# Example: Run memory alignment demo
go run 01-go-basics/15-memory-alignment-demo.go

# Example: Run race condition demo with race detector
go run -race 02-go-concurrency/12-race-condition-demo.go

# Example: Test init order
go test -v 01-go-basics/order_test.go
```
