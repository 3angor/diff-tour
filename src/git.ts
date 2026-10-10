import { execFileSync } from 'node:child_process';

// tour は読み取り専用。ここでは作業ツリーも .git も書き換えない git コマンドだけを使う

export type DiffLine = { kind: ' ' | '+' | '-'; oldNo?: number; newNo?: number; text: string };
export type Hunk = { header: string; lines: DiffLine[] };

export class Repo {
  #dir: string;
  #cache = new Map<string, string | null>();

  constructor(dir: string) {
    this.#dir = dir;
  }

  #git(args: string[]): string | null {
    const key = args.join('\0');
    if (this.#cache.has(key)) return this.#cache.get(key)!;
    let out: string | null;
    try {
      out = execFileSync('git', ['-C', this.#dir, '--no-pager', ...args], {
        encoding: 'utf8',
        maxBuffer: 64 * 1024 * 1024,
        stdio: ['ignore', 'pipe', 'ignore'],
        // ロックファイルなどの付随的な書き込みもさせない
        env: { ...process.env, GIT_OPTIONAL_LOCKS: '0' },
      });
    } catch {
      out = null;
    }
    this.#cache.set(key, out);
    return out;
  }

  /** ある時点のファイルの中身。存在しなければ null */
  show(ref: string, file: string): string[] | null {
    const out = this.#git(['show', `${ref}:${file}`]);
    return out === null ? null : out.replace(/\n$/, '').split('\n');
  }

  /** base から分岐して以降の head での変更（三点リーダー＝分岐点との比較） */
  diff(base: string, head: string, file: string): Hunk[] | null {
    const out = this.#git(['diff', '--no-color', '--no-ext-diff', '-U3', `${base}...${head}`, '--', file]);
    return out === null ? null : parseDiff(out);
  }

  /** ある時点で dir の下にあるファイルのパス */
  listFiles(ref: string, dir: string): string[] {
    return (this.#git(['ls-tree', '-r', '--name-only', ref, '--', dir]) ?? '').split('\n').filter(Boolean);
  }

  commits(base: string, head: string): Array<{ hash: string; subject: string }> {
    const out = this.#git(['log', '--format=%h%x09%s', `${base}..${head}`]) ?? '';
    return out
      .split('\n')
      .filter(Boolean)
      .map((l) => {
        const [hash, subject] = l.split('\t');
        return { hash, subject };
      });
  }

  shortstat(base: string, head: string): string {
    return (this.#git(['diff', '--shortstat', `${base}...${head}`]) ?? '').trim();
  }
}

export function parseDiff(text: string): Hunk[] {
  const hunks: Hunk[] = [];
  let hunk: Hunk | null = null;
  let oldNo = 0;
  let newNo = 0;
  for (const line of text.split('\n')) {
    const m = line.match(/^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@(.*)/);
    if (m) {
      hunk = { header: m[3].trim(), lines: [] };
      hunks.push(hunk);
      oldNo = Number(m[1]);
      newNo = Number(m[2]);
      continue;
    }
    if (!hunk) continue;
    if (line.startsWith('+')) hunk.lines.push({ kind: '+', newNo: newNo++, text: line.slice(1) });
    else if (line.startsWith('-')) hunk.lines.push({ kind: '-', oldNo: oldNo++, text: line.slice(1) });
    else if (line.startsWith(' ')) hunk.lines.push({ kind: ' ', oldNo: oldNo++, newNo: newNo++, text: line.slice(1) });
  }
  return hunks;
}
