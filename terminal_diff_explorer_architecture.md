# Terminal Diff Explorer Architecture

## 1. Purpose

Build a terminal-native Git history and diff explorer that combines:

- a visual commit graph
- fast navigation across branches, commits, and files
- side-by-side and inline diff views
- focused commit/file/range comparison workflows
- merge conflict inspection and resolution support
- keyboard-first interaction with a clean, modern TUI

The target user is a developer who likes the depth of GitLens/GitKraken diff exploration but wants the experience to live entirely in the terminal.

This is not a full Git client in v1. It is a **history, comparison, and review tool**.

---

## 2. Product Vision

### Problem

Current terminal Git tools split the experience awkwardly:

- `delta` renders diffs well but is not an interactive explorer
- `lazygit` is good for repo operations but weak for deep diff UX
- `tig` browses history well but is limited in modern comparison workflows
- `vimdiff` handles raw side-by-side diffs but has poor ergonomics for most users
- Neovim plugins are powerful but inherit editor-centric workflows rather than product-grade review UX

### Goal

Create a terminal application that feels like:

- **GitLens/GitKraken for exploration and comparison**
- **VS Code for diff readability and navigation**
- **a TUI, not an editor plugin**

### Non-goals for v1

- staging and partial staging
- commit authoring
- rebasing UI
- remote management and pull/push workflows
- stash management
- issue tracking integrations
- code hosting integrations (GitHub/GitLab/Bitbucket)

These can come later, but they should not shape the v1 architecture.

---

## 3. Primary User Workflows

### Workflow A: Explore history visually

1. Open repo
2. See visual commit graph with branches and refs
3. Move through commits with keyboard
4. See commit metadata and changed files update instantly
5. Open diff for a selected file

### Workflow B: Review a commit deeply

1. Select a commit from graph/log
2. Open changed files list
3. Preview diffs inline
4. Expand to full side-by-side diff
5. Jump between hunks and files quickly

### Workflow C: Compare branches or commit ranges

1. Choose base and target refs
2. See file list for the comparison
3. Filter and search changed files
4. Inspect diffs per file
5. Toggle inline and side-by-side view

### Workflow D: Trace a single file over time

1. Search/select a file
2. View file-specific commit history
3. Pick two commits affecting that file
4. See file diff between those commits

### Workflow E: Handle merge conflicts

1. Detect merge/rebase/cherry-pick conflict state
2. Show conflicting files clearly
3. Open 2-way or 3-way conflict viewer
4. Accept left/right/both or edit manually
5. Save and mark resolved

---

## 4. UX Principles

### 4.1 Keyboard first, obvious second

The tool should be fast for expert users without feeling cryptic. A user should be able to discover the core workflow without memorizing 60 keybindings.

### 4.2 Persistent spatial layout

Commit graph, file list, and diff view should have stable positions. Avoid jumping users across modes unless necessary.

### 4.3 Diff readability over density

The diff pane is the star. Favor readable layout, syntax highlighting, hunk navigation, and collapsed unchanged sections over maximizing information density.

The visual quality bar should be closer to tools like delta than to raw patch output. That means strong color hierarchy, polished file and hunk headers, readable gutters, syntax-aware coloring, and side-by-side layouts that feel designed rather than merely functional.

### 4.4 Selection drives everything

The entire application should follow a selection-based model:

- selected ref/commit/range determines file list
- selected file determines diff view
- selected hunk determines detail/metadata state

### 4.5 Fast path to comparison

Comparing branches, commits, and files should be a first-class concept, not a hidden advanced feature.

---

## 5. Proposed Information Architecture

## 5.1 Main layout

A three-pane layout with optional overlays:

### Left pane: Revision Navigator

Shows one of:

- commit graph/log
- branches/tags/refs
- file history mode
- compare selection mode

Responsibilities:

- browse graph
- search commits/refs
- select one or two revisions
- choose compare mode

### Middle pane: Changed Files / Context

Shows files for the current selection:

- files changed in selected commit
- files changed in selected range
- conflicted files in merge mode
- file history results in file trace mode

Responsibilities:

- search/filter files
- sort by path, status, size of diff
- open file diff

### Right pane: Diff Viewer

Shows:

- inline diff
- side-by-side diff
- conflict view
- optional file content peek

Responsibilities:

- render hunks with syntax awareness
- jump between hunks/files
- collapse unchanged sections
- show moved code markers later

