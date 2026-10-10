import { readFileSync } from 'node:fs';
import { parseArgs } from 'node:util';
import { Repo } from './git.ts';
import { renderFrame } from './render.ts';
import { toAnsi } from './text.ts';
import { flatten, type Tour } from './tour.ts';

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
const ctx = { tour, repo: new Repo(values.repo!) };
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

function draw() {
  const frame = renderFrame(ctx, steps, index, scroll, out.columns, out.rows);
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
