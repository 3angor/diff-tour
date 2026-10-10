# diff-tour

Walk through a diff the way a senior engineer reads it.

**English** | [日本語](README.ja.md)

![tour walking through a change that adds coupons](demo/demo-en.gif)

> Status: early prototype. Tours are written by [Claude Code](https://claude.com/claude-code) for now; other LLMs are planned.

AI can write code that works, but it does not teach you *why* the code is written that way.
`tour` walks you through a change the way a senior engineer would read it: what the change does to the behavior of the whole system, which existing code it depends on (even files that did not change), why a seemingly redundant line is there, and where a senior would stop and say "wait".

You only need two keys: **→** next and **←** back.

## Try it

Requires git and [Claude Code](https://claude.com/claude-code) (`claude`) signed in.

Download the archive for your OS from [Releases](https://github.com/init-co-jp/diff-tour/releases), and put `tour` on your `PATH`. Or, with Go 1.27 or later:

```sh
go install github.com/init-co-jp/diff-tour/cmd/tour@latest
```

Then:

```sh
cd your-repo
tour
```

On the first run, `tour` asks two things: the language (detected from your terminal, English by default) and the LLM that writes tours. Run `tour setup` to change them later.

Then pick the change to tour. Your current branch, including uncommitted changes, is selected, so you can just press Enter.
Writing a tour takes a few minutes; you can watch which files are being read. The tour is saved, so opening it again is instant.

| Command | Tours |
| --- | --- |
| `tour` | a branch you choose, compared with the default branch. If you have uncommitted changes, the current branch **with** them is listed first |
| `tour <branch>` | that branch |
| `tour <commit>` | that single commit |
| `tour <base>..<head>` | that range |
| `tour <file>.json` | a tour file (for example `tours/example-coupons.en.json`) |

| Key | Action |
| --- | --- |
| `→` `space` `enter` | Next step |
| `←` `backspace` | Previous step |
| `o` | Open the code of this step in your editor (`$VISUAL` / `$EDITOR`, or `TOUR_EDITOR="code -g {file}:{line}"`) |
| `q` | Quit |
| `↑` `↓` | Scroll, only when a step does not fit on the screen |

## What a step can show

- Code and diffs, always read from git at the right commit, with syntax highlighting for any language
- Code that is **not** in the repository (a naive alternative, a suggested fix), always labeled as such
- Flow and timeline diagrams
- Database schemas before and after the change, computed by applying the migrations to an in-memory SQLite database (SQLite migrations only for now)
- Tables of example data
- Questions that make you think before the answer is shown on the next step

Every explanation is labeled with how it is known: **Verified** (backed by code, tests or commit messages), **From the code** (inferred from the structure), or **Senior view** (general design judgment, not verified in this repository).

## Read-only by design

`tour` never writes to your working tree or `.git`.

- To write a tour, it exports the files before and after the change to your cache directory and runs Claude Code there with read-only tools only (`Read`, `Grep`, `Glob`; no shell, no web). Files ignored by `.gitignore` (such as `.env`) are never exported. The code of the change is sent to the LLM you chose.
- When you open a file that differs from the commit being toured, it opens a read-only copy under your cache directory instead.

## Development

```sh
go test ./...
go run ./cmd/tour tours/example-coupons.en.json --repo path/to/diff-tour-example --dump --cols 140 --rows 44 --plain
```

`--dump` prints every step at a fixed screen size, which is handy for checking layouts without a terminal.

## License

[MIT](LICENSE)
