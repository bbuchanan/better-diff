package render

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type LineKind string

const (
	LineContext LineKind = "context"
	LineAdd     LineKind = "add"
	LineDelete  LineKind = "delete"
	LineMeta    LineKind = "meta"
)

type Diff struct {
	Files []File
}

type File struct {
	OldPath string
	NewPath string
	Headers []string
	Hunks   []Hunk
}

type Hunk struct {
	Header string
	Lines  []Line
}

type Line struct {
	Kind    LineKind
	Raw     string
	Text    string
	OldLine int
	NewLine int
}

type renderPlan struct {
	kind     LineKind
	oldLine  int
	newLine  int
	text     string
	emphasis span
	language string
}

type span struct {
	start int
	end   int
}

type token struct {
	text  string
	style lipgloss.Style
}

type sideBySideRow struct {
	left  *renderPlan
	right *renderPlan
}

type wrappedChunk struct {
	text  string
	start int
	end   int
}

var highlightPattern = regexp.MustCompile(`(".*?"|'.*?'|` + "`.*?`" + `|//.*$|#.*$|\b\d+\b|\b[A-Za-z_][A-Za-z0-9_]*\b)`)

var keywordStyles = map[string]lipgloss.Style{
	"func":      lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true),
	"return":    lipgloss.NewStyle().Foreground(lipgloss.Color("81")),
	"type":      lipgloss.NewStyle().Foreground(lipgloss.Color("75")),
	"struct":    lipgloss.NewStyle().Foreground(lipgloss.Color("75")),
	"interface": lipgloss.NewStyle().Foreground(lipgloss.Color("75")),
	"const":     lipgloss.NewStyle().Foreground(lipgloss.Color("117")),
	"let":       lipgloss.NewStyle().Foreground(lipgloss.Color("117")),
	"var":       lipgloss.NewStyle().Foreground(lipgloss.Color("117")),
	"if":        lipgloss.NewStyle().Foreground(lipgloss.Color("215")),
	"else":      lipgloss.NewStyle().Foreground(lipgloss.Color("215")),
	"for":       lipgloss.NewStyle().Foreground(lipgloss.Color("215")),
	"range":     lipgloss.NewStyle().Foreground(lipgloss.Color("215")),
	"switch":    lipgloss.NewStyle().Foreground(lipgloss.Color("215")),
	"case":      lipgloss.NewStyle().Foreground(lipgloss.Color("215")),
	"import":    lipgloss.NewStyle().Foreground(lipgloss.Color("117")),
	"package":   lipgloss.NewStyle().Foreground(lipgloss.Color("117")),
	"from":      lipgloss.NewStyle().Foreground(lipgloss.Color("117")),
	"class":     lipgloss.NewStyle().Foreground(lipgloss.Color("75")),
	"true":      lipgloss.NewStyle().Foreground(lipgloss.Color("186")),
	"false":     lipgloss.NewStyle().Foreground(lipgloss.Color("186")),
	"nil":       lipgloss.NewStyle().Foreground(lipgloss.Color("186")),
	"null":      lipgloss.NewStyle().Foreground(lipgloss.Color("186")),
}

var (
	styleFileHeader = lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("24")).
			Bold(true).
			Padding(0, 1)
	styleFileMeta = lipgloss.NewStyle().
			Foreground(lipgloss.Color("110")).
			Background(lipgloss.Color("235"))
	styleOldPath = lipgloss.NewStyle().
			Foreground(lipgloss.Color("224")).
			Background(lipgloss.Color("52")).
			Bold(true).
			Padding(0, 1)
	styleNewPath = lipgloss.NewStyle().
			Foreground(lipgloss.Color("194")).
			Background(lipgloss.Color("22")).
			Bold(true).
			Padding(0, 1)
	styleHunkHeader = lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("60")).
			Bold(true).
			Padding(0, 1)
	styleSplitHeader = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				Background(lipgloss.Color("238")).
				Bold(true).
				Padding(0, 1)
	styleContextLine = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	styleMetaLine    = lipgloss.NewStyle().Foreground(lipgloss.Color("110"))
	styleAddLine     = lipgloss.NewStyle().Foreground(lipgloss.Color("194")).Background(lipgloss.Color("22"))
	styleDelLine     = lipgloss.NewStyle().Foreground(lipgloss.Color("224")).Background(lipgloss.Color("52"))
	styleAddFocus    = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("28")).Bold(true)
	styleDelFocus    = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("88")).Bold(true)
	styleLineNumber  = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	styleGutterBar   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleColumnGap   = lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Render(" │ ")
)

