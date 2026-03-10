export type PaneFocus = 'commits' | 'files' | 'diff';
export type DiffStyle = 'two-dot' | 'three-dot';
export type ExplorerMode = 'history' | 'compare-preset' | 'compare-commits';

export interface RepositoryInfo {
  rootPath: string;
  gitDir: string;
  headRef: string;
  defaultCompareBase: string | null;
}

export interface CommitSummary {
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
