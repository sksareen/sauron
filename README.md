# Sauron

**The all-seeing eye for Claude Code.**

A passive context daemon for macOS that gives Claude Code total awareness of what you're doing — and memory of what every agent has done before.

It watches your clipboard, tracks which apps you're using, takes screenshots, detects git commits, classifies your work sessions, and stores everything in a local SQLite database with vector embeddings for semantic search. It also maintains an **agent experience graph** — a collective memory where agents log what worked, what failed, and what tools helped, so future agents don't repeat the same mistakes.

One binary. One database. 10 MCP tools. Zero cloud dependencies.

---

## Quick Start

```bash
# Install
go install github.com/sksareen/sauron/cmd/sauron@latest

# Set up LaunchAgent + MCP server + Claude Code hints
sauron install

# Start the daemon
sauron start

# Verify
sauron status
```

Restart Claude Code. It now has access to all 10 Sauron tools.

### Alternative install methods

**From source:**
```bash
git clone https://github.com/sksareen/sauron.git
cd sauron
make install-local
```

**One-liner (downloads pre-built binary):**
```bash
curl -fsSL https://raw.githubusercontent.com/sksareen/sauron/main/install.sh | bash
```

---

## What It Does

### 1. Watches Everything

The daemon runs in the background and captures:

| Signal | How | Frequency |
|--------|-----|-----------|
| **Clipboard** | `pbpaste` with source app + window title | Every 1s |
| **Active app** | `osascript` (frontmost app + window title) | Every 5s |
| **Work sessions** | Classified from app switch patterns | Every 30s |
| **Screenshots** | `screencapture -x` on app/clipboard change | On event |
| **Git commits** | HEAD hash polling across `~/coding/*` repos | Every 10s |
| **Local servers** | `lsof` for listening TCP ports | On context query |

Session types: `deep_focus`, `exploration`, `communication`, `creative`, `admin`, `idle`. Focus score (0-100%) computed from app-switch frequency.

### 2. Remembers What Agents Learn

The **experience graph** is a structured log of every task an AI agent completes:

```
Task intent    →  "Fix authentication bug in login flow"
Approach       →  "Traced to expired JWT validation, updated token refresh logic"
Outcome        →  success
Tools used     →  [go, jwt-go, postgres]
Failure points →  ["initially tried session-based auth, didn't scale"]
Resolution     →  "Switched to stateless JWT with 24h expiry"
Tags           →  [auth, bugfix, security]
```

Before starting a new task, agents call `sauron_check_experience` to find similar past work. After completing a task, they call `sauron_log_experience` to record what happened. Over time, agents get smarter — they know which approaches work in your codebase and which don't.

All records are **automatically privacy-scrubbed** before storage: API keys, bearer tokens, emails, home directory paths, AWS keys, private key blocks, and database connection strings are redacted.

### 3. Screenshot Capture + Annotation

Sauron includes **SauronCapture**, a native macOS screenshot tool with annotation support:

- **Global hotkey:** `Ctrl+Shift+S` to capture anytime
- **Annotation:** Freehand drawing and text overlays on screenshots
- **Gallery:** Browse all captured screenshots from the menu bar (◎)
- **Auto-registration:** Screenshots are saved and registered in the Sauron database, making them available to Claude Code via `sauron_screenshots`

```bash
# First-time setup: build and install with auto-start
sauron capture --install

# Manual controls
sauron capture              # one-shot capture
sauron capture --start      # start background app
sauron capture --stop       # stop background app
```

SauronCapture runs as a menu bar app. It requires Accessibility permission (macOS will prompt on first launch).

---

## MCP Tools

Once installed, Claude Code gets 10 tools it can call on demand:

### Watcher

| Tool | Returns |
|------|---------|
| `sauron_context` | Current session type, focus score, dominant app, recent clipboard, local servers |
| `sauron_clipboard` | Last N clipboard items with source app + window title |
| `sauron_activity` | App usage breakdown with durations for last N hours |
| `sauron_search` | Full-text search (FTS5) across all clipboard history |
| `sauron_recall` | Semantic search over intent traces — "what was I doing when X happened?" |
| `sauron_timeline` | Fused chronological view of all event types in a time window |
| `sauron_screenshots` | Recent screenshot file paths (Claude can read images directly) |

### Experience Graph

| Tool | Does |
|------|------|
| `sauron_check_experience` | Semantic search over past agent experiences — find what worked, what failed, what to avoid |
| `sauron_log_experience` | Record a completed task with approach, outcome, tools, failures, resolution, tags |
| `sauron_experience_stats` | Total records, success/failure/partial breakdown |

---

## CLI Reference

