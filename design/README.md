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
and counts the pixels that differ from the reference by more than 16 per channel. A state can also
ask for a 1024 pixel `tablet` viewport (`viewports: ['tablet']`), because the layout between the two
breakpoints, 820 and 1200 pixels, is reached by neither of the default two; a state that does not
list `viewports` is still shot on desktop and phone only. Candidates and a
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

Needed for a new state, when the design changes, or when the Playwright version moves. Place the
design bundle in `work/bundle/` (it is git-ignored) and run:

```sh
npm --prefix web run parity:reference                      # every state
npm --prefix web run parity:reference -- inicio            # one state
npm --prefix web run parity:reference -- --update inicio   # replace an existing reference
```

The prototype loads its runtime and its fonts from the network.

A reference that already exists is never replaced without `--update`. A shot that comes out
different from it is reported with how far away it is, discarded, and the run exits non-zero.
The reason is an instability that was seen and not explained. Shooting the same state again
normally gives the same pixels: 61 of 62 repeats did, with a cold browser and with the CPU
throttled twenty times over. One did not, and four of about 116 did not while other builds were
running on the machine. Where it was measured the difference was 108 pixels of text
anti-aliasing on a tall page, 0.007%, well under the limit. So a refused shot on an unchanged
design is this, and the answer is to run it again, not to update.

### Where the port departs from the prototype on purpose

The prototype is the reference, and in a few places it contradicts its own handoff notes. The app
follows the notes there, and the measurement pays for it:

- **A currency amount never breaks after `R$`.** The notes ask for a non-breaking space. The
  prototype's "Esta semana" card has the text typed in with an ordinary one, so its second line
  breaks inside an amount. The app keeps the amount whole. This costs 0.04% on every wide state of
  the home screen, which is nearly all of the limit; a state there that fails by a hair is this
  plus something real.
- **The pending counter is in the mono font at weight 600.** The prototype asks for 600 and loads
  that font only up to 500, so it draws the counter lighter than it specifies.
- **Near-identical colors are one token.** Each token that absorbed a variant says so in its
  description.

### A state that covers part of a screen

A state may carry a `clip` rectangle and borrow another state's reference. That is how the frame
around every screen, the sidebar and the header, was checked before any screen inside it existed.
