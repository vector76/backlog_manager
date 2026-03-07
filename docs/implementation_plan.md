# Implementation Plan

This plan is organized into sequential chunks. Each chunk builds on the
previous and produces a testable, working increment. The architecture mirrors
the beads_server: single Go binary, chi router, cobra CLI, plain file storage,
embedded HTML dashboard.

## Chunk 1: Project Scaffolding and Data Model

**Goal:** Establish the Go module, package structure, and core data types.

**Package layout (mirroring beads_server):**

```
cmd/bm/main.go                Entry point — delegates to cli.NewRootCmd()
internal/
  model/                       Data types and enums
  store/                       File-based persistence and business logic
  server/                      HTTP server, chi router, handlers
  config/                      Server config file loading
  cli/                         Cobra commands, HTTP client
e2e/                           End-to-end tests
docs/                          Documentation
```

In beads_server, `internal/project/` parses the static multi-project
config file. In bm, projects are dynamic (created via UI, managed by
the store), so that package doesn't apply. Instead, `config/` handles
loading and validating the server config JSON (port, data_dir,
dashboard credentials).

**Data model (`internal/model/`):**

- `Project`: name, token, created_at
- `Feature`: id, project, name, status (enum), current_iteration (int),
  generate_after (optional feature ID), bead_ids (list of strings),
  created_at, updated_at
- `DialogIteration`: round number, is_final, created_at

The Feature struct does NOT store description content — descriptions,
questions, and responses live exclusively in versioned markdown files on
disk. The `current_iteration` field tracks the latest dialog round number,
and the latest description is at `description_v{current_iteration}.md`.
The `DialogIteration` struct stores per-round metadata in features.json;
the actual content (feature_description, questions, user_response) is
read from the corresponding files on demand.
- `FeatureStatus` enum: `draft`, `awaiting_client`, `awaiting_human`,
  `fully_specified`, `waiting`, `ready_to_generate`, `generating`,
  `beads_created`, `done`, `halted`, `abandoned`
- `FeatureAction` enum: `dialog_step`, `generate` (returned by poll to tell
  the client what to do)
- ID generation: `ft-` prefix + 4-8 random alphanumeric chars (same scheme as
  beads_server `bd-` IDs)

**Feature lifecycle:**

```
draft ──(start dialog)──> awaiting_client
awaiting_client ──(client submits dialog)──> awaiting_human
awaiting_human ──(user responds)──> awaiting_client
awaiting_human ──(user responds with "final")──> awaiting_client [final flag]
awaiting_client [final flag] ──(client submits, no questions)──> fully_specified
fully_specified ──(reopen with message)──> awaiting_client
fully_specified ──(generate now)──> ready_to_generate
fully_specified ──(generate after X)──> waiting
waiting ──(dependency done)──> ready_to_generate
ready_to_generate ──(client calls start-generate)──> generating
generating ──(beads registered)──> beads_created
beads_created ──(all beads closed, or manual complete)──> done
any ──(abandon/halt)──> abandoned/halted
```

The dialog phase uses two distinct statuses (`awaiting_client` and
`awaiting_human`) to make it unambiguous whose turn it is. The poll
endpoint returns features in `awaiting_client` or `ready_to_generate`
status.

The "final answer" flow works as follows: the user submits a response marked
as final. The feature transitions to `awaiting_client` as usual. The client
picks it up, processes it, and submits an updated description with no
questions. Because the previous response was marked final and there are no
new questions, the server transitions the feature to `fully_specified`.

**Deliverables:**
- `go.mod` with module name (to be decided, e.g.
  `github.com/vector76/backlog_manager` or similar)
- Model types with JSON serialization and enum validation
- Unit tests for model types
- Documentation: `docs/data-model.md`

## Chunk 2: File-Based Storage

**Goal:** Implement persistence using plain files, similar to beads_server but
with a directory structure instead of a single JSON file.

**Storage layout:**

