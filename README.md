# Backlog Manager (`bm`)

A feature management and planning tool that streamlines transforming underspecified feature ideas into fully-specified requirements, implementation plans, and actionable work items. It integrates with the [Beads Server](https://github.com/vector76/beads_server) issue tracker and drives an iterative AI-assisted dialog workflow.

## Overview

Backlog Manager uses a client-server architecture:

- **Server** — manages centralized state (projects, features, dialog history) via a REST API and web dashboard
- **Client** (`bm` CLI) — runs in project source directories and drives Claude-powered workflow steps for feature refinement and code generation

Multiple clients can work on different projects simultaneously, all coordinated through a single server.

## Prerequisites

- [Go 1.22+](https://go.dev/dl/)
- Git

## Installation

```bash
git clone https://github.com/vector76/backlog_manager
cd backlog_manager
go build -o bm ./cmd/bm
```

Move the binary to somewhere on your `PATH`, e.g.:

```bash
mv bm /usr/local/bin/bm
```

## Server Setup

### 1. Create a config file

Run the interactive wizard to generate a config file:

```bash
bm init
```

It will prompt for each value with sensible defaults:

```
Port [8080]:
Data directory [~/.local/share/bm]:
Dashboard user [admin]:
Dashboard password:
Beads Server URL (optional, press Enter to skip):
Config written to config.json
```

By default the file is written to `config.json` in the current directory. Use `--output` to specify a different path:

```bash
bm init --output /etc/bm/config.json
bm serve --config /etc/bm/config.json
```

| Field | Required | Description |
|---|---|---|
| `port` | Yes | HTTP server port (default: 8080) |
| `data_dir` | Yes | Directory where feature/project data is stored |
| `dashboard_user` | Yes | Web dashboard login username (default: admin) |
| `dashboard_password` | Yes | Web dashboard login password |
| `beads_server_url` | No | URL of a Beads Server instance for bead status monitoring |

### 2. Start the server

```bash
bm serve --config config.json
```

The web dashboard will be available at `http://localhost:8080`.

### 3. Create a project

Log into the web dashboard and create a new project. You will receive an authentication **token** — save this, as clients need it to connect.

## Client Setup

In your project's source directory, create a `.env` file (or export the variables in your shell profile):

```bash
BM_URL=http://localhost:8080
BM_TOKEN=<token-from-dashboard>
# BM_PROJECT=<project-name>    (optional: validates token belongs to this project)
```

The client reads `.env` from the current directory automatically; environment variables take priority over the file.

## Client Commands

```bash
# Check server connectivity and list features
bm status

# List all features for this project
bm features

# Show full detail for a feature
bm show <feature-id>

# Poll for the next action (dialog step or code generation)
bm poll

# Fetch feature data and dialog history
bm fetch <feature-id>

# Submit a dialog response
bm submit <feature-id> [options]

# Start code generation for a feature
bm start-generate <feature-id>

# Register created beads with a feature
bm register-beads <feature-id> <bead-ids...>

# Register generated artifacts for tracking
bm register-artifact <feature-id> [options]

# Mark a feature as complete
bm complete <feature-id>
```

Run `bm --help` or `bm <command> --help` for full usage details.

## Typical Workflow

1. **Add a feature** via the web dashboard (or API)
2. **Start dialog** — the server directs the client to refine the feature description iteratively with Claude
3. **Mark as specified** — once the feature is fully described, trigger code generation
4. **Generate code** — the client produces an implementation plan and registers beads in Beads Server
5. **Monitor beads** — the dashboard shows bead completion status
6. **Close** — mark the feature done once all beads are resolved

## Running Tests

```bash
# All tests
go test ./...

# Specific package
go test ./internal/server

# E2E tests
go test ./e2e

# With verbose output
go test -v ./...

# With coverage
go test -cover ./...
```

## Project Structure

```
cmd/bm/             CLI entry point
internal/
  model/            Data types and enums
  store/            File-based persistence
  server/           HTTP server, REST API, web dashboard
  cli/              Cobra CLI commands
  client/           HTTP client for API calls
  beadsserver/      Beads Server integration
  config/           Configuration loading
e2e/                End-to-end tests
docs/               Architecture and design documentation
```

## Documentation

See the `docs/` folder for architecture details, data model, and design decisions.
