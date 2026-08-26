# Review Protocol

This document owns what a reviewer **returns to its controller**, and what it
writes **to disk instead**. It applies to every review dispatch in this repo —
the named guideline reviewers, the plan-adherence reviewer, `task-reviewer`,
and any ad-hoc per-unit code review.

It does not change what reviewers look for, how adversarial they are, or their
scope. Each agent's own `## Scope` section and the audit checklists remain the
contract for *what* is reviewed. This is only the shape of the answer.

---

## Two artifacts, not one

MyFleet splits review output across two paths that never share a file:

- **`docs/tasks/<id>/audit.md`** — the **pre-PR** verdict for the whole task.
  Written by `plan-adherence-reviewer`, `backend-guidelines-reviewer`, and
  `frontend-guidelines-reviewer`, dispatched together by
  `superpowers:requesting-code-review` or `/audit-plan`. There is one of
  these per task.
- **`docs/tasks/<id>/reviews/<unit>.md`** — **one commit range's** review,
  written by `task-reviewer`. There are many of these per task, one per
  reviewed unit of work.

They are kept apart deliberately: sharing one file would make concurrent
per-unit reviews clobber each other as `task-reviewer` instances write in
parallel across a plan, while the whole-task verdict needs a single stable
path a controller can check once at the end. Every rule below about writing
the full review to disk and returning a compact verdict applies to both
artifact kinds equally — only the path differs.

---

## Why this shape

A review's prose is consumed by the controller on the turn it arrives and then
carried as dead weight for every turn after. Only the verdict and the blocking
lines change what the controller does next; the reasoning belongs on disk, where
it stays readable without being re-billed.

Implementer and verifier returns are already the right shape and are explicitly
**not** changed by this document — `task-verifier`'s 478–1,546 B return is the
reference. Reviewers are the outlier this contract exists to fix.

---

## The contract

### Write the full review to a durable artifact — always

Before returning, write the complete reasoning to the artifact your agent
definition names (`audit.md`, `audit.json`, `reviews/<unit>.md`, …). Everything
below stays there and only there:

- every PASS and its file:line evidence
- every evidence table, enumeration, and checklist disposition
- every N/A and the trigger that settled it
- pasted command output, gate logs, test transcripts
- narration, false-positive dismissals, and how you arrived at a conclusion

This is not deletion. The reasoning must exist — it is what makes a review
auditable and greppable after the session ends, which is precisely what an
artifact-less review cannot offer. It simply does not belong in a context
that re-reads it on every subsequent turn.

### Return this block, verdict first

```text
verdict: APPROVED | APPROVED_WITH_FINDINGS | CHANGES_REQUIRED
artifact: <repo-relative path>
scope_confirmed: <what was actually reviewed>
blocking: <n>
  - <file:line> — <one sentence>
  - <file:line> — <one sentence>
non_blocking: <n>
not_evaluable: <n>
```

Rules:

1. **`verdict` is the first line.** A controller must be able to decide from the
   first token whether to read further. Do not open with narration, a
   false-positive dismissal, or "Now I have everything needed to write the
   report."
2. **Blocking findings are enumerated, not counted.** One line each:
   `file:line` plus one sentence. This is the one place detail stays inline,
   because a controller must be able to dispatch a fix agent without opening the
   artifact. A compressed blocking finding the controller misreads is the
   failure mode this rule exists to prevent — so if a finding genuinely needs
   two sentences, use two.
3. **Non-blocking and not-evaluable are counts only.** The detail is in the
   artifact. `not_evaluable` is never zero by omission: if you could not
   evaluate something within your scope, it is counted here and described in the
   artifact — it is never silently absorbed into a PASS.
4. **`scope_confirmed` is not padding.** Scope is the reviewer's contract, and
   it is the one fact a controller cannot recover from the artifact path without
   opening the file. State the diff range, commit range, service path, or task
   range you actually reviewed — and say so plainly if it differs from what you
   were asked to review.
5. **No PASS evidence in the return.** If a check passed, the count and the
   artifact carry it.
6. **A clean review is small.** Target ≤600 B for `APPROVED`; ≤1,200 B with
   blocking findings. These are targets, not truncation points — never drop a
   blocking finding to hit a byte count.

### Verdict semantics

| Verdict | Meaning | Controller's next action |
|---|---|---|
| `APPROVED` | No findings that change the code | Ledger it and move on. Do not read the artifact. |
| `APPROVED_WITH_FINDINGS` | Non-blocking findings only | Ledger it; read the artifact if the findings bear on upcoming work. |
| `CHANGES_REQUIRED` | At least one blocking finding | Dispatch a fix from the enumerated `blocking` lines. **Read the artifact** if a finding is not actionable as written. |

