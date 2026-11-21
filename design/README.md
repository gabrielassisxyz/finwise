# Design

The interface is designed outside this repository and arrives as one HTML prototype with its own
template runtime. The prototype is a reference for appearance and behaviour. None of its code is
ported, and the bundle itself is not versioned. What is versioned is what was taken from it:

| path | what it is |
|---|---|
| `tokens/finwise.tokens.json` | every design decision of the prototype, in the W3C design tokens format |
| `reference/*.png` | the prototype rendered headless, one screenshot per state and viewport |

## Tokens

The prototype writes its values inline, about ninety distinct colors among them. Its handoff notes
name the ones that are decisions; the token file holds those, and maps the near-identical variants
onto them. Where two values were unified the token's description says so.

Colors are kept in `oklch`, the notation of the design.

```sh
npm --prefix web run tokens
```

builds `web/src/styles/tokens.css`. The gate fails when that file is stale, and when a component
writes a color, a type size, a radius, a shadow or a spacing step as a literal instead of a token.

## Parity

A ported screen is measured, not eyeballed.

```sh
npm --prefix web run parity              # every ported state
npm --prefix web run parity -- inicio    # one state
```

builds the app, renders each state in `web/scripts/parity/states.mjs` at 1440 and 390 pixels wide,
and counts the pixels that differ from the reference by more than 16 per channel. Candidates and a
heatmap per state are written to `work/parity/`. A page that comes out taller or shorter than its
reference is still measured: the area one side lacks counts as differing.

A state passes at 0.05% or less. The limit is that low on purpose. A faithful screen measures 0.00
to 0.01%, while a wrong logo color measured 0.11% and a webfont that failed to load measured 0.09%
on a screen that is mostly empty; a looser limit lets both through.

The app renders the prototype's own dataset through its in-memory backend
(`web/src/api/fake/prototype-data.ts`), which is what makes a pixel comparison possible. Changing
that dataset changes the baseline.

### The browser build

Both sides of the comparison must come out of the same browser build, or font rasterisation alone
moves the number. The references here were shot with the Chromium that the pinned Playwright
version downloads (`npx playwright install chromium`, run inside `web/`). `CHROMIUM_PATH` points
the scripts at another binary; the references then have to be shot again with it.

### Shooting the references again

Needed when the design changes or the Playwright version moves. Place the design bundle in
`work/bundle/` (it is git-ignored) and run:

```sh
npm --prefix web run parity:reference
```

The prototype loads its runtime and its fonts from the network.

### A state that covers part of a screen

A state may carry a `clip` rectangle and borrow another state's reference. That is how the frame
around every screen, the sidebar and the header, was checked before any screen inside it existed.
