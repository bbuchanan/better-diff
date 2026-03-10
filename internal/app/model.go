package app

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"better-diff/internal/domain"
	gitadapter "better-diff/internal/git"
	"better-diff/internal/render"
)

type paneFocus int

const (
	focusCommits paneFocus = iota
	focusFiles
	focusDiff
)

type diffLayout string

const (
	diffLayoutInline diffLayout = "inline"
	diffLayoutSplit  diffLayout = "split"
)

type repoLoadedMsg struct {
	repo      domain.RepositoryInfo
	commits   []domain.CommitSummary
	refs      []domain.RefSummary
	conflicts []domain.ConflictFile
	err       error
}

type filesLoadedMsg struct {
	key   string
	files []domain.FileChange
	err   error
}

type prefetchedFilesMsg struct {
	key   string
	files []domain.FileChange
}

type prefetchedDiffMsg struct {
	key  string
	diff string
}

type diffLoadedMsg struct {
	key      string
	diff     string
	conflict *domain.ConflictFileContents
	err      error
}

type actionDoneMsg struct {
	message string
	err     error
}

type paletteCommand struct {
	id          string
	label       string
	description string
}

type renderedDiff struct {
	rows     []string
	hunkRows []int
}

type model struct {
	cwd string

	width  int
	height int

	repo *domain.RepositoryInfo

	commits       []domain.CommitSummary
	refs          []domain.RefSummary
	files         []domain.FileChange
	conflictFiles []domain.ConflictFile

	mode            domain.ExplorerMode
	focus           paneFocus
	selectedCommit  int
	selectedFile    int
	diffScroll      int
	contextLines    int
	presetDiffStyle domain.DiffStyle
	commitDiffStyle domain.DiffStyle
	compareAnchor   string
	customCompare   *domain.CompareSelection
	paletteOpen     bool
	paletteQuery    string
	paletteSelected int
	diffLayout      diffLayout
	refPickerOpen   bool
	refPickerQuery  string
	refPickerStep   int
	refPickerLeft   *domain.RefSummary
	refPickerRight  *domain.RefSummary
	refPickerSelect int

	diff             string
	conflictContents *domain.ConflictFileContents
	loading          bool
	loadingFiles     bool
	loadingDiff      bool
	repositoryErr    string
	filesErr         string
	diffErr          string
	actionMessage    string

	fileCache     map[string][]domain.FileChange
	diffCache     map[string]string
	conflictCache map[string]domain.ConflictFileContents
	renderCache   map[string]renderedDiff
}

func NewModel(cwd string) tea.Model {
	return &model{
		cwd:             cwd,
		mode:            domain.ModeHistory,
		focus:           focusCommits,
		contextLines:    3,
		presetDiffStyle: domain.DiffThreeDot,
		commitDiffStyle: domain.DiffTwoDot,
		diffLayout:      diffLayoutInline,
		fileCache:       map[string][]domain.FileChange{},
		diffCache:       map[string]string{},
		conflictCache:   map[string]domain.ConflictFileContents{},
		renderCache:     map[string]renderedDiff{},
	}
}

func (m *model) Init() tea.Cmd {
	m.loading = true
	return loadRepositoryCmd(m.cwd)
}

func loadRepositoryCmd(cwd string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := gitadapter.Context(10 * time.Second)
		defer cancel()

		repo, err := gitadapter.DiscoverRepository(ctx, cwd)
		if err != nil {
			return repoLoadedMsg{err: err}
		}

		commits, err := gitadapter.ListCommits(ctx, repo.RootPath, 120)
		if err != nil {
			return repoLoadedMsg{err: err}
		}

		refs, err := gitadapter.ListRefs(ctx, repo.RootPath)
		if err != nil {
			return repoLoadedMsg{err: err}
		}

		conflicts, err := gitadapter.ListConflictFiles(ctx, repo.RootPath)
		if err != nil {
			return repoLoadedMsg{err: err}
		}

		return repoLoadedMsg{
			repo:      repo,
			commits:   commits,
			refs:      refs,
			conflicts: conflicts,
		}
	}
}

func loadCommitFilesCmd(root, sha string) tea.Cmd {
	key := "commit:" + sha
	return func() tea.Msg {
		ctx, cancel := gitadapter.Context(5 * time.Second)
		defer cancel()

		files, err := gitadapter.ListCommitFiles(ctx, root, sha)
		return filesLoadedMsg{key: key, files: files, err: err}
	}
}

func prefetchCommitFilesCmd(root, sha string) tea.Cmd {
	key := "commit:" + sha
	return func() tea.Msg {
		ctx, cancel := gitadapter.Context(5 * time.Second)
		defer cancel()

		files, err := gitadapter.ListCommitFiles(ctx, root, sha)
		if err != nil {
			return prefetchedFilesMsg{key: key}
		}
		return prefetchedFilesMsg{key: key, files: files}
	}
}

func loadRangeFilesCmd(root string, compare domain.CompareSelection) tea.Cmd {
	key := fmt.Sprintf("range:%s:%s:%s", compare.LeftRef, compare.DiffStyle, compare.RightRef)
	return func() tea.Msg {
		ctx, cancel := gitadapter.Context(5 * time.Second)
		defer cancel()

		files, err := gitadapter.ListRangeFiles(ctx, root, compare.LeftRef, compare.RightRef, compare.DiffStyle)
		return filesLoadedMsg{key: key, files: files, err: err}
	}
}

func loadCommitDiffCmd(root, sha, path string, contextLines int) tea.Cmd {
	key := fmt.Sprintf("commit:%s:%s:%d", sha, path, contextLines)
	return func() tea.Msg {
		ctx, cancel := gitadapter.Context(8 * time.Second)
		defer cancel()

		diff, err := gitadapter.GetCommitDiff(ctx, root, sha, path, contextLines)
		return diffLoadedMsg{key: key, diff: diff, err: err}
	}
}

