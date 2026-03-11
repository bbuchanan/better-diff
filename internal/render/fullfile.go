package render

import "strings"

type FullFileCompare struct {
	LeftLabel        string
	RightLabel       string
	LeftPath         string
	RightPath        string
	LeftText         string
	RightText        string
	IgnoreWhitespace bool
}

type fullFileLineOp struct {
	kind    LineKind
	text    string
	oldLine int
	newLine int
}

func BuildFullFileDocument(compare FullFileCompare, width int) Document {
	if width < 72 {
		width = 72
	}

	columnWidth := maxInt(28, (width-3)/2)
	language := detectLanguage(compare.RightPath, compare.LeftPath)
	rows := alignFullFileRows(compare.LeftText, compare.RightText, language, compare.IgnoreWhitespace)

	lines := make([]string, 0, len(rows)+6)
	rowMeta := make([]RowMeta, 0, len(rows)+6)
	hunkRows := []int{}

	title := compare.RightPath
	if title == "" {
		title = compare.LeftPath
	}
	if title == "" {
		title = "full-file compare"
	}

	lines = append(lines, trimStyled(styleFileHeader.Width(width).Render(trimPlain(" "+title+"  [full-file] ", width)), width))
	rowMeta = append(rowMeta, RowMeta{})

	leftMeta := compare.LeftLabel
	if leftMeta == "" {
		leftMeta = "left"
	}
	if compare.LeftPath != "" {
		leftMeta += "  " + compare.LeftPath
	}
	rightMeta := compare.RightLabel
	if rightMeta == "" {
		rightMeta = "right"
	}
	if compare.RightPath != "" {
		rightMeta += "  " + compare.RightPath
	}

	lines = append(lines, joinColumns(
		trimStyled(styleOldPath.Width(columnWidth).Render(trimPlain(" "+leftMeta+" ", columnWidth)), columnWidth),
		trimStyled(styleNewPath.Width(columnWidth).Render(trimPlain(" "+rightMeta+" ", columnWidth)), columnWidth),
	))
	rowMeta = append(rowMeta, RowMeta{})

	leftHeader := trimStyled(styleSplitHeader.Width(columnWidth).Render(" LEFT "), columnWidth)
	rightHeader := trimStyled(styleSplitHeader.Width(columnWidth).Render(" RIGHT "), columnWidth)
	lines = append(lines, joinColumns(leftHeader, rightHeader))
	rowMeta = append(rowMeta, RowMeta{})

	previousKind := LineMeta
	for _, row := range rows {
		meta := rowMetaForSideBySideRow(row)
		if meta.Kind != LineContext && meta.Kind != LineMeta && (previousKind == LineContext || previousKind == LineMeta) {
			hunkRows = append(hunkRows, len(lines))
		}
		rendered := renderSideBySideRowLines(row, columnWidth)
		lines = append(lines, rendered...)
		for index := range rendered {
			meta.Continuation = index > 0
			rowMeta = append(rowMeta, meta)
		}
		previousKind = meta.Kind
	}

	return Document{
		Rows:     lines,
		RowMeta:  rowMeta,
		HunkRows: hunkRows,
	}
}

func alignFullFileRows(leftText, rightText, language string, ignoreWhitespace bool) []sideBySideRow {
	leftLines := splitFullFileLines(leftText)
	rightLines := splitFullFileLines(rightText)
	ops := diffFullFileLines(leftLines, rightLines, ignoreWhitespace)

	rows := make([]sideBySideRow, 0, len(ops))
	for index := 0; index < len(ops); {
		if ops[index].kind == LineContext {
			leftPlan := renderPlan{
				kind:     LineContext,
				oldLine:  ops[index].oldLine,
				newLine:  ops[index].newLine,
				text:     ops[index].text,
				language: language,
			}
			rightPlan := leftPlan
			rows = append(rows, sideBySideRow{left: &leftPlan, right: &rightPlan})
			index++
			continue
		}

		deleteStart := index
		for index < len(ops) && ops[index].kind == LineDelete {
			index++
		}
		addStart := index
		for index < len(ops) && ops[index].kind == LineAdd {
			index++
		}

		deletes := ops[deleteStart:addStart]
		adds := ops[addStart:index]
		pairCount := maxInt(len(deletes), len(adds))
		for pairIndex := 0; pairIndex < pairCount; pairIndex++ {
			var leftPlan *renderPlan
			var rightPlan *renderPlan

			if pairIndex < len(deletes) && pairIndex < len(adds) {
				leftSpan, rightSpan := changedSpan(deletes[pairIndex].text, adds[pairIndex].text)
				leftPlan = &renderPlan{
					kind:     LineDelete,
					oldLine:  deletes[pairIndex].oldLine,
					text:     deletes[pairIndex].text,
					emphasis: leftSpan,
					language: language,
				}
				rightPlan = &renderPlan{
					kind:     LineAdd,
					newLine:  adds[pairIndex].newLine,
					text:     adds[pairIndex].text,
					emphasis: rightSpan,
					language: language,
				}
			} else {
				if pairIndex < len(deletes) {
					leftPlan = &renderPlan{
						kind:     LineDelete,
						oldLine:  deletes[pairIndex].oldLine,
						text:     deletes[pairIndex].text,
						language: language,
					}
				}
				if pairIndex < len(adds) {
					rightPlan = &renderPlan{
						kind:     LineAdd,
						newLine:  adds[pairIndex].newLine,
						text:     adds[pairIndex].text,
						language: language,
					}
				}
			}

			rows = append(rows, sideBySideRow{left: leftPlan, right: rightPlan})
		}
	}

	return rows
}