### Overlays / drawers

Optional modal or drawer views:

- commit details
- help / keybindings
- range picker
n- compare setup dialog
- command palette
- blame/line history popup

Note: the compare setup dialog is modal because it changes the context of the whole app.

---

## 6. Major Modes

Modes should be explicit but limited.

### 6.1 History Mode

Default mode.

- commit graph visible
- selecting a commit populates file list and diff preview

### 6.2 Compare Mode

Used for:

- commit vs commit
- branch vs branch
- branch vs working tree
- stash later

Should support:

- 2-dot and 3-dot semantics
- compare presets like `main...HEAD`

### 6.3 File Trace Mode

Focused on one file.

- file history list replaces graph or appears as sub-mode
- compare selected file revisions directly

### 6.4 Conflict Mode

Activated automatically when merge state exists.

- conflicted files prioritized
- diff view supports 3-way merge presentation
- conflict actions available

---

## 7. Functional Requirements

## 7.1 Repository Discovery

- open current working directory repo automatically
- support nested repos and worktrees later
- detect bare repo and unsupported states cleanly
- refresh repo state incrementally

## 7.2 Commit Graph

- render ASCII or Unicode graph of commits and branch topology
- show refs: local branches, remote branches, tags, HEAD
- support paging and virtualized scrolling
- search by SHA, author, commit message, file path
- filter by branch or author later

## 7.3 Commit Details

- full SHA
- author
- date
- subject/body
- parent SHAs
- refs attached
- stats summary

## 7.4 File List for Selection

For commit or range selection:

- added, modified, deleted, renamed, copied
- support rename detection when enabled
- show path and status
- later: show additions/deletions counts per file

## 7.5 Diff Viewer

Must support:

- inline diffs
- side-by-side diffs
- syntax highlighting
- line numbers
- next/prev hunk navigation
- next/prev file navigation
- collapse unchanged regions
- adjustable context lines
- later: word diff and moved block indicators

## 7.6 Comparison Engine

Must support:

- working tree vs index
- working tree vs HEAD
- commit vs parent
- commit vs commit
- branch vs branch
- merge-base comparison
- file-specific compare across arbitrary revisions

## 7.7 Merge Conflict Support

- detect unresolved conflict files
- display conflict markers as structured hunks
- support LOCAL / BASE / REMOTE / MERGED view model
- accept ours/theirs/both at hunk or file level later
- save merged file contents
- optionally call `git add` after resolve later

## 7.8 Search and Navigation

- fuzzy search commits
- fuzzy search files in comparison
- quick jump to branch/tag/ref
- command palette for major operations

## 7.9 Command Integration

- open external editor on current file and line
- copy SHA/path
- open raw patch view
- optional integration with pager tools later

---

## 8. Technical Architecture

## 8.1 High-level architecture

Use a layered architecture:

1. **TUI Presentation Layer**
2. **Application State + Commands Layer**
3. **Domain Services Layer**
4. **Git Adapter Layer**
5. **Rendering Engine Layer**

This keeps Git access, app logic, and rendering separated enough to swap implementations later.

### 8.1.1 Presentation Layer

Responsible for:

- layout
- focus management
- keybindings
- rendering panes
- modal/dialog orchestration

### 8.1.2 Application Layer

Responsible for:

- current mode
- selected commit/range/file
- command dispatch
- caching and background loading coordination
- optimistic UI state where appropriate

### 8.1.3 Domain Services Layer

Responsible for:

- commit graph building
- comparison setup
- diff preparation
- conflict state modeling
- file history modeling

### 8.1.4 Git Adapter Layer

Responsible for:

- executing Git commands
- parsing outputs
- normalizing into app models
- handling platform-specific process concerns

### 8.1.5 Rendering Engine Layer

Responsible for:

- side-by-side layout of diffs
- syntax tokenization hooks
- collapsed region computation
- hunk mapping and cursor navigation

---

## 9. Technology Options

## 9.1 Current implementation path: Go + Bubble Tea

The project is now implemented as:

- **Go**
- **Bubble Tea** for terminal state/update flow
- **Lip Gloss** for layout and styling
- Git via child processes initially

### Why this path

- better raw performance
- stronger native-terminal feel
- easier static distribution, especially on macOS
- better fit for richer diff rendering and large-repo interaction

