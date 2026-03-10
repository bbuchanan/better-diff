import {readFile} from 'node:fs/promises';
import {join, normalize, resolve} from 'node:path';
import {execFile, spawn} from 'node:child_process';
import {promisify} from 'node:util';

import type {
  CommitSummary,
  ConflictFile,
  ConflictFileContents,
  DiffStyle,
  FileChange,
  RepositoryInfo
} from '../domain/models.js';

const execFileAsync = promisify(execFile);
const RECORD_SEPARATOR = '\u001e';
const FIELD_SEPARATOR = '\u001f';

export class GitCommandError extends Error {
  readonly code: number | null;
  readonly stderr: string;

  constructor(message: string, code: number | null, stderr: string) {
    super(message);
    this.name = 'GitCommandError';
    this.code = code;
    this.stderr = stderr;
  }
}

function sanitizeOutput(value: string): string {
  return value.replace(/[\u0000-\u0008\u000b-\u001f\u007f-\u009f]/g, '');
}

async function runGit(args: string[], cwd: string): Promise<string> {
  try {
    const {stdout} = await execFileAsync('git', args, {
      cwd,
      encoding: 'utf8',
      maxBuffer: 1024 * 1024 * 16
    });

    return stdout;
  } catch (error) {
    const execError = error as NodeJS.ErrnoException & {
      code?: number | string;
      stderr?: string;
      message: string;
    };

    const numericCode = typeof execError.code === 'number' ? execError.code : null;
    throw new GitCommandError(
      execError.message,
      numericCode,
      sanitizeOutput(execError.stderr ?? '')
    );
  }
}

async function revisionExists(cwd: string, revision: string): Promise<boolean> {
  try {
    await runGit(['rev-parse', '--verify', `${revision}^{commit}`], cwd);
    return true;
  } catch {
    return false;
  }
}

async function refExists(cwd: string, revision: string): Promise<boolean> {
  try {
    await runGit(['rev-parse', '--verify', revision], cwd);
    return true;
  } catch {
    return false;
  }
}

async function discoverDefaultCompareBase(cwd: string, headRef: string): Promise<string | null> {
  const candidates = ['main', 'origin/main', 'master', 'origin/master'];

  if (headRef && headRef !== 'HEAD') {
    candidates.push(headRef);
  }

  for (const candidate of candidates) {
    if (await revisionExists(cwd, candidate)) {
      return candidate;
    }
  }

  return null;
}

function parseNameStatusOutput(raw: string): FileChange[] {
  return raw
    .split('\n')
    .map((line) => sanitizeOutput(line).trim())
    .filter(Boolean)
    .map((line) => {
      const parts = line.split('\t');
      const statusToken = parts[0] ?? '?';
      const status = statusToken[0] as FileChange['status'];

      if (status === 'R' || status === 'C') {
        return {
          status,
          oldPath: parts[1],
          path: parts[2] ?? parts[1] ?? ''
        };
      }

      return {
        status,
        path: parts[1] ?? ''
      };
    })
    .filter((change) => Boolean(change.path));
}

function toDiffSpecifier(left: string, right: string, diffStyle: DiffStyle): string {
  return diffStyle === 'three-dot' ? `${left}...${right}` : `${left}..${right}`;
}

function parseCommand(command: string): string[] {
  const parts: string[] = [];
  let current = '';
  let quote: '"' | "'" | null = null;

  for (const char of command) {
    if (quote) {
      if (char === quote) {
        quote = null;
      } else {
        current += char;
      }
      continue;
    }

    if (char === '"' || char === "'") {
      quote = char;
      continue;
    }

    if (/\s/.test(char)) {
      if (current) {
        parts.push(current);
        current = '';
      }
      continue;
    }

    current += char;
  }

  if (current) {
    parts.push(current);
  }

  return parts;
}

function isTerminalEditor(command: string): boolean {
  const terminalEditors = new Set(['vim', 'nvim', 'vi', 'nano', 'hx', 'helix', 'emacs', 'kak']);
  return terminalEditors.has(command);
}

