import React, {useEffect, useMemo, useRef, useState} from 'react';
import {Box, Text, useApp, useInput, useStdout} from 'ink';

import type {
  CommitSummary,
  CompareSelection,
  ConflictFile,
  ConflictFileContents,
  DiffStyle,
  ExplorerMode,
  FileChange,
  PaneFocus,
  RepositoryInfo
} from '../domain/models.js';
import {
  GitCommandError,
  acceptConflictSide,
  discoverRepository,
  getCommitDiff,
  getConflictFileContents,
  getRangeDiff,
  listCommitFiles,
  listCommits,
  listConflictFiles,
  listRangeFiles,
  openFileInEditor
} from '../git/client.js';
import {
  buildInlineRender,
  buildSideBySideRender,
  parseUnifiedDiff,
  tokenizeLine,
  type InlineRenderRow,
  type SideBySideCell,
  type SideBySideRenderRow,
  type TextToken
} from '../rendering/diff.js';

interface AppProps {
  cwd: string;
}

interface DataState {
  repository: RepositoryInfo | null;
  commits: CommitSummary[];
  files: FileChange[];
  conflictFiles: ConflictFile[];
  diff: string;
  conflictContents: ConflictFileContents | null;
  repositoryError: string | null;
  filesError: string | null;
  diffError: string | null;
  loadingRepository: boolean;
  loadingFiles: boolean;
  loadingDiff: boolean;
}

interface SearchState {
  open: boolean;
  target: 'commits' | 'files';
}

interface CompareOption {
  key: string;
  label: string;
  detail: string;
  disabled?: boolean;
  run: () => void;
}

interface CommandAction {
  key: string;
  label: string;
  detail: string;
  disabled?: boolean;
  run: () => void;
}

interface ConflictRenderRow {
  kind: 'section-header' | 'line' | 'blank';
  text: string;
  color?: string;
  lineNumber?: number;
}

type DiffPresentationMode = 'inline' | 'side-by-side';

const INITIAL_STATE: DataState = {
  repository: null,
  commits: [],
  files: [],
  conflictFiles: [],
  diff: '',
  conflictContents: null,
  repositoryError: null,
  filesError: null,
  diffError: null,
  loadingRepository: true,
  loadingFiles: false,
  loadingDiff: false
};

function getErrorMessage(error: unknown): string {
  if (error instanceof GitCommandError) {
    return error.stderr || error.message;
  }

  if (error instanceof Error) {
    return error.message;
  }

  return 'Unknown error';
}

function trimToWidth(value: string, width: number): string {
  if (width <= 0) {
    return '';
  }

  if (value.length <= width) {
    return value.padEnd(width, ' ');
  }

  if (width <= 3) {
    return value.slice(0, width);
  }

  return `${value.slice(0, Math.max(0, width - 3))}...`;
}

function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(value, max));
}

function createWindow<T>(items: T[], selectedIndex: number, maxItems: number): Array<{item: T; index: number}> {
  if (items.length === 0 || maxItems <= 0) {
    return [];
  }

  const halfWindow = Math.floor(maxItems / 2);
  const start = Math.max(0, Math.min(selectedIndex - halfWindow, items.length - maxItems));
  return items.slice(start, start + maxItems).map((item, offset) => ({item, index: start + offset}));
}

function formatComparisonSpec(selection: CompareSelection): string {
  const operator = selection.diffStyle === 'three-dot' ? '...' : '..';
  return `${selection.leftLabel}${operator}${selection.rightLabel}`;
}

function matchesCommit(commit: CommitSummary, query: string): boolean {
  const haystack = `${commit.shortSha} ${commit.sha} ${commit.subject} ${commit.authorName} ${commit.refs.join(' ')}`.toLowerCase();
  return haystack.includes(query.toLowerCase());
}

function matchesFile(file: {status: string; path: string; oldPath?: string}, query: string): boolean {
  const haystack = `${file.status} ${file.path} ${file.oldPath ?? ''}`.toLowerCase();
  return haystack.includes(query.toLowerCase());
}

function renderTokens(tokens: TextToken[], keyPrefix: string, fallbackColor?: string): React.JSX.Element[] {
  return tokens.map((token, index) => (
    <Text key={`${keyPrefix}-${index}`} color={token.color ?? fallbackColor}>
      {token.text}
    </Text>
  ));
}

function truncateTokens(tokens: TextToken[], width: number): TextToken[] {
  if (width <= 0) {
    return [];
  }

  let remaining = width;
  const result: TextToken[] = [];

  for (const token of tokens) {
    if (remaining <= 0) {
      break;
    }

    if (token.text.length <= remaining) {
      result.push(token);
      remaining -= token.text.length;
      continue;
    }

    if (remaining <= 3) {
      result.push({
        text: token.text.slice(0, remaining)
      });
    } else {
      result.push({
        ...token,
        text: `${token.text.slice(0, remaining - 3)}...`
      });
    }

    remaining = 0;
  }

  return result;
}

function getCellBaseColor(kind: SideBySideCell['kind']): string | undefined {
  if (kind === 'add') {
    return 'green';
  }

  if (kind === 'del') {
    return 'red';
  }

  if (kind === 'meta') {
    return 'yellow';
  }

  if (kind === 'context') {
    return 'white';
  }

  return 'gray';
}

function renderInlineRow(
  row: InlineRenderRow,
  index: number,
  gutterWidth: number,
  width: number
): React.JSX.Element {
  if (row.kind === 'file-header') {
    return (
      <Text key={`inline-${index}`} color="cyan">
        {trimToWidth(row.text, width)}
      </Text>
    );
  }

  if (row.kind === 'hunk-header') {
    return (
      <Text key={`inline-${index}`} color="yellow">
        {trimToWidth(row.text, width)}
      </Text>
    );
  }

  if (row.kind === 'collapsed') {
    return (
      <Text key={`inline-${index}`} color="gray">
        {trimToWidth(row.text, width)}
      </Text>
    );
  }

  const diffLine = row.diffLine;

  if (!diffLine) {
    return (
      <Text key={`inline-${index}`} color="gray">
        {trimToWidth(row.text, width)}
      </Text>
    );
  }

  const oldNumber = diffLine.oldLineNumber ? String(diffLine.oldLineNumber).padStart(gutterWidth, ' ') : ' '.repeat(gutterWidth);
  const newNumber = diffLine.newLineNumber ? String(diffLine.newLineNumber).padStart(gutterWidth, ' ') : ' '.repeat(gutterWidth);
  const prefix = diffLine.kind === 'add' ? '+' : diffLine.kind === 'del' ? '-' : diffLine.kind === 'context' ? ' ' : ' ';
  const contentWidth = Math.max(0, width - gutterWidth * 2 - 4);
  const content = trimToWidth(diffLine.text, contentWidth);
  const baseColor =
    diffLine.kind === 'add'
      ? 'green'
      : diffLine.kind === 'del'
        ? 'red'
        : diffLine.kind === 'meta'
          ? 'yellow'
          : 'white';
  const tokens = diffLine.kind === 'meta' ? [{text: content}] : tokenizeLine(content, 'plain');

  return (
    <Text key={`inline-${index}`}>
      <Text color="gray">{oldNumber}</Text>
      <Text color="gray"> </Text>
      <Text color="gray">{newNumber}</Text>
      <Text color="gray"> </Text>
      <Text color={baseColor}>{prefix}</Text>
      {renderTokens(tokens, `inline-${index}`, baseColor)}
    </Text>
  );
}