func splitFullFileLines(value string) []string {
	value = strings.ReplaceAll(value, "\r", "")
	if value == "" {
		return nil
	}
	lines := strings.Split(value, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func diffFullFileLines(leftLines, rightLines []string, ignoreWhitespace bool) []fullFileLineOp {
	if len(leftLines) == 0 && len(rightLines) == 0 {
		return nil
	}

	if len(leftLines) > 2500 || len(rightLines) > 2500 || len(leftLines)*len(rightLines) > 500_000 {
		return diffFullFileLinesFallback(leftLines, rightLines, ignoreWhitespace)
	}

	width := len(rightLines) + 1
	lcs := make([]int, (len(leftLines)+1)*width)
	for left := len(leftLines) - 1; left >= 0; left-- {
		for right := len(rightLines) - 1; right >= 0; right-- {
			index := left*width + right
			if fullFileLinesEqual(leftLines[left], rightLines[right], ignoreWhitespace) {
				lcs[index] = lcs[(left+1)*width+right+1] + 1
				continue
			}
			down := lcs[(left+1)*width+right]
			across := lcs[left*width+right+1]
			if down >= across {
				lcs[index] = down
			} else {
				lcs[index] = across
			}
		}
	}

	ops := make([]fullFileLineOp, 0, len(leftLines)+len(rightLines))
	left := 0
	right := 0
	for left < len(leftLines) && right < len(rightLines) {
		if fullFileLinesEqual(leftLines[left], rightLines[right], ignoreWhitespace) {
			ops = append(ops, fullFileLineOp{
				kind:    LineContext,
				text:    leftLines[left],
				oldLine: left + 1,
				newLine: right + 1,
			})
			left++
			right++
			continue
		}

		if lcs[(left+1)*width+right] >= lcs[left*width+right+1] {
			ops = append(ops, fullFileLineOp{
				kind:    LineDelete,
				text:    leftLines[left],
				oldLine: left + 1,
			})
			left++
			continue
		}

		ops = append(ops, fullFileLineOp{
			kind:    LineAdd,
			text:    rightLines[right],
			newLine: right + 1,
		})
		right++
	}

	for left < len(leftLines) {
		ops = append(ops, fullFileLineOp{
			kind:    LineDelete,
			text:    leftLines[left],
			oldLine: left + 1,
		})
		left++
	}
	for right < len(rightLines) {
		ops = append(ops, fullFileLineOp{
			kind:    LineAdd,
			text:    rightLines[right],
			newLine: right + 1,
		})
		right++
	}

	return ops
}

func diffFullFileLinesFallback(leftLines, rightLines []string, ignoreWhitespace bool) []fullFileLineOp {
	ops := make([]fullFileLineOp, 0, maxInt(len(leftLines), len(rightLines)))
	lineCount := maxInt(len(leftLines), len(rightLines))
	for index := 0; index < lineCount; index++ {
		leftText := ""
		rightText := ""
		if index < len(leftLines) {
			leftText = leftLines[index]
		}
		if index < len(rightLines) {
			rightText = rightLines[index]
		}

		switch {
		case index < len(leftLines) && index < len(rightLines) && fullFileLinesEqual(leftText, rightText, ignoreWhitespace):
			ops = append(ops, fullFileLineOp{
				kind:    LineContext,
				text:    leftText,
				oldLine: index + 1,
				newLine: index + 1,
			})
		default:
			if index < len(leftLines) {
				ops = append(ops, fullFileLineOp{
					kind:    LineDelete,
					text:    leftText,
					oldLine: index + 1,
				})
			}
			if index < len(rightLines) {
				ops = append(ops, fullFileLineOp{
					kind:    LineAdd,
					text:    rightText,
					newLine: index + 1,
				})
			}
		}
	}
	return ops
}

func fullFileLinesEqual(left, right string, ignoreWhitespace bool) bool {
	if !ignoreWhitespace {
		return left == right
	}
	return strings.Join(strings.Fields(left), " ") == strings.Join(strings.Fields(right), " ")
}