func loadRangeDiffCmd(root string, compare domain.CompareSelection, path string, contextLines int) tea.Cmd {
	key := fmt.Sprintf("range:%s:%s:%s:%s:%d", compare.LeftRef, compare.DiffStyle, compare.RightRef, path, contextLines)
	return func() tea.Msg {
		ctx, cancel := gitadapter.Context(8 * time.Second)
		defer cancel()

		diff, err := gitadapter.GetRangeDiff(ctx, root, compare.LeftRef, compare.RightRef, compare.DiffStyle, path, contextLines)
		return diffLoadedMsg{key: key, diff: diff, err: err}
	}
}

func prefetchCommitDiffCmd(root, sha, path string, contextLines int) tea.Cmd {
	key := fmt.Sprintf("commit:%s:%s:%d", sha, path, contextLines)
	return func() tea.Msg {
		ctx, cancel := gitadapter.Context(8 * time.Second)
		defer cancel()

		diff, err := gitadapter.GetCommitDiff(ctx, root, sha, path, contextLines)
		if err != nil {
			return prefetchedDiffMsg{}
		}
		return prefetchedDiffMsg{key: key, diff: diff}
	}
}

func prefetchRangeDiffCmd(root string, compare domain.CompareSelection, path string, contextLines int) tea.Cmd {
	key := fmt.Sprintf("range:%s:%s:%s:%s:%d", compare.LeftRef, compare.DiffStyle, compare.RightRef, path, contextLines)
	return func() tea.Msg {
		ctx, cancel := gitadapter.Context(8 * time.Second)
		defer cancel()

		diff, err := gitadapter.GetRangeDiff(ctx, root, compare.LeftRef, compare.RightRef, compare.DiffStyle, path, contextLines)
		if err != nil {
			return prefetchedDiffMsg{}
		}
		return prefetchedDiffMsg{key: key, diff: diff}
	}
}

func loadConflictContentsCmd(root, path string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := gitadapter.Context(5 * time.Second)
		defer cancel()

		contents, err := gitadapter.GetConflictFileContents(ctx, root, path)
		if err != nil {
			return diffLoadedMsg{err: err}
		}

		return diffLoadedMsg{
			key:      "conflict:" + path,
			conflict: &contents,
		}
	}
}

func acceptConflictCmd(root, path, side string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := gitadapter.Context(5 * time.Second)
		defer cancel()

		err := gitadapter.AcceptConflictSide(ctx, root, path, side)
		if err != nil {
			return actionDoneMsg{err: err}
		}

		return actionDoneMsg{message: fmt.Sprintf("Applied %s and staged %s.", side, path)}
	}
}

func (m *model) selectedCommitValue() *domain.CommitSummary {
	if m.selectedCommit < 0 || m.selectedCommit >= len(m.commits) {
		return nil
	}
	return &m.commits[m.selectedCommit]
}

func (m *model) selectedFileValue() *domain.FileChange {
	if m.selectedFile < 0 || m.selectedFile >= len(m.files) {
		return nil
	}
	return &m.files[m.selectedFile]
}

func (m *model) selectedConflictValue() *domain.ConflictFile {
	file := m.selectedFileValue()
	if file == nil {
		return nil
	}

	for _, conflict := range m.conflictFiles {
		if conflict.Path == file.Path {
			c := conflict
			return &c
		}
	}

	return nil
}

func (m *model) activeComparison() *domain.CompareSelection {
	switch m.mode {
	case domain.ModeComparePreset:
		if m.repo == nil || m.repo.DefaultCompareBase == "" {
			return nil
		}
		return &domain.CompareSelection{
			LeftRef:    m.repo.DefaultCompareBase,
			RightRef:   "HEAD",
			LeftLabel:  m.repo.DefaultCompareBase,
			RightLabel: "HEAD",
			DiffStyle:  m.presetDiffStyle,
		}
	case domain.ModeCompareCommits:
		if m.compareAnchor == "" {
			return nil
		}
		anchor := m.commitBySHA(m.compareAnchor)
		commit := m.selectedCommitValue()
		if anchor == nil || commit == nil {
			return nil
		}
		return &domain.CompareSelection{
			LeftRef:    anchor.SHA,
			RightRef:   commit.SHA,
			LeftLabel:  anchor.ShortSHA,
			RightLabel: commit.ShortSHA,
			DiffStyle:  m.commitDiffStyle,
		}
	case domain.ModeCompareRefs:
		if m.customCompare == nil {
			return nil
		}
		return m.customCompare
	}

	return nil
}

func (m *model) filteredRefs() []domain.RefSummary {
	query := strings.ToLower(strings.TrimSpace(m.refPickerQuery))
	if query == "" {
		return m.refs
	}

	filtered := make([]domain.RefSummary, 0, len(m.refs))
	for _, ref := range m.refs {
		haystack := strings.ToLower(ref.Name + " " + ref.FullName + " " + ref.ShortSHA + " " + ref.Type)
		if strings.Contains(haystack, query) {
			filtered = append(filtered, ref)
		}
	}
	return filtered
}

func (m *model) selectedRefValue() *domain.RefSummary {
	refs := m.filteredRefs()
	if m.refPickerSelect < 0 || m.refPickerSelect >= len(refs) {
		return nil
	}
	ref := refs[m.refPickerSelect]
	return &ref
}

func (m *model) openRefPicker() {
	m.refPickerOpen = true
	m.refPickerQuery = ""
	m.refPickerStep = 0
	m.refPickerLeft = nil
	m.refPickerRight = nil
	m.refPickerSelect = 0
	m.paletteOpen = false

	if m.repo == nil {
		return
	}

	for index, ref := range m.refs {
		if ref.Name == m.repo.HeadRef {
			copy := ref
			m.refPickerLeft = &copy
			m.refPickerStep = 1
			m.refPickerSelect = index
			if len(m.refs) > 1 && index == 0 {
				m.refPickerSelect = 1
			}
			return
		}
	}
}

func (m *model) closeRefPicker() {
	m.refPickerOpen = false
	m.refPickerQuery = ""
	m.refPickerSelect = 0
	m.refPickerStep = 0
}

