# Downgraded Card Variant — Cache Identity Fix — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop `media-service` from letting a browser store thumbnail bytes under a card URL for five minutes, by carrying the "this response was downgraded" fact from `Processor.Content` to the HTTP handler and serving those responses `private, no-store`.

**Architecture:** `ContentInfo` gains a `Downgraded bool`. `Processor.Content` sets it at the one call site that performs a substitution — the branch that falls through after a `card` lookup returns `server.ErrNotFound` and re-opens as `thumbnail`. The content handler selects `Cache-Control` from that flag. The domain reports *what happened*; transport decides *what that means on the wire*. No new types, no signature change, no HTTP vocabulary in the domain layer.

**Tech Stack:** Go 1.x (per-service modules under a root `go.work`), `chi` router, `logrus`, table-free hand-rolled `testing` tests with in-package fakes (`fakeStore`, `fakeVariants`, `fakeCardGenerator`).

## Global Constraints

Copied verbatim from PRD §2 (non-goals), §10 (scope discipline) and design §6. Every task's requirements implicitly include these:

- **No files under `apps/web/` may be modified.** The frontend is out of scope entirely; React Query's `staleTime`/`gcTime` are not tuned in this task.
- **No new response header.** `X-Media-Variant-Downgraded` and anything like it is rejected. The downgrade stays undetectable by clients.
- **`apps/media-service/internal/processing/card.go` is unmodified.** The saturation drop stays a drop.
- **No new metric, no log-level change, no new configuration, no migration, no backfill.** The existing `Debug` line in the downgrade branch is retained verbatim.
- **No status-code or body change** on any path.
- **`private` is retained in both cache policies.** `private, max-age=300` and `private, no-store` — the two values differ in exactly one token.
- **The three existing `private, max-age=300` assertions in `resource_test.go` (lines 313, 737, 764) must pass without being edited.** They are the regression guard for the non-downgraded row of the policy table. If a step wants to touch them, the production change is wrong.
- **Every new assertion must be proven able to fail** (project rule: revert the production change, confirm red, restore). A test that never went red has not been shown to test anything.
- **`make ci` must pass** before the branch is claimed done. No deployment manifests change, so no extra `kustomize` work beyond what `make ci` already runs.

### Vocabulary

| Term | Meaning here |
|---|---|
| **downgrade** | Serving `thumbnail` bytes in response to `?variant=card` because no card variant is servable. The *only* substitution in the service. |
| **sharp** / **soft** | The card image the caller asked for / the thumbnail actually sent. |
| **fault** | Any error that is not `server.ErrNotFound` — a database or object-store failure. Faults never downgrade. |

---

## File Structure

Four files, all in `apps/media-service/internal/mediaobject/`. No files are created.

| File | Responsibility | Task |
|---|---|---|
| `processor.go` | Domain. `ContentInfo` gains `Downgraded bool` (a statement of fact about the bytes); `Content` sets it on the one substituting branch. | 1 |
| `processor_test.go` | Proves the fact is set on exactly the substituting paths and absent everywhere else — including the explicit-`thumbnail` case whose bytes are identical to a downgraded response's. | 1 |
| `resource.go` | Transport. Selects the `Cache-Control` value from `info.Downgraded`. | 2 |
| `resource_test.go` | Proves a downgraded response carries `private, no-store` and is otherwise byte-identical to what it returns today. | 2 |

The two tasks split at the domain/transport boundary. Task 1 is reviewable on its own — the flag is correct, set, and unread. Task 2 consumes it. A reviewer can reject the flag's placement without rejecting the cache policy, or vice versa.

---

### Task 1: `ContentInfo.Downgraded` — the domain fact

**Files:**
- Modify: `apps/media-service/internal/mediaobject/processor.go:110-131` (the `ContentInfo` struct) and `:392-420` (the tail of `Processor.Content`)
- Test: `apps/media-service/internal/mediaobject/processor_test.go` (edits at `:667`, `:696`, `:891`, `:1016`, `:1046`)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `ContentInfo.Downgraded bool` — read by Task 2 as `info.Downgraded` on the value returned from `proc.Content(ctx, id, fleetID, v)`. The field is a plain `bool` whose `false` zero value is correct on every non-substituting path, so `ContentInfo` stays comparable (`info != (ContentInfo{})` at `processor_test.go:763` must keep compiling) and no existing construction site needs an edit.