`APPROVED_WITH_FINDINGS` exists so a real concern is never suppressed to hit a
compact return. A reviewer that hides a concern to look clean has broken this
protocol far more seriously than one that returns 2 KB.

---

## The controller's side

- On `APPROVED`, do not read the artifact. That read is the counter-metric for
  this whole change: if artifact reads rise by more than about one per review,
  the return is too tight and detail belongs back in it.
- On `CHANGES_REQUIRED`, the `blocking` lines are the fix brief. Read the
  artifact when a line is not actionable as written — that is the designed
  escalation, not a failure.
- Record the review in the task ledger:

  ```sh
  tools/agent-ledger.sh append <task> --unit "<unit>" --agent-type <type> \
    --model sonnet --verdict <verdict> --caused-fix <yes|no> --artifact <path>
  ```

  `--caused-fix` is the only cheap way to learn afterwards whether reviews were
  load-bearing — without it, a batch of clean-looking reviews and a batch of
  load-bearing ones are indistinguishable after the fact.

---

## Worked example — a clean review

Full reasoning (a `task-reviewer` pass over one commit range: PASS sections
with file:line evidence, the checklist disposition for every guideline
category) goes to
`docs/tasks/task-045-vehicle-assignment/reviews/apply-assignment-endpoint.md`.
The return is:

```text
verdict: APPROVED
artifact: docs/tasks/task-045-vehicle-assignment/reviews/apply-assignment-endpoint.md
scope_confirmed: apps/fleet-service changes, commits 8c3736a..bcb5cf5
blocking: 0
non_blocking: 0
not_evaluable: 0
```

Nothing was lost: every PASS justification is on disk, and the controller's
next action — mark the unit done — is unchanged.

## Worked example — a failed review

```text
verdict: CHANGES_REQUIRED
artifact: docs/tasks/task-045-vehicle-assignment/reviews/apply-assignment-endpoint.md
scope_confirmed: apps/fleet-service changes, commits 8c3736a..bcb5cf5
blocking: 2
  - apps/fleet-service/internal/assignment/processor.go:212 — the vehicle-status transition is applied before the driver-eligibility check, so an ineligible driver can still receive the assignment.
  - apps/fleet-service/internal/assignment/administrator.go:88 — the Kafka assignment-created event is published on the success path only, so a retried request after a partial failure can never emit the event.
non_blocking: 3
not_evaluable: 1
```

Both findings are dispatchable as written; the controller never has to open the
artifact to act. The three non-blocking findings and the one not-evaluable
item are on disk, counted here so nothing is hidden.

---

## Code review is a separate gate from `tools/verify.sh`

`tools/verify.sh` is green/red on build, lint, and tests — see
[docs/verification.md](verification.md). A green gate cannot see a
cross-service seam defect: it has no way to know that a producer's new event
field is actually consumed correctly downstream. When a change crosses a
service boundary (an `apps/*` service publishing or consuming a Kafka event,
or a JSON:API relationship another service resolves), trace the event or
field into its consumers by hand and confirm a test asserts the new contract.
This is reviewer work, not gate work, and it is why review remains a required
step even when `tools/verify.sh` is clean.

---

## Reviewers do not fan out

A reviewer answers its checklist itself. Do not dispatch child agents for
individual checklist questions.

Measured, in a sibling repository: one guideline reviewer dispatched six
children — "does a Dockerfile exist and is the service referenced in it", "is
this constant already defined in the shared package", and four similar.
Together they cost **4.32M billed input for 4,669 output tokens**; four of the
six returned *fewer than 40 output tokens*, and one made a single tool call
and cost 52,857. Their returns were maximally compact — the cost was the
~35k dispatch floor plus each child's own context growth, spent on questions
the parent could answer with a path glob.

Worse, the same agent then had nothing to do while its async children ran and
burned **30 `Bash true` no-op turns** — 33% of its tool calls, ≈3.6M tokens,
≈36% of the entire agent — for zero information. `.claude/hooks/wait-loop-guard.sh`
now refuses those calls; this rule removes the reason to make them.

The break-even, and when delegation *is* right, is in
[`docs/agent-dispatch.md`](agent-dispatch.md) §Inline vs delegate.

## Reviewer default

A reviewer's default is **FAIL** — `CHANGES_REQUIRED` or, for a whole-task
audit, an equivalent fail verdict — until file:line evidence proves PASS.
Absence of a finding is not evidence of correctness; every PASS disposition
in the artifact must point at the code that justifies it.
