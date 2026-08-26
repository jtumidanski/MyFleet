# Tooling Conventions

This document owns three conventions for how commands and edits are run in
this repo: locating Go module source, waiting on long-running processes, and
general shell/editing hygiene.

## Locating Go module source

Never sweep the filesystem to locate a Go dependency's source. Ask the
toolchain instead:

```sh
go list -m -f '{{.Dir}}' <module>
```

This prints the directory in ~0.02s, whether the module resolves to the
module cache or to a local `replace`. The same applies to `go doc <pkg>` for
a symbol and `go list -m all` for the version set.

`find /` takes minutes on WSL2 and answers a question the toolchain answers
directly. `find` is for paths you own, rooted at a directory you name — never
at `/`.

## Waiting on processes

Never spend inference turns waiting for a process. Launch it once with a
bound — `run_in_background: true`, or `Monitor` with an until-loop — and do
something else or hand back.

Repeated `sleep` / `ps aux | grep` / `echo waiting` / `for i in $(seq …); do
sleep` calls are the anti-pattern: each one re-reads the whole context to
learn nothing, and they cluster late in a session where that is most
expensive. If the process exceeds its bound, kill it and fall back; do not
keep polling.

The same holds for **waiting on a child agent**. There is no wait primitive
because none is needed: completions arrive as notifications, so do other work
or end the turn and be re-invoked. Emitting a no-op to stay alive is the worst
version of this — zero information for a full turn's cost.

`.claude/hooks/wait-loop-guard.sh` makes this machine-checked rather than
advisory, the way `.claude/hooks/fork-dispatch-guard.sh` does for forks. It
refuses bare no-ops, sleep-driven polls, and broad `ps`/`pgrep` sweeps. It
deliberately allows real process debugging — `ps -p <pid>`, `kill`/`pkill`,
`kubectl`, `docker ps`, `top -b -n1` — and anything prefixed
`POLL-JUSTIFIED: <reason>`, mirroring `FORK-JUSTIFIED:`. A considered wait
costs one sentence; the reflexive one is blocked.

## Ask for a fact rather than deriving it

Mechanical repository facts have deterministic sources. Use them:

| Want | MyFleet command |
|---|---|
| which task, which worktree, which branch | `git worktree list`; `git branch --show-current`; `ls docs/tasks/<id>/`; `tools/task-numbers.sh check` |
| which surfaces a diff touched | `git diff --name-only <base>...HEAD \| sed -n 's\|^\(apps\|packages\)/\([^/]*\)/.*\|\1/\2\|p' \| sort -u` |
| the next task number | `tools/task-numbers.sh next` |
| the pinned linter version | `tools/lint.versions` |
| a slice of a large document | `tools/doc-slice.sh` — see [slice-first.md](slice-first.md) |

Do not probe for toolchain availability (`command -v`, `--version`, `which`)
before running a command you already expect to exist; let the command itself
fail if it is missing. Prefer the table above's mechanical answer over deriving
the same fact from a longer `grep`/`find` chain — the tooling already knows it.

The same discipline applies to tracking documents: find `docs/tasks/<id>/` or
`docs/TODO.md` with `Glob`/`Grep` rather than assuming a path from memory or a
naming convention that may have drifted.

Batch a gate-log or review-artifact read with the ledger append that records
its verdict into the same tool call — reading `docs/tasks/<task>/audit.md` and
then separately calling `tools/agent-ledger.sh append` is two round trips for
one decision; issue them together.

## Shell and editing conventions

Prefer portable POSIX shell; avoid zsh/direnv-specific constructs and batch
patch loops that can produce garbled or unapplied output. For a multi-file
edit, prefer per-file Edit/Write over a shell patch loop.

Quote glob arguments in shell tool calls — `--include='*.go'`, not
`--include=*.go` — zsh expands an unquoted glob before `grep` sees it,
producing `no matches found` and a wasted retry.

Preserve line endings when editing — do not normalize CRLF→LF as a side
effect; it inflates diffs with spurious changes.

Always use repo-relative paths or placeholders in committed files; never a
literal home or absolute path like `/home/<name>/...` — a committed absolute
path is not reproducible on another machine. `block-home-paths-in-docs.sh`
enforces this under `docs/`.

**Defer to the global config; do not restate it.** Where a convention is
genuinely global rather than repository-scoped — which editing tool to reach
for, shell-output proxies — this document stays silent. An echo is a second
copy that will drift; the global file is the one source, and this document
only states mechanics that are specific to this repository.
