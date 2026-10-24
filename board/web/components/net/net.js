// net.js — the workspace as a neural net. The hierarchy is drawn as layers (the orchestrator, then each rank
// in the order work flows down, the skills beside), each neuron a rank-coloured tile whose cells are
// its last four weeks of work (TensorFlow-playground style: the neuron shows its own output), each
// tree bond weighted by the commissions it carried. Hover or focus lights a neuron's path to the
// orchestrator and its subtree; a Subagent's dispatch travels down the tree as a pulse and its report comes
// back up. The 3D stage (net3d.js) draws the same model. No dependencies; colours come from the css
// tokens only (the svg reads them through classes; the canvas resolves them from probes).
(() => {
  'use strict';
  const NS = 'http://www.w3.org/2000/svg';
  function s(tag, attrs, ...kids) {
    const n = document.createElementNS(NS, tag);
    for (const [k, v] of Object.entries(attrs || {})) if (v !== false && v !== null && v !== undefined) n.setAttribute(k, v === true ? '' : v);
    for (const kid of kids.flat()) if (kid !== null && kid !== undefined && kid !== false) n.append(typeof kid === 'string' ? document.createTextNode(kid) : kid);
    return n;
  }
  // the icons are the board's: a node's icon (/api/team) is its portrait or its family's glyph,
  // resolved once in Go; a skill holds the sprite's brain (Lucide brain-circuit, web/icons/SOURCE)
  const SPRITE = 'web/icons/icons.svg';
  const UP_BONDS = ['truth', 'verdict', 'reports', 'above'];
  const RANK_ORDER = ['orchestrator', 'coord', 'principal-coordinator', 'auditor', 'service', 'domain', 'principal-domain-owner', 'zone'];
  const clamp = (v, a, b) => Math.max(a, Math.min(b, v));
  const reducedQ = matchMedia('(prefers-reduced-motion: reduce)');

  // ---------- the model: one tree under the orchestrator, the skills beside it ----------
  // team: the /api/team payload as dash.js merged it (agents and skills joined). activity: the
  // /api/activity payload, or null when the instruments are silent (tiles stay blank, honestly).
  function build(team, activity, word) {
    const all = team.nodes || [];
    const byId = new Map(all.map(n => [n.id, n]));
    let orchestrator = byId.get(team.orchestrator) || all.find(n => n.orchestrator) || null;
    const isSkill = n => n.kind === 'skill';
    const members = all.filter(n => !isSkill(n));
    const skills = all.filter(isSkill);
    const kids = new Map(), parent = new Map();
    const edgeOf = new Map(); // child id → the team edge that ties it to its parent
    for (const e of team.edges || []) if (UP_BONDS.includes(e.bond) && byId.has(e.to)) {
      const have = edgeOf.get(e.from);
      if (!have || UP_BONDS.indexOf(e.bond) < UP_BONDS.indexOf(have.bond)) edgeOf.set(e.from, e);
    }
    for (const n of members) {
      if (n === orchestrator) continue;
      let p = n.parent && byId.has(n.parent) && n.parent !== n.id ? n.parent : (edgeOf.get(n.id) || {}).to;
      if (!p || !byId.has(p)) p = orchestrator ? orchestrator.id : '';
      if (!p) continue;
      parent.set(n.id, p);
      if (!kids.has(p)) kids.set(p, []);
      kids.get(p).push(n.id);
    }
    const rankKey = n => { const i = RANK_ORDER.indexOf(n.rank); return i < 0 ? RANK_ORDER.length : i; };
    for (const list of kids.values()) list.sort((a, b) => rankKey(byId.get(a)) - rankKey(byId.get(b)) || byId.get(a).name.localeCompare(byId.get(b).name));
    // depth by walking down from the orchestrator; anything the walk misses (a loop) hangs at depth 1
    const depth = new Map();
    const order = []; // members in preorder
    const walk = (id, d, seen) => {
      if (seen.has(id)) return;
      seen.add(id); depth.set(id, d); order.push(id);
      for (const k of kids.get(id) || []) walk(k, d + 1, seen);
    };
    const seen = new Set();
    if (orchestrator) walk(orchestrator.id, 0, seen);
    for (const n of members) if (!seen.has(n.id)) walk(n.id, orchestrator ? 1 : 0, seen);
    const act = (activity && activity.agents) || {};
    const tree = [];
    for (const [c, p] of parent) {
      const e = edgeOf.get(c);
      const bond = e && e.to === p ? e.bond : 'reports';
      const a = act[byId.get(c).name];
      tree.push({ up: p, down: c, bond, implied: !(e && e.to === p) || !!(e && e.implied), weight: a ? a.total : 0 });
    }
    const holds = [];
    for (const e of team.edges || []) if (e.bond === 'holds' && byId.has(e.from) && byId.has(e.to) && isSkill(byId.get(e.to))) holds.push({ from: e.from, to: e.to });
    let maxDepth = 0;
    for (const d of depth.values()) maxDepth = Math.max(maxDepth, d);
    // per-node activity: one value a day (tokens when the usage journal spoke, else runs)
    let peak = 0;
    const cells = new Map();
    for (const n of all) {
      const a = act[n.name];
      if (!a) continue;
      const v = (a.tokens || []).some(x => x > 0) ? a.tokens : a.runs || [];
      cells.set(n.id, v);
      for (const x of v) peak = Math.max(peak, x);
    }
    // size: the code a member owns (its mass, lines of source; the server walks it), normalised
    // over the workspace — a leaf by the square root of its own, a member with a subtree by the cube
    // root of its subtree's (a gentler curve), the orchestrator capped, a skill by how many hold it.
    // Activity never enters it: work shows as glow and cells, not size.
    const massK = new Map();
    const norm = (ids, f) => {
      const v = ids.map(id => f(byId.get(id).mass || 0));
      const lo = Math.min(...v), hi = Math.max(...v);
      ids.forEach((id, i) => massK.set(id, hi > lo ? (v[i] - lo) / (hi - lo) : 0.5));
    };
    const leafIds = members.filter(n => n !== orchestrator && !(kids.get(n.id) || []).length).map(n => n.id);
    const innerIds = members.filter(n => n !== orchestrator && (kids.get(n.id) || []).length).map(n => n.id);
    if (leafIds.length) norm(leafIds, Math.sqrt);
    if (innerIds.length) norm(innerIds, Math.cbrt);
    if (skills.length) norm(skills.map(n => n.id), Math.sqrt);
    if (orchestrator) massK.set(orchestrator.id, 1);
    const sized = all.some(n => n.mass > 0);
    return { byId, orchestrator, members, skills, kids, parent, depth, order, tree, holds, maxDepth, cells, peak, act, days: (activity && activity.days) || [], word, relates: team.relates || [], recent: (activity && activity.recent) || [], massK, sized };
  }

  // ancestors (the path to the orchestrator) and descendants (the subtree) of a node
  function lineage(m, id) {
    const up = [], down = [];
    for (let p = m.parent.get(id), guard = 0; p && guard < 64; p = m.parent.get(p), guard++) up.push(p);
    const stack = [...(m.kids.get(id) || [])];
    while (stack.length) { const k = stack.pop(); down.push(k); stack.push(...(m.kids.get(k) || [])); }
    return { up, down };
  }
  // the lit set of a focus: path to the orchestrator, subtree, held skills (or a skill's holders)
  function litSet(m, id) {
    const nodes = new Set([id]), tree = new Set(), holds = new Set();
    const n = m.byId.get(id);
    if (!n) return { nodes, tree, holds };
    if (n.kind === 'skill') {
      for (const w of m.holds) if (w.to === id) { nodes.add(w.from); holds.add(w.from + '>' + w.to); }
      return { nodes, tree, holds };
    }
    const { up, down } = lineage(m, id);
    let c = id;
    for (const p of up) { nodes.add(p); tree.add(p + '>' + c); c = p; }
    for (const d of down) { nodes.add(d); tree.add(m.parent.get(d) + '>' + d); }
    for (const w of m.holds) if (w.from === id) { nodes.add(w.to); holds.add(w.from + '>' + w.to); }
    return { nodes, tree, holds };
  }
  // a rank's plate lit: every member of those ranks, and the bond each hangs from
  function rankSet(m, ranks) {
    const nodes = new Set(), tree = new Set(), holds = new Set();
    for (const n of m.members || []) {
      if (!ranks.includes(n.rank || '')) continue;
      nodes.add(n.id);
      const p = m.parent.get(n.id);
      if (p) tree.add(p + '>' + n.id);
    }
    return { nodes, tree, holds };
  }
  // the hops a dispatch travels: from the nearest running ancestor (else the orchestrator) down to id
  function route(m, id, live) {
    const { up } = lineage(m, id);
    let stop = up.length;
    for (let i = 0; i < up.length; i++) if (live.has(m.byId.get(up[i]).name)) { stop = i + 1; break; }
    const chain = [id, ...up.slice(0, stop)].reverse(); // dispatcher … id
    const hops = [];
    for (let i = 0; i + 1 < chain.length; i++) hops.push(chain[i] + '>' + chain[i + 1]);
    return hops;
  }

  // ---------- text measure (labels decide the column widths) ----------
  const measureCtx = document.createElement('canvas').getContext('2d');
  function measure(text, font) { measureCtx.font = font; return measureCtx.measureText(text).width; }
  function fit(text, font, max) {
    if (measure(text, font) <= max + 0.5) return text; // half a pixel of slack: widths are sums of measures
    let lo = 1, hi = text.length;
    while (lo < hi) { const mid = (lo + hi + 1) >> 1; if (measure(text.slice(0, mid) + '…', font) <= max) lo = mid; else hi = mid - 1; }
    return text.slice(0, lo) + '…';
  }

  // ---------- the 2D layout ----------
  // wide: TF-playground columns, one per layer, the tree tidy (a parent sits at the middle of its
  // children, each upper layer centred in its own column); narrow: an outline, one row per neuron,
  // indented by depth — the tree still reads. availH is the box the net is given (the cockpit's
  // panel) or the height it would like; the layout fills it exactly when it can, and only grows
  // past it (the stage then scrolls) when even the tightest legible packing will not fit.
  const LABEL_EM = 6; // no label is cut to fewer than this many ems
  function layout2d(m, W, availH, fonts) {
    const narrow = W < 640;
    const label = n => n.orchestrator ? (n.label || n.name) : (n.rank && n.name.startsWith(n.rank + '-') ? n.name.slice(n.rank.length + 1) : n.name);
    const pos = new Map();
    const cols = [];
    const TT = narrow ? 32 : 38, TM = 18;
    if (narrow) {
      const T = 24, G = 40, indent = clamp((W - 190) / Math.max(1, m.maxDepth), 18, 34), top = 16;
      let y = top + TT / 2;
      for (const id of m.order) {
        const n = m.byId.get(id), d = m.depth.get(id) || 0, size = n.orchestrator ? TT : T;
        const x = 12 + size / 2 + d * indent;
        const room = W - (x + size / 2 + 8) - 8;
        pos.set(id, { x, y, size, label: fit(label(n), fonts.label, room), lx: x + size / 2 + 8, d });
        y += n.orchestrator ? G + 6 : G;
      }
      let H = y;
      if (m.skills.length) {
        cols.push({ kind: 'skills', x: 4, y: H - 8, w: W - 8, h: 0, title: m.word('skills', 'skills'), count: m.skills.length, rank: 'skill:shared' });
        let my = H + 24;
        for (const n of skillOrder(m, pos)) {
          const x = 12 + TM / 2;
          pos.set(n.id, { x, y: my, size: TM, label: fit(n.name, fonts.small, W - x - 40), lx: x + TM / 2 + 8, d: -1 });
          my += 30;
        }
        cols[0].h = my - cols[0].y;
        H = my;
      }
      return { pos, cols, W, H: H + 8, narrow };
    }
    const pxOf = f => parseFloat((String(f).match(/([\d.]+)px/) || [])[1]) || 14;
    const px = pxOf(fonts.label);
    const em6 = LABEL_EM * px;
    // rows: the deepest layer's leaves take consecutive slots and a parent sits at its children's
    // middle. Each subtree under the orchestrator is a lane (a coordinator with its domain owners and their zone workers): the
    // lanes are set apart by a gap and drawn as bands. A shallow leaf takes no row: under the
    // orchestrator (the service owner, a coordinator with no domain owners) it is a satellite drawn just below the orchestrator in its
    // own column; deeper, it hangs just below (or above) its nearest sibling that has a subtree.
    const leaves = [], slot = new Map(), shallow = new Map(), laneOf = new Map(), satellites = [];
    const isLeaf = id => !(m.kids.get(id) || []).length;
    const orchestratorId = m.orchestrator ? m.orchestrator.id : '';
    let laneN = 0;
    const place = (id, lane) => {
      const ks = m.kids.get(id) || [];
      if (!ks.length) { slot.set(id, leaves.length); leaves.push(id); laneOf.set(id, lane); return; }
      const anchored = ks.some(k => !isLeaf(k));
      const real = [];
      ks.forEach((k, i) => {
        if (!anchored || !isLeaf(k)) { place(k, id === orchestratorId && anchored ? laneN++ : lane); real.push(k); return; }
        if (id === orchestratorId) { satellites.push(k); slot.set(k, -1 + satellites.length * 1e-3); return; }
        let a = -1, n = 0;
        for (let j = i - 1; j >= 0 && a < 0; j--) if (!isLeaf(ks[j])) { a = j; n = i - j; }
        for (let j = i + 1; j < ks.length && a < 0; j++) if (!isLeaf(ks[j])) { a = j; n = i - j; }
        shallow.set(k, { anchor: ks[a], n });
      });
      slot.set(id, real.reduce((s, k) => s + slot.get(k), 0) / real.length);
      for (const [k, sh] of shallow) if (!slot.has(k) && ks.includes(k)) slot.set(k, slot.get(sh.anchor) + sh.n * 1e-3);
    };
    if (m.orchestrator) place(m.orchestrator.id, -1);
    for (const id of m.order) if (!slot.has(id)) place(id, laneN++);
    // the virtual row of each leaf: its index plus the lane gaps before it (0.8 of a row each)
    const LANE_GAP = 0.8;
    const breaks = [];
    let nb = 0;
    leaves.forEach((id, i) => { if (i && laneOf.get(id) !== laneOf.get(leaves[i - 1])) nb++; breaks.push(nb); });
    const sat = new Set(satellites);
    const D = m.maxDepth + 1;
    const top = 44, bottom = 16, subGap = 22, STRIP = 46;
    const fontOf = n => n.orchestrator ? fonts.bold : fonts.label;
    const layerIds = [];
    for (let d = 0; d < D; d++) layerIds.push([]);
    for (const id of m.order) if (!sat.has(id)) layerIds[m.depth.get(id) || 0].push(id);
    const labelW = new Map();
    for (const id of m.order) labelW.set(id, measure(label(m.byId.get(id)), fontOf(m.byId.get(id))));
    const deepIds = new Set(leaves.filter(id => (m.depth.get(id) || 0) === m.maxDepth));
    // a column is at least as wide as its title (upper case, tracked: measured, then widened)
    const titleW = layerIds.map((ids, d) => {
      const rs = [];
      for (const id of ids) { const r = m.byId.get(id).rank || ''; if (!rs.includes(r)) rs.push(r); }
      const t = rs.map(r => m.word('rank.' + r, r)).join(' · ').toUpperCase() + ' ' + ids.length;
      return measure(t, fonts.caps || fonts.small) + t.length * (fonts.track || 0.1) * pxOf(fonts.caps || fonts.small) + 24;
    });
    let skillLabel = 0;
    for (const n of m.skills) skillLabel = Math.max(skillLabel, measure(n.name, fonts.small));
    const nCols = strip => D + (m.skills.length && !strip ? 1 : 0);
    // one candidate packing: the skills as a column or a strip, the deepest layer in k sub-columns
    function trial(strip, k) {
      const room = availH - top - bottom - (strip && m.skills.length ? STRIP : 0);
      const rows = Math.max(1, Math.ceil((leaves.length + nb * LANE_GAP * k) / k));
      const Gmin = Math.max(18, px * 1.35);
      const G = clamp(room / rows, Gmin, 42);
      const fitsV = room / rows >= Gmin && (strip || !m.skills.length || m.skills.length * 22 <= room);
      const T = clamp(Math.round(G - 7), 14, 26);
      const want = [], floor = [];
      for (let d = 0; d < D; d++) {
        let w = 0, f = 0;
        for (const id of layerIds[d]) {
          const n = m.byId.get(id), size = n.orchestrator ? TT : deepIds.has(id) ? T : 26, lw = labelW.get(id);
          w = Math.max(w, size + 8 + lw); f = Math.max(f, size + 8 + Math.min(lw, em6));
        }
        if (d === 0) for (const id of satellites) {
          const lw = labelW.get(id);
          w = Math.max(w, TT / 2 + 12 + 26 + 8 + lw); f = Math.max(f, TT / 2 + 12 + 26 + 8 + Math.min(lw, em6));
        }
        if (k > 1 && d === D - 1) { w = w * k + subGap * (k - 1); f = f * k + subGap * (k - 1); }
        w = Math.max(w, titleW[d]); f = Math.max(f, titleW[d]);
        want.push(w); floor.push(f);
      }
      if (m.skills.length && !strip) { want.push(TM + 34 + skillLabel); floor.push(TM + 34 + Math.min(skillLabel, em6)); }
      const minGap = k > 1 ? 24 : 36, n = nCols(strip);
      const extra = minGap * (n - 1) + 24;
      const total = want.reduce((a, b) => a + b, 0) + extra, least = floor.reduce((a, b) => a + b, 0) + extra;
      return { strip, k, room, rows, G, T, want, floor, minGap, total, least, fitsV, full: total <= W, legible: least <= W };
    }
    const tries = [];
    // the skills keep out of the way (a strip) unless only their column lets the layers fit
    for (const strip of m.skills.length ? [true, false] : [false]) for (let k = 1; k <= 4; k++) tries.push(trial(strip, k));
    let P = tries.find(t => t.fitsV && t.full) || tries.find(t => t.fitsV && t.legible);
    if (!P) {
      // nothing fits the box: the most legible packing that fits the width, grown to its rows
      const need = t => t.rows * t.G + (t.strip && m.skills.length ? STRIP : 0) + (t.strip || !m.skills.length ? 0 : Math.max(0, m.skills.length * 22 - t.rows * t.G));
      const ok = tries.filter(t => t.legible);
      P = ok.length ? ok.reduce((a, b) => need(b) < need(a) - 0.5 ? b : a) : tries[tries.length - 1];
    }
    const { k, G, T, strip } = P;
    const rows = P.rows, room = Math.max(P.room, rows * G);
    const stripH = strip && m.skills.length ? STRIP : 0;
    const H = Math.max(availH, top + room + bottom + stripH);
    // widths: each column its want; too wide, the widest give way, never below their floor
    const widths = P.want.slice();
    let total = P.total;
    for (let guard = 0; total > W && guard < 2000; guard++) {
      let i = -1, best = 0;
      widths.forEach((w, j) => { if (w - P.floor[j] > best) { best = w - P.floor[j]; i = j; } });
      if (i < 0) break;
      const cut = Math.min(4, widths[i] - P.floor[i], total - W);
      widths[i] -= cut; total -= cut;
    }
    const W2 = Math.max(W, Math.ceil(total));
    const n = widths.length;
    const gap = n > 1 ? Math.min(P.minGap + Math.max(0, W2 - total) / (n - 1), P.minGap * 3.5) : 0;
    const used = widths.reduce((a, b) => a + b, 0) + gap * (n - 1);
    let x = Math.max(12, (W2 - used) / 2);
    const colX = widths.map(w => { const at = x; x += w + gap; return at; });
    const subW = k > 1 ? (widths[D - 1] - subGap * (k - 1)) / k : widths[D - 1];
    const bandH = H - 12 - stripH;
    const mid = top + room / 2;
    // rows: the leaves fill the room, centred; sub-column j sits j/k of a row lower so a bond to
    // it passes between the labels
    const ys = new Map();
    const offY = (room - rows * G) / 2;
    leaves.forEach((id, i) => ys.set(id, top + offY + G / 2 + (i + breaks[i] * LANE_GAP * k) * G / k));
    const stepU = TT + 6;
    const yOf = id => {
      if (ys.has(id)) return ys.get(id);
      const sh = shallow.get(id);
      let y;
      if (sh) y = yOf(sh.anchor) + sh.n * stepU;
      else { const ks = m.kids.get(id) || []; y = ks.length ? ks.reduce((a, c) => a + yOf(c), 0) / ks.length : mid; }
      ys.set(id, y);
      return y;
    };
    for (let d = 0; d < D; d++) {
      const ids = layerIds[d].slice().sort((a, b) => yOf(a) - yOf(b) || slot.get(a) - slot.get(b));
      const ranks = [];
      for (const id of ids) { const r = m.byId.get(id).rank || ''; if (!ranks.includes(r)) ranks.push(r); }
      const title = ranks.map(r => m.word('rank.' + r, r)).join(' · ');
      cols.push({ kind: 'rank', x: colX[d] - 6, y: 8, w: widths[d] + 12, h: bandH, title, count: ids.length, rank: ranks[0] || '', ranks });
      const wrapped = k > 1 && d === D - 1;
      // an upper layer's tiles shrink when the layer is crowded; the layer is centred in its column
      const crowd = wrapped ? T : clamp(Math.floor(room / Math.max(1, ids.length)) - 8, 14, 26);
      const sizeOf = id => m.byId.get(id).orchestrator ? TT : (deepIds.has(id) ? T : Math.min(26, Math.max(T, crowd)));
      const place2 = [];
      let last = -Infinity;
      for (const id of ids) {
        const size = sizeOf(id);
        let y = yOf(id);
        if (!(wrapped && deepIds.has(id))) { if (y - last < size + 6) y = last + size + 6; last = y; }
        place2.push({ id, y, size });
      }
      if (d < D - 1 && place2.length) {
        const lo = place2[0].y - place2[0].size / 2, hi = place2[place2.length - 1].y + place2[place2.length - 1].size / 2;
        let shift = mid - (lo + hi) / 2;
        shift = clamp(shift, top - lo, top + room - hi);
        if (hi - lo > room) shift = top - lo;
        for (const q of place2) q.y += shift;
      }
      for (const q of place2) {
        const nn = m.byId.get(q.id);
        let x0 = colX[d] + q.size / 2, roomL = widths[d] - q.size - 8;
        if (wrapped && deepIds.has(q.id)) {
          x0 = colX[d] + (leaves.indexOf(q.id) % k) * (subW + subGap) + q.size / 2;
          roomL = subW - q.size - 8;
        }
        pos.set(q.id, { x: x0, y: q.y, size: q.size, label: fit(label(nn), fontOf(nn), roomL), lx: x0 + q.size / 2 + 8, d });
      }
      if (d === 0 && satellites.length && pos.has(orchestratorId)) {
        // the orchestrator's satellites: stacked just under it (over it when the column runs out)
        // the orchestrator and its satellites are one block, centred in the column
        const th = pos.get(orchestratorId), S = 26, step = S + 10, x0 = colX[0] + TT / 2 + 12 + S / 2;
        const ext = TT / 2 + 12 + satellites.length * step - 10;
        th.y = clamp(mid - (ext - TT / 2) / 2, top + TT / 2, Math.max(top + TT / 2, top + room - ext));
        satellites.forEach((id, i) => {
          const nn = m.byId.get(id);
          pos.set(id, { x: x0, y: th.y + TT / 2 + 12 + S / 2 + i * step, size: S, label: fit(label(nn), fonts.label, widths[0] - (x0 - colX[0]) - S / 2 - 8), lx: x0 + S / 2 + 8, d: 1, sat: true, below: true });
        });
      }
    }
    // lanes: one band per subtree under the orchestrator, from its head's column to the deepest
    const lanes = [];
    if (D > 2) for (const head of (m.kids.get(orchestratorId) || [])) {
      if (sat.has(head) || !pos.has(head)) continue;
      const ids = [head, ...lineage(m, head).down].filter(id => pos.has(id));
      let lo = Infinity, hi = -Infinity;
      for (const id of ids) { const q = pos.get(id); lo = Math.min(lo, q.y - q.size / 2 - 7); hi = Math.max(hi, q.y + q.size / 2 + 7); }
      const x0 = colX[1] - 10, x1 = colX[D - 1] + widths[D - 1] + 4;
      lanes.push({ x: x0, y: lo, w: x1 - x0, h: hi - lo, i: lanes.length, head });
    }
    if (m.skills.length && !strip) {
      const cx = colX[D];
      cols.push({ kind: 'skills', x: cx - 6, y: 8, w: widths[D] + 12, h: bandH, title: m.word('skills', 'skills'), count: m.skills.length, rank: 'skill:shared' });
      const list = skillOrder(m, pos);
      const MG = clamp(room / list.length, 22, 40);
      list.forEach((nd, i) => {
        const y = top + (i + 0.5) * MG + (room - MG * list.length) / 2;
        pos.set(nd.id, { x: cx + TM / 2, y, size: TM, label: fit(nd.name, fonts.small, widths[D] - TM - 34), lx: cx + TM + 8, d: -1 });
      });
    } else if (m.skills.length) {
      // the skills as a compact strip under the layers: a brain per skill, its name when there is
      // room for six ems of it, else on hover or focus
      const sy = H - stripH, title = m.word('skills', 'skills');
      const tt = title.toUpperCase() + ' ' + m.skills.length;
      const tw = measure(tt, fonts.caps || fonts.small) + tt.length * (fonts.track || 0.1) * pxOf(fonts.caps || fonts.small) + 28;
      cols.push({ kind: 'skills', strip: true, x: 6, y: sy, w: W2 - 12, h: stripH - 6, title, count: m.skills.length, rank: 'skill:shared' });
      const list = [...m.skills].sort((a, b) => {
        const at = nd => { const xs = m.holds.filter(w => w.to === nd.id && pos.has(w.from)).map(w => pos.get(w.from).x); return xs.length ? xs.reduce((s, v) => s + v, 0) / xs.length : Infinity; };
        return at(a) - at(b) || a.name.localeCompare(b.name);
      });
      const x0 = 12 + tw, cell = (W2 - 12 - x0) / list.length, room2 = cell - TM - 14;
      const named = room2 >= em6 * 0.8;
      list.forEach((nd, i) => {
        const cx = x0 + i * cell + TM / 2;
        pos.set(nd.id, { x: cx, y: sy + (stripH - 6) / 2, size: TM, label: named ? fit(nd.name, fonts.small, room2) : nd.name, lx: cx + TM / 2 + 6, d: -1, strip: true, hide: !named });
      });
    }
    // where a neuron's bonds leave it: past its label, so the label rides the axon
    for (const [id, p] of pos) {
      const nn = m.byId.get(id);
      p.out = p.hide ? p.x + p.size / 2 : p.lx + measure(p.label, nn.orchestrator ? fonts.bold : (nn.kind === 'skill' ? fonts.small : fonts.label)) + 6;
    }
    return { pos, cols, lanes, W: W2, H, narrow, k, strip: !!stripH, G, T };
  }
  // skills in the order of their holders (the barycentre), so the threads cross as little as they can
  function skillOrder(m, pos) {
    const at = n => {
      const ys = m.holds.filter(w => w.to === n.id && pos.has(w.from)).map(w => pos.get(w.from).y);
      return ys.length ? ys.reduce((a, b) => a + b, 0) / ys.length : Infinity;
    };
    return [...m.skills].map(n => [n, at(n)]).sort((a, b) => a[1] - b[1] || a[0].name.localeCompare(b[0].name)).map(x => x[0]);
  }

  // ---------- the view ----------
  function create(host, opts) {
    const word = opts.word || ((k, d) => d || k);
    const st = {
      m: null, L: null, focus: '', hover: '', live: new Set(), mode: 'auto', visible: true, three: null,
      pulses: [], raf: 0, edgeEls: new Map(), nodeEls: new Map(), current: '', sparks: null, fonts: null, busy: new Map(),
    };
    host.classList.add('net');
    const stage = document.createElement('div');
    stage.className = 'net-stage';
    stage.dataset.mode = '2d';
    host.replaceChildren(stage);
    // probes: the canvas reads the theme's colours through these (never a literal)
    const probes = document.createElement('div');
    probes.setAttribute('aria-hidden', 'true');
    host.append(probes);
    const hint = document.createElement('p');
    hint.className = 'visually-hidden';
    hint.id = 'net-keys';
    hint.textContent = 'Arrow keys walk the net: left to the parent, right to the first child, up and down within a layer. Enter pins a neuron, Escape clears.';
    host.append(hint);
    // the 3D stage has no element per neuron: the neuron the keys reach is said here
    const say = document.createElement('p');
    say.className = 'visually-hidden';
    say.setAttribute('aria-live', 'polite');
    host.append(say);

    // the label fonts as the css resolves them (the svg text and the measure must agree); read
    // once per draw — the root size moves with the viewport, so a redraw re-reads them
    const fonts = () => {
      const probe = document.createElement('span');
      probe.className = 'net-probe';
      host.append(probe);
      const at = size => { probe.style.font = size + ' var(--font-body)'; const cs = getComputedStyle(probe); return cs.fontSize + ' ' + cs.fontFamily; };
      const out = { label: at('var(--step--1)'), small: at('var(--step--2)'), bold: '650 ' + at('var(--step--1)') };
      // the column titles: tracked capitals in the heading face
      probe.style.font = 'var(--weight-bold) var(--step--2) var(--font-heading)';
      const cs = getComputedStyle(probe);
      out.caps = cs.fontWeight + ' ' + cs.fontSize + ' ' + cs.fontFamily;
      out.track = parseFloat(getComputedStyle(probe).getPropertyValue('--tracking-label')) || 0.1;
      probe.remove();
      return out;
    };
    function draw() {
      if (!st.m || !host.isConnected || host.offsetParent === null) return;
      // in the cockpit the net fills exactly the box it is given (opts.fit); elsewhere it takes
      // the height it wants
      const fitH = opts.fit && opts.fit();
      if (fitH) stage.style.setProperty('--net-h', Math.max(160, fitH) + 'px'); else stage.style.removeProperty('--net-h');
      stage.toggleAttribute('data-fit', !!fitH);
      if (st.mode === '3d' && st.three) { stage.dataset.mode = '3d'; st.three.resize(); return; }
      stage.dataset.mode = '2d';
      const W = Math.max(300, stage.clientWidth || host.clientWidth);
      const availH = fitH ? Math.max(160, fitH) : Math.max(560, innerHeight - 190);
      const f = fonts();
      const L = layout2d(st.m, W, availH, f);
      st.L = L;
      const m = st.m;
      // a tile keeps its slot and shrinks by its mass (70%–100% of the slot), so the layout never
      // moves and the bonds still meet the tile's edge
      if (m.sized) for (const [id, p] of L.pos) { const n = m.byId.get(id); if (n && !n.orchestrator) p.size *= 0.7 + 0.3 * (m.massK.get(id) || 0); }
      const g = s('svg', { viewBox: `0 0 ${L.W} ${L.H}`, style: L.W > W ? `inline-size:${L.W}px` : false, role: 'group', 'aria-label': 'the ' + word('workspace', 'workspace') + ' as a neural net — ' + m.members.length + ' ' + word('members', 'members') + ', ' + m.skills.length + ' ' + word('skills', 'skills'), 'aria-describedby': 'net-keys' });
      const defs = s('defs');
      g.append(defs);
      for (const c of L.cols) {
        if (c.kind === 'skills' && L.narrow) {
          g.append(s('g', { 'data-rank': c.rank }, s('text', { class: 'net-col-label', x: 12, y: c.y + 14 }, c.title + ' ', s('tspan', { class: 'net-col-count' }, String(c.count)))));
          continue;
        }
        if (L.narrow) continue;
        if (c.strip) {
          g.append(s('g', { 'data-rank': c.rank },
            s('rect', { class: 'net-col', x: c.x, y: c.y, width: c.w, height: c.h, rx: 12 }),
            s('text', { class: 'net-col-label', x: c.x + 10, y: c.y + c.h / 2, 'dominant-baseline': 'central' }, c.title + ' ', s('tspan', { class: 'net-col-count' }, String(c.count)))));
          continue;
        }
        g.append(s('g', { 'data-rank': c.rank },
          s('rect', { class: 'net-col', x: c.x, y: c.y, width: c.w, height: c.h, rx: 12 }),
          s('text', { class: 'net-col-label', x: c.x + 10, y: c.y + 20 }, c.title + ' ', s('tspan', { class: 'net-col-count' }, String(c.count)))));
        // the column's head is its rank's plate: one press lights every member of the rank
        if (c.ranks && c.ranks.length && !c.ranks.includes('orchestrator')) {
          const key = c.ranks.join('|');
          const plate = s('g', { class: 'net-col-plate', 'data-rank': c.rank, role: 'button', tabindex: '0', 'data-race': key, 'aria-pressed': String(st.rank === key), 'aria-label': 'light every ' + c.title.toLowerCase() + ' (' + c.count + ')' },
            s('rect', { x: c.x, y: c.y, width: c.w, height: 32, rx: 12 }));
          plate.addEventListener('click', () => rank(c.ranks));
          plate.addEventListener('keydown', ev => { if (ev.key === 'Enter' || ev.key === ' ') { ev.preventDefault(); rank(c.ranks); } });
          g.append(plate);
        }
      }
      for (const ln of L.lanes || []) g.append(s('rect', { class: 'net-lane', 'data-lane': ln.i % 2 ? 'b' : 'a', x: ln.x, y: ln.y, width: ln.w, height: ln.h, rx: 10 }));
      const edges = s('g', { 'data-layer': 'edges' }), sparks = s('g', { 'data-layer': 'sparks' }), nodes = s('g', { 'data-layer': 'nodes' });
      g.append(edges, nodes, sparks);
      st.edgeEls = new Map(); st.nodeEls = new Map();
      // bonds
      const maxW = Math.max(1, ...m.tree.map(t => t.weight));
      const curve = (A, B) => {
        if (L.narrow) {
          const x0 = A.x, y0 = A.y + A.size / 2, x1 = B.x - B.size / 2, y1 = B.y, r = Math.min(8, Math.abs(y1 - y0) / 2, Math.abs(x1 - x0));
          return `M${x0} ${y0} V${y1 - r} Q${x0} ${y1} ${x0 + r} ${y1} H${x1}`;
        }
        if (B.sat) {
          // the orchestrator to a satellite: down out of the orchestrator, then across
          const x0 = A.x, y0 = B.below ? A.y + A.size / 2 : A.y - A.size / 2, x1 = B.x - B.size / 2 - 2, y1 = B.y, r = Math.min(8, Math.abs(y1 - y0) / 2), sg = B.below ? 1 : -1;
          return `M${x0.toFixed(1)} ${y0.toFixed(1)} V${(y1 - sg * r).toFixed(1)} Q${x0.toFixed(1)} ${y1.toFixed(1)} ${(x0 + r).toFixed(1)} ${y1.toFixed(1)} H${x1.toFixed(1)}`;
        }
        if (B.strip) {
          // to a skill in the strip below: leave sideways, fall into the brain from above
          const x0 = A.out, y1 = B.y - B.size / 2 - 2, k = Math.max(24, (y1 - A.y) * 0.5);
          return `M${x0.toFixed(1)} ${A.y.toFixed(1)} C${(x0 + 30).toFixed(1)} ${A.y.toFixed(1)} ${B.x.toFixed(1)} ${(y1 - k).toFixed(1)} ${B.x.toFixed(1)} ${y1.toFixed(1)}`;
        }
        const x0 = A.out, x1 = B.x - B.size / 2 - 2, k = Math.max(24, (x1 - x0) * 0.5);
        return `M${x0.toFixed(1)} ${A.y.toFixed(1)} C${(x0 + k).toFixed(1)} ${A.y.toFixed(1)} ${(x1 - k).toFixed(1)} ${B.y.toFixed(1)} ${x1.toFixed(1)} ${B.y.toFixed(1)}`;
      };
      if (!L.narrow) for (const w of m.holds) {
        const A = L.pos.get(w.from), B = L.pos.get(w.to);
        if (!A || !B) continue;
        const p = s('path', { class: 'net-edge', 'data-bond': 'holds', d: curve(A, B) });
        edges.append(p);
        st.edgeEls.set('w:' + w.from + '>' + w.to, p);
      }
      for (const t of m.tree) {
        const A = L.pos.get(t.up), B = L.pos.get(t.down);
        if (!A || !B) continue;
        const k = t.weight ? Math.log2(1 + t.weight) / Math.log2(1 + maxW) : 0;
        const p = s('path', { class: 'net-edge', 'data-bond': t.bond, 'data-implied': t.implied || false, d: curve(A, B),
          style: `stroke-width:${(1.4 + 3.2 * k).toFixed(2)};--net-edge-o:${(0.5 + 0.45 * k).toFixed(2)}` });
        p.append(s('title', {}, nameOf(t.up) + ' → ' + nameOf(t.down) + ' · ' + word('bond.' + t.bond, t.bond) + (t.implied ? ' (implied)' : '') + (t.weight ? ' · ' + t.weight + ' commission' + (t.weight === 1 ? '' : 's') + ' in 4 weeks' : '')));
        edges.append(p);
        st.edgeEls.set('t:' + t.up + '>' + t.down, p);
      }
      // neurons
      let k = 0;
      const cellsOf = id => m.cells.get(id);
      for (const id of [...m.order, ...m.skills.map(n => n.id)]) {
        const p = L.pos.get(id);
        if (!p) continue;
        const n = m.byId.get(id), skill = n.kind === 'skill';
        const S = p.size, x0 = p.x - S / 2, y0 = p.y - S / 2;
        const a = m.act[n.name];
        const rank = skill ? n.lane : (n.rank || 'plain');
        const node = s('g', { class: 'net-node', 'data-id': id, 'data-rank': rank, 'data-family': n.family || false, 'data-kind': skill ? 'skill' : 'agent', 'data-orchestrator': n.orchestrator || false,
          'data-stress': n.workingNotes && n.workingNotes.stress && n.workingNotes.stress !== 'ok' ? n.workingNotes.stress : false, 'data-compact': p.hide || false, tabindex: -1, role: 'button', 'aria-label': aria(n, a) });
        node.append(s('rect', { class: 'net-halo', x: x0 - 6, y: y0 - 6, width: S + 12, height: S + 12, rx: S * 0.3 + 4 }));
        node.append(s('rect', { class: 'net-tile', x: x0, y: y0, width: S, height: S, rx: S * 0.22 }));
        // the value band ring: the rank's own meter, banded by config
        if (!skill && n.value && n.value.band && n.value.band !== 'none') node.append(s('rect', { class: 'net-band', 'data-band': n.value.band, 'data-confidence': n.value.confidence, x: x0 - 3.5, y: y0 - 3.5, width: S + 7, height: S + 7, rx: S * 0.22 + 3 }));
        if (skill) {
          const b = S * 0.72;
          node.append(s('use', { class: 'net-brain', href: SPRITE + '#skill', x: (p.x - b / 2).toFixed(1), y: (p.y - b / 2).toFixed(1), width: b.toFixed(1), height: b.toFixed(1) }));
        } else {
          // the activity tile: 4 weeks × 7 days, oldest top-left; blank when no instrument saw it
          const pad = S * 0.14, cw = (S - 2 * pad) / 7, ch = (S - 2 * pad) / 4;
          const v = cellsOf(id) || [];
          const days = v.slice(-28), off = 28 - days.length;
          for (let j = 0; j < 28; j++) {
            const val = j >= off ? days[j - off] : 0;
            const o = val && m.peak > 0 ? 0.3 + 0.7 * Math.log1p(val) / Math.log1p(m.peak) : 0.13;
            node.append(s('rect', { class: 'net-cell', 'data-on': val ? true : false, x: (x0 + pad + (j % 7) * cw + 0.35).toFixed(2), y: (y0 + pad + Math.floor(j / 7) * ch + 0.35).toFixed(2), width: (cw - 0.7).toFixed(2), height: (ch - 0.7).toFixed(2), rx: 0.6, opacity: o.toFixed(2) }));
          }
          // the rank's icon as a badge on the tile's corner: its portrait, else its family's glyph
          const icon = n.icon || null;
          if (icon) {
            // every depth holds its face, scaled to the neuron (never below legible)
            const r = n.orchestrator ? 11 : clamp(S * 0.34, 6, 10), cx = x0 + 1, cy = y0 + 1;
            node.append(s('circle', { class: 'net-ring', cx, cy, r }));
            if (icon.kind === 'portrait') {
              const cid = 'netp-' + (k++);
              defs.append(s('clipPath', { id: cid }, s('circle', { cx, cy, r: r - 1.2 })));
              const img = s('image', { class: 'net-portrait', href: icon.url, x: cx - r, y: cy - r, width: r * 2, height: r * 2, preserveAspectRatio: 'xMidYMid slice', 'clip-path': `url(#${cid})` });
              img.addEventListener('error', () => img.remove());
              node.append(img);
            } else {
              const g2 = r * 1.2;
              node.append(s('use', { class: 'net-glyph', href: icon.url, x: cx - g2 / 2, y: cy - g2 / 2, width: g2, height: g2 }));
            }
          }
        }
        if (p.hide) {
          // a strip skill's name sits above its brain, kept inside the view, shown on hover or focus
          const w = measure(p.label, f.small) / 2 + 4;
          node.append(s('text', { class: 'net-label', x: clamp(p.x, w, L.W - w), y: p.y - S / 2 - 10, 'text-anchor': 'middle' }, p.label));
        } else node.append(s('text', { class: 'net-label', x: p.lx, y: p.y }, p.label));
        if (n.workingNotes && skill && n.workingNotes.notes && !p.hide) node.append(s('text', { class: 'net-sub', x: p.out + 2, y: p.y }, n.workingNotes.notes + '/' + n.workingNotes.limit));
        node.append(s('title', {}, n.name));
        node.addEventListener('click', () => pin(id));
        node.addEventListener('dblclick', ev => { ev.preventDefault(); openCardAt(id); });
        node.addEventListener('pointerenter', () => preview(id));
        node.addEventListener('pointerleave', () => preview(''));
        node.addEventListener('focus', () => { st.current = id; preview(id); });
        node.addEventListener('blur', () => preview(''));
        node.addEventListener('keydown', ev => keys(ev, id));
        nodes.append(node);
        st.nodeEls.set(id, node);
      }
      // one tab stop: the current neuron (else the orchestrator)
      const first = st.nodeEls.get(st.current) || st.nodeEls.get(m.orchestrator && m.orchestrator.id) || nodes.firstElementChild;
      if (first) first.setAttribute('tabindex', '0');
      st.sparks = sparks;
      stage.replaceChildren(g);
      applyLive(); applyFocus();
    }
    const nameOf = id => { const n = st.m && st.m.byId.get(id); return n ? (n.orchestrator ? n.label || n.name : n.name) : id; };
    function aria(n, a) {
      const m = st.m;
      const bits = [n.orchestrator ? (n.label || n.name) : n.name];
      if (n.kind === 'skill') bits.push(word('skill', 'skill'));
      else {
        bits.push(word('rank.' + n.rank, n.rank || 'agent'));
        const p = m.parent.get(n.id);
        if (p) bits.push('answers to ' + nameOf(p));
        const kids = (m.kids.get(n.id) || []).length;
        if (kids) bits.push(kids + ' below');
      }
      bits.push(a && a.total ? a.total + ' run' + (a.total === 1 ? '' : 's') + ' in 4 weeks' : 'no journaled work in 4 weeks');
      if (n.value && n.value.meter) bits.push('score ' + Math.round(n.value.score) + (n.value.confidence === 'low' ? ' (below its record)' : '') + ', ' + n.value.meter.label + ' ' + n.value.meter.text + ' (' + n.value.band + ')');
      if (n.mass) bits.push(n.kind === 'skill' ? n.mass + ' holding it' : n.mass.toLocaleString() + ' lines of code owned');
      return bits.join(', ');
    }

    // ---------- focus: hover previews, click or Enter pins ----------
    function pin(id) { st.focus = st.focus === id ? '' : id; if (id) { st.current = id; st.rank = ''; } applyFocus(); opts.onFocus && opts.onFocus(st.focus); }
    // a rank's plate (a 2D column's head, a 3D key row) lights the rank while nothing is focused;
    // pressed again it lets go
    function rank(ranks) {
      const key = (ranks || []).join('|');
      st.rank = st.rank === key ? '' : key;
      if (st.rank && st.focus) { st.focus = ''; opts.onFocus && opts.onFocus(''); }
      applyFocus();
    }
    // a double-click selects the neuron (whatever its two clicks toggled) and opens its card
    function openCardAt(id) {
      if (!id) return;
      st.focus = id; st.current = id; applyFocus();
      opts.onFocus && opts.onFocus(st.focus);
      opts.onCard && opts.onCard(st.focus);
    }
    function preview(id) { st.hover = id; applyFocus(); }
    function applyFocus() {
      const id = st.hover || st.focus;
      const raced = !id && st.rank && st.m ? st.rank.split('|') : null;
      host.toggleAttribute('data-focus', !!id || !!raced);
      const lit = id && st.m ? litSet(st.m, id) : raced ? rankSet(st.m, raced) : null;
      for (const el of host.querySelectorAll('.net-col-plate')) el.setAttribute('aria-pressed', String(el.dataset.rank === st.rank));
      if (st.three) st.three.raced(st.rank);
      for (const [nid, el] of st.nodeEls) {
        el.toggleAttribute('data-near', !!lit && lit.nodes.has(nid));
        el.toggleAttribute('data-self', nid === id);
        el.setAttribute('aria-pressed', String(nid === st.focus));
      }
      for (const [k, el] of st.edgeEls) {
        const key = k.slice(2);
        el.toggleAttribute('data-lit', !!lit && (k[0] === 't' ? lit.tree.has(key) : lit.holds.has(key)));
      }
      if (st.three) st.three.focus(id, lit, st.focus);
    }
    function keys(ev, id) {
      const m = st.m, L = st.L;
      if (!m || !L) return;
      let to = '';
      const p = L.pos.get(id);
      const peers = [...L.pos.keys()].filter(x => L.pos.get(x).d === p.d).sort((a, b) => L.pos.get(a).y - L.pos.get(b).y || L.pos.get(a).x - L.pos.get(b).x);
      const i = peers.indexOf(id);
      switch (ev.key) {
        case 'ArrowLeft': to = m.parent.get(id) || ''; break;
        case 'ArrowRight': to = (m.kids.get(id) || []).slice().sort((a, b) => L.pos.get(a).y - L.pos.get(b).y)[0] || (m.holds.find(w => w.from === id) || {}).to || ''; break;
        case 'ArrowUp': to = peers[i - 1] || ''; break;
        case 'ArrowDown': to = peers[i + 1] || ''; break;
        case 'Home': to = m.orchestrator ? m.orchestrator.id : ''; break;
        case 'Enter': case ' ': ev.preventDefault(); pin(id); return;
        case 'Escape': if (st.focus) { ev.preventDefault(); ev.stopPropagation(); pin(st.focus); } return;
        default: return;
      }
      ev.preventDefault();
      if (!to || !st.nodeEls.has(to)) return;
      st.nodeEls.get(id).setAttribute('tabindex', '-1');
      const el = st.nodeEls.get(to);
      el.setAttribute('tabindex', '0');
      el.focus();
    }

    // the same walk in 3D, over the model (preorder is the layers' top-to-bottom order): arrows
    // move a cursor the camera follows, Enter pins it, Escape clears; Shift+arrows still turn
    function keys3d(ev) {
      const m = st.m;
      if (!m || ev.shiftKey || ev.altKey || ev.ctrlKey || ev.metaKey) return false;
      const id = st.current && m.byId.has(st.current) ? st.current : (m.orchestrator ? m.orchestrator.id : '');
      if (!id) return false;
      const skill = m.byId.get(id).kind === 'skill';
      const peers = skill ? m.skills.map(n => n.id) : m.order.filter(x => (m.depth.get(x) || 0) === (m.depth.get(id) || 0));
      const i = peers.indexOf(id);
      let to = '';
      switch (ev.key) {
        case 'ArrowLeft': to = skill ? (m.holds.find(w => w.to === id) || {}).from || '' : m.parent.get(id) || ''; break;
        case 'ArrowRight': to = (m.kids.get(id) || [])[0] || (m.holds.find(w => w.from === id) || {}).to || ''; break;
        case 'ArrowUp': to = peers[i - 1] || ''; break;
        case 'ArrowDown': to = peers[i + 1] || ''; break;
        case 'Home': to = m.orchestrator ? m.orchestrator.id : ''; break;
        case 'Enter': case ' ': pin(id); say.textContent = (st.focus ? 'pinned ' : 'unpinned ') + nameOf(id); return true;
        case 'Escape': if (!st.focus) return false; pin(st.focus); return true;
        default: return false;
      }
      if (to && m.byId.has(to)) {
        st.current = to;
        preview(to);
        if (st.three) st.three.follow(to);
        say.textContent = aria(m.byId.get(to), m.act[m.byId.get(to).name]);
      }
      return true;
    }

    // ---------- live: running agents glow, their bond to the dispatcher flows ----------
    function applyLive() {
      const m = st.m;
      if (!m) return;
      const liveIds = new Set();
      for (const [id, n] of m.byId) if (st.live.has(n.name)) liveIds.add(id);
      for (const [id, el] of st.nodeEls) el.toggleAttribute('data-live', liveIds.has(id));
      for (const [k, el] of st.edgeEls) {
        if (k[0] !== 't') continue;
        const [up, down] = k.slice(2).split('>');
        el.toggleAttribute('data-live', liveIds.has(down) && (liveIds.has(up) || up === (m.orchestrator && m.orchestrator.id)));
      }
      if (st.three) st.three.live(liveIds);
    }

    // ---------- signals: a pulse down on a dispatch, back up on a report ----------
    function signal(kind, name, tone) {
      const m = st.m;
      if (!m) return;
      let id = '';
      for (const [nid, n] of m.byId) if (n.name === name && n.kind !== 'skill') id = nid;
      if (!id || (m.orchestrator && id === m.orchestrator.id)) return;
      const hops = route(m, id, st.live);
      if (st.three) st.three.signal(kind, hops, id, tone);
      if (st.mode === '3d') return;
      if (reducedQ.matches || !st.visible || !hops.length) { ping(kind === 'up' ? hops.length ? hops[0].split('>')[0] : id : id, kind === 'up' ? tone : 'down'); return; }
      const els = hops.map(h => st.edgeEls.get('t:' + h)).filter(Boolean);
      if (!els.length) { ping(id, kind === 'up' ? tone : 'down'); return; }
      if (kind === 'up') els.reverse();
      const t = kind === 'up' ? (tone || 'ok') : '';
      const glow = s('circle', { class: 'net-spark', 'data-glow': true, 'data-tone': t || false, r: 9 });
      const trail = [0, 1, 2].map(i => s('circle', { class: 'net-spark', 'data-tone': t || false, r: 3.6 - i * 0.9, opacity: (1 - i * 0.3).toFixed(2) }));
      st.sparks.append(glow, ...trail);
      st.pulses.push({ els, dir: kind, at: performance.now(), per: 620, glow, trail, end: kind === 'up' ? hops[0].split('>')[0] : id, tone: kind === 'up' ? (tone || 'ok') : 'down', lens: els.map(e => e.getTotalLength()) });
      tick();
    }
    function ping(id, tone, kind) {
      const el = st.nodeEls.get(id);
      if (!el) return;
      el.setAttribute('data-ping', tone || 'down');
      clearTimeout(el._ping);
      el._ping = setTimeout(() => el.removeAttribute('data-ping'), kind === 'read' ? 700 : kind === 'evolve' ? 2600 : 1400);
      if (!reducedQ.matches && st.L) {
        const p = st.L.pos.get(id);
        if (!p) return;
        const r = s('circle', { class: 'net-ripple', 'data-tone': tone === 'down' ? false : tone, 'data-kind': kind || false, cx: p.x, cy: p.y, r: p.size * 0.7 });
        el.append(r);
        setTimeout(() => r.remove(), kind === 'evolve' ? 2600 : 1300);
      }
    }
    // an action on a neuron: a tool step, a turn's end, a gate verdict — each class reads its own
    // way, calmly (read: a soft accent ripple · write and a plain turn's end: the rank's own pulse ·
    // outward/destructive and a waiting gate: an amber ring · a passing verdict: a green flare · a
    // failing one or a denial: a red one). Bursts coalesce: within a burst (acts less than 1.5 s
    // apart) a neuron ripples only when something louder than the burst so far arrives; otherwise
    // it just breathes until the burst ends — a 20-step Subagent is one breathing neuron.
    // an evolution (policy.evolution) is the loudest: a wide, slow ring in its arm's hue (the caller's tone)
    const LOUD = { read: 0, write: 1, done: 1, gate: 2, outward: 2, destructive: 3, pass: 3, fail: 4, evolve: 5 };
    const TONE = { read: 'info', write: 'rank', done: 'rank', gate: 'warn', outward: 'warn', destructive: 'warn', pass: 'ok', fail: 'danger', evolve: 's6' };
    const BURST = 1500;
    function act(name, kind, tone) {
      const m = st.m;
      if (!m || !(kind in LOUD)) return;
      let id = m.orchestrator ? m.orchestrator.id : '';
      if (name) for (const [nid, n] of m.byId) if (n.name === name && n.kind !== 'skill') { id = nid; break; }
      if (!id) return;
      const now = performance.now(), was = st.busy.get(id);
      const burst = was && now - was.at < BURST;
      const loud = LOUD[kind];
      clearTimeout(was && was.timer);
      const timer = setTimeout(() => { st.busy.delete(id); const el = st.nodeEls.get(id); if (el) el.removeAttribute('data-busy'); if (st.three) st.three.busy(id, false); }, BURST);
      st.busy.set(id, { at: now, loud: burst ? Math.max(was.loud, loud) : loud, timer });
      if (burst) {
        const el = st.nodeEls.get(id);
        if (el) el.setAttribute('data-busy', '');
        if (st.three) st.three.busy(id, true);
        if (loud <= was.loud) return;
      }
      const t = (kind === 'evolve' && tone) || TONE[kind];
      if (st.three) st.three.act(id, t, kind);
      if (st.mode !== '3d' && st.visible) ping(id, t, kind);
    }
    function tick() {
      if (st.raf || !st.pulses.length || !st.visible) return;
      st.raf = requestAnimationFrame(frame);
    }
    function frame(now) {
      st.raf = 0;
      st.pulses = st.pulses.filter(p => {
        const total = p.per * p.els.length;
        const t = Math.min(1, (now - p.at) / total);
        const hop = Math.min(p.els.length - 1, Math.floor(t * p.els.length));
        const local = t * p.els.length - hop;
        const place = (c, lag) => {
          let lt = local - lag, h = hop;
          if (lt < 0) { h--; lt += 1; }
          if (h < 0) { c.setAttribute('opacity', '0'); return; }
          const len = p.lens[h], u = p.dir === 'up' ? 1 - lt : lt;
          const e = ease(u), pt = p.els[h].getPointAtLength(len * (p.dir === 'up' ? 1 - ease(1 - u) : e));
          c.setAttribute('cx', pt.x.toFixed(1)); c.setAttribute('cy', pt.y.toFixed(1));
        };
        place(p.glow, 0);
        p.trail.forEach((c, i) => place(c, i * 0.05));
        if (t >= 1) { p.glow.remove(); p.trail.forEach(c => c.remove()); ping(p.end, p.tone); return false; }
        return true;
      });
      if (st.pulses.length) tick();
    }
    const ease = u => u < 0.5 ? 2 * u * u : 1 - Math.pow(-2 * u + 2, 2) / 2;

    // ---------- calm: animate only while seen ----------
    const io = new IntersectionObserver(es => { for (const e of es) setVisible(e.isIntersecting && !document.hidden); });
    io.observe(stage);
    document.addEventListener('visibilitychange', () => setVisible(!document.hidden && stage.offsetParent !== null));
    function setVisible(v) {
      st.visible = v;
      host.toggleAttribute('data-asleep', !v);
      if (st.three) st.three.running(v && st.mode === '3d');
      if (v) tick();
    }

    // ---------- the 3D stage ----------
    const can3d = () => !!(window.agentOneNet3D && window.agentOneNet3D.supported());
    function mode(v) {
      if (v === '3d' && !can3d()) v = '2d';
      st.mode = v;
      if (v === '3d') {
        if (!st.three) {
          st.three = window.agentOneNet3D.create(stage, { probes, word, meters: () => (opts.meters ? opts.meters() : {}), onPick: id => pin(id || st.focus), onCard: id => openCardAt(id), onRank: rank => rank([rank]), onHover: id => preview(id), onKey: keys3d, onBlur: () => preview(''), reduced: () => reducedQ.matches });
          if (!st.three) { st.mode = '2d'; return mode('2d'); }
        }
        stage.dataset.mode = '3d';
        stage.replaceChildren(...st.three.elements);
        if (st.m) st.three.model(st.m);
        st.edgeEls = new Map(); st.nodeEls = new Map();
        applyLive(); applyFocus();
        for (const id of st.busy.keys()) st.three.busy(id, true);
        st.three.running(st.visible);
      } else {
        if (st.three) { st.three.running(false); st.three.dispose(); st.three = null; }
        stage.dataset.mode = '2d';
        draw();
      }
      return st.mode;
    }

    let resizeT = 0;
    new ResizeObserver(() => { clearTimeout(resizeT); resizeT = setTimeout(draw, 100); }).observe(host);
    return {
      set(team, activity) {
        st.m = build(team, activity, word);
        if (st.focus && !st.m.byId.has(st.focus)) st.focus = '';
        if (st.three) st.three.model(st.m);
        draw();
        return st.m;
      },
      live(names) { st.live = new Set(names); applyLive(); },
      signal,
      act,
      hover(id) { preview(id || ''); },
      focus(id) { st.focus = id || ''; applyFocus(); },
      mode,
      can3d,
      redraw: draw,
      model: () => st.m,
      route: id => st.m ? route(st.m, id, st.live) : [],
    };
  }

  window.agentOneNet = { create, build, litSet, route, layout2d };
})();