**Note on line numbers:** the `:NNN` references above are as of commit `01ab457`. Steps 1–7 add lines to `processor_test.go`, so later step targets shift downward as you go. Locate each edit by the function/subtest name given in the step, not by line number.

**Note on "no `variant` parameter":** `ParseContentVariant("")` returns `ContentOriginal` (`contentvariant.go:26-27`), so at the processor level the no-parameter request *is* the `original` request — the same code path, already covered by Step 1. Do not write a separate processor test for it. Its distinct handler-level coverage is the pre-existing `TestGetContent_noVariantParamIsUnchanged` (`resource_test.go:718`), which asserts `private, max-age=300` and must keep passing untouched.

- [ ] **Step 1: Write the failing assertions in `processor_test.go` — the negative space first**

These four edits assert `Downgraded == false` (or a zero `ContentInfo`) on paths where nothing was substituted. They are what stops an implementation that flags everything from passing.

In `TestContent_originalIsUnchanged`, after the `info.Size` check:

```go
	if info.Downgraded {
		t.Fatal("Downgraded = true for ?variant=original, want false — the original is never a substitution")
	}
```

In `TestContent_variantFoundServesVariantBytes`, after the `info.Size` check:

```go
	// The D2 regression catcher: these bytes are byte-identical to a
	// downgraded response's, so the flag is the ONLY thing distinguishing
	// them. The caller asked for the thumbnail and got the thumbnail.
	if info.Downgraded {
		t.Fatal("Downgraded = true for an explicit ?variant=thumbnail, want false — nothing was substituted")
	}
```

In `TestContent_cardPresentServesTheCardBytes`, change the `Content` call from `_, rc, err :=` to bind the info, and add the assertion after the `cards.scheduled` check:

```go
	info, rc, err := pr.Content(context.Background(), obj.ID(), "fleet-a", ContentCard)
```

```go
	if info.Downgraded {
		t.Fatal("Downgraded = true for a card that exists, want false — the downgrade branch was never entered")
	}
```

In `TestContent_cardLookupErrorIs500WithNoDowngrade`, change `_, _, err :=` to bind the info, and add the assertion after the `cards.scheduled` check:

```go
	info, _, err := pr.Content(context.Background(), obj.ID(), "fleet-a", ContentCard)
```

```go
	if info != (ContentInfo{}) {
		t.Fatalf("ContentInfo = %+v on a database fault, want the zero value — a fault must never look like a served response", info)
	}
```

- [ ] **Step 2: Write the failing assertions in the downgrade matrix**

All four edits are inside `TestContent_cardDowngradesToThumbnailOnly`.

Subtest `"card missing, thumbnail present, serves the thumbnail bytes"` — add after the `info.Size` check:

```go
		// The fact the handler needs: these are thumbnail bytes under a card
		// URL, so nothing may store them under that identity.
		if !info.Downgraded {
			t.Fatal("Downgraded = false on a card→thumbnail substitution, want true")
		}
```

Subtest `"card missing and thumbnail missing is 404"` — change `_, rc, err :=` to `info, rc, err :=` and add after the `rc != nil` check:

```go
		if info != (ContentInfo{}) {
			t.Fatalf("ContentInfo = %+v, want the zero value — nothing was served, so there is nothing to mark", info)
		}
```

Subtest `"display missing still 404s even with a thumbnail present"` — change `_, rc, err :=` to `info, rc, err :=` and add after the `rc != nil` check:

```go
		if info != (ContentInfo{}) {
			t.Fatalf("ContentInfo = %+v, want the zero value — the downgrade must not generalise to display", info)
		}
```

Subtest `"card row present but object missing downgrades AND reschedules"` — change `_, rc, err :=` to `info, rc, err :=` and add after the `store.getCalls` check:

