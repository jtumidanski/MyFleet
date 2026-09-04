# Git Workflow

This document owns the mechanics of branch safety, the shared-stash hazard,
pushing after history rewrites, what triggers a PR build, and `gh`
authentication in this repo.

## Branch safety

Never commit or push directly to `main`. Branch protection blocks the push,
so a commit made on local `main` is stranded and never reaches the remote.
Check the branch before every `git commit`.

Setup work that must precede a feature branch still goes *on* the feature
branch — create it first; it branches from the same HEAD.

Recovery from a stray `main` commit: preserve the content on a branch
(cherry-pick if needed), then:

```sh
git fetch origin main && git reset --hard origin/main
```

Triage and the fix that resolves it stay on the same branch — do not open a
second branch for the fix. Produce the clean PR branch by rebasing onto the
latest `main` at PR time, not by juggling parallel branches. Interactive git
flags (`-i`, e.g. `git rebase -i`) are not available in this environment; use
non-interactive `git rebase <base>` and resolve conflicts as they surface.

## The shared-stash hazard

MyFleet runs many concurrent worktrees off one `.git`, so the stash stack is
shared and another session may push or pop it while you work. Never use bare
`git stash` / `git stash pop`. Prefer a temporary WIP commit to set work
aside. If you must stash: `git stash push -u -m "<unique-tag>"`, immediately
capture the SHA with `git stash list --format='%H %gs'`, restore with `git
stash apply <sha>` (not `pop`), then drop the entry after re-finding its
current `stash@{n}` by tag.

## Pushing and history rewrites

After completing a rebase/merge/history-rewrite, always push (force-push
when history was rewritten) so the PR reflects the resolved state. Do not
stop at local-only completion — a rebase resolved only locally leaves the PR
still showing conflicts.

If a push does not build — the PR checks fail on the pushed commit — fix
forward on the same branch and push again; do not force-push over the failing
commit to hide it, and do not open a fresh branch to start over.

## Build triggering and the conflict exception

A plain push to a task branch triggers the PR workflow (`.github/workflows/pr.yml`).
Do not merge `origin/main` as a routine build-triggering ritual.

The one exception: when the branch conflicts with `main`, a plain push will
fail to apply cleanly against the merge target — merge `origin/main`, resolve,
push the merge commit. The merge is the conflict resolution, not the trigger.

## `gh` authentication

Run `gh` with the token env explicitly cleared so it uses the stored
`hosts.yml` account:

```sh
env -u GH_TOKEN -u GITHUB_TOKEN gh …
```

A stale `GH_TOKEN`/`GITHUB_TOKEN` in the shell environment takes precedence
over the stored credentials and causes 401s. Never echo the token.
