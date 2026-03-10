package gitadapter

import (
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