```
data/
  projects.json                  Project registry (list of projects + tokens)
  <project-name>/
    features.json                Feature metadata (list of features, statuses)
    features/
      <feature-id>/
        description_v0.md        Initial feature description (user-created)
        description_v1.md        Updated by client, round 1
        questions_v1.md          Client questions/assumptions, round 1
        response_v1.md           User response to round 1
        description_v2.md        Updated by client, round 2
        questions_v2.md          Client questions/assumptions, round 2
        response_v2.md           User response to round 2, etc.
        plan.md                  Plan document (registered artifact)
        beads.md                 Beads document (registered artifact)
```

Version numbering: `description_v0` is the user's initial description.
For round N, `description_vN`, `questions_vN`, and `response_vN` all
refer to the same dialog round. This keeps the numbering consistent
across file types.

**Store (`internal/store/`):**

- JSON metadata (`projects.json`, `features.json`) loaded into memory on
  startup, protected by `sync.RWMutex`
- Markdown files (descriptions, questions, responses) are NOT held in memory;
  they are read from disk on demand and written directly. This avoids loading
  potentially large dialog histories into RAM.
- Atomic writes (temp file + rename) for JSON metadata files
- CRUD for projects and features
- Dialog iteration management (add iteration, get history)
- Feature status transitions with validation

**Deliverables:**
- Store implementation with all CRUD operations
- Store tests using temp directories
- Atomic file write utilities

## Chunk 3: Server Foundation and Project Management

**Goal:** HTTP server with auth, project management API, and basic client
endpoints.

**Server config file (`config.json`):**

```json
{
  "port": 8080,
  "data_dir": "./data",
  "dashboard_user": "admin",
  "dashboard_password": "changeme"
}
```

**Authentication:**
- Client requests: `Authorization: Bearer <token>` (token maps to project)
- Dashboard: HTTP basic auth or session-based login with username/password
  from config

**API endpoints (all under `/api/v1/`):**

| Method | Endpoint              | Auth      | Purpose                        |
|--------|-----------------------|-----------|--------------------------------|
| GET    | `/health`             | None      | Health check                   |
| GET    | `/version`            | None      | Server version                 |
| POST   | `/projects`           | Dashboard | Create project, returns token  |
| GET    | `/projects`           | Dashboard | List all projects              |
| GET    | `/projects/{name}`    | Dashboard | Get project info (feature counts, client status) |
| DELETE | `/projects/{name}`    | Dashboard | Remove project                 |
| GET    | `/project`            | Token     | Get own project info (backing `bm status`)       |

The token uniquely determines the project, so `GET /project` (singular,
no name) returns info for the token's project. The dashboard uses
`GET /projects/{name}` for the same data.

**Initial CLI commands (introduced here, not deferred):**

The CLI is built incrementally alongside the API. This chunk introduces the
cobra command structure and the first commands:

- `bm serve` — Start the server
- `bm status` — Query project status (name, feature counts by status)

Environment variables: `BM_URL`, `BM_TOKEN` (required), `BM_PROJECT`
(optional, with `.env` fallback). The token uniquely determines the
project — the server resolves the project from the token. `BM_PROJECT`,
if set, is validated against the token's project on the server to catch
human configuration errors. All client output is JSON to stdout.

**Deliverables:**
- Server with chi router, auth middleware, config loading
- Project management endpoints
- Token generation (random secure tokens)
- Server tests with httptest
- `bm serve` and `bm status` cobra commands
- HTTP client wrapper and .env file support
- CLI tests with test server

## Chunk 4: Feature CRUD API

**Goal:** API for creating and managing features within a project.

Feature endpoints exist in two forms: project-scoped (with `/projects/{name}`
prefix, used by the dashboard) and token-scoped (without prefix, project
inferred from token, used by clients). Both map to the same handlers.

**Dashboard endpoints (project in URL):**

| Method | Endpoint                                | Auth      | Purpose                    |
|--------|-----------------------------------------|-----------|----------------------------|
| POST   | `/projects/{name}/features`             | Dashboard | Create feature             |
| GET    | `/projects/{name}/features`             | Dashboard | List features (filterable) |
| GET    | `/projects/{name}/features/{id}`        | Dashboard | Get feature with history   |
| PATCH  | `/projects/{name}/features/{id}`        | Dashboard | Update feature metadata    |
| DELETE | `/projects/{name}/features/{id}`        | Dashboard | Abandon/delete feature     |

**Client endpoints (project from token):**