```bash
# ── Daemon ─────────────────────────────────────
sauron start                      # start background daemon
sauron stop                       # stop daemon
sauron status                     # running? + capture stats

# ── Context & Activity ─────────────────────────
sauron context                    # current session summary
sauron context --brief            # one-line: "deep_focus | 90% | VS Code"
sauron clipboard [n]              # last n clipboard items (default: 10)
sauron activity [hours]           # app breakdown (default: 2h)
sauron search <query>             # full-text search across clipboard
sauron recall <query>             # semantic search over work history
sauron timeline [--hours 2]       # fused timeline of all events
sauron traces [--limit 10]        # recent intent traces (git commits, etc.)

# ── Experience Graph ───────────────────────────
sauron experience search <query>  # semantic search over past experiences
sauron experience stats           # graph statistics
sauron experience recent [n]      # most recent experiences

# ── Screenshot Capture ─────────────────────────
sauron capture                    # one-shot screenshot + annotate
sauron capture --install          # build app + auto-start on login
sauron capture --start            # start capture app
sauron capture --stop             # stop capture app

# ── Setup ──────────────────────────────────────
sauron install                    # register LaunchAgent + MCP + CLAUDE.md
sauron uninstall                  # remove everything
sauron migrate-agentgraph         # import from standalone AgentGraph
sauron version                    # print version
```

All query commands support `--json` and `--md` output formats.

---

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                    Your Mac                         │
│  clipboard · apps · windows · git repos · screens   │
└──────────────────────┬──────────────────────────────┘
                       │
          ┌────────────▼────────────┐
          │    Sauron Daemon        │
          │  clipboard poll (1s)    │
          │  activity poll  (5s)    │
          │  session classify (30s) │
          │  intent detect  (10s)   │
          │  screenshot capture     │
          └────────────┬────────────┘
                       │ writes
          ┌────────────▼────────────┐
          │  ~/.sauron/sauron.db    │
          │  ├─ clipboard_history   │
          │  │  └─ FTS5 index       │
          │  ├─ activity_log        │
          │  ├─ context_sessions    │
          │  ├─ screenshots         │
          │  ├─ intent_traces       │
          │  │  └─ embeddings       │
          │  └─ experiences         │
          │     └─ embeddings       │
          └──────┬──────────┬───────┘
                 │          │
        ┌────────▼──┐  ┌───▼────────┐
        │  CLI      │  │ MCP Server │
        │ (read)    │  │ (read/write)│
        └───────────┘  └───┬────────┘
                           │ stdio
                    ┌──────▼──────┐
                    │ Claude Code │
                    │  10 tools   │
                    └─────────────┘
```

- **Daemon** owns all writes to the database
- **CLI** is read-only — safe to run anytime
- **MCP server** is read-write (experience logging needs writes)
- **SQLite WAL mode** for concurrent read/write access
- **Vector embeddings** stored as float32 BLOBs — no separate vector DB
- **Semantic search** uses cosine similarity over `text-embedding-3-small` embeddings via OpenRouter

---

## Configuration

### Required

Nothing. Sauron works with zero configuration out of the box.

### Optional

**Semantic search** — set an OpenRouter API key to enable vector-based search for `recall` and `check_experience`. Without it, both fall back to text matching (LIKE queries).

```bash
export OPENROUTER_API_KEY=sk-or-...
```

Add to your `~/.zshrc` or `~/.bashrc` to persist.

---

## Data Storage

Everything stays local. No cloud, no telemetry, no network calls except optional embedding generation.

```
~/.sauron/
├── sauron.db                # SQLite database (all tables)
├── sauron.pid               # daemon process ID
├── daemon.log               # daemon output log
├── screenshots/             # captured screenshot images
└── SauronCapture.app/       # screenshot annotation app (if installed)
```

### Database tables

| Table | Records | Purpose |
|-------|---------|---------|
| `clipboard_history` | Every clipboard change | Content + source app + window title |
| `clipboard_fts` | FTS5 virtual table | Full-text search over clipboard |
| `activity_log` | Every app switch | App name, duration, window title |
| `context_sessions` | Every 30s | Session type, focus score, dominant app |
| `screenshots` | On events | File paths + metadata |
| `intent_traces` | Git commits, etc. | Outcome + activity context + embeddings |
| `experiences` | Agent task logs | Intent, approach, outcome, tools, failures + embeddings |

---

## Migrating from Other Tools

### From AgentGraph

If you used the standalone AgentGraph MCP server:

```bash
sauron migrate-agentgraph
```

Copies all records from `~/.agentgraph/experiences.db`, converting Float64 embeddings to Float32. Then remove the `agentgraph` entry from `~/.claude.json`.

### From Sakshi

Sauron's watcher is the same codebase as Sakshi with an identical schema:

```bash
sakshi stop && sakshi uninstall
cp ~/.sakshi/sakshi.db ~/.sauron/sauron.db
sauron install && sauron start
```

---

## How `sauron install` Works

The install command does three things:

1. **LaunchAgent** — writes `~/Library/LaunchAgents/com.sauron.daemon.plist` so the daemon starts on login and auto-restarts if killed
2. **MCP registration** — adds a `sauron` entry to `~/.claude.json` pointing to the binary with `mcp` argument (stdio transport)
3. **CLAUDE.md hint** — appends a block to `~/.claude/CLAUDE.md` telling Claude Code what tools are available and when to use them

`sauron uninstall` reverses all three steps.

---

## Requirements

- **macOS** (uses `osascript`, `pbpaste`, `screencapture`, `lsof`)
- **Go 1.23+** to build from source
- **Xcode Command Line Tools** for SauronCapture (Swift compilation)
- Optional: `OPENROUTER_API_KEY` for semantic search

---

## License

MIT
