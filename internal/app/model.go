package app

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"better-diff/internal/conflicts"
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

type diffViewMode string

const (
	diffViewPatch     diffViewMode = "patch"
	diffViewFullFile  diffViewMode = "full"
	helpOverlayHeight              = 14
)

type conflictSide string

const (
	conflictSideOurs   conflictSide = "ours"
	conflictSideTheirs conflictSide = "theirs"
)

type localCompareMode string

const (
	localCompareNone     localCompareMode = ""
	localCompareAll      localCompareMode = "all-local"
	localCompareStaged   localCompareMode = "staged"
	localCompareUnstaged localCompareMode = "unstaged"
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

type fullFileLoadedMsg struct {
	key     string
	compare *render.FullFileCompare
	err     error
}

type blameLoadedMsg struct {
	key   string
	lines map[int]domain.BlameLine
	err   error
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

type keyBinding struct {
	section string
	key     string
	label   string
}

type renderedDiff struct {
	rows     []string
	rowMeta  []render.RowMeta
	hunkRows []int
}

const (
	maxFileCacheEntries     = 128
	maxDiffCacheEntries     = 128
	maxFullFileCacheEntries = 24
	maxConflictCacheEntries = 24
	maxRenderCacheEntries   = 48
	maxBlameCacheEntries    = 48
	maxParsedDiffEntries    = 96
)

type fullFileSpec struct {
	leftRevision  string
	rightRevision string
	leftLabel     string
	rightLabel    string
	leftPath      string
	rightPath     string
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

	mode               domain.ExplorerMode
	focus              paneFocus
	selectedCommit     int
	selectedFile       int
	diffScroll         int
	diffCursor         int
	contextLines       int
	ignoreWhitespace   bool
	presetDiffStyle    domain.DiffStyle
	commitDiffStyle    domain.DiffStyle
	compareAnchor      string
	customCompare      *domain.CompareSelection
	paletteOpen        bool
	paletteQuery       string
	paletteSelected    int
	helpOpen           bool
	helpScroll         int
	conflictSideFocus  conflictSide
	diffLayout         diffLayout
	diffViewMode       diffViewMode
	diffFullScreen     bool
	refPickerOpen      bool
	refPickerQuery     string
	refPickerStep      int
	refPickerLeft      *domain.RefSummary
	refPickerRight     *domain.RefSummary
	refPickerSelect    int
	commitPickerOpen   bool
	commitPickerQuery  string
	commitPickerSelect int
	preferredFilePath  string
	showBlame          bool
	blameDetailOpen    bool
	conflictBaseOpen   bool

	diff             string
	diffLoaded       bool
	fullFileCompare  *render.FullFileCompare
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
	fullFileCache map[string]render.FullFileCompare
	conflictCache map[string]domain.ConflictFileContents
	renderCache   map[string]renderedDiff
	blameCache    map[string]map[int]domain.BlameLine
	parsedDiffs   map[string]render.Diff
	blameLoading  map[string]struct{}
}

func NewModel(cwd string) tea.Model {
	return &model{
		cwd:               cwd,
		mode:              domain.ModeHistory,
		focus:             focusFiles,
		contextLines:      3,
		ignoreWhitespace:  true,
		presetDiffStyle:   domain.DiffThreeDot,
		commitDiffStyle:   domain.DiffTwoDot,
		conflictSideFocus: conflictSideOurs,
		diffLayout:        diffLayoutInline,
		diffViewMode:      diffViewPatch,
		showBlame:         true,
		fileCache:         map[string][]domain.FileChange{},
		diffCache:         map[string]string{},
		fullFileCache:     map[string]render.FullFileCompare{},
		conflictCache:     map[string]domain.ConflictFileContents{},
		renderCache:       map[string]renderedDiff{},
		blameCache:        map[string]map[int]domain.BlameLine{},
		parsedDiffs:       map[string]render.Diff{},
		blameLoading:      map[string]struct{}{},
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
	return loadCommitFilesCmdWithOptions(root, sha, false)
}

func loadCommitFilesCmdWithOptions(root, sha string, ignoreWhitespace bool) tea.Cmd {
	key := "commit:" + sha
	if ignoreWhitespace {
		key += ":ws"
	}
	return func() tea.Msg {
		ctx, cancel := gitadapter.Context(5 * time.Second)
		defer cancel()

		files, err := gitadapter.ListCommitFilesWithOptions(ctx, root, sha, ignoreWhitespace)
		return filesLoadedMsg{key: key, files: files, err: err}
	}
}

func prefetchCommitFilesCmd(root, sha string) tea.Cmd {
	return prefetchCommitFilesCmdWithOptions(root, sha, false)
}

func prefetchCommitFilesCmdWithOptions(root, sha string, ignoreWhitespace bool) tea.Cmd {
	key := "commit:" + sha
	if ignoreWhitespace {
		key += ":ws"
	}
	return func() tea.Msg {
		ctx, cancel := gitadapter.Context(5 * time.Second)
		defer cancel()

		files, err := gitadapter.ListCommitFilesWithOptions(ctx, root, sha, ignoreWhitespace)
		if err != nil {
			return prefetchedFilesMsg{key: key}
		}
		return prefetchedFilesMsg{key: key, files: files}
	}
}

func loadRangeFilesCmd(root string, compare domain.CompareSelection) tea.Cmd {
	return loadRangeFilesCmdWithOptions(root, compare, false)
}

func loadRangeFilesCmdWithOptions(root string, compare domain.CompareSelection, ignoreWhitespace bool) tea.Cmd {
	key := fmt.Sprintf("range:%s:%s:%s", compare.LeftRef, compare.DiffStyle, compare.RightRef)
	if ignoreWhitespace {
		key += ":ws"
	}
	return func() tea.Msg {
		ctx, cancel := gitadapter.Context(5 * time.Second)
		defer cancel()

		files, err := gitadapter.ListRangeFilesWithOptions(ctx, root, compare.LeftRef, compare.RightRef, compare.DiffStyle, ignoreWhitespace)
		return filesLoadedMsg{key: key, files: files, err: err}
	}
}

func loadCommitDiffCmd(root, sha, path string, contextLines int) tea.Cmd {
	return loadCommitDiffCmdWithOptions(root, sha, path, contextLines, false)
}

func loadCommitDiffCmdWithOptions(root, sha, path string, contextLines int, ignoreWhitespace bool) tea.Cmd {
	key := fmt.Sprintf("commit:%s:%s:%d", sha, path, contextLines)
	if ignoreWhitespace {
		key += ":ws"
	}
	return func() tea.Msg {
		ctx, cancel := gitadapter.Context(8 * time.Second)
		defer cancel()

		diff, err := gitadapter.GetCommitDiffWithOptions(ctx, root, sha, path, contextLines, ignoreWhitespace)
		return diffLoadedMsg{key: key, diff: diff, err: err}
	}
}

func loadRangeDiffCmd(root string, compare domain.CompareSelection, path string, contextLines int) tea.Cmd {
	return loadRangeDiffCmdWithOptions(root, compare, path, contextLines, false)
}

func loadRangeDiffCmdWithOptions(root string, compare domain.CompareSelection, path string, contextLines int, ignoreWhitespace bool) tea.Cmd {
	key := fmt.Sprintf("range:%s:%s:%s:%s:%d", compare.LeftRef, compare.DiffStyle, compare.RightRef, path, contextLines)
	if ignoreWhitespace {
		key += ":ws"
	}
	return func() tea.Msg {
		ctx, cancel := gitadapter.Context(8 * time.Second)
		defer cancel()

		diff, err := gitadapter.GetRangeDiffWithOptions(ctx, root, compare.LeftRef, compare.RightRef, compare.DiffStyle, path, contextLines, ignoreWhitespace)
		return diffLoadedMsg{key: key, diff: diff, err: err}
	}
}

func loadFullFileCompareCmd(root, key, leftRevision, rightRevision, leftLabel, rightLabel, leftPath, rightPath string, ignoreWhitespace bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := gitadapter.Context(8 * time.Second)
		defer cancel()

		leftText, err := gitadapter.GetFileContent(ctx, root, leftRevision, leftPath)
		if err != nil {
			return fullFileLoadedMsg{key: key, err: err}
		}

		rightText, err := gitadapter.GetFileContent(ctx, root, rightRevision, rightPath)
		if err != nil {
			return fullFileLoadedMsg{key: key, err: err}
		}

		compare := render.FullFileCompare{
			LeftLabel:        leftLabel,
			RightLabel:       rightLabel,
			LeftPath:         leftPath,
			RightPath:        rightPath,
			LeftText:         leftText,
			RightText:        rightText,
			IgnoreWhitespace: ignoreWhitespace,
		}
		return fullFileLoadedMsg{key: key, compare: &compare}
	}
}

func prefetchCommitDiffCmd(root, sha, path string, contextLines int) tea.Cmd {
	return prefetchCommitDiffCmdWithOptions(root, sha, path, contextLines, false)
}

func prefetchCommitDiffCmdWithOptions(root, sha, path string, contextLines int, ignoreWhitespace bool) tea.Cmd {
	key := fmt.Sprintf("commit:%s:%s:%d", sha, path, contextLines)
	if ignoreWhitespace {
		key += ":ws"
	}
	return func() tea.Msg {
		ctx, cancel := gitadapter.Context(8 * time.Second)
		defer cancel()

		diff, err := gitadapter.GetCommitDiffWithOptions(ctx, root, sha, path, contextLines, ignoreWhitespace)
		if err != nil {
			return prefetchedDiffMsg{}
		}
		return prefetchedDiffMsg{key: key, diff: diff}
	}
}

func prefetchRangeDiffCmd(root string, compare domain.CompareSelection, path string, contextLines int) tea.Cmd {
	return prefetchRangeDiffCmdWithOptions(root, compare, path, contextLines, false)
}

func prefetchRangeDiffCmdWithOptions(root string, compare domain.CompareSelection, path string, contextLines int, ignoreWhitespace bool) tea.Cmd {
	key := fmt.Sprintf("range:%s:%s:%s:%s:%d", compare.LeftRef, compare.DiffStyle, compare.RightRef, path, contextLines)
	if ignoreWhitespace {
		key += ":ws"
	}
	return func() tea.Msg {
		ctx, cancel := gitadapter.Context(8 * time.Second)
		defer cancel()

		diff, err := gitadapter.GetRangeDiffWithOptions(ctx, root, compare.LeftRef, compare.RightRef, compare.DiffStyle, path, contextLines, ignoreWhitespace)
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

func loadBlameCmd(root, revision, path string) tea.Cmd {
	key := revision + ":" + path
	return func() tea.Msg {
		ctx, cancel := gitadapter.Context(8 * time.Second)
		defer cancel()

		lines, err := gitadapter.GetBlame(ctx, root, revision, path)
		return blameLoadedMsg{key: key, lines: lines, err: err}
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

func applyConflictBlockCmd(root, path string, blockIndex int, resolution string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := gitadapter.Context(5 * time.Second)
		defer cancel()

		result, err := gitadapter.ApplyConflictBlockResolution(ctx, root, path, blockIndex, resolution)
		if err != nil {
			return actionDoneMsg{err: err}
		}

		if result.Resolved {
			return actionDoneMsg{message: fmt.Sprintf("Applied %s to conflict %d in %s. All conflicts resolved; file staged.", resolution, blockIndex+1, path)}
		}

		return actionDoneMsg{
			message: fmt.Sprintf(
				"Applied %s to conflict %d in %s. Warning: %d conflict(s) remain; file is not staged yet.",
				resolution,
				blockIndex+1,
				path,
				result.RemainingBlocks,
			),
		}
	}
}

func revertHunkCmd(root, patch string, mode localCompareMode) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := gitadapter.Context(5 * time.Second)
		defer cancel()

		var err error
		switch mode {
		case localCompareStaged:
			err = gitadapter.ApplyReversePatchCached(ctx, root, patch)
		default:
			err = gitadapter.ApplyReversePatch(ctx, root, patch)
		}
		if err != nil {
			return actionDoneMsg{err: err}
		}

		switch mode {
		case localCompareStaged:
			return actionDoneMsg{message: "Reverted selected hunk from index."}
		default:
			return actionDoneMsg{message: "Reverted selected hunk from working tree."}
		}
	}
}

func revertFileCmd(root, revision, path string, mode localCompareMode) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := gitadapter.Context(5 * time.Second)
		defer cancel()

		var err error
		switch mode {
		case localCompareStaged:
			err = gitadapter.RestoreIndexFile(ctx, root, revision, path)
		default:
			err = gitadapter.RestoreWorktreeFile(ctx, root, revision, path)
		}
		if err != nil {
			return actionDoneMsg{err: err}
		}

		switch mode {
		case localCompareStaged:
			return actionDoneMsg{message: fmt.Sprintf("Reverted %s to %s in index.", path, revision)}
		default:
			return actionDoneMsg{message: fmt.Sprintf("Reverted %s to %s in working tree.", path, revision)}
		}
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

func (m *model) currentFullFileSpec() *fullFileSpec {
	file := m.selectedFileValue()
	if m.repo == nil || file == nil {
		return nil
	}

	leftPath := file.Path
	if file.OldPath != "" {
		leftPath = file.OldPath
	}

	if compare := m.activeComparison(); compare != nil {
		return &fullFileSpec{
			leftRevision:  compare.LeftRef,
			rightRevision: compare.RightRef,
			leftLabel:     compare.LeftLabel,
			rightLabel:    compare.RightLabel,
			leftPath:      leftPath,
			rightPath:     file.Path,
		}
	}

	commit := m.selectedCommitValue()
	if commit == nil {
		return nil
	}

	return &fullFileSpec{
		leftRevision:  commit.SHA + "^",
		rightRevision: commit.SHA,
		leftLabel:     commit.ShortSHA + "^",
		rightLabel:    commit.ShortSHA,
		leftPath:      leftPath,
		rightPath:     file.Path,
	}
}

func (m *model) currentConflictBlockIndex(width int) int {
	if m.mode != domain.ModeConflict {
		return -1
	}
	meta := m.currentDiffRowMeta(width)
	if meta == nil || !meta.Conflict {
		return -1
	}
	return meta.ConflictIndex
}

func (m *model) currentConflictBlock(width int) *conflicts.Block {
	if m.mode != domain.ModeConflict || m.conflictContents == nil {
		return nil
	}

	blockIndex := m.currentConflictBlockIndex(width)
	if blockIndex < 0 {
		return nil
	}

	parsed := conflicts.Parse(m.conflictContents.Merged)
	for _, segment := range parsed.Segments {
		if segment.Block != nil && segment.Block.Index == blockIndex {
			block := *segment.Block
			return &block
		}
	}
	return nil
}

func (m *model) setConflictSide(side conflictSide) {
	if side != conflictSideOurs && side != conflictSideTheirs {
		return
	}
	m.conflictSideFocus = side
}

func (m *model) currentConflictSide(width int) conflictSide {
	meta := m.currentDiffRowMeta(width)
	if meta == nil {
		return conflictSideOurs
	}

	switch {
	case meta.OldLine > 0 && meta.NewLine <= 0:
		return conflictSideOurs
	case meta.NewLine > 0 && meta.OldLine <= 0:
		return conflictSideTheirs
	case m.conflictSideFocus == conflictSideTheirs && meta.NewLine > 0:
		return conflictSideTheirs
	case meta.OldLine > 0:
		return conflictSideOurs
	default:
		return conflictSideTheirs
	}
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

func (m *model) currentLocalCompareMode() localCompareMode {
	compare := m.activeComparison()
	if compare == nil {
		return localCompareNone
	}

	switch {
	case !strings.HasPrefix(compare.LeftRef, "__") && compare.RightRef == gitadapter.WorkingTreeRef:
		return localCompareAll
	case !strings.HasPrefix(compare.LeftRef, "__") && compare.RightRef == gitadapter.IndexRef:
		return localCompareStaged
	case compare.LeftRef == gitadapter.IndexRef && compare.RightRef == gitadapter.WorkingTreeRef:
		return localCompareUnstaged
	default:
		return localCompareNone
	}
}

func (m *model) editableLocalComparison() *domain.CompareSelection {
	if m.mode == domain.ModeConflict || m.diffViewMode != diffViewPatch {
		return nil
	}
	compare := m.activeComparison()
	if compare == nil {
		return nil
	}
	if mode := m.currentLocalCompareMode(); mode != localCompareStaged && mode != localCompareUnstaged {
		return nil
	}
	file := m.selectedFileValue()
	if file == nil || file.Path == "" || file.OldPath != "" {
		return nil
	}
	return compare
}

func (m *model) startLocalCompare(mode localCompareMode) tea.Cmd {
	if m.repo == nil || len(m.commits) == 0 {
		return nil
	}

	if file := m.selectedFileValue(); file != nil {
		m.preferredFilePath = file.Path
	}
	m.selectedCommit = 0
	m.compareAnchor = ""
	m.mode = domain.ModeCompareRefs

	switch mode {
	case localCompareAll:
		m.customCompare = &domain.CompareSelection{
			LeftRef:    "HEAD",
			RightRef:   gitadapter.WorkingTreeRef,
			LeftLabel:  "HEAD",
			RightLabel: "Working Tree",
			DiffStyle:  domain.DiffTwoDot,
		}
		m.actionMessage = "Comparing HEAD to Working Tree."
	case localCompareStaged:
		m.customCompare = &domain.CompareSelection{
			LeftRef:    "HEAD",
			RightRef:   gitadapter.IndexRef,
			LeftLabel:  "HEAD",
			RightLabel: "Index",
			DiffStyle:  domain.DiffTwoDot,
		}
		m.actionMessage = "Comparing HEAD to Index (staged changes)."
	case localCompareUnstaged:
		m.customCompare = &domain.CompareSelection{
			LeftRef:    gitadapter.IndexRef,
			RightRef:   gitadapter.WorkingTreeRef,
			LeftLabel:  "Index",
			RightLabel: "Working Tree",
			DiffStyle:  domain.DiffTwoDot,
		}
		m.actionMessage = "Comparing Index to Working Tree (unstaged changes)."
	default:
		return nil
	}

	return m.refreshFiles()
}

func buildHunkPatch(diffText string, hunkIndex int) string {
	parsed := render.ParseUnifiedDiff(diffText)
	if len(parsed.Files) == 0 {
		return ""
	}

	file := parsed.Files[0]
	if hunkIndex < 0 || hunkIndex >= len(file.Hunks) {
		return ""
	}

	lines := append([]string{}, file.Headers...)
	lines = append(lines, file.Hunks[hunkIndex].Header)
	for _, line := range file.Hunks[hunkIndex].Lines {
		lines = append(lines, line.Raw)
	}
	return strings.Join(lines, "\n")
}

func (m *model) currentEditableHunkPatch(width int) (string, int, bool) {
	if m.editableLocalComparison() == nil || strings.TrimSpace(m.diff) == "" {
		return "", -1, false
	}

	document := m.renderDocument(m.diffRenderWidth(width))
	if len(document.hunkRows) == 0 {
		return "", -1, false
	}

	hunkIndex := -1
	for index, row := range document.hunkRows {
		if row <= m.diffCursor {
			hunkIndex = index
			continue
		}
		break
	}
	if hunkIndex < 0 {
		hunkIndex = 0
	}

	patch := buildHunkPatch(m.diff, hunkIndex)
	if strings.TrimSpace(patch) == "" {
		return "", -1, false
	}
	return patch, hunkIndex, true
}

func (m *model) availableRefs() []domain.RefSummary {
	refs := make([]domain.RefSummary, 0, len(m.refs)+2)
	refs = append(refs, domain.RefSummary{
		Name:     "Index",
		FullName: gitadapter.IndexRef,
		Type:     "workspace",
	})
	refs = append(refs, domain.RefSummary{
		Name:     "Working Tree",
		FullName: gitadapter.WorkingTreeRef,
		Type:     "workspace",
	})
	refs = append(refs, m.refs...)
	return refs
}

func (m *model) filteredRefs() []domain.RefSummary {
	query := strings.ToLower(strings.TrimSpace(m.refPickerQuery))
	refs := m.availableRefs()
	if m.refPickerStep == 0 {
		filtered := make([]domain.RefSummary, 0, len(refs))
		for _, ref := range refs {
			if ref.FullName == gitadapter.WorkingTreeRef {
				continue
			}
			filtered = append(filtered, ref)
		}
		refs = filtered
	}
	if query == "" {
		return refs
	}

	filtered := make([]domain.RefSummary, 0, len(refs))
	for _, ref := range refs {
		haystack := strings.ToLower(ref.Name + " " + ref.FullName + " " + ref.ShortSHA + " " + ref.Type)
		if strings.Contains(haystack, query) {
			filtered = append(filtered, ref)
		}
	}
	return filtered
}

func compareRefRevision(ref *domain.RefSummary) string {
	if ref == nil {
		return ""
	}
	if ref.FullName == gitadapter.WorkingTreeRef {
		return gitadapter.WorkingTreeRef
	}
	if ref.FullName == gitadapter.IndexRef {
		return gitadapter.IndexRef
	}
	return ref.Name
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

	refs := m.filteredRefs()
	for _, ref := range refs {
		if ref.Name == m.repo.HeadRef {
			copy := ref
			m.refPickerLeft = &copy
			m.refPickerStep = 1
			m.refPickerSelect = 0
			for candidateIndex, candidate := range m.filteredRefs() {
				if candidate.FullName == gitadapter.WorkingTreeRef {
					m.refPickerSelect = candidateIndex
					break
				}
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
		LeftRef:    compareRefRevision(m.refPickerLeft),
		RightRef:   compareRefRevision(m.refPickerRight),
		LeftLabel:  m.refPickerLeft.Name,
		RightLabel: m.refPickerRight.Name,
		DiffStyle:  m.presetDiffStyle,
	}
	m.mode = domain.ModeCompareRefs
	m.focus = focusDiff
	m.actionMessage = fmt.Sprintf("Comparing %s...%s", m.refPickerLeft.Name, m.refPickerRight.Name)
	return m.refreshFiles()
}

func (m *model) filteredCommitPickerCommits() []domain.CommitSummary {
	query := strings.ToLower(strings.TrimSpace(m.commitPickerQuery))
	if m.selectedCommit < 0 || m.selectedCommit >= len(m.commits) {
		return nil
	}

	filtered := make([]domain.CommitSummary, 0, len(m.commits)+2)
	workingTree := domain.CommitSummary{
		SHA:      gitadapter.WorkingTreeRef,
		ShortSHA: "WT",
		Subject:  "Working Tree (uncommitted changes)",
	}
	if query == "" || strings.Contains(strings.ToLower(workingTree.ShortSHA+" "+workingTree.Subject), query) {
		filtered = append(filtered, workingTree)
	}
	indexSnapshot := domain.CommitSummary{
		SHA:      gitadapter.IndexRef,
		ShortSHA: "IDX",
		Subject:  "Index (staged snapshot)",
	}
	if query == "" || strings.Contains(strings.ToLower(indexSnapshot.ShortSHA+" "+indexSnapshot.Subject), query) {
		filtered = append(filtered, indexSnapshot)
	}

	for _, commit := range m.commits[m.selectedCommit+1:] {
		if query != "" {
			haystack := strings.ToLower(commit.ShortSHA + " " + commit.AuthorName + " " + commit.Subject + " " + strings.Join(commit.Refs, " "))
			if !strings.Contains(haystack, query) {
				continue
			}
		}
		filtered = append(filtered, commit)
	}

	return filtered
}

func (m *model) selectedCommitPickerValue() *domain.CommitSummary {
	commits := m.filteredCommitPickerCommits()
	if m.commitPickerSelect < 0 || m.commitPickerSelect >= len(commits) {
		return nil
	}
	commit := commits[m.commitPickerSelect]
	return &commit
}

func (m *model) openCommitPicker() {
	if m.mode == domain.ModeConflict || m.selectedFileValue() == nil || m.selectedCommitValue() == nil {
		return
	}

	m.commitPickerOpen = true
	m.commitPickerQuery = ""
	m.commitPickerSelect = 0
	m.paletteOpen = false
	m.refPickerOpen = false
}

func (m *model) closeCommitPicker() {
	m.commitPickerOpen = false
	m.commitPickerQuery = ""
	m.commitPickerSelect = 0
}

func (m *model) exitTransientView() tea.Cmd {
	if m.diffFullScreen {
		m.diffFullScreen = false
		m.actionMessage = "Exited fullscreen diff."
		return nil
	}
	if m.mode == domain.ModeConflict {
		return nil
	}
	if m.mode == domain.ModeComparePreset || m.mode == domain.ModeCompareCommits || m.mode == domain.ModeCompareRefs {
		m.customCompare = nil
		m.compareAnchor = ""
		m.mode = domain.ModeHistory
		m.actionMessage = "Returned to history mode."
		return m.refreshFiles()
	}
	return nil
}

func (m *model) applySelectedCommitPicker() tea.Cmd {
	selected := m.selectedCommitPickerValue()
	file := m.selectedFileValue()
	current := m.selectedCommitValue()
	if selected == nil || file == nil || current == nil {
		return nil
	}

	m.commitPickerOpen = false
	m.commitPickerQuery = ""
	m.commitPickerSelect = 0
	m.preferredFilePath = file.Path
	m.focus = focusDiff
	if selected.SHA == gitadapter.IndexRef {
		m.compareAnchor = ""
		m.customCompare = &domain.CompareSelection{
			LeftRef:    current.SHA,
			RightRef:   gitadapter.IndexRef,
			LeftLabel:  current.ShortSHA,
			RightLabel: "Index",
			DiffStyle:  domain.DiffTwoDot,
		}
		m.mode = domain.ModeCompareRefs
		m.actionMessage = fmt.Sprintf("Comparing %s | %s -> Index", file.Path, formatCompareTargetLabel(current))
		return m.refreshFiles()
	}
	if selected.SHA == gitadapter.WorkingTreeRef {
		m.compareAnchor = ""
		m.customCompare = &domain.CompareSelection{
			LeftRef:    current.SHA,
			RightRef:   gitadapter.WorkingTreeRef,
			LeftLabel:  current.ShortSHA,
			RightLabel: "Working Tree",
			DiffStyle:  domain.DiffTwoDot,
		}
		m.mode = domain.ModeCompareRefs
		m.actionMessage = fmt.Sprintf("Comparing %s | %s -> Working Tree", file.Path, formatCompareTargetLabel(current))
		return m.refreshFiles()
	}

	m.compareAnchor = selected.SHA
	m.customCompare = nil
	m.mode = domain.ModeCompareCommits
	m.actionMessage = fmt.Sprintf("Comparing %s | %s -> %s", file.Path, formatCompareTargetLabel(selected), formatCompareTargetLabel(current))
	return m.refreshFiles()
}

func (m *model) toggleCommitCompare() tea.Cmd {
	if m.mode == domain.ModeConflict {
		return nil
	}

	commit := m.selectedCommitValue()
	if commit == nil {
		m.actionMessage = "No commit selected."
		return nil
	}

	m.customCompare = nil
	if m.compareAnchor == commit.SHA {
		m.compareAnchor = ""
		m.mode = domain.ModeHistory
		m.actionMessage = "Cleared compare anchor."
		return m.refreshFiles()
	}

	if m.compareAnchor == "" {
		m.compareAnchor = commit.SHA
		m.mode = domain.ModeCompareCommits
		m.actionMessage = fmt.Sprintf("Anchored %s for compare. Move to another commit and press Enter again.", commit.ShortSHA)
		return m.refreshFiles()
	}

	anchor := m.commitBySHA(m.compareAnchor)
	m.mode = domain.ModeCompareCommits
	if anchor != nil {
		m.actionMessage = fmt.Sprintf("Comparing %s to %s.", anchor.ShortSHA, commit.ShortSHA)
	}
	return m.refreshFiles()
}

func selectFileIndexByPath(files []domain.FileChange, path string) int {
	if path == "" {
		return 0
	}
	for index, file := range files {
		if file.Path == path {
			return index
		}
	}
	return 0
}

type blameTarget struct {
	revision string
	path     string
}

func blameCacheKey(revision, path string) string {
	return revision + ":" + path
}

func (m *model) leftBlameTarget() *blameTarget {
	file := m.selectedFileValue()
	if m.repo == nil || file == nil {
		return nil
	}

	path := file.Path
	if file.OldPath != "" {
		path = file.OldPath
	}

	if compare := m.activeComparison(); compare != nil {
		if compare.LeftRef == "" || path == "" {
			return nil
		}
		if compare.LeftRef == gitadapter.WorkingTreeRef {
			return nil
		}
		return &blameTarget{revision: compare.LeftRef, path: path}
	}

	commit := m.selectedCommitValue()
	if commit == nil || path == "" {
		return nil
	}
	return &blameTarget{revision: commit.SHA + "^", path: path}
}

func (m *model) rightBlameTarget() *blameTarget {
	file := m.selectedFileValue()
	if m.repo == nil || file == nil || file.Path == "" {
		return nil
	}

	if compare := m.activeComparison(); compare != nil {
		if compare.RightRef == "" {
			return nil
		}
		revision := compare.RightRef
		if revision == gitadapter.WorkingTreeRef {
			revision = ""
		}
		return &blameTarget{revision: revision, path: file.Path}
	}

	commit := m.selectedCommitValue()
	if commit == nil {
		return nil
	}
	return &blameTarget{revision: commit.SHA, path: file.Path}
}

func (m *model) currentBlameTargets() []blameTarget {
	targets := []blameTarget{}
	seen := map[string]struct{}{}
	for _, target := range []*blameTarget{m.leftBlameTarget(), m.rightBlameTarget()} {
		if target == nil {
			continue
		}
		key := blameCacheKey(target.revision, target.path)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		targets = append(targets, *target)
	}
	return targets
}

func (m *model) maybeLoadBlame() tea.Cmd {
	if !m.showBlame || m.repo == nil || m.mode == domain.ModeConflict {
		return nil
	}
	if m.focus != focusDiff && !m.blameDetailOpen {
		return nil
	}

	cmds := []tea.Cmd{}
	for _, target := range m.currentBlameTargets() {
		key := blameCacheKey(target.revision, target.path)
		if _, ok := m.blameCache[key]; ok {
			continue
		}
		if _, ok := m.blameLoading[key]; ok {
			continue
		}
		m.blameLoading[key] = struct{}{}
		cmds = append(cmds, loadBlameCmd(m.repo.RootPath, target.revision, target.path))
	}
	return tea.Batch(cmds...)
}

func (m *model) blameLineForMeta(meta render.RowMeta) *domain.BlameLine {
	if meta.Continuation {
		return nil
	}

	if meta.Kind == render.LineDelete && meta.OldLine > 0 {
		target := m.leftBlameTarget()
		if target == nil {
			return nil
		}
		if lines, ok := m.blameCache[blameCacheKey(target.revision, target.path)]; ok {
			if blame, ok := lines[meta.OldLine]; ok {
				copy := blame
				return &copy
			}
		}
		return nil
	}

	if meta.NewLine > 0 {
		target := m.rightBlameTarget()
		if target == nil {
			return nil
		}
		if lines, ok := m.blameCache[blameCacheKey(target.revision, target.path)]; ok {
			if blame, ok := lines[meta.NewLine]; ok {
				copy := blame
				return &copy
			}
		}
	}

	return nil
}

func (m *model) currentCursorBlame(width int) *domain.BlameLine {
	document := m.renderDocument(m.diffRenderWidth(width))
	if len(document.rows) == 0 {
		return nil
	}

	if m.diffCursor < 0 || m.diffCursor >= len(document.rowMeta) {
		return nil
	}
	return m.blameLineForMeta(document.rowMeta[m.diffCursor])
}

func (m *model) currentDiffRowMeta(width int) *render.RowMeta {
	document := m.renderDocument(m.diffRenderWidth(width))
	if len(document.rowMeta) == 0 || m.diffCursor < 0 || m.diffCursor >= len(document.rowMeta) {
		return nil
	}
	meta := document.rowMeta[m.diffCursor]
	return &meta
}

func diffLineLabel(meta render.RowMeta) string {
	switch meta.Kind {
	case render.LineAdd:
		return "add"
	case render.LineDelete:
		return "delete"
	case render.LineContext:
		return "context"
	case render.LineMeta:
		return "meta"
	default:
		if meta.Continuation {
			return "continued"
		}
		return "header"
	}
}

func formatLineNumber(value int) string {
	if value <= 0 {
		return "-"
	}
	return fmt.Sprintf("%d", value)
}

func (m *model) currentEditorLine(width int) int {
	meta := m.currentDiffRowMeta(width)
	if meta == nil {
		return 0
	}
	if m.mode == domain.ModeConflict {
		switch m.currentConflictSide(width) {
		case conflictSideOurs:
			return meta.OldLine
		case conflictSideTheirs:
			return meta.NewLine
		}
	}
	if meta.NewLine > 0 {
		return meta.NewLine
	}
	return meta.OldLine
}

func (m *model) currentDiffStatus(width int) string {
	if m.mode == domain.ModeConflict {
		meta := m.currentDiffRowMeta(width)
		if meta == nil {
			return ""
		}
		parts := []string{"merge view"}
		if meta.Conflict {
			parts = append(parts, fmt.Sprintf("conflict %d", meta.ConflictIndex+1))
		}
		if meta.Conflict {
			side := m.currentConflictSide(width)
			parts = append(parts, "target "+string(side))
			parts = append(parts, "ours "+formatLineNumber(meta.OldLine), "theirs "+formatLineNumber(meta.NewLine))
			if block := m.currentConflictBlock(width); block != nil && len(block.Base) > 0 {
				parts = append(parts, fmt.Sprintf("base %d line(s)", len(block.Base)))
			}
		} else {
			line := meta.NewLine
			if line <= 0 {
				line = meta.OldLine
			}
			parts = append(parts, "merged "+formatLineNumber(line))
		}
		status := strings.Join(parts, "  |  ")
		if width > 0 {
			status = trimToWidth(status, width)
		}
		return status
	}
	if !m.diffLoaded {
		return ""
	}

	meta := m.currentDiffRowMeta(width)
	if meta == nil {
		return ""
	}

	parts := []string{
		"line " + diffLineLabel(*meta),
		"old " + formatLineNumber(meta.OldLine),
		"new " + formatLineNumber(meta.NewLine),
	}

	if meta.Continuation {
		parts = append(parts, "wrapped")
	}

	if blame := m.blameLineForMeta(*meta); blame != nil {
		summary := strings.TrimSpace(blameSummary(blame))
		if summary != "" {
			parts = append(parts, summary)
		}
	}

	status := strings.Join(parts, "  |  ")
	if width > 0 {
		status = trimToWidth(status, width)
	}
	return status
}

func (m *model) currentConflictInlineSummary(width int) string {
	if m.mode != domain.ModeConflict {
		return ""
	}

	block := m.currentConflictBlock(width)
	if block == nil || len(block.Base) == 0 {
		return ""
	}

	summary := fmt.Sprintf("Base available for conflict %d: %d line(s). Press K for detail.", block.Index+1, len(block.Base))
	if width > 0 {
		summary = trimToWidth(summary, width)
	}
	return summary
}

func conflictBlockRenderedLineCount(block conflicts.Block) int {
	count := 2 + len(block.Ours) + len(block.Theirs)
	if len(block.Base) > 0 {
		count += 1 + len(block.Base)
	}
	return count
}

func (m *model) currentConflictResultLines(width, height int) []string {
	if m.mode != domain.ModeConflict || m.conflictContents == nil || width <= 0 || height <= 0 {
		return nil
	}

	block := m.currentConflictBlock(m.currentDiffContentWidth())
	if block == nil {
		return nil
	}

	lines := strings.Split(strings.ReplaceAll(m.conflictContents.Merged, "\r", ""), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return nil
	}

	startLine := maxInt(1, block.StartMergedLine-2)
	endLine := minInt(len(lines), block.StartMergedLine+conflictBlockRenderedLineCount(*block)+1)
	window := lines[startLine-1 : endLine]

	rendered := make([]string, 0, minInt(len(window), height))
	for index, line := range window {
		lineNumber := startLine + index
		label := fmt.Sprintf("%4d │ ", lineNumber)
		bodyWidth := maxInt(8, width-lipgloss.Width(label))
		text := trimToWidth(line, bodyWidth)
		row := styleMuted.Render(label) + text
		if strings.HasPrefix(line, "<<<<<<< ") || strings.HasPrefix(line, "||||||| ") || line == "=======" || strings.HasPrefix(line, ">>>>>>> ") {
			row = styleMuted.Render(trimToWidth(label+line, width))
		} else if lineNumber >= block.StartMergedLine && lineNumber < block.StartMergedLine+conflictBlockRenderedLineCount(*block) {
			row = styleSelectedDiffLine.Width(width).Render(trimToWidth(label+line, width))
		}
		rendered = append(rendered, row)
		if len(rendered) >= height {
			break
		}
	}

	return rendered
}

func (m *model) renderConflictResultPane(width, height int) string {
	lines := []string{
		styleAccent.Render("Merge Result"),
	}

	block := m.currentConflictBlock(m.currentDiffContentWidth())
	if block == nil {
		lines = append(lines, styleMuted.Render("Move the diff cursor onto a conflict block to inspect merged output."))
	} else {
		target := m.currentConflictSide(m.currentDiffContentWidth())
		lines = append(lines, styleMuted.Render(fmt.Sprintf("Conflict %d  |  target %s  |  merged file context", block.Index+1, target)))
		lines = append(lines, m.currentConflictResultLines(width-4, maxInt(1, height-4))...)
	}
	lines = append(lines, styleMuted.Render("The merged file updates here after block actions."))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("8")).
		Width(width).
		Height(height).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))
}

func (m *model) emptyDiffMessage() string {
	if m.loadingDiff {
		return "Loading diff..."
	}
	if !m.diffLoaded {
		return "No diff loaded."
	}
	if m.ignoreWhitespace {
		return "No visible changes for this file with whitespace ignored. Press w to show whitespace changes."
	}
	return "No visible changes for this file in the current selection."
}

func blameSummary(blame *domain.BlameLine) string {
	if blame == nil {
		return ""
	}

	parts := []string{}
	if blame.AuthorName != "" {
		parts = append(parts, blame.AuthorName)
	}
	if blame.ShortSHA != "" {
		parts = append(parts, blame.ShortSHA)
	}
	if blame.Summary != "" {
		parts = append(parts, blame.Summary)
	}
	return strings.Join(parts, "  ")
}

func renderBlameSeparator(summary string, width int) string {
	if width <= 0 || summary == "" {
		return ""
	}
	label := "  " + summary + " "
	if lipgloss.Width(label) > width {
		label = "  " + trimToWidth(summary, maxInt(1, width-2))
	}
	fillWidth := maxInt(0, width-lipgloss.Width(label))
	return styleMuted.Render(label + strings.Repeat("─", fillWidth))
}

func (m *model) blameSummaryBefore(document renderedDiff, start int) (string, bool) {
	for index := minInt(start-1, len(document.rowMeta)-1); index >= 0; index-- {
		if blame := m.blameLineForMeta(document.rowMeta[index]); blame != nil {
			summary := blameSummary(blame)
			if summary != "" {
				return summary, true
			}
		}
	}
	return "", false
}

func (m *model) renderedLineCount(document renderedDiff, start, end int) int {
	if start < 0 {
		start = 0
	}
	if end >= len(document.rows) {
		end = len(document.rows) - 1
	}
	if start > end || len(document.rows) == 0 {
		return 0
	}

	lastSummary, hasLastSummary := m.blameSummaryBefore(document, start)
	count := 0
	for index := start; index <= end; index++ {
		if index < len(document.rowMeta) {
			if blame := m.blameLineForMeta(document.rowMeta[index]); blame != nil {
				summary := blameSummary(blame)
				if summary != "" && (!hasLastSummary || summary != lastSummary) {
					count++
					lastSummary = summary
					hasLastSummary = true
				}
			}
		}
		count++
	}
	return count
}

func (m *model) diffViewportStart(document renderedDiff, height int) int {
	if len(document.rows) == 0 || height <= 0 {
		return 0
	}

	maxStart := maxInt(0, len(document.rows)-1)
	start := clampInt(m.diffCursor-height/2, 0, maxStart)
	if !m.showBlame {
		return clampInt(start, 0, maxInt(0, len(document.rows)-height))
	}

	for start < m.diffCursor && m.renderedLineCount(document, start, m.diffCursor) > height {
		start++
	}
	return clampInt(start, 0, maxStart)
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

	ws := ""
	if m.ignoreWhitespace {
		ws = ":ws"
	}

	if compare := m.activeComparison(); compare != nil {
		return fmt.Sprintf("range:%s:%s:%s%s", compare.LeftRef, compare.DiffStyle, compare.RightRef, ws)
	}

	commit := m.selectedCommitValue()
	if commit == nil {
		return ""
	}

	return "commit:" + commit.SHA + ws
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

	ws := ""
	if m.ignoreWhitespace {
		ws = ":ws"
	}

	if compare := m.activeComparison(); compare != nil {
		return fmt.Sprintf("range:%s:%s:%s:%s:%d%s", compare.LeftRef, compare.DiffStyle, compare.RightRef, path, m.contextLines, ws)
	}

	commit := m.selectedCommitValue()
	if commit == nil {
		return ""
	}

	return fmt.Sprintf("commit:%s:%s:%d%s", commit.SHA, path, m.contextLines, ws)
}

func (m *model) currentFullFileCacheKey() string {
	key := m.currentDiffCacheKey()
	if key == "" {
		return ""
	}
	return key + ":full"
}

func (m *model) currentRenderCacheKey(width int) string {
	baseKey := m.currentDiffCacheKey()
	if m.diffViewMode == diffViewFullFile {
		baseKey = m.currentFullFileCacheKey()
	}
	return fmt.Sprintf("%s:%s:%s:%d", baseKey, m.diffViewMode, m.diffLayout, width)
}

func clearMapWhenTooLarge[T any](items map[string]T, limit int) {
	if len(items) > limit {
		for key := range items {
			delete(items, key)
		}
	}
}

func (m *model) renderDocument(width int) renderedDiff {
	cacheKey := m.currentRenderCacheKey(width)
	if cached, ok := m.renderCache[cacheKey]; ok {
		return cached
	}

	document := render.Document{}
	switch {
	case m.mode == domain.ModeConflict && m.conflictContents != nil:
		document = render.BuildConflictDocument(m.conflictContents.Path, m.conflictContents.Merged, width)
	case m.diffViewMode == diffViewFullFile && m.fullFileCompare != nil:
		document = render.BuildFullFileDocument(*m.fullFileCompare, width)
	default:
		parsedKey := m.currentDiffCacheKey()
		parsed, ok := m.parsedDiffs[parsedKey]
		if !ok {
			parsed = render.ParseUnifiedDiff(m.diff)
			m.parsedDiffs[parsedKey] = parsed
			clearMapWhenTooLarge(m.parsedDiffs, maxParsedDiffEntries)
		}
		if m.diffLayout == diffLayoutSplit {
			document = render.BuildSideBySideDocumentFromParsed(parsed, width)
		} else {
			document = render.BuildInlineDocumentFromParsed(parsed, width)
		}
	}

	rendered := renderedDiff{
		rows:     document.Rows,
		rowMeta:  document.RowMeta,
		hunkRows: document.HunkRows,
	}
	m.renderCache[cacheKey] = rendered
	clearMapWhenTooLarge(m.renderCache, maxRenderCacheEntries)
	return rendered
}

func (m *model) currentDiffViewportHeight() int {
	headerLines := 4
	if m.actionMessage != "" {
		headerLines++
	}

	contentHeight := m.height - headerLines - 2
	if m.paletteOpen {
		contentHeight -= 9
	}
	if m.commitPickerOpen {
		contentHeight -= 12
	}
	if m.helpOpen {
		contentHeight -= helpOverlayHeight
	}
	if m.blameDetailOpen {
		contentHeight -= 7
	}
	if m.conflictBaseOpen {
		contentHeight -= 9
	}
	if m.refPickerOpen {
		contentHeight -= 12
	}
	if contentHeight < 12 {
		contentHeight = 12
	}
	return maxInt(1, contentHeight-3)
}

func (m *model) syncDiffCursor(width int) renderedDiff {
	document := m.renderDocument(width)
	rowCount := len(document.rows)
	if rowCount == 0 {
		m.diffCursor = 0
		m.diffScroll = 0
		return document
	}

	m.diffCursor = clampInt(m.diffCursor, 0, rowCount-1)
	viewportHeight := m.currentDiffViewportHeight()
	targetScroll := m.diffCursor - viewportHeight/2
	maxScroll := maxInt(0, rowCount-viewportHeight)
	m.diffScroll = clampInt(targetScroll, 0, maxScroll)
	return document
}

func (m *model) resetDiffCursorToContentStart(width int) {
	if width <= 0 {
		width = 80
	}

	document := m.renderDocument(width)
	if len(document.rows) == 0 {
		m.diffCursor = 0
		m.diffScroll = 0
		return
	}

	m.diffCursor = firstSelectableDiffRow(document)
	m.diffScroll = 0
	m.syncDiffCursor(width)
}

func (m *model) moveDiffCursor(delta int, width int) {
	document := m.renderDocument(width)
	if len(document.rows) == 0 {
		m.diffCursor = 0
		m.diffScroll = 0
		return
	}

	m.diffCursor = clampInt(m.diffCursor+delta, 0, len(document.rows)-1)
	m.syncDiffCursor(width)
}

func (m *model) jumpToHunk(direction int, width int) {
	if width <= 0 {
		return
	}
	if m.diffViewMode == diffViewFullFile {
		if m.fullFileCompare == nil {
			return
		}
	} else if m.diff == "" {
		return
	}

	document := m.renderDocument(width)
	if len(document.hunkRows) == 0 {
		return
	}

	if direction > 0 {
		for _, row := range document.hunkRows {
			if row > m.diffCursor {
				m.diffCursor = row
				m.syncDiffCursor(width)
				return
			}
		}
		m.diffCursor = document.hunkRows[len(document.hunkRows)-1]
		m.syncDiffCursor(width)
		return
	}

	for index := len(document.hunkRows) - 1; index >= 0; index-- {
		if document.hunkRows[index] < m.diffCursor {
			m.diffCursor = document.hunkRows[index]
			m.syncDiffCursor(width)
			return
		}
	}
	m.diffCursor = document.hunkRows[0]
	m.syncDiffCursor(width)
}

func (m *model) toggleDiffFullScreen() {
	m.diffFullScreen = !m.diffFullScreen
	if m.diffFullScreen {
		m.focus = focusDiff
	}
}

func (m *model) currentSelectionLabel() string {
	if m.mode == domain.ModeConflict {
		if m.ignoreWhitespace {
			return "Conflict Mode [ignore ws]"
		}
		return "Conflict Mode"
	}

	if compare := m.activeComparison(); compare != nil {
		sep := ".."
		if compare.DiffStyle == domain.DiffThreeDot {
			sep = "..."
		}
		label := "Compare " + compare.LeftLabel + sep + compare.RightLabel
		if m.ignoreWhitespace {
			label += " [ignore ws]"
		}
		return label
	}

	if m.ignoreWhitespace {
		return "History (selected commit) [ignore ws]"
	}
	return "History (selected commit)"
}

func (m *model) refreshFiles() tea.Cmd {
	if m.repo == nil {
		return nil
	}

	m.filesErr = ""
	m.diffErr = ""
	m.diff = ""
	m.diffLoaded = false
	m.conflictContents = nil
	m.diffScroll = 0
	m.diffCursor = 0
	m.conflictBaseOpen = false
	m.conflictSideFocus = conflictSideOurs
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
			m.selectedFile = selectFileIndexByPath(m.files, m.preferredFilePath)
			m.preferredFilePath = ""
			return tea.Batch(m.refreshDiff(), m.prefetchNeighborFiles(), m.prefetchNeighborDiffs())
		}
	}

	m.loadingFiles = true
	if compare := m.activeComparison(); compare != nil {
		return loadRangeFilesCmdWithOptions(m.repo.RootPath, *compare, m.ignoreWhitespace)
	}

	commit := m.selectedCommitValue()
	if commit == nil {
		m.loadingFiles = false
		return nil
	}

	return tea.Batch(loadCommitFilesCmdWithOptions(m.repo.RootPath, commit.SHA, m.ignoreWhitespace), m.prefetchNeighborFiles())
}

func (m *model) refreshDiff() tea.Cmd {
	if m.repo == nil {
		return nil
	}

	m.loadingDiff = true
	m.diffErr = ""
	m.diff = ""
	m.diffLoaded = false
	m.fullFileCompare = nil
	m.conflictContents = nil
	m.diffScroll = 0
	m.diffCursor = 0
	m.conflictBaseOpen = false
	if m.mode == domain.ModeConflict {
		m.conflictSideFocus = conflictSideOurs
	}

	cacheKey := m.currentDiffCacheKey()
	fullCacheKey := m.currentFullFileCacheKey()
	if cacheKey != "" {
		if m.mode == domain.ModeConflict {
			if cached, ok := m.conflictCache[cacheKey]; ok {
				m.loadingDiff = false
				copy := cached
				m.conflictContents = &copy
				m.resetDiffCursorToContentStart(m.currentDiffContentWidth())
				return nil
			}
		} else if m.diffViewMode == diffViewFullFile {
			if cached, ok := m.fullFileCache[fullCacheKey]; ok {
				m.loadingDiff = false
				copy := cached
				m.fullFileCompare = &copy
				m.diffLoaded = true
				m.resetDiffCursorToContentStart(m.currentDiffContentWidth())
				return m.maybeLoadBlame()
			}
		} else if cached, ok := m.diffCache[cacheKey]; ok {
			m.loadingDiff = false
			m.diff = cached
			m.diffLoaded = true
			m.resetDiffCursorToContentStart(m.currentDiffContentWidth())
			return m.maybeLoadBlame()
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

	if m.diffViewMode == diffViewFullFile {
		spec := m.currentFullFileSpec()
		if spec == nil {
			m.loadingDiff = false
			return nil
		}
		return tea.Batch(
			loadFullFileCompareCmd(
				m.repo.RootPath,
				fullCacheKey,
				spec.leftRevision,
				spec.rightRevision,
				spec.leftLabel,
				spec.rightLabel,
				spec.leftPath,
				spec.rightPath,
				m.ignoreWhitespace,
			),
			m.maybeLoadBlame(),
		)
	}

	if compare := m.activeComparison(); compare != nil {
		return tea.Batch(loadRangeDiffCmdWithOptions(m.repo.RootPath, *compare, path, m.contextLines, m.ignoreWhitespace), m.maybeLoadBlame())
	}

	commit := m.selectedCommitValue()
	if commit == nil {
		m.loadingDiff = false
		return nil
	}

	return tea.Batch(loadCommitDiffCmdWithOptions(m.repo.RootPath, commit.SHA, path, m.contextLines, m.ignoreWhitespace), m.maybeLoadBlame())
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
		if m.ignoreWhitespace {
			key += ":ws"
		}
		if _, ok := m.fileCache[key]; ok {
			continue
		}
		cmds = append(cmds, prefetchCommitFilesCmdWithOptions(m.repo.RootPath, m.commits[index].SHA, m.ignoreWhitespace))
	}

	return tea.Batch(cmds...)
}

func (m *model) prefetchNeighborDiffs() tea.Cmd {
	if m.repo == nil || m.mode == domain.ModeConflict || len(m.files) == 0 || len(m.files) > 80 || m.diffViewMode == diffViewFullFile {
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
			if m.ignoreWhitespace {
				key += ":ws"
			}
			if _, ok := m.diffCache[key]; ok {
				continue
			}
			cmds = append(cmds, prefetchRangeDiffCmdWithOptions(m.repo.RootPath, *compare, path, m.contextLines, m.ignoreWhitespace))
			continue
		}

		commit := m.selectedCommitValue()
		if commit == nil {
			continue
		}
		key := fmt.Sprintf("commit:%s:%s:%d", commit.SHA, path, m.contextLines)
		if m.ignoreWhitespace {
			key += ":ws"
		}
		if _, ok := m.diffCache[key]; ok {
			continue
		}
		cmds = append(cmds, prefetchCommitDiffCmdWithOptions(m.repo.RootPath, commit.SHA, path, m.contextLines, m.ignoreWhitespace))
	}

	return tea.Batch(cmds...)
}

func (m *model) hardRefresh() tea.Cmd {
	m.loading = true
	m.fileCache = map[string][]domain.FileChange{}
	m.diffCache = map[string]string{}
	m.fullFileCache = map[string]render.FullFileCompare{}
	m.conflictCache = map[string]domain.ConflictFileContents{}
	m.renderCache = map[string]renderedDiff{}
	m.blameCache = map[string]map[int]domain.BlameLine{}
	m.parsedDiffs = map[string]render.Diff{}
	m.blameLoading = map[string]struct{}{}
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

	line := m.currentEditorLine(m.currentDiffContentWidth())
	command, err := gitadapter.OpenFileInEditor(m.repo.RootPath, file.Path, line)
	if err != nil {
		m.actionMessage = err.Error()
		return nil
	}

	if line > 0 {
		m.actionMessage = fmt.Sprintf("Opened in %s at line %d.", command, line)
	} else {
		m.actionMessage = "Opened in " + command + "."
	}
	return nil
}

func (m *model) toggleDiffLayout() {
	if m.diffLayout == diffLayoutInline {
		m.diffLayout = diffLayoutSplit
		return
	}
	m.diffLayout = diffLayoutInline
}

func (m *model) toggleDiffViewMode() {
	if m.diffViewMode == diffViewPatch {
		m.diffViewMode = diffViewFullFile
		return
	}
	m.diffViewMode = diffViewPatch
}

func (m *model) diffViewLabel() string {
	if m.diffViewMode == diffViewFullFile {
		return "full-file"
	}
	return string(m.diffLayout)
}

func (m *model) currentActionHints() []string {
	hints := []string{"[tab] cycle", "[h/l] left diff", "[j/k] move", "[enter] act", "[:] menu", "[?] keys"}
	if m.mode == domain.ModeConflict {
		hints = append(hints, "[[]/[]] conflicts", "[H/L] side", "[enter] apply", "[1/2/3] resolve", "[K] base", "[F] fullscreen")
		return hints
	}

	hints = append(hints, "[[]/[]] hunks", "[A/S/W] local", "[b] refs", "[f] view", "[F] fullscreen")
	if m.editableLocalComparison() != nil {
		hints = append(hints, "[u/U] revert")
	}
	return hints
}

func (m *model) renderActionBar(width int) string {
	if width <= 0 {
		return ""
	}
	text := strings.Join(m.currentActionHints(), "  ")
	return styleHeaderBar.Width(width).Render(trimToWidth(text, width))
}

func (m *model) currentKeyBindings() []keyBinding {
	bindings := []keyBinding{
		{section: "Global", key: "tab", label: "cycle files, commits, and diff focus"},
		{section: "Global", key: "h l", label: "switch between the left stack and diff"},
		{section: "Global", key: "j k", label: "move inside the focused pane or overlay"},
		{section: "Global", key: ":", label: "open the action menu"},
		{section: "Global", key: "?", label: "open keyboard help"},
		{section: "Global", key: "esc", label: "close overlays, exit fullscreen, or leave compare mode"},
		{section: "Global", key: "r", label: "refresh repository state"},
		{section: "Global", key: "q", label: "quit"},
	}

	if m.mode == domain.ModeConflict {
		bindings = append(bindings,
			keyBinding{section: "Conflict", key: "[ ]", label: "jump between conflict blocks"},
			keyBinding{section: "Conflict", key: "H L", label: "target ours vs theirs for status and editor jumps"},
			keyBinding{section: "Conflict", key: "enter", label: "apply the currently targeted side to the selected block"},
			keyBinding{section: "Conflict", key: "result pane", label: "shows live merged-file context for the selected block"},
			keyBinding{section: "Conflict", key: "K", label: "show base detail for the selected conflict"},
			keyBinding{section: "Conflict", key: "1", label: "accept ours for the selected block"},
			keyBinding{section: "Conflict", key: "2", label: "accept theirs for the selected block"},
			keyBinding{section: "Conflict", key: "3", label: "accept both sides for the selected block"},
			keyBinding{section: "Conflict", key: "O", label: "accept the whole file as ours"},
			keyBinding{section: "Conflict", key: "T", label: "accept the whole file as theirs"},
			keyBinding{section: "Conflict", key: "o", label: "open the selected file at the current line in the editor"},
			keyBinding{section: "View", key: "F", label: "toggle diff fullscreen"},
		)
		return bindings
	}

	bindings = append(bindings,
		keyBinding{section: "Review", key: "enter", label: "compare the selected file against Working Tree or another commit"},
		keyBinding{section: "Compare", key: "enter (commits)", label: "anchor or compare the selected commit from the graph"},
		keyBinding{section: "Review", key: "[ ]", label: "jump between hunks or change blocks"},
		keyBinding{section: "Review", key: "B", label: "toggle inline blame"},
		keyBinding{section: "Review", key: "K", label: "show blame detail for the selected line"},
		keyBinding{section: "Review", key: "w", label: "toggle whitespace-ignore"},
		keyBinding{section: "Review", key: "+ -", label: "change hunk context"},
		keyBinding{section: "Review", key: "o", label: "open the selected file at the current line in the editor"},
		keyBinding{section: "Compare", key: "b", label: "compare arbitrary refs"},
		keyBinding{section: "Compare", key: "A", label: "compare HEAD to Working Tree (all local changes)"},
		keyBinding{section: "Compare", key: "S", label: "compare HEAD to Index (staged changes)"},
		keyBinding{section: "Compare", key: "W", label: "compare Index to Working Tree (unstaged changes)"},
		keyBinding{section: "Compare", key: "c", label: "toggle base vs HEAD compare"},
		keyBinding{section: "Compare", key: "v", label: "anchor the selected commit for compare"},
		keyBinding{section: "Compare", key: "g", label: "return to history mode"},
		keyBinding{section: "View", key: "i", label: "toggle inline vs side-by-side patch view"},
		keyBinding{section: "View", key: "f", label: "toggle patch vs full-file view"},
		keyBinding{section: "View", key: "F", label: "toggle diff fullscreen"},
	)

	if m.editableLocalComparison() != nil {
		bindings = append(bindings,
			keyBinding{section: "Working Tree", key: "u", label: "revert the selected hunk"},
			keyBinding{section: "Working Tree", key: "U", label: "revert the selected file"},
		)
	}

	return bindings
}

func (m *model) helpLineCount() int {
	count := 3
	currentSection := ""
	for _, binding := range m.currentKeyBindings() {
		if binding.section != currentSection {
			currentSection = binding.section
			count++
		}
		count++
	}
	return count
}

func (m *model) maxHelpScroll(height int) int {
	visibleHeight := maxInt(1, height-2)
	return maxInt(0, m.helpLineCount()-visibleHeight)
}

func (m *model) filteredPaletteCommands() []paletteCommand {
	commands := []paletteCommand{
		{id: "show-help", label: "Show keyboard help", description: "Open the full keyboard reference overlay"},
		{id: "refresh", label: "Refresh repo", description: "Reload commits, files, conflicts, and caches"},
		{id: "focus-commits", label: "Focus commits", description: "Move focus to the commit graph pane"},
		{id: "focus-files", label: "Focus files", description: "Move focus to the changed files pane"},
		{id: "focus-diff", label: "Focus diff", description: "Move focus to the diff pane"},
		{id: "compare-all-local", label: "Compare HEAD to Working Tree", description: "Review all local changes including staged, unstaged, and untracked"},
		{id: "compare-staged", label: "Compare HEAD to Index", description: "Review only staged changes as they would be committed now"},
		{id: "compare-unstaged", label: "Compare Index to Working Tree", description: "Review only unstaged and untracked local changes"},
		{id: "compare-refs", label: "Compare arbitrary refs", description: "Choose any two branches, tags, or refs to compare"},
		{id: "toggle-whitespace", label: "Toggle ignore whitespace", description: "Hide or show whitespace-only diff noise"},
		{id: "toggle-layout", label: "Toggle diff layout", description: "Switch between inline and side-by-side diff rendering"},
		{id: "toggle-view-mode", label: "Toggle patch/full-file", description: "Switch between patch review and full-file compare"},
		{id: "toggle-fullscreen", label: "Toggle diff fullscreen", description: "Show only the diff pane and keep the header visible"},
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
			id:          "compare-file-commit",
			label:       "Compare selected file to commit",
			description: "Pick another commit and keep the current file selected for the diff",
		}, paletteCommand{
			id:          "open-editor",
			label:       "Open in editor",
			description: "Open the selected file in $VISUAL, $EDITOR, or VS Code",
		})
	}
	if m.editableLocalComparison() != nil {
		commands = append(commands,
			paletteCommand{
				id:          "revert-hunk",
				label:       "Revert selected hunk",
				description: "Apply the left side of the current patch hunk back to the working tree",
			},
			paletteCommand{
				id:          "revert-file",
				label:       "Revert selected file",
				description: "Restore the selected file in the working tree from the left side of the comparison",
			},
		)
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
	case "show-help":
		m.helpOpen = true
		m.helpScroll = 0
		return nil
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
	case "compare-all-local":
		return m.startLocalCompare(localCompareAll)
	case "compare-staged":
		return m.startLocalCompare(localCompareStaged)
	case "compare-unstaged":
		return m.startLocalCompare(localCompareUnstaged)
	case "compare-refs":
		m.openRefPicker()
		return nil
	case "toggle-whitespace":
		m.ignoreWhitespace = !m.ignoreWhitespace
		if m.ignoreWhitespace {
			m.actionMessage = "Ignoring whitespace changes."
		} else {
			m.actionMessage = "Showing whitespace changes."
		}
		return m.refreshFiles()
	case "compare-file-commit":
		m.openCommitPicker()
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
	case "toggle-view-mode":
		m.toggleDiffViewMode()
		return m.refreshDiff()
	case "toggle-fullscreen":
		m.toggleDiffFullScreen()
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
	case "revert-hunk":
		if m.repo == nil {
			m.actionMessage = "No repository loaded."
			return nil
		}
		patch, hunkIndex, ok := m.currentEditableHunkPatch(m.currentDiffContentWidth())
		if !ok {
			m.actionMessage = "No editable hunk is selected."
			return nil
		}
		m.actionMessage = fmt.Sprintf("Reverting hunk %d from working tree...", hunkIndex+1)
		return revertHunkCmd(m.repo.RootPath, patch, m.currentLocalCompareMode())
	case "revert-file":
		if m.repo == nil {
			m.actionMessage = "No repository loaded."
			return nil
		}
		compare := m.editableLocalComparison()
		file := m.selectedFileValue()
		if compare == nil || file == nil {
			m.actionMessage = "Selected file is not editable in this view."
			return nil
		}
		m.actionMessage = fmt.Sprintf("Reverting %s from working tree...", file.Path)
		return revertFileCmd(m.repo.RootPath, compare.LeftRef, file.Path, m.currentLocalCompareMode())
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
		clearMapWhenTooLarge(m.fileCache, maxFileCacheEntries)
		if msg.key != m.currentFileCacheKey() {
			return m, nil
		}

		m.files = msg.files
		if len(m.files) == 0 {
			m.filesErr = "No changed files for this selection."
			return m, nil
		}

		m.selectedFile = selectFileIndexByPath(m.files, m.preferredFilePath)
		m.preferredFilePath = ""
		return m, tea.Batch(m.refreshDiff(), m.prefetchNeighborDiffs())
	case prefetchedFilesMsg:
		if msg.key != "" && len(msg.files) >= 0 {
			m.fileCache[msg.key] = msg.files
			clearMapWhenTooLarge(m.fileCache, maxFileCacheEntries)
		}
		return m, nil
	case prefetchedDiffMsg:
		if msg.key != "" {
			m.diffCache[msg.key] = msg.diff
			clearMapWhenTooLarge(m.diffCache, maxDiffCacheEntries)
		}
		return m, nil
	case blameLoadedMsg:
		if msg.key != "" {
			delete(m.blameLoading, msg.key)
		}
		if msg.err == nil && msg.key != "" {
			m.blameCache[msg.key] = msg.lines
			clearMapWhenTooLarge(m.blameCache, maxBlameCacheEntries)
		}
		return m, nil
	case fullFileLoadedMsg:
		m.loadingDiff = false
		if msg.err != nil {
			m.diffErr = msg.err.Error()
			return m, nil
		}
		if msg.compare == nil {
			return m, nil
		}
		m.fullFileCache[msg.key] = *msg.compare
		clearMapWhenTooLarge(m.fullFileCache, maxFullFileCacheEntries)
		if msg.key == m.currentFullFileCacheKey() {
			copy := *msg.compare
			m.fullFileCompare = &copy
			m.diffLoaded = true
			m.resetDiffCursorToContentStart(m.currentDiffContentWidth())
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
			clearMapWhenTooLarge(m.conflictCache, maxConflictCacheEntries)
			if msg.key == m.currentDiffCacheKey() {
				copy := *msg.conflict
				m.conflictContents = &copy
				m.resetDiffCursorToContentStart(m.currentDiffContentWidth())
			}
			return m, nil
		}

		m.diffCache[msg.key] = msg.diff
		clearMapWhenTooLarge(m.diffCache, maxDiffCacheEntries)
		if msg.key != "" && m.mode != domain.ModeConflict {
			m.parsedDiffs[msg.key] = render.ParseUnifiedDiff(msg.diff)
			clearMapWhenTooLarge(m.parsedDiffs, maxParsedDiffEntries)
		}
		if msg.key == m.currentDiffCacheKey() {
			m.diff = msg.diff
			m.diffLoaded = true
			m.resetDiffCursorToContentStart(m.currentDiffContentWidth())
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
		if m.blameDetailOpen && msg.String() == "esc" {
			m.blameDetailOpen = false
			return m, nil
		}
		if m.helpOpen {
			switch msg.String() {
			case "esc", "?":
				m.helpOpen = false
				m.helpScroll = 0
				return m, nil
			case "up", "ctrl+p", "k":
				m.helpScroll = maxInt(0, m.helpScroll-1)
				return m, nil
			case "down", "ctrl+n", "j":
				m.helpScroll = minInt(m.maxHelpScroll(helpOverlayHeight), m.helpScroll+1)
				return m, nil
			default:
				return m, nil
			}
		}
		if m.conflictBaseOpen && msg.String() == "esc" {
			m.conflictBaseOpen = false
			return m, nil
		}

		if m.commitPickerOpen {
			switch msg.String() {
			case "esc":
				m.closeCommitPicker()
				return m, nil
			case "enter":
				return m, m.applySelectedCommitPicker()
			case "backspace":
				runes := []rune(m.commitPickerQuery)
				if len(runes) > 0 {
					m.commitPickerQuery = string(runes[:len(runes)-1])
				}
				m.commitPickerSelect = 0
				return m, nil
			case "up", "ctrl+p", "k":
				commits := m.filteredCommitPickerCommits()
				if len(commits) > 0 {
					m.commitPickerSelect = clampInt(m.commitPickerSelect-1, 0, len(commits)-1)
				}
				return m, nil
			case "down", "ctrl+n", "j":
				commits := m.filteredCommitPickerCommits()
				if len(commits) > 0 {
					m.commitPickerSelect = clampInt(m.commitPickerSelect+1, 0, len(commits)-1)
				}
				return m, nil
			default:
				if len(msg.Runes) > 0 && !msg.Alt {
					m.commitPickerQuery += string(msg.Runes)
					m.commitPickerSelect = 0
				}
				return m, nil
			}
		}

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
		case "?":
			m.helpOpen = true
			m.helpScroll = 0
			m.paletteOpen = false
			m.paletteQuery = ""
			m.paletteSelected = 0
			return m, nil
		case ":":
			m.paletteOpen = true
			m.paletteQuery = ""
			m.paletteSelected = 0
			m.helpOpen = false
			m.helpScroll = 0
			return m, nil
		case "esc":
			if cmd := m.exitTransientView(); cmd != nil {
				return m, cmd
			}
			return m, nil
		case "b":
			m.openRefPicker()
			return m, nil
		case "A":
			if m.mode == domain.ModeConflict {
				return m, nil
			}
			return m, m.startLocalCompare(localCompareAll)
		case "S":
			if m.mode == domain.ModeConflict {
				return m, nil
			}
			return m, m.startLocalCompare(localCompareStaged)
		case "W":
			if m.mode == domain.ModeConflict {
				return m, nil
			}
			return m, m.startLocalCompare(localCompareUnstaged)
		case "w":
			m.ignoreWhitespace = !m.ignoreWhitespace
			if m.ignoreWhitespace {
				m.actionMessage = "Ignoring whitespace changes."
			} else {
				m.actionMessage = "Showing whitespace changes."
			}
			return m, m.refreshFiles()
		case "B":
			m.showBlame = !m.showBlame
			m.blameDetailOpen = false
			if m.showBlame {
				m.actionMessage = "Inline blame on."
				return m, m.maybeLoadBlame()
			}
			m.actionMessage = "Inline blame off."
			return m, nil
		case "K":
			if m.mode == domain.ModeConflict {
				m.conflictBaseOpen = !m.conflictBaseOpen
				return m, nil
			}
			if !m.showBlame {
				m.showBlame = true
				m.blameDetailOpen = true
				return m, m.maybeLoadBlame()
			}
			m.blameDetailOpen = !m.blameDetailOpen
			if m.blameDetailOpen {
				return m, m.maybeLoadBlame()
			}
			return m, nil
		case "enter":
			if m.mode == domain.ModeConflict && m.focus == focusDiff && m.repo != nil {
				conflict := m.selectedConflictValue()
				if conflict != nil {
					if blockIndex := m.currentConflictBlockIndex(m.currentDiffContentWidth()); blockIndex >= 0 {
						side := m.currentConflictSide(m.currentDiffContentWidth())
						m.actionMessage = fmt.Sprintf("Applying %s to conflict %d...", side, blockIndex+1)
						return m, applyConflictBlockCmd(m.repo.RootPath, conflict.Path, blockIndex, string(side))
					}
				}
			}
			if m.focus == focusCommits {
				return m, m.toggleCommitCompare()
			}
			if m.focus == focusFiles {
				m.openCommitPicker()
				return m, nil
			}
			return m, nil
		case "tab":
			switch m.focus {
			case focusFiles:
				m.focus = focusCommits
			case focusCommits:
				m.focus = focusDiff
				return m, m.maybeLoadBlame()
			default:
				m.focus = focusFiles
			}
			return m, nil
		case "h":
			if m.focus == focusDiff {
				m.focus = focusFiles
			}
			return m, nil
		case "l":
			if m.focus == focusFiles || m.focus == focusCommits {
				m.focus = focusDiff
				return m, m.maybeLoadBlame()
			}
			return m, nil
		case "H", "left":
			if m.mode == domain.ModeConflict && m.focus == focusDiff {
				m.setConflictSide(conflictSideOurs)
				m.actionMessage = "Conflict target: ours."
				return m, nil
			}
			return m, nil
		case "L", "right":
			if m.mode == domain.ModeConflict && m.focus == focusDiff {
				m.setConflictSide(conflictSideTheirs)
				m.actionMessage = "Conflict target: theirs."
				return m, nil
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
				m.moveDiffCursor(1, m.currentDiffContentWidth())
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
				m.moveDiffCursor(-1, m.currentDiffContentWidth())
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
			if m.diffViewMode == diffViewFullFile {
				m.actionMessage = "Full-file view is always side-by-side."
				return m, nil
			}
			m.toggleDiffLayout()
			return m, nil
		case "f":
			if m.mode == domain.ModeConflict {
				return m, nil
			}
			m.toggleDiffViewMode()
			return m, m.refreshDiff()
		case "F":
			m.toggleDiffFullScreen()
			return m, nil
		case "g":
			if m.mode != domain.ModeConflict {
				m.customCompare = nil
				m.mode = domain.ModeHistory
				return m, m.refreshFiles()
			}
			return m, nil
		case "v":
			return m, m.toggleCommitCompare()
		case "1":
			if m.mode == domain.ModeConflict && m.repo != nil {
				conflict := m.selectedConflictValue()
				if conflict != nil {
					if blockIndex := m.currentConflictBlockIndex(m.currentDiffContentWidth()); blockIndex >= 0 {
						m.actionMessage = fmt.Sprintf("Applying ours to conflict %d...", blockIndex+1)
						return m, applyConflictBlockCmd(m.repo.RootPath, conflict.Path, blockIndex, "ours")
					}
					m.actionMessage = "Applying whole file ours..."
					return m, acceptConflictCmd(m.repo.RootPath, conflict.Path, "ours")
				}
			}
			return m, nil
		case "2":
			if m.mode == domain.ModeConflict && m.repo != nil {
				conflict := m.selectedConflictValue()
				if conflict != nil {
					if blockIndex := m.currentConflictBlockIndex(m.currentDiffContentWidth()); blockIndex >= 0 {
						m.actionMessage = fmt.Sprintf("Applying theirs to conflict %d...", blockIndex+1)
						return m, applyConflictBlockCmd(m.repo.RootPath, conflict.Path, blockIndex, "theirs")
					}
					m.actionMessage = "Applying whole file theirs..."
					return m, acceptConflictCmd(m.repo.RootPath, conflict.Path, "theirs")
				}
			}
			return m, nil
		case "3":
			if m.mode == domain.ModeConflict && m.repo != nil {
				conflict := m.selectedConflictValue()
				if conflict != nil {
					if blockIndex := m.currentConflictBlockIndex(m.currentDiffContentWidth()); blockIndex >= 0 {
						m.actionMessage = fmt.Sprintf("Applying both sides to conflict %d...", blockIndex+1)
						return m, applyConflictBlockCmd(m.repo.RootPath, conflict.Path, blockIndex, "both")
					}
				}
			}
			return m, nil
		case "O":
			if m.mode == domain.ModeConflict && m.repo != nil {
				conflict := m.selectedConflictValue()
				if conflict != nil {
					m.actionMessage = "Applying whole file ours..."
					return m, acceptConflictCmd(m.repo.RootPath, conflict.Path, "ours")
				}
			}
			return m, nil
		case "T":
			if m.mode == domain.ModeConflict && m.repo != nil {
				conflict := m.selectedConflictValue()
				if conflict != nil {
					m.actionMessage = "Applying whole file theirs..."
					return m, acceptConflictCmd(m.repo.RootPath, conflict.Path, "theirs")
				}
			}
			return m, nil
		case "r":
			return m, m.hardRefresh()
		case "u":
			if m.mode == domain.ModeConflict {
				return m, nil
			}
			if m.repo == nil {
				m.actionMessage = "No repository loaded."
				return m, nil
			}
			patch, hunkIndex, ok := m.currentEditableHunkPatch(m.currentDiffContentWidth())
			if !ok {
				m.actionMessage = "Hunk revert is only available in staged or unstaged local patch views."
				return m, nil
			}
			switch m.currentLocalCompareMode() {
			case localCompareStaged:
				m.actionMessage = fmt.Sprintf("Reverting hunk %d from index...", hunkIndex+1)
			default:
				m.actionMessage = fmt.Sprintf("Reverting hunk %d from working tree...", hunkIndex+1)
			}
			return m, revertHunkCmd(m.repo.RootPath, patch, m.currentLocalCompareMode())
		case "U":
			if m.mode == domain.ModeConflict {
				return m, nil
			}
			if m.repo == nil {
				m.actionMessage = "No repository loaded."
				return m, nil
			}
			compare := m.editableLocalComparison()
			file := m.selectedFileValue()
			if compare == nil || file == nil {
				m.actionMessage = "File revert is only available in staged or unstaged local patch views."
				return m, nil
			}
			switch m.currentLocalCompareMode() {
			case localCompareStaged:
				m.actionMessage = fmt.Sprintf("Reverting %s from index...", file.Path)
			default:
				m.actionMessage = fmt.Sprintf("Reverting %s from working tree...", file.Path)
			}
			return m, revertFileCmd(m.repo.RootPath, compare.LeftRef, file.Path, m.currentLocalCompareMode())
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
		m.renderActionBar(m.width - 2),
		styleMuted.Render("Press ? for all keys and : for the action menu."),
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
	commitPicker := ""
	if m.commitPickerOpen {
		pickerHeight := 12
		contentHeight -= pickerHeight
		commitPicker = m.renderCommitPicker(m.width-2, pickerHeight)
	}
	helpOverlay := ""
	if m.helpOpen {
		panelHeight := helpOverlayHeight
		contentHeight -= panelHeight
		helpOverlay = m.renderHelpOverlay(m.width-2, panelHeight)
	}
	blameDetail := ""
	if m.blameDetailOpen {
		panelHeight := 7
		contentHeight -= panelHeight
		blameDetail = m.renderBlameDetail(m.width-2, panelHeight)
	}
	conflictBaseDetail := ""
	if m.conflictBaseOpen {
		panelHeight := 9
		contentHeight -= panelHeight
		conflictBaseDetail = m.renderConflictBaseDetail(m.width-2, panelHeight)
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

	panes := ""
	if m.diffFullScreen {
		panes = m.renderDiffPane(maxInt(40, m.width-2), contentHeight)
	} else {
		leftWidth := clampInt(m.width/3, 30, 42)
		rightWidth := m.width - leftWidth - 4
		if rightWidth < 40 {
			rightWidth = 40
		}
		filesHeight := clampInt(contentHeight/3, 8, maxInt(8, contentHeight-10))
		commitsHeight := contentHeight - filesHeight - 1
		if commitsHeight < 8 {
			commitsHeight = 8
			filesHeight = maxInt(8, contentHeight-commitsHeight-1)
		}

		leftColumn := lipgloss.JoinVertical(
			lipgloss.Left,
			m.renderFilesPane(leftWidth, filesHeight),
			m.renderCommitsPane(leftWidth, commitsHeight),
		)

		panes = lipgloss.JoinHorizontal(
			lipgloss.Top,
			leftColumn,
			m.renderDiffPane(rightWidth, contentHeight),
		)
	}

	parts := append([]string{}, header...)
	if palette != "" {
		parts = append(parts, palette)
	}
	if commitPicker != "" {
		parts = append(parts, commitPicker)
	}
	if helpOverlay != "" {
		parts = append(parts, helpOverlay)
	}
	if blameDetail != "" {
		parts = append(parts, blameDetail)
	}
	if conflictBaseDetail != "" {
		parts = append(parts, conflictBaseDetail)
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
	if m.diffFullScreen {
		return m.diffRenderWidth(maxInt(8, m.width-6))
	}
	leftWidth := clampInt(m.width/3, 30, 42)
	rightWidth := m.width - leftWidth - 4
	if rightWidth < 40 {
		rightWidth = 40
	}
	return m.diffRenderWidth(maxInt(8, rightWidth-4))
}

func (m *model) blameColumnWidth(totalWidth int) int {
	return 0
}

func (m *model) diffRenderWidth(totalWidth int) int {
	return totalWidth
}

func (m *model) renderCommitsPane(width, height int) string {
	lines := []string{}
	title := fmt.Sprintf("Commits (%d)", len(m.commits))
	if m.focus == focusCommits {
		title += " [enter compare]"
	}
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
		if anchor := m.commitBySHA(m.compareAnchor); anchor != nil {
			selected := m.selectedCommitValue()
			if selected != nil && selected.SHA != anchor.SHA {
				lines = append(lines, styleMode.Render(trimToWidth("Compare: "+formatCompareTargetLabel(anchor)+" -> "+formatCompareTargetLabel(selected), maxInt(10, width-4))))
			} else {
				lines = append(lines, styleMode.Render(fmt.Sprintf("Anchor: %s  |  press Enter on another commit", anchor.ShortSHA)))
			}
		} else if selected := m.selectedCommitValue(); selected != nil {
			lines = append(lines, styleMuted.Render(trimToWidth("Selected: "+selected.ShortSHA+" "+selected.Subject, width-4)))
		}
		start, end := visibleListRange(len(m.commits), m.selectedCommit, height-2-len(lines))
		for i := start; i < end; i++ {
			commit := m.commits[i]
			lines = append(lines, m.renderCommitLine(commit, i == m.selectedCommit, width-4))
		}
	}

	if len(lines) == 0 {
		lines = append(lines, styleMuted.Render("No commits loaded."))
	}

	return paneStyle(width, height, m.focus == focusCommits).Render(styleTitle.Render(title) + "\n\n" + strings.Join(lines, "\n"))
}

func (m *model) renderPalette(width, height int) string {
	commands := m.filteredPaletteCommands()
	lines := []string{
		styleAccent.Render("Action Menu"),
		styleMuted.Render("Type to filter. Enter runs. Esc closes. Use ? for the full keyboard map."),
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

func (m *model) renderHelpOverlay(width, height int) string {
	lines := []string{
		styleAccent.Render("Keyboard Help"),
		styleMuted.Render("j/k scroll. Esc or ? closes."),
		"",
	}

	currentSection := ""
	for _, binding := range m.currentKeyBindings() {
		if binding.section != currentSection {
			currentSection = binding.section
			lines = append(lines, styleSection.Render(currentSection))
		}
		line := fmt.Sprintf("%-10s %s", binding.key, binding.label)
		lines = append(lines, trimToWidth(line, width-4))
	}

	visibleHeight := maxInt(1, height-2)
	start := clampInt(m.helpScroll, 0, maxInt(0, len(lines)-visibleHeight))
	end := minInt(len(lines), start+visibleHeight)

	return lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("13")).
		Width(width).
		Height(height).
		Padding(0, 1).
		Render(strings.Join(lines[start:end], "\n"))
}

func (m *model) renderCommitPicker(width, height int) string {
	file := m.selectedFileValue()
	current := m.selectedCommitValue()
	target := "(no file selected)"
	currentLabel := "(no commit selected)"
	if file != nil {
		target = file.Path
	}
	if current != nil {
		currentLabel = current.ShortSHA + " " + current.Subject
	}

	lines := []string{
		styleAccent.Render("Compare File To..."),
		styleMuted.Render("Pick Working Tree, Index, or another commit for the selected file. Type to filter. Enter selects. Esc closes."),
		styleMuted.Render("File: " + trimToWidth(target, width-10)),
		styleMuted.Render("Current: " + trimToWidth(currentLabel, width-13)),
		styleMuted.Render("Query: " + m.commitPickerQuery),
		"",
	}

	commits := m.filteredCommitPickerCommits()
	if len(commits) == 0 {
		lines = append(lines, styleMuted.Render("No matching commits."))
	} else {
		start, end := visibleListRange(len(commits), m.commitPickerSelect, height-len(lines)-2)
		for i := start; i < end; i++ {
			lines = append(lines, renderCommitPickerLine(commits[i], width-4, i == m.commitPickerSelect))
		}
	}

	return lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("10")).
		Width(width).
		Height(height).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))
}

func (m *model) renderBlameDetail(width, height int) string {
	blame := m.currentCursorBlame(width - 2)
	lines := []string{
		styleAccent.Render("Inline Blame"),
	}

	if blame == nil {
		lines = append(lines, styleMuted.Render("No blamed diff line is selected right now. Move the diff cursor to a content line and press K again."))
	} else {
		lines = append(lines,
			styleMuted.Render("Author: "+blame.AuthorName),
			styleMuted.Render("Commit: "+blame.ShortSHA+"  Date: "+blame.AuthorTime),
			styleMuted.Render("Line: "+fmt.Sprintf("%d", blame.Line)),
			trimToWidth(blame.Summary, width-4),
			styleMuted.Render("Esc closes."),
		)
	}

	return lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("8")).
		Width(width).
		Height(height).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))
}