| Method | Endpoint                    | Auth  | Purpose                    |
|--------|-----------------------------|-------|----------------------------|
| GET    | `/features`                 | Token | List features (filterable) |
| GET    | `/features/{id}`            | Token | Get feature with history   |

Create, update, and delete are Dashboard-only because features are created
and edited by the human user through the web UI, not by the client.

Create feature accepts: `name`, `description` (markdown).
Feature starts in `draft` status. The initial description is written to
`description_v0.md`.

PATCH accepts an optional `description` field. While the feature is in
`draft` status, this overwrites `description_v0.md`. This allows the user
to edit and refine the description before starting dialog. The description
field is rejected if the feature is not in draft status (the dialog loop
manages descriptions from that point on).

List supports filtering by status (e.g., `?status=draft,awaiting_client`).

Get returns the full feature including the latest description, full dialog
history, and current status.

**CLI commands added in this chunk:**

- `bm features` — List features with status
- `bm show <feature-id>` — Show feature details

**Deliverables:**
- Feature CRUD handlers and store methods
- Feature ID generation
- `bm features` and `bm show` CLI commands
- Handler tests, store tests, and CLI tests

## Chunk 5: Dialog Loop API

**Goal:** The core dialog loop between client and human, plus the poll
mechanism.

**Dashboard endpoints:**

| Method | Endpoint                                          | Auth      | Purpose                              |
|--------|---------------------------------------------------|-----------|--------------------------------------|
| POST   | `/projects/{name}/features/{id}/start-dialog`     | Dashboard | Transition draft -> awaiting_client  |
| POST   | `/projects/{name}/features/{id}/respond`          | Dashboard | Submit user response (optional final flag) |
| POST   | `/projects/{name}/features/{id}/reopen`           | Dashboard | Reopen with user message -> awaiting_client |

**Client endpoints (project from token):**

| Method | Endpoint                          | Auth  | Purpose                              |
|--------|-----------------------------------|-------|--------------------------------------|
| GET    | `/poll`                           | Token | Block until work is available        |
| GET    | `/features/{id}/pending`          | Token | Get pending work for a feature       |
| POST   | `/features/{id}/submit-dialog`    | Token | Submit updated feature + questions   |

The separate `finalize` endpoint is removed. Instead, `respond` accepts an
optional `final` flag. When set, the user's response is stored with
`is_final=true`, and the feature transitions to `awaiting_client` as usual.
The client processes the final response, submits an updated description
without questions, and the server transitions to `fully_specified`.

The `reopen` endpoint accepts a `message` field (the user's message that
motivates the reopen). This message becomes the user_response for the next
dialog round. The previous questions field is empty for a reopen.

**Poll semantics:**

`GET /poll` blocks (long-poll with configurable timeout, e.g., 30s) until
any feature in the project is in `awaiting_client` or `ready_to_generate`
status. Returns:

```json
{
  "action": "dialog_step",
  "feature_id": "ft-a1b2",
  "feature_name": "Add user profiles"
}
```

Or for generation:

```json
{
  "action": "generate",
  "feature_id": "ft-c3d4",
  "feature_name": "OAuth integration"
}
```

If nothing is pending, returns 204 No Content after timeout. The client
retries immediately.

**Pending work (`GET /features/{id}/pending`):**

For `dialog_step`, returns:

```json
{
  "iteration": 3,
  "feature_description": "...(markdown)...",
  "questions": "...(markdown, from previous round)...",
  "user_response": "...(markdown)..."
}
```

Field values vary by context:
- **First round** (after start-dialog): `questions` and `user_response`
  are both empty; client has only the initial description and codebase.
- **Subsequent rounds**: all three fields populated; `questions` is
  what the client asked last round, `user_response` is the user's answer.
- **Reopen**: `questions` is empty, `user_response` contains the user's
  reopen message.

For `generate`, returns:

```json
{
  "feature_description": "...(markdown, fully specified)..."
}
```

**Submit dialog (`POST /features/{id}/submit-dialog`):**

```json
{
  "updated_description": "...(markdown)...",
  "questions": "...(markdown)..."
}
```

This stores the new iteration. If the previous user response was marked
final and the submission contains no questions, the feature transitions to
`fully_specified`. Otherwise, the feature transitions to `awaiting_human`.