func ParseUnifiedDiff(diffText string) Diff {
	if strings.TrimSpace(diffText) == "" {
		return Diff{}
	}

	lines := strings.Split(diffText, "\n")
	result := Diff{}
	var currentFile *File
	var currentHunk *Hunk
	oldLine := 0
	newLine := 0

	ensureFile := func() *File {
		if currentFile != nil {
			return currentFile
		}
		result.Files = append(result.Files, File{})
		currentFile = &result.Files[len(result.Files)-1]
		return currentFile
	}

	for _, raw := range lines {
		switch {
		case strings.HasPrefix(raw, "diff --git "):
			result.Files = append(result.Files, File{
				Headers: []string{raw},
			})
			currentFile = &result.Files[len(result.Files)-1]
			currentHunk = nil
		case strings.HasPrefix(raw, "--- "):
			file := ensureFile()
			file.OldPath = strings.TrimPrefix(strings.TrimPrefix(raw[4:], "a/"), "b/")
			file.Headers = append(file.Headers, raw)
			currentHunk = nil
		case strings.HasPrefix(raw, "+++ "):
			file := ensureFile()
			file.NewPath = strings.TrimPrefix(strings.TrimPrefix(raw[4:], "b/"), "a/")
			file.Headers = append(file.Headers, raw)
			currentHunk = nil
		case strings.HasPrefix(raw, "@@"):
			file := ensureFile()
			file.Hunks = append(file.Hunks, Hunk{Header: raw})
			currentHunk = &file.Hunks[len(file.Hunks)-1]
			oldLine, newLine = parseHunkHeader(raw)
		default:
			file := ensureFile()
			if currentHunk == nil {
				file.Headers = append(file.Headers, raw)
				continue
			}

			line := Line{Raw: raw}
			switch {
			case strings.HasPrefix(raw, "+") && !strings.HasPrefix(raw, "+++"):
				line.Kind = LineAdd
				line.Text = raw[1:]
				line.NewLine = newLine
				newLine++
			case strings.HasPrefix(raw, "-") && !strings.HasPrefix(raw, "---"):
				line.Kind = LineDelete
				line.Text = raw[1:]
				line.OldLine = oldLine
				oldLine++
			case strings.HasPrefix(raw, " "):
				line.Kind = LineContext
				line.Text = raw[1:]
				line.OldLine = oldLine
				line.NewLine = newLine
				oldLine++
				newLine++
			default:
				line.Kind = LineMeta
				line.Text = raw
			}
			currentHunk.Lines = append(currentHunk.Lines, line)
		}
	}

	return result
}

func RenderInline(diffText string, width int) []string {
	parsed := ParseUnifiedDiff(diffText)
	if len(parsed.Files) == 0 {
		return []string{styleFileMeta.Render("No diff loaded.")}
	}

	if width < 24 {
		width = 24
	}

	lines := make([]string, 0, 64)
	for fileIndex, file := range parsed.Files {
		if fileIndex > 0 {
			lines = append(lines, "")
		}

		lines = append(lines, renderFileHeader(file, width))
		lines = append(lines, renderFileMetadata(file, width)...)

		for _, hunk := range file.Hunks {
			lines = append(lines, trimStyled(styleHunkHeader.Width(width).Render(trimPlain(" "+hunk.Header+" ", width)), width))
			for _, plan := range buildRenderPlans(hunk, detectLanguage(file.NewPath, file.OldPath)) {
				lines = append(lines, renderPlanLines(plan, width)...)
			}
		}
	}

	return lines
}

