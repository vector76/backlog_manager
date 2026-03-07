# Data Model

This document describes the core data types used by the backlog manager (`bm`).

## Project

Represents a client project. Projects are the top-level grouping for features.

| Field        | Type      | JSON key     | Description                        |
|--------------|-----------|--------------|------------------------------------|
| Name         | string    | `name`       | Unique project name                |
| Token        | string    | `token`      | Authentication token for the project |
| CreatedAt    | time.Time | `created_at` | Creation timestamp                 |

## Feature

Represents a single feature in a project. **Description content is not stored in this struct** — all description text, AI questions, and user responses live exclusively in versioned markdown files on disk. The `current_iteration` field tracks the latest dialog round number.

| Field             | Type          | JSON key            | Description                                          |
|-------------------|---------------|---------------------|------------------------------------------------------|
| ID                | string        | `id`                | Unique feature ID (e.g. `ft-abcd`)                  |
| Project           | string        | `project`           | Name of the owning project                           |
| Name              | string        | `name`              | Human-readable feature name                          |
| Status            | FeatureStatus | `status`            | Current lifecycle status (string enum in JSON)       |
| CurrentIteration  | int           | `current_iteration` | Latest dialog round number                           |
| GenerateAfter     | string        | `generate_after`    | ID of a feature this one depends on (optional)       |
| BeadIDs           | []string      | `bead_ids`          | IDs of beads created for this feature (optional)     |
| CreatedAt         | time.Time     | `created_at`        | Creation timestamp                                   |
| UpdatedAt         | time.Time     | `updated_at`        | Last modification timestamp                          |

## DialogIteration

Stores per-round metadata for a feature's dialog. Actual content (description, AI questions, user responses) is read from versioned files on disk and is not stored here.

| Field     | Type      | JSON key     | Description                              |
|-----------|-----------|--------------|------------------------------------------|
| Round     | int       | `round`      | Dialog round number (1-based)            |
| IsFinal   | bool      | `is_final`   | Whether this is the final dialog round   |
| CreatedAt | time.Time | `created_at` | When this iteration was created          |

## FeatureStatus Enum

`FeatureStatus` represents the lifecycle state of a feature. Stored as a string in JSON.

| Constant              | JSON value          | Description                                           |
|-----------------------|---------------------|-------------------------------------------------------|
| `StatusDraft`         | `draft`             | Initial state; dialog not yet started                 |
| `StatusAwaitingClient`| `awaiting_client`   | Waiting for the AI client to process the dialog       |
| `StatusAwaitingHuman` | `awaiting_human`    | Waiting for a human response to AI questions          |
| `StatusFullySpecified`| `fully_specified`   | Dialog complete; feature is fully described           |
| `StatusWaiting`       | `waiting`           | Waiting on a dependency feature to complete           |
| `StatusReadyToGenerate`| `ready_to_generate`| Ready for code generation                             |
| `StatusGenerating`    | `generating`        | Code generation is in progress                        |
| `StatusBeadsCreated`  | `beads_created`     | Generation done; beads (tasks) have been registered   |
| `StatusDone`          | `done`              | All beads closed or manually marked complete          |
| `StatusHalted`        | `halted`            | Paused; can be resumed                                |
| `StatusAbandoned`     | `abandoned`         | Permanently cancelled                                 |

### Feature Lifecycle

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

## FeatureAction Enum

`FeatureAction` is returned by the poll endpoint to tell the client what action to take next. Stored as a string in JSON.

| Constant          | JSON value    | Description                         |
|-------------------|---------------|-------------------------------------|
| `ActionDialogStep`| `dialog_step` | Client should perform a dialog step |
| `ActionGenerate`  | `generate`    | Client should start code generation |

## ID Generation

Feature IDs use the format `ft-XXXX` where `X` is a random alphanumeric character (lowercase letters and digits). The generation scheme is collision-resistant:

1. Start with 4 random characters (e.g. `ft-ab3z`)
2. If a collision is detected, increase to 5 characters
3. Continue escalating up to 8 characters
4. At 8 characters, keep retrying without further escalation

This mirrors the `bd-` ID scheme used by beads_server.
