# Sauron

**The all-seeing eye for Claude Code.**

A background daemon polls your clipboard, active app, window title, and screenshots every few seconds and writes everything to a local SQLite database. It classifies your work into session types (deep focus, exploration, communication) and computes a focus score from app-switch frequency. When it detects git commits, it generates vector embeddings and stores them for semantic search over your work history. An MCP server exposes this as tools that Claude Code can call on demand — context, clipboard, activity, search, recall, timeline, screenshots. One `install` command registers the LaunchAgent, adds the MCP server to `~/.claude.json`, and hints `CLAUDE.md` so Claude uses it proactively. Everything stays local — one binary, one SQLite file, no cloud.

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

### Homebrew (coming soon)

```bash
brew install sksareen/tap/sauron
```

## What it captures

| Signal | Method | Interval |
|--------|--------|----------|
| Clipboard | `pbpaste` | 1s |
| Active app + window title | `osascript` | 5s |
| Session classification | Derived from app switches | 30s |
| Screenshots | `screencapture` | On app/clipboard change |
| Git commits | HEAD hash check | 10s |

## Tools exposed to Claude Code

Once installed, Claude Code can call these MCP tools:

| Tool | What it returns |
|------|----------------|
| `sauron_context` | Current session type, focus score, dominant app, recent clipboard |
| `sauron_clipboard` | Last N clipboard items with source app + window title |
| `sauron_activity` | App usage breakdown for last N hours |
| `sauron_search` | Full-text search across all clipboard history |
| `sauron_recall` | Semantic search over intent traces (what were you doing when X happened?) |
| `sauron_timeline` | Fused chronological view of all events |
| `sauron_screenshots` | Recent screenshot paths (Claude can read images) |

## CLI

```bash
sauron start                    # start the daemon
sauron stop                     # stop the daemon
sauron status                   # check if running + stats

sauron context                  # what you're working on right now
sauron context --brief          # one-line summary
sauron clipboard [n]            # last n clipboard items
sauron activity [hours]         # app usage breakdown
sauron search <query>           # full-text search
sauron recall <query>           # semantic search over work history
sauron timeline                 # fused timeline of all events
sauron traces                   # recent intent traces

sauron install                  # set up LaunchAgent + MCP + CLAUDE.md
sauron uninstall                # reverse everything
```

All commands support `--json` and `--md` output formats.

## How it works

```
You (working on your Mac)
       ↓
Daemon (clipboard 1s, activity 5s, intent 10s)
       ↓
SQLite (~/.sauron/sauron.db)
       ↓
MCP Server (stdio, 7 tools)
       ↓
Claude Code (calls tools on demand)
```

The daemon owns all writes. The CLI and MCP server are read-only. SQLite runs in WAL mode for concurrent access. Vector embeddings are stored as BLOBs — no separate vector database needed. Semantic search uses cosine similarity over embeddings from `text-embedding-3-small`.

## Configuration

Set `OPENROUTER_API_KEY` in your environment for semantic search (recall). Without it, recall falls back to text matching. Everything else works with zero configuration.

```bash
export OPENROUTER_API_KEY=sk-or-...
```

## Data

Everything is stored locally at `~/.sauron/`:

```
~/.sauron/
├── sauron.db       # SQLite database
├── sauron.pid      # daemon PID
└── daemon.log      # daemon logs
```

## Requirements

- macOS (uses `osascript`, `pbpaste`, `screencapture`)
- Go 1.23+ (to build from source)
- Optional: `OPENROUTER_API_KEY` for semantic search

## License

MIT