func (m *model) renderConflictBaseDetail(width, height int) string {
	block := m.currentConflictBlock(m.currentDiffContentWidth())
	lines := []string{
		styleAccent.Render("Conflict Base"),
	}

	switch {
	case block == nil:
		lines = append(lines, styleMuted.Render("No conflict block is selected right now. Move the diff cursor onto a conflict block and press K again."))
	case len(block.Base) == 0:
		lines = append(lines,
			styleMuted.Render(fmt.Sprintf("Conflict %d has no base section in the merge markers.", block.Index+1)),
			styleMuted.Render("Esc closes."),
		)
	default:
		lines = append(lines, styleMuted.Render(fmt.Sprintf("Conflict %d  |  base lines: %d", block.Index+1, len(block.Base))))
		available := maxInt(1, height-5)
		visible := minInt(len(block.Base), available)
		for index := 0; index < visible; index++ {
			lines = append(lines, styleMuted.Render(trimToWidth(block.Base[index], width-4)))
		}
		if remaining := len(block.Base) - visible; remaining > 0 {
			lines = append(lines, styleMuted.Render(fmt.Sprintf("... %d more line(s)", remaining)))
		}
		lines = append(lines, styleMuted.Render("Esc closes."))
	}

	return lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("8")).
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
	if m.focus == focusFiles {
		title += " [enter compare]"
	}
	if m.loadingFiles {
		lines = append(lines, styleAccent.Render("Loading files..."))
	}
	if m.filesErr != "" {
		lines = append(lines, styleMuted.Render(m.filesErr))
	}
	if file := m.selectedFileValue(); file != nil {
		lines = append(lines, styleMuted.Render(trimToWidth("Selected: "+file.Path, width-4)))
	}

	start, end := visibleListRange(len(m.files), m.selectedFile, height-2-len(lines))
	for i := start; i < end; i++ {
		file := m.files[i]
		selected := i == m.selectedFile
		switch {
		case selected:
			lines = append(lines, renderSelectedFileLine(file, width-4))
		default:
			line := renderFileStatusBadge(file.Status, false) + " " + trimPathMiddle(file.Path, maxInt(1, width-8))
			if file.OldPath != "" {
				line += " <- " + trimPathMiddle(file.OldPath, maxInt(1, width-lipgloss.Width(line)-4))
			}
			line = trimToWidth(line, width-4)
			if file.Status == "U" {
				lines = append(lines, styleError.Render(line))
			} else {
				lines = append(lines, styleDefault.Render(line))
			}
		}
	}

	if len(lines) == 0 {
		lines = append(lines, styleMuted.Render("No files loaded."))
	}

	return paneStyle(width, height, m.focus == focusFiles).Render(styleTitle.Render(title) + "\n\n" + strings.Join(lines, "\n"))
}

