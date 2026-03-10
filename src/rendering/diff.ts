export type DiffLineKind = 'context' | 'add' | 'del' | 'meta';
export type RenderRowKind = 'file-header' | 'hunk-header' | 'diff-line' | 'collapsed';

export interface ParsedDiffLine {
  kind: DiffLineKind;
  raw: string;
  text: string;
  oldLineNumber?: number;
  newLineNumber?: number;
}

export interface ParsedHunk {
  header: string;
  lines: ParsedDiffLine[];
}

export interface ParsedDiffFile {
  oldPath: string;
  newPath: string;
  headerLines: string[];
  hunks: ParsedHunk[];
  language: string;
}

export interface ParsedDiff {
  files: ParsedDiffFile[];
}

export interface TextToken {
  text: string;
  color?: string;
}

export interface InlineRenderRow {
  kind: RenderRowKind;
  text: string;
  diffLine?: ParsedDiffLine;
  hunkIndex?: number;
}

export interface SideBySideCell {
  kind: DiffLineKind | 'empty';
  prefix: string;
  text: string;
  oldLineNumber?: number;
  newLineNumber?: number;
  tokens: TextToken[];
}

export interface SideBySideRenderRow {
  kind: RenderRowKind;
  text?: string;
  left?: SideBySideCell;
  right?: SideBySideCell;
  hunkIndex?: number;
}

export interface InlineRenderResult {
  rows: InlineRenderRow[];
  hunkRowIndexes: number[];
}

export interface SideBySideRenderResult {
  rows: SideBySideRenderRow[];
  hunkRowIndexes: number[];
}

export interface RenderOptions {
  collapseContext: boolean;
  collapseKeep: number;
}

const KEYWORD_COLORS: Record<string, string> = {
  function: 'cyan',
  return: 'cyan',
  const: 'blue',
  let: 'blue',
  var: 'blue',
  if: 'magenta',
  else: 'magenta',
  switch: 'magenta',
  case: 'magenta',
  for: 'magenta',
  while: 'magenta',
  import: 'blue',
  export: 'blue',
  from: 'blue',
  class: 'cyan',
  interface: 'cyan',
  type: 'cyan',
  extends: 'cyan',
  implements: 'cyan',
  async: 'yellow',
  await: 'yellow',
  true: 'yellow',
  false: 'yellow',
  null: 'yellow',
  undefined: 'yellow'
};

function detectLanguage(path: string): string {
  const lower = path.toLowerCase();

  if (lower.endsWith('.ts') || lower.endsWith('.tsx')) {
    return 'ts';
  }

  if (lower.endsWith('.js') || lower.endsWith('.jsx') || lower.endsWith('.mjs')) {
    return 'js';
  }

  if (lower.endsWith('.json')) {
    return 'json';
  }

  if (lower.endsWith('.md')) {
    return 'md';
  }

  if (lower.endsWith('.css')) {
    return 'css';
  }

  if (lower.endsWith('.go')) {
    return 'go';
  }

  if (lower.endsWith('.rs')) {
    return 'rs';
  }

  if (lower.endsWith('.py')) {
    return 'py';
  }

  return 'plain';
}

function parseHunkHeader(header: string): {oldStart: number; newStart: number} {
  const match = /@@ -(\d+)/.exec(header);
  const matchNew = /\+(\d+)/.exec(header);

  return {
    oldStart: match ? Number(match[1]) : 1,
    newStart: matchNew ? Number(matchNew[1]) : 1
  };
}