func RenderSideBySide(diffText string, width int) []string {
	parsed := ParseUnifiedDiff(diffText)
	if len(parsed.Files) == 0 {
		return []string{styleFileMeta.Render("No diff loaded.")}
	}

	if width < 72 {
		return RenderInline(diffText, width)
	}

	columnWidth := maxInt(28, (width-3)/2)
	lines := make([]string, 0, 64)
	for fileIndex, file := range parsed.Files {
		if fileIndex > 0 {
			lines = append(lines, "")
		}

		lines = append(lines, renderFileHeader(file, width))
		lines = append(lines, renderFileMetadata(file, width)...)

		leftLabel := trimStyled(styleSplitHeader.Width(columnWidth).Render(" OLD "), columnWidth)
		rightLabel := trimStyled(styleSplitHeader.Width(columnWidth).Render(" NEW "), columnWidth)
		lines = append(lines, joinColumns(leftLabel, rightLabel))

		for _, hunk := range file.Hunks {
			lines = append(lines, trimStyled(styleHunkHeader.Width(width).Render(trimPlain(" "+hunk.Header+" ", width)), width))
			for _, row := range buildSideBySideRows(hunk, detectLanguage(file.NewPath, file.OldPath)) {
				lines = append(lines, renderSideBySideRowLines(row, columnWidth)...)
			}
		}
	}

	return lines
}

func buildRenderPlans(hunk Hunk, language string) []renderPlan {
	plans := make([]renderPlan, 0, len(hunk.Lines))
	for index := 0; index < len(hunk.Lines); {
		line := hunk.Lines[index]
		if line.Kind != LineDelete && line.Kind != LineAdd {
			plans = append(plans, renderPlan{
				kind:     line.Kind,
				oldLine:  line.OldLine,
				newLine:  line.NewLine,
				text:     line.Text,
				language: language,
			})
			index++
			continue
		}

		deleteStart := index
		for index < len(hunk.Lines) && hunk.Lines[index].Kind == LineDelete {
			index++
		}
		addStart := index
		for index < len(hunk.Lines) && hunk.Lines[index].Kind == LineAdd {
			index++
		}

		deletes := hunk.Lines[deleteStart:addStart]
		adds := hunk.Lines[addStart:index]
		pairCount := minInt(len(deletes), len(adds))

		for pairIndex := 0; pairIndex < pairCount; pairIndex++ {
			leftSpan, rightSpan := changedSpan(deletes[pairIndex].Text, adds[pairIndex].Text)
			plans = append(plans, renderPlan{
				kind:     LineDelete,
				oldLine:  deletes[pairIndex].OldLine,
				text:     deletes[pairIndex].Text,
				emphasis: leftSpan,
				language: language,
			})
			plans = append(plans, renderPlan{
				kind:     LineAdd,
				newLine:  adds[pairIndex].NewLine,
				text:     adds[pairIndex].Text,
				emphasis: rightSpan,
				language: language,
			})
		}

		for _, leftover := range deletes[pairCount:] {
			plans = append(plans, renderPlan{
				kind:     LineDelete,
				oldLine:  leftover.OldLine,
				text:     leftover.Text,
				language: language,
			})
		}
		for _, leftover := range adds[pairCount:] {
			plans = append(plans, renderPlan{
				kind:     LineAdd,
				newLine:  leftover.NewLine,
				text:     leftover.Text,
				language: language,
			})
		}
	}

	return plans
}

