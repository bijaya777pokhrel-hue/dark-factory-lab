# dark-factory-lab
Dark factory lab: goal in, reviewed PR out

## What it is

A semantic font renderer in Go (standard library only). It turns plain text
into one self-contained HTML page in which every word's typography carries
what the word means:

- `lexiscore` splits the text into words and gives each word four scores.
- `renderer` turns each scored word into a `<span>` with an inline style.
- `cmd/semanticfont` is the command-line wrapper (logic in `internal/cli`).

The visual spec is [`spec/target.html`](spec/target.html).

## Scores (per word)

| Score | Range | How it is derived |
|---|---|---|
| valence | -1 .. 1 | VADER lexicon mean / 4; words not in the lexicon are 0 |
| salience | 0 .. 1 | function words (the, was, but) 0.15; other words 0.35 + 0.65 × \|valence\| |
| surprise | 0 .. 1 | \|valence − mean valence of the earlier sentiment words in the sentence\| / 2, capped at 1; 0 for neutral words and when there is no earlier sentiment word |
| certainty | 0 .. 1 | hedge words (perhaps, maybe, …) 0.15; assertive words (definitely, never, …) 0.95; everything else 0.5 |

## Mapping (scores → inline style)

| Property | Driven by | Formula |
|---|---|---|
| `font-weight` | salience | 300 + 500 × salience, rounded to a multiple of 50 |
| `font-size` | salience | 0.9 + 0.5 × salience em |
| `color` | valence | ink `#1c1c1c` eased toward vermilion `#a82e00` (negative) or blue `#0b4fa8` (positive) by √\|valence\| |
| `font-style` | certainty | `oblique Ndeg`, N = round(14 × max(0, 0.5 − certainty) / 0.5); `normal` when N = 0 |
| `letter-spacing` | surprise | 0.12 × surprise em |
| `opacity` | certainty | 0.8 + 0.2 × certainty |

Each span also carries its four scores as `data-valence`, `data-salience`,
`data-surprise` and `data-certainty` attributes.

## Accessibility

What the renderer actually does (see `renderer/render.go`):

- The page declares `lang="en"`, a responsive viewport, and
  `color-scheme: light dark`.
- Contrast: every colour the mapping can produce, at the opacity floor 0.8,
  stays at or above 4.5:1 (WCAG AA) on the white background; the worst case
  is about 4.66:1.
- Dark mode: under `prefers-color-scheme: dark` the page switches to a
  `#121212` background with lighter anchors (ink `#e6e6e6`, vermilion
  `#ff9e7a`, blue `#8cb8ff`), each word's dark colour precomputed in a
  `--dark-color` custom property with the same easing.
- Under `prefers-contrast: more` or `forced-colors: active`, every word is
  drawn at full opacity, so hedging is shown by slant alone, never by
  faintness. The page does not override forced colours.
- Colour is never the only signal: salience is also weight and size,
  surprise is letter-spacing, certainty is slant and opacity.
- Sizes are relative (`rem`/`em`), so the page follows the reader's font-size
  setting. There is no animation or transition.
- Words stay ordinary text inside `<p>` elements, so they can be selected,
  copied and read by screen readers; input text is HTML-escaped.

## Build and run

From the repository root:

```sh
go build -o /tmp/semanticfont ./cmd/semanticfont          # build
printf 'The meal was perhaps good, but dessert was horrible.' | go run ./cmd/semanticfont > spec/out.html   # run: stdin
go run ./cmd/semanticfont path/to/file.txt > out.html     # run: file
```

`semanticfont --help` prints usage. With no argument, or `-`, it reads
standard input. Exit status: 0 on success, 1 on a read or write error, 2 on
a usage error.

## Compare with the target

Writes screenshots of the target and of your output, only if Playwright is
installed:

```sh
if command -v npx >/dev/null && npx --no-install playwright --version >/dev/null 2>&1; then
  npx --no-install playwright screenshot --viewport-size=1100,420 "file://$PWD/spec/target.html" spec/target.png
  npx --no-install playwright screenshot --viewport-size=1100,420 "file://$PWD/spec/out.html" spec/out.png
else
  echo "Playwright not installed; skipping screenshots"
fi
```

`spec/out.html`, `spec/target.png` and `spec/out.png` are git-ignored.

## Third-party data

`lexiscore` embeds the VADER sentiment lexicon (MIT licence). See
[`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md).