function resolveWorktreePath(rootPath: string, relativePath: string): string {
  const normalizedRelativePath = normalize(relativePath);
  const resolvedPath = resolve(join(rootPath, normalizedRelativePath));
  const resolvedRoot = resolve(rootPath);

  if (!resolvedPath.startsWith(resolvedRoot)) {
    throw new Error(`Refusing to read path outside repository root: ${relativePath}`);
  }

  return resolvedPath;
}

async function readStageBlob(cwd: string, stage: 1 | 2 | 3, path: string): Promise<string | undefined> {
  try {
    const raw = await runGit(['show', `:${stage}:${path}`], cwd);
    return sanitizeOutput(raw).trimEnd();
  } catch {
    return undefined;
  }
}

export async function discoverRepository(cwd: string): Promise<RepositoryInfo> {
  const [rootPath, gitDir, headRef] = await Promise.all([
    runGit(['rev-parse', '--show-toplevel'], cwd),
    runGit(['rev-parse', '--git-dir'], cwd),
    runGit(['branch', '--show-current'], cwd)
  ]);
  const normalizedRootPath = sanitizeOutput(rootPath).trim();
  const normalizedHeadRef = sanitizeOutput(headRef).trim() || 'HEAD';
  const [defaultCompareBase, isMergeInProgress, isRebaseInProgress, isCherryPickInProgress] = await Promise.all([
    discoverDefaultCompareBase(normalizedRootPath, normalizedHeadRef),
    refExists(normalizedRootPath, 'MERGE_HEAD'),
    refExists(normalizedRootPath, 'REBASE_HEAD'),
    refExists(normalizedRootPath, 'CHERRY_PICK_HEAD')
  ]);

  return {
    rootPath: normalizedRootPath,
    gitDir: sanitizeOutput(gitDir).trim(),
    headRef: normalizedHeadRef,
    defaultCompareBase,
    isMergeInProgress,
    isRebaseInProgress,
    isCherryPickInProgress
  };
}

export async function listCommits(cwd: string, limit = 60): Promise<CommitSummary[]> {
  const format = [
    '%H',
    '%h',
    '%ad',
    '%an',
    '%D',
    '%s'
  ].join(FIELD_SEPARATOR);
  const raw = await runGit(
    [
      'log',
      '--graph',
      `--max-count=${limit}`,
      '--date=short',
      `--format=${FIELD_SEPARATOR}${format}${RECORD_SEPARATOR}`
    ],
    cwd
  );

  return raw
    .split(RECORD_SEPARATOR)
    .map((record) => record.replace(/^\n+/, '').replace(/\n$/, ''))
    .filter(Boolean)
    .map((record) => {
      const firstSeparatorIndex = record.indexOf(FIELD_SEPARATOR);
      const graph = firstSeparatorIndex >= 0 ? record.slice(0, firstSeparatorIndex) : '';
      const fields = firstSeparatorIndex >= 0 ? record.slice(firstSeparatorIndex + 1).split(FIELD_SEPARATOR) : record.split(FIELD_SEPARATOR);
      const [sha, shortSha, authoredAt, authorName, refsRaw, subject] = fields;
      return {
        graph: sanitizeOutput(graph).replace(/\n/g, ''),
        sha: sanitizeOutput(sha),
        shortSha: sanitizeOutput(shortSha),
        authoredAt: sanitizeOutput(authoredAt),
        authorName: sanitizeOutput(authorName),
        refs: refsRaw
          .split(',')
          .map((value) => sanitizeOutput(value).trim())
          .filter(Boolean),
        subject: sanitizeOutput(subject)
      };
    });
}

export async function listCommitFiles(cwd: string, sha: string): Promise<FileChange[]> {
  const raw = await runGit(
    ['show', '--no-ext-diff', '--format=', '--name-status', '--find-renames', sha],
    cwd
  );

  return parseNameStatusOutput(raw);
}

