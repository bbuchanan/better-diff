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

const multiHunkDiff = `diff --git a/demo.go b/demo.go
index 1111111..2222222 100644
--- a/demo.go
+++ b/demo.go
@@ -1,3 +1,3 @@
-func alpha() string {
+func alpha() int {
 	return 1
 }
@@ -10,3 +10,3 @@
-func beta() string {
+func beta() int {
 	return 2
 }
`

const fullFileLeft = `package main

func alpha() string {
	return "alpha"
}

func gamma() string {
	return "gamma"
}
`

const fullFileRight = `package main

func alpha() int {
	return 1
}

func beta() string {
	return "beta"
}
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

func TestBuildDocumentsTrackHunkRows(t *testing.T) {
	inline := BuildInlineDocument(multiHunkDiff, 100)
	if got, want := len(inline.HunkRows), 2; got != want {
		t.Fatalf("inline HunkRows = %d, want %d", got, want)
	}
	if len(inline.RowMeta) != len(inline.Rows) {
		t.Fatalf("inline RowMeta length = %d, want %d", len(inline.RowMeta), len(inline.Rows))
	}
	if inline.HunkRows[1] <= inline.HunkRows[0] {
		t.Fatalf("expected inline hunk rows to increase, got %+v", inline.HunkRows)
	}

	split := BuildSideBySideDocument(multiHunkDiff, 120)
	if got, want := len(split.HunkRows), 2; got != want {
		t.Fatalf("split HunkRows = %d, want %d", got, want)
	}
	if len(split.RowMeta) != len(split.Rows) {
		t.Fatalf("split RowMeta length = %d, want %d", len(split.RowMeta), len(split.Rows))
	}
	if split.HunkRows[1] <= split.HunkRows[0] {
		t.Fatalf("expected split hunk rows to increase, got %+v", split.HunkRows)
	}
}

func TestBuildFullFileDocument(t *testing.T) {
	document := BuildFullFileDocument(FullFileCompare{
		LeftLabel:  "HEAD",
		RightLabel: "Working Tree",
		LeftPath:   "demo.go",
		RightPath:  "demo.go",
		LeftText:   fullFileLeft,
		RightText:  fullFileRight,
	}, 120)

	rendered := strings.Join(document.Rows, "\n")
	if !strings.Contains(rendered, "[full-file]") {
		t.Fatalf("expected full-file header, got %q", rendered)
	}
	if !strings.Contains(rendered, "func gamma() string") {
		t.Fatalf("expected left-side full-file content, got %q", rendered)
	}
	if !strings.Contains(rendered, "func beta() string") {
		t.Fatalf("expected right-side full-file content, got %q", rendered)
	}
	if len(document.HunkRows) == 0 {
		t.Fatalf("expected full-file document to track change blocks")
	}
	if len(document.RowMeta) != len(document.Rows) {
		t.Fatalf("row meta length = %d, want %d", len(document.RowMeta), len(document.Rows))
	}
}

func TestBuildConflictDocument(t *testing.T) {
	document := BuildConflictDocument("demo.go", `package main

<<<<<<< ours
func alpha() string {
	return "ours"
=======
func alpha() int {
	return 1
>>>>>>> theirs
`, 120)

	rendered := strings.Join(document.Rows, "\n")
	if !strings.Contains(rendered, "[conflict]") {
		t.Fatalf("expected conflict header, got %q", rendered)
	}
	if len(document.HunkRows) != 1 {
		t.Fatalf("expected 1 conflict block row marker, got %d", len(document.HunkRows))
	}
	foundConflict := false
	for _, meta := range document.RowMeta {
		if meta.Conflict {
			foundConflict = true
			break
		}
	}
	if !foundConflict {
		t.Fatal("expected conflict row metadata")
	}
	if strings.Contains(rendered, "base hidden") {
		t.Fatalf("expected conflict renderer to omit repeated base-hidden row, got %q", rendered)
	}

	var firstConflictLine RowMeta
	foundFirstConflictLine := false
	for _, meta := range document.RowMeta {
		if meta.Conflict && meta.Kind == LineAdd {
			firstConflictLine = meta
			foundFirstConflictLine = true
			break
		}
	}
	if !foundFirstConflictLine {
		t.Fatal("expected a conflict content row")
	}
	if got, want := firstConflictLine.OldLine, 4; got != want {
		t.Fatalf("first conflict old line = %d, want %d", got, want)
	}
	if got, want := firstConflictLine.NewLine, 7; got != want {
		t.Fatalf("first conflict new line = %d, want %d", got, want)
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

func TestWrapPlainTextPrefersCodeBoundaries(t *testing.T) {
	chunks := wrapPlainText(`return fmt.Sprintf("Keys: tab/h/l switch panes, j/k move in focused pane")`, 20)
	if len(chunks) < 2 {
		t.Fatalf("expected wrapped chunks, got %+v", chunks)
	}

	first := chunks[0].text
	if strings.HasSuffix(first, "pan") || strings.HasSuffix(first, "swit") {
		t.Fatalf("expected first chunk to avoid mid-token split, got %q", first)
	}
	if !strings.HasSuffix(first, " ") && !strings.HasSuffix(first, "(") && !strings.HasSuffix(first, ":") {
		t.Fatalf("expected first chunk to end on a natural boundary, got %q", first)
	}
}

func TestWrapPlainTextHardWrapsLongToken(t *testing.T) {
	chunks := wrapPlainText("supercalifragilisticexpialidocious", 10)
	if got, want := len(chunks), 4; got != want {
		t.Fatalf("chunk count = %d, want %d", got, want)
	}
	if chunks[0].text != "supercalif" {
		t.Fatalf("unexpected first chunk: %q", chunks[0].text)
	}
}
