package conflicts

import "strings"

type Parsed struct {
	Segments           []Segment
	HasTrailingNewline bool
}

type Segment struct {
	Context []string
	Block   *Block
}

type Block struct {
	Index           int
	StartMergedLine int
	Ours            []string
	Base            []string
	Theirs          []string
}

func CountBlocks(parsed Parsed) int {
	count := 0
	for _, segment := range parsed.Segments {
		if segment.Block != nil {
			count++
		}
	}
	return count
}

func Parse(content string) Parsed {
	content = strings.ReplaceAll(content, "\r", "")
	hasTrailingNewline := strings.HasSuffix(content, "\n")
	lines := strings.Split(content, "\n")
	if hasTrailingNewline && len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	segments := make([]Segment, 0, 8)
	context := make([]string, 0, len(lines))
	blockIndex := 0
	mergedLine := 1

	flushContext := func() {
		if len(context) == 0 {
			return
		}
		copyLines := append([]string{}, context...)
		segments = append(segments, Segment{Context: copyLines})
		context = context[:0]
	}

	for index := 0; index < len(lines); {
		line := lines[index]
		if !strings.HasPrefix(line, "<<<<<<< ") {
			context = append(context, line)
			index++
			mergedLine++
			continue
		}

		flushContext()
		startLine := mergedLine
		index++
		mergedLine++

		ours := []string{}
		for index < len(lines) && !strings.HasPrefix(lines[index], "||||||| ") && lines[index] != "=======" {
			ours = append(ours, lines[index])
			index++
			mergedLine++
		}

		base := []string{}
		if index < len(lines) && strings.HasPrefix(lines[index], "||||||| ") {
			index++
			mergedLine++
			for index < len(lines) && lines[index] != "=======" {
				base = append(base, lines[index])
				index++
				mergedLine++
			}
		}

		if index < len(lines) && lines[index] == "=======" {
			index++
			mergedLine++
		}

		theirs := []string{}
		for index < len(lines) && !strings.HasPrefix(lines[index], ">>>>>>> ") {
			theirs = append(theirs, lines[index])
			index++
			mergedLine++
		}

		if index < len(lines) && strings.HasPrefix(lines[index], ">>>>>>> ") {
			index++
			mergedLine++
		}

		segments = append(segments, Segment{
			Block: &Block{
				Index:           blockIndex,
				StartMergedLine: startLine,
				Ours:            append([]string{}, ours...),
				Base:            append([]string{}, base...),
				Theirs:          append([]string{}, theirs...),
			},
		})
		blockIndex++
	}

	flushContext()

	return Parsed{
		Segments:           segments,
		HasTrailingNewline: hasTrailingNewline,
	}
}

func RenderResolved(parsed Parsed, blockIndex int, resolution string) (string, bool) {
	lines := make([]string, 0, 64)
	hasConflicts := false

	for _, segment := range parsed.Segments {
		if segment.Block == nil {
			lines = append(lines, segment.Context...)
			continue
		}

		block := segment.Block
		if block.Index == blockIndex {
			switch resolution {
			case "ours":
				lines = append(lines, block.Ours...)
			case "theirs":
				lines = append(lines, block.Theirs...)
			case "both":
				lines = append(lines, block.Ours...)
				lines = append(lines, block.Theirs...)
			default:
				lines = append(lines, renderConflictBlock(*block)...)
				hasConflicts = true
			}
			continue
		}

		lines = append(lines, renderConflictBlock(*block)...)
		hasConflicts = true
	}

	content := strings.Join(lines, "\n")
	if parsed.HasTrailingNewline {
		content += "\n"
	}
	return content, !hasConflicts
}

func renderConflictBlock(block Block) []string {
	lines := []string{"<<<<<<< ours"}
	lines = append(lines, block.Ours...)
	if len(block.Base) > 0 {
		lines = append(lines, "||||||| base")
		lines = append(lines, block.Base...)
	}
	lines = append(lines, "=======")
	lines = append(lines, block.Theirs...)
	lines = append(lines, ">>>>>>> theirs")
	return lines
}