export function parseUnifiedDiff(diffText: string): ParsedDiff {
  if (diffText.trim() === '') {
    return {files: []};
  }

  const lines = diffText.split('\n');
  const files: ParsedDiffFile[] = [];
  let currentFile: ParsedDiffFile | null = null;
  let currentHunk: ParsedHunk | null = null;
  let oldLineNumber = 0;
  let newLineNumber = 0;

  const ensureFile = (): ParsedDiffFile => {
    if (!currentFile) {
      currentFile = {
        oldPath: '',
        newPath: '',
        headerLines: [],
        hunks: [],
        language: 'plain'
      };
      files.push(currentFile);
    }

    return currentFile;
  };

  for (const rawLine of lines) {
    if (rawLine.startsWith('diff --git ')) {
      currentFile = {
        oldPath: '',
        newPath: '',
        headerLines: [rawLine],
        hunks: [],
        language: 'plain'
      };
      files.push(currentFile);
      currentHunk = null;
      continue;
    }

    const file = ensureFile();

    if (rawLine.startsWith('--- ')) {
      file.oldPath = rawLine.slice(4).replace(/^a\//, '');
      file.headerLines.push(rawLine);
      file.language = detectLanguage(file.oldPath || file.newPath);
      currentHunk = null;
      continue;
    }

    if (rawLine.startsWith('+++ ')) {
      file.newPath = rawLine.slice(4).replace(/^b\//, '');
      file.headerLines.push(rawLine);
      file.language = detectLanguage(file.newPath || file.oldPath);
      currentHunk = null;
      continue;
    }

    if (rawLine.startsWith('@@')) {
      currentHunk = {
        header: rawLine,
        lines: []
      };
      file.hunks.push(currentHunk);
      const positions = parseHunkHeader(rawLine);
      oldLineNumber = positions.oldStart;
      newLineNumber = positions.newStart;
      continue;
    }

    if (!currentHunk) {
      file.headerLines.push(rawLine);
      continue;
    }

    if (rawLine.startsWith('+') && !rawLine.startsWith('+++')) {
      currentHunk.lines.push({
        kind: 'add',
        raw: rawLine,
        text: rawLine.slice(1),
        newLineNumber
      });
      newLineNumber += 1;
      continue;
    }

    if (rawLine.startsWith('-') && !rawLine.startsWith('---')) {
      currentHunk.lines.push({
        kind: 'del',
        raw: rawLine,
        text: rawLine.slice(1),
        oldLineNumber
      });
      oldLineNumber += 1;
      continue;
    }

    if (rawLine.startsWith(' ')) {
      currentHunk.lines.push({
        kind: 'context',
        raw: rawLine,
        text: rawLine.slice(1),
        oldLineNumber,
        newLineNumber
      });
      oldLineNumber += 1;
      newLineNumber += 1;
      continue;
    }

    currentHunk.lines.push({
      kind: 'meta',
      raw: rawLine,
      text: rawLine
    });
  }

  return {files};
}

function collapseHunkLines(lines: ParsedDiffLine[], collapseKeep: number): Array<ParsedDiffLine | {collapsed: number}> {
  if (lines.length === 0) {
    return [];
  }

  const firstNonContext = lines.findIndex((line) => line.kind !== 'context');
  const lastNonContext = [...lines].reverse().findIndex((line) => line.kind !== 'context');

  if (firstNonContext === -1 || lastNonContext === -1) {
    return lines;
  }

  const leadingContext = firstNonContext;
  const trailingContext = lastNonContext;
  const trailingCount = trailingContext;
  const result: Array<ParsedDiffLine | {collapsed: number}> = [];

  if (leadingContext > collapseKeep) {
    result.push(...lines.slice(0, collapseKeep));
    result.push({collapsed: leadingContext - collapseKeep});
  } else {
    result.push(...lines.slice(0, leadingContext));
  }

  const middleStart = leadingContext;
  const middleEnd = lines.length - trailingCount;
  result.push(...lines.slice(middleStart, middleEnd));

  if (trailingCount > collapseKeep) {
    result.push({collapsed: trailingCount - collapseKeep});
    result.push(...lines.slice(lines.length - collapseKeep));
  } else {
    result.push(...lines.slice(lines.length - trailingCount));
  }

  return result;
}

function buildVisibleHunkLines(hunk: ParsedHunk, options: RenderOptions): Array<ParsedDiffLine | {collapsed: number}> {
  if (!options.collapseContext) {
    return hunk.lines;
  }

  return collapseHunkLines(hunk.lines, options.collapseKeep);
}

export function buildInlineRender(parsed: ParsedDiff, options: RenderOptions): InlineRenderResult {
  const rows: InlineRenderRow[] = [];
  const hunkRowIndexes: number[] = [];
  let hunkIndex = 0;

  for (const file of parsed.files) {
    const pathLabel = file.newPath || file.oldPath || 'unknown';
    rows.push({
      kind: 'file-header',
      text: `diff ${pathLabel}`
    });

    for (const headerLine of file.headerLines) {
      if (headerLine.startsWith('diff --git ')) {
        continue;
      }

      rows.push({
        kind: 'file-header',
        text: headerLine
      });
    }

    for (const hunk of file.hunks) {
      hunkRowIndexes.push(rows.length);
      rows.push({
        kind: 'hunk-header',
        text: hunk.header,
        hunkIndex
      });

      for (const line of buildVisibleHunkLines(hunk, options)) {
        if ('collapsed' in line) {
          rows.push({
            kind: 'collapsed',
            text: `... ${line.collapsed} unchanged lines ...`,
            hunkIndex
          });
          continue;
        }

        rows.push({
          kind: 'diff-line',
          text: line.raw,
          diffLine: line,
          hunkIndex
        });
      }

      hunkIndex += 1;
    }
  }

  return {rows, hunkRowIndexes};
}

function tokenizePlain(text: string): TextToken[] {
  const pattern = /(".*?"|'.*?'|`.*?`|\/\/.*$|#.*$|\b\d+\b|\b[A-Za-z_][A-Za-z0-9_]*\b)/g;
  const tokens: TextToken[] = [];
  let lastIndex = 0;

  for (const match of text.matchAll(pattern)) {
    const matched = match[0];
    const start = match.index ?? 0;

    if (start > lastIndex) {
      tokens.push({text: text.slice(lastIndex, start)});
    }

    let color: string | undefined;

    if (matched.startsWith('//') || matched.startsWith('#')) {
      color = 'gray';
    } else if (matched.startsWith('"') || matched.startsWith("'") || matched.startsWith('`')) {
      color = 'green';
    } else if (/^\d+$/.test(matched)) {
      color = 'magenta';
    } else {
      color = KEYWORD_COLORS[matched];
    }

    tokens.push({text: matched, color});
    lastIndex = start + matched.length;
  }

  if (lastIndex < text.length) {
    tokens.push({text: text.slice(lastIndex)});
  }

  return tokens.length > 0 ? tokens : [{text}];
}

export function tokenizeLine(text: string, language: string): TextToken[] {
  if (language === 'plain' || text.trim() === '') {
    return [{text}];
  }

  return tokenizePlain(text);
}

function makeCell(line: ParsedDiffLine, language: string): SideBySideCell {
  return {
    kind: line.kind,
    prefix: line.kind === 'add' ? '+' : line.kind === 'del' ? '-' : line.kind === 'context' ? ' ' : ' ',
    text: line.text,
    oldLineNumber: line.oldLineNumber,
    newLineNumber: line.newLineNumber,
    tokens: line.kind === 'meta' ? [{text: line.text}] : tokenizeLine(line.text, language)
  };
}

function makeEmptyCell(): SideBySideCell {
  return {
    kind: 'empty',
    prefix: ' ',
    text: '',
    tokens: [{text: ''}]
  };
}

function flushChangeBuffers(
  rows: SideBySideRenderRow[],
  deletes: ParsedDiffLine[],
  adds: ParsedDiffLine[],
  language: string,
  hunkIndex: number
): void {
  if (deletes.length === 0 && adds.length === 0) {
    return;
  }

  const width = Math.max(deletes.length, adds.length);

  for (let index = 0; index < width; index += 1) {
    rows.push({
      kind: 'diff-line',
      left: deletes[index] ? makeCell(deletes[index], language) : makeEmptyCell(),
      right: adds[index] ? makeCell(adds[index], language) : makeEmptyCell(),
      hunkIndex
    });
  }

  deletes.length = 0;
  adds.length = 0;
}

export function buildSideBySideRender(parsed: ParsedDiff, options: RenderOptions): SideBySideRenderResult {
  const rows: SideBySideRenderRow[] = [];
  const hunkRowIndexes: number[] = [];
  let hunkIndex = 0;

  for (const file of parsed.files) {
    const pathLabel = file.newPath || file.oldPath || 'unknown';
    rows.push({
      kind: 'file-header',
      text: `diff ${pathLabel}`
    });

    for (const hunk of file.hunks) {
      hunkRowIndexes.push(rows.length);
      rows.push({
        kind: 'hunk-header',
        text: hunk.header,
        hunkIndex
      });

      const deletes: ParsedDiffLine[] = [];
      const adds: ParsedDiffLine[] = [];

      for (const line of buildVisibleHunkLines(hunk, options)) {
        if ('collapsed' in line) {
          flushChangeBuffers(rows, deletes, adds, file.language, hunkIndex);
          rows.push({
            kind: 'collapsed',
            text: `... ${line.collapsed} unchanged lines ...`,
            hunkIndex
          });
          continue;
        }

        if (line.kind === 'del') {
          deletes.push(line);
          continue;
        }

        if (line.kind === 'add') {
          adds.push(line);
          continue;
        }

        flushChangeBuffers(rows, deletes, adds, file.language, hunkIndex);

        if (line.kind === 'context') {
          const cell = makeCell(line, file.language);
          rows.push({
            kind: 'diff-line',
            left: cell,
            right: cell,
            hunkIndex
          });
          continue;
        }

        rows.push({
          kind: 'diff-line',
          left: {
            kind: 'meta',
            prefix: ' ',
            text: line.text,
            tokens: [{text: line.text}]
          },
          right: {
            kind: 'meta',
            prefix: ' ',
            text: line.text,
            tokens: [{text: line.text}]
          },
          hunkIndex
        });
      }

      flushChangeBuffers(rows, deletes, adds, file.language, hunkIndex);
      hunkIndex += 1;
    }
  }

  return {rows, hunkRowIndexes};
}