### Risks

- richer rendering still requires custom work
- graph rendering and virtualization are still non-trivial
- iteration speed is lower than the original TS prototype path

## 9.2 Retired prototype path: TypeScript + Ink

The React/Ink prototype served its purpose as a UX spike, but it has been retired from the repo after proving too weak on performance for the intended rendering complexity.

## 9.3 Recommendation

Continue investing in the Go/Bubble Tea implementation and use the earlier TS prototype only as historical product reference, not as a maintained runtime.

---

## 10. Domain Model

## 10.1 Core entities

### Repository

```ts
interface Repository {
  rootPath: string;
  gitDir: string;
  headRef?: string;
  isMergeInProgress: boolean;
  isRebaseInProgress: boolean;
  isCherryPickInProgress: boolean;
}
```

### Ref

```ts
interface GitRef {
  name: string;
  fullName: string;
  type: 'local-branch' | 'remote-branch' | 'tag' | 'head' | 'stash';
  targetSha: string;
}
```

### Commit

```ts
interface Commit {
  sha: string;
  shortSha: string;
  parents: string[];
  authorName: string;
  authorEmail: string;
  authoredAt: string;
  subject: string;
  body?: string;
  refs: string[];
}
```

### File change

```ts
interface FileChange {
  path: string;
  oldPath?: string;
  status: 'A' | 'M' | 'D' | 'R' | 'C' | 'U';
  additions?: number;
  deletions?: number;
}
```

### Diff selection

```ts
interface DiffSelection {
  left: RevisionSpec;
  right: RevisionSpec;
  path?: string;
  comparisonMode: 'working-tree' | 'commit-range' | 'branch-range' | 'file-history' | 'conflict';
}
```

### Revision spec

```ts
interface RevisionSpec {
  kind: 'working-tree' | 'index' | 'ref' | 'commit' | 'merge-base';
  value?: string;
}
```

### Diff model

```ts
interface DiffFileModel {
  path: string;
  hunks: DiffHunk[];
  isBinary: boolean;
  language?: string;
}

interface DiffHunk {
  header: string;
  oldStart: number;
  oldLines: number;
  newStart: number;
  newLines: number;
  lines: DiffLine[];
}

interface DiffLine {
  type: 'context' | 'add' | 'del' | 'meta';
  oldLineNumber?: number;
  newLineNumber?: number;
  text: string;
}
```

### Conflict model

```ts
interface ConflictFile {
  path: string;
  stages: {
    base?: string;
    ours?: string;
    theirs?: string;
    merged?: string;
  };
}
```

---

## 11. Git Adapter Design

## 11.1 Strategy

Use Git CLI commands as the initial backend.

Do not start with libgit2.

### Why

- Git CLI is available everywhere the app will run
- behavior matches user expectations
- avoids deep native bindings and platform pain
- lets the product team validate UX before investing in lower-level integration

### Risks

- parsing command output requires careful normalization
- large repos may need caching and batching
- process spawning needs throttling

## 11.2 Commands by feature

### Repo state

- `git rev-parse --show-toplevel`
- `git rev-parse --git-dir`
- `git status --porcelain=v2 --branch`

### Commit graph / log

- `git log --graph --decorate --date=iso --format=...`
- file-specific history: `git log --follow -- <path>`

### Refs

- `git for-each-ref --format=...`

### Changed files for commit/range

- `git diff --name-status`
- `git diff --numstat`
- `git show --name-status --format=... <sha>`

### Diff content

- `git diff --no-ext-diff --unified=<n> <left> <right> -- <path>`
- `git show --no-ext-diff <sha> -- <path>`

### Merge conflict data

- `git ls-files -u`
- `git show :1:path`
- `git show :2:path`
- `git show :3:path`

## 11.3 Parsing strategy

Where possible, prefer custom `--format` strings with explicit separators over human-friendly output. Avoid parsing presentation-oriented output if a machine-friendly alternative exists.

For commit graph rendering, there are two paths:

1. parse `git log --graph` and keep the textual graph representation for MVP
2. build a true graph model from parents/refs later for richer rendering

Recommendation: start with option 1 for speed, evolve to option 2 when the graph UI matures.

---

## 12. State Management

## 12.1 App state slices