func (m *model) applySelectedRefPickerRef() tea.Cmd {
	selected := m.selectedRefValue()
	if selected == nil {
		return nil
	}

	if m.refPickerStep == 0 {
		copy := *selected
		m.refPickerLeft = &copy
		m.refPickerStep = 1
		m.refPickerQuery = ""
		m.refPickerSelect = 0
		return nil
	}

	copy := *selected
	m.refPickerRight = &copy
	m.refPickerOpen = false
	m.refPickerQuery = ""
	m.refPickerStep = 0
	m.refPickerSelect = 0
	if m.refPickerLeft == nil || m.refPickerRight == nil {
		return nil
	}

	m.customCompare = &domain.CompareSelection{
		LeftRef:    m.refPickerLeft.Name,
		RightRef:   m.refPickerRight.Name,
		LeftLabel:  m.refPickerLeft.Name,
		RightLabel: m.refPickerRight.Name,
		DiffStyle:  m.presetDiffStyle,
	}
	m.mode = domain.ModeCompareRefs
	m.actionMessage = fmt.Sprintf("Comparing %s...%s", m.refPickerLeft.Name, m.refPickerRight.Name)
	return m.refreshFiles()
}

func (m *model) commitBySHA(sha string) *domain.CommitSummary {
	for _, commit := range m.commits {
		if commit.SHA == sha {
			c := commit
			return &c
		}
	}
	return nil
}

func (m *model) currentFileCacheKey() string {
	if m.mode == domain.ModeConflict {
		return "conflict"
	}

	if compare := m.activeComparison(); compare != nil {
		return fmt.Sprintf("range:%s:%s:%s", compare.LeftRef, compare.DiffStyle, compare.RightRef)
	}

	commit := m.selectedCommitValue()
	if commit == nil {
		return ""
	}

	return "commit:" + commit.SHA
}

func (m *model) currentDiffCacheKey() string {
	if m.mode == domain.ModeConflict {
		file := m.selectedFileValue()
		if file == nil {
			return ""
		}
		return "conflict:" + file.Path
	}

	file := m.selectedFileValue()
	path := ""
	if file != nil {
		path = file.Path
	}

	if compare := m.activeComparison(); compare != nil {
		return fmt.Sprintf("range:%s:%s:%s:%s:%d", compare.LeftRef, compare.DiffStyle, compare.RightRef, path, m.contextLines)
	}

	commit := m.selectedCommitValue()
	if commit == nil {
		return ""
	}

	return fmt.Sprintf("commit:%s:%s:%d", commit.SHA, path, m.contextLines)
}

func (m *model) currentRenderCacheKey(width int) string {
	return fmt.Sprintf("%s:%s:%d", m.currentDiffCacheKey(), m.diffLayout, width)
}

func (m *model) renderDocument(width int) renderedDiff {
	cacheKey := m.currentRenderCacheKey(width)
	if cached, ok := m.renderCache[cacheKey]; ok {
		return cached
	}

	document := render.BuildInlineDocument(m.diff, width)
	if m.diffLayout == diffLayoutSplit {
		document = render.BuildSideBySideDocument(m.diff, width)
	}

	rendered := renderedDiff{
		rows:     document.Rows,
		hunkRows: document.HunkRows,
	}
	m.renderCache[cacheKey] = rendered
	return rendered
}

func (m *model) jumpToHunk(direction int, width int) {
	if m.diff == "" || width <= 0 {
		return
	}

	document := m.renderDocument(width)
	if len(document.hunkRows) == 0 {
		return
	}

	if direction > 0 {
		for _, row := range document.hunkRows {
			if row > m.diffScroll {
				m.diffScroll = row
				return
			}
		}
		m.diffScroll = document.hunkRows[len(document.hunkRows)-1]
		return
	}

	for index := len(document.hunkRows) - 1; index >= 0; index-- {
		if document.hunkRows[index] < m.diffScroll {
			m.diffScroll = document.hunkRows[index]
			return
		}
	}
	m.diffScroll = document.hunkRows[0]
}

func (m *model) currentSelectionLabel() string {
	if m.mode == domain.ModeConflict {
		return "Conflict Mode"
	}

	if compare := m.activeComparison(); compare != nil {
		sep := ".."
		if compare.DiffStyle == domain.DiffThreeDot {
			sep = "..."
		}
		return "Compare " + compare.LeftLabel + sep + compare.RightLabel
	}

	return "History selected commit"
}

func (m *model) refreshFiles() tea.Cmd {
	if m.repo == nil {
		return nil
	}

	m.filesErr = ""
	m.diffErr = ""
	m.diff = ""
	m.conflictContents = nil
	m.diffScroll = 0
	m.selectedFile = 0

	if m.mode == domain.ModeConflict {
		m.files = make([]domain.FileChange, 0, len(m.conflictFiles))
		for _, conflict := range m.conflictFiles {
			m.files = append(m.files, domain.FileChange{
				Path:   conflict.Path,
				Status: "U",
			})
		}
		if len(m.files) == 0 {
			m.filesErr = "No conflicted files remain."
			return nil
		}
		return m.refreshDiff()
	}

	cacheKey := m.currentFileCacheKey()
	if cacheKey != "" {
		if cached, ok := m.fileCache[cacheKey]; ok {
			m.files = cached
			if len(m.files) == 0 {
				m.filesErr = "No changed files for this selection."
				return nil
			}
			return tea.Batch(m.refreshDiff(), m.prefetchNeighborFiles(), m.prefetchNeighborDiffs())
		}
	}

	m.loadingFiles = true
	if compare := m.activeComparison(); compare != nil {
		return loadRangeFilesCmd(m.repo.RootPath, *compare)
	}

	commit := m.selectedCommitValue()
	if commit == nil {
		m.loadingFiles = false
		return nil
	}

	return tea.Batch(loadCommitFilesCmd(m.repo.RootPath, commit.SHA), m.prefetchNeighborFiles())
}