**Client connectivity tracking:**

The server records the timestamp of each poll request per project. The
dashboard uses this to show client connectivity status:
- "Connected" if last poll was within 2x the poll timeout
- "Last seen: X ago" otherwise

**CLI commands added in this chunk:**

- `bm poll` — Block until work available, print action JSON
- `bm fetch <feature-id>` — Get pending work for a feature
- `bm submit <feature-id>` — Submit dialog results from files

**Deliverables:**
- Dialog loop handlers and store methods
- Long-poll implementation
- Client connectivity tracking
- Dialog iteration storage (versioned files)
- `bm poll`, `bm fetch`, `bm submit` CLI commands
- Comprehensive tests for the dialog state machine
- E2E tests covering the full dialog loop (create project, create
  feature, start dialog, poll, fetch, submit, respond, final answer)

## Chunk 6: Web Dashboard

**Goal:** Interactive web UI for the human user, embedded in the binary.

**Technology:** Go HTML templates with vanilla JavaScript (matching
beads_server). Possible use of HTMX for interactivity if it simplifies the
implementation.

**Pages:**

1. **Login page** — Simple username/password form
2. **Dashboard** (`/`) — All projects with:
   - Client connectivity indicator per project
   - Feature counts by status
   - Collapsible project sections
3. **Project view** (`/project/{name}`) — Features for a project:
   - Grouped by status (awaiting_human, awaiting_client, draft,
     fully_specified, etc.)
   - Each feature shows name, status, last updated
4. **Feature detail** (`/feature/{project}/{id}`) — Full feature view:
   - Current description (markdown rendered)
   - Dialog history (expandable iterations)
   - Action buttons based on status:
     - Draft: "Edit Description", "Start Dialog"
     - Awaiting Human: questions displayed, response textarea,
       "Send Response", "Final Answer" buttons
     - Awaiting Client: status indicator (processing)
     - Fully Specified: "Reopen", "Generate Now",
       "Generate After" (dropdown of incomplete features)
       (generation buttons are rendered but disabled until Chunk 7
       adds the backing API endpoints)
     - Generating/Beads Created: status display, halt button
       (halt button is a placeholder until halt mechanism is defined)
5. **Create feature** (`/project/{name}/new`) — Form with name and markdown
   description editor
6. **Add project** — Form on dashboard to create new project, displays token

**Deliverables:**
- HTML templates embedded in binary
- Dashboard session management (cookie-based after login)
- All interactive forms and actions
- Markdown rendering (goldmark, matching beads_server)
- Dark/light theme toggle

## Chunk 7: Generation Pipeline Integration

**Goal:** Support the feature -> plan -> beads pipeline and dependency
tracking.

**Dashboard endpoints:**

| Method | Endpoint                                            | Auth      | Purpose                                 |
|--------|-----------------------------------------------------|-----------|------------------------------------------|
| POST   | `/projects/{name}/features/{id}/generate-now`       | Dashboard | Transition fully_specified -> ready_to_generate |
| POST   | `/projects/{name}/features/{id}/generate-after`     | Dashboard | Set dependency, fully_specified -> waiting |

`generate-after` accepts a body with `{"after_feature_id": "ft-xxxx"}`.
The dropdown in the UI shows incomplete features in the same project.

**Client endpoints (project from token):**

| Method | Endpoint                                | Auth  | Purpose                                   |
|--------|-----------------------------------------|-------|-------------------------------------------|
| POST   | `/features/{id}/start-generate`         | Token | Claim generation, ready_to_generate -> generating |
| POST   | `/features/{id}/register-beads`         | Token | Register bead IDs, generating -> beads_created |
| POST   | `/features/{id}/register-artifact`      | Token | Store plan/beads document for analysis    |
| POST   | `/features/{id}/complete`               | Token | Manual beads_created -> done              |

`complete` provides a manual way to mark a feature as done. This is the
initial mechanism before beads server integration (Chunk 8) can automate
the `beads_created → done` transition by detecting all beads are closed.

**Behavior:**

1. **"Generate now"** — Transitions fully_specified -> ready_to_generate.
   Next `bm poll` returns a `generate` action. The client calls
   `start-generate` to transition ready_to_generate -> generating,
   preventing the poll from returning the same feature again.
