# Project Instructions

## Navigation

- Vim motions are the primary navigation model for the TUI.
- Default movement and pane navigation must always support `h`, `j`, `k`, and `l`.
- New navigation features should prefer Vim-style bindings first, then add secondary aliases only when they clearly improve discoverability.
- Do not replace Vim-style navigation with arrow-key-only or non-Vim-first controls.
- When keybinding tradeoffs come up, preserve a Vim-first workflow.
