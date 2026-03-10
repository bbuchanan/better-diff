import {execFile} from 'node:child_process';
import {promisify} from 'node:util';

import type {CommitSummary, DiffStyle, FileChange, RepositoryInfo} from '../domain/models.js';

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

export async function discoverRepository(cwd: string): Promise<RepositoryInfo> {
  const [rootPath, gitDir, headRef] = await Promise.all([
    runGit(['rev-parse', '--show-toplevel'], cwd),
    runGit(['rev-parse', '--git-dir'], cwd),
    runGit(['branch', '--show-current'], cwd)
  ]);
  const normalizedRootPath = sanitizeOutput(rootPath).trim();
  const normalizedHeadRef = sanitizeOutput(headRef).trim() || 'HEAD';
  const defaultCompareBase = await discoverDefaultCompareBase(normalizedRootPath, normalizedHeadRef);

  return {
    rootPath: normalizedRootPath,
    gitDir: sanitizeOutput(gitDir).trim(),
    headRef: normalizedHeadRef,
    defaultCompareBase
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
      `--max-count=${limit}`,
      '--date=short',
      `--format=${format}${RECORD_SEPARATOR}`
    ],
    cwd
  );

  return raw
    .split(RECORD_SEPARATOR)
    .map((record) => record.trim())
    .filter(Boolean)
    .map((record) => {
      const [sha, shortSha, authoredAt, authorName, refsRaw, subject] = record.split(FIELD_SEPARATOR);
      return {
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