```go
		// Store/DB drift is still a genuine substitution: the caller asked for
		// a card and is holding a thumbnail, so the response must not be
		// stored under the card's URL.
		if !info.Downgraded {
			t.Fatal("Downgraded = false when the card row's object is missing, want true")
		}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run from the worktree root:

```bash
go test -race github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject/ -run 'TestContent_' -v
```

Expected: **compile failure**, `info.Downgraded undefined (type ContentInfo has no field or method Downgraded)`. That is the correct red for Step 1 and 2 — the field does not exist yet. Do not proceed until you have seen it.

- [ ] **Step 4: Add the `Downgraded` field to `ContentInfo`**

In `processor.go`, append the field to the `ContentInfo` struct, after `Disposition`:

```go
	// Downgraded is true when the bytes being served are a smaller rendition
	// than the caller asked for: today, a thumbnail standing in for a card
	// that has not been generated yet. It is a statement of fact about the
	// bytes, deliberately carrying no HTTP vocabulary — the handler decides
	// what the fact means on the wire. The false zero value is correct for
	// every path that serves what was asked for, which is why no existing
	// construction site needs to change.
	Downgraded bool
```

- [ ] **Step 5: Run the tests to verify the failure moves from compile to assertion**

```bash
go test -race github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject/ -run 'TestContent_' -v
```

Expected: it compiles now. The four negative assertions from Step 1 **PASS** (the zero value is already `false`); the two positive assertions from Step 2 **FAIL** with `Downgraded = false on a card→thumbnail substitution, want true`. Two failing subtests inside `TestContent_cardDowngradesToThumbnailOnly`, nothing else red.

- [ ] **Step 6: Set the flag on the downgrade branch**

In `processor.go`, replace the final statement of `Processor.Content`:

```go
	// 200 with the thumbnail's own bytes, type and disposition — or its own 404
	// if there is no thumbnail either. No third attempt.
	return pr.openVariant(ctx, m, ContentThumbnail)
```

with:

```go
	// 200 with the thumbnail's own bytes, type and disposition — or its own 404
	// if there is no thumbnail either. No third attempt.
	info, rc, err = pr.openVariant(ctx, m, ContentThumbnail)
	if err != nil {
		// No thumbnail either: a 404, and no response to mark. Setting the
		// flag before this check would stamp it on a zero struct being
		// discarded — harmless today, and a trap the first time someone
		// inspects info on an error path.
		return ContentInfo{}, nil, err
	}
	// Only this call site knows a substitution happened. openVariant opened
	// exactly the rendition it was asked for, so from where it stands nothing
	// was substituted; pushing the flag down there would make an explicit
	// ?variant=thumbnail request and a downgraded one indistinguishable again,
	// which is the bug this change exists to fix.
	info.Downgraded = true
	return info, rc, nil
```

`info`, `rc` and `err` are already declared by `info, rc, err := pr.openVariant(ctx, m, want)` earlier in the function, so this is plain assignment (`=`), not a short declaration. `ContentInfo` is returned by value, so mutating the local copy touches nothing else.

- [ ] **Step 7: Run the tests to verify they pass**

```bash
go test -race github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject/ -run 'TestContent_' -v
```

Expected: PASS, all subtests.

- [ ] **Step 8: Prove the new assertions can fail**

Per the project's standing rule, a test that has not been seen red proves nothing. The compile error in Step 3 does not count as red for the *positive* assertions — it would have appeared no matter what they asserted.

Comment out the `info.Downgraded = true` line added in Step 6, then run:

```bash
go test -race github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject/ -run TestContent_cardDowngradesToThumbnailOnly -v
```

Expected: `card missing, thumbnail present` and `card row present but object missing` both FAIL with `want true`; the other two subtests still pass.

Then invert the check — restore the line and instead set the flag unconditionally on every path (temporarily set `Downgraded: true` in `openVariant`'s success return) and run:

```bash
go test -race github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject/ -run 'TestContent_' -v
```

Expected: `TestContent_variantFoundServesVariantBytes` FAILS with `want false — nothing was substituted`. This is the assertion that proves the negative space is guarded, and it is the one a D2 regression would trip.

**Restore both edits before continuing.** Re-run the full package and confirm green:

```bash
go test -race github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject/ -v
```

- [ ] **Step 9: Verify no unrelated file changed and commit**

```bash
git status --porcelain
```

Expected: exactly two modified files, `apps/media-service/internal/mediaobject/processor.go` and `apps/media-service/internal/mediaobject/processor_test.go`. Nothing under `apps/web/`, nothing in `internal/processing/`.

```bash
git add apps/media-service/internal/mediaobject/processor.go apps/media-service/internal/mediaobject/processor_test.go
git commit -m "feat(media-service): carry the card→thumbnail downgrade on ContentInfo