```ts
interface AppState {
  repo: RepoState;
  layout: LayoutState;
  focus: FocusState;
  history: HistoryState;
  comparison: ComparisonState;
  files: FileListState;
  diff: DiffViewState;
  conflicts: ConflictState;
  search: SearchState;
  ui: UIState;
}
```

### RepoState

- repo root
- current branch
- merge/rebase status
- loading/error flags

### HistoryState

- visible commit window
- selected commit index
- loaded commits
- commit search query/results

### ComparisonState

- base revision
- target revision
- compare mode
- recent comparisons

### FileListState

- file list for active selection
- selected file
- filter query
- sort mode

### DiffViewState

- loaded diff models by selection cache key
- active file diff
- display mode inline/side-by-side
- collapsed region state
- active hunk index

### ConflictState

- conflict files
- selected conflict file
- resolution state

## 12.2 Command model

Use a command/action model rather than mutating component-local state directly.

Examples:

- `OPEN_REPOSITORY`
- `SELECT_COMMIT`
- `SELECT_FILE`
- `SET_COMPARE_LEFT`
- `SET_COMPARE_RIGHT`
- `TOGGLE_DIFF_MODE`
- `NEXT_HUNK`
- `ENTER_CONFLICT_MODE`
- `ACCEPT_OURS`

This keeps UX logic testable and detached from the rendering layer.

---

## 13. Diff Rendering Engine

This is the heart of the product.

## 13.1 Rendering requirements

- predictable column widths
- stable gutter widths
- syntax-aware tokenization where possible
- side-by-side mapping by hunk
- support wrapping or horizontal scroll as a mode
- collapsed unchanged regions

## 13.2 Inline view

Simpler mode:

- one unified patch stream
- ideal for narrow terminals and fast scanning

## 13.3 Side-by-side view

Main premium experience:

- left old content
- right new content
- matched line mapping when possible
- empty placeholder rows for insertions/deletions
- syntax highlighting per side
- selectable active side later for conflict resolution

## 13.4 Conflict rendering

For unresolved conflicts:

- show ours/base/theirs/merged as structured views
- v1 can begin with a 3-column or 2+1 stacked model
- keep model separate from generic diff rendering

## 13.5 Renderer implementation note

Do not depend on `delta` for primary rendering inside the app.

Reasons:

- delta is optimized as a pager renderer
- the app needs full control over layout, selection, collapse state, and navigation
- side-by-side mapping is product logic, not a post-processing concern

You may optionally support exporting the current diff to delta or opening a raw patch via delta outside the main UI.

---

## 14. Caching and Performance

## 14.1 Key principle

Perceived performance matters more than raw completeness.

Users should feel the app reacts immediately when moving through commits.

## 14.2 Caching strategy

Cache by stable keys:

- commit metadata by SHA
- file lists by selection range key
- diff models by `left:right:path:context:mode`

### Suggested approach

- LRU cache for diff models
- prefetch adjacent commit file lists
- prefetch selected file diff after commit selection stabilizes

## 14.3 Background loading

- initial graph window loads fast
- details and file lists populate asynchronously
- diff loads lazily for selected file
- cancel stale requests when selection changes rapidly

## 14.4 Large repo strategy

- virtualize commit list
- virtualize file list
- defer expensive stats until needed
- cap default diff context lines
- add manual “expand more context” action

---

## 15. Keybindings Strategy

Keybindings should be mnemonic and minimal.

Vim motions are a hard requirement for navigation. The TUI should be Vim-first by default, with `h`, `j`, `k`, and `l` treated as the primary movement model across panes, lists, and diff navigation. Secondary aliases can exist for discoverability, but they should not displace Vim-style control.

### Proposed core set

- `j` / `k`: move selection
- `h` / `l`: move focus between panes
- `enter`: open / drill in
- `tab`: cycle focus
- `/`: search in focused pane
- `g`: go to graph/history
- `f`: focus files
- `d`: focus diff
- `c`: open compare picker
- `i`: toggle inline vs side-by-side
- `[` / `]`: previous/next hunk
- `{` / `}`: previous/next file
- `o`: open in external editor
- `?`: help
- `q`: close overlay / back

Avoid trying to clone Vim, Lazygit, and GitKraken all at once.

---

## 16. Error Handling and Resilience

The app must gracefully handle:

- not in a git repo
- detached HEAD
- binary files
- huge files
- malformed encoding
- missing refs
- in-progress rebase/merge/cherry-pick
- command timeouts or Git not installed