function renderSideCell(
  cell: SideBySideCell | undefined,
  keyPrefix: string,
  width: number,
  gutterWidth: number,
  side: 'left' | 'right'
): React.JSX.Element {
  const current = cell ?? {
    kind: 'empty' as const,
    prefix: ' ',
    text: '',
    tokens: [{text: ''}]
  };
  const lineNumber = side === 'left' ? current.oldLineNumber : current.newLineNumber;
  const gutter = lineNumber ? String(lineNumber).padStart(gutterWidth, ' ') : ' '.repeat(gutterWidth);
  const contentWidth = Math.max(0, width - gutterWidth - 3);
  const baseColor = getCellBaseColor(current.kind);
  const text =
    current.tokens.length === 1 && current.tokens[0].text === current.text
      ? [{text: trimToWidth(current.text, contentWidth), color: current.tokens[0].color}]
      : truncateTokens(current.tokens, contentWidth);

  return (
    <Text>
      <Text color="gray">{gutter}</Text>
      <Text color="gray"> </Text>
      <Text color={baseColor}>{current.prefix}</Text>
      {renderTokens(text, keyPrefix, baseColor)}
    </Text>
  );
}

function renderSideBySideRow(
  row: SideBySideRenderRow,
  index: number,
  paneWidth: number,
  gutterWidth: number
): React.JSX.Element {
  if (row.kind === 'file-header') {
    return (
      <Text key={`side-${index}`} color="cyan">
        {trimToWidth(row.text ?? '', paneWidth)}
      </Text>
    );
  }

  if (row.kind === 'hunk-header') {
    return (
      <Text key={`side-${index}`} color="yellow">
        {trimToWidth(row.text ?? '', paneWidth)}
      </Text>
    );
  }

  if (row.kind === 'collapsed') {
    return (
      <Text key={`side-${index}`} color="gray">
        {trimToWidth(row.text ?? '', paneWidth)}
      </Text>
    );
  }

  const leftWidth = Math.max(10, Math.floor((paneWidth - 3) / 2));
  const rightWidth = Math.max(10, paneWidth - leftWidth - 3);

  return (
    <Box key={`side-${index}`}>
      <Box width={leftWidth}>
        {renderSideCell(row.left, `side-left-${index}`, leftWidth, gutterWidth, 'left')}
      </Box>
      <Text color="gray"> | </Text>
      <Box width={rightWidth}>
        {renderSideCell(row.right, `side-right-${index}`, rightWidth, gutterWidth, 'right')}
      </Box>
    </Box>
  );
}

function buildConflictRows(contents: ConflictFileContents | null): ConflictRenderRow[] {
  if (!contents) {
    return [];
  }

  const sections: Array<{title: string; color: string; body?: string}> = [
    {title: 'BASE', color: 'cyan', body: contents.base},
    {title: 'OURS', color: 'green', body: contents.ours},
    {title: 'THEIRS', color: 'red', body: contents.theirs},
    {title: 'MERGED (working tree)', color: 'yellow', body: contents.merged}
  ];
  const rows: ConflictRenderRow[] = [];

  for (const section of sections) {
    rows.push({
      kind: 'section-header',
      text: section.title,
      color: section.color
    });

    const lines = (section.body ?? '(not available)').split('\n');

    lines.forEach((line, index) => {
      rows.push({
        kind: 'line',
        text: line,
        color: section.color,
        lineNumber: index + 1
      });
    });

    rows.push({
      kind: 'blank',
      text: ''
    });
  }

  return rows;
}

function renderConflictRow(
  row: ConflictRenderRow,
  index: number,
  width: number,
  gutterWidth: number
): React.JSX.Element {
  if (row.kind === 'blank') {
    return <Text key={`conflict-${index}`}> </Text>;
  }

  if (row.kind === 'section-header') {
    return (
      <Text key={`conflict-${index}`} color={row.color} bold>
        {trimToWidth(row.text, width)}
      </Text>
    );
  }

  const lineNumber = row.lineNumber ? String(row.lineNumber).padStart(gutterWidth, ' ') : ' '.repeat(gutterWidth);
  const contentWidth = Math.max(0, width - gutterWidth - 1);
  const trimmed = trimToWidth(row.text, contentWidth);
  const tokens = tokenizeLine(trimmed, 'plain');

  return (
    <Text key={`conflict-${index}`}>
      <Text color="gray">{lineNumber}</Text>
      <Text color="gray"> </Text>
      {renderTokens(tokens, `conflict-${index}`, row.color)}
    </Text>
  );
}

function PaneFrame(props: {
  title: string;
  focused: boolean;
  width?: number;
  children: React.ReactNode;
}) {
  return (
    <Box
      borderStyle="round"
      borderColor={props.focused ? 'green' : 'gray'}
      flexDirection="column"
      width={props.width}
      minHeight={10}
      paddingX={1}
    >
      <Text bold color={props.focused ? 'green' : 'white'}>
        {props.title}
      </Text>
      <Box flexDirection="column" marginTop={1}>
        {props.children}
      </Box>
    </Box>
  );
}

