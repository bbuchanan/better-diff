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
