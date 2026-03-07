# Additional Notes

## Bead Creation and Monitoring

The raymond workflow creates the beads (as it does currently) and also registers
them with bm. The mechanism by which bm monitors those beads (to detect
completion and unblock dependent features) is not yet decided.

There is a distinction between beads created/registered and beads all closed.
A feature can be in a "waiting" state where it is ready to generate but blocked
on another feature's beads being fully closed.

## Plan and Beads Documents

The plan-to-beads step is non-interactive. It operates as two steps:

1. The plan document is transformed into a beads document
2. The beads document is iteratively refined, then used to generate actual beads

These artifacts (plan document and beads document) could be registered with bm
for later analysis and improvement of the workflows, but they do not directly
feed into anything interactive. The files on the client side are deleted after
serving their purpose.

## Feature Dependencies ("Generate After")

The "generate after ___" field refers to another feature name or ID as
understood by the backlog manager. The UI should present this as a drop-down
showing not-yet-completed features within the same project, rather than a
free-text entry box.

## Feature Dialog Concurrency

Multiple features may be in dialog simultaneously for the same project. However,
the client processes one dialog step per feature at a time. Each feature cycles
through the statuses: `awaiting_human` -> `awaiting_client` -> (client
processing) -> `awaiting_human`. The client picks up any feature in
`awaiting_client` status, processes it (generating updated feature description
and assumptions/questions), submits the result (transitioning to
`awaiting_human`), and can then pick up another feature.

## Client Commands

The client does not know in advance what the next action will be (dialog step
vs plan-to-beads kickoff). The `bm poll` command acts like a `select()` that
blocks on anything needing processing and returns information about what needs
handling. Other commands include fetching feature data, submitting results, and
registering beads.

## Feature Generation Ordering

There is no explicit priority or ordering. The client structure generally allows
only one feature-plan-beads process at a time. While one feature's beads are
being consumed (implemented separately), another feature-plan-beads process can
operate if it is not marked as dependent. There is some risk that plans may be
slightly out of date as implementation changes the codebase, but this is up to
the user to manage via dependencies. If features are orthogonal, the user may
choose to run planning before the prior feature's implementation is finished.

## Client Lifecycle

Raymond runs in an infinite loop, invoking `bm` to check for new features and
process plans and beads. Most of the time raymond is waiting on `bm`, which is
in turn stalled waiting for the human to create a feature or respond to a query.
The client is long-lived, driven by raymond's loop.

## Project and Token Scoping

One token = one client = one project. Only one client manages user dialog,
planning, and bead creation per project. Additional workers may implement beads
for the same project, but they use `bs` (beads server), not `bm`.

## Client-Server Communication

The client polls the server, or alternatively uses an open websocket (or slow
HTTP response) for faster response and better interactivity. Latency affects
user experience, so a push-based mechanism is preferred.

The server sets status flags that `bm poll` detects; it does not push actions
to clients directly. The UI should show a connectivity indicator for each
project's client -- either online/offline or time since last poll -- so the user
can tell if a client has died and needs attention.

## Feature Deletion, Cancellation, and Halting

- A user can abandon or delete a feature at any time before the
  feature-plan-bead process has started.
- Once planning/implementation has begun, it should be possible to halt. The
  raymond process performing feature-to-plan and plan-to-beads checks for a halt
  indicator after each step.
- If beads have already been created and are being worked, any unclaimed beads
  can be transitioned to "not ready" to prevent being picked up. The user can
  then decide to resume or abort.

## Dialog History

All iterations are stored: every version of the feature definition,
assumptions/questions, and human feedback. This full history enables refining
prompts after the fact.

## Halt Mechanism

The specific halt mechanism will be decided later. It won't be exclusive to
`bm poll` and may depend on where in the lifecycle the feature is.

## Binary Structure

Single binary (`bm`) that acts as server or client based on subcommand, similar
to the beads server.

## Configuration

- **Server**: JSON config file (matching beads_server convention). Not
  environment variables. Contains username/password for dashboard access and
  other server settings.
- **Clients**: Environment variables or a `.env` file for server URL and token.
  Project name is optional (for validation only).

## Storage

Plain files rather than a database, at least for early development. Structure:
- A folder for each project
- A JSON file for metadata
- Subfolders with files using a naming convention for iterations of feature
  descriptions

This mirrors the beads_server approach of using plain files, allowing convenient
inspection of the underlying structures. Migration to SQLite may happen later.

## Project/Client Registration

The server has a UI button to add a project/client, which produces a token. The
client is then configured with the server URL and token via environment variables
or a `.env` file. The project name may optionally be specified for validation
purposes — if it doesn't match the project associated with the token on the
server, an error is raised, catching human configuration errors. The `bm` client
connects using the token, which uniquely determines the project and scope.
This mirrors beads_server's structure, except that bm automates the token
creation and project addition through the UI (whereas beads_server handles
this manually).

## Authentication

- **Clients**: Authenticated via tokens (issued when a project is added)
- **Web UI/Dashboard**: Protected by a simple username/password configured in a
  config file. This prevents agents from accessing the frontend.

## Dashboard and Bead Progress

The bm dashboard can query the beads server to show implementation progress
alongside feature status. The beads server will need a new monitoring REST
endpoint for the bm dashboard to consume. The beads server is written in Go
and resides at ../beads_server. Adding that endpoint is outside the scope of
this project; once the API is defined it will be added to requirements here.

## Feature Description Format

Feature descriptions are markdown.
