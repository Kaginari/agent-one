# Craft — how a UI earns the word "designed"

Rules for the agent building a UI in this app. Terse and imperative; the reasons are one line each.
Everything here works offline: the numbers are the method, no site is consulted at work time.

## What model-made UIs get wrong, and the standing fix

| Symptom | Why it happens | Do this instead |
|---|---|---|
| The generic look: centred hero, three feature cards, blue gradient button, rounded-xl everything | "clean and modern" is not a decision | Pick one palette by name (`palettes.css`) and one voice (§Density) before writing markup. Say it in the component header. |
| Over-decoration: gradients, glass, shadows, icons on every row | decoration substitutes for hierarchy | One emphasis device per surface: a border **or** a shadow **or** a fill, never all three. Icons only where a word would be longer. |
| Weak hierarchy: everything bold, all text the same size | no answer to "what is this screen for" | Name the one primary action and the one headline first; everything else is subordinate to them. |
| Inconsistent spacing: 13px here, 18px there | values invented per element | Only the spacing tokens (§Rhythm). A value not on the scale is a bug. |
| Missing states | only the happy path was imagined | Every component ships empty, loading, error, disabled and focus-visible states, in its own `.html`, before it is called done. |
| Placeholder content: lorem ipsum, "Item 1", "$0.00" | layout tuned to fake data | Real names, real amounts, real lengths, including the longest plausible string and a 0-count. |
| Feature overload: eight sections on one screen | the whole product in one prompt | One primary action per screen; secondary actions go to menus, tabs, or another screen. |
| Drift between screens | each screen restyled from scratch | Components and tokens only; a new colour or size is added to `tokens.css`, never inline. |
| Fixed widths, hover-only affordances, unlabelled icon buttons | desktop and mouse assumed | Layout blocks from `layout.css` (they wrap by themselves), 44px targets, every control labelled. |
| Contrast that looks fine but fails | judged by eye | Compute it (§Contrast). Nothing ships under 4.5:1 for text, 3:1 for UI edges and large text. |
| Nested cards, boxes in boxes | a border used as a grouping tool | Group with space and headings first; a card is for a thing that is actually a unit (one record, one choice). |
| First draft accepted | one pass | Three passes, in this order: structure and content → tokens and type → states, motion, polish. |

## Hierarchy

- Three levels per screen, no more: primary (one headline, one action), secondary (section titles,
  key figures), tertiary (body, meta, controls). If a fourth is needed, split the screen.
- Emphasise with size and weight before colour; colour is the scarcest channel (see §Palette).
- The accent is spent, not sprinkled: one primary button, links, the active nav item, focus rings.
  Two accent-coloured buttons side by side is a hierarchy failure.
- Numbers people scan (totals, prices, counts) get the tabular figures feature and a larger step
  than their label. The label sits above or left in `--ink-muted`.
- Muted text is for metadata and helper copy, never for the thing the user came to read.
- Alignment carries hierarchy: left-align text, right-align numbers, one baseline grid per row.
- Reading order equals DOM order equals tab order. If the visual order differs, fix the layout.

## Rhythm — spacing

- One scale, in tokens.css, fluid and scaled by `--density`: `--space-3xs 2xs xs s m l xl 2xl 3xl`.
  Nothing off-scale.
- Inside a component: `--space-2xs` to `--space-s`. Between components: `--space-m` to `--space-l`.
  Between sections: `--space-xl` to `--space-3xl`. Related things sit closer than unrelated things; that
  is the whole law of grouping.
- Set gaps on the parent (`stack`, `cluster`, `grid`), never margins on children; a child does
  not know where it will land.
- Padding on a surface is even on all sides, or wider horizontally than vertically; never the
  reverse.
- Page gutter: `--gutter` (fluid). Text measure: `max-width: 65ch` on prose, never on tables.
- Radius comes from one token per size (`--radius-s`, `--radius-m`, `--radius-l`, `--radius-pill`); the inner
  radius of a nested element is the outer radius minus the padding, never the same value.

## Type — scale and use

- One family for UI, optionally one for display, plus one mono for code and figures. Never a
  third. Prefer the system stack unless the brand names a font; no font loads at work time.
- A ratio scale on a fluid base: `--type-base` (fluid) and `--type-ratio` — ×1.2 (dense tools) or
  ×1.25 (marketing) — give `--step--2 … --step-5`. Use the steps, not px.