Errors should be displayed inline in the relevant pane when possible, not as catastrophic app failures.

---

## 17. Testing Strategy

## 17.1 Unit tests

Focus on:

- Git output parsers
- selection/comparison reducers
- diff model transformations
- conflict parsing

## 17.2 Integration tests

Spin up fixture repos and test:

- graph loading
- file list for commit/range
- diff generation
- merge conflict detection
- rename handling

## 17.3 Snapshot tests

Use carefully for:

- commit graph rendering
- diff pane rendering
- collapsed region rendering

## 17.4 Manual UX validation

Essential because this is a UX-heavy product.

Create a set of real-world repo scenarios:

- simple linear history
- messy branch graph
- large monorepo-ish history
- rename-heavy history
- merge conflict state
- generated files / binary files

---

## 18. Security and Safety Considerations

- treat Git output as untrusted text
- sanitize terminal control characters where needed
- avoid shell interpolation with untrusted paths
- prefer argument arrays to raw shell command strings
- protect against path traversal assumptions in conflict file loading

---

## 19. Packaging and Distribution

### MVP

- run via `npx` or local dev command
- optional Homebrew tap later

### Longer term

- standalone binaries if moved to Go
- npm package with install script if staying in Node

---

## 20. Implementation Roadmap

## Phase 0: UX spike

Goal: prove the core interaction model quickly.

Build:

- basic TUI shell
- commit list pane
- file list pane
- simple inline diff pane
- selection-driven state updates

Success criteria:

- can move through commits and see file list and preview update smoothly

## Phase 1: Real comparison workflows

Build:

- compare mode
- branch/commit range selection
- file-specific compare
- basic search in commits and files
- diff context controls

Success criteria:

- can compare `main...HEAD`, commit-to-commit, and file-between-two-commits cleanly

## Phase 2: Premium diff UX

Build:

- side-by-side renderer
- collapsed unchanged sections
- hunk navigation
- syntax highlighting

Success criteria:

- diff readability is clearly better than current terminal alternatives for target workflows

## Phase 3: Conflict mode

Build:

- conflict detection
- conflicted files list
- 3-way conflict viewer
- simple accept ours/theirs flow

Success criteria:

- can inspect and resolve common merge conflicts without leaving the app

## Phase 4: Polish and scale

Build:

- caching/prefetching
- better graph rendering
- command palette
- external editor integration
- profiling on large repos

Success criteria:

- app feels fast and stable on daily-driver repos

---

## 21. Suggested Project Structure

```text
cmd/
  better-diff/
    main.go
internal/
  app/
    model.go
  domain/
    models.go
  git/
    client.go
    client_test.go
  render/
    diff.go
    diff_test.go
bin/
```

---

## 22. Recommended MVP Feature Set

If you want the sharpest first version, ship only this:

- open repo automatically
- commit list with graph-ish view
- changed files list for selected commit
- inline diff preview
- compare mode for `main...HEAD` and commit-to-commit
- side-by-side diff for selected file
- commit and file search

That is enough to prove whether this is actually better than Lazygit/Tig/Vimdiff for your use case.

---

## 23. Open Design Questions

These should be answered early with prototypes.

1. Should the commit graph be a true custom graph model immediately or start as parsed Git graph output?
2. How should side-by-side diff handle long lines: wrap, crop, or horizontal scroll?
3. How much of conflict resolution belongs in v1 versus deferring to editor handoff?
4. Should compare mode be modal, or should it feel like a persistent dual-selection model?
5. What profiling work is still needed to keep the Go/Bubble Tea build fast on daily-driver repos?

---

## 24. Initial Recommendation

Build this as a **selfish tool** first.

Make the first success metric simple:

> Can I use this tool to inspect commit history and compare files across commits/branches more comfortably than I can with Lazygit, Tig, Vimdiff, and raw Git commands?

If the answer becomes yes, then you have something worth continuing.

If the answer is no, you will still have identified exactly which UX layer is missing and whether the right next move is a refined internal tool, a Neovim plugin, or a more ambitious terminal product.

---

## 25. Suggested Next Step

Before writing much production code, prototype these three screens only:

1. commit graph/list pane
2. changed files pane
3. side-by-side diff pane

Use fake data first, then wire Git data in.

That will answer the most important question quickly:

**Does the interaction model feel right enough to justify the project?**
