import React, {useEffect, useMemo, useState} from 'react';
import {Box, Text, useApp, useInput, useStdout} from 'ink';

import type {
  CommitSummary,
  CompareSelection,
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

function renderDiffLine(line: string, index: number): React.JSX.Element {
  let color: string | undefined;

  if (line.startsWith('+') && !line.startsWith('+++')) {
    color = 'green';
  } else if (line.startsWith('-') && !line.startsWith('---')) {
    color = 'red';
  } else if (line.startsWith('@@')) {
    color = 'yellow';
  } else if (line.startsWith('diff --git') || line.startsWith('index ')) {
    color = 'cyan';
  }

  return (
    <Text key={`${index}-${line}`} color={color}>
      {line || ' '}
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
  const [data, setData] = useState<DataState>(INITIAL_STATE);
  const [focus, setFocus] = useState<PaneFocus>('commits');
  const [mode, setMode] = useState<ExplorerMode>('history');
  const [selectedCommitIndex, setSelectedCommitIndex] = useState(0);
  const [selectedFileIndex, setSelectedFileIndex] = useState(0);
  const [compareAnchorSha, setCompareAnchorSha] = useState<string | null>(null);
  const [diffScroll, setDiffScroll] = useState(0);

  useEffect(() => {
    let active = true;

    async function loadRepository(): Promise<void> {
      setMode('history');
      setFocus('commits');
      setSelectedCommitIndex(0);
      setSelectedFileIndex(0);
      setCompareAnchorSha(null);
      setDiffScroll(0);
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

  const selectedCommit = data.commits[selectedCommitIndex] ?? null;
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
        diffStyle: 'three-dot'
      };
    }

    if (mode === 'compare-commits' && compareAnchorCommit && selectedCommit) {
      return {
        mode,
        leftRef: compareAnchorCommit.sha,
        rightRef: selectedCommit.sha,
        leftLabel: compareAnchorCommit.shortSha,
        rightLabel: selectedCommit.shortSha,
        diffStyle: 'two-dot'
      };
    }

    return null;
  }, [compareAnchorCommit, data.repository, mode, selectedCommit]);

  const activeSelectionKey = activeComparison
    ? `${activeComparison.mode}:${activeComparison.leftRef}:${activeComparison.diffStyle}:${activeComparison.rightRef}`
    : `history:${selectedCommit?.sha ?? 'none'}`;

  useEffect(() => {
    setSelectedFileIndex(0);
    setDiffScroll(0);
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
        filesError: null
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
  }, [activeComparison, data.repository, selectedCommit]);

  const selectedFile = data.files[selectedFileIndex] ?? null;

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
          selectedFile?.path
        )
      : getCommitDiff(data.repository.rootPath, selectedCommit!.sha, selectedFile?.path);

    void loadDiff
      .then((diff) => {
        if (!active) {
          return;
        }

        setDiffScroll(0);
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
  }, [activeComparison, data.repository, selectedCommit, selectedFile]);

  const diffLines = useMemo(
    () => (data.diff ? data.diff.split('\n').map((line) => sanitizeText(line)) : []),
    [data.diff]
  );
  const diffHeight = Math.max(8, (stdout.rows ?? 24) - 10);
  const diffWindow = diffLines.slice(diffScroll, diffScroll + diffHeight);
  const listHeight = Math.max(8, (stdout.rows ?? 24) - 12);
  const commitWindow = createWindow(data.commits, selectedCommitIndex, listHeight);
  const fileWindow = createWindow(data.files, selectedFileIndex, listHeight);
  const commitWidth = Math.max(36, Math.floor((stdout.columns ?? 120) * 0.32));
  const fileWidth = Math.max(28, Math.floor((stdout.columns ?? 120) * 0.24));
  const activeModeLabel = activeComparison ? `Compare ${formatComparisonSpec(activeComparison)}` : 'History selected commit';
  const comparePresetLabel = data.repository?.defaultCompareBase
    ? `${data.repository.defaultCompareBase}...HEAD`
    : 'unavailable';

  useInput((input, key) => {
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

    if (input === 'g') {
      setMode('history');
      return;
    }

    if (input === 'c') {
      if (!data.repository?.defaultCompareBase) {
        return;
      }

      setMode((current) => (current === 'compare-preset' ? 'history' : 'compare-preset'));
      setFocus('commits');
      return;
    }

    if (input === 'v' && selectedCommit) {
      if (mode === 'compare-commits' && compareAnchorSha === selectedCommit.sha) {
        setCompareAnchorSha(null);
        setMode('history');
      } else {
        setCompareAnchorSha(selectedCommit.sha);
        setMode('compare-commits');
      }
      setFocus('commits');
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
        setSelectedCommitIndex((current) => Math.min(current + 1, Math.max(0, data.commits.length - 1)));
      } else if (focus === 'files') {
        setSelectedFileIndex((current) => Math.min(current + 1, Math.max(0, data.files.length - 1)));
      } else {
        setDiffScroll((current) => Math.min(current + 1, Math.max(0, diffLines.length - diffHeight)));
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
        <Text color="gray">{`Keys: j/k move, h/l focus, tab cycle, v anchor compare, c ${comparePresetLabel}, g history, q quit`}</Text>
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
          <PaneFrame title={`Commits (${data.commits.length})`} focused={focus === 'commits'} width={commitWidth}>
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

          <PaneFrame title={`Files (${data.files.length})`} focused={focus === 'files'} width={fileWidth}>
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
            title={selectedFile ? `Diff: ${selectedFile.path}` : activeComparison ? 'Diff Preview (comparison)' : 'Diff Preview'}
            focused={focus === 'diff'}
          >
            {data.loadingDiff ? <Text color="yellow">Loading diff...</Text> : null}
            {data.diffError && !data.loadingDiff ? <Text color="gray">{data.diffError}</Text> : null}
            {!data.loadingDiff && diffWindow.length === 0 ? <Text color="gray">No diff loaded.</Text> : null}
            {!data.loadingDiff && diffWindow.map((line, index) => renderDiffLine(line, diffScroll + index))}
          </PaneFrame>
        </Box>
      )}
    </Box>
  );
}
