import { spawnSync } from 'node:child_process';
import { chmodSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { homedir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { parseArgs } from 'node:util';
import { Repo } from './git.ts';
import { renderFrame, stepLocation } from './render.ts';
import { toAnsi } from './text.ts';
import { flatten, type Location, type Tour } from './tour.ts';

const { values, positionals } = parseArgs({
  allowPositionals: true,
  options: {
    repo: { type: 'string', default: process.cwd() },
    // 確認用: 全手順を指定サイズで標準出力に書き出す
    dump: { type: 'boolean', default: false },
    plain: { type: 'boolean', default: false },
    cols: { type: 'string' },
    rows: { type: 'string' },
  },
});

if (positionals.length !== 1) {
  console.error('usage: tour <tour.json> [--repo <path>]');
  process.exit(2);
}

const tour = JSON.parse(readFileSync(positionals[0], 'utf8')) as Tour;
const repoDir = resolve(values.repo!);
const ctx = { tour, repo: new Repo(repoDir) };
const steps = flatten(tour);

if (values.dump) {
  const cols = Number(values.cols ?? 100);
  const rows = Number(values.rows ?? 30);
  steps.forEach((_, i) => {
    const { lines, maxScroll } = renderFrame(ctx, steps, i, 0, cols, rows);
    console.log(`\n===== step ${i + 1}${maxScroll ? `  (overflow ${maxScroll} lines)` : ''} =====`);
    for (const l of lines) console.log(toAnsi(l, values.plain));
  });
  process.exit(0);
}

if (!process.stdin.isTTY || !process.stdout.isTTY) {
  console.error('tour は端末の中で実行してください');
  process.exit(2);
}

const out = process.stdout;
let index = 0;
let scroll = 0;
let maxScroll = 0;
let message: string | undefined;

function draw() {
  const frame = renderFrame(ctx, steps, index, scroll, out.columns, out.rows, message);
  message = undefined;
  maxScroll = frame.maxScroll;
  scroll = Math.min(scroll, maxScroll);
  out.write('\x1b[H' + frame.lines.map((l) => toAnsi(l)).join('\r\n'));
}

function go(next: number) {
  if (next < 0 || next >= steps.length || next === index) return;
  index = next;
  scroll = 0;
  draw();
}

/** 環境変数のエディタに「ファイルの何行目」を渡す形を、よく使われるエディタに合わせて作る */
function editorCommand(file: string, line: number): string[] {
  const template = process.env.TOUR_EDITOR;
  if (template) {
    return template.split(/\s+/).map((a) => a.replaceAll('{file}', file).replaceAll('{line}', String(line)));
  }
  const cmd = (process.env.VISUAL || process.env.EDITOR || 'vi').split(/\s+/).filter(Boolean);
  const name = basename(cmd[0]);
  if (['code', 'code-insiders', 'cursor', 'codium', 'windsurf'].includes(name)) return [...cmd, '-g', `${file}:${line}`];
  if (['hx', 'helix', 'micro', 'zed', 'subl'].includes(name)) return [...cmd, `${file}:${line}`];
  return [...cmd, `+${line}`, file];
}

/**
 * 作業ツリーのファイルがツアーの時点と同じ内容ならそれを開く。
 * 違う場合（別のブランチや PR を見ているとき）は、その時点の内容を読み取り専用のコピーにして開く。
 * どちらの場合も、リポジトリには何も書かない
 */
function openInEditor(loc: Location) {
  const ref = loc.at === 'base' ? tour.base : tour.head;
  const content = ctx.repo.show(ref, loc.file);
  if (!content) {
    message = `ファイルが見つかりません: ${loc.file}`;
    draw();
    return;
  }
  const real = join(repoDir, loc.file);
  let target = real;
  let same = false;
  try {
    same = readFileSync(real, 'utf8').replace(/\n$/, '') === content.join('\n');
  } catch {}
  if (!same) {
    const cacheDir = process.env.XDG_CACHE_HOME || join(homedir(), '.cache');
    target = join(cacheDir, 'tour', ref.replace(/[^\w.-]/g, '_'), loc.file);
    mkdirSync(dirname(target), { recursive: true });
    rmSync(target, { force: true });
    writeFileSync(target, content.join('\n') + '\n');
    chmodSync(target, 0o444);
  }

  const [cmd, ...args] = editorCommand(target, loc.line);
  out.write('\x1b[?25h\x1b[?1049l');
  process.stdin.setRawMode(false);
  process.stdin.pause();
  const result = spawnSync(cmd, args, { stdio: 'inherit' });
  process.stdin.setRawMode(true);
  process.stdin.resume();
  out.write('\x1b[?1049h\x1b[?25l\x1b[2J');
  if (result.error) message = `エディタを起動できません: ${cmd}`;
  else if (!same) message = `作業ツリーが ${ref} と違うため、読み取り専用のコピーを開きました`;
  draw();
}

function quit() {
  out.write('\x1b[0m\x1b[?25h\x1b[?1049l');
  process.stdin.setRawMode(false);
  process.exit(0);
}

out.write('\x1b[?1049h\x1b[?25l\x1b[2J');
process.stdin.setRawMode(true);
process.stdin.setEncoding('utf8');
process.stdin.on('data', (key: string) => {
  switch (key) {
    case '\x1b[C': case 'l': case ' ': case '\r': case 'n':
      go(index + 1);
      break;
    case '\x1b[D': case 'h': case '\x7f': case 'p':
      go(index - 1);
      break;
    case '\x1b[B': case 'j':
      if (scroll < maxScroll) { scroll++; draw(); }
      break;
    case '\x1b[A': case 'k':
      if (scroll > 0) { scroll--; draw(); }
      break;
    case 'o': {
      const loc = stepLocation(steps[index], ctx);
      if (loc) openInEditor(loc);
      break;
    }
    case 'q': case '\x03':
      quit();
  }
});
out.on('resize', () => {
  out.write('\x1b[2J');
  draw();
});
process.on('SIGTERM', quit);
draw();
