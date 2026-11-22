# aimem

**Memory for AI agents.** One local, token-efficient store that lets a coding agent remember what it learned in one session and use it in the next.

[![Go](https://img.shields.io/badge/go-1.26%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)
[![Build](https://github.com/lendradev/aimem/actions/workflows/build.yml/badge.svg)](https://github.com/lendradev/aimem/actions/workflows/build.yml)

---

## The problem

Coding agents are amnesiac. Every new session starts from zero. The decisions you made last week, the environment quirk that broke the build, the approach that already failed twice — all of it has to be re-explained or re-read from scratch.

The usual fix is to dump the session transcript into a file and paste it into the next prompt. That burns tokens, stores noise, and repeats solved problems.

`aimem` takes the opposite approach: store **one durable fact per item**, retrieve **only the items that match the current question**, and **let the agent report which items actually helped**. Items that proved useful get stronger. Items that led nowhere get weaker and drop out of recall. Over time the store gets sharper instead of bigger.

## What it looks like

```console
$ aimem remember "pakai Go bukan Rust" --kind decision \
    --body "Keputusan user 2026-11-21. Binary static, tanpa cgo." \
    --tag keputusan --tag stack
saved FRA3HIFL (decision, scope project)

$ aimem recall "bahasa"
- [decision] pakai Go bukan Rust (id FRA3HIFL, score 1.25, scope project)
  Keputusan user 2026-11-21. Binary static, tanpa cgo.

$ aimem feedback FRA3HIFL useful --source session-2
FRA3HIFL -> status preferred, score 2.0000
```

## Why it stays small

| | |
|---|---|
| Storage | ~2.5 KB per item. Two SQLite files, nothing else. |
| Tokens | ~350 tokens per recall, capped by an explicit budget (default 600). |
| Retrieval | FTS5 with English stemming, plus a trigram fallback for prefix queries. |
| Decay | Outcome signals halve every 30 days. Unused items get pruned. |
| Network | None. Fully offline, no telemetry, no account. |

## Fundamental vs stale memory

Not every fact deserves the same lifetime.

| | Lifetime |
|---|---|
| **Fundamental** | Knowledge that holds across projects and years: the language the project is written in, a rule the team never breaks, a physical constant. Mark it with `--fundamental` (or `fundamental: true` over MCP). Prune never drops it, no matter how old it is. |
| **Ordinary** | Everything else. Kept while it is useful, dropped once it has not been recalled for five years. |

That split is what makes a larger body budget safe. A body can be up to 32 KB, and the storage that costs is still bounded, because ordinary memory goes stale and leaves, while the few fundamental items stay forever.

## Two scopes

| Scope | Path | Holds |
|---|---|---|
| **Project** (default) | `<project-root>/.aimem/data.db` | decisions, procedures, config, dead ends for this repo |
| **Global** | `~/.aimem/data.db` | your preferences and lessons that hold across every repo |

Project wins when both hold the same key. Global items stay reachable with `--scope global`.

An older install that used `.ai-memory` is migrated to `.aimem` automatically on the next run. The existing store is renamed, never rebuilt, so nothing is lost.

## Languages

The store holds text in any language, and the query pipeline handles the scripts that break naive tokenizers.

- **Latin, Cyrillic, Arabic, Hebrew, Greek** tokenize on word boundaries as expected.
- **Chinese, Japanese, Korean, and Thai** are written without word separators. The tokenizer keeps a whole run together as one term, so a query has to match the complete run instead of a fragment.
- **Combining marks** stay attached to their letter. Thai and Devanagari break badly when marks are stripped, so they are never treated as separators.
- **Stopwords** are known for English, Indonesian, German, French, Spanish, Portuguese, Italian, Dutch, Chinese, and Japanese. A term that is a stopword in one language is not automatically a stopword in another.

## How memory improves

Every recall result can be marked with an outcome. The agent does this automatically as it works:

| Outcome | Weight | Meaning |
|---|---|---|
| `useful` | +1 | this item helped |
| `dead_end` | -1 | this item did not help, do not repeat it |
| `corrected` | -1.5 | this item was wrong, here is the right answer |

Score is the time-decayed sum of those weights. Item status follows from the signal history: `tentative` → `preferred` (two distinct sources confirmed it), `contested` (both good and bad signals), `dead_end`, or `corrected`.

Beyond explicit feedback, four mechanisms make the store sharper on its own:

| Mechanism | What it does |
|---|---|
| **Reinforcement** | Being recalled counts as weak evidence. Each use adds a small, capped, decaying boost, so an item that keeps proving relevant climbs. |
| **Decay** | Outcome evidence and reinforcement both fade with time. An old item loses to a fresh one unless it was confirmed. |
| **Deduplication** | `remember` checks for an existing item that says the same thing and refuses to store a second copy. Pass `--allow-duplicate` to override. |
| **Promotion** | A project item confirmed by two sources and recalled twice is proposed for the global scope, because it generalises beyond one conversation. `aimem promote` moves it. |

`aimem reflect` writes a deterministic `LESSONS.md` summarising what the agent should read first next session.

## Install

Full step-by-step per OS, including checksum verification and troubleshooting: **[docs/install.md](docs/install.md)**.

MCP setup for every AI agent, with copy-paste prompts per OS: **[docs/agents.md](docs/agents.md)**.

### Option A: download a release binary (no toolchain needed)

Grab the archive for your platform from [Releases](https://github.com/lendradev/aimem/releases) and put the binary on your `PATH`:

```bash
# Linux / macOS
install -m 0755 aimem_linux_amd64 ~/.local/bin/aimem    # macOS: aimem_darwin_arm64
```

On Windows, copy `aimem_windows_amd64.exe` to any directory on your `PATH`.

### Option B: install with Go

```bash
go install github.com/lendradev/aimem/cmd/aimem@latest
```

The binary lands in `$(go env GOPATH)/bin`. Make sure that directory is on your `PATH`.

### Option C: build from source

```bash
git clone https://github.com/lendradev/aimem.git
cd aimem
./scripts/build.sh --install
```

This builds `dist/aimem` and installs it to `~/.local/bin` on Linux, `~/bin` on macOS, or `%LOCALAPPDATA%\Programs\aimem` on Windows. Set `AIMEM_INSTALL_DIR` to override the target.

If you have no Go toolchain but do have Docker, the scripts fall back to a container automatically and cache the toolchain between runs.

### Requirements

- Go 1.26 or newer to build from source (the shipped binaries need nothing).
- Docker is optional, only used as a fallback toolchain.

## Command names

The binary works under several names, so you can pick whichever reads best in your shell: `aimem`, `ai-memory`, `aimemory`, `ai-memory-cli`, `aimem-cli`.

Every name uses the same data directory, so switching names never splits your memory. The name you invoke it under is what shows in the output and in the MCP server name.


## Compile for every platform

```bash
# Linux
GOOS=linux   GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o dist/aimem_linux_amd64   ./cmd/aimem
GOOS=linux   GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o dist/aimem_linux_arm64   ./cmd/aimem

# macOS
GOOS=darwin  GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o dist/aimem_darwin_amd64  ./cmd/aimem
GOOS=darwin  GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o dist/aimem_darwin_arm64  ./cmd/aimem

# Windows
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o dist/aimem_windows_amd64.exe ./cmd/aimem
GOOS=windows GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o dist/aimem_windows_arm64.exe ./cmd/aimem
```

The build is pure Go with no cgo, so the output is a single static binary with no runtime dependencies. `modernc.org/sqlite` is used instead of `mattn/go-sqlite3` for exactly this reason.

Cross-compilation from any OS works. You do not need a Mac to build the Mac binary.

## Use it with your agent

### MCP server (recommended)

`aimem` speaks MCP over stdio and exposes three tools:

| Tool | Purpose |
|---|---|
| `memory_recall` | search the store, returns ranked items within a token budget |
| `memory_remember` | store one durable item |
| `memory_feedback` | report `useful`, `dead_end`, or `corrected` for an item |

Ready-to-paste config for Kilo Code, Claude Code, Claude Desktop, Cursor, Codex, Gemini CLI, VS Code Copilot, Zed, OpenCode, Cline, Roo Code, and Amazon Q, plus per-OS install prompts for agents: **[docs/agents.md](docs/agents.md)**.

For Kilo Code, add to `~/.config/kilo/kilo.jsonc`:

```jsonc
{
  "mcp": {
    "aimem": {
      "enabled": true,
      "type": "local",
      "command": ["/path/to/aimem", "mcp"],
      "timeout": 15000
    }
  }
}
```

For Claude Code, add to `.mcp.json` in your project:

```json
{
  "mcpServers": {
    "aimem": {
      "command": "/path/to/aimem",
      "args": ["mcp"]
    }
  }
}
```

### CLI

```bash
aimem remember "judul" --kind fact --body "detail" --tag tag
aimem remember "judul fundamental" --fundamental
aimem recall "query" --limit 5 --tokens 600
aimem show <id>
aimem forget <id>
aimem feedback <id> useful|dead_end|corrected
aimem reflect
aimem stats
aimem prune
aimem mcp
```

Every command supports `--json` for machine-readable output.

## CLI reference

See [docs/api.md](docs/api.md) for the full contract: every flag, the MCP method and tool schemas, the data model, the SQLite schema, and the storage and token budget measurements.

## Reference and inspiration

This project was inspired by [Graphify-Labs/graphify](https://github.com/Graphify-Labs/graphify), which maps code into a local knowledge graph and uses outcome signals (`useful` / `dead_end` / `corrected`) plus deterministic reflection to score what it surfaces.

`aimem` borrows that signal model and the reflection idea. It deliberately does **not** build a code graph:

- Graph traversal over a code index costs far more tokens than "what did I decide about this repo".
- `aimem` stores *experience* (facts, decisions, procedures, dead ends), not *code structure*.
- Retrieval is lexical by default and stays deterministic. No LLM calls, no embedding service.

Everything runs on-device. Nothing is uploaded, and there is no telemetry.

## Known limits

- **Lexical retrieval.** A natural-language query that shares no word or substring with the stored text is not found. Paraphrase matching needs semantic retrieval, which is on the roadmap but not implemented.
- **No stemming for non-English.** The porter stemmer is English. Indonesian and other languages match on whole words, which works but misses derivational variants.
- **No word segmentation for CJK or Thai.** A run of characters is one term, so a query must match the whole run. A dictionary-based segmenter would fix this for Chinese and Japanese.
- **Duplicate detection is lexical.** Two items that say the same thing in different words are not detected as duplicates. The check uses token overlap, not meaning.
- **Single writer.** One process at a time per database. SQLite locking is configured, but there is no multi-process coordination.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## Reporting bugs

**This project is written with AI assistance.** The code was generated by AI agents from a specification, reviewed and tested along the way, but it has not been through a human code audit at the scale a mature project would have. Expect rough edges.

If you find a bug, a wrong result, or anything that behaves unexpectedly:

1. Open an issue at <https://github.com/lendradev/aimem/issues>.
2. Include your OS and architecture, the `aimem version` output, and the exact commands that reproduce the problem.
3. If a memory item in your store caused it, mention the item title. **Do not paste secrets, tokens, or personal data** — memory items often contain environment specifics.

Security issues: please report them privately through GitHub's "Report a vulnerability" button rather than opening a public issue.

Feature ideas are welcome too. Open an issue first so we can agree on the scope before you write code.

## License

MIT. See [LICENSE](LICENSE).
