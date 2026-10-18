# goconfig release procedure

This repository is a single Go module:
`github.com/majiddarvishan/goconfig`. Release tags use a `v` prefix.

## v1.7.0 release gate

1. Merge the release-readiness PR into `main` after its CI succeeds.
2. Confirm the **post-merge** CI run succeeds for the exact `main` commit.
3. Verify `v1.7.0` does not exist (avoid moving or overwriting an existing tag).
4. Use an authorized Git client with write access to create and push one
   **annotated** `v1.7.0` tag on the verified `main` commit:

```bash
git fetch origin --tags
git switch main
git pull --ff-only origin main
git status --short
git log -1 --format='%H %s'
git ls-remote --tags origin refs/tags/v1.7.0
# Verify the command above returned nothing, then create the tag:
git tag -a v1.7.0 -m "goconfig v1.7.0 — post-commit replacement observers"
git push origin refs/tags/v1.7.0
```

The displayed HEAD must be the exact CI-verified release commit and the
worktree must be clean before tagging. If `v1.7.0` exists, stop and inspect
it; **never force-push or replace release tags**.

5. Confirm the tag points at the intended commit:

```bash
git ls-remote --tags origin refs/tags/v1.7.0 refs/tags/v1.7.0^{}
GOPROXY=https://proxy.golang.org go list -m github.com/majiddarvishan/goconfig@v1.7.0
```

6. Publish GitHub release notes from
   [`docs/releases/v1.7.0.md`](docs/releases/v1.7.0.md), only once the tag
   is visible and the module can be resolved.

**Do not run these commands against the Fluxlog repository.** Its core and
nested adapter tags are independent of the goconfig release.

The ChatGPT GitHub connector may not expose tag or release creation. In that
case, this repository change only prepares the release. The tag must be pushed
through an authorized Git client or approved release interface. Do not claim
publication without verifying the remote tag.