Processor.Content already knows it is substituting a thumbnail for an
unavailable card, but the fact evaporated at the return: the value was
byte-identical to a genuine ?variant=thumbnail response. ContentInfo now
carries Downgraded, set at the one call site that performs the
substitution, so a caller can behave differently without re-deriving a
condition it has no way to reconstruct.

No behaviour change yet — the flag is set and unread."
```

---

### Task 2: `no-store` on downgraded responses

**Files:**
- Modify: `apps/media-service/internal/mediaobject/resource.go:207-208` (inside the `GET /media/{id}/content` handler)
- Test: `apps/media-service/internal/mediaobject/resource_test.go` (one new test, inserted after `TestGetContent_thumbnailServesVariantWithoutContentLength`)

**Interfaces:**
- Consumes: `ContentInfo.Downgraded bool` from Task 1, read as `info.Downgraded` on the value returned by `proc.Content(req.Context(), id, identity.ActiveFleetID, v)` at `resource.go:164`. This is the only caller of `Processor.Content` in the service.
- Produces: nothing consumed by a later task. The `Cache-Control` policy table is the deliverable:

  | `info.Downgraded` | `Cache-Control` |
  |---|---|
  | `false` | `private, max-age=300` (unchanged) |
  | `true` | `private, no-store` |

**Fixture note:** the existing `thumbnailRouter` helper (`resource_test.go:697`) seeds `thumbnail` and `display` refs and **no `card`**, so `?variant=card` against it already produces a downgraded response. No new fixture is needed. `seedStoredObject` uploads `photo.png` as `image/png`; the thumbnail row's own type is `image/jpeg`, so a downgraded response's `Content-Disposition` is `inline; filename="photo.png"` — the same value `TestGetContent_variantIsHardenedLikeTheOriginal` already asserts at `:662`.

- [ ] **Step 1: Write the failing test**

Insert into `resource_test.go` immediately after `TestGetContent_thumbnailServesVariantWithoutContentLength` (which ends at `:767`):

```go
// TestGetContent_downgradedCardIsNotStored is the whole point of the cache
// change. thumbnailRouter seeds thumbnail and display but no card, so
// ?variant=card downgrades. Those soft bytes must not be stored under the
// sharp image's URL: the card generation the downgrade schedules usually
// completes within seconds, and nothing can invalidate a cache entry that
// recorded no substitution.
func TestGetContent_downgradedCardIsNotStored(t *testing.T) {
	router, proc, _ := thumbnailRouter(t)
	id := seedStoredObject(t, proc, "fleet-a", []byte("original-bytes"))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, memberRequest(http.MethodGet, "/media/"+id+"/content?variant=card", nil))

	if cc := rec.Header().Get("Cache-Control"); cc != "private, no-store" {
		t.Fatalf("Cache-Control = %q, want private, no-store — a soft image must never be stored under the card URL", cc)
	}
	// Everything else is byte-identical to what this request returns today:
	// the substitution stays undetectable by the client (FR-DG-4).
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "thumb-bytes" {
		t.Fatalf("body = %q, want thumb-bytes", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Fatalf("Content-Type = %q, want the thumbnail row's own image/jpeg", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); cd != `inline; filename="photo.png"` {
		t.Fatalf("Content-Disposition = %q, want inline with the original's filename", cd)
	}
	if xcto := rec.Header().Get("X-Content-Type-Options"); xcto != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff on every response", xcto)
	}
	if cl := rec.Header().Get("Content-Length"); cl != "" {
		t.Fatalf("Content-Length = %q, want it omitted — a variant records no byte count", cl)
	}
	// No new response header may be introduced: the four above are the entire
	// header set a content response carries, downgraded or not.
	if n := len(rec.Header()); n != 4 {
		t.Fatalf("response carries %d headers (%v), want exactly Content-Type, X-Content-Type-Options, "+
			"Content-Disposition and Cache-Control — the downgrade must stay invisible to clients", n, rec.Header())
	}
}
```

No new imports are needed: `net/http`, `net/http/httptest` and `testing` are already imported.

- [ ] **Step 2: Run the test to verify it fails**

```bash
go test -race github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject/ -run TestGetContent_downgradedCardIsNotStored -v
```

Expected: FAIL with `Cache-Control = "private, max-age=300", want private, no-store`. If it fails on any other line — status, body, disposition, header count — stop and investigate: the fixture assumption is wrong and the test is measuring something other than the cache policy.

- [ ] **Step 3: Select the cache policy from the flag**

In `resource.go`, replace:

```go
			// Per-fleet authorized bytes — never store in a shared cache.
			w.Header().Set("Cache-Control", "private, max-age=300")
