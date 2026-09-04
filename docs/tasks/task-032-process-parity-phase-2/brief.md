# Brief — Process Parity Phase 2 (MyFleet)

**Read `docs/process-parity.md` first.** It is the canonical specification for this
work and was written to be self-contained: you do not need any prior conversation.
This brief only tells you which parts apply to MyFleet and in what order.

## Provenance

`docs/process-parity.md` here is a verbatim copy pinned at atlas commit
`e83f59e61`, on the unmerged branch `task-266-process-parity-agent-rename`. Atlas
phase 1 is complete and gated green but not yet merged.

Re-synced 2026-08-26 from the original pin `e75c2a168`. The only intervening
atlas commit, `e83f59e61`, added a five-line evidence note to §7 check 3
recording that `atlas-*` agent names were verified absent from the three target
repositories; it changed no portable file and no MyFleet obligation.

Set `ATLAS` to the path of the atlas worktree
`.worktrees/task-266-process-parity-agent-rename` on your machine. Before copying
anything, confirm the copy is still current:

```sh
diff "$ATLAS/docs/process-parity.md" docs/process-parity.md
```

If that diff is non-empty, the atlas PR changed the spec after this brief was
written. Stop and re-sync before proceeding — do not merge the two by hand.

## Your task

Execute `docs/process-parity.md` §6 step 2 for MyFleet, using MyFleet's own
four-phase flow (`/spec-task` → `/design-task` → `/plan-task` → `/execute-task`).
Task numbers come from `tools/task-numbers.sh next`, which this repo already has.

Full parity is the scope. That means all of §3: the portable hooks, the agent
trio, the verify entrypoint, the owner documents, `/fix-pr-bug`, the
`service-documentation` agent plus `/service-doc`, the `.claude/settings.json`
hook wiring, and the `CLAUDE.md` restructure.

## What MyFleet already has

Do not re-create these:

- `tools/task-numbers.sh` and its `SessionStart` hook `task-num-collision-detector.sh`
- the four phase commands, `/audit-plan`, `/review-todos`
- `backend-guidelines-reviewer`, `frontend-guidelines-reviewer`,
  `plan-adherence-reviewer`, `todo-scanner`
- `backend-dev-guidelines` / `frontend-dev-guidelines` skills, `skill-rules.json`,
  the `skill-activation-prompt` hook

## MyFleet's binding row (`docs/process-parity.md` §4)

| Binding | Value |
|---|---|
| Verify entrypoint | **create** `tools/verify.sh` wrapping `make ci` plus the manifest gates |
| Manifest gates | `kustomize build deploy/k8s/overlays/local` and `.../main`, then **both** `kubectl apply --dry-run=server` runs when a cluster is reachable |
| `main` overlay assertion | renders with no PVCs, no Secrets, no ClusterRole, no placeholders |
| Node bootstrap | `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22` when `npm` is absent |
| Go layout | `go.work`, `apps/*` + `packages/*` |
| Frontend path | `apps/web` |
| Go formatter | `tools/lint.sh` + `tools/lint.versions` |

`tools/verify.sh` is the only real engineering here; everything else is porting
text. Its contract must match atlas's: **flagless run exits 0 means the branch may
be called done**; `--quick` / `--no-docker` also exit 0 but skip the slow gates and
do NOT count as done.

Fold in the existing `CLAUDE.md` requirement that both `--dry-run=server` runs
happen, not just `main`. That requirement exists because a missing `namespace:` in
`deploy/k8s/infra-local/kustomization.yaml` made `kubectl apply -k
deploy/k8s/overlays/local` fail outright and slipped through ten reviews when only
the `main` dry-run was being run. Preserve that incident note somewhere durable —
it is the evidence for the rule.

## Copying the portable files

From `$ATLAS`, copy verbatim into `.claude/hooks/`:

`wait-loop-guard.sh`, `wait-loop-guard_test.sh`, `block-home-paths-in-docs.sh`,
`turn-budget.sh`, `turn-budget-guard.sh`, `fork-dispatch-guard.sh`,
`commit-boundary.sh`

These contain no atlas-specific strings as of `e75c2a168` — that is what atlas
phase 1 established. Verify after copying:

```sh
grep -l 'atlas-' .claude/hooks/*.sh   # must print nothing
```

`format-on-write.sh` must NOT be copied verbatim. Atlas's version hardcodes
`services/atlas-ui` for prettier and sources `tools/toolchain.versions`. Rebind it
to `apps/web` and to `tools/lint.sh` / `tools/lint.versions`.

The agent trio (`task-implementer`, `task-verifier`, `task-reviewer`) and the owner
documents copy from `$ATLAS/.claude/agents/` and `$ATLAS/docs/`. The owner docs
need the §5.2 genericization pass — replace atlas-specific examples (packet work,
WZ data, IDA) with MyFleet equivalents or neutral ones. **Do not delete a rule
because its example does not transfer; find a new example.**

Do not port: anything under `docs/packets/`, `docs/reverse-engineering.md`.

## The one carve-out that differs from atlas

`docs/process-parity.md` §7 check 3 exempts two files from the "no `atlas-*`
references" rule. In MyFleet **only `docs/process-parity.md` is exempt** — the
`docs/agent-dispatch.md` exemption is atlas-only, because atlas is the only repo
that ever used the `atlas-*` names and therefore the only one needing a
historical-cutoff note. Your check is:

```sh
git grep -lE 'atlas-(implementer|verifier|reviewer)' -- . ':!docs/tasks' \
  | grep -vxE 'docs/process-parity\.md'
```

It must print nothing.

## Done

`docs/process-parity.md` §7 lists the six checks. Checks 1, 4, 5, and 6 are
cross-repo and cannot be fully evaluated from MyFleet alone — report your side of
them and say plainly that the pairwise comparison is not evaluable here. Checks 2
and 3 are fully checkable in this repo and must pass.

Report back what you could not verify rather than asserting it.
