package domain

type DiffStyle string

const (
	DiffTwoDot   DiffStyle = "two-dot"
	DiffThreeDot DiffStyle = "three-dot"
)

type ExplorerMode string

const (
	ModeHistory        ExplorerMode = "history"
	ModeComparePreset  ExplorerMode = "compare-preset"
	ModeCompareCommits ExplorerMode = "compare-commits"
	ModeConflict       ExplorerMode = "conflict"
)

type RepositoryInfo struct {
	RootPath           string
	GitDir             string
	HeadRef            string
	DefaultCompareBase string
	IsMergeInProgress  bool
	IsRebaseInProgress bool
	IsCherryPick       bool
}

type CommitSummary struct {
	Graph      string
	SHA        string
	ShortSHA   string
	AuthoredAt string
	AuthorName string
	Refs       []string
	Subject    string
}

type FileChange struct {
	Path    string
	OldPath string
	Status  string
}

type CompareSelection struct {
	LeftRef   string
	RightRef  string
	LeftLabel string
	RightLabel string
	DiffStyle DiffStyle
}

type ConflictFile struct {
	Path      string
	HasBase   bool
	HasOurs   bool
	HasTheirs bool
}

type ConflictFileContents struct {
	Path   string
	Base   string
	Ours   string
	Theirs string
	Merged string
}
