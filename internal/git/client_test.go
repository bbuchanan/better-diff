package gitadapter

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"better-diff/internal/domain"
)

func smokeRepoPath(t *testing.T) string {
	t.Helper()

	return filepath.Clean(filepath.Join("..", "..", ".tmp-smoke-repo"))
}

func runTestGit(t *testing.T, cwd string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(output))
	}
}

func TestDiscoverRepository(t *testing.T) {
	ctx, cancel := Context(5 * time.Second)
	defer cancel()

	repo, err := DiscoverRepository(ctx, smokeRepoPath(t))
	if err != nil {
		t.Fatalf("DiscoverRepository returned error: %v", err)
	}

	if got, want := repo.HeadRef, "feature"; got != want {
		t.Fatalf("HeadRef = %q, want %q", got, want)
	}

	if repo.DefaultCompareBase == "" {
		t.Fatal("DefaultCompareBase should not be empty")
	}
}

func TestListCommits(t *testing.T) {
	ctx, cancel := Context(5 * time.Second)
	defer cancel()

	commits, err := ListCommits(ctx, smokeRepoPath(t), 10)
	if err != nil {
		t.Fatalf("ListCommits returned error: %v", err)
	}

	if len(commits) < 3 {
		t.Fatalf("expected at least 3 commits, got %d", len(commits))
	}

	head := commits[0]
	if got, want := head.ShortSHA, "dedb968"; got != want {
		t.Fatalf("head.ShortSHA = %q, want %q", got, want)
	}
	if !strings.Contains(head.Subject, "Feature delta") {
		t.Fatalf("head.Subject = %q, want to contain %q", head.Subject, "Feature delta")
	}
	if head.Graph == "" {
		t.Fatal("head.Graph should not be empty")
	}
}

func TestListRefs(t *testing.T) {
	ctx, cancel := Context(5 * time.Second)
	defer cancel()

	refs, err := ListRefs(ctx, smokeRepoPath(t))
	if err != nil {
		t.Fatalf("ListRefs returned error: %v", err)
	}

	if len(refs) < 2 {
		t.Fatalf("expected at least 2 refs, got %d", len(refs))
	}

	foundFeature := false
	foundMaster := false
	for _, ref := range refs {
		if ref.Name == "feature" && ref.Type == "branch" {
			foundFeature = true
		}
		if ref.Name == "master" && ref.Type == "branch" {
			foundMaster = true
		}
	}

	if !foundFeature || !foundMaster {
		t.Fatalf("expected feature and master branches in refs, got %+v", refs)
	}
}

func TestListCommitFilesAndDiff(t *testing.T) {
	ctx, cancel := Context(5 * time.Second)
	defer cancel()

	files, err := ListCommitFiles(ctx, smokeRepoPath(t), "HEAD")
	if err != nil {
		t.Fatalf("ListCommitFiles returned error: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected 1 changed file, got %d", len(files))
	}
	if got, want := files[0].Path, "demo.txt"; got != want {
		t.Fatalf("files[0].Path = %q, want %q", got, want)
	}
	if got, want := files[0].Status, "M"; got != want {
		t.Fatalf("files[0].Status = %q, want %q", got, want)
	}

	diff, err := GetRangeDiff(ctx, smokeRepoPath(t), "HEAD~1", "HEAD", domain.DiffTwoDot, "demo.txt", 3)
	if err != nil {
		t.Fatalf("GetRangeDiff returned error: %v", err)
	}

	if !strings.Contains(diff, "+delta") {
		t.Fatalf("diff = %q, want to contain %q", diff, "+delta")
	}
}

func TestGetBlame(t *testing.T) {
	ctx, cancel := Context(5 * time.Second)
	defer cancel()

	blame, err := GetBlame(ctx, smokeRepoPath(t), "HEAD", "demo.txt")
	if err != nil {
		t.Fatalf("GetBlame returned error: %v", err)
	}

	if len(blame) == 0 {
		t.Fatal("expected blame lines, got none")
	}

	line := blame[1]
	if line.Line != 1 {
		t.Fatalf("line.Line = %d, want 1", line.Line)
	}
	if line.ShortSHA == "" || line.AuthorName == "" || line.Summary == "" {
		t.Fatalf("expected blame metadata, got %+v", line)
	}
}

