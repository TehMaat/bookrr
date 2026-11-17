# bookrr

## Versioning

The app version lives in `VERSION` (semver, no `v`). It is shown next to the
name in the header and becomes the release when merged to main: CI publishes
the `X.Y.Z` / `X.Y` images and creates the `vX.Y.Z` tag. Never create tags by hand.

Every PR that changes what users run (Go code, web UI, Dockerfile, config)
bumps `VERSION` in the same PR:

- **patch** (0.5.0 → 0.5.1): bug fixes, small tweaks, text and style changes;
- **minor** (0.5.1 → 0.6.0): new features or anything a user would notice as new.
  While on 0.x, breaking changes (renamed env vars, changed webhook/API, data
  that needs manual action) are minor bumps too: say so in the PR description;
- **major** (1.0.0): only when the owner decides.

Docs-only, CI-only and test-only changes don't bump. If main already has the
version a branch bumped to (another PR got there first), bump again after
merging main, otherwise the merge is not released.

## Issues

Every time the owner proposes something (a feature, a change, a bug, an idea),
open a GitHub issue in `TehMaat/bookrr` for it before doing anything else, even
if the work starts right away in the same session:

- title: short and descriptive; body: what was asked, why, and any open
  questions or decisions taken in the conversation;
- check open issues first and comment on an existing one instead of opening a
  duplicate;
- reply with the issue link;
- the PR that implements it references the issue (`Closes #N`), so merging it
  closes the issue.