func (m *model) renderDiffPane(width, height int) string {
	lines := []string{}
	viewLabel := m.diffViewLabel()
	if m.mode == domain.ModeConflict {
		viewLabel = "conflict"
	}
	title := "Diff [" + viewLabel + "]"
	if m.focus == focusDiff {
		title += " [j/k selects line]"
	}
	if m.diffFullScreen {
		title += " [fullscreen]"
	}
	if m.loadingDiff {
		lines = append(lines, styleAccent.Render("Loading diff..."))
	}
	if m.diffErr != "" {
		lines = append(lines, styleMuted.Render(m.diffErr))
	}

	status := m.currentDiffStatus(width - 4)
	if status != "" {
		lines = append(lines, styleCursorInfo.Width(width-4).Render(status))
	}
	if summary := m.currentConflictInlineSummary(width - 4); summary != "" {
		lines = append(lines, styleMuted.Width(width-4).Render(summary))
	}
	resultPane := ""
	resultPaneHeight := 0
	if m.mode == domain.ModeConflict && m.currentConflictBlock(m.currentDiffContentWidth()) != nil {
		resultPaneHeight = minInt(10, maxInt(6, height/4))
	}
	lines = append(lines, m.renderDiffLines(width-4, maxInt(1, height-3-len(lines)-resultPaneHeight))...)
	if resultPaneHeight > 0 {
		resultPane = m.renderConflictResultPane(width-4, resultPaneHeight)
	}

	if file := m.selectedFileValue(); file != nil {
		title = "Diff [" + viewLabel + "]: " + trimToWidth(file.Path, width-20)
	}
	body := strings.Join(lines, "\n")
	if resultPane != "" {
		body = body + "\n\n" + resultPane
	}
	return paneStyle(width, height, m.focus == focusDiff).Render(styleTitle.Render(title) + "\n\n" + body)
}