func (m *model) refreshDiff() tea.Cmd {
	if m.repo == nil {
		return nil
	}

	m.loadingDiff = true
	m.diffErr = ""
	m.diff = ""
	m.conflictContents = nil
	m.diffScroll = 0

	cacheKey := m.currentDiffCacheKey()
	if cacheKey != "" {
		if m.mode == domain.ModeConflict {
			if cached, ok := m.conflictCache[cacheKey]; ok {
				m.loadingDiff = false
				copy := cached
				m.conflictContents = &copy
				return nil
			}
		} else if cached, ok := m.diffCache[cacheKey]; ok {
			m.loadingDiff = false
			m.diff = cached
			return nil
		}
	}

	if m.mode == domain.ModeConflict {
		file := m.selectedFileValue()
		if file == nil {
			m.loadingDiff = false
			return nil
		}
		return loadConflictContentsCmd(m.repo.RootPath, file.Path)
	}

	file := m.selectedFileValue()
	path := ""
	if file != nil {
		path = file.Path
	}

	if compare := m.activeComparison(); compare != nil {
		return loadRangeDiffCmd(m.repo.RootPath, *compare, path, m.contextLines)
	}

	commit := m.selectedCommitValue()
	if commit == nil {
		m.loadingDiff = false
		return nil
	}

	return loadCommitDiffCmd(m.repo.RootPath, commit.SHA, path, m.contextLines)
}

func (m *model) prefetchNeighborFiles() tea.Cmd {
	if m.repo == nil || m.mode == domain.ModeConflict {
		return nil
	}

	cmds := []tea.Cmd{}
	for _, index := range []int{m.selectedCommit - 1, m.selectedCommit + 1} {
		if index < 0 || index >= len(m.commits) {
			continue
		}
		key := "commit:" + m.commits[index].SHA
		if _, ok := m.fileCache[key]; ok {
			continue
		}
		cmds = append(cmds, prefetchCommitFilesCmd(m.repo.RootPath, m.commits[index].SHA))
	}

	return tea.Batch(cmds...)
}

func (m *model) prefetchNeighborDiffs() tea.Cmd {
	if m.repo == nil || m.mode == domain.ModeConflict || len(m.files) == 0 {
		return nil
	}

	cmds := []tea.Cmd{}
	for _, index := range []int{m.selectedFile - 1, m.selectedFile + 1} {
		if index < 0 || index >= len(m.files) {
			continue
		}

		path := m.files[index].Path
		if path == "" {
			continue
		}

		if compare := m.activeComparison(); compare != nil {
			key := fmt.Sprintf("range:%s:%s:%s:%s:%d", compare.LeftRef, compare.DiffStyle, compare.RightRef, path, m.contextLines)
			if _, ok := m.diffCache[key]; ok {
				continue
			}
			cmds = append(cmds, prefetchRangeDiffCmd(m.repo.RootPath, *compare, path, m.contextLines))
			continue
		}

		commit := m.selectedCommitValue()
		if commit == nil {
			continue
		}
		key := fmt.Sprintf("commit:%s:%s:%d", commit.SHA, path, m.contextLines)
		if _, ok := m.diffCache[key]; ok {
			continue
		}
		cmds = append(cmds, prefetchCommitDiffCmd(m.repo.RootPath, commit.SHA, path, m.contextLines))
	}

	return tea.Batch(cmds...)
}

func (m *model) hardRefresh() tea.Cmd {
	m.loading = true
	m.fileCache = map[string][]domain.FileChange{}
	m.diffCache = map[string]string{}
	m.conflictCache = map[string]domain.ConflictFileContents{}
	m.renderCache = map[string]renderedDiff{}
	return loadRepositoryCmd(m.cwd)
}

func (m *model) openSelectedFileInEditor() tea.Cmd {
	if m.repo == nil {
		m.actionMessage = "No repository loaded."
		return nil
	}

	file := m.selectedFileValue()
	if file == nil {
		m.actionMessage = "No file selected."
		return nil
	}

	command, err := gitadapter.OpenFileInEditor(m.repo.RootPath, file.Path)
	if err != nil {
		m.actionMessage = err.Error()
		return nil
	}

	m.actionMessage = "Opened in " + command + "."
	return nil
}

func (m *model) toggleDiffLayout() {
	if m.diffLayout == diffLayoutInline {
		m.diffLayout = diffLayoutSplit
		return
	}
	m.diffLayout = diffLayoutInline
}

func (m *model) filteredPaletteCommands() []paletteCommand {
	commands := []paletteCommand{
		{id: "refresh", label: "Refresh repo", description: "Reload commits, files, conflicts, and caches"},
		{id: "focus-commits", label: "Focus commits", description: "Move focus to the commit graph pane"},
		{id: "focus-files", label: "Focus files", description: "Move focus to the changed files pane"},
		{id: "focus-diff", label: "Focus diff", description: "Move focus to the diff pane"},
		{id: "compare-refs", label: "Compare arbitrary refs", description: "Choose any two branches, tags, or refs to compare"},
		{id: "toggle-layout", label: "Toggle diff layout", description: "Switch between inline and side-by-side diff rendering"},
		{id: "context-up", label: "Increase context", description: "Show more unchanged lines around each hunk"},
		{id: "context-down", label: "Decrease context", description: "Show fewer unchanged lines around each hunk"},
	}

	if m.mode != domain.ModeConflict {
		commands = append(commands,
			paletteCommand{id: "history", label: "History mode", description: "Return to single-commit history browsing"},
			paletteCommand{id: "anchor-compare", label: "Toggle anchor compare", description: "Anchor the selected commit for commit-to-commit comparison"},
			paletteCommand{id: "toggle-style", label: "Toggle compare style", description: "Switch between two-dot and three-dot range comparisons"},
		)
		if m.repo != nil && m.repo.DefaultCompareBase != "" {
			commands = append(commands, paletteCommand{
				id:          "compare-preset",
				label:       "Compare base to HEAD",
				description: fmt.Sprintf("Compare %s against HEAD", m.repo.DefaultCompareBase),
			})
		}
	}

	if m.selectedFileValue() != nil {
		commands = append(commands, paletteCommand{
			id:          "open-editor",
			label:       "Open in editor",
			description: "Open the selected file in $VISUAL, $EDITOR, or VS Code",
		})
	}

	query := strings.ToLower(strings.TrimSpace(m.paletteQuery))
	if query == "" {
		return commands
	}

	filtered := make([]paletteCommand, 0, len(commands))
	for _, command := range commands {
		haystack := strings.ToLower(command.label + " " + command.description)
		if strings.Contains(haystack, query) {
			filtered = append(filtered, command)
		}
	}

	return filtered
}