func buildSideBySideRows(hunk Hunk, language string) []sideBySideRow {
	rows := make([]sideBySideRow, 0, len(hunk.Lines))
	for index := 0; index < len(hunk.Lines); {
		line := hunk.Lines[index]
		if line.Kind != LineDelete && line.Kind != LineAdd {
			plan := renderPlan{
				kind:     line.Kind,
				oldLine:  line.OldLine,
				newLine:  line.NewLine,
				text:     line.Text,
				language: language,
			}
			rows = append(rows, sideBySideRow{left: &plan, right: &plan})
			index++
			continue
		}

		deleteStart := index
		for index < len(hunk.Lines) && hunk.Lines[index].Kind == LineDelete {
			index++
		}
		addStart := index
		for index < len(hunk.Lines) && hunk.Lines[index].Kind == LineAdd {
			index++
		}

		deletes := hunk.Lines[deleteStart:addStart]
		adds := hunk.Lines[addStart:index]
		pairCount := maxInt(len(deletes), len(adds))
		for pairIndex := 0; pairIndex < pairCount; pairIndex++ {
			var leftPlan *renderPlan
			var rightPlan *renderPlan

			if pairIndex < len(deletes) && pairIndex < len(adds) {
				leftSpan, rightSpan := changedSpan(deletes[pairIndex].Text, adds[pairIndex].Text)
				leftPlan = &renderPlan{
					kind:     LineDelete,
					oldLine:  deletes[pairIndex].OldLine,
					text:     deletes[pairIndex].Text,
					emphasis: leftSpan,
					language: language,
				}
				rightPlan = &renderPlan{
					kind:     LineAdd,
					newLine:  adds[pairIndex].NewLine,
					text:     adds[pairIndex].Text,
					emphasis: rightSpan,
					language: language,
				}
			} else {
				if pairIndex < len(deletes) {
					leftPlan = &renderPlan{
						kind:     LineDelete,
						oldLine:  deletes[pairIndex].OldLine,
						text:     deletes[pairIndex].Text,
						language: language,
					}
				}
				if pairIndex < len(adds) {
					rightPlan = &renderPlan{
						kind:     LineAdd,
						newLine:  adds[pairIndex].NewLine,
						text:     adds[pairIndex].Text,
						language: language,
					}
				}
			}

			rows = append(rows, sideBySideRow{left: leftPlan, right: rightPlan})
		}
	}

	return rows
}

func renderFileHeader(file File, width int) string {
	path := file.NewPath
	if path == "" {
		path = file.OldPath
	}
	if path == "" {
		path = "unknown"
	}

	adds, dels := fileChangeCounts(file)
	ext := strings.TrimPrefix(filepath.Ext(path), ".")
	label := fmt.Sprintf(" %s  +%d -%d", path, adds, dels)
	if ext != "" {
		label += "  [" + ext + "]"
	}

	return trimStyled(styleFileHeader.Width(width).Render(trimPlain(label, width)), width)
}

func renderFileMetadata(file File, width int) []string {
	lines := []string{}
	if file.OldPath != "" {
		lines = append(lines, trimStyled(lipgloss.JoinHorizontal(
			lipgloss.Left,
			styleOldPath.Render("OLD"),
			" ",
			styleFileMeta.Width(maxInt(8, width-6)).Render(trimPlain(file.OldPath, width-6)),
		), width))
	}
	if file.NewPath != "" {
		lines = append(lines, trimStyled(lipgloss.JoinHorizontal(
			lipgloss.Left,
			styleNewPath.Render("NEW"),
			" ",
			styleFileMeta.Width(maxInt(8, width-6)).Render(trimPlain(file.NewPath, width-6)),
		), width))
	}
	for _, header := range file.Headers {
		if strings.HasPrefix(header, "diff --git ") || strings.HasPrefix(header, "--- ") || strings.HasPrefix(header, "+++ ") || strings.TrimSpace(header) == "" {
			continue
		}
		lines = append(lines, trimStyled(styleFileMeta.Render(trimPlain(header, width)), width))
	}
	return lines
}

func fileChangeCounts(file File) (int, int) {
	adds := 0
	dels := 0
	for _, hunk := range file.Hunks {
		for _, line := range hunk.Lines {
			switch line.Kind {
			case LineAdd:
				adds++
			case LineDelete:
				dels++
			}
		}
	}
	return adds, dels
}

func renderPlanLine(plan renderPlan, width int) string {
	oldNumber := ""
	newNumber := ""
	sign := " "
	baseStyle := styleContextLine

	switch plan.kind {
	case LineAdd:
		newNumber = fmt.Sprintf("%4d", plan.newLine)
		sign = "+"
		baseStyle = styleAddLine
	case LineDelete:
		oldNumber = fmt.Sprintf("%4d", plan.oldLine)
		sign = "-"
		baseStyle = styleDelLine
	case LineContext:
		oldNumber = fmt.Sprintf("%4d", plan.oldLine)
		newNumber = fmt.Sprintf("%4d", plan.newLine)
	case LineMeta:
		baseStyle = styleMetaLine
	}

	if plan.kind == LineMeta {
		return trimStyled(baseStyle.Render(trimPlain(plan.text, width)), width)
	}

	gutter := lipgloss.JoinHorizontal(
		lipgloss.Left,
		styleLineNumber.Render(emptyNumber(oldNumber)),
		styleGutterBar.Render(" │ "),
		styleLineNumber.Render(emptyNumber(newNumber)),
		styleGutterBar.Render(" │ "),
		styleGutterBar.Render(sign+" "),
	)
	bodyWidth := maxInt(8, width-lipgloss.Width(gutter))
	body := renderLineBody(plan, bodyWidth, baseStyle)
	return trimStyled(gutter+body, width)
}

