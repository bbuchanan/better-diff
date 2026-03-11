package gitadapter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"better-diff/internal/conflicts"
	"better-diff/internal/domain"
)

const (
	recordSeparator = '\x1e'
	fieldSeparator  = '\x1f'
	WorkingTreeRef  = "__WORKTREE__"
	IndexRef        = "__INDEX__"
)

type CommandError struct {
	Command string
	Stderr  string
	Err     error
}

func (e *CommandError) Error() string {
	if e.Stderr != "" {
		return e.Stderr
	}
	return e.Err.Error()
}

func sanitizeOutput(value string) string {
	value = strings.ReplaceAll(value, "\r", "")
	value = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r == recordSeparator || r == fieldSeparator {
			return r
		}
		if r < 32 || (r >= 127 && r <= 159) {
			return -1
		}
		return r
	}, value)
	return value
}

func runGit(ctx context.Context, cwd string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = cwd

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", &CommandError{
			Command: strings.Join(append([]string{"git"}, args...), " "),
			Stderr:  sanitizeOutput(strings.TrimSpace(stderr.String())),
			Err:     err,
		}
	}

	return sanitizeOutput(stdout.String()), nil
}

func runGitWithStdin(ctx context.Context, cwd, input string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = cwd
	cmd.Stdin = strings.NewReader(input)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", &CommandError{
			Command: strings.Join(append([]string{"git"}, args...), " "),
			Stderr:  sanitizeOutput(strings.TrimSpace(stderr.String())),
			Err:     err,
		}
	}

	return sanitizeOutput(stdout.String()), nil
}

func revisionExists(ctx context.Context, cwd, revision string) bool {
	_, err := runGit(ctx, cwd, "rev-parse", "--verify", revision+"^{commit}")
	return err == nil
}

func refExists(ctx context.Context, cwd, revision string) bool {
	_, err := runGit(ctx, cwd, "rev-parse", "--verify", revision)
	return err == nil
}

func discoverDefaultCompareBase(ctx context.Context, cwd, headRef string) string {
	candidates := []string{"main", "origin/main", "master", "origin/master"}
	if headRef != "" && headRef != "HEAD" {
		candidates = append(candidates, headRef)
	}

	for _, candidate := range candidates {
		if revisionExists(ctx, cwd, candidate) {
			return candidate
		}
	}

	return ""
}

func DiscoverRepository(ctx context.Context, cwd string) (domain.RepositoryInfo, error) {
	rootPath, err := runGit(ctx, cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return domain.RepositoryInfo{}, err
	}

	gitDir, err := runGit(ctx, cwd, "rev-parse", "--git-dir")
	if err != nil {
		return domain.RepositoryInfo{}, err
	}

	headRef, err := runGit(ctx, cwd, "branch", "--show-current")
	if err != nil {
		return domain.RepositoryInfo{}, err
	}

	rootPath = strings.TrimSpace(rootPath)
	headRef = strings.TrimSpace(headRef)
	if headRef == "" {
		headRef = "HEAD"
	}

	return domain.RepositoryInfo{
		RootPath:           rootPath,
		GitDir:             strings.TrimSpace(gitDir),
		HeadRef:            headRef,
		DefaultCompareBase: discoverDefaultCompareBase(ctx, rootPath, headRef),
		IsMergeInProgress:  refExists(ctx, rootPath, "MERGE_HEAD"),
		IsRebaseInProgress: refExists(ctx, rootPath, "REBASE_HEAD"),
		IsCherryPick:       refExists(ctx, rootPath, "CHERRY_PICK_HEAD"),
	}, nil
}

