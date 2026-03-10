# Project Instructions

## Navigation

- Vim motions are the primary navigation model for the TUI.
- Default movement and pane navigation must always support `h`, `j`, `k`, and `l`.
- New navigation features should prefer Vim-style bindings first, then add secondary aliases only when they clearly improve discoverability.
- Do not replace Vim-style navigation with arrow-key-only or non-Vim-first controls.
- When keybinding tradeoffs come up, preserve a Vim-first workflow.

## Diff Presentation

- The diff viewer should aim for a delta-inspired level of visual polish.
- Prioritize strong color hierarchy, readable gutters, clear file and hunk headers, and polished side-by-side presentation over bare patch fidelity.
- New diff rendering work should move toward richer syntax-aware coloring, refined added/removed line backgrounds, better wrapping behavior, and clearer intra-line emphasis.
- The goal is not to clone delta feature-for-feature, but the UI should feel intentional and high quality rather than utilitarian.
