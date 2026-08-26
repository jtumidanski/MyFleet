# Codemod vs. agents — when a templated transformation earns a tool

This doc answers one question at dispatch time: **you are about to send
several implementers at the same mechanical, repeated change — should one of
them write a codemod instead?** It exists because a large templated
transformation can consume far more implementer turns and tokens than
anyone evaluates at dispatch time, when a one-time AST rewrite would have been
cheaper for the mechanical portion of the work.

This document is the rule, a worked (hypothetical) example, and the
specification for the rewriter that would apply it — the rewriter itself is
**not built yet** (see "Current status" below).

## The rule

> Evaluate whether an AST codemod is cheaper **before dispatching the second
> implementer** at the same templated transformation.

"Templated transformation" means: the same multi-step edit, repeated across
call sites/files/services, where most steps are syntactic (a rename, an
added import, a threaded parameter, a fixed call inserted at a fixed
location) and at most one step requires per-site judgment (a log message, a
comment, a domain-specific choice).

### The arithmetic that sets the trigger at the second dispatch

An `task-implementer` dispatch is capped at **120 tool calls** before it must
hand back `PARTIAL` (`docs/agent-dispatch.md` §The implementer budget). That
cap is a standing contract, not a measurement, and it is what makes the
threshold computable without waiting for a transformation to run away first:

1. **Why not evaluate at the first dispatch — a precondition, not a cost
   argument.** A single site cannot tell you a transformation is templated;
   it could be a one-off. The first implementer dispatch is what reveals the
   shape (the same edit needed again elsewhere) — there is nothing to
   evaluate before that.
2. **Why the second dispatch is the trigger — a cost argument.** Writing a
   codemod is itself exactly the implementer's shape of task — a small,
   self-contained Go module (its own `go.mod`, an `analyzer.go`, table-driven
   tests over `testdata/`, a `cmd/` entry point) — so building and testing it
   once is bounded by the *same* 120-tool-call cap as any other implementer
   dispatch: at most 120 turns, worst case. A second manual dispatch at the
   same transformation is bounded by that identical cap — the same worst
   case, because it is the same kind of dispatch. So the second manual
   dispatch is the first point that both (a) confirms the transformation is
   templated (step 1) and (b) has not yet cost more than the rewriter itself
   would — one further manual dispatch already reaches the codemod's own
   worst-case build cost. That is the break-even: evaluate there, not later.

Every site the codemod covers beyond that point is a `--check`-verified
mechanical rewrite instead of another implementer turn.

## Worked example (hypothetical, MyFleet-shaped)

This did not happen; it illustrates the split the rule asks for. Suppose a
JSON:API field is renamed across resource builders in multiple services —
`apps/fleet-service/internal/fleet/resource.go`,
`apps/media-service/internal/mediaobject/resource.go`,
`apps/auth-service/internal/user/resource.go` — plus every handler that
constructs a `server.Document` from that field. The per-call-site
transformation is typically several steps:

1. rename the JSON:API attribute key in the resource's `Transform` function —
   **AST**
2. rename the corresponding struct field the transform reads — **AST**
3. thread the rename through request-decoding structs on the write path —
   **AST** (call-graph walk, AST-derivable)
4. update the field in every fixture/test that asserts on the JSON:API
   payload shape — **AST**
5. any place a human-readable label derived from the field name appears in a
   log message or an error string — **judgment**, because the wording is
   service-appropriate and not mechanically derivable from the rename alone.

Four of five steps are pure AST rewrites, one (step 3) is AST-derivable via a
call-graph walk, and one (step 5) is irreducibly judgment. That is the split
the rule asks for: **rewrite what is derivable, list what is not, and never
silently skip a site.** A codemod covering steps 1-4 would turn every site's
remaining work into "confirm/write one label," reviewable from a residue list
rather than dispatched as a full implementer turn per site.

## The deferred rewriter's contract (specification only)

If a templated transformation clears the second-dispatch threshold, the
rewriter that gets written should follow this shape — this is a specification
for future work, not a description of an existing tool. Nothing under
`tools/` currently implements it.

**Module layout**, as a standalone Go module under `tools/<name>/`:

- `tools/<name>/go.mod` — own module
- `tools/<name>/analyzer.go` — the AST rewrite logic
- `tools/<name>/analyzer_test.go` — table-driven tests over `testdata/`
- `tools/<name>/cmd/` — the CLI entry point
- `tools/<name>/testdata/` — before/after fixture pairs, built from a real
  diff that established the transformation's shape

**Two contracts it must honor:**

- **Every site is rewritten or listed, never silently skipped.**
  A site the tool cannot safely rewrite (the judgment step, or any pattern
  it doesn't recognize) goes into a residue report with file:line and reason.
  Silent omission is the failure mode that makes a codemod untrustworthy —
  a human has to be able to trust that "not in the residue list" means
  "rewritten," not "not looked at."
- **`--check` mode for use as a guard afterward.** The same analyzer, run in
  a mode that exits non-zero if any un-migrated site remains, becomes the
  regression guard once the migration lands — replacing hand-maintained
  allowlists with a mechanical check.

## Current status — dormant

**No rewriter exists yet.** This document specifies what one would look like
and the threshold at which writing one pays for itself; it does not claim one
is available to run. Because no `--check` mode exists, no batch today can be
verified as codemod-applied — until a rewriter with `--check` lands, every
batch is treated as judgment-bearing and gets the full per-task
`task-reviewer` dispatch (see [`docs/review-protocol.md`](review-protocol.md)).