func ListCommits(ctx context.Context, cwd string, limit int) ([]domain.CommitSummary, error) {
	format := "%H%x1f%h%x1f%ad%x1f%an%x1f%D%x1f%s"
	raw, err := runGit(
		ctx,
		cwd,
		"log",
		"--graph",
		fmt.Sprintf("--max-count=%d", limit),
		"--date=short",
		fmt.Sprintf("--format=%c%s%c", fieldSeparator, format, recordSeparator),
	)
	if err != nil {
		return nil, err
	}

	records := strings.Split(raw, string(recordSeparator))
	commits := make([]domain.CommitSummary, 0, len(records))

	for _, record := range records {
		record = strings.TrimSuffix(record, "\n")
		record = strings.TrimLeft(record, "\n")
		if record == "" {
			continue
		}

		firstSep := strings.IndexRune(record, fieldSeparator)
		if firstSep < 0 {
			continue
		}

		graph := strings.ReplaceAll(record[:firstSep], "\n", "")
		fields := strings.Split(record[firstSep+1:], string(fieldSeparator))
		if len(fields) < 6 {
			continue
		}

		refs := []string{}
		for _, ref := range strings.Split(fields[4], ",") {
			ref = strings.TrimSpace(ref)
			if ref != "" {
				refs = append(refs, ref)
			}
		}

		commits = append(commits, domain.CommitSummary{
			Graph:      graph,
			SHA:        strings.TrimSpace(fields[0]),
			ShortSHA:   strings.TrimSpace(fields[1]),
			AuthoredAt: strings.TrimSpace(fields[2]),
			AuthorName: strings.TrimSpace(fields[3]),
			Refs:       refs,
			Subject:    strings.TrimSpace(fields[5]),
		})
	}

	return commits, nil
}

func ListRefs(ctx context.Context, cwd string) ([]domain.RefSummary, error) {
	raw, err := runGit(
		ctx,
		cwd,
		"for-each-ref",
		"--sort=-committerdate",
		fmt.Sprintf("--format=%%(refname:short)%c%%(refname)%c%%(objectname:short)%c%%(refname:lstrip=2)", fieldSeparator, fieldSeparator, fieldSeparator),
		"refs/heads",
		"refs/remotes",
		"refs/tags",
	)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(raw), "\n")
	refs := make([]domain.RefSummary, 0, len(lines))
	seen := map[string]struct{}{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, string(fieldSeparator))
		if len(fields) < 4 {
			continue
		}

		name := strings.TrimSpace(fields[0])
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}

		fullName := strings.TrimSpace(fields[1])
		refType := "other"
		switch {
		case strings.HasPrefix(fullName, "refs/heads/"):
			refType = "branch"
		case strings.HasPrefix(fullName, "refs/remotes/"):
			refType = "remote"
		case strings.HasPrefix(fullName, "refs/tags/"):
			refType = "tag"
		}

		refs = append(refs, domain.RefSummary{
			Name:     name,
			FullName: fullName,
			ShortSHA: strings.TrimSpace(fields[2]),
			Type:     refType,
		})
	}

	return refs, nil
}

func parseNameStatusOutput(raw string) []domain.FileChange {
	lines := strings.Split(raw, "\n")
	files := make([]domain.FileChange, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}

		status := string(parts[0][0])
		change := domain.FileChange{
			Status: status,
		}

		if status == "R" || status == "C" {
			if len(parts) >= 3 {
				change.OldPath = parts[1]
				change.Path = parts[2]
			} else {
				change.Path = parts[1]
			}
		} else {
			change.Path = parts[1]
		}

		if change.Path != "" {
			files = append(files, change)
		}
	}

	return files
}

func ListCommitFiles(ctx context.Context, cwd, sha string) ([]domain.FileChange, error) {
	return ListCommitFilesWithOptions(ctx, cwd, sha, false)
}

func ListCommitFilesWithOptions(ctx context.Context, cwd, sha string, ignoreWhitespace bool) ([]domain.FileChange, error) {
	args := []string{"show", "--no-ext-diff", "--format=", "--name-status", "--find-renames"}
	if ignoreWhitespace {
		args = append(args, "-w")
	}
	args = append(args, sha)

	raw, err := runGit(ctx, cwd, args...)
	if err != nil {
		return nil, err
	}

	return parseNameStatusOutput(raw), nil
}

