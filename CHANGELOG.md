# Changelog

## Unreleased

### Added

- Review threads drawer: `t` in the panel shows the selected MR's discussion threads beside the list, unresolved first; expand a thread to read it in full, `R` resolves or reopens it.

### Changed

- `c` names the worktree workspace `!<iid> <source branch>` instead of `!<iid> <project>`: the sidebar already nests it under the repository's workspace.

## 0.1.0 — 2026-09-17

- MR panel grouped by review requested, assigned, authored and mentioning you, with pipeline, approvals, merge status and thread counts.
- Actions: checkout into a worktree workspace, review in tuicr, jump to the workspace that has the MR checked out, open in browser, copy link.
- `$mr` sidebar token for each workspace's branch, and an MR count in the tab bar.
- Single-MR overlay from Ctrl+click on any MR link.