func (m *model) renderCommitLine(commit domain.CommitSummary, selected bool, width int) string {
	if width <= 0 {
		return ""
	}

	graphPlain := commit.Graph
	if m.compareAnchor == commit.SHA {
		if graphPlain != "" {
			graphPlain = "• " + graphPlain
		} else {
			graphPlain = "•"
		}
	}
	graphPlain = compactGraphLead(graphPlain, maxInt(4, width/5))

	graphRendered := renderGraphLead(graphPlain, selected)
	graphWidth := lipgloss.Width(graphPlain)
	if graphWidth > 0 {
		graphRendered += " "
		graphWidth++
	}

	shaRendered := styleSHA.Render(commit.ShortSHA)
	if selected {
		shaRendered = styleSelectedSHA.Render(commit.ShortSHA)
	}
	shaWidth := lipgloss.Width(commit.ShortSHA)

	badgesRendered, badgesWidth := renderRefBadges(commit.Refs, maxInt(0, width/3), selected)

	subjectBudget := width - graphWidth - shaWidth - 1
	if badgesWidth > 0 {
		subjectBudget -= badgesWidth + 1
	}
	if subjectBudget < 12 && badgesWidth > 0 {
		badgesRendered = ""
		badgesWidth = 0
		subjectBudget = width - graphWidth - shaWidth - 1
	}
	subjectBudget = maxInt(8, subjectBudget)
	subject := trimToWidth(commit.Subject, subjectBudget)

	parts := []string{}
	if graphRendered != "" {
		parts = append(parts, graphRendered)
	}
	parts = append(parts, shaRendered, " "+subject)
	if badgesRendered != "" {
		parts = append(parts, " ", badgesRendered)
	}

	line := lipgloss.JoinHorizontal(lipgloss.Left, parts...)
	line = lipgloss.NewStyle().Width(width).MaxWidth(width).Render(line)
	if selected {
		return styleSelectedCommit.Render(line)
	}
	return line
}

