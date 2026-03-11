# better-diff

`better-diff` is a Mac-first terminal Git review tool for people who want a fast, Vim-friendly way to inspect history, compare refs, review local changes, and resolve merge conflicts without leaving the terminal.

It is built with Go and Bubble Tea and aims for a more premium diff experience than a plain `git diff` dump: stronger visual hierarchy, side-by-side layouts, full-file compare mode, inline blame, and a conflict workflow that feels closer to a real merge tool.

## Highlights

- Vim-first navigation with `h/j/k/l`, `tab`, `/`-style filtering, and focused-pane movement
- Commit graph browsing with direct compare selection from the history pane
- Patch and full-file compare modes
- Inline and side-by-side patch rendering
- Local review modes for:
  - `HEAD -> Working Tree`
  - `HEAD -> Index`
  - `Index -> Working Tree`
- Arbitrary ref-to-ref compare
- File-to-commit, file-to-working-tree, and file-to-index compare
- Inline blame with detail view
- Whitespace-ignore on by default, toggleable with `w`
- Conflict review with:
  - side-by-side `ours` / `theirs`
  - live merge-result preview
  - block-level `ours` / `theirs` / `both` resolution
  - whole-file `ours` / `theirs` resolution
  - base inspection for the selected block
- Fullscreen diff mode
- Open selected file at the selected diff line in your editor

## Feature List

### Review

- Browse commits and changed files
- See the selected file diff immediately
- Jump between hunks or change blocks with `[` and `]`
- Toggle patch vs full-file compare with `f`
- Toggle inline vs side-by-side patch layout with `i`
- Toggle fullscreen diff with `F`
- Adjust diff context with `+` and `-`

### Compare

- Compare default base branch vs `HEAD`
- Compare anchored commit vs selected commit
- Compare arbitrary refs, branches, remotes, or tags
- Compare selected file against:
  - working tree
  - staged snapshot (`Index`)
  - an older commit
- Review all local changes, staged changes only, or unstaged changes only

### Local Change Workflows

- `A`: compare `HEAD -> Working Tree`
- `S`: compare `HEAD -> Index`
- `W`: compare `Index -> Working Tree`
- `u`: revert selected hunk in editable local patch views
- `U`: revert selected file in editable local patch views

### Merge / Conflict Workflows

- Detect merge conflicts automatically
- Browse conflicted files
- Target `ours` or `theirs` with `H` / `L`
- Apply the targeted side to the selected block with `Enter`
- Apply `ours`, `theirs`, or `both` explicitly with `1`, `2`, `3`
- Accept whole-file `ours` / `theirs` with `O`, `T`
- Inspect base content with `K`
- See live merged-file context below the conflict inputs

### Diff Presentation

- Delta-inspired visual direction
- Styled file banners and hunk headers
- Syntax-aware coloring
- Intra-line emphasis for paired edits
- Wrapped long lines with continuation handling
- Inline blame section separators

## Requirements

- macOS arm64 is the primary supported target today
- Git installed and available on `PATH`
- Go 1.24+ to build from source
- A terminal with ANSI color support

## Install

### Build locally

```sh
make build
```

This creates:

```sh
bin/better-diff
```

### Install locally

```sh
make install
```

By default this installs to:

```sh
~/bin/better-diff
```

Install somewhere else:

```sh
PREFIX=/usr/local/bin ./scripts/install.sh bin/better-diff
```

### Build a Mac release bundle

```sh
make dist-mac VERSION=0.1.0
```

This creates:

- `dist/better-diff_darwin_arm64/better-diff`
- `dist/better-diff_darwin_arm64/install.sh`
- `dist/better-diff_darwin_arm64/README.md`
- `dist/better-diff_darwin_arm64.tar.gz`

### Build all packaged targets

```sh
make dist-all VERSION=0.1.0
```

This currently produces release bundles for:

- `darwin/arm64`
- `darwin/amd64`
- `linux/amd64`
- `linux/arm64`
- `windows/amd64`

Artifacts land in `dist/`.

### Move to another Mac

1. Copy `dist/better-diff_darwin_arm64.tar.gz` to the other machine.
2. Unpack it.
3. Run `./install.sh`.
4. Ensure `~/bin` is on `PATH`.

### Move to Windows

1. Copy `dist/better-diff_windows_amd64.zip` to the target machine.
2. Unzip it.
3. Run from PowerShell or Command Prompt inside a Git repo:

```powershell
.\better-diff.exe
.\better-diff.exe C:\path\to\repo
```

Windows is included as a test build right now rather than a fully validated primary platform.

## Usage

Run in the current repository:

```sh
better-diff
```

Run against another repository:

```sh
better-diff /path/to/repo
```

Show version:

```sh
better-diff --version
```

## Sample Workflows

### 1. Review what you are about to commit

```text
Launch better-diff
Press S
Review the staged files and patch hunks
Press U to discard the selected staged file if needed
```

### 2. Review only unstaged work

```text
Launch better-diff
Press W
Review unstaged tracked and untracked changes
Press u to discard a selected hunk from the working tree
```

### 3. Compare your branch against another branch

```text
Press b
Pick the left ref
Pick the right ref
Browse files and diff output
Press Esc to leave compare mode
```

### 4. Compare a single file against the working tree or an older commit

```text
Focus the Files pane
Select a file
Press Enter
Pick Working Tree, Index, or an older commit
```

### 5. Resolve a merge conflict

```text
Open better-diff during a conflicted merge
Select a conflicted file
Move to a conflict block with [ or ]
Use H or L to target ours or theirs
Press Enter to apply that side
Watch the merge-result pane update below
Press K to inspect the base block when needed
```

## Navigation

### Global

- `tab`: cycle `Files -> Commits -> Diff`
- `h` / `l`: switch between the left stack and the diff
- `j` / `k`: move inside the focused pane
- `:`: open the action menu
- `?`: open full keyboard help
- `Esc`: close overlays, exit fullscreen, or leave compare mode
- `q`: quit

### Review / Compare

- `Enter` on files: compare the selected file
- `Enter` on commits: anchor or compare the selected commit from the graph
- `A`: compare `HEAD -> Working Tree`
- `S`: compare `HEAD -> Index`
- `W`: compare `Index -> Working Tree`
- `b`: compare arbitrary refs
- `c`: toggle default base vs `HEAD`
- `v`: toggle commit compare anchor
- `g`: return to history mode
- `w`: toggle whitespace-ignore
- `B`: toggle inline blame
- `K`: open blame detail
- `[` / `]`: jump hunks or change blocks
- `f`: toggle patch vs full-file
- `i`: toggle inline vs side-by-side patch view
- `F`: toggle fullscreen
- `o`: open in editor at selected line

### Conflict mode

- `H` / `L`: target `ours` or `theirs`
- `Enter`: apply the targeted side to the selected block
- `1`, `2`, `3`: apply `ours`, `theirs`, or `both`
- `O`, `T`: accept whole-file `ours` or `theirs`
- `K`: inspect the base block

## Editor Integration

`better-diff` will try to open the selected file in:

1. `$VISUAL`
2. `$EDITOR`
3. `code`

When a selected diff line maps cleanly to a file line, it opens the editor at that line.

## Notes

- `Working Tree` is the default file-compare target.
- `Index` is available as an explicit staged-snapshot compare target.
- Whitespace-ignore is enabled by default to reduce noise.
- Conflict mode only stages a file once all conflict markers are gone.

## Status

This project is already usable, but the roadmap still includes more real-world feedback and polish on:

- large-repo performance
- merge ergonomics
- graph interaction
- premium renderer polish

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE).