func renderPlanLines(plan renderPlan, width int) []string {
	oldNumber := ""
	newNumber := ""
	sign := " "
	baseStyle := styleContextLine

	switch plan.kind {
	case LineAdd:
		newNumber = fmt.Sprintf("%4d", plan.newLine)
		sign = "+"
		baseStyle = styleAddLine
	case LineDelete:
		oldNumber = fmt.Sprintf("%4d", plan.oldLine)
		sign = "-"
		baseStyle = styleDelLine
	case LineContext:
		oldNumber = fmt.Sprintf("%4d", plan.oldLine)
		newNumber = fmt.Sprintf("%4d", plan.newLine)
	case LineMeta:
		return []string{trimStyled(baseStyle.Render(trimPlain(plan.text, width)), width)}
	}

	gutter := lipgloss.JoinHorizontal(
		lipgloss.Left,
		styleLineNumber.Render(emptyNumber(oldNumber)),
		styleGutterBar.Render(" │ "),
		styleLineNumber.Render(emptyNumber(newNumber)),
		styleGutterBar.Render(" │ "),
		styleGutterBar.Render(sign+" "),
	)
	continuation := lipgloss.JoinHorizontal(
		lipgloss.Left,
		styleLineNumber.Render("    "),
		styleGutterBar.Render(" │ "),
		styleLineNumber.Render("    "),
		styleGutterBar.Render(" │ "),
		styleGutterBar.Render("· "),
	)
	bodyWidth := maxInt(8, width-lipgloss.Width(gutter))
	bodyLines := renderLineBodyLines(plan, bodyWidth, baseStyle)
	lines := make([]string, 0, len(bodyLines))
	for index, body := range bodyLines {
		prefix := gutter
		if index > 0 {
			prefix = continuation
		}
		lines = append(lines, trimStyled(prefix+body, width))
	}
	return lines
}

func renderSideBySideRowLines(row sideBySideRow, columnWidth int) []string {
	leftLines := renderSideCellLines(row.left, columnWidth, "left")
	rightLines := renderSideCellLines(row.right, columnWidth, "right")
	lineCount := maxInt(len(leftLines), len(rightLines))
	joined := make([]string, 0, lineCount)
	for index := 0; index < lineCount; index++ {
		left := ""
		right := ""
		if index < len(leftLines) {
			left = leftLines[index]
		} else {
			left = lipgloss.NewStyle().Width(columnWidth).Render("")
		}
		if index < len(rightLines) {
			right = rightLines[index]
		} else {
			right = lipgloss.NewStyle().Width(columnWidth).Render("")
		}
		joined = append(joined, joinColumns(left, right))
	}
	return joined
}

func renderSideCell(plan *renderPlan, width int, side string) string {
	if plan == nil {
		return lipgloss.NewStyle().Width(width).Render("")
	}

	number := ""
	sign := " "
	baseStyle := styleContextLine

	switch plan.kind {
	case LineDelete:
		number = fmt.Sprintf("%4d", plan.oldLine)
		sign = "-"
		baseStyle = styleDelLine
	case LineAdd:
		number = fmt.Sprintf("%4d", plan.newLine)
		sign = "+"
		baseStyle = styleAddLine
	case LineContext:
		if side == "left" {
			number = fmt.Sprintf("%4d", plan.oldLine)
		} else {
			number = fmt.Sprintf("%4d", plan.newLine)
		}
	case LineMeta:
		return trimStyled(styleMetaLine.Width(width).Render(trimPlain(plan.text, width)), width)
	}

	gutter := lipgloss.JoinHorizontal(
		lipgloss.Left,
		styleLineNumber.Render(emptyNumber(number)),
		styleGutterBar.Render(" │ "),
		styleGutterBar.Render(sign+" "),
	)
	bodyWidth := maxInt(8, width-lipgloss.Width(gutter))
	body := renderLineBody(*plan, bodyWidth, baseStyle)
	return trimStyled(gutter+body, width)
}