func (m *model) renderDiffLines(width, height int) []string {
	if m.mode == domain.ModeConflict {
		if m.conflictContents == nil {
			return []string{styleMuted.Render("No conflict content loaded.")}
		}
	} else if m.diffViewMode == diffViewFullFile {
		if m.fullFileCompare == nil {
			return []string{styleMuted.Render(m.emptyDiffMessage())}
		}
	} else if m.diff == "" {
		return []string{styleMuted.Render(m.emptyDiffMessage())}
	}

	document := m.syncDiffCursor(width)
	start := m.diffViewportStart(document, height)
	m.diffScroll = start

	if !m.showBlame {
		end := minInt(len(document.rows), start+height)
		lines := make([]string, 0, end-start)
		for index := start; index < end; index++ {
			row := document.rows[index]
			if index == m.diffCursor {
				row = renderSelectedDiffRow(row, width)
			}
			lines = append(lines, row)
		}
		return lines
	}

	lines := make([]string, 0, height)
	lastSummary, hasLastSummary := m.blameSummaryBefore(document, start)

	for index := start; index < len(document.rows) && len(lines) < height; index++ {
		meta := render.RowMeta{}
		if index < len(document.rowMeta) {
			meta = document.rowMeta[index]
		}

		if blame := m.blameLineForMeta(meta); blame != nil {
			summary := blameSummary(blame)
			if summary != "" && (!hasLastSummary || summary != lastSummary) {
				lines = append(lines, renderBlameSeparator(summary, width))
				lastSummary = summary
				hasLastSummary = true
				if len(lines) >= height {
					break
				}
			}
		}
		row := document.rows[index]
		if index == m.diffCursor {
			row = renderSelectedDiffRow(row, width)
		}
		lines = append(lines, row)
	}

	return lines
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
	styleHeaderBar      = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(lipgloss.Color("236")).Padding(0, 1)
	styleSection        = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
	styleAdd            = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	styleDel            = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	styleError          = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	styleDefault        = lipgloss.NewStyle()
	styleGraph          = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	styleGraphLane      = lipgloss.NewStyle().Foreground(lipgloss.Color("37"))
	styleGraphNode      = lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true)
	styleAnchor         = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	styleSHA            = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	styleRefHead        = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("25")).Bold(true)
	styleRefBranch      = lipgloss.NewStyle().Foreground(lipgloss.Color("153")).Background(lipgloss.Color("237"))
	styleRefTag         = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("60"))
	styleSelectedCommit = lipgloss.NewStyle().
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("25")).
				Bold(true)
	styleSelectedGraphLane = lipgloss.NewStyle().Foreground(lipgloss.Color("195")).Background(lipgloss.Color("25"))
	styleSelectedGraphNode = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("25")).Bold(true)
	styleSelectedAnchor    = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Background(lipgloss.Color("25")).Bold(true)
	styleSelectedSHA       = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("25")).Bold(true)
	styleSelectedRefHead   = lipgloss.NewStyle().Foreground(lipgloss.Color("25")).Background(lipgloss.Color("230")).Bold(true)
	styleSelectedRefBranch = lipgloss.NewStyle().Foreground(lipgloss.Color("25")).Background(lipgloss.Color("195"))
	styleSelectedRefTag    = lipgloss.NewStyle().Foreground(lipgloss.Color("25")).Background(lipgloss.Color("229"))
	styleSelectedFile      = lipgloss.NewStyle().
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("24")).
				Bold(true)
	styleCursorInfo = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")).
			Background(lipgloss.Color("237"))
	styleSelectedDiffLine = lipgloss.NewStyle().
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("24"))
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