2. **"Generate after X"** — Transitions fully_specified -> waiting. The
   server monitors feature X and transitions to ready_to_generate when X
   reaches `done`.
3. **Bead registration** — `bm register-beads ft-a1b2 bd-x1 bd-x2 bd-x3`
   stores the bead IDs with the feature. Transitions generating ->
   beads_created.
4. **Artifact registration** — `bm register-artifact ft-a1b2 --type plan --file plan.md`
   stores plan/beads documents for later analysis.
5. **Dependency resolution** — When a feature reaches `done`, the server
   checks for any features in `waiting` state that depend on it and
   transitions them to `ready_to_generate`.
6. **Bead monitoring** — Mechanism TBD. Could be periodic polling of beads
   server, or the raymond workflow could notify bm when all beads are
   closed.

**CLI commands added in this chunk:**

- `bm register-beads <feature-id>` — Register bead IDs with a feature
- `bm register-artifact <feature-id>` — Register plan/beads document
- `bm complete <feature-id>` — Mark generation complete

**Deliverables:**
- Generation lifecycle state management
- Dependency tracking and resolution
- Bead and artifact registration endpoints and CLI commands
- Tests for dependency resolution

## Chunk 8: Beads Server Dashboard Integration

**Goal:** Show bead/implementation progress in the bm dashboard.

This depends on a monitoring REST endpoint being added to the beads server
(out of scope for this project). Once that API is defined:

- The bm server periodically queries the beads server for bead status
- The dashboard shows implementation progress per feature (e.g., "3/7 beads
  closed")
- This information also drives the `beads_created -> done` transition when
  all registered beads are closed

**Deliverables:**
- Beads server client in bm
- Dashboard integration
- Automatic feature completion detection

## CLI Command Summary

CLI commands are introduced incrementally alongside the API features they
use, rather than in a single chunk. All client commands output JSON to
stdout. With `.env` file fallback.

| Variable     | Purpose                              | Default                  |
|--------------|--------------------------------------|--------------------------|
| `BM_URL`     | Server URL                           | `http://localhost:8080`  |
| `BM_TOKEN`   | Bearer token (uniquely determines project) | (required)         |
| `BM_PROJECT` | Project name (validation only)       | (optional)               |

The token uniquely determines the project. `BM_PROJECT` is optional — if
set, the server validates it matches the token's project to catch
misconfiguration.

| Command                            | Chunk | Purpose                                 |
|------------------------------------|-------|-----------------------------------------|
| `bm serve`                         | 3     | Start the server                        |
| `bm status`                        | 3     | Show project status summary             |
| `bm features`                      | 4     | List features with status               |
| `bm show <feature-id>`             | 4     | Show feature details                    |
| `bm poll`                          | 5     | Block until work available, print action|
| `bm fetch <feature-id>`            | 5     | Get pending work for a feature          |
| `bm submit <feature-id>`           | 5     | Submit dialog results from files        |
| `bm start-generate <feature-id>`   | 7     | Claim feature for generation            |
| `bm register-beads <feature-id>`   | 7     | Register bead IDs with a feature        |
| `bm register-artifact <feature-id>`| 7     | Register plan/beads document            |
| `bm complete <feature-id>`         | 7     | Mark generation complete                |

`bm submit` reads file paths from flags:
```bash
bm submit ft-a1b2 --description updated_feature.md --questions questions.md
```

## Implementation Notes

**Testing strategy (TDD):**
- Write tests before or alongside implementation in each chunk
- Unit tests for model, store tests with temp dirs, httptest for handlers,
  CLI tests with test server, e2e tests for full workflows
- Run relevant tests during development, full suite before commits

**Dependencies (Go modules):**
- `github.com/go-chi/chi/v5` — HTTP router
- `github.com/spf13/cobra` — CLI framework
- `github.com/yuin/goldmark` — Markdown rendering
- Standard library for everything else

**Design principles:**
- Mirror beads_server conventions where applicable
- JSON output from CLI for machine consumption
- Atomic file writes for data integrity
- Simple auth model (tokens for clients, password for dashboard)
- Long-poll for responsive client-server communication