func diffSpecifier(left, right string, style domain.DiffStyle) string {
	if style == domain.DiffThreeDot {
		return left + "..." + right
	}
	return left + ".." + right
}

func isWorkingTreeRef(revision string) bool {
	return revision == WorkingTreeRef
}

func isIndexRef(revision string) bool {
	return revision == IndexRef
}

func appendDiffRange(args []string, left, right string, style domain.DiffStyle) []string {
	switch {
	case isWorkingTreeRef(left) && isWorkingTreeRef(right):
		return args
	case isWorkingTreeRef(right):
		if left != "" && !isWorkingTreeRef(left) {
			return append(args, left)
		}
		return args
	case isWorkingTreeRef(left):
		if right != "" && !isWorkingTreeRef(right) {
			return append(args, right)
		}
		return args
	default:
		return append(args, diffSpecifier(left, right, style))
	}
}

func listUntrackedFiles(ctx context.Context, cwd string) ([]domain.FileChange, error) {
	raw, err := runGit(ctx, cwd, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}

	files := []domain.FileChange{}
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		files = append(files, domain.FileChange{Path: line, Status: "A"})
	}
	return files, nil
}

func mergeFileChanges(base, extra []domain.FileChange) []domain.FileChange {
	if len(extra) == 0 {
		return base
	}

	merged := make([]domain.FileChange, 0, len(base)+len(extra))
	seen := map[string]struct{}{}
	makeKey := func(change domain.FileChange) string {
		return change.Status + ":" + change.OldPath + "->" + change.Path
	}

	for _, change := range base {
		merged = append(merged, change)
		seen[makeKey(change)] = struct{}{}
	}
	for _, change := range extra {
		key := makeKey(change)
		if _, ok := seen[key]; ok {
			continue
		}
		merged = append(merged, change)
		seen[key] = struct{}{}
	}

	sort.Slice(merged, func(i, j int) bool {
		if merged[i].Path == merged[j].Path {
			return merged[i].OldPath < merged[j].OldPath
		}
		return merged[i].Path < merged[j].Path
	})
	return merged
}

func diffUntrackedFile(ctx context.Context, cwd, path string) (string, error) {
	targetPath, err := resolveWorktreePath(cwd, path)
	if err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, "git", "diff", "--no-index", "--", "/dev/null", targetPath)
	cmd.Dir = cwd

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 && strings.TrimSpace(stderr.String()) == "" {
			return strings.TrimRight(sanitizeOutput(stdout.String()), "\n"), nil
		}
		return "", &CommandError{
			Command: "git diff --no-index -- /dev/null " + targetPath,
			Stderr:  sanitizeOutput(strings.TrimSpace(stderr.String())),
			Err:     err,
		}
	}

	return strings.TrimRight(sanitizeOutput(stdout.String()), "\n"), nil
}

func pathTrackedInIndex(ctx context.Context, cwd, path string) bool {
	_, err := runGit(ctx, cwd, "ls-files", "--error-unmatch", "--", path)
	return err == nil
}

func ListRangeFiles(ctx context.Context, cwd, left, right string, style domain.DiffStyle) ([]domain.FileChange, error) {
	return ListRangeFilesWithOptions(ctx, cwd, left, right, style, false)
}