func trimPathMiddle(path string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(path) <= width {
		return path
	}

	parts := strings.Split(path, "/")
	if len(parts) <= 1 {
		return trimToWidth(path, width)
	}

	base := parts[len(parts)-1]
	if lipgloss.Width(base)+4 >= width {
		return ".../" + trimToWidth(base, maxInt(1, width-4))
	}

	first := parts[0]
	candidate := first + "/.../" + base
	if lipgloss.Width(candidate) <= width {
		return candidate
	}

	return ".../" + base
}

func renderFileStatusBadge(status string, selected bool) string {
	label := " " + strings.TrimSpace(status) + " "
	switch status {
	case "A":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("22")).Bold(true).Render(label)
	case "D":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("52")).Bold(true).Render(label)
	case "R":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("60")).Bold(true).Render(label)
	case "U":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("88")).Bold(true).Render(label)
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("24")).Bold(true).Render(label)
	}
}

func renderSelectedFileLine(file domain.FileChange, width int) string {
	line := strings.TrimSpace(file.Status) + " " + trimPathMiddle(file.Path, maxInt(1, width-2))
	if file.OldPath != "" {
		line += " <- " + trimPathMiddle(file.OldPath, maxInt(1, width-lipgloss.Width(line)-4))
	}
	return styleSelectedFile.Width(width).Render(trimToWidth(line, width))
}

