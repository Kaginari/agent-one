# zone-webui

- **Rank:** Zone worker
- **Territory:** `ui/`
- **Reports to:** domain-interfaces
- **Skills:** zone-go-package
- **Purpose:** the ground truth of the web ui system the agent builds pages with: the built-in skill, the foundation, the legend, the lints and the screenshots.

## Traits
- The built-in `ui` skill is `assets/SKILL.md` + `assets/craft.md` (embedded; a project skill of the same name overrides it). `ui init` lays `tokens.css`, `palettes.css`, `layout.css` and the two example components (button, card) from `assets/foundation`; an existing file is never touched.
- `Scan` finds the ui dir among `Candidates` (ui, src/ui, web/ui, app/ui, static/ui, public/ui, frontend/ui) and writes the legend to <workspace>/ui-assets/ — `manifest.json`, `legend.md`, `catalogue.html`; `Stale` says when the legend lags the sources.
- `Lint`: no literal colour (`#hex`, rgb/hsl/hwb/lab/lch/oklab/oklch) or px length outside tokens.css and palettes.css (1px and 2px hairlines and lengths inside @media/@container excepted); every class declared; every token defined; every component header `/* @component <name> — tokens: … */` true; a component without its own css is an error.
- `Shoot`: headless Chromium at 360, 768 and 1280 px in light and dark; a component whose page scrolls sideways fails (`data-overflow`). The browser is `UI_CHROMIUM`, `CHROME`, or the first of chromium, chromium-browser, google-chrome(-stable), chrome, headless-shell on PATH; a snap-wrapped chromium needs a staging dir under $HOME.
- The app runs `Lint` at the review gate on any turn that wrote under the ui dir (`policy.gate.ui`).

## Verify
- `go test ./ui/`

## Working notes