func (m *model) executePaletteCommand(command paletteCommand) tea.Cmd {
	switch command.id {
	case "refresh":
		return m.hardRefresh()
	case "focus-commits":
		m.focus = focusCommits
		return nil
	case "focus-files":
		m.focus = focusFiles
		return nil
	case "focus-diff":
		m.focus = focusDiff
		return nil
	case "compare-refs":
		m.openRefPicker()
		return nil
	case "context-up":
		if m.contextLines < 20 {
			m.contextLines++
			return tea.Batch(m.refreshDiff(), m.prefetchNeighborDiffs())
		}
		return nil
	case "toggle-layout":
		m.toggleDiffLayout()
		return nil
	case "context-down":
		if m.contextLines > 0 {
			m.contextLines--
			return tea.Batch(m.refreshDiff(), m.prefetchNeighborDiffs())
		}
		return nil
	case "history":
		if m.mode != domain.ModeConflict {
			m.mode = domain.ModeHistory
			m.customCompare = nil
			return m.refreshFiles()
		}
		return nil
	case "compare-preset":
		if m.mode != domain.ModeConflict && m.repo != nil && m.repo.DefaultCompareBase != "" {
			m.customCompare = nil
			m.mode = domain.ModeComparePreset
			return m.refreshFiles()
		}
		return nil
	case "anchor-compare":
		if m.mode == domain.ModeConflict {
			return nil
		}
		commit := m.selectedCommitValue()
		if commit == nil {
			m.actionMessage = "No commit selected."
			return nil
		}
		if m.compareAnchor == commit.SHA {
			m.compareAnchor = ""
			m.mode = domain.ModeHistory
		} else {
			m.compareAnchor = commit.SHA
			m.mode = domain.ModeCompareCommits
		}
		return m.refreshFiles()
	case "toggle-style":
		if m.mode == domain.ModeCompareCommits {
			if m.commitDiffStyle == domain.DiffTwoDot {
				m.commitDiffStyle = domain.DiffThreeDot
			} else {
				m.commitDiffStyle = domain.DiffTwoDot
			}
			return m.refreshFiles()
		}
		if m.mode == domain.ModeCompareRefs && m.customCompare != nil {
			if m.customCompare.DiffStyle == domain.DiffTwoDot {
				m.customCompare.DiffStyle = domain.DiffThreeDot
			} else {
				m.customCompare.DiffStyle = domain.DiffTwoDot
			}
			return m.refreshFiles()
		}
		if m.presetDiffStyle == domain.DiffTwoDot {
			m.presetDiffStyle = domain.DiffThreeDot
		} else {
			m.presetDiffStyle = domain.DiffTwoDot
		}
		if m.mode == domain.ModeComparePreset {
			return m.refreshFiles()
		}
		return nil
	case "open-editor":
		return m.openSelectedFileInEditor()
	}

	return nil
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case repoLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.repositoryErr = msg.err.Error()
			return m, nil
		}

		repo := msg.repo
		m.repo = &repo
		m.commits = msg.commits
		m.refs = msg.refs
		m.conflictFiles = msg.conflicts
		if len(msg.conflicts) > 0 {
			m.mode = domain.ModeConflict
			m.focus = focusFiles
		} else {
			m.mode = domain.ModeHistory
		}
		return m, m.refreshFiles()
	case filesLoadedMsg:
		m.loadingFiles = false
		if msg.err != nil {
			m.filesErr = msg.err.Error()
			return m, nil
		}

		m.fileCache[msg.key] = msg.files
		if msg.key != m.currentFileCacheKey() {
			return m, nil
		}

		m.files = msg.files
		if len(m.files) == 0 {
			m.filesErr = "No changed files for this selection."
			return m, nil
		}

		m.selectedFile = 0
		return m, tea.Batch(m.refreshDiff(), m.prefetchNeighborDiffs())
	case prefetchedFilesMsg:
		if msg.key != "" && len(msg.files) >= 0 {
			m.fileCache[msg.key] = msg.files
		}
		return m, nil
	case prefetchedDiffMsg:
		if msg.key != "" {
			m.diffCache[msg.key] = msg.diff
		}
		return m, nil
	case diffLoadedMsg:
		m.loadingDiff = false
		if msg.err != nil {
			m.diffErr = msg.err.Error()
			return m, nil
		}

		if msg.conflict != nil {
			m.conflictCache[msg.key] = *msg.conflict
			if msg.key == m.currentDiffCacheKey() {
				copy := *msg.conflict
				m.conflictContents = &copy
			}
			return m, nil
		}

		m.diffCache[msg.key] = msg.diff
		if msg.key == m.currentDiffCacheKey() {
			m.diff = msg.diff
		}
		return m, nil
	case actionDoneMsg:
		if msg.err != nil {
			m.actionMessage = msg.err.Error()
			return m, nil
		}
		m.actionMessage = msg.message
		return m, m.hardRefresh()
	case tea.KeyMsg:
		if m.refPickerOpen {
			switch msg.String() {
			case "esc":
				m.closeRefPicker()
				return m, nil
			case "enter":
				return m, m.applySelectedRefPickerRef()
			case "backspace":
				runes := []rune(m.refPickerQuery)
				if len(runes) > 0 {
					m.refPickerQuery = string(runes[:len(runes)-1])
				}
				m.refPickerSelect = 0
				return m, nil
			case "up", "ctrl+p", "k":
				refs := m.filteredRefs()
				if len(refs) > 0 {
					m.refPickerSelect = clampInt(m.refPickerSelect-1, 0, len(refs)-1)
				}
				return m, nil
			case "down", "ctrl+n", "j":
				refs := m.filteredRefs()
				if len(refs) > 0 {
					m.refPickerSelect = clampInt(m.refPickerSelect+1, 0, len(refs)-1)
				}
				return m, nil
			default:
				if len(msg.Runes) > 0 && !msg.Alt {
					m.refPickerQuery += string(msg.Runes)
					m.refPickerSelect = 0
				}
				return m, nil
			}
		}

		if m.paletteOpen {
			switch msg.String() {
			case "esc":
				m.paletteOpen = false
				m.paletteQuery = ""
				m.paletteSelected = 0
				return m, nil
			case "enter":
				commands := m.filteredPaletteCommands()
				if len(commands) == 0 {
					m.paletteOpen = false
					m.paletteQuery = ""
					return m, nil
				}
				m.paletteSelected = clampInt(m.paletteSelected, 0, len(commands)-1)
				selected := commands[m.paletteSelected]
				m.paletteOpen = false
				m.paletteQuery = ""
				m.paletteSelected = 0
				return m, m.executePaletteCommand(selected)
			case "backspace":
				runes := []rune(m.paletteQuery)
				if len(runes) > 0 {
					m.paletteQuery = string(runes[:len(runes)-1])
				}
				m.paletteSelected = 0
				return m, nil
			case "up", "ctrl+p", "k":
				commands := m.filteredPaletteCommands()
				if len(commands) > 0 {
					m.paletteSelected = clampInt(m.paletteSelected-1, 0, len(commands)-1)
				}
				return m, nil
			case "down", "ctrl+n", "j":
				commands := m.filteredPaletteCommands()
				if len(commands) > 0 {
					m.paletteSelected = clampInt(m.paletteSelected+1, 0, len(commands)-1)
				}
				return m, nil
			default:
				if len(msg.Runes) > 0 && msg.Alt == false {
					m.paletteQuery += string(msg.Runes)
					m.paletteSelected = 0
				}
				return m, nil
			}
		}

		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			return m, tea.Quit
		case ":":
			m.paletteOpen = true
			m.paletteQuery = ""
			m.paletteSelected = 0
			return m, nil
		case "b":
			m.openRefPicker()
			return m, nil
		case "tab":
			m.focus = paneFocus((int(m.focus) + 1) % 3)
			return m, nil
		case "h":
			if m.focus > focusCommits {
				m.focus--
			}
			return m, nil
		case "l":
			if m.focus < focusDiff {
				m.focus++
			}
			return m, nil
		case "j", "down":
			switch m.focus {
			case focusCommits:
				if m.mode != domain.ModeConflict && m.selectedCommit < len(m.commits)-1 {
					m.selectedCommit++
					return m, m.refreshFiles()
				}
			case focusFiles:
				if m.selectedFile < len(m.files)-1 {
					m.selectedFile++
					return m, tea.Batch(m.refreshDiff(), m.prefetchNeighborDiffs())
				}
			case focusDiff:
				m.diffScroll++
			}
			return m, nil
		case "k", "up":
			switch m.focus {
			case focusCommits:
				if m.mode != domain.ModeConflict && m.selectedCommit > 0 {
					m.selectedCommit--
					return m, m.refreshFiles()
				}
			case focusFiles:
				if m.selectedFile > 0 {
					m.selectedFile--
					return m, tea.Batch(m.refreshDiff(), m.prefetchNeighborDiffs())
				}
			case focusDiff:
				if m.diffScroll > 0 {
					m.diffScroll--
				}
			}
			return m, nil
		case "]":
			m.jumpToHunk(1, m.currentDiffContentWidth())
			return m, nil
		case "[":
			m.jumpToHunk(-1, m.currentDiffContentWidth())
			return m, nil
		case "c":
			if m.mode != domain.ModeConflict && m.repo != nil && m.repo.DefaultCompareBase != "" {
				m.customCompare = nil
				if m.mode == domain.ModeComparePreset {
					m.mode = domain.ModeHistory
				} else {
					m.mode = domain.ModeComparePreset
				}
				return m, m.refreshFiles()
			}
			return m, nil
		case "i":
			m.toggleDiffLayout()
			return m, nil
		case "g":
			if m.mode != domain.ModeConflict {
				m.customCompare = nil
				m.mode = domain.ModeHistory
				return m, m.refreshFiles()
			}
			return m, nil
		case "v":
			if m.mode == domain.ModeConflict {
				return m, nil
			}

			commit := m.selectedCommitValue()
			if commit == nil {
				return m, nil
			}

			if m.compareAnchor == commit.SHA {
				m.compareAnchor = ""
				m.customCompare = nil
				m.mode = domain.ModeHistory
			} else {
				m.customCompare = nil
				m.compareAnchor = commit.SHA
				m.mode = domain.ModeCompareCommits
			}
			return m, m.refreshFiles()
		case "1":
			if m.mode == domain.ModeConflict && m.repo != nil {
				conflict := m.selectedConflictValue()
				if conflict != nil {
					m.actionMessage = "Applying ours..."
					return m, acceptConflictCmd(m.repo.RootPath, conflict.Path, "ours")
				}
			}
			return m, nil
		case "2":
			if m.mode == domain.ModeConflict && m.repo != nil {
				conflict := m.selectedConflictValue()
				if conflict != nil {
					m.actionMessage = "Applying theirs..."
					return m, acceptConflictCmd(m.repo.RootPath, conflict.Path, "theirs")
				}
			}
			return m, nil
		case "r":
			return m, m.hardRefresh()
		case "+":
			if m.contextLines < 20 {
				m.contextLines++
				return m, tea.Batch(m.refreshDiff(), m.prefetchNeighborDiffs())
			}
		case "-":
			if m.contextLines > 0 {
				m.contextLines--
				return m, tea.Batch(m.refreshDiff(), m.prefetchNeighborDiffs())
			}
		case "o":
			return m, m.openSelectedFileInEditor()
		}
	}

	return m, nil
}

