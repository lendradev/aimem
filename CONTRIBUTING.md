# Contributing to aimem

Thanks for looking at this. `aimem` is small and the surface area is narrow, so a focused patch is easier to review than a large one.

## Before you start

Open an issue first for anything bigger than a bug fix. Agreeing on scope up front saves both of us a round of rework.

## Development setup

You need Go 1.26 or newer. If you do not have it, the helper scripts fall back to a Docker container automatically, so Docker is a valid alternative to installing Go.

```bash
git clone https://github.com/lendradev/aimem.git
cd aimem
```

## The commands you will use

```bash
./scripts/go.sh build ./...     # compile
./scripts/go.sh test ./...      # run the test suite
./scripts/go.sh vet ./...       # static analysis
./scripts/go.sh run ./cmd/spike # the FTS5 capability spike
./scripts/build.sh --install    # build into dist/ and install
```

`scripts/go.sh` runs `go` natively when it is on `PATH`, and in a container otherwise. It accepts any `go` subcommand, so `./scripts/go.sh fmt ./...` works too.

The container reuses a module cache and build cache between runs, so dependencies are downloaded only once. The cache lives next to the other user caches by default. Point it somewhere else with `AIMEM_GO_CACHE`:

```bash
AIMEM_GO_CACHE=/mnt/data/aimem-go ./scripts/go.sh build ./...
```

When `/mnt/workstation` exists and is writable, `scripts/go.sh` uses `/mnt/workstation/.cache/aimem-go` automatically, so the Go toolchain does not write to the internal disk. `AIMEM_GO_CACHE` overrides that. The cache only affects building. The released binary is statically linked and needs neither the cache nor a Go toolchain to run.

## Code layout

```
cmd/aimem/          the binary entry point
cmd/spike/          a spike that verifies FTS5 support in the pure-Go driver
internal/memory/    item, status, and outcome types
internal/store/     SQLite schema, CRUD, query escaping, language handling
internal/score/     outcome scoring, status derivation, similarity
internal/engine/    scope-aware operations: remember, recall, feedback, forget, promote
internal/reflect/   deterministic reflection into LESSONS.md
internal/cli/       the CLI and the MCP stdio server
```

## Command names

The binary works under several names: `aimem`, `ai-memory`, `aimemory`, `ai-memory-cli`, `aimem-cli`. The name is detected at runtime and changes the output label and the MCP server name. Every alias uses the same `.aimem` data directory on purpose, so switching names never splits a user's memory.

If you add an alias, add it to `engine.Aliases` and keep the data directory shared.

## Languages

The store is meant to be used in any language. Two things are easy to break:

- **Scripts without word separators.** Chinese, Japanese, Korean, and Thai must be kept as whole runs, because the FTS5 tokenizer treats a run as a single token.
- **Combining marks.** Thai and Devanagari use marks that belong to the preceding letter. Splitting on them breaks the words.

`internal/store/lang.go` holds the script detection, the tokenizer, and the stopword lists. Add a language to `stopwordsByLanguage` rather than special-casing it elsewhere.

## Rules for changes

1. **Keep it deterministic.** Retrieval, scoring, and reflection must produce the same result for the same input and the same clock. No LLM calls, no network.
2. **Never let user input reach the FTS5 parser raw.** Escape it in `internal/store/query.go`. Raw FTS5 syntax raises SQL errors, and worse, it lets a query change the meaning of the search.
3. **Bound every input.** Titles, bodies, tag counts, and token budgets all have explicit limits. Keep them, and keep the limits in sync with `docs/api.md`.
4. **Keep the tokenizer script-safe.** Scripts written without word separators (Chinese, Japanese, Korean, Thai) must stay whole, and combining marks must never be treated as separators. A regression here silently breaks recall for those languages.
5. **Add a test with behaviour changes.** The suite is fast; there is no excuse for an uncovered branch.
6. **Do not touch the memory store of a user.** Tests must point `AI_MEMORY_HOME` and `AI_MEMORY_PROJECT` at a temp directory. Never write to a real `~/.aimem`.
7. **Stay offline.** No telemetry, no analytics, no outbound calls.

## The learning mechanisms

Four mechanisms make the store sharper over time. When you change scoring or retrieval, keep them working together:

| Mechanism | Where |
|---|---|
| Outcome scoring and decay | `internal/score/score.go` |
| Usage reinforcement | `internal/score/score.go`, applied in `internal/engine` |
| Duplicate detection | `internal/score/similarity.go` |
| Promotion to the global scope | `internal/engine/engine.go` |

Outcome evidence must always outweigh usage reinforcement. An item that was merely recalled often should never outrank one that a human or an agent explicitly confirmed.


## Commit messages

Conventional Commits:

```
feat: add trigram fallback for prefix recall
fix: escape quotes in FTS5 query terms
docs: document the MCP tool schemas
```

## Releasing

The release workflow owns both the tag and the GitHub release.

1. Bump `versionNumber` in `internal/cli/cli.go` to the next semver version. A breaking change to the CLI, the MCP schema, or the database layout bumps the major version. A new flag or a new column bumps the minor version. A fix bumps the patch version.
2. Merge that change to `main`. The workflow reads the version from the code, publishes the tag `v<version>`, and creates the release with the binaries for every platform.

A tag pushed by hand must match the version in the code, otherwise the workflow stops. Re-running the workflow for a version that already has a release refreshes the assets instead of failing.

## Reporting bugs

This project is built with AI assistance, so it has not had the kind of human audit a mature project would have. Bug reports are genuinely valuable here.

Open an issue with:

- your OS and architecture
- `aimem version` output
- the exact commands that reproduce the problem
- the expected and actual result

If a stored memory item triggered the problem, mention the item title. **Do not include secrets, tokens, or personal data** — memory items routinely contain environment specifics.

Security issues: use GitHub's private vulnerability reporting instead of a public issue.

## License

By contributing you agree that your work is licensed under the MIT License, the same as the rest of the project.
