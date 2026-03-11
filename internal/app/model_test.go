package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"better-diff/internal/domain"
	gitadapter "better-diff/internal/git"
	"better-diff/internal/render"
)

func TestHelpOverlayScrollClampsAndCanRecoverUpward(t *testing.T) {
	m := &model{
		helpOpen: true,
	}

	maxScroll := m.maxHelpScroll(helpOverlayHeight)
	for i := 0; i < 200; i++ {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		m = next.(*model)
	}

	if got, want := m.helpScroll, maxScroll; got != want {
		t.Fatalf("helpScroll after repeated down = %d, want %d", got, want)
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = next.(*model)
	if got, want := m.helpScroll, maxScroll-1; got != want {
		t.Fatalf("helpScroll after one up = %d, want %d", got, want)
	}
}

func TestFilteredCommitPickerCommitsStartsWithWorkingTreeAndIndex(t *testing.T) {
	m := &model{
		selectedCommit: 0,
		commits: []domain.CommitSummary{
			{SHA: "head", ShortSHA: "head", Subject: "current"},
			{SHA: "older", ShortSHA: "older", Subject: "older commit"},
		},
	}

	commits := m.filteredCommitPickerCommits()
	if len(commits) < 3 {
		t.Fatalf("expected working tree, index, plus older commit, got %+v", commits)
	}
	if got, want := commits[0].SHA, gitadapter.WorkingTreeRef; got != want {
		t.Fatalf("first commit picker entry SHA = %q, want %q", got, want)
	}
	if got, want := commits[0].Subject, "Working Tree (uncommitted changes)"; got != want {
		t.Fatalf("first commit picker entry subject = %q, want %q", got, want)
	}
	if got, want := commits[1].SHA, gitadapter.IndexRef; got != want {
		t.Fatalf("second commit picker entry SHA = %q, want %q", got, want)
	}
}

func TestApplySelectedCommitPickerUsesWorkingTreeCompare(t *testing.T) {
	m := &model{
		mode:           domain.ModeHistory,
		selectedCommit: 0,
		selectedFile:   0,
		commits: []domain.CommitSummary{
			{SHA: "head-sha", ShortSHA: "head", Subject: "current"},
			{SHA: "older-sha", ShortSHA: "old", Subject: "older"},
		},
		files: []domain.FileChange{
			{Path: "demo.txt", Status: "M"},
		},
	}

	cmd := m.applySelectedCommitPicker()
	if cmd != nil {
		t.Fatal("expected nil refresh command without a loaded repo")
	}
	if got, want := m.mode, domain.ModeCompareRefs; got != want {
		t.Fatalf("mode = %q, want %q", got, want)
	}
	if m.customCompare == nil {
		t.Fatal("expected custom compare to be set")
	}
	if got, want := m.customCompare.LeftRef, "head-sha"; got != want {
		t.Fatalf("LeftRef = %q, want %q", got, want)
	}
	if got, want := m.customCompare.RightRef, gitadapter.WorkingTreeRef; got != want {
		t.Fatalf("RightRef = %q, want %q", got, want)
	}
	if got, want := m.customCompare.RightLabel, "Working Tree"; got != want {
		t.Fatalf("RightLabel = %q, want %q", got, want)
	}
	if got, want := m.preferredFilePath, "demo.txt"; got != want {
		t.Fatalf("preferredFilePath = %q, want %q", got, want)
	}
	if !strings.Contains(m.actionMessage, "Working Tree") {
		t.Fatalf("actionMessage = %q, want Working Tree mention", m.actionMessage)
	}
}

func TestNewModelDefaultsToFilesFocus(t *testing.T) {
	m := NewModel("/tmp/repo").(*model)
	if got, want := m.focus, focusFiles; got != want {
		t.Fatalf("focus = %v, want %v", got, want)
	}
}

func TestStartLocalCompareModes(t *testing.T) {
	repo := &domain.RepositoryInfo{RootPath: "/tmp/repo"}
	m := &model{
		repo: repo,
		commits: []domain.CommitSummary{
			{SHA: "head-sha", ShortSHA: "head", Subject: "current"},
		},
		files: []domain.FileChange{
			{Path: "demo.txt", Status: "M"},
		},
		selectedFile: 0,
	}

	cmd := m.startLocalCompare(localCompareStaged)
	if cmd == nil {
		t.Fatal("expected staged compare to trigger refresh")
	}
	if got, want := m.currentLocalCompareMode(), localCompareStaged; got != want {
		t.Fatalf("currentLocalCompareMode() = %q, want %q", got, want)
	}
	if m.customCompare == nil || m.customCompare.RightRef != gitadapter.IndexRef {
		t.Fatalf("expected staged compare against index, got %+v", m.customCompare)
	}
	if got, want := m.preferredFilePath, "demo.txt"; got != want {
		t.Fatalf("preferredFilePath = %q, want %q", got, want)
	}

	cmd = m.startLocalCompare(localCompareUnstaged)
	if cmd == nil {
		t.Fatal("expected unstaged compare to trigger refresh")
	}
	if got, want := m.currentLocalCompareMode(), localCompareUnstaged; got != want {
		t.Fatalf("currentLocalCompareMode() = %q, want %q", got, want)
	}
	if m.customCompare == nil || m.customCompare.LeftRef != gitadapter.IndexRef || m.customCompare.RightRef != gitadapter.WorkingTreeRef {
		t.Fatalf("expected unstaged compare against working tree, got %+v", m.customCompare)
	}
}

func TestConflictSideTargetDrivesEditorLineAndStatus(t *testing.T) {
	m := &model{
		mode:         domain.ModeConflict,
		selectedFile: 0,
		files: []domain.FileChange{
			{Path: "demo.txt", Status: "U"},
		},
		conflictContents: &domain.ConflictFileContents{
			Path: "demo.txt",
			Merged: `<<<<<<< ours
alpha
||||||| base
beta
=======
gamma
>>>>>>> theirs
`,
		},
		diffLayout:   diffLayoutInline,
		diffViewMode: diffViewPatch,
		renderCache:  map[string]renderedDiff{},
	}

	width := 80
	m.renderCache[m.currentRenderCacheKey(width)] = renderedDiff{
		rows: []string{"conflict row"},
		rowMeta: []render.RowMeta{{
			Conflict:      true,
			ConflictIndex: 0,
			OldLine:       2,
			NewLine:       6,
		}},
	}

	m.setConflictSide(conflictSideTheirs)
	if got, want := m.currentEditorLine(width), 6; got != want {
		t.Fatalf("editor line with theirs target = %d, want %d", got, want)
	}
	status := m.currentDiffStatus(width)
	if !strings.Contains(status, "target theirs") {
		t.Fatalf("status = %q, want target theirs", status)
	}

	m.setConflictSide(conflictSideOurs)
	if got, want := m.currentEditorLine(width), 2; got != want {
		t.Fatalf("editor line with ours target = %d, want %d", got, want)
	}
	status = m.currentDiffStatus(width)
	if !strings.Contains(status, "target ours") {
		t.Fatalf("status = %q, want target ours", status)
	}
}

func TestEditableLocalComparisonSkipsRenames(t *testing.T) {
	m := &model{
		files: []domain.FileChange{
			{Path: "new.txt", OldPath: "old.txt", Status: "R"},
		},
		selectedFile: 0,
		customCompare: &domain.CompareSelection{
			LeftRef:    "HEAD",
			RightRef:   gitadapter.WorkingTreeRef,
			LeftLabel:  "HEAD",
			RightLabel: "Working Tree",
			DiffStyle:  domain.DiffTwoDot,
		},
		diffViewMode: diffViewPatch,
		mode:         domain.ModeCompareRefs,
	}

	if compare := m.editableLocalComparison(); compare != nil {
		t.Fatalf("expected rename working-tree comparison to be non-editable, got %+v", compare)
	}
}

func TestConflictEnterAppliesTargetSide(t *testing.T) {
	m := &model{
		repo:   &domain.RepositoryInfo{RootPath: "/tmp/repo"},
		mode:   domain.ModeConflict,
		focus:  focusDiff,
		width:  120,
		height: 40,
		files: []domain.FileChange{
			{Path: "demo.txt", Status: "U"},
		},
		conflictFiles: []domain.ConflictFile{
			{Path: "demo.txt", HasOurs: true, HasTheirs: true},
		},
		selectedFile:      0,
		diffCursor:        3,
		conflictSideFocus: conflictSideTheirs,
		renderCache:       map[string]renderedDiff{},
		conflictContents: &domain.ConflictFileContents{
			Path: "demo.txt",
			Merged: `<<<<<<< ours
alpha
=======
gamma
>>>>>>> theirs
`,
		},
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(*model)
	if cmd == nil {
		t.Fatal("expected enter in conflict mode to produce apply command")
	}
	if !strings.Contains(m.actionMessage, "Applying theirs to conflict 1") {
		t.Fatalf("actionMessage = %q, want theirs apply message", m.actionMessage)
	}
}

func TestEscapeLeavesCompareMode(t *testing.T) {
	m := &model{
		repo: &domain.RepositoryInfo{RootPath: "/tmp/repo"},
		mode: domain.ModeCompareRefs,
		commits: []domain.CommitSummary{
			{SHA: "head-sha", ShortSHA: "head", Subject: "current"},
		},
		customCompare: &domain.CompareSelection{
			LeftRef:    "HEAD",
			RightRef:   gitadapter.WorkingTreeRef,
			LeftLabel:  "HEAD",
			RightLabel: "Working Tree",
			DiffStyle:  domain.DiffTwoDot,
		},
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(*model)
	if cmd == nil {
		t.Fatal("expected escape in compare mode to trigger refresh")
	}
	if got, want := m.mode, domain.ModeHistory; got != want {
		t.Fatalf("mode = %q, want %q", got, want)
	}
	if m.customCompare != nil {
		t.Fatalf("expected custom compare to be cleared, got %+v", m.customCompare)
	}
}

func TestHLMovesBetweenLeftStackAndDiff(t *testing.T) {
	m := &model{focus: focusFiles}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = next.(*model)
	if got, want := m.focus, focusDiff; got != want {
		t.Fatalf("focus after l = %v, want %v", got, want)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = next.(*model)
	if got, want := m.focus, focusFiles; got != want {
		t.Fatalf("focus after h = %v, want %v", got, want)
	}

	m.focus = focusCommits
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = next.(*model)
	if got, want := m.focus, focusDiff; got != want {
		t.Fatalf("focus from commits after l = %v, want %v", got, want)
	}
}

func TestTabCyclesFilesCommitsDiff(t *testing.T) {
	m := &model{focus: focusFiles}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(*model)
	if got, want := m.focus, focusCommits; got != want {
		t.Fatalf("focus after first tab = %v, want %v", got, want)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(*model)
	if got, want := m.focus, focusDiff; got != want {
		t.Fatalf("focus after second tab = %v, want %v", got, want)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(*model)
	if got, want := m.focus, focusFiles; got != want {
		t.Fatalf("focus after third tab = %v, want %v", got, want)
	}
}