func (m *model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	header := []string{
		styleTitle.Render("Better Diff (Go/Bubble Tea)"),
		styleMuted.Render("Repo: " + m.currentRepoLabel()),
		styleMode.Render("Mode: " + m.currentSelectionLabel()),
		styleMuted.Render(m.keyHelp()),
	}

	if m.actionMessage != "" {
		header = append(header, styleAccent.Render(m.actionMessage))
	}

	if m.repositoryErr != "" {
		header = append(header, styleError.Render(m.repositoryErr))
		return strings.Join(header, "\n")
	}

	contentHeight := m.height - len(header) - 2
	palette := ""
	if m.paletteOpen {
		paletteHeight := 9
		contentHeight -= paletteHeight
		palette = m.renderPalette(m.width-2, paletteHeight)
	}
	refPicker := ""
	if m.refPickerOpen {
		pickerHeight := 12
		contentHeight -= pickerHeight
		refPicker = m.renderRefPicker(m.width-2, pickerHeight)
	}
	if contentHeight < 12 {
		contentHeight = 12
	}

	leftWidth := clampInt(m.width/3, 28, 42)
	midWidth := clampInt(m.width/4, 24, 34)
	rightWidth := m.width - leftWidth - midWidth - 6
	if rightWidth < 40 {
		rightWidth = 40
	}

	panes := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.renderCommitsPane(leftWidth, contentHeight),
		m.renderFilesPane(midWidth, contentHeight),
		m.renderDiffPane(rightWidth, contentHeight),
	)

	parts := append([]string{}, header...)
	if palette != "" {
		parts = append(parts, palette)
	}
	if refPicker != "" {
		parts = append(parts, refPicker)
	}
	parts = append(parts, panes)
	return strings.Join(parts, "\n")
}