export async function getCommitDiff(
  cwd: string,
  sha: string,
  path?: string,
  contextLines = 3
): Promise<string> {
  const args = ['show', '--no-ext-diff', `--unified=${contextLines}`, '--format=', sha];

  if (path) {
    args.push('--', path);
  }

  const raw = await runGit(args, cwd);
  return sanitizeOutput(raw).trimEnd();
}

export async function listRangeFiles(
  cwd: string,
  left: string,
  right: string,
  diffStyle: DiffStyle
): Promise<FileChange[]> {
  const raw = await runGit(
    ['diff', '--no-ext-diff', '--name-status', '--find-renames', toDiffSpecifier(left, right, diffStyle)],
    cwd
  );

  return parseNameStatusOutput(raw);
}

export async function getRangeDiff(
  cwd: string,
  left: string,
  right: string,
  diffStyle: DiffStyle,
  path?: string,
  contextLines = 3
): Promise<string> {
  const args = ['diff', '--no-ext-diff', `--unified=${contextLines}`, toDiffSpecifier(left, right, diffStyle)];

  if (path) {
    args.push('--', path);
  }

  const raw = await runGit(args, cwd);
  return sanitizeOutput(raw).trimEnd();
}

export async function listConflictFiles(cwd: string): Promise<ConflictFile[]> {
  const raw = await runGit(['ls-files', '-u', '-z'], cwd);

  if (!raw) {
    return [];
  }

  const conflictsByPath = new Map<string, ConflictFile>();

  for (const entry of raw.split('\0').filter(Boolean)) {
    const match = /^(\d+)\s+([0-9a-f]+)\s+(\d)\t(.+)$/.exec(entry);

    if (!match) {
      continue;
    }

    const stage = Number(match[3]);
    const path = sanitizeOutput(match[4]);
    const existing = conflictsByPath.get(path) ?? {
      path,
      status: 'U' as const,
      hasBase: false,
      hasOurs: false,
      hasTheirs: false
    };

    if (stage === 1) {
      existing.hasBase = true;
    } else if (stage === 2) {
      existing.hasOurs = true;
    } else if (stage === 3) {
      existing.hasTheirs = true;
    }

    conflictsByPath.set(path, existing);
  }

  return [...conflictsByPath.values()].sort((left, right) => left.path.localeCompare(right.path));
}

export async function getConflictFileContents(cwd: string, path: string): Promise<ConflictFileContents> {
  const [base, ours, theirs, merged] = await Promise.all([
    readStageBlob(cwd, 1, path),
    readStageBlob(cwd, 2, path),
    readStageBlob(cwd, 3, path),
    readFile(resolveWorktreePath(cwd, path), 'utf8')
      .then((contents) => sanitizeOutput(contents).trimEnd())
      .catch(() => undefined)
  ]);

  return {
    path,
    base,
    ours,
    theirs,
    merged
  };
}

export async function acceptConflictSide(cwd: string, path: string, side: 'ours' | 'theirs'): Promise<void> {
  await runGit(['checkout', `--${side}`, '--', path], cwd);
  await runGit(['add', '--', path], cwd);
}

export function openFileInEditor(
  cwd: string,
  path: string
): {mode: 'handoff' | 'background'; command: string} {
  const editorCommand = process.env.VISUAL || process.env.EDITOR || 'code';
  const parts = parseCommand(editorCommand);
  const command = parts[0] ?? 'code';
  const extraArgs = parts.slice(1);
  const targetPath = resolveWorktreePath(cwd, path);
  const isCodeFamily = ['code', 'cursor', 'codium', 'code-insiders'].includes(command);
  const args = isCodeFamily ? [...extraArgs, '-g', targetPath] : [...extraArgs, targetPath];

  if (isTerminalEditor(command)) {
    spawn(command, args, {
      cwd,
      stdio: 'inherit'
    });
    return {
      mode: 'handoff',
      command
    };
  }

  const child = spawn(command, args, {
    cwd,
    detached: true,
    stdio: 'ignore'
  });
  child.unref();

  return {
    mode: 'background',
    command
  };
}