- Weights: two, usually 400 and 600. Bold everything is bold nothing.
- Line height: 1.5 body, 1.25 headings, 1.1 display, 1.4 in dense tables. Tighter as it gets bigger.
- Letter spacing only on small caps or all-caps labels (`.04em`); never on body text.
- Headings are semantic (`h1`–`h3`) and styled by class; the visual size never dictates the level.

## Density

- Choose a voice per app and keep it: **dense** (data tools: `data-density="compact"`, ratio 1.2),
  **regular** (products: the default), **airy** (marketing, reading: `data-density="comfortable"`,
  ratio 1.25). One attribute on `<html>` moves every space token; components never change.
- Tables and lists in dense voice get zebra or row borders, not both; hover highlight uses
  `--surface-2`, not the accent.
- Whitespace is the affordable emphasis. Before adding a divider, try doubling the gap.

## Palette — choosing colour offline

Colour is decided once, in `tokens.css`, from the raw tier. Start from a mood or a brand hue.

1. **Mood → named palette.** Pick from `palettes.css`: calm, trust, warm, earth, neon, pastel,
   mono, retro, forest, ocean, sunset, ink. Set `data-palette` on `<html>` and stop; the mapping
   below is already wired (no attribute: the neutral-blue default on `:root`). Only continue if
   the brand gives a colour of its own.
2. **Brand hue → ladder.** Convert the brand colour to OKLCH (L, C, H). Place it on the rung
   whose L it is nearest, then build the other rungs at these L values with the same hue:
   `--p-4` deep .30 · `--p-1` accent .46 · `--p-2` hero .64 · `--p-1-dark` .79 · `--p-5` tint .94.
   Then apply drift: the deep leans up to 20° toward the cool pole of the hue (blue/violet
   side), the tint and `--p-1-dark` up to 30° toward the warm pole (yellow side); a hand-picked
   palette almost always has this drift, a flat one looks synthetic. Chroma peaks at the hero
   and falls toward both ends: tint C ≤ .05, deep C ≤ .10. `--p-3` is the counter-hue at hero
   lightness (§3); `--p-1-ink` is `--n-0`, `--p-1-dark-ink` is `--n-9`.
3. **Hue relationships** — one of four, never mixed on one screen:
   - single hue with drift (calm, trust, forest, ocean, ink): the safe default;
   - analogous, hues within 60° (warm, earth, sunset): rich without clashing;
   - lightness-split complement (retro): the cool hue lives at the deep rungs, the warm hue at the
     bright and tint rungs; they never meet at the same L, so they never fight;
   - neon pair: two vivid hues 120–160° apart at the bright rung on a near-black base; only one
     is ever text.
4. **Chroma bands by mood:** pastel and earth peak ≤ .10; calm, trust, forest, ocean .10–.17;
   warm, sunset, retro .14–.18; neon .19–.26. Neutrals: C .004–.02 on the palette's hue, rising
   slightly toward the dark end; C 0 only for mono. Pure black and pure white are never used.
5. **Neutral ladder**, ten rungs of L: .985 .955 .90 .82 .70 .58 .47 .35 .25 .16. The light
   theme reads it top-down, the dark theme bottom-up: one raw tier serves both. The rungs
   that carry text (`--n-9`, `--n-6` light; `--n-0`, `--n-3` dark) clear 4.5:1 on both
   surfaces by construction; keep the L values if you retint the hue.
6. **Map raw → meaning**, the same for every palette:

   | meaning token | light | dark | use |
   |---|---|---|---|
   | `--surface` | `--n-0` | `--n-9` | page |
   | `--surface-2` | `--n-1` | `--n-8` | cards, inputs, table stripes, hover |
   | `--line` | `--n-3` | `--n-6` | borders, dividers (≥1.3:1 on surface) |
   | `--ink` | `--n-9` | `--n-0` | body text |
   | `--ink-muted` | `--n-6` | `--n-3` | labels, meta, helper copy |
   | `--accent` | `--p-1` | `--p-1-dark` | primary button, links, active nav |
   | `--accent-ink` | `--p-1-ink` | `--p-1-dark-ink` | text on the accent: the surface colour, always |
   | `--accent-wash` | `--p-5` | `--p-4` | selected rows, callouts, badges |
   | `--accent-hero` | `--p-2` | `--p-2` | brand fills, charts, focus ring; never body text |
   | second series / tag | `--p-3` | `--p-3` | the counter-hue; charts, tags, never text |
   | `--ok` / `--warn` / `--danger` | L .48 / .50 / .50 | L .78 / .80 / .76 | set in `tokens.css`, not per palette |

   Status hues are fixed (ok 150, warn 75, danger 25); their L follows the accent's rung in each
   theme and their chroma sits mid-band (.10–.14), so they belong to every palette without
   being the brand. Dark mode is a second reading of the ladder, not a second palette.