func (m *model) currentRepoLabel() string {
	if m.repo == nil {
		return m.cwd
	}
	return fmt.Sprintf("%s | Branch: %s", m.repo.RootPath, m.repo.HeadRef)
}

func (m *model) currentDiffContentWidth() int {
	leftWidth := clampInt(m.width/3, 28, 42)
	midWidth := clampInt(m.width/4, 24, 34)
	rightWidth := m.width - leftWidth - midWidth - 6
	if rightWidth < 40 {
		rightWidth = 40
	}
	return maxInt(8, rightWidth-4)
}

func (m *model) keyHelp() string {
	if m.mode == domain.ModeConflict {
		return fmt.Sprintf("Keys: h/j/k/l navigate, tab focus, : palette, b refs, i layout (%s), [ ] hunks, 1 ours, 2 theirs, r refresh, q quit", m.diffLayout)
	}
	return fmt.Sprintf("Keys: h/j/k/l navigate, tab focus, : palette, b refs, i layout (%s), [ ] hunks, c compare, v anchor compare, g history, +/- context %d, o editor, r refresh, q quit", m.diffLayout, m.contextLines)
}

func (m *model) renderCommitsPane(width, height int) string {
	lines := []string{}
	title := fmt.Sprintf("Commits (%d)", len(m.commits))
	if selected := m.selectedCommitValue(); selected != nil && len(selected.Refs) > 0 {
		title += " " + trimToWidth(renderInlineRefs(selected.Refs), maxInt(10, width-lipgloss.Width(title)-4))
	}

	if m.mode == domain.ModeConflict {
		lines = append(lines,
			styleError.Render(fmt.Sprintf("%d conflicted file(s)", len(m.conflictFiles))),
			styleMuted.Render(fmt.Sprintf("Merge: %t", m.repo != nil && m.repo.IsMergeInProgress)),
			styleMuted.Render(fmt.Sprintf("Rebase: %t", m.repo != nil && m.repo.IsRebaseInProgress)),
			styleMuted.Render(fmt.Sprintf("Cherry-pick: %t", m.repo != nil && m.repo.IsCherryPick)),
		)
	} else {
		start, end := visibleListRange(len(m.commits), m.selectedCommit, height-2)
		for i := start; i < end; i++ {
			commit := m.commits[i]
			lines = append(lines, m.renderCommitLine(commit, i == m.selectedCommit, width-4))
		}
	}

	if len(lines) == 0 {
		lines = append(lines, styleMuted.Render("No commits loaded."))
	}

	return paneStyle(width, height, m.focus == focusCommits).Render(title + "\n\n" + strings.Join(lines, "\n"))
}

func (m *model) renderPalette(width, height int) string {
	commands := m.filteredPaletteCommands()
	lines := []string{
		styleAccent.Render("Command Palette"),
		styleMuted.Render("Type to filter. Enter runs. Esc closes."),
		styleMuted.Render("Query: " + m.paletteQuery),
		"",
	}

	if len(commands) == 0 {
		lines = append(lines, styleMuted.Render("No matching commands."))
	} else {
		start, end := visibleListRange(len(commands), m.paletteSelected, height-len(lines)-2)
		for i := start; i < end; i++ {
			prefix := "  "
			if i == m.paletteSelected {
				prefix = "> "
			}
			lines = append(lines, trimToWidth(prefix+commands[i].label+"  "+commands[i].description, width-4))
		}
	}

	return lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("12")).
		Width(width).
		Height(height).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))
}

func (m *model) renderRefPicker(width, height int) string {
	title := "Compare Refs"
	step := "Pick left ref"
	if m.refPickerStep == 1 {
		step = "Pick right ref"
	}

	lines := []string{
		styleAccent.Render(title),
		styleMuted.Render(step + ". Type to filter. Enter selects. Esc closes."),
		styleMuted.Render("Left: " + m.refPickerLabel(m.refPickerLeft)),
		styleMuted.Render("Right: " + m.refPickerLabel(m.refPickerRight)),
		styleMuted.Render("Query: " + m.refPickerQuery),
		"",
	}

	refs := m.filteredRefs()
	if len(refs) == 0 {
		lines = append(lines, styleMuted.Render("No matching refs."))
	} else {
		start, end := visibleListRange(len(refs), m.refPickerSelect, height-len(lines)-2)
		for i := start; i < end; i++ {
			prefix := "  "
			if i == m.refPickerSelect {
				prefix = "> "
			}
			ref := refs[i]
			line := fmt.Sprintf("%s%-24s %-7s %s", prefix, trimToWidth(ref.Name, 24), ref.Type, ref.ShortSHA)
			lines = append(lines, trimToWidth(line, width-4))
		}
	}

	return lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("11")).
		Width(width).
		Height(height).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))
}

func (m *model) refPickerLabel(ref *domain.RefSummary) string {
	if ref == nil {
		return "(not set)"
	}
	return ref.Name
}

