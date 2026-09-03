# Architecture

## Shape

```
┌──────────────────────────────────────────────────────────┐
│                   Wails v2 desktop shell                 │
│  ┌────────────────────────────────────────────────────┐  │
│  │            React interface (embedded)              │  │
│  │   task grid · drop zone · options · progress       │  │
│  └───────────────────────┬────────────────────────────┘  │
│                          │  typed bindings               │
│  ┌───────────────────────▼────────────────────────────┐  │
│  │                internal/app                        │  │
│  │      the only package that imports Wails           │  │
│  └───────────────────────┬────────────────────────────┘  │
└──────────────────────────┼───────────────────────────────┘
                           │  the same interfaces the CLI drives
        ┌──────────────────▼──────────────────┐
        │  task registry → job queue          │
        │            ↓                        │
        │  pipeline: validate → prepare →     │
        │  execute → verify → write           │
        └──────────────────┬──────────────────┘
                           │
        ┌──────────────────▼──────────────────┐
        │           engine adapters           │
        └──┬────────┬─────────┬────────┬──────┘
           │        │         │        │
        pdfcpu   image     tesseract  ffmpeg
        (Go)     (Go)      libreoffice
        always   always    detected   downloaded
```

Two entry points sit on the same core: `main.go` is the desktop app and
`cmd/lathe-cli` is the headless one. Neither holds logic. A task added to the
registry appears in both without further work, which is the main reason the CLI
was cheap enough to be worth having.

## The rules that hold the shape together

Both are enforced by `scripts/boundary` in CI, not by convention.

**Wails is confined to `internal/app`.** No package under `internal/` may
import it, and `internal/app` is a thin binding layer over the interfaces the
CLI already uses. Wails v3 will eventually be the right target; if this
boundary holds, moving is rewriting one package, and if it leaks it is a
rewrite of the application. See D1 in [DECISIONS.md](DECISIONS.md).

**Network code is confined to `internal/deps` and `internal/update`.** Nothing
else may import `net`, `net/http` or anything under them. This is what makes
the privacy claim a fact about the code rather than a promise in a document,
and [`test/isolation`](../test/isolation) checks it again from the outside.

## The packages

| Package | Responsibility |
|---|---|
| `internal/exec` | **The only place a process is spawned.** Arguments are always a slice, output is capped, and cancellation kills the whole process tree. |
| `internal/fsatomic` | Every write goes through a temp file beside its destination and an atomic rename. Nothing else touches the destination directly. |
| `internal/detect` | Identifies files by magic bytes rather than extension, and flags encrypted PDFs before a job starts. |
| `internal/task` | The registry: ~30 user-facing operations, with the interface rules validated at construction. |
| `internal/pipeline` | One job, start to finish, in fixed stages. |
| `internal/job` | The queue: bounded concurrency, cancellation, progress events. |
| `internal/engine` | The adapter interface plus typed option access and page-range parsing. |
| `internal/engine/*` | One adapter per tool. Each translates a request into that tool's conventions, parses its progress, and translates its errors. |
| `internal/deps` | The tier system: detect, download, verify, install, remove. |
| `internal/usererr` | Engine output to sentences a person can act on. |
| `internal/settings` | The only state kept between launches. |
| `internal/shellint` | The opt-in context-menu entry. |
| `internal/update` | The opt-in version check. |
| `internal/app` | Wails bindings. No logic. |

## Why the pipeline stages are fixed

```
validate → prepare → execute → verify → write
```

Every stage exists because skipping it produces a specific bad outcome.

**Validate** runs everything checkable before any work: the file exists, is the
type it claims, is not encrypted without a password, and the output folder is
writable. Without it, a predictable failure arrives five minutes into a video
conversion instead of immediately.

**Prepare** confirms the components a task needs are installed, so the download
prompt appears before the work rather than during it.

**Execute** runs the engine in a private workspace. Nothing an engine writes is
visible to the user until it has been checked.

**Verify** is the one people skip. A truncated FFmpeg output has a plausible
file size and is broken. Checking that the result exists, is non-empty and
parses as its claimed type is the difference between an honest error message
and a corrupt file in someone's Documents folder.

**Write** moves results into place atomically, and never over an existing file:
`report.pdf` becomes `report (1).pdf`.

## Concurrency

Default is `min(2, NumCPU/2)`. Media conversion saturates every core it is
given; running four at once makes the machine unusable and the user blames the
app. It is configurable, and the default is deliberately conservative.

## Adding to it

A new task is one registry entry plus a case in one engine. A new engine is one
type satisfying `engine.Engine` plus a line in `internal/engines`. Neither
requires touching the interface, the CLI, the queue or the pipeline — see
[ADDING_A_TASK.md](ADDING_A_TASK.md).