export function App({cwd}: AppProps): React.JSX.Element {
  const {exit} = useApp();
  const {stdout} = useStdout();
  const fileListCache = useRef(new Map<string, FileChange[]>());
  const diffCache = useRef(new Map<string, string>());
  const conflictContentCache = useRef(new Map<string, ConflictFileContents>());
  const [refreshNonce, setRefreshNonce] = useState(0);
  const [data, setData] = useState<DataState>(INITIAL_STATE);
  const [focus, setFocus] = useState<PaneFocus>('commits');
  const [mode, setMode] = useState<ExplorerMode>('history');
  const [presetDiffStyle, setPresetDiffStyle] = useState<DiffStyle>('three-dot');
  const [commitDiffStyle, setCommitDiffStyle] = useState<DiffStyle>('two-dot');
  const [selectedCommitIndex, setSelectedCommitIndex] = useState(0);
  const [selectedFileIndex, setSelectedFileIndex] = useState(0);
  const [compareAnchorSha, setCompareAnchorSha] = useState<string | null>(null);
  const [diffScroll, setDiffScroll] = useState(0);
  const [contextLines, setContextLines] = useState(3);
  const [collapseContext, setCollapseContext] = useState(true);
  const [diffMode, setDiffMode] = useState<DiffPresentationMode>('inline');
  const [activeHunkIndex, setActiveHunkIndex] = useState(0);
  const [searchState, setSearchState] = useState<SearchState>({open: false, target: 'commits'});
  const [commitQuery, setCommitQuery] = useState('');
  const [fileQuery, setFileQuery] = useState('');
  const [comparePickerOpen, setComparePickerOpen] = useState(false);
  const [comparePickerIndex, setComparePickerIndex] = useState(0);
  const [commandPaletteOpen, setCommandPaletteOpen] = useState(false);
  const [commandPaletteQuery, setCommandPaletteQuery] = useState('');
  const [commandPaletteIndex, setCommandPaletteIndex] = useState(0);
  const [actionMessage, setActionMessage] = useState<string | null>(null);
  const [resolvingConflict, setResolvingConflict] = useState(false);

  useEffect(() => {
    let active = true;

    async function loadRepository(): Promise<void> {
      fileListCache.current.clear();
      diffCache.current.clear();
      conflictContentCache.current.clear();
      setPresetDiffStyle('three-dot');
      setCommitDiffStyle('two-dot');
      setFocus('commits');
      setSelectedCommitIndex(0);
      setSelectedFileIndex(0);
      setCompareAnchorSha(null);
      setDiffScroll(0);
      setContextLines(3);
      setCollapseContext(true);
      setDiffMode('inline');
      setActiveHunkIndex(0);
      setCommitQuery('');
      setFileQuery('');
      setSearchState({open: false, target: 'commits'});
      setComparePickerOpen(false);
      setComparePickerIndex(0);
      setCommandPaletteOpen(false);
      setCommandPaletteQuery('');
      setCommandPaletteIndex(0);
      setData(INITIAL_STATE);

      try {
        const repository = await discoverRepository(cwd);
        const [commits, conflictFiles] = await Promise.all([
          listCommits(repository.rootPath),
          listConflictFiles(repository.rootPath)
        ]);

        if (!active) {
          return;
        }

        setMode(conflictFiles.length > 0 ? 'conflict' : 'history');
        setData({
          repository,
          commits,
          files: [],
          conflictFiles,
          diff: '',
          conflictContents: null,
          repositoryError: commits.length === 0 ? 'Repository has no commits yet.' : null,
          filesError: null,
          diffError: null,
          loadingRepository: false,
          loadingFiles: false,
          loadingDiff: false
        });
      } catch (error) {
        if (!active) {
          return;
        }

        setData({
          ...INITIAL_STATE,
          loadingRepository: false,
          repositoryError: getErrorMessage(error)
        });
      }
    }

    void loadRepository();

    return () => {
      active = false;
    };
  }, [cwd, refreshNonce]);

  const filteredCommits = useMemo(
    () => data.commits.filter((commit) => matchesCommit(commit, commitQuery)),
    [commitQuery, data.commits]
  );

  useEffect(() => {
    setSelectedCommitIndex((current) => clamp(current, 0, Math.max(0, filteredCommits.length - 1)));
  }, [filteredCommits.length]);

  const selectedCommit = filteredCommits[selectedCommitIndex] ?? null;
  const compareAnchorCommit = useMemo(
    () => data.commits.find((commit) => commit.sha === compareAnchorSha) ?? null,
    [compareAnchorSha, data.commits]
  );

  const activeComparison = useMemo<CompareSelection | null>(() => {
    if (mode === 'compare-preset' && data.repository?.defaultCompareBase) {
      return {
        mode,
        leftRef: data.repository.defaultCompareBase,
        rightRef: 'HEAD',
        leftLabel: data.repository.defaultCompareBase,
        rightLabel: 'HEAD',
        diffStyle: presetDiffStyle
      };
    }

    if (mode === 'compare-commits' && compareAnchorCommit && selectedCommit) {
      return {
        mode,
        leftRef: compareAnchorCommit.sha,
        rightRef: selectedCommit.sha,
        leftLabel: compareAnchorCommit.shortSha,
        rightLabel: selectedCommit.shortSha,
        diffStyle: commitDiffStyle
      };
    }

    return null;
  }, [compareAnchorCommit, commitDiffStyle, data.repository, mode, presetDiffStyle, selectedCommit]);

  const activeSelectionKey =
    mode === 'conflict'
      ? `conflict:${data.conflictFiles.map((file) => file.path).join('|')}`
      : activeComparison
        ? `${activeComparison.mode}:${activeComparison.leftRef}:${activeComparison.diffStyle}:${activeComparison.rightRef}`
        : `history:${selectedCommit?.sha ?? 'none'}`;

  useEffect(() => {
    setSelectedFileIndex(0);
    setDiffScroll(0);
    setActiveHunkIndex(0);
    setData((current) => ({
      ...current,
      files: mode === 'conflict' ? current.files : [],
      diff: '',
      conflictContents: null,
      filesError: null,
      diffError: null
    }));
  }, [activeSelectionKey, mode]);

  useEffect(() => {
    if (!data.repository) {
      return;
    }

    if (mode === 'conflict') {
      setData((current) => ({
        ...current,
        files: current.conflictFiles.map((file) => ({
          path: file.path,
          status: file.status
        })),
        loadingFiles: false,
        filesError: current.conflictFiles.length === 0 ? 'No conflicted files remain.' : null
      }));
      return;
    }

    if (!activeComparison && !selectedCommit) {
      setData((current) => ({
        ...current,
        files: [],
        loadingFiles: false,
        filesError: commitQuery ? 'No commits match the current filter.' : null
      }));
      return;
    }

    let active = true;
    setData((current) => ({
      ...current,
      loadingFiles: true,
      filesError: null
    }));

    const fileCacheKey = activeComparison
      ? `range:${activeComparison.leftRef}:${activeComparison.diffStyle}:${activeComparison.rightRef}`
      : `commit:${selectedCommit!.sha}`;
    const cachedFiles = fileListCache.current.get(fileCacheKey);

    if (cachedFiles) {
      setData((current) => ({
        ...current,
        files: cachedFiles,
        loadingFiles: false,
        filesError: cachedFiles.length === 0 ? 'No changed files for this selection.' : null
      }));
      return;
    }

    const loadFiles = activeComparison
      ? listRangeFiles(
          data.repository.rootPath,
          activeComparison.leftRef,
          activeComparison.rightRef,
          activeComparison.diffStyle
        )
      : listCommitFiles(data.repository.rootPath, selectedCommit!.sha);

    void loadFiles
      .then((files) => {
        if (!active) {
          return;
        }

        fileListCache.current.set(fileCacheKey, files);

        let filesError: string | null = null;

        if (files.length === 0) {
          if (activeComparison?.mode === 'compare-commits' && activeComparison.leftRef === activeComparison.rightRef) {
            filesError = 'Anchor selected. Move to another commit to compare.';
          } else if (activeComparison) {
            filesError = 'No changed files in this comparison.';
          } else {
            filesError = 'No changed files in this commit.';
          }
        }

        setData((current) => ({
          ...current,
          files,
          loadingFiles: false,
          filesError
        }));
      })
      .catch((error) => {
        if (!active) {
          return;
        }

        setData((current) => ({
          ...current,
          files: [],
          loadingFiles: false,
          filesError: getErrorMessage(error)
        }));
      });

    return () => {
      active = false;
    };
  }, [activeComparison, commitQuery, data.repository, mode, selectedCommit]);

  useEffect(() => {
    if (!data.repository || mode === 'conflict') {
      return;
    }

    const neighbors = [filteredCommits[selectedCommitIndex - 1], filteredCommits[selectedCommitIndex + 1]].filter(
      (commit): commit is CommitSummary => Boolean(commit)
    );

    for (const commit of neighbors) {
      const cacheKey = `commit:${commit.sha}`;

      if (fileListCache.current.has(cacheKey)) {
        continue;
      }

      void listCommitFiles(data.repository.rootPath, commit.sha)
        .then((files) => {
          fileListCache.current.set(cacheKey, files);
        })
        .catch(() => {
          // Prefetch failures are non-fatal.
        });
    }
  }, [data.repository, filteredCommits, mode, selectedCommitIndex]);

  const filteredFiles = useMemo(
    () => data.files.filter((file) => matchesFile(file, fileQuery)),
    [data.files, fileQuery]
  );

  useEffect(() => {
    setSelectedFileIndex((current) => clamp(current, 0, Math.max(0, filteredFiles.length - 1)));
  }, [filteredFiles.length]);

  const selectedFile = filteredFiles[selectedFileIndex] ?? null;
  const selectedConflict = useMemo(
    () => (mode === 'conflict' && selectedFile ? data.conflictFiles.find((file) => file.path === selectedFile.path) ?? null : null),
    [data.conflictFiles, mode, selectedFile]
  );

  useEffect(() => {
    if (!data.repository) {
      return;
    }

    if (mode === 'conflict') {
      if (!selectedConflict) {
        setData((current) => ({
          ...current,
          conflictContents: null,
          diff: '',
          loadingDiff: false,
          diffError: fileQuery && filteredFiles.length === 0 ? 'No conflicted file matches the current filter.' : null
        }));
        return;
      }

      let active = true;
      setData((current) => ({
        ...current,
        loadingDiff: true,
        diffError: null
      }));

      const conflictCacheKey = selectedConflict.path;
      const cachedConflict = conflictContentCache.current.get(conflictCacheKey);

    if (cachedConflict) {
        setDiffScroll(0);
        setData((current) => ({
          ...current,
          conflictContents: cachedConflict,
          diff: '',
          loadingDiff: false,
          diffError: null
        }));
        return;
      }

      void getConflictFileContents(data.repository.rootPath, selectedConflict.path)
        .then((contents) => {
          if (!active) {
            return;
          }

          conflictContentCache.current.set(conflictCacheKey, contents);
          setDiffScroll(0);
          setData((current) => ({
            ...current,
            conflictContents: contents,
            diff: '',
            loadingDiff: false,
            diffError: null
          }));
        })
        .catch((error) => {
          if (!active) {
            return;
          }

          setData((current) => ({
            ...current,
            conflictContents: null,
            diff: '',
            loadingDiff: false,
            diffError: getErrorMessage(error)
          }));
        });

      return () => {
        active = false;
      };
    }

    if (!activeComparison && !selectedCommit) {
      setData((current) => ({
        ...current,
        diff: '',
        conflictContents: null,
        loadingDiff: false,
        diffError: null
      }));
      return;
    }

    if (fileQuery && filteredFiles.length === 0 && data.files.length > 0) {
      setData((current) => ({
        ...current,
        diff: '',
        conflictContents: null,
        loadingDiff: false,
        diffError: 'No file matches the current filter.'
      }));
      return;
    }

    let active = true;
    setData((current) => ({
      ...current,
      loadingDiff: true,
      diffError: null
    }));

    const diffCacheKey = activeComparison
      ? `range:${activeComparison.leftRef}:${activeComparison.diffStyle}:${activeComparison.rightRef}:${selectedFile?.path ?? '*'}:${contextLines}`
      : `commit:${selectedCommit!.sha}:${selectedFile?.path ?? '*'}:${contextLines}`;
    const cachedDiff = diffCache.current.get(diffCacheKey);

    if (cachedDiff !== undefined) {
      setDiffScroll(0);
      setActiveHunkIndex(0);
      setData((current) => ({
        ...current,
        diff: cachedDiff,
        conflictContents: null,
        loadingDiff: false,
        diffError: cachedDiff ? null : activeComparison ? 'No diff available for this comparison.' : 'No diff available.'
      }));
      return;
    }

    const loadDiff = activeComparison
      ? getRangeDiff(
          data.repository.rootPath,
          activeComparison.leftRef,
          activeComparison.rightRef,
          activeComparison.diffStyle,
          selectedFile?.path,
          contextLines
        )
      : getCommitDiff(data.repository.rootPath, selectedCommit!.sha, selectedFile?.path, contextLines);

    void loadDiff
      .then((diff) => {
        if (!active) {
          return;
        }

        diffCache.current.set(diffCacheKey, diff);
        setDiffScroll(0);
        setActiveHunkIndex(0);
        setData((current) => ({
          ...current,
          diff,
          conflictContents: null,
          loadingDiff: false,
          diffError: diff ? null : activeComparison ? 'No diff available for this comparison.' : 'No diff available.'
        }));
      })
      .catch((error) => {
        if (!active) {
          return;
        }

        setData((current) => ({
          ...current,
          diff: '',
          conflictContents: null,
          loadingDiff: false,
          diffError: getErrorMessage(error)
        }));
      });

    return () => {
      active = false;
    };
  }, [activeComparison, contextLines, data.files.length, data.repository, fileQuery, filteredFiles.length, mode, selectedCommit, selectedConflict, selectedFile]);

  const parsedDiff = useMemo(() => parseUnifiedDiff(data.diff), [data.diff]);
  const inlineRender = useMemo(
    () => buildInlineRender(parsedDiff, {collapseContext, collapseKeep: 2}),
    [collapseContext, parsedDiff]
  );
  const sideBySideRender = useMemo(
    () => buildSideBySideRender(parsedDiff, {collapseContext, collapseKeep: 2}),
    [collapseContext, parsedDiff]
  );
  const conflictRows = useMemo(() => buildConflictRows(data.conflictContents), [data.conflictContents]);
  const totalColumns = stdout.columns ?? 120;
  const totalRows = stdout.rows ?? 24;
  const commitWidth = Math.max(36, Math.floor(totalColumns * 0.28));
  const fileWidth = Math.max(28, Math.floor(totalColumns * 0.22));
  const diffWidth = Math.max(52, totalColumns - commitWidth - fileWidth - 10);
  const hasOverlay = comparePickerOpen || searchState.open || commandPaletteOpen;
  const diffHeight = Math.max(8, totalRows - (hasOverlay ? 17 : 13));
  const listHeight = Math.max(8, totalRows - (hasOverlay ? 19 : 15));
  const activeRender = diffMode === 'inline' ? inlineRender : sideBySideRender;
  const activeDiffRows = mode === 'conflict' ? conflictRows : activeRender.rows;

  useEffect(() => {
    if (mode !== 'conflict') {
      setActiveHunkIndex((current) => clamp(current, 0, Math.max(0, activeRender.hunkRowIndexes.length - 1)));
    }
  }, [activeRender.hunkRowIndexes.length, mode]);

  const diffWindow = activeDiffRows.slice(diffScroll, diffScroll + diffHeight);
  const commitWindow = createWindow(filteredCommits, selectedCommitIndex, listHeight);
  const fileWindow = createWindow(filteredFiles, selectedFileIndex, listHeight);
  const maxLineNumber = useMemo(() => {
    if (mode === 'conflict') {
      return conflictRows.reduce((max, row) => Math.max(max, row.lineNumber ?? 0), 0);
    }

    let value = 0;

    for (const file of parsedDiff.files) {
      for (const hunk of file.hunks) {
        for (const line of hunk.lines) {
          value = Math.max(value, line.oldLineNumber ?? 0, line.newLineNumber ?? 0);
        }
      }
    }

    return value;
  }, [conflictRows, mode, parsedDiff.files]);
  const gutterWidth = Math.max(3, String(maxLineNumber || 0).length);
  const activeModeLabel =
    mode === 'conflict'
      ? 'Conflict Mode'
      : activeComparison
        ? `Compare ${formatComparisonSpec(activeComparison)}`
        : 'History selected commit';
  const comparePresetLabel = data.repository?.defaultCompareBase ? `${data.repository.defaultCompareBase}...HEAD` : 'unavailable';
  const hunkLabel =
    mode === 'conflict'
      ? '-'
      : activeRender.hunkRowIndexes.length > 0
        ? `${activeHunkIndex + 1}/${activeRender.hunkRowIndexes.length}`
        : '0/0';
  const commitTitle =
    mode === 'conflict'
      ? 'Conflict State'
      : commitQuery
        ? `Commits (${filteredCommits.length}/${data.commits.length})`
        : `Commits (${data.commits.length})`;
  const fileTitle =
    mode === 'conflict'
      ? fileQuery
        ? `Conflicts (${filteredFiles.length}/${data.conflictFiles.length})`
        : `Conflicts (${data.conflictFiles.length})`
      : fileQuery
        ? `Files (${filteredFiles.length}/${data.files.length})`
        : `Files (${data.files.length})`;

  const compareOptions = useMemo<CompareOption[]>(() => {
    const options: CompareOption[] = [
      {
        key: 'history',
        label: 'History Mode',
        detail: 'Use the selected commit against its parent.',
        run: () => {
          setMode('history');
          setComparePickerOpen(false);
        }
      }
    ];

    if (data.repository?.defaultCompareBase) {
      options.push({
        key: 'preset-three-dot',
        label: `${data.repository.defaultCompareBase}...HEAD`,
        detail: 'Merge-base comparison from the default branch to HEAD.',
        run: () => {
          setPresetDiffStyle('three-dot');
          setMode('compare-preset');
          setComparePickerOpen(false);
        }
      });
      options.push({
        key: 'preset-two-dot',
        label: `${data.repository.defaultCompareBase}..HEAD`,
        detail: 'Direct range comparison from the default branch to HEAD.',
        run: () => {
          setPresetDiffStyle('two-dot');
          setMode('compare-preset');
          setComparePickerOpen(false);
        }
      });
    }

    options.push({
      key: 'anchor-current',
      label: compareAnchorCommit ? `Anchor ${compareAnchorCommit.shortSha}` : 'Anchor Selected Commit',
      detail: compareAnchorCommit
        ? 'Keep the current anchor and compare from it.'
        : 'Mark the selected commit as the left side for commit-to-commit compare.',
      run: () => {
        if (selectedCommit) {
          setCompareAnchorSha(selectedCommit.sha);
        }
        setComparePickerOpen(false);
      },
      disabled: !selectedCommit
    });

    options.push({
      key: 'anchored-two-dot',
      label: compareAnchorCommit && selectedCommit ? `${compareAnchorCommit.shortSha}..${selectedCommit.shortSha}` : 'Anchored Commit Compare (..)',
      detail: 'Direct commit-to-commit range using the current anchor and selection.',
      run: () => {
        setCommitDiffStyle('two-dot');
        setMode('compare-commits');
        setComparePickerOpen(false);
      },
      disabled: !compareAnchorCommit || !selectedCommit
    });

    options.push({
      key: 'anchored-three-dot',
      label: compareAnchorCommit && selectedCommit ? `${compareAnchorCommit.shortSha}...${selectedCommit.shortSha}` : 'Anchored Commit Compare (...)',
      detail: 'Merge-base commit comparison using the current anchor and selection.',
      run: () => {
        setCommitDiffStyle('three-dot');
        setMode('compare-commits');
        setComparePickerOpen(false);
      },
      disabled: !compareAnchorCommit || !selectedCommit
    });

    return options;
  }, [compareAnchorCommit, data.repository, selectedCommit]);

  const commandActions = useMemo<CommandAction[]>(() => {
    const actions: CommandAction[] = [
      {
        key: 'refresh',
        label: 'Refresh repository state',
        detail: 'Reload commits, files, conflict state, and clear in-memory caches.',
        run: () => {
          setCommandPaletteOpen(false);
          setRefreshNonce((current) => current + 1);
        }
      },
      {
        key: 'clear-filters',
        label: 'Clear filters',
        detail: 'Reset commit and file filter queries.',
        run: () => {
          setCommitQuery('');
          setFileQuery('');
          setCommandPaletteOpen(false);
        }
      },
      {
        key: 'open-editor',
        label: 'Open current file in editor',
        detail: 'Open the selected file in your external editor.',
        disabled: !selectedFile || !data.repository,
        run: () => {
          if (!selectedFile || !data.repository) {
            return;
          }

          try {
            const result = openFileInEditor(data.repository.rootPath, selectedFile.path);
            setCommandPaletteOpen(false);
            if (result.mode === 'handoff') {
              exit();
              return;
            }
            setActionMessage(`Opened ${selectedFile.path} in ${result.command}.`);
          } catch (error) {
            setActionMessage(`Editor launch failed: ${getErrorMessage(error)}`);
          }
        }
      }
    ];

    if (mode === 'conflict') {
      actions.push(
        {
          key: 'accept-ours',
          label: 'Accept ours for selected conflict',
          detail: 'Resolve the selected conflicted file using the current branch version.',
          disabled: !selectedConflict || resolvingConflict,
          run: () => {
            setCommandPaletteOpen(false);
            handleConflictResolution('ours');
          }
        },
        {
          key: 'accept-theirs',
          label: 'Accept theirs for selected conflict',
          detail: 'Resolve the selected conflicted file using the incoming branch version.',
          disabled: !selectedConflict || resolvingConflict,
          run: () => {
            setCommandPaletteOpen(false);
            handleConflictResolution('theirs');
          }
        }
      );
    } else {
      actions.push(
        {
          key: 'history',
          label: 'Switch to history mode',
          detail: 'Show the selected commit against its parent.',
          run: () => {
            setMode('history');
            setCommandPaletteOpen(false);
          }
        },
        {
          key: 'compare-preset',
          label: `Compare ${comparePresetLabel}`,
          detail: 'Switch to the default branch preset comparison.',
          disabled: !data.repository?.defaultCompareBase,
          run: () => {
            setPresetDiffStyle('three-dot');
            setMode('compare-preset');
            setCommandPaletteOpen(false);
          }
        },
        {
          key: 'toggle-diff-mode',
          label: `Toggle diff view (${diffMode})`,
          detail: 'Switch between inline and side-by-side diff rendering.',
          run: () => {
            setDiffMode((current) => (current === 'inline' ? 'side-by-side' : 'inline'));
            setCommandPaletteOpen(false);
          }
        },
        {
          key: 'toggle-collapse',
          label: `${collapseContext ? 'Expand' : 'Collapse'} unchanged context`,
          detail: 'Toggle collapsed unchanged sections in the diff view.',
          run: () => {
            setCollapseContext((current) => !current);
            setCommandPaletteOpen(false);
          }
        }
      );
    }

    return actions.filter((action) => {
      if (!commandPaletteQuery) {
        return true;
      }

      const haystack = `${action.label} ${action.detail}`.toLowerCase();
      return haystack.includes(commandPaletteQuery.toLowerCase());
    });
  }, [
    collapseContext,
    commandPaletteQuery,
    comparePresetLabel,
    data.repository,
    diffMode,
    exit,
    mode,
    resolvingConflict,
    selectedConflict,
    selectedFile
  ]);

  useEffect(() => {
    setComparePickerIndex((current) => clamp(current, 0, Math.max(0, compareOptions.length - 1)));
  }, [compareOptions.length]);

  useEffect(() => {
    setCommandPaletteIndex((current) => clamp(current, 0, Math.max(0, commandActions.length - 1)));
  }, [commandActions.length]);

  const jumpToHunk = (nextIndex: number): void => {
    if (mode === 'conflict' || activeRender.hunkRowIndexes.length === 0) {
      return;
    }

    const clamped = clamp(nextIndex, 0, activeRender.hunkRowIndexes.length - 1);
    setActiveHunkIndex(clamped);
    setDiffScroll(clamp(activeRender.hunkRowIndexes[clamped] - 1, 0, Math.max(0, activeDiffRows.length - diffHeight)));
    setFocus('diff');
  };

  function handleConflictResolution(side: 'ours' | 'theirs'): void {
    if (!data.repository || !selectedConflict || resolvingConflict) {
      return;
    }

    setResolvingConflict(true);
    setActionMessage(`Applying ${side} for ${selectedConflict.path}...`);

    void acceptConflictSide(data.repository.rootPath, selectedConflict.path, side)
      .then(() => {
        setActionMessage(`Applied ${side} and staged ${selectedConflict.path}.`);
        setRefreshNonce((current) => current + 1);
      })
      .catch((error) => {
        setActionMessage(`Conflict action failed: ${getErrorMessage(error)}`);
      })
      .finally(() => {
        setResolvingConflict(false);
      });
  }

  useInput((input, key) => {
    if (commandPaletteOpen) {
      if (key.escape) {
        setCommandPaletteOpen(false);
        setCommandPaletteQuery('');
        return;
      }

      if (key.return) {
        const action = commandActions[commandPaletteIndex];

        if (action && !action.disabled) {
          action.run();
        }
        return;
      }

      if (key.backspace || key.delete) {
        setCommandPaletteQuery((current) => current.slice(0, -1));
        return;
      }

      if (input === 'j' || key.downArrow) {
        setCommandPaletteIndex((current) => clamp(current + 1, 0, Math.max(0, commandActions.length - 1)));
        return;
      }

      if (input === 'k' || key.upArrow) {
        setCommandPaletteIndex((current) => clamp(current - 1, 0, Math.max(0, commandActions.length - 1)));
        return;
      }

      if (!key.ctrl && !key.meta && input) {
        setCommandPaletteQuery((current) => current + input);
      }

      return;
    }

    if (searchState.open) {
      const updateQuery = (updater: (current: string) => string): void => {
        if (searchState.target === 'commits') {
          setCommitQuery((current) => updater(current));
        } else {
          setFileQuery((current) => updater(current));
        }
      };

      if (key.escape) {
        setSearchState((current) => ({...current, open: false}));
        return;
      }

      if (key.return) {
        setSearchState((current) => ({...current, open: false}));
        return;
      }

      if (key.backspace || key.delete) {
        updateQuery((current) => current.slice(0, -1));
        return;
      }

      if (!key.ctrl && !key.meta && input) {
        updateQuery((current) => current + input);
      }

      return;
    }

    if (comparePickerOpen) {
      if (key.escape || input === 'q' || input === 'c') {
        setComparePickerOpen(false);
        return;
      }

      if (input === 'j' || key.downArrow) {
        setComparePickerIndex((current) => clamp(current + 1, 0, Math.max(0, compareOptions.length - 1)));
        return;
      }

      if (input === 'k' || key.upArrow) {
        setComparePickerIndex((current) => clamp(current - 1, 0, Math.max(0, compareOptions.length - 1)));
        return;
      }

      if (key.return || input === 'l') {
        const option = compareOptions[comparePickerIndex];

        if (option && !option.disabled) {
          option.run();
        }
      }

      return;
    }

    if (key.escape || input === 'q') {
      exit();
      return;
    }

    if (key.tab) {
      setFocus((current) => {
        if (current === 'commits') {
          return 'files';
        }

        if (current === 'files') {
          return 'diff';
        }

        return 'commits';
      });
      return;
    }

    if (input === '/') {
      setSearchState({
        open: true,
        target: focus === 'commits' && mode !== 'conflict' ? 'commits' : 'files'
      });
      return;
    }

    if (input === ':') {
      setCommandPaletteOpen(true);
      setCommandPaletteQuery('');
      setCommandPaletteIndex(0);
      return;
    }

    if (input === 'o' && selectedFile && data.repository) {
      try {
        const result = openFileInEditor(data.repository.rootPath, selectedFile.path);
        if (result.mode === 'handoff') {
          exit();
          return;
        }
        setActionMessage(`Opened ${selectedFile.path} in ${result.command}.`);
      } catch (error) {
        setActionMessage(`Editor launch failed: ${getErrorMessage(error)}`);
      }
      return;
    }

    if (input === 'c' && mode !== 'conflict') {
      setComparePickerOpen((current) => !current);
      return;
    }

    if (input === 'g' && mode !== 'conflict') {
      setMode('history');
      return;
    }

    if (input === 'r' && mode === 'conflict') {
      setRefreshNonce((current) => current + 1);
      return;
    }

    if (input === '1' && mode === 'conflict') {
      handleConflictResolution('ours');
      return;
    }

    if (input === '2' && mode === 'conflict') {
      handleConflictResolution('theirs');
      return;
    }

    if (input === 'v' && selectedCommit && mode !== 'conflict') {
      if (compareAnchorSha === selectedCommit.sha) {
        setCompareAnchorSha(null);
        if (mode === 'compare-commits') {
          setMode('history');
        }
      } else {
        setCompareAnchorSha(selectedCommit.sha);
      }
      return;
    }

    if (input === 'i' && mode !== 'conflict') {
      setDiffMode((current) => (current === 'inline' ? 'side-by-side' : 'inline'));
      return;
    }

    if (input === 'z' && mode !== 'conflict') {
      setCollapseContext((current) => !current);
      return;
    }

    if ((input === '-' || input === '=' || input === '+') && mode === 'conflict') {
      return;
    }

    if (input === '-') {
      setContextLines((current) => Math.max(0, current - 1));
      return;
    }

    if (input === '=' || input === '+') {
      setContextLines((current) => Math.min(20, current + 1));
      return;
    }

    if (input === '[') {
      jumpToHunk(activeHunkIndex - 1);
      return;
    }

    if (input === ']') {
      jumpToHunk(activeHunkIndex + 1);
      return;
    }

    if (input === '{') {
      setFocus('files');
      setSelectedFileIndex((current) => Math.max(current - 1, 0));
      return;
    }

    if (input === '}') {
      setFocus('files');
      setSelectedFileIndex((current) => Math.min(current + 1, Math.max(0, filteredFiles.length - 1)));
      return;
    }

    if (input === 'h') {
      setFocus((current) => (current === 'diff' ? 'files' : 'commits'));
      return;
    }

    if (input === 'l') {
      setFocus((current) => (current === 'commits' ? 'files' : 'diff'));
      return;
    }

    if (input === 'j' || key.downArrow) {
      if (focus === 'commits') {
        if (mode !== 'conflict') {
          setSelectedCommitIndex((current) => clamp(current + 1, 0, Math.max(0, filteredCommits.length - 1)));
        }
      } else if (focus === 'files') {
        setSelectedFileIndex((current) => clamp(current + 1, 0, Math.max(0, filteredFiles.length - 1)));
      } else {
        setDiffScroll((current) => clamp(current + 1, 0, Math.max(0, activeDiffRows.length - diffHeight)));
      }
      return;
    }

    if (input === 'k' || key.upArrow) {
      if (focus === 'commits') {
        if (mode !== 'conflict') {
          setSelectedCommitIndex((current) => Math.max(current - 1, 0));
        }
      } else if (focus === 'files') {
        setSelectedFileIndex((current) => Math.max(current - 1, 0));
      } else {
        setDiffScroll((current) => Math.max(current - 1, 0));
      }
    }
  });

  return (
    <Box flexDirection="column">
      <Box marginBottom={1} flexDirection="column">
        <Text bold color="cyan">
          Better Diff
        </Text>
        <Text color="gray">
          {data.repository
            ? `Repo: ${data.repository.rootPath} | Branch: ${data.repository.headRef}`
            : `Target: ${cwd}`}
        </Text>
        <Text color={mode === 'conflict' ? 'red' : activeComparison ? 'yellow' : 'green'}>{`Mode: ${activeModeLabel}`}</Text>
        {compareAnchorCommit && mode !== 'conflict' ? (
          <Text color="gray">{`Anchor: ${compareAnchorCommit.shortSha} ${compareAnchorCommit.subject}`}</Text>
        ) : null}
        {mode === 'conflict' && data.repository ? (
          <Text color="gray">
            {`Repo state: merge ${data.repository.isMergeInProgress ? 'yes' : 'no'}, rebase ${data.repository.isRebaseInProgress ? 'yes' : 'no'}, cherry-pick ${data.repository.isCherryPickInProgress ? 'yes' : 'no'}`}
          </Text>
        ) : null}
        <Text color="gray">
          {mode === 'conflict'
            ? `Keys: h/j/k/l navigate, / filter conflicts, 1 accept ours, 2 accept theirs, r refresh, q quit`
            : `Keys: h/j/k/l navigate, : commands, c compare, / search, o editor, i ${diffMode}, z collapse ${collapseContext ? 'on' : 'off'}, [ ] hunk ${hunkLabel}, { } file, +/- context ${contextLines}, q quit`}
        </Text>
        {commitQuery && mode !== 'conflict' ? <Text color="gray">{`Commit filter: ${commitQuery}`}</Text> : null}
        {fileQuery ? <Text color="gray">{`${mode === 'conflict' ? 'Conflict' : 'File'} filter: ${fileQuery}`}</Text> : null}
        {actionMessage ? <Text color={actionMessage.includes('failed') ? 'red' : 'yellow'}>{actionMessage}</Text> : null}
      </Box>

      {data.repositoryError && !data.repository ? (
        <Box borderStyle="round" borderColor="red" paddingX={1} flexDirection="column">
          <Text bold color="red">
            Repository unavailable
          </Text>
          <Text>{data.repositoryError}</Text>
          <Text color="gray">Open the app from inside a Git repository or pass a repo path as the first argument.</Text>
        </Box>
      ) : (
        <Box gap={1}>
          <PaneFrame title={commitTitle} focused={focus === 'commits'} width={commitWidth}>
            {mode === 'conflict' ? (
              <Box flexDirection="column">
                <Text color="red">{`${data.conflictFiles.length} conflicted file${data.conflictFiles.length === 1 ? '' : 's'} detected`}</Text>
                <Text color="gray">{data.repository?.isMergeInProgress ? 'Merge in progress.' : 'Conflict state detected outside a merge as well.'}</Text>
                {selectedConflict ? (
                  <>
                    <Text color="gray">{`Selected: ${selectedConflict.path}`}</Text>
                    <Text color="gray">{`Stages: base ${selectedConflict.hasBase ? 'yes' : 'no'}, ours ${selectedConflict.hasOurs ? 'yes' : 'no'}, theirs ${selectedConflict.hasTheirs ? 'yes' : 'no'}`}</Text>
                  </>
                ) : null}
                <Text color="gray">Use `1` to accept ours or `2` to accept theirs for the selected file.</Text>
                <Text color="gray">The command stages the chosen version immediately.</Text>
              </Box>
            ) : (
              <>
                {data.loadingRepository ? <Text color="yellow">Loading history...</Text> : null}
                {!data.loadingRepository && commitWindow.length === 0 ? <Text color="gray">No commits loaded.</Text> : null}
                {commitWindow.map(({item, index}) => {
                  const refs = item.refs.length > 0 ? ` [${item.refs.join(', ')}]` : '';
                  const prefix = index === selectedCommitIndex ? (item.sha === compareAnchorSha ? '*> ' : '>  ') : item.sha === compareAnchorSha ? '*  ' : '   ';
                  const line = trimToWidth(`${item.graph}${item.shortSha} ${item.subject}${refs}`, commitWidth - 8);

                  return (
                    <Text key={item.sha} color={index === selectedCommitIndex ? 'green' : undefined}>
                      {prefix}
                      {line}
                    </Text>
                  );
                })}
                {selectedCommit ? (
                  <Box marginTop={1} flexDirection="column">
                    <Text color="gray">{`${selectedCommit.authoredAt} by ${selectedCommit.authorName}`}</Text>
                  </Box>
                ) : null}
              </>
            )}
          </PaneFrame>

          <PaneFrame title={fileTitle} focused={focus === 'files'} width={fileWidth}>
            {data.loadingFiles ? <Text color="yellow">Loading files...</Text> : null}
            {data.filesError && !data.loadingFiles ? <Text color="gray">{data.filesError}</Text> : null}
            {!data.loadingFiles &&
              fileWindow.map(({item, index}) => {
                const oldPath = item.oldPath ? ` <- ${item.oldPath}` : '';
                const line = trimToWidth(`${item.status} ${item.path}${oldPath}`, fileWidth - 6);
                return (
                  <Text key={`${item.status}-${item.path}`} color={index === selectedFileIndex ? 'green' : item.status === 'U' ? 'red' : undefined}>
                    {index === selectedFileIndex ? '> ' : '  '}
                    {line}
                  </Text>
                );
              })}
          </PaneFrame>

          <PaneFrame
            title={
              mode === 'conflict'
                ? selectedConflict
                  ? `Conflict: ${selectedConflict.path}`
                  : 'Conflict Viewer'
                : selectedFile
                  ? `Diff: ${selectedFile.path}`
                  : activeComparison
                    ? `Diff Preview (${formatComparisonSpec(activeComparison)})`
                    : 'Diff Preview'
            }
            focused={focus === 'diff'}
            width={diffWidth}
          >
            {data.loadingDiff ? <Text color="yellow">{mode === 'conflict' ? 'Loading conflict view...' : 'Loading diff...'}</Text> : null}
            {data.diffError && !data.loadingDiff ? <Text color="gray">{data.diffError}</Text> : null}
            {!data.loadingDiff && diffWindow.length === 0 ? <Text color="gray">{mode === 'conflict' ? 'No conflict content loaded.' : 'No diff loaded.'}</Text> : null}
            {!data.loadingDiff &&
              diffWindow.map((row, index) =>
                mode === 'conflict'
                  ? renderConflictRow(row as ConflictRenderRow, diffScroll + index, diffWidth - 4, gutterWidth)
                  : diffMode === 'inline'
                    ? renderInlineRow(row as InlineRenderRow, diffScroll + index, gutterWidth, diffWidth - 4)
                    : renderSideBySideRow(row as SideBySideRenderRow, diffScroll + index, diffWidth - 4, gutterWidth)
              )}
          </PaneFrame>
        </Box>
      )}

      {comparePickerOpen ? (
        <Box marginTop={1} borderStyle="round" borderColor="yellow" paddingX={1} flexDirection="column">
          <Text bold color="yellow">
            Compare Picker
          </Text>
          {compareOptions.map((option, index) => (
            <Box key={option.key} flexDirection="column" marginTop={index === 0 ? 1 : 0}>
              <Text color={option.disabled ? 'gray' : index === comparePickerIndex ? 'green' : undefined}>
                {index === comparePickerIndex ? '> ' : '  '}
                {option.label}
              </Text>
              <Text color="gray">{trimToWidth(option.detail, totalColumns - 8)}</Text>
            </Box>
          ))}
          <Text color="gray">Use j/k to pick, enter to apply, c or q to close.</Text>
        </Box>
      ) : null}

      {searchState.open ? (
        <Box marginTop={1} borderStyle="round" borderColor="cyan" paddingX={1} flexDirection="column">
          <Text bold color="cyan">
            {`Search ${searchState.target}`}
          </Text>
          <Text>
            {searchState.target === 'commits' ? commitQuery : fileQuery}
            <Text color="gray">_</Text>
          </Text>
          <Text color="gray">Type to filter live. Enter closes. Backspace edits. Esc keeps the current filter and closes.</Text>
        </Box>
      ) : null}

      {commandPaletteOpen ? (
        <Box marginTop={1} borderStyle="round" borderColor="magenta" paddingX={1} flexDirection="column">
          <Text bold color="magenta">
            Command Palette
          </Text>
          <Text>
            {commandPaletteQuery}
            <Text color="gray">_</Text>
          </Text>
          {commandActions.map((action, index) => (
            <Box key={action.key} flexDirection="column" marginTop={index === 0 ? 1 : 0}>
              <Text color={action.disabled ? 'gray' : index === commandPaletteIndex ? 'green' : undefined}>
                {index === commandPaletteIndex ? '> ' : '  '}
                {action.label}
              </Text>
              <Text color="gray">{trimToWidth(action.detail, totalColumns - 8)}</Text>
            </Box>
          ))}
          <Text color="gray">Type to filter. j/k move. Enter runs. Esc closes.</Text>
        </Box>
      ) : null}

      {!comparePickerOpen && !searchState.open && !commandPaletteOpen ? (
        <Box marginTop={1}>
          <Text color="gray">
            {mode === 'conflict'
              ? 'Conflict mode activates automatically when unresolved files exist. Vim motions remain the default navigation model.'
              : `Preset compare base: ${comparePresetLabel}. Vim motions remain the default navigation model.`}
          </Text>
        </Box>
      ) : null}
    </Box>
  );
}
