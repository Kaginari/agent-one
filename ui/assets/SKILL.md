---
name: ui
description: Build or change a web UI (HTML, CSS, JS) — pages, components, themes, layouts. A token-driven component system that stays responsive and takes global and radical changes in one file. Use before writing any markup or styles.
---

# ui — building pages that take a global change in one edit

## 0. Read before you write

1. Find the ui dir: `{{BIN}} ui scan` finds it (`ui/`, `src/ui/`, `web/ui/` …) and regenerates
   the legend. No ui dir and no other system → `{{BIN}} ui init` lays the foundation into `ui/`.
2. Read `{{WORLD}}/ui-assets/legend.md` — every component, the classes it owns, its variants, the
   tokens it consumes. Reuse before you add: a new component that duplicates one in the legend
   is a defect.
3. The project already has a system (Tailwind, Bootstrap, a design-system package, CSS modules,
   a framework's components)? **Follow it** and apply the principles below inside it: its theme
   config is the tokens file, its components are the components. Do not lay a second system
   beside the first.

## 1. The shape (vanilla or framework-free pages)

```
ui/
  tokens.css        the tiers: raw scales, meaning (--surface --ink --accent --space-m --step-1 …)
  palettes.css      the raw colour tier: [data-palette="…"] swaps every colour at once
  layout.css        the page grid and primitives: .page .stack .cluster .grid .cols .sidebar .switcher .center .cover .frame
  components/<name>/<name>.css    one component: its header, its classes, its data-variants
  components/<name>/<name>.html   a sample of it, every variant — the catalogue renders it
```

Cascade layers fix the order: `@layer reset, tokens, layout, components, utilities;` — a
global rule never loses a specificity fight, and a third-party sheet goes in its own layer below
`components`.

## 2. The rules

- **Tokens only.** Outside tokens.css and palettes.css there is no literal colour and no pixel
  length (1px/2px hairlines and media/container query conditions excepted). Colours are
  `var(--surface)`, `var(--ink)`, `var(--accent)`…; lengths are `var(--space-*)`, `var(--step-*)`,
  `rem`, `ch`, `%`, or `clamp()`. `ui check` fails a literal.
- **Three tiers, one direction.** raw → meaning → component. A component reads meaning tokens
  and may define its own `--<name>-*` aliases for its variants; it never reads the raw tier.
- **One component = one dir, one root class** named after it (`.card`), children prefixed
  (`.card-title`), variants as attributes (`data-variant="raised"`, `data-size="s"`), states as
  ARIA or native attributes (`aria-expanded`, `:disabled`). No ids in a reusable piece.
- **A component's inputs are `--<name>-*`.** A value only the page knows (a fill, a progress, a
  tint) comes in as a custom property on the element — `style="--meter-value: 42%"` or
  `el.style.setProperty('--meter-value', …)` — and the css reads it with a fallback
  (`var(--meter-value, 0%)`). The legend lists them as inputs; nothing else is set inline.
- **The header is the legend's source.** A component css starts
  `/* @component <name> — tokens: --a, --b. <what it is, its variants>. */` listing exactly the
  tokens it reads; `ui check` holds it true.
- **Layout belongs to layout.css.** A component never sets its own outer margin or page
  position; it sits inside a primitive (.stack gives the gap, .grid the columns). Spacing between
  siblings is the parent's gap.
- **Responsive by the space given, not the screen.** Prefer intrinsic layouts (flex-wrap,
  `repeat(auto-fit, minmax(min(16rem, 100%), 1fr))`), container queries on the component
  (`container-type: inline-size` + `@container`), fluid type (`--step-*`). A viewport media query
  is for the page shell only. Nothing scrolls sideways at 360px.
- **Semantic HTML first**: landmarks (`header nav main footer`), headings in order, `button` for
  actions and `a` for navigation, labels on every input, alt on every image. Keyboard reachable,
  visible focus (the reset gives `:focus-visible`).
- **JS enhances.** The page reads and navigates without it; JS toggles attributes
  (`data-state`, `aria-*`, `hidden`) and the CSS draws the states — no inline styles from JS
  except custom properties (`el.style.setProperty('--progress', …)`).

## 3. Global and radical changes — edit one tier

| change | where |
|---|---|
| retheme, brand colour | palettes.css (a palette) or `data-palette` on `<html>` |
| dark / light | meaning tier in tokens.css (`[data-theme]` + `prefers-color-scheme`) |
| denser / airier | `--density` (or `data-density="compact|comfortable"`) |
| bigger / smaller type, more contrast in scale | `--type-base`, `--type-ratio` |
| rounder / sharper | `--radius-*` |
| new page layout | layout.css primitives, or the page's own arrangement of them |
| one component's look | its css, through its own `--<name>-*` aliases |

A change that needs edits in many component files means a token is missing: add the token, then
make the components read it.

## 4. Prove it

1. `{{BIN}} ui check` — scans, regenerates the legend, lints (literal values, undeclared classes,
   unknown tokens, untrue headers), and renders `{{WORLD}}/ui-assets/catalogue.html` at 360, 768
   and 1280px in light and dark into `{{WORLD}}/ui-assets/shots/`. It reports any component that
   scrolls sideways.
2. **Look at the screenshots** (read the PNGs) — markup that passes every lint can still be ugly
   or wrong. Fix, re-check, look again.
3. The gate runs the lints on every turn that touched the ui dir; it does not take screenshots —
   you do, before calling the work done.

Colour and visual craft follow.