func renderSideCellLines(plan *renderPlan, width int, side string) []string {
	if plan == nil {
		return []string{lipgloss.NewStyle().Width(width).Render("")}
	}

	number := ""
	sign := " "
	baseStyle := styleContextLine

	switch plan.kind {
	case LineDelete:
		number = fmt.Sprintf("%4d", plan.oldLine)
		sign = "-"
		baseStyle = styleDelLine
	case LineAdd:
		number = fmt.Sprintf("%4d", plan.newLine)
		sign = "+"
		baseStyle = styleAddLine
	case LineContext:
		if side == "left" {
			number = fmt.Sprintf("%4d", plan.oldLine)
		} else {
			number = fmt.Sprintf("%4d", plan.newLine)
		}
	case LineMeta:
		return []string{trimStyled(styleMetaLine.Width(width).Render(trimPlain(plan.text, width)), width)}
	}

	gutter := lipgloss.JoinHorizontal(
		lipgloss.Left,
		styleLineNumber.Render(emptyNumber(number)),
		styleGutterBar.Render(" │ "),
		styleGutterBar.Render(sign+" "),
	)
	continuation := lipgloss.JoinHorizontal(
		lipgloss.Left,
		styleLineNumber.Render("    "),
		styleGutterBar.Render(" │ "),
		styleGutterBar.Render("· "),
	)
	bodyWidth := maxInt(8, width-lipgloss.Width(gutter))
	bodyLines := renderLineBodyLines(*plan, bodyWidth, baseStyle)
	lines := make([]string, 0, len(bodyLines))
	for index, body := range bodyLines {
		prefix := gutter
		if index > 0 {
			prefix = continuation
		}
		lines = append(lines, trimStyled(prefix+body, width))
	}
	return lines
}

func joinColumns(left, right string) string {
	return lipgloss.JoinHorizontal(lipgloss.Top, left, styleColumnGap, right)
}

func renderLineBody(plan renderPlan, width int, baseStyle lipgloss.Style) string {
	text := trimPlain(plan.text, width)

	switch plan.kind {
	case LineAdd, LineDelete:
		segments := []string{}
		start := clampInt(plan.emphasis.start, 0, len(text))
		end := clampInt(plan.emphasis.end, 0, len(text))
		if end < start {
			end = start
		}

		if start > 0 {
			segments = append(segments, baseStyle.Render(text[:start]))
		}
		if start < end {
			focusStyle := styleAddFocus
			if plan.kind == LineDelete {
				focusStyle = styleDelFocus
			}
			segments = append(segments, focusStyle.Render(text[start:end]))
		}
		if end < len(text) {
			segments = append(segments, baseStyle.Render(text[end:]))
		}
		if len(segments) == 0 {
			return baseStyle.Width(width).Render("")
		}
		return baseStyle.Width(width).Render(strings.Join(segments, ""))
	case LineContext:
		return styleContextLine.Width(width).Render(renderSyntax(text, plan.language, styleContextLine))
	default:
		return baseStyle.Width(width).Render(text)
	}
}

func renderLineBodyLines(plan renderPlan, width int, baseStyle lipgloss.Style) []string {
	chunks := wrapPlainText(plan.text, width)
	if len(chunks) == 0 {
		return []string{baseStyle.Width(width).Render("")}
	}

	lines := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		lines = append(lines, renderLineBodyChunk(plan, chunk, width, baseStyle))
	}
	return lines
}