func ListRangeFilesWithOptions(ctx context.Context, cwd, left, right string, style domain.DiffStyle, ignoreWhitespace bool) ([]domain.FileChange, error) {
	switch {
	case !isIndexRef(left) && isIndexRef(right):
		args := []string{"diff", "--cached", "--no-ext-diff", "--name-status", "--find-renames"}
		if ignoreWhitespace {
			args = append(args, "-w")
		}
		if left != "" && !isWorkingTreeRef(left) {
			args = append(args, left)
		}
		raw, err := runGit(ctx, cwd, args...)
		if err != nil {
			return nil, err
		}
		return parseNameStatusOutput(raw), nil
	case isIndexRef(left) && isWorkingTreeRef(right):
		args := []string{"diff", "--no-ext-diff", "--name-status", "--find-renames"}
		if ignoreWhitespace {
			args = append(args, "-w")
		}
		raw, err := runGit(ctx, cwd, args...)
		if err != nil {
			return nil, err
		}
		files := parseNameStatusOutput(raw)
		untracked, err := listUntrackedFiles(ctx, cwd)
		if err != nil {
			return nil, err
		}
		return mergeFileChanges(files, untracked), nil
	case !isIndexRef(left) && isWorkingTreeRef(right):
		args := []string{"diff", "--no-ext-diff", "--name-status", "--find-renames"}
		if ignoreWhitespace {
			args = append(args, "-w")
		}
		if left != "" && !isWorkingTreeRef(left) {
			args = append(args, left)
		}
		raw, err := runGit(ctx, cwd, args...)
		if err != nil {
			return nil, err
		}
		files := parseNameStatusOutput(raw)
		untracked, err := listUntrackedFiles(ctx, cwd)
		if err != nil {
			return nil, err
		}
		return mergeFileChanges(files, untracked), nil
	}

	args := []string{"diff", "--no-ext-diff", "--name-status", "--find-renames"}
	if ignoreWhitespace {
		args = append(args, "-w")
	}
	args = appendDiffRange(args, left, right, style)

	raw, err := runGit(ctx, cwd, args...)
	if err != nil {
		return nil, err
	}

	return parseNameStatusOutput(raw), nil
}

func GetCommitDiff(ctx context.Context, cwd, sha, path string, contextLines int) (string, error) {
	return GetCommitDiffWithOptions(ctx, cwd, sha, path, contextLines, false)
}

func GetCommitDiffWithOptions(ctx context.Context, cwd, sha, path string, contextLines int, ignoreWhitespace bool) (string, error) {
	args := []string{"show", "--no-ext-diff", fmt.Sprintf("--unified=%d", contextLines), "--format="}
	if ignoreWhitespace {
		args = append(args, "-w")
	}
	args = append(args, sha)
	if path != "" {
		args = append(args, "--", path)
	}

	raw, err := runGit(ctx, cwd, args...)
	if err != nil {
		return "", err
	}

	return strings.TrimRight(raw, "\n"), nil
}

func GetRangeDiff(ctx context.Context, cwd, left, right string, style domain.DiffStyle, path string, contextLines int) (string, error) {
	return GetRangeDiffWithOptions(ctx, cwd, left, right, style, path, contextLines, false)
}

func GetRangeDiffWithOptions(ctx context.Context, cwd, left, right string, style domain.DiffStyle, path string, contextLines int, ignoreWhitespace bool) (string, error) {
	switch {
	case !isIndexRef(left) && isIndexRef(right):
		args := []string{"diff", "--cached", "--no-ext-diff", fmt.Sprintf("--unified=%d", contextLines)}
		if ignoreWhitespace {
			args = append(args, "-w")
		}
		if left != "" && !isWorkingTreeRef(left) {
			args = append(args, left)
		}
		if path != "" {
			args = append(args, "--", path)
		}
		raw, err := runGit(ctx, cwd, args...)
		if err != nil {
			return "", err
		}
		return strings.TrimRight(raw, "\n"), nil
	case isIndexRef(left) && isWorkingTreeRef(right):
		if path != "" && !pathTrackedInIndex(ctx, cwd, path) {
			return diffUntrackedFile(ctx, cwd, path)
		}
		args := []string{"diff", "--no-ext-diff", fmt.Sprintf("--unified=%d", contextLines)}
		if ignoreWhitespace {
			args = append(args, "-w")
		}
		if path != "" {
			args = append(args, "--", path)
		}
		raw, err := runGit(ctx, cwd, args...)
		if err != nil {
			return "", err
		}
		return strings.TrimRight(raw, "\n"), nil
	case !isIndexRef(left) && isWorkingTreeRef(right):
		if path != "" && !pathTrackedInIndex(ctx, cwd, path) {
			return diffUntrackedFile(ctx, cwd, path)
		}
	}

	args := []string{"diff", "--no-ext-diff", fmt.Sprintf("--unified=%d", contextLines)}
	if ignoreWhitespace {
		args = append(args, "-w")
	}
	args = appendDiffRange(args, left, right, style)
	if path != "" {
		args = append(args, "--", path)
	}

	raw, err := runGit(ctx, cwd, args...)
	if err != nil {
		return "", err
	}

	return strings.TrimRight(raw, "\n"), nil
}

