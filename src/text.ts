// 端末に描く 1 行を「スタイル付きの文字列の並び」として扱う。
// 日本語は 1 文字で 2 マス使うので、幅はすべて表示幅で数える。

export type Span = { text: string; style?: string };
export type Line = Span[];

const WIDE: Array<[number, number]> = [
  [0x1100, 0x115f],
  [0x2e80, 0x303e],
  [0x3041, 0x33ff],
  [0x3400, 0x4dbf],
  [0x4e00, 0x9fff],
  [0xa000, 0xa4cf],
  [0xac00, 0xd7a3],
  [0xf900, 0xfaff],
  [0xfe30, 0xfe4f],
  [0xff00, 0xff60],
  [0xffe0, 0xffe6],
  [0x1f300, 0x1f64f],
  [0x1f900, 0x1f9ff],
  [0x20000, 0x3fffd],
];

export function charWidth(cp: number): number {
  if (cp < 0x20 || (cp >= 0x7f && cp < 0xa0)) return 0;
  if ((cp >= 0x300 && cp <= 0x36f) || (cp >= 0x200b && cp <= 0x200f) || cp === 0xfe0f) return 0;
  for (const [lo, hi] of WIDE) {
    if (cp < lo) break;
    if (cp <= hi) return 2;
  }
  return 1;
}

export function strWidth(s: string): number {
  let w = 0;
  for (const ch of s) w += charWidth(ch.codePointAt(0)!);
  return w;
}

export function lineWidth(line: Line): number {
  return line.reduce((w, s) => w + strWidth(s.text), 0);
}

export function join(...styles: Array<string | undefined>): string | undefined {
  const s = styles.filter(Boolean).join(';');
  return s || undefined;
}

function append(line: Line, text: string, style?: string) {
  const last = line.at(-1);
  if (last && last.style === style) last.text += text;
  else line.push({ text, style });
}

/** 幅ちょうどに切り詰める（はみ出すなら末尾を … にする）か、空白で埋める */
export function fit(line: Line, width: number, padStyle?: string): Line {
  const out: Line = [];
  const overflow = lineWidth(line) > width;
  const limit = overflow ? width - 1 : width;
  let w = 0;
  outer: for (const span of line) {
    for (const ch of span.text) {
      const cw = charWidth(ch.codePointAt(0)!);
      if (w + cw > limit) break outer;
      append(out, ch, span.style);
      w += cw;
    }
  }
  if (overflow) {
    append(out, '…', join(padStyle, '2'));
    w += 1;
  }
  if (w < width) append(out, ' '.repeat(width - w), padStyle);
  return out;
}

export function concat(...lines: Line[]): Line {
  const out: Line = [];
  for (const line of lines) for (const s of line) append(out, s.text, s.style);
  return out;
}

export function toAnsi(line: Line, plain = false): string {
  return line.map((s) => (s.style && !plain ? `\x1b[${s.style}m${s.text}\x1b[0m` : s.text)).join('');
}

/** `code` と **強調** だけを解釈する */
export function parseInline(text: string, base?: string): Span[] {
  const spans: Span[] = [];
  // **強調** の中に `code` が入ることがあるので、強調を先に分ける
  for (const part of text.split(/(\*\*.+?\*\*)/)) {
    if (!part) continue;
    const bold = part.startsWith('**') && part.endsWith('**') && part.length > 4;
    const style = bold ? join(base, '1') : base;
    for (const piece of (bold ? part.slice(2, -2) : part).split(/(`[^`]+`)/)) {
      if (!piece) continue;
      if (piece.startsWith('`') && piece.endsWith('`')) spans.push({ text: piece.slice(1, -1), style: join(style, '36') });
      else spans.push({ text: piece, style });
    }
  }
  return spans;
}

// 行頭に来てはいけない文字。直前の単位にくっつけて、一緒に折り返す
const NO_LINE_START = new Set([...'、。，．）」』】〕！？：；・ー…’”)]}.,!?:;']);

type Unit = { text: string; style?: string };

function units(spans: Span[]): Unit[] {
  const out: Unit[] = [];
  for (const span of spans) {
    let word = '';
    const flush = () => {
      if (word) out.push({ text: word, style: span.style });
      word = '';
    };
    for (const ch of span.text) {
      const cp = ch.codePointAt(0)!;
      if (ch === '\n' || ch === ' ') {
        flush();
        out.push({ text: ch, style: span.style });
      } else if (charWidth(cp) === 2) {
        flush();
        out.push({ text: ch, style: span.style });
      } else {
        word += ch;
      }
    }
    flush();
  }
  // 行頭禁止の文字を前の単位に寄せる
  const merged: Unit[] = [];
  for (const u of out) {
    const prev = merged.at(-1);
    if (prev && prev.text !== '\n' && prev.text !== ' ' && NO_LINE_START.has([...u.text][0]) && prev.style === u.style) {
      prev.text += u.text;
    } else {
      merged.push({ ...u });
    }
  }
  return merged;
}

/** 幅に収まるように折り返す。英単語は途中で切らず、日本語はどこでも折り返せる */
export function wrap(spans: Span[], width: number): Line[] {
  const lines: Line[] = [];
  let cur: Line = [];
  let w = 0;
  const newline = () => {
    lines.push(cur);
    cur = [];
    w = 0;
  };
  for (const u of units(spans)) {
    if (u.text === '\n') {
      newline();
      continue;
    }
    const uw = strWidth(u.text);
    if (u.text === ' ' && (w === 0 || w + 1 > width)) continue;
    if (w + uw > width && w > 0) newline();
    if (uw > width) {
      for (const ch of u.text) {
        const cw = charWidth(ch.codePointAt(0)!);
        if (w + cw > width) newline();
        append(cur, ch, u.style);
        w += cw;
      }
      continue;
    }
    append(cur, u.text, u.style);
    w += uw;
  }
  lines.push(cur);
  return lines;
}