```

with:

```go
			// Per-fleet authorized bytes — never store in a shared cache.
			// private is unconditional; only the freshness half varies.
			cacheControl := "private, max-age=300"
			if info.Downgraded {
				// The bytes under this URL are a stand-in for a card that is
				// usually generated within seconds of this request. Storing
				// them would pin the soft image to the sharp image's URL for
				// the whole max-age window, with nothing able to invalidate it
				// — the response records no substitution, so no client and no
				// cache can tell it apart from the real thing.
				cacheControl = "private, no-store"
			}
			w.Header().Set("Cache-Control", cacheControl)
```

Do not extract a `cacheControlFor(bool) string` helper: one branch, one caller, both rows covered by tests. Extract it if a third policy ever appears (design D3).

- [ ] **Step 4: Run the test to verify it passes**

```bash
go test -race github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject/ -run TestGetContent_ -v
```

Expected: PASS — the new test, and every pre-existing `TestGetContent_*`. In particular `TestGetContent_pdfIsAttachmentWithNosniff`, `TestGetContent_noVariantParamIsUnchanged` and `TestGetContent_thumbnailServesVariantWithoutContentLength` still pass with their `private, max-age=300` assertions **unedited**. If any of them needed a change, the production edit is wrong.

- [ ] **Step 5: Prove the new assertion can fail**

Comment out the `cacheControl = "private, no-store"` line added in Step 3, then run:

```bash
go test -race github.com/jtumidanski/myfleet/apps/media-service/internal/mediaobject/ -run TestGetContent_downgradedCardIsNotStored -v
```

Expected: FAIL with `Cache-Control = "private, max-age=300", want private, no-store`. Restore the line and re-run to confirm green.

- [ ] **Step 6: Confirm the regression guards were not edited**

```bash
git diff -- apps/media-service/internal/mediaobject/resource_test.go | grep -E '^-' | grep 'max-age=300'
```

Expected: **no output.** Any hit means an existing `private, max-age=300` assertion was removed or rewritten, which PRD §10 forbids — the whole point of those three lines is that the non-downgraded row is unchanged. Revert the edit if there is a hit.

- [ ] **Step 7: Run the full package and the scope check**

```bash
go test -race github.com/jtumidanski/myfleet/apps/media-service/... 
git status --porcelain
git diff --stat main...HEAD -- apps/web apps/media-service/internal/processing
```

Expected: all tests pass; `git status` shows only `resource.go` and `resource_test.go` modified; the last command prints **nothing** — `apps/web/` and `internal/processing/card.go` are untouched across the whole branch.

- [ ] **Step 8: Commit**

```bash
git add apps/media-service/internal/mediaobject/resource.go apps/media-service/internal/mediaobject/resource_test.go
git commit -m "fix(media-service): do not cache a downgraded card response

A ?variant=card request that falls back to the thumbnail was served
Cache-Control: private, max-age=300, so the browser stored the soft image
under the sharp image's URL for five minutes — long past the point where
the card generation the downgrade schedules had completed, with nothing
able to invalidate the entry.

Downgraded responses are now private, no-store. Every other content
response keeps max-age=300 unchanged. private is retained on both: these
are per-fleet authorized bytes and must never reach a shared cache."
```

---

### Task 3: Full verification

**Files:** none modified. This task is the gate before review.

**Interfaces:**
- Consumes: the working tree as left by Tasks 1 and 2.
- Produces: evidence that `make ci` passes, for the completion claim.

- [ ] **Step 1: Load Node, which is not always on PATH**

`make ci` includes `fe-test` and `fe-build`, so `npm` must be available:

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
```

