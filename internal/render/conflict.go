package render

import (
	"fmt"

	"better-diff/internal/conflicts"
)

func BuildConflictDocument(path, merged string, width int) Document {
	if width < 72 {
		width = 72
	}

	parsed := conflicts.Parse(merged)
	columnWidth := maxInt(28, (width-3)/2)
	language := detectLanguage(path, path)

	lines := make([]string, 0, 64)
	rowMeta := make([]RowMeta, 0, 64)
	hunkRows := []int{}

	title := path
	if title == "" {
		title = "merge conflict"
	}
	lines = append(lines, trimStyled(styleFileHeader.Width(width).Render(trimPlain(" "+title+"  [conflict] ", width)), width))
	rowMeta = append(rowMeta, RowMeta{})

	leftHeader := trimStyled(styleOldPath.Width(columnWidth).Render(" OURS "), columnWidth)
	rightHeader := trimStyled(styleNewPath.Width(columnWidth).Render(" THEIRS "), columnWidth)
	lines = append(lines, joinColumns(leftHeader, rightHeader))
	rowMeta = append(rowMeta, RowMeta{})

	mergedLine := 1
	for _, segment := range parsed.Segments {
		if segment.Block == nil {
			for _, line := range segment.Context {
				plan := renderPlan{
					kind:     LineContext,
					oldLine:  mergedLine,
					newLine:  mergedLine,
					text:     line,
					language: language,
				}
				row := sideBySideRow{left: &plan, right: &plan}
				rendered := renderSideBySideRowLines(row, columnWidth)
				lines = append(lines, rendered...)
				meta := rowMetaForSideBySideRow(row)
				for rowIndex := range rendered {
					meta.Continuation = rowIndex > 0
					rowMeta = append(rowMeta, meta)
				}
				mergedLine++
			}
			continue
		}

		block := segment.Block
		blockStart := mergedLine
		oursStart := blockStart + 1
		currentLine := oursStart + len(block.Ours)
		if len(block.Base) > 0 {
			currentLine++
			currentLine += len(block.Base)
		}
		theirsStart := currentLine + 1
		mergedLine = theirsStart + len(block.Theirs) + 1

		hunkRows = append(hunkRows, len(lines))
		header := fmt.Sprintf(" Conflict %d ", block.Index+1)
		lines = append(lines, trimStyled(styleHunkHeader.Width(width).Render(trimPlain(header, width)), width))
		rowMeta = append(rowMeta, RowMeta{Kind: LineMeta, OldLine: blockStart, NewLine: blockStart, Conflict: true, ConflictIndex: block.Index})

		pairCount := maxInt(len(block.Ours), len(block.Theirs))
		for pairIndex := 0; pairIndex < pairCount; pairIndex++ {
			var leftPlan *renderPlan
			var rightPlan *renderPlan

			if pairIndex < len(block.Ours) && pairIndex < len(block.Theirs) {
				leftSpan, rightSpan := changedSpan(block.Ours[pairIndex], block.Theirs[pairIndex])
				leftPlan = &renderPlan{
					kind:     LineDelete,
					oldLine:  oursStart + pairIndex,
					text:     block.Ours[pairIndex],
					emphasis: leftSpan,
					language: language,
				}
				rightPlan = &renderPlan{
					kind:     LineAdd,
					newLine:  theirsStart + pairIndex,
					text:     block.Theirs[pairIndex],
					emphasis: rightSpan,
					language: language,
				}
			} else {
				if pairIndex < len(block.Ours) {
					leftPlan = &renderPlan{
						kind:     LineDelete,
						oldLine:  oursStart + pairIndex,
						text:     block.Ours[pairIndex],
						language: language,
					}
				}
				if pairIndex < len(block.Theirs) {
					rightPlan = &renderPlan{
						kind:     LineAdd,
						newLine:  theirsStart + pairIndex,
						text:     block.Theirs[pairIndex],
						language: language,
					}
				}
			}

			rendered := renderSideBySideRowLines(sideBySideRow{left: leftPlan, right: rightPlan}, columnWidth)
			lines = append(lines, rendered...)
			meta := rowMetaForSideBySideRow(sideBySideRow{left: leftPlan, right: rightPlan})
			meta.Conflict = true
			meta.ConflictIndex = block.Index
			for rowIndex := range rendered {
				meta.Continuation = rowIndex > 0
				rowMeta = append(rowMeta, meta)
			}
		}
	}

	return Document{
		Rows:     lines,
		RowMeta:  rowMeta,
		HunkRows: hunkRows,
	}
}
