package conflicts

import (
	"strings"
	"testing"
)

const sampleConflict = `package main

func main() {
<<<<<<< ours
	println("ours")
||||||| base
	println("base")
=======
	println("theirs")
>>>>>>> theirs
}
`

func TestParseConflictFile(t *testing.T) {
	parsed := Parse(sampleConflict)
	if len(parsed.Segments) != 3 {
		t.Fatalf("segment count = %d, want 3", len(parsed.Segments))
	}
	if parsed.Segments[1].Block == nil {
		t.Fatal("expected middle segment to be a conflict block")
	}
	block := parsed.Segments[1].Block
	if got, want := block.StartMergedLine, 4; got != want {
		t.Fatalf("StartMergedLine = %d, want %d", got, want)
	}
	if got, want := strings.Join(block.Ours, "\n"), "\tprintln(\"ours\")"; got != want {
		t.Fatalf("ours = %q, want %q", got, want)
	}
	if got, want := strings.Join(block.Theirs, "\n"), "\tprintln(\"theirs\")"; got != want {
		t.Fatalf("theirs = %q, want %q", got, want)
	}
}

func TestRenderResolvedConflictBlock(t *testing.T) {
	parsed := Parse(sampleConflict)
	rendered, resolved := RenderResolved(parsed, 0, "both")
	if !resolved {
		t.Fatal("expected file to be resolved when the only conflict block is accepted")
	}
	if !strings.Contains(rendered, "println(\"ours\")\n\tprintln(\"theirs\")") {
		t.Fatalf("expected both resolution output, got %q", rendered)
	}

	rendered, resolved = RenderResolved(parsed, 0, "ours")
	if !resolved {
		t.Fatal("expected file to be resolved when the only conflict block is accepted")
	}
	if strings.Contains(rendered, "<<<<<<<") || strings.Contains(rendered, ">>>>>>>") {
		t.Fatalf("expected targeted conflict markers to be removed, got %q", rendered)
	}
}