func (m *model) renderFilesPane(width, height int) string {
	lines := []string{}
	title := fmt.Sprintf("Files (%d)", len(m.files))
	if m.loadingFiles {
		lines = append(lines, styleAccent.Render("Loading files..."))
	}
	if m.filesErr != "" {
		lines = append(lines, styleMuted.Render(m.filesErr))
	}

	start, end := visibleListRange(len(m.files), m.selectedFile, height-2-len(lines))
	for i := start; i < end; i++ {
		file := m.files[i]
		prefix := "  "
		if i == m.selectedFile {
			prefix = "> "
		}
		statusColor := styleDefault
		if file.Status == "U" {
			statusColor = styleError
		}
		line := prefix + file.Status + " " + file.Path
		if file.OldPath != "" {
			line += " <- " + file.OldPath
		}
		lines = append(lines, statusColor.Render(trimToWidth(line, width-4)))
	}

	if len(lines) == 0 {
		lines = append(lines, styleMuted.Render("No files loaded."))
	}

	return paneStyle(width, height, m.focus == focusFiles).Render(title + "\n\n" + strings.Join(lines, "\n"))
}

func (m *model) renderDiffPane(width, height int) string {
	lines := []string{}
	title := "Diff [" + string(m.diffLayout) + "]"
	if m.loadingDiff {
		lines = append(lines, styleAccent.Render("Loading diff..."))
	}
	if m.diffErr != "" {
		lines = append(lines, styleMuted.Render(m.diffErr))
	}

	if m.mode == domain.ModeConflict {
		lines = append(lines, m.renderConflictContents(width-4)...)
	} else {
		lines = append(lines, m.renderDiffLines(width-4, height-3)...)
	}

	if file := m.selectedFileValue(); file != nil {
		title = "Diff [" + string(m.diffLayout) + "]: " + trimToWidth(file.Path, width-20)
	}

	return paneStyle(width, height, m.focus == focusDiff).Render(title + "\n\n" + strings.Join(lines, "\n"))
}

func (m *model) renderCommitLine(commit domain.CommitSummary, selected bool, width int) string {
	anchor := ""
	if m.compareAnchor == commit.SHA {
		anchor = "*"
	}

	lead := commit.Graph
	if anchor != "" {
		if lead != "" {
			lead = anchor + " " + lead
		} else {
			lead = anchor
		}
	}

	baseWidth := lipgloss.Width(lead)
	if lead != "" {
		baseWidth++
	}
	baseWidth += lipgloss.Width(commit.ShortSHA) + 1
	subjectWidth := width - baseWidth
	if subjectWidth < 8 {
		subjectWidth = 8
	}

	subject := trimToWidth(commit.Subject, subjectWidth)
	if selected {
		lineParts := []string{}
		if lead != "" {
			lineParts = append(lineParts, lead)
		}
		lineParts = append(lineParts, commit.ShortSHA, subject)
		line := strings.Join(lineParts, " ")
		return styleSelectedCommit.Width(width).Render(trimToWidth(line, width))
	}

	parts := []string{
		styleSHA.Render(commit.ShortSHA),
		" " + subject,
	}
	if commit.Graph != "" {
		parts = append([]string{styleGraph.Render(commit.Graph + " ")}, parts...)
	}
	if anchor != "" {
		parts = append([]string{styleMuted.Render(anchor + " ")}, parts...)
	}

	return lipgloss.JoinHorizontal(lipgloss.Left, parts...)
}

func (m *model) renderDiffLines(width, height int) []string {
	if m.diff == "" {
		return []string{styleMuted.Render("No diff loaded.")}
	}

	document := m.renderDocument(width)
	if m.diffScroll > len(document.rows)-1 {
		m.diffScroll = maxInt(0, len(document.rows)-1)
	}

	end := minInt(len(document.rows), m.diffScroll+height)
	return document.rows[m.diffScroll:end]
}

func (m *model) renderConflictContents(width int) []string {
	if m.conflictContents == nil {
		return []string{styleMuted.Render("No conflict content loaded.")}
	}

	sections := []struct {
		title string
		body  string
		style lipgloss.Style
	}{
		{title: "BASE", body: m.conflictContents.Base, style: styleSection},
		{title: "OURS", body: m.conflictContents.Ours, style: styleAdd},
		{title: "THEIRS", body: m.conflictContents.Theirs, style: styleDel},
		{title: "MERGED", body: m.conflictContents.Merged, style: styleAccent},
	}

	lines := []string{}
	for _, section := range sections {
		lines = append(lines, section.style.Render(section.title))
		body := section.body
		if body == "" {
			body = "(not available)"
		}
		for _, line := range strings.Split(body, "\n") {
			lines = append(lines, trimToWidth(line, width))
		}
		lines = append(lines, "")
	}

	if m.diffScroll > len(lines)-1 {
		m.diffScroll = maxInt(0, len(lines)-1)
	}

	end := minInt(len(lines), m.diffScroll+(m.height-8))
	return lines[m.diffScroll:end]
}

func paneStyle(width, height int, focused bool) lipgloss.Style {
	border := lipgloss.Color("8")
	if focused {
		border = lipgloss.Color("10")
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Width(width).
		Height(height).
		Padding(0, 1)
}

var (
	styleTitle          = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	styleMode           = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	styleMuted          = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleAccent         = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	styleSection        = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	styleAdd            = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	styleDel            = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	styleError          = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	styleDefault        = lipgloss.NewStyle()
	styleGraph          = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	styleSHA            = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	styleSelectedCommit = lipgloss.NewStyle().
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("25")).
				Bold(true)
)

func trimToWidth(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(value) <= width {
		return value
	}
	if width <= 3 {
		return value[:width]
	}
	runes := []rune(value)
	if len(runes) > width-3 {
		runes = runes[:width-3]
	}
	return string(runes) + "..."
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func visibleListRange(total, selected, height int) (int, int) {
	if total <= 0 {
		return 0, 0
	}

	if height < 1 {
		height = 1
	}

	selected = clampInt(selected, 0, total-1)
	start := selected - height/2
	if start < 0 {
		start = 0
	}

	end := start + height
	if end > total {
		end = total
		start = maxInt(0, end-height)
	}

	return start, end
}

func renderInlineRefs(refs []string) string {
	if len(refs) == 0 {
		return ""
	}

	parts := make([]string, 0, len(refs))
	for _, ref := range refs {
		parts = append(parts, ref)
	}

	return "[" + strings.Join(parts, ", ") + "]"
}
