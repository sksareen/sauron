# Sauron

**The all-seeing eye for Claude Code.**

A background daemon that watches everything you do on your Mac — clipboard, active apps, screenshots, git commits — and stores it in a local SQLite database with vector embeddings. It also includes a built-in **agent experience graph**: every AI coding session logs what worked, what failed, and what tools helped, so future agents learn from past mistakes. One binary, one install command, 10 MCP tools for Claude Code. Everything stays local — no cloud, no telemetry.

## Install

### One-liner (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/sksareen/sauron/main/install.sh | bash
```

### From source

```bash
git clone https://github.com/sksareen/sauron.git
cd sauron
make install-local
```

### Go install

```bash
go install github.com/sksareen/sauron/cmd/sauron@latest
sauron install
sauron start
```

## What it captures

| Signal | Method | Interval |
|--------|--------|----------|
| Clipboard | `pbpaste` | 1s |
| Active app + window title | `osascript` | 5s |
| Session classification | Derived from app switches | 30s |
| Screenshots | `screencapture` | On app/clipboard change |
| Git commits | HEAD hash check | 10s |

## MCP Tools for Claude Code

Once installed, Claude Code gets 10 tools:

### Watcher tools
| Tool | What it returns |
|------|----------------|
| `sauron_context` | Current session type, focus score, dominant app, recent clipboard |
| `sauron_clipboard` | Last N clipboard items with source app + window title |
| `sauron_activity` | App usage breakdown for last N hours |
| `sauron_search` | Full-text search across all clipboard history |
| `sauron_recall` | Semantic search over intent traces |
| `sauron_timeline` | Fused chronological view of all events |
| `sauron_screenshots` | Recent screenshot paths (Claude can read images) |

### Experience graph tools
| Tool | What it does |
|------|-------------|
| `sauron_check_experience` | Search past agent experiences before starting a task — find approaches that worked, failures to avoid |
| `sauron_log_experience` | Record a completed task: what worked, what failed, tools used, resolution |
| `sauron_experience_stats` | Graph statistics: total records, success/failure/partial breakdown |

## CLI

```bash
# Daemon
sauron start                    # start the daemon
sauron stop                     # stop the daemon
sauron status                   # check if running + stats

# Watcher
sauron context                  # what you're working on right now
sauron context --brief          # one-line summary
sauron clipboard [n]            # last n clipboard items
sauron activity [hours]         # app usage breakdown
sauron search <query>           # full-text search
sauron recall <query>           # semantic search over work history
sauron timeline                 # fused timeline of all events
sauron traces                   # recent intent traces

# Experience graph
sauron experience search <q>    # semantic search over past experiences
sauron experience stats         # graph statistics
sauron experience recent [n]    # show recent experiences

# Setup
sauron install                  # set up LaunchAgent + MCP + CLAUDE.md
sauron uninstall                # reverse everything
sauron migrate-agentgraph       # import from ~/.agentgraph/experiences.db
```

All commands support `--json` and `--md` output formats.

## How it works

```
You (working on your Mac)
       ↓
Daemon (clipboard 1s, activity 5s, intent 10s)
       ↓
SQLite (~/.sauron/sauron.db)
  ├── clipboard_history + FTS5
  ├── activity_log
  ├── context_sessions
  ├── screenshots
  ├── intent_traces + embeddings
  └── experiences + embeddings     ← agent experience graph
       ↓
MCP Server (stdio, 10 tools)
       ↓
Claude Code (calls tools on demand)
```

The daemon owns all writes. The CLI is read-only. The MCP server is read-write (experience logging needs writes). SQLite runs in WAL mode for concurrent access. Vector embeddings are stored as BLOBs — no separate vector database needed.

## Agent Experience Graph

The experience graph is a collective memory for AI agents. Every time an agent completes a task, it can log:

- **Task intent** — what it was trying to do
- **Approach** — how it did it, key decisions
- **Outcome** — success, failure, or partial
- **Tools used** — languages, frameworks, services
- **Failure points** — what went wrong
- **Resolution** — how failures were fixed
- **Tags** — for categorization and retrieval

Before starting a new task, agents call `sauron_check_experience` to find relevant past experiences. This prevents repeating mistakes and surfaces proven approaches.

All records are **privacy-scrubbed** before storage: API keys, tokens, emails, home paths, and connection strings are automatically redacted.

### Migrating from AgentGraph

If you were using the standalone AgentGraph MCP server:

```bash
sauron migrate-agentgraph
```

This copies all records from `~/.agentgraph/experiences.db` into Sauron's unified database, converting Float64 embeddings to Float32. You can then remove the agentgraph MCP server from `~/.claude.json`.

## Configuration

Set `OPENROUTER_API_KEY` in your environment for semantic search (recall + experience graph). Without it, both fall back to text matching. Everything else works with zero configuration.

```bash
export OPENROUTER_API_KEY=sk-or-...
```

## Data

Everything is stored locally at `~/.sauron/`:

```
~/.sauron/
├── sauron.db       # SQLite database (watcher + experience graph)
├── sauron.pid      # daemon PID
└── daemon.log      # daemon logs
```

## Requirements

- macOS (uses `osascript`, `pbpaste`, `screencapture`)
- Go 1.23+ (to build from source)
- Optional: `OPENROUTER_API_KEY` for semantic search

## License

MIT