func GetBlame(ctx context.Context, cwd, revision, path string) (map[int]domain.BlameLine, error) {
	args := []string{"blame", "--line-porcelain"}
	if revision != "" {
		args = append(args, revision)
	}
	args = append(args, "--", path)

	raw, err := runGit(ctx, cwd, args...)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(raw, "\n")
	blame := map[int]domain.BlameLine{}
	commitCache := map[string]domain.BlameLine{}

	for index := 0; index < len(lines); {
		line := lines[index]
		fields := strings.Fields(line)
		if len(fields) < 3 {
			index++
			continue
		}

		sha := fields[0]
		finalLine := 0
		if _, err := fmt.Sscanf(fields[2], "%d", &finalLine); err != nil {
			index++
			continue
		}

		entry := commitCache[sha]
		entry.CommitSHA = sha
		if len(sha) >= 7 {
			entry.ShortSHA = sha[:7]
		} else {
			entry.ShortSHA = sha
		}
		entry.Line = finalLine
		index++

		for index < len(lines) {
			meta := lines[index]
			if strings.HasPrefix(meta, "\t") {
				index++
				break
			}
			switch {
			case strings.HasPrefix(meta, "author "):
				entry.AuthorName = strings.TrimSpace(strings.TrimPrefix(meta, "author "))
			case strings.HasPrefix(meta, "author-time "):
				var unixSeconds int64
				if _, err := fmt.Sscanf(strings.TrimPrefix(meta, "author-time "), "%d", &unixSeconds); err == nil {
					entry.AuthorTime = time.Unix(unixSeconds, 0).Format("2006-01-02")
				}
			case strings.HasPrefix(meta, "summary "):
				entry.Summary = strings.TrimSpace(strings.TrimPrefix(meta, "summary "))
			}
			index++
		}

		commitCache[sha] = entry
		blame[finalLine] = entry
	}

	return blame, nil
}

func GetFileContent(ctx context.Context, cwd, revision, path string) (string, error) {
	if path == "" {
		return "", nil
	}

	if revision == IndexRef {
		raw, err := runGit(ctx, cwd, "show", ":"+path)
		if err != nil {
			if commandErr, ok := err.(*CommandError); ok {
				stderr := commandErr.Stderr
				if strings.Contains(stderr, "does not exist in") || strings.Contains(stderr, "exists on disk, but not in") || strings.Contains(stderr, "path '"+path+"' does not exist") {
					return "", nil
				}
			}
			return "", err
		}
		return raw, nil
	}

	if revision == "" || revision == WorkingTreeRef {
		targetPath, err := resolveWorktreePath(cwd, path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return "", nil
			}
			return "", err
		}
		content, err := os.ReadFile(targetPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return "", nil
			}
			return "", err
		}
		return sanitizeOutput(string(content)), nil
	}

	raw, err := runGit(ctx, cwd, "show", revision+":"+path)
	if err != nil {
		if commandErr, ok := err.(*CommandError); ok {
			stderr := commandErr.Stderr
			if strings.Contains(stderr, "does not exist in") || strings.Contains(stderr, "exists on disk, but not in") || strings.Contains(stderr, "path '"+path+"' does not exist") {
				return "", nil
			}
		}
		return "", err
	}

	return raw, nil
}