7. **Contrast, computed not eyeballed.** For text on a neutral surface, relative luminance is
   close to L³, so contrast ≈ (L₁³ + .05) / (L₂³ + .05). The rungs that clear 4.5:1 on both
   surfaces: light theme text L ≤ .50 on L ≥ .955; dark theme text L ≥ .65 on L ≤ .25. The safe
   zone is wider: ink L ≥ .90 on surface L ≤ .25 gives 12:1+, and the inverse in light. For
   the hero rung (L .64) expect ≈3:1 on both surfaces: large text and UI edges only. Chroma
   lowers luminance a little for blues and raises it for yellows, so keep .05 of L in hand
   near a limit. `agent-one ui check` measures the real pairs; until it runs, do the arithmetic.
8. **Chromatic text** is only the accent of its theme (`--p-1` light, `--p-1-dark` dark) or a
   status colour; any other rung as text is a defect, however good it looks.

## Interfaces for a model — chat, agents, consoles

The default chat layout (prompt parked at the bottom, output above, a sidebar of mystery icons) is
inherited, not designed. An agent console is a reading surface first and a control surface second.

- **Output reads like a book.** Model text sits in `.center` at `--measure`, body step, `--leading`;
  code and tables may break the measure, prose never does. No bubbles for long answers.
- **One input, always in the same place**, reachable by keyboard (focus on load, Enter sends,
  Shift+Enter breaks a line, Up recalls the last ask, Esc interrupts). Its state is visible:
  idle · sending · streaming · waiting on the human.
- **Stream with a stable layout.** Reserve the block before tokens arrive; never reflow what the
  reader is reading; auto-scroll only while the reader is at the bottom.
- **Work is shown as structure, not chatter.** Tool calls, file edits, verdicts and costs are
  collapsible cards with a status word (running, done, refused) and one line of summary; the
  detail (diff, output) opens on demand.
- **Every icon has a word** (visible label or tooltip plus `aria-label`); a control nobody can
  name is removed.
- **Numbers carry their source and age** (tokens, cost, time): a stale or missing reading shows
  as such, never as zero.
- **Long sessions stay navigable:** headings per turn, a jump list, search; old turns fold.
- **Interrupting and undoing are first-class** buttons, not hidden commands.

## Self-review before "done"

- [ ] One primary action per screen; one headline; three hierarchy levels, not four.
- [ ] Every value is a token: colour, space, radius, size. `grep -c "px" *.css` is near zero
      outside `tokens.css` and borders.
- [ ] Palette named on `<html>` (or the default meant); both themes rendered; components use
      the meaning tier only, never `--p-*`/`--n-*` directly.
- [ ] Text ≥ 4.5:1, UI edges and large text ≥ 3:1, on `--surface` and `--surface-2`, both themes.
- [ ] Empty, loading, error, disabled and focus-visible states exist and are reachable in the
      component's `.html`.
- [ ] Real content, including the longest string, a zero, a very large number, a 2-line title.
- [ ] Narrow (360px), mid (768px) and wide (1280px) all read: no horizontal scroll, no
      overlapping text, no orphaned button; the layout blocks wrap rather than break.
- [ ] Keyboard: everything reachable, order matches reading order, focus ring visible on the
      accent hero colour.
- [ ] Icon buttons labelled; images have alt; form fields have `<label>`, errors are text.
- [ ] `prefers-reduced-motion` respected; animation only communicates a state change, ≤ 200ms.
- [ ] Component header lists the tokens it uses and no others: `/* @component card — tokens: … */`.
- [ ] Zoom the page to 200% and to 50%: hierarchy still reads at both.
- [ ] Ask "what would a stranger click first?" and check it is the primary action.