func TestIgnoreWhitespaceOptions(t *testing.T) {
	repo := t.TempDir()
	runTestGit(t, repo, "init")
	runTestGit(t, repo, "config", "user.name", "Test User")
	runTestGit(t, repo, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(repo, "demo.txt"), []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	runTestGit(t, repo, "add", "demo.txt")
	runTestGit(t, repo, "commit", "-m", "initial")

	if err := os.WriteFile(filepath.Join(repo, "demo.txt"), []byte("alpha \n beta\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	runTestGit(t, repo, "add", "demo.txt")
	runTestGit(t, repo, "commit", "-m", "whitespace only")

	ctx, cancel := Context(5 * time.Second)
	defer cancel()

	files, err := ListCommitFilesWithOptions(ctx, repo, "HEAD", false)
	if err != nil {
		t.Fatalf("ListCommitFilesWithOptions returned error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 changed file without whitespace ignore, got %d", len(files))
	}

	files, err = ListCommitFilesWithOptions(ctx, repo, "HEAD", true)
	if err != nil {
		t.Fatalf("ListCommitFilesWithOptions(ignoreWhitespace) returned error: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected 0 changed files with whitespace ignore, got %d", len(files))
	}

	diff, err := GetRangeDiffWithOptions(ctx, repo, "HEAD~1", "HEAD", domain.DiffTwoDot, "demo.txt", 3, false)
	if err != nil {
		t.Fatalf("GetRangeDiffWithOptions returned error: %v", err)
	}
	if !strings.Contains(diff, "@@") {
		t.Fatalf("expected diff output without whitespace ignore, got %q", diff)
	}

	diff, err = GetRangeDiffWithOptions(ctx, repo, "HEAD~1", "HEAD", domain.DiffTwoDot, "demo.txt", 3, true)
	if err != nil {
		t.Fatalf("GetRangeDiffWithOptions(ignoreWhitespace) returned error: %v", err)
	}
	if strings.TrimSpace(diff) != "" {
		t.Fatalf("expected empty diff with whitespace ignore, got %q", diff)
	}
}

func TestBuildEditorInvocation(t *testing.T) {
	command, args, err := buildEditorInvocation("code --reuse-window", "/tmp/demo.go", 42)
	if err != nil {
		t.Fatalf("buildEditorInvocation(code) returned error: %v", err)
	}
	if got, want := command, "code"; got != want {
		t.Fatalf("command = %q, want %q", got, want)
	}
	if got, want := strings.Join(args, " "), "--reuse-window -g /tmp/demo.go:42"; got != want {
		t.Fatalf("args = %q, want %q", got, want)
	}

	command, args, err = buildEditorInvocation("nvim", "/tmp/demo.go", 17)
	if err != nil {
		t.Fatalf("buildEditorInvocation(nvim) returned error: %v", err)
	}
	if got, want := command, "nvim"; got != want {
		t.Fatalf("command = %q, want %q", got, want)
	}
	if got, want := strings.Join(args, " "), "+17 /tmp/demo.go"; got != want {
		t.Fatalf("args = %q, want %q", got, want)
	}
}

func TestWorkingTreeRangeDiff(t *testing.T) {
	repo := t.TempDir()
	runTestGit(t, repo, "init")
	runTestGit(t, repo, "config", "user.name", "Test User")
	runTestGit(t, repo, "config", "user.email", "test@example.com")

	if err := os.WriteFile(filepath.Join(repo, "demo.txt"), []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	runTestGit(t, repo, "add", "demo.txt")
	runTestGit(t, repo, "commit", "-m", "initial")

	if err := os.WriteFile(filepath.Join(repo, "demo.txt"), []byte("alpha\nbeta\nworktree\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	ctx, cancel := Context(5 * time.Second)
	defer cancel()

	files, err := ListRangeFilesWithOptions(ctx, repo, "HEAD", WorkingTreeRef, domain.DiffTwoDot, false)
	if err != nil {
		t.Fatalf("ListRangeFilesWithOptions(worktree) returned error: %v", err)
	}
	if len(files) != 1 || files[0].Path != "demo.txt" {
		t.Fatalf("unexpected working tree files: %+v", files)
	}

	diff, err := GetRangeDiffWithOptions(ctx, repo, "HEAD", WorkingTreeRef, domain.DiffTwoDot, "demo.txt", 3, false)
	if err != nil {
		t.Fatalf("GetRangeDiffWithOptions(worktree) returned error: %v", err)
	}
	if !strings.Contains(diff, "+worktree") {
		t.Fatalf("expected working tree diff to contain uncommitted line, got %q", diff)
	}
}

func TestLocalChangeLayers(t *testing.T) {
	repo := t.TempDir()
	runTestGit(t, repo, "init")
	runTestGit(t, repo, "config", "user.name", "Test User")
	runTestGit(t, repo, "config", "user.email", "test@example.com")

	path := filepath.Join(repo, "demo.txt")
	untrackedPath := filepath.Join(repo, "notes.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	runTestGit(t, repo, "add", "demo.txt")
	runTestGit(t, repo, "commit", "-m", "initial")

	if err := os.WriteFile(path, []byte("alpha\nBETA\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	runTestGit(t, repo, "add", "demo.txt")
	if err := os.WriteFile(path, []byte("alpha\nBETA\ngamma\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if err := os.WriteFile(untrackedPath, []byte("todo\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	ctx, cancel := Context(5 * time.Second)
	defer cancel()

	stagedFiles, err := ListRangeFilesWithOptions(ctx, repo, "HEAD", IndexRef, domain.DiffTwoDot, false)
	if err != nil {
		t.Fatalf("ListRangeFilesWithOptions(staged) returned error: %v", err)
	}
	if len(stagedFiles) != 1 || stagedFiles[0].Path != "demo.txt" {
		t.Fatalf("unexpected staged files: %+v", stagedFiles)
	}

	stagedDiff, err := GetRangeDiffWithOptions(ctx, repo, "HEAD", IndexRef, domain.DiffTwoDot, "demo.txt", 3, false)
	if err != nil {
		t.Fatalf("GetRangeDiffWithOptions(staged) returned error: %v", err)
	}
	if !strings.Contains(stagedDiff, "-beta") || !strings.Contains(stagedDiff, "+BETA") {
		t.Fatalf("expected staged diff to contain staged line change, got %q", stagedDiff)
	}
	if strings.Contains(stagedDiff, "+gamma") {
		t.Fatalf("staged diff should not contain unstaged line, got %q", stagedDiff)
	}

	indexContent, err := GetFileContent(ctx, repo, IndexRef, "demo.txt")
	if err != nil {
		t.Fatalf("GetFileContent(IndexRef) returned error: %v", err)
	}
	if got, want := indexContent, "alpha\nBETA\n"; got != want {
		t.Fatalf("index content = %q, want %q", got, want)
	}

	unstagedFiles, err := ListRangeFilesWithOptions(ctx, repo, IndexRef, WorkingTreeRef, domain.DiffTwoDot, false)
	if err != nil {
		t.Fatalf("ListRangeFilesWithOptions(unstaged) returned error: %v", err)
	}
	if len(unstagedFiles) != 2 {
		t.Fatalf("expected tracked + untracked unstaged files, got %+v", unstagedFiles)
	}

	unstagedTrackedDiff, err := GetRangeDiffWithOptions(ctx, repo, IndexRef, WorkingTreeRef, domain.DiffTwoDot, "demo.txt", 3, false)
	if err != nil {
		t.Fatalf("GetRangeDiffWithOptions(unstaged tracked) returned error: %v", err)
	}
	if !strings.Contains(unstagedTrackedDiff, "+gamma") {
		t.Fatalf("expected unstaged tracked diff to contain gamma, got %q", unstagedTrackedDiff)
	}
	if strings.Contains(unstagedTrackedDiff, "-beta") || strings.Contains(unstagedTrackedDiff, "+BETA") {
		t.Fatalf("unstaged tracked diff should not contain staged-only change, got %q", unstagedTrackedDiff)
	}

	untrackedDiff, err := GetRangeDiffWithOptions(ctx, repo, IndexRef, WorkingTreeRef, domain.DiffTwoDot, "notes.txt", 3, false)
	if err != nil {
		t.Fatalf("GetRangeDiffWithOptions(unstaged untracked) returned error: %v", err)
	}
	if !strings.Contains(untrackedDiff, "+++ ") || !strings.Contains(untrackedDiff, "+todo") {
		t.Fatalf("expected untracked diff to render as added file, got %q", untrackedDiff)
	}
}

func TestWorkingTreeDeletedFileFlow(t *testing.T) {
	repo := t.TempDir()
	runTestGit(t, repo, "init")
	runTestGit(t, repo, "config", "user.name", "Test User")
	runTestGit(t, repo, "config", "user.email", "test@example.com")

	path := filepath.Join(repo, "demo.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	runTestGit(t, repo, "add", "demo.txt")
	runTestGit(t, repo, "commit", "-m", "initial")

	if err := os.Remove(path); err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}

	ctx, cancel := Context(5 * time.Second)
	defer cancel()

	files, err := ListRangeFilesWithOptions(ctx, repo, "HEAD", WorkingTreeRef, domain.DiffTwoDot, false)
	if err != nil {
		t.Fatalf("ListRangeFilesWithOptions(worktree delete) returned error: %v", err)
	}
	if len(files) != 1 || files[0].Path != "demo.txt" || files[0].Status != "D" {
		t.Fatalf("unexpected deleted file range output: %+v", files)
	}

	diff, err := GetRangeDiffWithOptions(ctx, repo, "HEAD", WorkingTreeRef, domain.DiffTwoDot, "demo.txt", 3, false)
	if err != nil {
		t.Fatalf("GetRangeDiffWithOptions(worktree delete) returned error: %v", err)
	}
	if !strings.Contains(diff, "deleted file mode") {
		t.Fatalf("expected deleted file diff, got %q", diff)
	}

	content, err := GetFileContent(ctx, repo, WorkingTreeRef, "demo.txt")
	if err != nil {
		t.Fatalf("GetFileContent(worktree delete) returned error: %v", err)
	}
	if content != "" {
		t.Fatalf("expected deleted worktree file to read as empty content, got %q", content)
	}

	if err := RestoreWorktreeFile(ctx, repo, "HEAD", "demo.txt"); err != nil {
		t.Fatalf("RestoreWorktreeFile(worktree delete) returned error: %v", err)
	}
	restored, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if got, want := string(restored), "alpha\nbeta\n"; got != want {
		t.Fatalf("restored content = %q, want %q", got, want)
	}
}

func TestWorkingTreeRenameFlow(t *testing.T) {
	repo := t.TempDir()
	runTestGit(t, repo, "init")
	runTestGit(t, repo, "config", "user.name", "Test User")
	runTestGit(t, repo, "config", "user.email", "test@example.com")

	oldPath := filepath.Join(repo, "old.txt")
	newPath := filepath.Join(repo, "new.txt")
	if err := os.WriteFile(oldPath, []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	runTestGit(t, repo, "add", "old.txt")
	runTestGit(t, repo, "commit", "-m", "initial")

	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatalf("Rename returned error: %v", err)
	}
	runTestGit(t, repo, "add", "-A")

	ctx, cancel := Context(5 * time.Second)
	defer cancel()

	files, err := ListRangeFilesWithOptions(ctx, repo, "HEAD", WorkingTreeRef, domain.DiffTwoDot, false)
	if err != nil {
		t.Fatalf("ListRangeFilesWithOptions(worktree rename) returned error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 renamed file, got %+v", files)
	}
	if got, want := files[0].Status, "R"; got != want {
		t.Fatalf("rename status = %q, want %q", got, want)
	}
	if got, want := files[0].OldPath, "old.txt"; got != want {
		t.Fatalf("OldPath = %q, want %q", got, want)
	}
	if got, want := files[0].Path, "new.txt"; got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}

	newContent, err := GetFileContent(ctx, repo, WorkingTreeRef, "new.txt")
	if err != nil {
		t.Fatalf("GetFileContent(new working tree path) returned error: %v", err)
	}
	if got, want := newContent, "alpha\nbeta\n"; got != want {
		t.Fatalf("new content = %q, want %q", got, want)
	}

	oldContent, err := GetFileContent(ctx, repo, "HEAD", "old.txt")
	if err != nil {
		t.Fatalf("GetFileContent(old HEAD path) returned error: %v", err)
	}
	if got, want := oldContent, "alpha\nbeta\n"; got != want {
		t.Fatalf("old content = %q, want %q", got, want)
	}
}

func TestApplyConflictBlockResolution(t *testing.T) {
	repo := t.TempDir()
	runTestGit(t, repo, "init")
	runTestGit(t, repo, "config", "user.name", "Test User")
	runTestGit(t, repo, "config", "user.email", "test@example.com")
	conflictPath := filepath.Join(repo, "demo.txt")
	content := "<<<<<<< ours\nalpha\n=======\nbeta\n>>>>>>> theirs\n"
	if err := os.WriteFile(conflictPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	ctx, cancel := Context(5 * time.Second)
	defer cancel()

	result, err := ApplyConflictBlockResolution(ctx, repo, "demo.txt", 0, "both")
	if err != nil {
		t.Fatalf("ApplyConflictBlockResolution returned error: %v", err)
	}
	if !result.Resolved {
		t.Fatalf("expected conflict file to be resolved, got %+v", result)
	}
	if got, want := result.RemainingBlocks, 0; got != want {
		t.Fatalf("RemainingBlocks = %d, want %d", got, want)
	}

	updated, err := os.ReadFile(conflictPath)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	got := string(updated)
	if strings.Contains(got, "<<<<<<<") || strings.Contains(got, ">>>>>>>") {
		t.Fatalf("expected conflict markers to be removed, got %q", got)
	}
	if !strings.Contains(got, "alpha\nbeta\n") {
		t.Fatalf("expected both sides to remain, got %q", got)
	}
}

func TestApplyConflictBlockResolutionLeavesFileUnstagedWhenConflictsRemain(t *testing.T) {
	repo := t.TempDir()
	conflictPath := filepath.Join(repo, "demo.txt")
	content := "<<<<<<< ours\nalpha\n=======\nbeta\n>>>>>>> theirs\nmiddle\n<<<<<<< ours\ngamma\n=======\ndelta\n>>>>>>> theirs\n"
	if err := os.WriteFile(conflictPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	ctx, cancel := Context(5 * time.Second)
	defer cancel()

	result, err := ApplyConflictBlockResolution(ctx, repo, "demo.txt", 0, "ours")
	if err != nil {
		t.Fatalf("ApplyConflictBlockResolution returned error: %v", err)
	}
	if result.Resolved {
		t.Fatalf("expected file to remain unresolved, got %+v", result)
	}
	if got, want := result.RemainingBlocks, 1; got != want {
		t.Fatalf("RemainingBlocks = %d, want %d", got, want)
	}

	updated, err := os.ReadFile(conflictPath)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	got := string(updated)
	if !strings.Contains(got, "<<<<<<< ours") {
		t.Fatalf("expected remaining conflict markers, got %q", got)
	}
	if !strings.Contains(got, "alpha\nmiddle\n<<<<<<< ours\ngamma") {
		t.Fatalf("expected first block to resolve to ours, got %q", got)
	}
}

func TestApplyReversePatch(t *testing.T) {
	repo := t.TempDir()
	runTestGit(t, repo, "init")
	runTestGit(t, repo, "config", "user.name", "Test User")
	runTestGit(t, repo, "config", "user.email", "test@example.com")

	path := filepath.Join(repo, "demo.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	runTestGit(t, repo, "add", "demo.txt")
	runTestGit(t, repo, "commit", "-m", "initial")

	if err := os.WriteFile(path, []byte("alpha\nBETA\ngamma\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	ctx, cancel := Context(5 * time.Second)
	defer cancel()

	diff, err := GetRangeDiffWithOptions(ctx, repo, "HEAD", WorkingTreeRef, domain.DiffTwoDot, "demo.txt", 3, false)
	if err != nil {
		t.Fatalf("GetRangeDiffWithOptions returned error: %v", err)
	}

	if err := ApplyReversePatch(ctx, repo, diff); err != nil {
		t.Fatalf("ApplyReversePatch returned error: %v", err)
	}

	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if got, want := string(updated), "alpha\nbeta\ngamma\n"; got != want {
		t.Fatalf("updated file = %q, want %q", got, want)
	}
}

func TestApplyReversePatchCachedAndRestoreIndexFile(t *testing.T) {
	repo := t.TempDir()
	runTestGit(t, repo, "init")
	runTestGit(t, repo, "config", "user.name", "Test User")
	runTestGit(t, repo, "config", "user.email", "test@example.com")

	path := filepath.Join(repo, "demo.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	runTestGit(t, repo, "add", "demo.txt")
	runTestGit(t, repo, "commit", "-m", "initial")

	if err := os.WriteFile(path, []byte("alpha\nBETA\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	runTestGit(t, repo, "add", "demo.txt")

	ctx, cancel := Context(5 * time.Second)
	defer cancel()

	diff, err := GetRangeDiffWithOptions(ctx, repo, "HEAD", IndexRef, domain.DiffTwoDot, "demo.txt", 3, false)
	if err != nil {
		t.Fatalf("GetRangeDiffWithOptions(staged) returned error: %v", err)
	}
	if err := ApplyReversePatchCached(ctx, repo, diff); err != nil {
		t.Fatalf("ApplyReversePatchCached returned error: %v", err)
	}

	indexContent, err := GetFileContent(ctx, repo, IndexRef, "demo.txt")
	if err != nil {
		t.Fatalf("GetFileContent(IndexRef) returned error: %v", err)
	}
	if got, want := indexContent, "alpha\nbeta\n"; got != want {
		t.Fatalf("index content after cached reverse = %q, want %q", got, want)
	}

	if err := os.WriteFile(path, []byte("alpha\nBETA\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	runTestGit(t, repo, "add", "demo.txt")
	if err := RestoreIndexFile(ctx, repo, "HEAD", "demo.txt"); err != nil {
		t.Fatalf("RestoreIndexFile returned error: %v", err)
	}

	indexContent, err = GetFileContent(ctx, repo, IndexRef, "demo.txt")
	if err != nil {
		t.Fatalf("GetFileContent(IndexRef) returned error: %v", err)
	}
	if got, want := indexContent, "alpha\nbeta\n"; got != want {
		t.Fatalf("index content after restore = %q, want %q", got, want)
	}
	worktreeContent, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if got, want := string(worktreeContent), "alpha\nBETA\n"; got != want {
		t.Fatalf("worktree content after index restore = %q, want %q", got, want)
	}
}

func TestRestoreWorktreeFile(t *testing.T) {
	repo := t.TempDir()
	runTestGit(t, repo, "init")
	runTestGit(t, repo, "config", "user.name", "Test User")
	runTestGit(t, repo, "config", "user.email", "test@example.com")

	path := filepath.Join(repo, "demo.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	runTestGit(t, repo, "add", "demo.txt")
	runTestGit(t, repo, "commit", "-m", "initial")

	if err := os.WriteFile(path, []byte("alpha\nBETA\ngamma\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	ctx, cancel := Context(5 * time.Second)
	defer cancel()

	if err := RestoreWorktreeFile(ctx, repo, "HEAD", "demo.txt"); err != nil {
		t.Fatalf("RestoreWorktreeFile returned error: %v", err)
	}

	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if got, want := string(updated), "alpha\nbeta\ngamma\n"; got != want {
		t.Fatalf("updated file = %q, want %q", got, want)
	}
}

func TestResolveWorktreePathRejectsTraversal(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "tmp", "repo")

	if _, err := resolveWorktreePath(root, filepath.Join("..", "repo-evil", "demo.txt")); err == nil {
		t.Fatal("expected sibling-path traversal to be rejected")
	}
	if _, err := resolveWorktreePath(root, filepath.Join("nested", "..", "..", "escape.txt")); err == nil {
		t.Fatal("expected parent traversal to be rejected")
	}

	target, err := resolveWorktreePath(root, filepath.Join("nested", "..", "demo.txt"))
	if err != nil {
		t.Fatalf("resolveWorktreePath(valid) returned error: %v", err)
	}
	if got, want := target, filepath.Join(root, "demo.txt"); got != want {
		t.Fatalf("target = %q, want %q", got, want)
	}
}