- [ ] **Step 2: Run the full CI target**

```bash
make ci
```

Expected: PASS through `lint-check`, `vet`, `test`, `build`, `fe-test`, `fe-build`, `manifests`, `carfax-template`. No deployment manifest changed in this task, so `manifests` is a no-op regression check rather than something this branch affects — but it must still be green.

If `lint-check` objects to the new `cacheControl` local, fix the lint complaint rather than reverting the branch structure; the shape is a design decision (D3), not an accident.

- [ ] **Step 3: Confirm the acceptance criteria**

Walk PRD §10 and confirm each box against the work, not against memory:

```bash
# The flag exists and is documented
grep -n -B 8 'Downgraded bool' apps/media-service/internal/mediaobject/processor.go
# It is set at exactly one site
grep -rn 'Downgraded = true' apps/media-service/
# Both policy values, and only those two
grep -n 'private, ' apps/media-service/internal/mediaobject/resource.go
# Nothing out of scope changed
git diff --stat main...HEAD
```

Expected: one `Downgraded bool` with a doc comment; exactly one `Downgraded = true`, in `processor.go`; exactly `private, max-age=300` and `private, no-store` in `resource.go`; the diff touching only the four files named in the File Structure table plus this task's docs.

- [ ] **Step 4: Code review before PR**

Per `CLAUDE.md`, the code-review step is mandatory and must not be skipped even though the plan looks complete. Invoke `superpowers:requesting-code-review` (it dispatches `plan-adherence-reviewer` and `backend-guidelines-reviewer`; no frontend files changed, so the frontend reviewer is not needed). Findings land in `docs/tasks/task-021-downgraded-variant-cache-control/audit.md`.

Address the findings, then open the PR.

---

## Self-Review

**Spec coverage** — every PRD functional requirement and design decision maps to a step:

| Requirement | Where |
|---|---|
| FR-DG-1 — `ContentInfo.Downgraded bool`, documented, `false` zero value | Task 1 Step 4 |
| FR-DG-2 — only the card→thumbnail downgrade sets it | Task 1 Step 6; asserted positively in Step 2 and negatively in Step 1 |
| FR-DG-2 rows: present card / explicit thumbnail / original / no parameter | Task 1 Step 1 (with the no-parameter case explained as identical to `original` at the processor level, and covered at the handler level by the untouched `TestGetContent_noVariantParamIsUnchanged`) |
| FR-DG-2 rows: display miss, missing thumbnail, store drift, fault | Task 1 Steps 1–2 |
| FR-DG-3 — the policy table | Task 2 Step 3; both rows asserted in Step 4 |
| FR-DG-4 — all other headers unchanged | Task 2 Step 1 (the four explicit header assertions plus the header-count check) |
| FR-DG-5 — scheduling untouched | Global Constraints; verified by Task 3 Step 3's `git diff --stat`, and by the pre-existing `cards.scheduled` assertions that Task 1 leaves in place |
| Design D1 — the fact rides on `ContentInfo` | Task 1 Step 4 |
| Design D2 — set in `Content`, never in `openVariant`, after the error check | Task 1 Step 6, with the explicit-thumbnail assertion in Step 1 as its regression catcher and the inversion in Step 8 as its proof |
| Design D3 — the handler owns the policy, no helper extracted | Task 2 Step 3 |
| Design D4 — `private` in both branches | Task 2 Steps 1 and 3; Task 3 Step 3's `grep` |
| PRD §10 verification — existing assertions unedited | Task 2 Step 6 |
| PRD §10 verification — tests proven red | Task 1 Step 8, Task 2 Step 5 |
| PRD §10 verification — `make ci` | Task 3 Step 2 |

**Placeholder scan:** no TBDs, no "add error handling", no "similar to Task N". Every code step carries the literal code to write; every run step carries the exact command and the expected output.

**Type consistency:** `Downgraded` is spelled identically in the struct (Task 1 Step 4), the assignment (Task 1 Step 6), the processor assertions (Task 1 Steps 1–2) and the handler read (Task 2 Step 3). `ContentInfo` stays comparable, so the `info != (ContentInfo{})` form used in three assertions compiles. `Processor.Content`'s signature is unchanged, so its single caller at `resource.go:164` needs no edit.