func renderCommitPickerLine(commit domain.CommitSummary, width int, selected bool) string {
	prefix := "  "
	if selected {
		prefix = "> "
	}

	if commit.SHA == gitadapter.WorkingTreeRef || commit.SHA == gitadapter.IndexRef {
		line := prefix + commit.ShortSHA + "  " + commit.Subject
		return trimToWidth(line, width)
	}

	meta := commit.AuthoredAt
	if len(commit.Refs) > 0 {
		meta += "  " + strings.Join(commit.Refs, ", ")
	} else if commit.AuthorName != "" {
		meta += "  " + commit.AuthorName
	}

	line := fmt.Sprintf("%s%-8s %-24s %s", prefix, commit.ShortSHA, trimToWidth(meta, 24), commit.Subject)
	return trimToWidth(line, width)
}

func compactGraphLead(graph string, maxWidth int) string {
	graph = strings.TrimRight(graph, " ")
	if maxWidth <= 0 || lipgloss.Width(graph) <= maxWidth {
		return graph
	}
	runes := []rune(graph)
	if len(runes) <= maxWidth {
		return graph
	}
	return "…" + string(runes[len(runes)-maxWidth+1:])
}

func renderSelectedDiffRow(row string, width int) string {
	if width <= 0 {
		return row
	}
	return styleSelectedDiffLine.Width(width).Render(row)
}