func renderLineBodyChunk(plan renderPlan, chunk wrappedChunk, width int, baseStyle lipgloss.Style) string {
	text := chunk.text
	runeCount := len([]rune(text))

	switch plan.kind {
	case LineAdd, LineDelete:
		segments := []string{}
		start := clampInt(plan.emphasis.start-chunk.start, 0, runeCount)
		end := clampInt(plan.emphasis.end-chunk.start, 0, runeCount)
		if end < start {
			end = start
		}

		if start > 0 {
			segments = append(segments, renderSyntaxWithBase(sliceRunes(text, 0, start), plan.language, tokenPaletteForPlan(plan, false), baseStyle))
		}
		if start < end {
			focusStyle := styleAddFocus
			if plan.kind == LineDelete {
				focusStyle = styleDelFocus
			}
			segments = append(segments, renderSyntaxWithBase(sliceRunes(text, start, end), plan.language, tokenPaletteForPlan(plan, true), focusStyle))
		}
		if end < runeCount {
			segments = append(segments, renderSyntaxWithBase(sliceRunes(text, end, runeCount), plan.language, tokenPaletteForPlan(plan, false), baseStyle))
		}
		if len(segments) == 0 {
			return baseStyle.Width(width).Render("")
		}
		return baseStyle.Width(width).Render(strings.Join(segments, ""))
	case LineContext:
		return styleContextLine.Width(width).Render(renderSyntax(text, plan.language, styleContextLine))
	default:
		return baseStyle.Width(width).Render(text)
	}
}

type tokenPalette struct {
	defaultFG string
	bg        string
}

func tokenPaletteForPlan(plan renderPlan, focus bool) tokenPalette {
	switch plan.kind {
	case LineAdd:
		if focus {
			return tokenPalette{defaultFG: "230", bg: "28"}
		}
		return tokenPalette{defaultFG: "194", bg: "22"}
	case LineDelete:
		if focus {
			return tokenPalette{defaultFG: "230", bg: "88"}
		}
		return tokenPalette{defaultFG: "224", bg: "52"}
	default:
		return tokenPalette{defaultFG: "252", bg: ""}
	}
}

func renderSyntaxWithBase(text, language string, palette tokenPalette, baseStyle lipgloss.Style) string {
	if strings.TrimSpace(text) == "" || language == "plain" {
		return baseStyle.Render(text)
	}

	matches := highlightPattern.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return baseStyle.Render(text)
	}

	var builder strings.Builder
	last := 0
	for _, match := range matches {
		if match[0] > last {
			builder.WriteString(styleToken(text[last:match[0]], palette, false).Render(text[last:match[0]]))
		}

		value := text[match[0]:match[1]]
		fg, bold := classifyTokenStyle(value, palette.defaultFG)
		builder.WriteString(styleTokenWithFG(value, palette, fg, bold).Render(value))
		last = match[1]
	}
	if last < len(text) {
		builder.WriteString(styleToken(text[last:], palette, false).Render(text[last:]))
	}
	return builder.String()
}

func styleToken(text string, palette tokenPalette, bold bool) lipgloss.Style {
	return styleTokenWithFG(text, palette, palette.defaultFG, bold)
}

func styleTokenWithFG(_ string, palette tokenPalette, fg string, bold bool) lipgloss.Style {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(fg))
	if palette.bg != "" {
		style = style.Background(lipgloss.Color(palette.bg))
	}
	if bold {
		style = style.Bold(true)
	}
	return style
}

func renderSyntax(text, language string, base lipgloss.Style) string {
	if strings.TrimSpace(text) == "" || language == "plain" {
		return text
	}

	matches := highlightPattern.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return text
	}

	tokens := make([]token, 0, len(matches)*2)
	last := 0
	for _, match := range matches {
		if match[0] > last {
			tokens = append(tokens, token{text: text[last:match[0]], style: base})
		}

		value := text[match[0]:match[1]]
		fg, bold := classifyTokenStyle(value, "252")
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(fg))
		if bold {
			style = style.Bold(true)
		}
		tokens = append(tokens, token{text: value, style: style})
		last = match[1]
	}
	if last < len(text) {
		tokens = append(tokens, token{text: text[last:], style: base})
	}

	var builder strings.Builder
	for _, token := range tokens {
		builder.WriteString(token.style.Render(token.text))
	}
	return builder.String()
}

