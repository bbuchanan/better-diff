import React, {useEffect, useMemo, useState} from 'react';
import {Box, Text, useApp, useInput, useStdout} from 'ink';

import type {
  CommitSummary,
  CompareSelection,
  DiffStyle,
  ExplorerMode,
  FileChange,
  PaneFocus,
  RepositoryInfo
} from '../domain/models.js';
import {
  GitCommandError,
  discoverRepository,
  getCommitDiff,
  getRangeDiff,
  listCommitFiles,
  listCommits,
  listRangeFiles
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
  diff: string;
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

type DiffPresentationMode = 'inline' | 'side-by-side';

const INITIAL_STATE: DataState = {
  repository: null,
  commits: [],
  files: [],
  diff: '',
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

function sanitizeText(value: string): string {
  return value.replace(/[\u0000-\u0008\u000b-\u001f\u007f-\u009f]/g, '');
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

function matchesFile(file: FileChange, query: string): boolean {
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
        text: `${token.text.slice(0, remaining)}`
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
  const lineNumber =
    side === 'left'
      ? current.oldLineNumber
      : current.newLineNumber;
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

  useEffect(() => {
    let active = true;

    async function loadRepository(): Promise<void> {
      setMode('history');
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
      setData(INITIAL_STATE);

      try {
        const repository = await discoverRepository(cwd);
        const commits = await listCommits(repository.rootPath);

        if (!active) {
          return;
        }

        setData({
          repository,
          commits,
          files: [],
          diff: '',
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
  }, [cwd]);

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

  const activeSelectionKey = activeComparison
    ? `${activeComparison.mode}:${activeComparison.leftRef}:${activeComparison.diffStyle}:${activeComparison.rightRef}`
    : `history:${selectedCommit?.sha ?? 'none'}`;

  useEffect(() => {
    setSelectedFileIndex(0);
    setDiffScroll(0);
    setActiveHunkIndex(0);
    setData((current) => ({
      ...current,
      files: [],
      diff: '',
      filesError: null,
      diffError: null
    }));
  }, [activeSelectionKey]);

  useEffect(() => {
    if (!data.repository) {
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
  }, [activeComparison, commitQuery, data.repository, selectedCommit]);

  const filteredFiles = useMemo(
    () => data.files.filter((file) => matchesFile(file, fileQuery)),
    [data.files, fileQuery]
  );

  useEffect(() => {
    setSelectedFileIndex((current) => clamp(current, 0, Math.max(0, filteredFiles.length - 1)));
  }, [filteredFiles.length]);

  const selectedFile = filteredFiles[selectedFileIndex] ?? null;

  useEffect(() => {
    if (!data.repository) {
      return;
    }

    if (!activeComparison && !selectedCommit) {
      setData((current) => ({
        ...current,
        diff: '',
        loadingDiff: false,
        diffError: null
      }));
      return;
    }

    if (fileQuery && filteredFiles.length === 0 && data.files.length > 0) {
      setData((current) => ({
        ...current,
        diff: '',
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

        setDiffScroll(0);
        setActiveHunkIndex(0);
        setData((current) => ({
          ...current,
          diff,
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
          loadingDiff: false,
          diffError: getErrorMessage(error)
        }));
      });

    return () => {
      active = false;
    };
  }, [activeComparison, contextLines, data.files.length, data.repository, fileQuery, filteredFiles.length, selectedCommit, selectedFile]);

  const parsedDiff = useMemo(() => parseUnifiedDiff(data.diff), [data.diff]);
  const collapseKeep = 2;
  const inlineRender = useMemo(
    () => buildInlineRender(parsedDiff, {collapseContext, collapseKeep}),
    [collapseContext, parsedDiff]
  );
  const sideBySideRender = useMemo(
    () => buildSideBySideRender(parsedDiff, {collapseContext, collapseKeep}),
    [collapseContext, parsedDiff]
  );
  const activeRender = diffMode === 'inline' ? inlineRender : sideBySideRender;

  useEffect(() => {
    setActiveHunkIndex((current) => clamp(current, 0, Math.max(0, activeRender.hunkRowIndexes.length - 1)));
  }, [activeRender.hunkRowIndexes.length]);

  const totalColumns = stdout.columns ?? 120;
  const totalRows = stdout.rows ?? 24;
  const commitWidth = Math.max(36, Math.floor(totalColumns * 0.28));
  const fileWidth = Math.max(28, Math.floor(totalColumns * 0.22));
  const diffWidth = Math.max(52, totalColumns - commitWidth - fileWidth - 10);
  const diffHeight = Math.max(8, totalRows - (comparePickerOpen || searchState.open ? 16 : 12));
  const listHeight = Math.max(8, totalRows - (comparePickerOpen || searchState.open ? 18 : 14));
  const commitWindow = createWindow(filteredCommits, selectedCommitIndex, listHeight);
  const fileWindow = createWindow(filteredFiles, selectedFileIndex, listHeight);
  const diffRows = activeRender.rows;
  const diffWindow = diffRows.slice(diffScroll, diffScroll + diffHeight);
  const maxLineNumber = useMemo(() => {
    let value = 0;

    for (const file of parsedDiff.files) {
      for (const hunk of file.hunks) {
        for (const line of hunk.lines) {
          value = Math.max(value, line.oldLineNumber ?? 0, line.newLineNumber ?? 0);
        }
      }
    }

    return value;
  }, [parsedDiff.files]);
  const gutterWidth = Math.max(3, String(maxLineNumber || 0).length);
  const activeModeLabel = activeComparison ? `Compare ${formatComparisonSpec(activeComparison)}` : 'History selected commit';
  const comparePresetLabel = data.repository?.defaultCompareBase ? `${data.repository.defaultCompareBase}...HEAD` : 'unavailable';
  const hunkLabel =
    activeRender.hunkRowIndexes.length > 0 ? `${activeHunkIndex + 1}/${activeRender.hunkRowIndexes.length}` : '0/0';
  const commitTitle = commitQuery
    ? `Commits (${filteredCommits.length}/${data.commits.length})`
    : `Commits (${data.commits.length})`;
  const fileTitle = fileQuery ? `Files (${filteredFiles.length}/${data.files.length})` : `Files (${data.files.length})`;

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

  useEffect(() => {
    setComparePickerIndex((current) => clamp(current, 0, Math.max(0, compareOptions.length - 1)));
  }, [compareOptions.length]);

  const jumpToHunk = (nextIndex: number): void => {
    if (activeRender.hunkRowIndexes.length === 0) {
      return;
    }

    const clamped = clamp(nextIndex, 0, activeRender.hunkRowIndexes.length - 1);
    setActiveHunkIndex(clamped);
    setDiffScroll(clamp(activeRender.hunkRowIndexes[clamped] - 1, 0, Math.max(0, diffRows.length - diffHeight)));
    setFocus('diff');
  };

  useInput((input, key) => {
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
        target: focus === 'files' ? 'files' : 'commits'
      });
      return;
    }

    if (input === 'c') {
      setComparePickerOpen((current) => !current);
      return;
    }

    if (input === 'g') {
      setMode('history');
      return;
    }

    if (input === 'v' && selectedCommit) {
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

    if (input === 'i') {
      setDiffMode((current) => (current === 'inline' ? 'side-by-side' : 'inline'));
      return;
    }

    if (input === 'z') {
      setCollapseContext((current) => !current);
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
        setSelectedCommitIndex((current) => clamp(current + 1, 0, Math.max(0, filteredCommits.length - 1)));
      } else if (focus === 'files') {
        setSelectedFileIndex((current) => clamp(current + 1, 0, Math.max(0, filteredFiles.length - 1)));
      } else {
        setDiffScroll((current) => clamp(current + 1, 0, Math.max(0, diffRows.length - diffHeight)));
      }
      return;
    }

    if (input === 'k' || key.upArrow) {
      if (focus === 'commits') {
        setSelectedCommitIndex((current) => Math.max(current - 1, 0));
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
        <Text color={activeComparison ? 'yellow' : 'green'}>{`Mode: ${activeModeLabel}`}</Text>
        {compareAnchorCommit ? (
          <Text color="gray">{`Anchor: ${compareAnchorCommit.shortSha} ${compareAnchorCommit.subject}`}</Text>
        ) : null}
        <Text color="gray">
          {`Keys: h/j/k/l navigate, c compare, / search, i ${diffMode}, z collapse ${collapseContext ? 'on' : 'off'}, [ ] hunk ${hunkLabel}, { } file, +/- context ${contextLines}, q quit`}
        </Text>
        {commitQuery ? <Text color="gray">{`Commit filter: ${commitQuery}`}</Text> : null}
        {fileQuery ? <Text color="gray">{`File filter: ${fileQuery}`}</Text> : null}
        {!data.repository?.defaultCompareBase ? (
          <Text color="gray">Default branch compare base not found. Preset compare options will stay limited.</Text>
        ) : null}
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
            {data.loadingRepository ? <Text color="yellow">Loading history...</Text> : null}
            {!data.loadingRepository && commitWindow.length === 0 ? <Text color="gray">No commits loaded.</Text> : null}
            {commitWindow.map(({item, index}) => {
              const refs = item.refs.length > 0 ? ` [${item.refs.join(', ')}]` : '';
              const prefix = index === selectedCommitIndex ? (item.sha === compareAnchorSha ? '*> ' : '>  ') : item.sha === compareAnchorSha ? '*  ' : '   ';
              const line = trimToWidth(`${item.shortSha} ${item.subject}${refs}`, commitWidth - 8);

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
          </PaneFrame>

          <PaneFrame title={fileTitle} focused={focus === 'files'} width={fileWidth}>
            {data.loadingFiles ? <Text color="yellow">Loading files...</Text> : null}
            {data.filesError && !data.loadingFiles ? <Text color="gray">{data.filesError}</Text> : null}
            {!data.loadingFiles &&
              fileWindow.map(({item, index}) => {
                const oldPath = item.oldPath ? ` <- ${item.oldPath}` : '';
                const line = trimToWidth(`${item.status} ${item.path}${oldPath}`, fileWidth - 6);
                return (
                  <Text key={`${item.status}-${item.path}`} color={index === selectedFileIndex ? 'green' : undefined}>
                    {index === selectedFileIndex ? '> ' : '  '}
                    {line}
                  </Text>
                );
              })}
          </PaneFrame>

          <PaneFrame
            title={
              selectedFile
                ? `Diff: ${selectedFile.path}`
                : activeComparison
                  ? `Diff Preview (${formatComparisonSpec(activeComparison)})`
                  : 'Diff Preview'
            }
            focused={focus === 'diff'}
            width={diffWidth}
          >
            {data.loadingDiff ? <Text color="yellow">Loading diff...</Text> : null}
            {data.diffError && !data.loadingDiff ? <Text color="gray">{data.diffError}</Text> : null}
            {!data.loadingDiff && diffWindow.length === 0 ? <Text color="gray">No diff loaded.</Text> : null}
            {!data.loadingDiff &&
              diffWindow.map((row, index) =>
                diffMode === 'inline'
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
          <Text color="gray">Type to filter live. Enter closes. Backspace clears. Esc keeps the current filter and closes.</Text>
        </Box>
      ) : null}

      {!comparePickerOpen && !searchState.open ? (
        <Box marginTop={1}>
          <Text color="gray">{`Preset compare base: ${comparePresetLabel}. Vim motions remain the default navigation model.`}</Text>
        </Box>
      ) : null}
    </Box>
  );
}
