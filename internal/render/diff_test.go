package render

import (
	"strings"
	"testing"
)

const sampleDiff = `diff --git a/demo.go b/demo.go
index 1111111..2222222 100644
--- a/demo.go
+++ b/demo.go
@@ -1,5 +1,5 @@
 func greet(name string) string {
-	return "hello " + name
+	return "hello, " + name
 }
`

const longLineDiff = `diff --git a/demo.go b/demo.go
index 1111111..2222222 100644
--- a/demo.go
+++ b/demo.go
@@ -1,3 +1,3 @@
-const message = "this is a deliberately long line for wrapping behavior in the delete column"
+const message = "this is a deliberately long line for wrapping behavior in the add column"
`

func TestParseUnifiedDiff(t *testing.T) {
	parsed := ParseUnifiedDiff(sampleDiff)
	if len(parsed.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(parsed.Files))
	}

	file := parsed.Files[0]
	if got, want := file.NewPath, "demo.go"; got != want {
		t.Fatalf("NewPath = %q, want %q", got, want)
	}
	if len(file.Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(file.Hunks))
	}
	if got, want := file.Hunks[0].Lines[1].Kind, LineDelete; got != want {
		t.Fatalf("delete line kind = %q, want %q", got, want)
	}
	if got, want := file.Hunks[0].Lines[2].Kind, LineAdd; got != want {
		t.Fatalf("add line kind = %q, want %q", got, want)
	}
}

func TestRenderInline(t *testing.T) {
	lines := RenderInline(sampleDiff, 100)
	rendered := strings.Join(lines, "\n")

	if !strings.Contains(rendered, "demo.go") {
		t.Fatalf("rendered output missing file header: %q", rendered)
	}
	if !strings.Contains(rendered, "@@ -1,5 +1,5 @@") {
		t.Fatalf("rendered output missing hunk header: %q", rendered)
	}
	if !strings.Contains(rendered, "return \"hello, \" + name") {
		t.Fatalf("rendered output missing add line content: %q", rendered)
	}
}

func TestRenderSideBySide(t *testing.T) {
	lines := RenderSideBySide(sampleDiff, 120)
	rendered := strings.Join(lines, "\n")

	if !strings.Contains(rendered, "OLD") || !strings.Contains(rendered, "NEW") {
		t.Fatalf("rendered side-by-side output missing column headers: %q", rendered)
	}
	if !strings.Contains(rendered, "return \"hello \" + name") {
		t.Fatalf("rendered side-by-side output missing delete side content: %q", rendered)
	}
	if !strings.Contains(rendered, "return \"hello, \" + name") {
		t.Fatalf("rendered side-by-side output missing add side content: %q", rendered)
	}
}

func TestRenderSideBySideWrapsLongLines(t *testing.T) {
	lines := RenderSideBySide(longLineDiff, 90)
	rendered := strings.Join(lines, "\n")

	if !strings.Contains(rendered, "· ") {
		t.Fatalf("expected wrapped continuation marker in side-by-side output: %q", rendered)
	}
	if len(lines) < 6 {
		t.Fatalf("expected wrapped side-by-side output to expand line count, got %d", len(lines))
	}
}

func TestChangedSpan(t *testing.T) {
	left, right := changedSpan(`return "hello " + name`, `return "hello, " + name`)

	if right.start >= right.end {
		t.Fatalf("expected right emphasis span, got %+v", right)
	}
	if left.start != left.end {
		t.Fatalf("expected insertion-only change to keep left span empty, got %+v", left)
	}
}
