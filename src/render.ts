import type { Repo } from './git.ts';
import { highlight } from './highlight.ts';
import { concat, fit, join, lineWidth, parseInline, strWidth, wrap, type Line } from './text.ts';
import type { Basis, Block, CodeRef, DiffRef, FlatStep, Snippet, Tour } from './tour.ts';

type Ctx = { tour: Tour; repo: Repo };

const BASIS: Record<Basis, { label: string; style: string }> = {
  fact: { label: '確かめた', style: '30;42' },
  inferred: { label: '読み取れる', style: '97;44' },
  opinion: { label: 'シニアの見方', style: '30;43' },
};

const PAD = 2;
const SIDE_BY_SIDE_MIN_COLS = 110;

function error(message: string, width: number): Line[] {
  return wrap([{ text: `⚠ ${message}`, style: '91' }], width);
}

function fileHeader(file: string, detail: string, note: string | undefined, width: number): Line {
  const line: Line = [
    { text: ` ${file} `, style: '1;97;100' },
    { text: ` ${detail}`, style: '90' },
  ];
  if (note) line.push({ text: `  ${note}`, style: '33' });
  return fit(line, width);
}

function untab(s: string): string {
  return s.replace(/\t/g, '  ');
}

function renderCode(ref: CodeRef, width: number, { tour, repo }: Ctx): Line[] {
  const at = ref.at === 'base' ? tour.base : tour.head;
  const src = repo.show(at, ref.file);
  if (!src) return error(`ファイルが見つかりません: ${ref.file}（${at}）`, width);
  const [hl0, hl1] = ref.lines;
  if (hl0 < 1 || hl1 > src.length || hl0 > hl1) {
    return error(`行の指定が範囲外です: ${ref.file} L${hl0}–${hl1}（全 ${src.length} 行）`, width);
  }
  const [s0, s1] = ref.show ?? [Math.max(1, hl0 - 2), Math.min(src.length, hl1 + 2)];
  // 複数行コメントの途中から表示しても色が狂わないよう、ファイル全体を色分けしてから切り出す
  const colored = highlight(src.map(untab));
  const numW = String(s1).length;
  const out: Line[] = [fileHeader(ref.file, hl0 === hl1 ? `L${hl0}` : `L${hl0}–${hl1}`, ref.note, width)];
  for (let n = s0; n <= s1; n++) {
    const on = n >= hl0 && n <= hl1;
    const gutter: Line = [
      { text: `${String(n).padStart(numW)} `, style: on ? '33' : '90' },
      { text: on ? '▌ ' : '  ', style: '33' },
    ];
    const code = on ? colored[n - 1] : colored[n - 1].map((s) => ({ text: s.text, style: join('2', s.style) }));
    out.push(fit(concat(gutter, code), width));
  }
  return out;
}

function renderDiff(ref: DiffRef, width: number, { tour, repo }: Ctx): Line[] {
  const hunks = repo.diff(tour.base, tour.head, ref.file);
  if (!hunks || hunks.length === 0) return error(`差分が見つかりません: ${ref.file}`, width);
  const picked = ref.hunks ? ref.hunks.map((i) => hunks[i]).filter(Boolean) : hunks;
  if (picked.length === 0) return error(`指定したハンクがありません: ${ref.file} ${ref.hunks}`, width);

  const added = picked.flatMap((h) => h.lines).filter((l) => l.kind === '+').length;
  const removed = picked.flatMap((h) => h.lines).filter((l) => l.kind === '-').length;
  const out: Line[] = [fileHeader(ref.file, `差分  +${added} −${removed}`, undefined, width)];
  const numW = Math.max(...picked.flatMap((h) => h.lines.map((l) => String(l.newNo ?? l.oldNo ?? 0).length)));

  picked.forEach((hunk, hi) => {
    if (hi > 0) out.push([{ text: '⋯', style: '90' }]);
    const colored = highlight(hunk.lines.map((l) => untab(l.text)));
    hunk.lines.forEach((l, i) => {
      const bg = l.kind === '+' ? '48;5;22' : l.kind === '-' ? '48;5;52' : undefined;
      const sign = l.kind === '+' ? { text: '+ ', style: join(bg, '92') } : l.kind === '-' ? { text: '− ', style: join(bg, '91') } : { text: '  ' };
      const num = l.kind === '-' ? l.oldNo : l.newNo;
      const gutter: Line = [{ text: `${String(num).padStart(numW)} `, style: join(bg, '90') }, sign];
      const code = colored[i].map((s) => ({ text: s.text, style: join(bg, l.kind === ' ' ? '2' : undefined, s.style) }));
      out.push(fit(concat(gutter, code), width, bg));
    });
  });
  return out;
}