func firstSelectableDiffRow(document renderedDiff) int {
	for index, meta := range document.rowMeta {
		switch meta.Kind {
		case render.LineContext, render.LineAdd, render.LineDelete:
			return index
		}
	}
	if len(document.hunkRows) > 0 {
		return document.hunkRows[0]
	}
	return 0
}

func formatCompareTargetLabel(commit *domain.CommitSummary) string {
	if commit == nil {
		return ""
	}

	parts := []string{commit.ShortSHA}
	if commit.AuthoredAt != "" {
		parts = append(parts, commit.AuthoredAt)
	}
	if len(commit.Refs) > 0 {
		parts = append(parts, strings.Join(commit.Refs, ", "))
	}
	return strings.Join(parts, " · ")
}

func renderGraphLead(graph string, selected bool) string {
	if graph == "" {
		return ""
	}

	var builder strings.Builder
	for _, r := range graph {
		switch r {
		case '*':
			if selected {
				builder.WriteString(styleSelectedGraphNode.Render("●"))
			} else {
				builder.WriteString(styleGraphNode.Render("●"))
			}
		case '•':
			if selected {
				builder.WriteString(styleSelectedAnchor.Render("•"))
			} else {
				builder.WriteString(styleAnchor.Render("•"))
			}
		case '|', '/', '\\', '_':
			if selected {
				builder.WriteString(styleSelectedGraphLane.Render(string(r)))
			} else {
				builder.WriteString(styleGraphLane.Render(string(r)))
			}
		default:
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func renderRefBadges(refs []string, budget int, selected bool) (string, int) {
	if len(refs) == 0 || budget <= 0 {
		return "", 0
	}

	rendered := make([]string, 0, len(refs))
	used := 0
	for _, ref := range refs {
		label := strings.TrimSpace(ref)
		if label == "" {
			continue
		}

		badge := refBadgeStyle(label, selected).Render(" " + label + " ")
		width := lipgloss.Width(badge)
		if len(rendered) > 0 {
			width++
		}
		if used+width > budget {
			break
		}
		if len(rendered) > 0 {
			used++
		}
		rendered = append(rendered, badge)
		used += lipgloss.Width(badge)
	}

	if len(rendered) == 0 {
		return "", 0
	}
	return strings.Join(rendered, " "), used
}

func refBadgeStyle(ref string, selected bool) lipgloss.Style {
	switch {
	case strings.Contains(ref, "HEAD"):
		if selected {
			return styleSelectedRefHead
		}
		return styleRefHead
	case strings.HasPrefix(ref, "tag:"):
		if selected {
			return styleSelectedRefTag
		}
		return styleRefTag
	default:
		if selected {
			return styleSelectedRefBranch
		}
		return styleRefBranch
	}
}
