// ツアーのデータ形式（試作版）

/** fact: 確かめた / inferred: コードから読み取れる / opinion: シニアの見方（一般論。このリポジトリでは確かめていない） */
export type Basis = 'fact' | 'inferred' | 'opinion';

export type CodeRef = {
  file: string;
  /** どの時点のコードを見せるか。既定は head */
  at?: 'base' | 'head';
  /** 強調する行 [開始, 終了] */
  lines: [number, number];
  /** 表示する範囲。省略時は強調する行の前後 2 行 */
  show?: [number, number];
  /** 「変更なし」などの補足 */
  note?: string;
};

export type DiffRef = {
  file: string;
  /** 見せるハンクの番号（0 始まり）。省略時はすべて */
  hunks?: number[];
};

export type Snippet = {
  /** リポジトリにないコードであることを必ず示す */
  label: string;
  text: string;
};

export type Block =
  | { p: string; basis?: Basis }
  | { quote: string }
  | { list: string[] }
  | { pre: string }
  | { table: { head: string[]; rows: string[][] } }
  | { code: CodeRef }
  | { diff: DiffRef }
  | { snippet: Snippet }
  | { target: true };

export type Step = {
  title: string;
  /** 問いだけを見せる手順。答えは次の手順に置く */
  question?: boolean;
  blocks: Block[];
};

export type Chapter = { title: string; steps: Step[] };

export type Tour = {
  title: string;
  base: string;
  head: string;
  chapters: Chapter[];
};

export type FlatStep = Step & { chapter: number; chapterTitle: string };

export function flatten(tour: Tour): FlatStep[] {
  return tour.chapters.flatMap((c, i) => c.steps.map((s) => ({ ...s, chapter: i, chapterTitle: c.title })));
}