function renderSnippet(snippet: Snippet, width: number): Line[] {
  const out: Line[] = [fit([{ text: ' 例 ', style: '30;43' }, { text: ` ${snippet.label}`, style: '33' }], width)];
  for (const code of highlight(snippet.text.split('\n').map(untab))) {
    out.push(fit(concat([{ text: '┆ ', style: '33' }], code), width));
  }
  return out;
}

const PRE_MARKS: Record<string, string> = { '✔': '92', '✘': '91', '★': '93', '✎': '33' };

function renderPre(text: string, width: number): Line[] {
  return text.split('\n').map((src) => {
    const line: Line = [];
    for (const ch of src) {
      const style = PRE_MARKS[ch];
      const last = line.at(-1);
      if (last && last.style === style) last.text += ch;
      else line.push({ text: ch, style });
    }
    return fit(line, width);
  });
}

function renderTable(table: { head: string[]; rows: string[][] }, width: number): Line[] {
  const gap = 2;
  const n = table.head.length;
  const plainWidth = (s: string) => strWidth(s.replace(/\*\*|`/g, ''));
  const widths = table.head.map((h, i) => Math.max(plainWidth(h), ...table.rows.map((r) => plainWidth(r[i] ?? ''))));
  const avail = width - gap * (n - 1);
  while (widths.reduce((a, b) => a + b, 0) > avail) {
    const i = widths.indexOf(Math.max(...widths));
    if (widths[i] <= 6) break;
    widths[i]--;
  }
  const row = (cells: string[], style?: string): Line[] => {
    const wrapped = cells.map((c, i) => wrap(parseInline(c, style), widths[i]));
    const h = Math.max(...wrapped.map((w) => w.length));
    const lines: Line[] = [];
    for (let k = 0; k < h; k++) {
      lines.push(
        concat(...wrapped.map((w, i) => concat(fit(w[k] ?? [], widths[i]), i < n - 1 ? [{ text: ' '.repeat(gap) }] : []))),
      );
    }
    return lines;
  };
  const rule: Line = [{ text: '─'.repeat(Math.min(width, widths.reduce((a, b) => a + b, 0) + gap * (n - 1))), style: '90' }];
  const out = [...row(table.head, '1'), rule];
  table.rows.forEach((r, i) => {
    if (i > 0) out.push([{ text: rule[0].text.replace(/─/g, '┄'), style: '90' }]);
    out.push(...row(r));
  });
  return out;
}

function renderTarget(width: number, { tour, repo }: Ctx): Line[] {
  const stat = repo.shortstat(tour.base, tour.head);
  const files = stat.match(/(\d+) files? changed/)?.[1] ?? '0';
  const ins = stat.match(/(\d+) insertions?/)?.[1] ?? '0';
  const del = stat.match(/(\d+) deletions?/)?.[1] ?? '0';
  const commits = repo.commits(tour.base, tour.head);
  const out: Line[] = [
    [
      { text: tour.head, style: '1;36' },
      { text: `（${tour.base} から分岐）`, style: '90' },
    ],
    [{ text: `${commits.length} コミット · ${files} ファイル変更（+${ins} −${del}）` }],
    [],
  ];
  for (const c of commits.reverse()) {
    out.push(...wrap([{ text: c.hash, style: '33' }, { text: ` ${c.subject}` }], width));
  }
  return out;
}

export function renderBlock(block: Block, width: number, ctx: Ctx): Line[] {
  if ('p' in block) {
    const spans = parseInline(block.p);
    if (block.basis) {
      const b = BASIS[block.basis];
      spans.unshift({ text: ` ${b.label} `, style: b.style }, { text: ' ' });
    }
    return wrap(spans, width);
  }
  if ('quote' in block) {
    return block.quote
      .split('\n')
      .flatMap((para) => wrap(parseInline(para), width - 2))
      .map((l) => concat([{ text: '┃ ', style: '36' }], l));
  }
  if ('list' in block) {
    return block.list.flatMap((item) =>
      wrap(parseInline(item), width - 2).map((l, i) => concat([{ text: i === 0 ? '・' : '  ', style: '36' }], l)),
    );
  }
  if ('pre' in block) return renderPre(block.pre, width);
  if ('table' in block) return renderTable(block.table, width);
  if ('code' in block) return renderCode(block.code, width, ctx);
  if ('diff' in block) return renderDiff(block.diff, width, ctx);
  if ('snippet' in block) return renderSnippet(block.snippet, width);
  if ('target' in block) return renderTarget(width, ctx);
  return error('不明なブロック', width);
}

function stack(blocks: Block[], width: number, ctx: Ctx): Line[] {
  return blocks.flatMap((b, i) => [...(i > 0 ? [[]] : []), ...renderBlock(b, width, ctx)]);
}

const isVisual = (b: Block) => 'code' in b || 'diff' in b || 'snippet' in b;

function trimEnd(line: Line): Line {
  const out = line.map((s) => ({ ...s }));
  while (out.length && /^\s*$/.test(out.at(-1)!.text)) out.pop();
  if (out.length) out[out.length - 1].text = out[out.length - 1].text.trimEnd();
  return out;
}

function spread(left: Line, right: Line, width: number): Line {
  const room = width - lineWidth(right);
  if (room < 8) return fit(left, width);
  return concat(fit(left, room), right);
}

export type Frame = { lines: Line[]; maxScroll: number };

export function renderFrame(
  ctx: Ctx,
  steps: FlatStep[],
  index: number,
  scroll: number,
  cols: number,
  rows: number,
): Frame {
  const step = steps[index];
  const inner = Math.max(20, cols - PAD * 2);

  const header: Line[] = [
    spread(
      [
        { text: ' tour ', style: '30;46' },
        { text: ` ${ctx.tour.title}`, style: '1' },
      ],
      [{ text: `第${step.chapter + 1}章 ${step.chapterTitle} `, style: '36' }],
      cols,
    ),
    [{ text: '─'.repeat(cols), style: '90' }],
  ];

  const titleSpans = step.question
    ? [{ text: ' 問い ', style: '30;43' }, { text: ' ' }, { text: step.title, style: '1' }]
    : [{ text: step.title, style: '1' }];
  const title = wrap(titleSpans, inner);
  const hint: Line[] = step.question ? [[], [{ text: '考えてから → で次へ', style: '2;33' }]] : [];

  // 左に置くのは、先頭から続くコードのブロック（なければ最初のコードのブロック）だけ。
  // それ以外は説明の流れを崩さないよう、元の順番のまま右に置く
  const leading = step.blocks.findIndex((b) => !isVisual(b));
  const visual =
    leading > 0 ? step.blocks.slice(0, leading) : step.blocks.filter(isVisual).slice(0, 1);
  const other = step.blocks.filter((b) => !visual.includes(b));
  let body: Line[];
  if (cols >= SIDE_BY_SIDE_MIN_COLS && visual.length > 0 && other.length > 0) {
    // コードが切れないよう、左はコードの実際の幅に合わせる（全体の 45〜65% の範囲）
    const natural = Math.max(...stack(visual, 400, ctx).map((l) => lineWidth(trimEnd(l))));
    const leftW = Math.max(Math.floor(inner * 0.45), Math.min(natural, Math.floor(inner * 0.65)));
    const rightW = inner - leftW - 3;
    const left = stack(visual, leftW, ctx);
    const right = [...stack(other, rightW, ctx), ...hint];
    const h = Math.max(left.length, right.length);
    body = [...title, []];
    for (let i = 0; i < h; i++) {
      body.push(concat(fit(left[i] ?? [], leftW), [{ text: ' │ ', style: '90' }], fit(right[i] ?? [], rightW)));
    }
  } else {
    body = [...title, [], ...stack(step.blocks, inner, ctx), ...hint];
  }

  const bodyH = Math.max(1, rows - header.length - 2);
  const maxScroll = Math.max(0, body.length - bodyH);
  const top = Math.min(scroll, maxScroll);
  const visible = body.slice(top, top + bodyH);
  while (visible.length < bodyH) visible.push([]);

  const barW = Math.max(10, cols - 10);
  const filled = Math.round(((index + 1) / steps.length) * barW);
  const progress: Line = fit(
    [
      { text: ' ' },
      { text: '━'.repeat(filled), style: '36' },
      { text: '─'.repeat(barW - filled), style: '90' },
      { text: ` ${index + 1}/${steps.length}`, style: '90' },
    ],
    cols,
  );

  const first = index === 0;
  const last = index === steps.length - 1;
  const keys: Line = [
    { text: ' ← 戻る', style: first ? '90' : undefined },
    { text: '   ' },
    last ? { text: '最後の手順です', style: '90' } : { text: '→ 次へ', style: '1' },
    { text: '   q 終了', style: '90' },
  ];
  const more: Line =
    maxScroll === 0 ? [] : [{ text: top < maxScroll ? '▼ 続きあり  ↑↓ ' : '▲ ↑↓ ', style: '33' }];

  return {
    lines: [
      ...header,
      ...visible.map((l) => concat([{ text: ' '.repeat(PAD) }], fit(l, inner), [{ text: ' '.repeat(cols - inner - PAD) }])),
      progress,
      spread(keys, more, cols),
    ],
    maxScroll,
  };
}