func ListConflictFiles(ctx context.Context, cwd string) ([]domain.ConflictFile, error) {
	raw, err := runGit(ctx, cwd, "ls-files", "-u", "-z")
	if err != nil {
		var cmdErr *CommandError
		if !errors.As(err, &cmdErr) {
			return nil, err
		}
		if strings.Contains(cmdErr.Stderr, "not a git repository") {
			return nil, err
		}
		if strings.TrimSpace(cmdErr.Stderr) == "" {
			return []domain.ConflictFile{}, nil
		}
		return nil, err
	}

	if raw == "" {
		return []domain.ConflictFile{}, nil
	}

	byPath := map[string]domain.ConflictFile{}
	for _, entry := range strings.Split(raw, "\x00") {
		if entry == "" {
			continue
		}

		stageTab := strings.SplitN(entry, "\t", 2)
		if len(stageTab) != 2 {
			continue
		}

		meta := strings.Fields(stageTab[0])
		if len(meta) < 3 {
			continue
		}

		stage := meta[2]
		path := stageTab[1]
		conflict := byPath[path]
		conflict.Path = path

		switch stage {
		case "1":
			conflict.HasBase = true
		case "2":
			conflict.HasOurs = true
		case "3":
			conflict.HasTheirs = true
		}

		byPath[path] = conflict
	}

	conflicts := make([]domain.ConflictFile, 0, len(byPath))
	for _, conflict := range byPath {
		conflicts = append(conflicts, conflict)
	}

	sort.Slice(conflicts, func(i, j int) bool {
		return conflicts[i].Path < conflicts[j].Path
	})

	return conflicts, nil
}

func readStageBlob(ctx context.Context, cwd, path string, stage int) string {
	raw, err := runGit(ctx, cwd, "show", fmt.Sprintf(":%d:%s", stage, path))
	if err != nil {
		return ""
	}
	return strings.TrimRight(raw, "\n")
}

func resolveWorktreePath(root, relative string) (string, error) {
	root = filepath.Clean(root)
	target := filepath.Clean(filepath.Join(root, relative))

	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", fmt.Errorf("resolve repo path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("refusing to read outside repo: %s", relative)
	}
	return target, nil
}

func GetConflictFileContents(ctx context.Context, cwd, path string) (domain.ConflictFileContents, error) {
	targetPath, err := resolveWorktreePath(cwd, path)
	if err != nil {
		return domain.ConflictFileContents{}, err
	}

	merged, _ := os.ReadFile(targetPath)

	return domain.ConflictFileContents{
		Path:   path,
		Base:   readStageBlob(ctx, cwd, path, 1),
		Ours:   readStageBlob(ctx, cwd, path, 2),
		Theirs: readStageBlob(ctx, cwd, path, 3),
		Merged: strings.TrimRight(sanitizeOutput(string(merged)), "\n"),
	}, nil
}

func AcceptConflictSide(ctx context.Context, cwd, path, side string) error {
	if side != "ours" && side != "theirs" {
		return fmt.Errorf("unsupported conflict side: %s", side)
	}

	if _, err := runGit(ctx, cwd, "checkout", "--"+side, "--", path); err != nil {
		return err
	}

	if _, err := runGit(ctx, cwd, "add", "--", path); err != nil {
		return err
	}

	return nil
}

type ConflictApplyResult struct {
	Resolved        bool
	RemainingBlocks int
}

func ApplyConflictBlockResolution(ctx context.Context, cwd, path string, blockIndex int, resolution string) (ConflictApplyResult, error) {
	if resolution != "ours" && resolution != "theirs" && resolution != "both" {
		return ConflictApplyResult{}, fmt.Errorf("unsupported conflict block resolution: %s", resolution)
	}

	targetPath, err := resolveWorktreePath(cwd, path)
	if err != nil {
		return ConflictApplyResult{}, err
	}

	content, err := os.ReadFile(targetPath)
	if err != nil {
		return ConflictApplyResult{}, err
	}

	parsed := conflicts.Parse(string(content))
	rendered, resolved := conflicts.RenderResolved(parsed, blockIndex, resolution)
	if err := os.WriteFile(targetPath, []byte(rendered), 0o644); err != nil {
		return ConflictApplyResult{}, err
	}

	if resolved {
		if _, err := runGit(ctx, cwd, "add", "--", path); err != nil {
			return ConflictApplyResult{}, err
		}
	}

	remaining := conflicts.CountBlocks(conflicts.Parse(rendered))
	return ConflictApplyResult{
		Resolved:        resolved,
		RemainingBlocks: remaining,
	}, nil
}

