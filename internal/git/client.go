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
	"strings"
	"time"

	"better-diff/internal/domain"
)

const (
	recordSeparator = '\x1e'
	fieldSeparator  = '\x1f'
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
	raw, err := runGit(ctx, cwd, "show", "--no-ext-diff", "--format=", "--name-status", "--find-renames", sha)
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

func ListRangeFiles(ctx context.Context, cwd, left, right string, style domain.DiffStyle) ([]domain.FileChange, error) {
	raw, err := runGit(ctx, cwd, "diff", "--no-ext-diff", "--name-status", "--find-renames", diffSpecifier(left, right, style))
	if err != nil {
		return nil, err
	}

	return parseNameStatusOutput(raw), nil
}

func GetCommitDiff(ctx context.Context, cwd, sha, path string, contextLines int) (string, error) {
	args := []string{"show", "--no-ext-diff", fmt.Sprintf("--unified=%d", contextLines), "--format=", sha}
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
	args := []string{"diff", "--no-ext-diff", fmt.Sprintf("--unified=%d", contextLines), diffSpecifier(left, right, style)}
	if path != "" {
		args = append(args, "--", path)
	}

	raw, err := runGit(ctx, cwd, args...)
	if err != nil {
		return "", err
	}

	return strings.TrimRight(raw, "\n"), nil
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
	target := filepath.Clean(filepath.Join(root, relative))
	root = filepath.Clean(root)
	if !strings.HasPrefix(target, root) {
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

func OpenFileInEditor(cwd, path string) (string, error) {
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

	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return "", errors.New("editor command is empty")
	}

	command := parts[0]
	args := parts[1:]
	if command == "code" || command == "cursor" || command == "codium" || command == "code-insiders" {
		args = append(args, "-g", targetPath)
	} else {
		args = append(args, targetPath)
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
