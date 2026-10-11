// check.mjs — the 2D net's layout, held to its promises without a browser: board/web's net.js runs
// in a bare vm (a canvas stub measures text at a fixed advance per character), lays out a fixture
// workspace at the boxes the dashboard gives the net at 1280, 1440 and 1920 wide (the cockpit's net
// panel, and the net's own page — measured in Chromium), and fails when a tile leaves the view or
// a label is cut to fewer than six ems. usage: node check.mjs [team.json]; TestNetLayoutHolds
// runs it when node is on the PATH.
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import vm from 'node:vm';

const here = dirname(fileURLToPath(import.meta.url));
const src = readFileSync(join(here, '../../web/components/net/net.js'), 'utf8');
const team = JSON.parse(readFileSync(process.argv[2] || join(here, 'team60.json'), 'utf8'));

const pxOf = font => parseFloat((/([\d.]+)px/.exec(font) || [])[1]) || 14;
const advance = font => (/^(6|7|8|9)\d\d /.test(font) ? 0.6 : 0.55) * pxOf(font);
const ctx = { font: '', measureText(t) { return { width: [...t].length * advance(this.font) }; } };
const sandbox = {
  window: {},
  document: { createElement: () => ({ getContext: () => ctx }) },
  matchMedia: () => ({ matches: false, addEventListener() {} }),
};
vm.runInNewContext(src, sandbox);
const net = sandbox.window.agentOneNet;
const m = net.build(team, null, (k, d) => d || k);
const fonts = { label: '15.11px sans-serif', small: '13.43px sans-serif', bold: '650 15.11px sans-serif', caps: '700 13.43px sans-serif', track: 0.2 };
const em6 = 6 * pxOf(fonts.label) - advance(fonts.label) - 1; // six ems, less the one glyph the ellipsis takes

// [name, width, height] — the net's box at each viewport (Chromium, 16px root)
const boxes = [
  ['cockpit 1280×800', 696, 274], ['page 1280×800', 786, 610],
  ['cockpit 1440×900', 852, 316], ['page 1440×900', 942, 710],
  ['cockpit 1920×1080', 1188, 614], ['page 1920×1080', 1422, 890],
];
let bad = 0;
for (const [name, W, H] of boxes) {
  const L = net.layout2d(m, W, H, fonts);
  const problems = [];
  for (const [id, p] of L.pos) {
    const n = m.byId.get(id);
    if (p.x - p.size / 2 < 0 || p.x + p.size / 2 > L.W || p.y - p.size / 2 < 0 || p.y + p.size / 2 > L.H) problems.push(`${id}: tile outside the ${L.W}×${L.H} view at ${p.x.toFixed(0)},${p.y.toFixed(0)}`);
    if (p.hide) continue; // a strip skill's name shows on hover, whole
    const full = n.orchestrator ? (n.label || n.name) : n.kind === 'skill' ? n.name : n.name.replace(new RegExp('^' + n.rank + '-'), '');
    if (p.label !== full && ctx.measureText.call({ font: fonts.label }, p.label).width < em6) problems.push(`${id}: label cut to "${p.label}"`);
    if (!p.strip && p.lx + ctx.measureText.call({ font: fonts.label }, p.label).width > L.W + 1) problems.push(`${id}: label runs past the view`);
  }
  const fits = L.H <= H + 0.5 && L.W <= W + 0.5;
  console.log(`${problems.length ? 'FAIL' : 'ok  '} ${name}: ${L.W}×${L.H.toFixed(0)} in ${W}×${H}${fits ? '' : ' (grown: the stage scrolls)'} · ${L.k} sub-column(s) · skills ${L.strip ? 'as a strip' : 'as a column'} · row ${L.G.toFixed(1)} px, tile ${L.T}`);
  for (const p of problems) console.log('     ' + p);
  bad += problems.length;
}
process.exit(bad ? 1 : 0);