func classifyTokenStyle(value string, defaultFG string) (string, bool) {
	switch {
	case strings.HasPrefix(value, "//"), strings.HasPrefix(value, "#"):
		return "244", false
	case strings.HasPrefix(value, `"`), strings.HasPrefix(value, `'`), strings.HasPrefix(value, "`"):
		return "114", false
	case regexp.MustCompile(`^\d+$`).MatchString(value):
		return "179", false
	default:
		if style, ok := keywordStyles[value]; ok {
			rendered := style.Render(value)
			_ = rendered
			switch value {
			case "func", "type", "struct", "interface", "class":
				return "81", true
			case "return":
				return "81", false
			case "const", "let", "var", "import", "package", "from":
				return "117", false
			case "if", "else", "for", "range", "switch", "case":
				return "215", false
			case "true", "false", "nil", "null":
				return "186", false
			}
		}
		return defaultFG, false
	}
}

func parseHunkHeader(header string) (int, int) {
	oldStart := 1
	newStart := 1

	if oldMatch := regexp.MustCompile(`@@ -(\d+)`).FindStringSubmatch(header); len(oldMatch) == 2 {
		fmt.Sscanf(oldMatch[1], "%d", &oldStart)
	}
	if newMatch := regexp.MustCompile(`\+(\d+)`).FindStringSubmatch(header); len(newMatch) == 2 {
		fmt.Sscanf(newMatch[1], "%d", &newStart)
	}

	return oldStart, newStart
}

func changedSpan(left, right string) (span, span) {
	leftPrefix := sharedPrefix(left, right)
	leftSuffix := sharedSuffix(sliceRunes(left, leftPrefix, len([]rune(left))), sliceRunes(right, leftPrefix, len([]rune(right))))

	leftEnd := len([]rune(left)) - leftSuffix
	rightEnd := len([]rune(right)) - leftSuffix
	if leftEnd < leftPrefix {
		leftEnd = leftPrefix
	}
	if rightEnd < leftPrefix {
		rightEnd = leftPrefix
	}

	return span{start: leftPrefix, end: leftEnd}, span{start: leftPrefix, end: rightEnd}
}

func sharedPrefix(left, right string) int {
	leftRunes := []rune(left)
	rightRunes := []rune(right)
	limit := minInt(len(leftRunes), len(rightRunes))
	index := 0
	for index < limit && leftRunes[index] == rightRunes[index] {
		index++
	}
	return index
}

func sharedSuffix(left, right string) int {
	leftRunes := []rune(left)
	rightRunes := []rune(right)
	limit := minInt(len(leftRunes), len(rightRunes))
	index := 0
	for index < limit && leftRunes[len(leftRunes)-1-index] == rightRunes[len(rightRunes)-1-index] {
		index++
	}
	return index
}

func detectLanguage(newPath, oldPath string) string {
	path := newPath
	if path == "" {
		path = oldPath
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "ts"
	case ".js", ".jsx", ".mjs":
		return "js"
	case ".json":
		return "json"
	case ".md":
		return "md"
	case ".css":
		return "css"
	case ".py":
		return "py"
	case ".rs":
		return "rs"
	default:
		return "plain"
	}
}

func emptyNumber(value string) string {
	if value == "" {
		return "    "
	}
	return value
}

func trimPlain(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(value) <= width {
		return value
	}
	runes := []rune(value)
	if width <= 1 {
		return string(runes[:1])
	}
	if len(runes) > width-1 {
		runes = runes[:width-1]
	}
	return string(runes) + "…"
}

func trimStyled(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(value) <= width {
		return value
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(value)
}

func wrapPlainText(value string, width int) []wrappedChunk {
	if width <= 0 {
		return nil
	}

	runes := []rune(value)
	if len(runes) == 0 {
		return []wrappedChunk{{text: "", start: 0, end: 0}}
	}

	chunks := make([]wrappedChunk, 0, (len(runes)/width)+1)
	for start := 0; start < len(runes); start += width {
		end := minInt(len(runes), start+width)
		chunks = append(chunks, wrappedChunk{
			text:  string(runes[start:end]),
			start: start,
			end:   end,
		})
	}
	return chunks
}

func sliceRunes(value string, start, end int) string {
	runes := []rune(value)
	start = clampInt(start, 0, len(runes))
	end = clampInt(end, 0, len(runes))
	if end < start {
		end = start
	}
	return string(runes[start:end])
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