func ApplyReversePatch(ctx context.Context, cwd, patch string) error {
	if strings.TrimSpace(patch) == "" {
		return errors.New("patch is empty")
	}
	if !strings.HasSuffix(patch, "\n") {
		patch += "\n"
	}

	_, err := runGitWithStdin(ctx, cwd, patch, "apply", "-R", "--recount", "--whitespace=nowarn", "-")
	return err
}

func ApplyReversePatchCached(ctx context.Context, cwd, patch string) error {
	if strings.TrimSpace(patch) == "" {
		return errors.New("patch is empty")
	}
	if !strings.HasSuffix(patch, "\n") {
		patch += "\n"
	}

	_, err := runGitWithStdin(ctx, cwd, patch, "apply", "-R", "--cached", "--recount", "--whitespace=nowarn", "-")
	return err
}

func RestoreWorktreeFile(ctx context.Context, cwd, revision, path string) error {
	if revision == "" || isWorkingTreeRef(revision) {
		return fmt.Errorf("invalid restore revision: %s", revision)
	}
	if path == "" {
		return errors.New("restore path is empty")
	}

	if revision == IndexRef {
		if pathTrackedInIndex(ctx, cwd, path) {
			_, err := runGit(ctx, cwd, "restore", "--worktree", "--", path)
			return err
		}

		targetPath, err := resolveWorktreePath(cwd, path)
		if err != nil {
			return err
		}
		if err := os.Remove(targetPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}

	_, err := runGit(ctx, cwd, "restore", "--worktree", "--source", revision, "--", path)
	return err
}

func RestoreIndexFile(ctx context.Context, cwd, revision, path string) error {
	if revision == "" || isWorkingTreeRef(revision) || isIndexRef(revision) {
		return fmt.Errorf("invalid restore revision for index: %s", revision)
	}
	if path == "" {
		return errors.New("restore path is empty")
	}

	_, err := runGit(ctx, cwd, "restore", "--staged", "--source", revision, "--", path)
	return err
}

func buildEditorInvocation(editor, targetPath string, line int) (string, []string, error) {
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return "", nil, errors.New("editor command is empty")
	}

	command := parts[0]
	args := parts[1:]
	name := filepath.Base(command)

	switch name {
	case "code", "cursor", "codium", "code-insiders":
		location := targetPath
		if line > 0 {
			location = fmt.Sprintf("%s:%d", targetPath, line)
		}
		args = append(args, "-g", location)
	case "vim", "nvim", "vi", "nano":
		if line > 0 {
			args = append(args, "+"+strconv.Itoa(line))
		}
		args = append(args, targetPath)
	default:
		location := targetPath
		if line > 0 && (name == "subl" || name == "mate" || name == "zed") {
			location = fmt.Sprintf("%s:%d", targetPath, line)
		}
		args = append(args, location)
	}

	return command, args, nil
}

func OpenFileInEditor(cwd, path string, line int) (string, error) {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "code"
	}

	targetPath, err := resolveWorktreePath(cwd, path)
	if err != nil {
		return "", err
	}

	command, args, err := buildEditorInvocation(editor, targetPath, line)
	if err != nil {
		return "", err
	}

	cmd := exec.Command(command, args...)
	cmd.Dir = cwd
	if err := cmd.Start(); err != nil {
		return "", err
	}

	return command, nil
}

func Context(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}
