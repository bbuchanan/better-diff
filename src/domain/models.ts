export type PaneFocus = 'commits' | 'files' | 'diff';
export type DiffStyle = 'two-dot' | 'three-dot';
export type ExplorerMode = 'history' | 'compare-preset' | 'compare-commits' | 'conflict';

export interface RepositoryInfo {
  rootPath: string;
  gitDir: string;
  headRef: string;
  defaultCompareBase: string | null;
  isMergeInProgress: boolean;
  isRebaseInProgress: boolean;
  isCherryPickInProgress: boolean;
}

export interface CommitSummary {
  graph: string;
  sha: string;
  shortSha: string;
  authoredAt: string;
  authorName: string;
  refs: string[];
  subject: string;
}

export interface FileChange {
  path: string;
  oldPath?: string;
  status: 'A' | 'M' | 'D' | 'R' | 'C' | 'U' | '?';
}

export interface CompareSelection {
  mode: 'compare-preset' | 'compare-commits';
  leftRef: string;
  rightRef: string;
  leftLabel: string;
  rightLabel: string;
  diffStyle: DiffStyle;
}

export interface ConflictFile {
  path: string;
  status: 'U';
  hasBase: boolean;
  hasOurs: boolean;
  hasTheirs: boolean;
}

export interface ConflictFileContents {
  path: string;
  base?: string;
  ours?: string;
  theirs?: string;
  merged?: string;
}
