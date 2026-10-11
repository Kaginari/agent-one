// net3d.js — the neural net in depth: a hand-written WebGL2 stage (no library). The tree stands as
// layers: the orchestrator at the top, each rank a ring on its own faint plane below it (a parent sits at
// the middle of its children's arc; each plane is captioned with the lexicon's rank words), the
// skills a halo beneath. Nodes are glowing sprites (a core and a soft radial halo, additive on a
// dark theme so they bloom, alpha-blended and smaller on a light one), bonds are screen-space
// ribbons weighted by the commissions they carried, depth fog sets the near layers forward, and
// particles drift along the bonds the way work travels — down. Drag orbits, the wheel or a pinch
// zooms, a click focuses (the camera eases to it), arrows walk the tree (Shift+arrows turn it),
// "reset view" comes home; a dispatch sends a spark down and a report one back up. It renders only
// while seen, at 30 frames a second while it breathes, 12 after ten idle seconds, stops turning
// after an idle minute, and is still (no particles, no spin) under prefers-reduced-motion.
// Colours are the css tokens, read through probe elements.
(() => {
  'use strict';
  const TAU = Math.PI * 2;
  const clamp = (v, a, b) => Math.max(a, Math.min(b, v));

  function supported() {
    if (supported.v !== undefined) return supported.v;
    try { const c = document.createElement('canvas'); supported.v = !!c.getContext('webgl2'); } catch (_) { supported.v = false; }
    return supported.v;
  }

  // ---------- tiny matrix kit (column-major) ----------
  function perspective(fovy, aspect, near, far) {
    const f = 1 / Math.tan(fovy / 2), nf = 1 / (near - far);
    return new Float32Array([f / aspect, 0, 0, 0, 0, f, 0, 0, 0, 0, (far + near) * nf, -1, 0, 0, 2 * far * near * nf, 0]);
  }
  function lookAt(e, t, u) {
    let zx = e[0] - t[0], zy = e[1] - t[1], zz = e[2] - t[2];
    let l = Math.hypot(zx, zy, zz) || 1; zx /= l; zy /= l; zz /= l;
    let xx = u[1] * zz - u[2] * zy, xy = u[2] * zx - u[0] * zz, xz = u[0] * zy - u[1] * zx;
    l = Math.hypot(xx, xy, xz) || 1; xx /= l; xy /= l; xz /= l;
    const yx = zy * xz - zz * xy, yy = zz * xx - zx * xz, yz = zx * xy - zy * xx;
    return new Float32Array([xx, yx, zx, 0, xy, yy, zy, 0, xz, yz, zz, 0,
      -(xx * e[0] + xy * e[1] + xz * e[2]), -(yx * e[0] + yy * e[1] + yz * e[2]), -(zx * e[0] + zy * e[1] + zz * e[2]), 1]);
  }
  function mul(a, b) {
    const o = new Float32Array(16);
    for (let c = 0; c < 4; c++) for (let r = 0; r < 4; r++) {
      let s = 0;
      for (let k = 0; k < 4; k++) s += a[k * 4 + r] * b[c * 4 + k];
      o[c * 4 + r] = s;
    }
    return o;
  }

  // ---------- shaders ----------
  const POINT_VS = `#version 300 es
  in vec3 aPos; in vec4 aColor; in float aSize; in float aCore;
  uniform mat4 uVP; uniform float uScale; uniform vec2 uFog;
  out vec4 vColor; out float vCore;
  void main() {
    vec4 p = uVP * vec4(aPos, 1.0);
    gl_Position = p;
    gl_PointSize = clamp(aSize * uScale / max(p.w, 0.01), 1.0, 512.0);
    float fog = clamp((p.w - uFog.x) / max(uFog.y - uFog.x, 0.001), 0.0, 1.0);
    vColor = vec4(aColor.rgb, aColor.a * (1.0 - 0.62 * fog)); vCore = aCore;
  }`;
  const POINT_FS = `#version 300 es
  precision mediump float;
  in vec4 vColor; in float vCore;
  out vec4 o;
  void main() {
    vec2 d = gl_PointCoord * 2.0 - 1.0;
    float r = length(d);
    if (r > 1.0) discard;
    float core = vCore > 0.0 ? 1.0 - smoothstep(vCore - 0.12, vCore, r) : 0.0;
    float rim = vCore > 0.0 ? smoothstep(vCore - 0.3, vCore - 0.12, r) * core : 0.0;
    float glow = exp(-r * r * 4.5) * (1.0 - core);
    vec3 c = mix(vColor.rgb, vColor.rgb * 1.25 + 0.08, rim);
    float a = vColor.a * clamp(core + glow * 0.6, 0.0, 1.0);
    o = vec4(c * a, a);
  }`;
  const LINE_VS = `#version 300 es
  in vec2 aCorner;
  in vec3 aA; in vec3 aB; in vec4 aCol; in float aWidth;
  uniform mat4 uVP; uniform vec2 uView; uniform vec2 uFog;
  out vec4 vColor; out float vSide;
  void main() {
    vec4 pa = uVP * vec4(aA, 1.0), pb = uVP * vec4(aB, 1.0);
    vec2 sa = pa.xy / pa.w, sb = pb.xy / pb.w;
    vec2 d = (sb - sa) * uView;
    vec2 dir = length(d) > 0.0001 ? normalize(d) : vec2(1.0, 0.0);
    vec2 n = vec2(-dir.y, dir.x);
    vec4 p = mix(pa, pb, aCorner.x);
    p.xy += n * aCorner.y * aWidth / uView * p.w;
    gl_Position = p;
    float fog = clamp((p.w - uFog.x) / max(uFog.y - uFog.x, 0.001), 0.0, 1.0);
    vColor = vec4(aCol.rgb, aCol.a * (1.0 - 0.55 * fog)); vSide = aCorner.y;
  }`;
  // a layer's plane: a faint disc, a little brighter at its rim
  const DISC_VS = `#version 300 es
  in vec4 aPos;
  uniform mat4 uVP;
  out float vA;
  void main() { gl_Position = uVP * vec4(aPos.xyz, 1.0); vA = aPos.w; }`;
  const DISC_FS = `#version 300 es
  precision mediump float;
  in float vA; uniform vec4 uCol;
  out vec4 o;
  void main() { float a = uCol.a * vA; o = vec4(uCol.rgb * a, a); }`;
  const LINE_FS = `#version 300 es
  precision mediump float;
  in vec4 vColor; in float vSide;
  out vec4 o;
  void main() {
    float a = vColor.a * (1.0 - smoothstep(0.35, 1.0, abs(vSide)));
    o = vec4(vColor.rgb * a, a);
  }`;
  function program(gl, vs, fs) {
    const mk = (type, src) => { const sh = gl.createShader(type); gl.shaderSource(sh, src); gl.compileShader(sh); if (!gl.getShaderParameter(sh, gl.COMPILE_STATUS)) throw new Error(gl.getShaderInfoLog(sh)); return sh; };
    const p = gl.createProgram();
    gl.attachShader(p, mk(gl.VERTEX_SHADER, vs)); gl.attachShader(p, mk(gl.FRAGMENT_SHADER, fs));
    gl.linkProgram(p);
    if (!gl.getProgramParameter(p, gl.LINK_STATUS)) throw new Error(gl.getProgramInfoLog(p));
    return p;
  }

  // ---------- colours: the tokens, through probes, as linear-ish rgb ----------
  const conv = document.createElement('canvas');
  conv.width = conv.height = 1;
  const cctx = conv.getContext('2d', { willReadFrequently: true });
  function rgb(css) {
    cctx.clearRect(0, 0, 1, 1);
    cctx.fillStyle = css;
    cctx.fillRect(0, 0, 1, 1);
    const d = cctx.getImageData(0, 0, 1, 1).data;
    return [d[0] / 255, d[1] / 255, d[2] / 255];
  }

  function create(stage, opts) {
    if (!supported()) return null;
    const canvas = document.createElement('canvas');
    canvas.className = 'net-canvas';
    canvas.tabIndex = 0;
    canvas.setAttribute('role', 'img');
    canvas.setAttribute('aria-label', 'the net in 3D — drag to orbit, wheel to zoom, click a neuron to focus it; arrow keys walk the tree and the camera follows, Enter pins, Shift and the arrows turn it, + and - zoom');
    canvas.setAttribute('aria-describedby', 'net-keys');
    const overlay = document.createElement('canvas');
    overlay.className = 'net-overlay';
    overlay.setAttribute('aria-hidden', 'true');
    const gl = canvas.getContext('webgl2', { antialias: true, premultipliedAlpha: true, alpha: true });
    if (!gl) return null;
    const octx = overlay.getContext('2d');
    // the view's own tools: home, and a first-use hint until the reader has touched the stage
    const tools = document.createElement('div');
    tools.className = 'net-3d-tools';
    const resetBtn = document.createElement('button');
    resetBtn.type = 'button'; resetBtn.className = 'button'; resetBtn.dataset.size = 's'; resetBtn.dataset.variant = 'quiet';
    resetBtn.textContent = 'reset view';
    tools.append(resetBtn);
    const hint = document.createElement('p');
    hint.className = 'net-3d-hint';
    hint.textContent = 'drag to orbit · wheel or pinch to zoom · click a neuron to focus · arrows walk the tree';
    let seen = false;
    try { seen = localStorage.getItem('dash.net3d.seen') === '1'; } catch (_) { /* private mode */ }
    hint.hidden = seen;
    const touched = () => { if (hint.hidden) return; hint.hidden = true; try { localStorage.setItem('dash.net3d.seen', '1'); } catch (_) { /* private mode */ } };
    let P, LP, DP, quad, pbuf, lbuf, dbuf, vaoP, vaoL, vaoD;
    function init() {
      P = program(gl, POINT_VS, POINT_FS);
      LP = program(gl, LINE_VS, LINE_FS);
      DP = program(gl, DISC_VS, DISC_FS);
      dbuf = gl.createBuffer();
      vaoD = gl.createVertexArray();
      gl.bindVertexArray(vaoD);
      gl.bindBuffer(gl.ARRAY_BUFFER, dbuf);
      gl.enableVertexAttribArray(gl.getAttribLocation(DP, 'aPos')); gl.vertexAttribPointer(gl.getAttribLocation(DP, 'aPos'), 4, gl.FLOAT, false, 16, 0);
      gl.bindVertexArray(null);
      pbuf = gl.createBuffer(); lbuf = gl.createBuffer(); quad = gl.createBuffer();
      gl.bindBuffer(gl.ARRAY_BUFFER, quad);
      gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([0, -1, 1, -1, 1, 1, 0, -1, 1, 1, 0, 1]), gl.STATIC_DRAW);
      vaoP = gl.createVertexArray();
      gl.bindVertexArray(vaoP);
      gl.bindBuffer(gl.ARRAY_BUFFER, pbuf);
      const pl = n => gl.getAttribLocation(P, n);
      const PS = 9 * 4; // pos3 col4 size core
      gl.enableVertexAttribArray(pl('aPos')); gl.vertexAttribPointer(pl('aPos'), 3, gl.FLOAT, false, PS, 0);
      gl.enableVertexAttribArray(pl('aColor')); gl.vertexAttribPointer(pl('aColor'), 4, gl.FLOAT, false, PS, 12);
      gl.enableVertexAttribArray(pl('aSize')); gl.vertexAttribPointer(pl('aSize'), 1, gl.FLOAT, false, PS, 28);
      gl.enableVertexAttribArray(pl('aCore')); gl.vertexAttribPointer(pl('aCore'), 1, gl.FLOAT, false, PS, 32);
      vaoL = gl.createVertexArray();
      gl.bindVertexArray(vaoL);
      const ll = n => gl.getAttribLocation(LP, n);
      gl.bindBuffer(gl.ARRAY_BUFFER, quad);
      gl.enableVertexAttribArray(ll('aCorner')); gl.vertexAttribPointer(ll('aCorner'), 2, gl.FLOAT, false, 8, 0);
      gl.bindBuffer(gl.ARRAY_BUFFER, lbuf);
      const LS = 11 * 4; // a3 b3 col4 width
      const inst = (n, size, off) => { const loc = ll(n); gl.enableVertexAttribArray(loc); gl.vertexAttribPointer(loc, size, gl.FLOAT, false, LS, off); gl.vertexAttribDivisor(loc, 1); };
      inst('aA', 3, 0); inst('aB', 3, 12); inst('aCol', 4, 24); inst('aWidth', 1, 40);
      gl.bindVertexArray(null);
    }
    init();

    const st = {
      m: null, nodes: new Map(), edges: [], holds: [], rings: [], focus: '', shown: '', lit: null, live: new Set(), hover: '',
      yaw: 0.7, pitch: 0.42, dist: 10, dist0: 10, target: [0, 0, 0], goal: { target: [0, 0, 0], dist: 10 },
      run: false, raf: 0, last: 0, t: 0, idleAt: performance.now(), pulses: [], flashes: new Map(), pal: null, dark: true, W: 1, H: 1, dpr: 1, VP: null,
      slow: 0, busy: new Set(), R: 4, discs: null, font: null,
    };

    // ---------- the scene: a radial tidy tree, one plane per layer ----------
    function scene(m) {
      st.m = m;
      st.nodes = new Map();
      const leaves = [], slot = new Map();
      const place = id => {
        const ks = m.kids.get(id) || [];
        if (!ks.length) { slot.set(id, leaves.length); leaves.push(id); return; }
        ks.forEach(place);
        slot.set(id, ks.reduce((a, k) => a + slot.get(k), 0) / ks.length);
      };
      if (m.orchestrator) place(m.orchestrator.id);
      for (const id of m.order) if (!slot.has(id)) place(id);
      const D = Math.max(1, m.maxDepth), N = Math.max(1, leaves.length);
      // the layers stand apart by about 0.6 of the widest ring, so they read as planes
      const R = clamp(2 + N * 0.11, 2.6, 6.2), DY = R * 0.6;
      st.R = R;
      const y0 = (D * DY) / 2 + 0.3;
      st.rings = [];
      for (const id of m.order) {
        const n = m.byId.get(id), d = m.depth.get(id) || 0;
        const a = (slot.get(id) / N) * TAU + 0.5 / N * TAU;
        const r = d === 0 ? 0 : R * Math.pow(d / D, 0.9);
        const size = n.orchestrator ? 0.62 : d === 1 ? 0.42 : d === 2 ? 0.34 : 0.27;
        st.nodes.set(id, { id, n, p: [Math.cos(a) * r, y0 - d * DY, Math.sin(a) * r], a, d, size, rank: n.rank || 'plain' });
      }
      // each plane's caption: the lexicon's words for the ranks on it, and how many stand there
      const word = opts.word || ((k, d) => d || k);
      const caption = d => {
        const ranks = [];
        let n = 0;
        for (const id of m.order) if ((m.depth.get(id) || 0) === d) { n++; const r = m.byId.get(id).rank || ''; if (!ranks.includes(r)) ranks.push(r); }
        return ranks.map(r => word('rank.' + r, r)).join(' · ').toUpperCase() + '  ' + n;
      };
      for (let d = 1; d <= m.maxDepth; d++) st.rings.push({ y: y0 - d * DY, r: R * Math.pow(d / D, 0.9), caption: caption(d) });
      // skills: a halo beneath, each at the middle of its holders, spread so none overlap
      if (m.skills.length) {
        const my = y0 - (m.maxDepth + 1) * DY, mr = R * 0.78;
        const want = m.skills.map(n => {
          let sx = 0, sz = 0, k = 0;
          for (const w of m.holds) if (w.to === n.id && st.nodes.has(w.from)) { const q = st.nodes.get(w.from); sx += Math.cos(q.a); sz += Math.sin(q.a); k++; }
          return { n, a: k ? Math.atan2(sz, sx) : Math.PI };
        }).sort((x, y) => x.a - y.a);
        const gap = TAU / want.length * 0.7;
        for (let i = 1; i < want.length; i++) if (want[i].a - want[i - 1].a < gap) want[i].a = want[i - 1].a + gap;
        want.forEach(w => st.nodes.set(w.n.id, { id: w.n.id, n: w.n, p: [Math.cos(w.a) * mr, my, Math.sin(w.a) * mr], a: w.a, d: -1, size: 0.2, rank: w.n.lane || 'skill:shared' }));
        st.rings.push({ y: my, r: mr, skills: true, caption: String(word('skills', 'skills')).toUpperCase() + '  ' + m.skills.length });
      }
      const maxW = Math.max(1, ...m.tree.map(t => t.weight));
      st.edges = m.tree.filter(t => st.nodes.has(t.up) && st.nodes.has(t.down)).map((t, i) => ({ ...t, k: t.weight ? Math.log2(1 + t.weight) / Math.log2(1 + maxW) : 0, seed: (i * 0.6180339) % 1 }));
      st.holds = m.holds.filter(w => st.nodes.has(w.from) && st.nodes.has(w.to));
      // the camera looks at the middle of the whole stack, orchestrator to skills, from far enough to
      // see all of it in the stage's shape
      let lo = Infinity, hi = -Infinity;
      for (const q of st.nodes.values()) { lo = Math.min(lo, q.p[1]); hi = Math.max(hi, q.p[1]); }
      st.home = [0, isFinite(lo) ? (lo + hi) / 2 : 0, 0];
      st.halfH = isFinite(lo) ? (hi - lo) / 2 : 1;
      fitDist();
      if (!st.focus) { st.goal.dist = st.dist0; st.dist = st.dist0; st.goal.target = st.home.slice(); st.target = st.home.slice(); }
      // the planes, once per model: a fan of triangles per ring (alpha in w: centre dim, rim brighter)
      const DV = [], SEG = 64;
      for (const r of st.rings) for (let i = 0; i < SEG; i++) {
        const a0 = i / SEG * TAU, a1 = (i + 1) / SEG * TAU, rr = r.r * 1.12;
        DV.push(0, r.y, 0, 0.35, Math.cos(a0) * rr, r.y, Math.sin(a0) * rr, 1, Math.cos(a1) * rr, r.y, Math.sin(a1) * rr, 1);
      }
      st.discs = new Float32Array(DV);
      gl.bindBuffer(gl.ARRAY_BUFFER, dbuf);
      gl.bufferData(gl.ARRAY_BUFFER, st.discs, gl.STATIC_DRAW);
      st.idleAt = performance.now();
      palette();
      kick();
    }
    // the home distance: the stack's height and its widest ring both inside the view
    function fitDist() {
      const t = Math.tan(0.36), aspect = st.W / Math.max(1, st.H);
      const v = (st.halfH || 1) * Math.cos(0.42) + st.R * 1.12 * Math.sin(0.42) * 0.6 + 0.3, h = st.R * 1.12 + 0.3;
      st.dist0 = Math.max(v / t, h / (t * aspect)) + st.R * 0.2;
      if (!st.focus && !st.zoomed) { st.goal.dist = st.dist0; st.dist = st.dist0; }
    }
    // a bond curves: it leaves its parent level, then falls to the child (a bezier, sampled)
    function bez(A, B, t) {
      const C = [A[0] * 0.35 + B[0] * 0.65, A[1] - (A[1] - B[1]) * 0.12, A[2] * 0.35 + B[2] * 0.65];
      const u = 1 - t;
      return [u * u * A[0] + 2 * u * t * C[0] + t * t * B[0], u * u * A[1] + 2 * u * t * C[1] + t * t * B[1], u * u * A[2] + 2 * u * t * C[2] + t * t * B[2]];
    }

    // ---------- the theme ----------
    function palette() {
      const probes = opts.probes;
      const want = new Set(['ink', 'muted', 'line', 'surface', 'ok', 'warn', 'danger', 'accent']);
      const ranks = new Set();
      for (const q of st.nodes.values()) ranks.add(q.rank);
      probes.replaceChildren();
      const read = (attr, v) => { const sp = document.createElement('span'); sp.className = 'net-probe'; sp.setAttribute(attr, v); probes.append(sp); return sp; };
      const els = {};
      for (const t of want) els['t:' + t] = read('data-tone', t);
      for (const r of ranks) els['r:' + r] = read('data-rank', r);
      const pal = { rank: {} };
      for (const t of want) pal[t] = rgb(getComputedStyle(els['t:' + t]).color);
      for (const r of ranks) pal.rank[r] = rgb(getComputedStyle(els['r:' + r]).color);
      const s = pal.surface;
      st.dark = 0.2126 * s[0] + 0.7152 * s[1] + 0.0722 * s[2] < 0.5;
      st.pal = pal;
      fontCache();
      kick();
    }
    // the label fonts and inks, read once (per theme or resize), never per frame
    function fontCache() {
      if (!st.pal) return;
      const fam = getComputedStyle(stage).getPropertyValue('--font-body').trim() || 'system-ui';
      const px = parseFloat(getComputedStyle(document.documentElement).fontSize) || 16;
      const css = c => 'rgb(' + c.map(v => Math.round(v * 255)).join(',') + ')';
      st.font = {
        px, fam, ink: css(st.pal.ink), muted: css(st.pal.muted), halo: css(st.pal.surface),
        orchestrator: '650 ' + px * 1.05 + 'px ' + fam, first: '600 ' + px * 0.8 + 'px ' + fam, rest: px * 0.8 + 'px ' + fam, skill: px * 0.72 + 'px ' + fam,
        caption: '650 ' + px * 0.62 + 'px ' + fam,
        widths: new Map(),
      };
    }
    const mo = new MutationObserver(() => palette());
    mo.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme', 'data-palette'] });
    const mq = matchMedia('(prefers-color-scheme: dark)');
    const onScheme = () => palette();
    mq.addEventListener('change', onScheme);

    // ---------- the camera ----------
    function camera() {
      const t = st.target, cp = Math.cos(st.pitch);
      const eye = [t[0] + st.dist * cp * Math.sin(st.yaw), t[1] + st.dist * Math.sin(st.pitch), t[2] + st.dist * cp * Math.cos(st.yaw)];
      const proj = perspective(0.72, st.W / st.H, 0.1, 200);
      st.VP = mul(proj, lookAt(eye, t, [0, 1, 0]));
      st.scale = st.H * st.dpr / (2 * Math.tan(0.36));
    }
    function project(p) {
      const M = st.VP;
      const x = M[0] * p[0] + M[4] * p[1] + M[8] * p[2] + M[12];
      const y = M[1] * p[0] + M[5] * p[1] + M[9] * p[2] + M[13];
      const w = M[3] * p[0] + M[7] * p[1] + M[11] * p[2] + M[15];
      return { x: (x / w + 1) / 2 * st.W, y: (1 - y / w) / 2 * st.H, w };
    }

    // ---------- a frame ----------
    function frame(now) {
      st.raf = 0;
      if (!st.run) return;
      const reduced = opts.reduced();
      const busy = st.drag || st.pulses.length || st.flashes.size || st.busy.size || Math.abs(st.dist - st.goal.dist) > 0.01 || Math.hypot(st.target[0] - st.goal.target[0], st.target[1] - st.goal.target[1], st.target[2] - st.goal.target[2]) > 0.01;
      const ambient = !reduced;
      // the budget: 60 a second while something moves for a reason, 30 while it only breathes,
      // 12 once nobody has touched it for ten seconds; the turn stops after an idle minute
      const idle = now - st.idleAt;
      const budget = busy ? 15 : idle > 10000 ? 83 : 32;
      if (st.last && now - st.last < budget) { schedule(); return; }
      const dt = st.last ? Math.min(0.1, (now - st.last) / 1000) : 0.016;
      const spent = performance.now();
      st.last = now;
      st.t += dt;
      if (ambient && !st.drag && !st.focus && idle > 3500 && idle < 60000) st.yaw += dt * 0.07;
      const k = 1 - Math.pow(0.0015, dt);
      st.dist += (st.goal.dist - st.dist) * k;
      for (let i = 0; i < 3; i++) st.target[i] += (st.goal.target[i] - st.target[i]) * k;
      render(now, reduced);
      st.slow = st.slow * 0.9 + (performance.now() - spent) * 0.1;
      if (ambient || busy) schedule();
    }
    function schedule() { if (st.run && !st.raf) st.raf = requestAnimationFrame(frame); }
    function kick() { if (st.run) { if (!st.raf) st.raf = requestAnimationFrame(frame); } }

    function render(now, reduced) {
      if (!st.pal || !st.m) return;
      camera();
      const pal = st.pal, dark = st.dark, lit = st.lit, focus = st.shown || '';
      const cw = Math.round(st.W * st.dpr), ch = Math.round(st.H * st.dpr);
      gl.viewport(0, 0, cw, ch);
      gl.clearColor(0, 0, 0, 0);
      gl.clear(gl.COLOR_BUFFER_BIT);
      gl.enable(gl.BLEND);
      if (dark) gl.blendFunc(gl.ONE, gl.ONE); else gl.blendFunc(gl.ONE, gl.ONE_MINUS_SRC_ALPHA);
      // fog: the far side of the stack steps back
      const fog = [Math.max(0.1, st.dist - st.R * 0.6), st.dist + st.R * 1.4];
      // the planes first, under everything
      if (st.discs && st.discs.length) {
        gl.useProgram(DP);
        gl.uniformMatrix4fv(gl.getUniformLocation(DP, 'uVP'), false, st.VP);
        const c = pal.muted;
        gl.uniform4f(gl.getUniformLocation(DP, 'uCol'), c[0], c[1], c[2], (dark ? 0.035 : 0.05) * (focus ? 0.5 : 1));
        gl.bindVertexArray(vaoD);
        gl.drawArrays(gl.TRIANGLES, 0, st.discs.length / 4);
      }
      const dim = id => focus && lit && !lit.nodes.has(id);
      const col = r => pal.rank[r] || pal.muted;
      // ribbons
      const RB = [];
      const seg = (a, b, c, alpha, w) => RB.push(a[0], a[1], a[2], b[0], b[1], b[2], c[0], c[1], c[2], alpha, w * st.dpr);
      for (const r of st.rings) {
        const n = 72;
        for (let i = 0; i < n; i++) {
          const a0 = i / n * TAU, a1 = (i + 1) / n * TAU;
          seg([Math.cos(a0) * r.r, r.y, Math.sin(a0) * r.r], [Math.cos(a1) * r.r, r.y, Math.sin(a1) * r.r], r.skills ? pal.accent : pal.muted, (dark ? 0.16 : 0.22) * (focus ? 0.4 : 1), 1);
        }
      }
      for (const w of st.holds) {
        const on = focus && lit && lit.holds.has(w.from + '>' + w.to);
        seg(st.nodes.get(w.from).p, st.nodes.get(w.to).p, pal.accent, on ? 0.55 : focus ? 0.02 : (dark ? 0.05 : 0.07), on ? 1.4 : 1);
      }
      for (const e of st.edges) {
        const A = st.nodes.get(e.up).p, B = st.nodes.get(e.down).p, c = col(st.nodes.get(e.down).rank);
        const on = focus && lit && lit.tree.has(e.up + '>' + e.down);
        const liveE = st.live.has(e.down);
        const base = (dark ? 0.42 : 0.55) + 0.4 * e.k;
        const alpha = focus ? (on ? 0.95 : 0.05) : liveE ? 0.95 : base * (e.implied ? 0.5 : 1);
        const w = (1.3 + 3 * e.k) * (on || liveE ? 1.5 : 1);
        let prev = A;
        for (let i = 1; i <= 12; i++) { const q = bez(A, B, i / 12); seg(prev, q, c, e.implied && i % 2 ? alpha * 0.3 : alpha, w); prev = q; }
      }
      gl.useProgram(LP);
      gl.uniformMatrix4fv(gl.getUniformLocation(LP, 'uVP'), false, st.VP);
      gl.uniform2f(gl.getUniformLocation(LP, 'uView'), cw, ch);
      gl.uniform2f(gl.getUniformLocation(LP, 'uFog'), fog[0], fog[1]);
      gl.bindBuffer(gl.ARRAY_BUFFER, lbuf);
      gl.bufferData(gl.ARRAY_BUFFER, new Float32Array(RB), gl.DYNAMIC_DRAW);
      gl.bindVertexArray(vaoL);
      gl.drawArraysInstanced(gl.TRIANGLES, 0, 6, RB.length / 11);
      // sprites: halos, cores, particles, sparks — back to front
      const S = [];
      const put = (p, c, a, size, core) => S.push(p[0], p[1], p[2], c[0], c[1], c[2], a, size, core);
      const items = [];
      const breath = reduced ? 1 : 0.75 + 0.25 * Math.sin(st.t * 2.4);
      for (const q of st.nodes.values()) {
        const c = col(q.rank), d = dim(q.id), on = q.id === focus;
        const liveN = st.live.has(q.id) || st.busy.has(q.id);
        const a = d ? 0.12 : 1;
        // on a light ground a halo smears: it is 40% smaller there
        const haloK = dark ? 1 : 0.6;
        items.push({ z: project(q.p).w, draw: () => {
          put(q.p, c, (dark ? 0.42 : 0.4) * a * (liveN ? 1.6 * breath : on ? 1.4 : 1), q.size * (liveN ? 4.2 : 3.2) * haloK, 0);
          put(q.p, c, (dark ? 0.95 : 1) * a, q.size, q.n.kind === 'skill' ? 0.5 : 0.62);
        } });
      }
      // particles drift along the bonds, the way work goes: down
      const thin = st.slow > 12 ? 2 : 1;
      if (!reduced) for (const e of st.edges) {
        if (focus && lit && !lit.tree.has(e.up + '>' + e.down)) continue;
        const n = (1 + Math.min(3, Math.floor(Math.log2(1 + e.weight)))) / thin;
        const c = col(st.nodes.get(e.down).rank), A = st.nodes.get(e.up).p, B = st.nodes.get(e.down).p;
        const speed = 0.16 + 0.12 * e.k + (st.live.has(e.down) ? 0.3 : 0);
        for (let j = 0; j < n; j++) {
          const u = (st.t * speed + j / n + e.seed) % 1;
          const fade = Math.sin(u * Math.PI);
          const p = bez(A, B, u);
          items.push({ z: project(p).w, draw: () => put(p, c, (dark ? 0.9 : 0.75) * fade, 0.13, 0.35) });
        }
      }
      // live sparks: a dispatch down, a report up
      st.pulses = st.pulses.filter(pl => {
        const t = (now - pl.at) / (pl.per * pl.hops.length);
        if (t >= 1) { flash(pl.end, pl.tone); return false; }
        const h = Math.min(pl.hops.length - 1, Math.floor(t * pl.hops.length));
        const lt = t * pl.hops.length - h;
        const [up, down] = pl.hops[h];
        const A = st.nodes.get(up).p, B = st.nodes.get(down).p;
        const c = pl.tone === 'down' ? col(st.nodes.get(pl.end).rank) : pal[pl.tone] || pal.ok;
        for (let i = 0; i < 6; i++) {
          let u = lt - i * 0.035;
          if (u < 0) continue;
          u = u < 0.5 ? 2 * u * u : 1 - Math.pow(-2 * u + 2, 2) / 2;
          const p = bez(A, B, pl.dir === 'up' ? 1 - u : u);
          items.push({ z: -1e9, draw: () => { put(p, c, 1 - i * 0.15, i ? 0.22 - i * 0.025 : 0.95, i ? 0.4 : 0); if (!i) put(p, c, 1, 0.24, 0.6); } });
        }
        return true;
      });
      for (const [id, f] of st.flashes) {
        const t = (now - f.at) / (f.kind === 'read' ? 700 : 1100);
        if (t >= 1) { st.flashes.delete(id); continue; }
        const q = st.nodes.get(id);
        if (!q) continue;
        const c = f.tone === 'info' ? pal.accent : f.tone === 'rank' || f.tone === 'down' ? col(q.rank) : pal[f.tone] || col(q.rank);
        const big = f.kind === 'read' ? 0.55 : 1;
        items.push({ z: -1e9, draw: () => { put(q.p, c, (1 - t) * 0.9 * big, q.size * (2 + 5 * t * big) * (dark ? 1 : 0.6), 0); if (t < 0.4) put(q.p, c, (0.4 - t) * 2, q.size * 1.2, 0.6); } });
      }
      items.sort((a, b) => b.z - a.z);
      for (const it of items) it.draw();
      gl.useProgram(P);
      gl.uniformMatrix4fv(gl.getUniformLocation(P, 'uVP'), false, st.VP);
      gl.uniform1f(gl.getUniformLocation(P, 'uScale'), st.scale);
      gl.uniform2f(gl.getUniformLocation(P, 'uFog'), fog[0], fog[1]);
      gl.bindBuffer(gl.ARRAY_BUFFER, pbuf);
      gl.bufferData(gl.ARRAY_BUFFER, new Float32Array(S), gl.DYNAMIC_DRAW);
      gl.bindVertexArray(vaoP);
      gl.drawArrays(gl.POINTS, 0, S.length / 9);
      gl.bindVertexArray(null);
      labels(focus, lit);
    }

    // ---------- labels: the upper layers always, the rest on hover, focus or while live — and
    // every one once the view holds 24 neurons or fewer; each plane carries its rank caption ----------
    function labels(focus, lit) {
      const ctx = octx, W = st.W, H = st.H, dpr = st.dpr, F = st.font;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      ctx.clearRect(0, 0, W, H);
      if (!F) return;
      const px = F.px;
      const width = (text, font) => { const k = font + '|' + text; let w = F.widths.get(k); if (w === undefined) { ctx.font = font; w = ctx.measureText(text).width; F.widths.set(k, w); } return w; };
      ctx.textBaseline = 'middle';
      ctx.lineJoin = 'round';
      // the planes' captions, at each ring's leftmost point on screen (else its rightmost)
      ctx.font = F.caption;
      for (const r of st.rings) {
        if (!r.caption) continue;
        let lo = null, hi = null;
        for (let i = 0; i < 24; i++) {
          const a = i / 24 * TAU, s = project([Math.cos(a) * r.r * 1.12, r.y, Math.sin(a) * r.r * 1.12]);
          if (s.w <= 0) continue;
          if (!lo || s.x < lo.x) lo = s;
          if (!hi || s.x > hi.x) hi = s;
        }
        if (!lo) continue;
        const w = width(r.caption, F.caption);
        const left = lo.x - 10 - w > 4;
        const x = left ? lo.x - 10 : hi.x + 10, y = left ? lo.y : hi.y;
        if (!left && x + w > W - 4) continue;
        ctx.globalAlpha = focus ? 0.35 : 0.8;
        ctx.textAlign = left ? 'right' : 'left';
        ctx.lineWidth = 3; ctx.strokeStyle = F.halo; ctx.strokeText(r.caption, x, y);
        ctx.fillStyle = F.muted; ctx.fillText(r.caption, x, y);
      }
      ctx.textAlign = 'left';
      const onScreen = [];
      for (const q of st.nodes.values()) {
        const s = project(q.p);
        if (s.w <= 0 || s.x < -40 || s.x > W + 40 || s.y < -20 || s.y > H + 20) continue;
        onScreen.push({ q, s });
      }
      const few = onScreen.length <= 24;
      const many = st.nodes.size > 40;
      const shown = [];
      for (const { q, s } of onScreen) {
        const on = q.id === focus || (focus && lit && lit.nodes.has(q.id));
        const always = few || q.d === 0 || q.d === 1 || (q.d === 2 && !many);
        if (!(on || st.live.has(q.id) || (!focus && always))) continue;
        shown.push({ q, s });
      }
      // the focus first, then the nearest: a farther label gives way where two would overlap
      shown.sort((a, b) => a.q.id === focus ? -1 : b.q.id === focus ? 1 : a.s.w - b.s.w);
      const taken = [];
      for (const { q, s } of shown) {
        const n = q.n;
        const text = n.orchestrator ? (n.label || n.name) : n.kind === 'skill' ? n.name : (n.rank && n.name.startsWith(n.rank + '-') ? n.name.slice(n.rank.length + 1) : n.name);
        const font = q.d === 0 ? F.orchestrator : q.d === 1 ? F.first : q.d < 0 ? F.skill : F.rest;
        const w = width(text, font);
        const r = q.size * st.scale / s.w / dpr * 0.5;
        const x = s.x + r + 6, y = s.y;
        const box = [x - 2, y - px * 0.6, x + w + 2, y + px * 0.6];
        if (q.id !== focus && taken.some(b => !(box[2] < b[0] || box[0] > b[2] || box[3] < b[1] || box[1] > b[3]))) continue;
        taken.push(box);
        const near = clamp(1.4 - (s.w - st.dist * 0.75) / (st.dist0 * 0.9), 0.45, 1);
        ctx.globalAlpha = q.id === focus ? 1 : near;
        ctx.font = font;
        ctx.lineWidth = 3.5;
        ctx.strokeStyle = F.halo;
        ctx.strokeText(text, x, y);
        ctx.fillStyle = F.ink;
        ctx.fillText(text, x, y);
      }
      ctx.globalAlpha = 1;
    }

    // ---------- picking and the pointer ----------
    function pick(x, y) {
      if (!st.VP) return '';
      let best = '', bd = Infinity;
      for (const q of st.nodes.values()) {
        const s = project(q.p);
        if (s.w <= 0) continue;
        const r = Math.max(10, q.size * st.scale / s.w / st.dpr * 0.7);
        const d = Math.hypot(s.x - x, s.y - y);
        if (d < r && d < bd) { bd = d; best = q.id; }
      }
      return best;
    }
    const ptrs = new Map();
    let down = null, pinch = 0;
    const local = ev => { const r = canvas.getBoundingClientRect(); return [ev.clientX - r.left, ev.clientY - r.top]; };
    canvas.addEventListener('pointerdown', ev => {
      canvas.setPointerCapture(ev.pointerId);
      ptrs.set(ev.pointerId, local(ev));
      down = { at: local(ev), moved: false, yaw: st.yaw, pitch: st.pitch };
      touched();
      if (ptrs.size === 2) { const [a, b] = [...ptrs.values()]; pinch = Math.hypot(a[0] - b[0], a[1] - b[1]); }
      st.idleAt = performance.now();
    });
    canvas.addEventListener('pointermove', ev => {
      const p = local(ev);
      if (ptrs.has(ev.pointerId)) ptrs.set(ev.pointerId, p);
      if (ptrs.size === 2 && pinch) {
        const [a, b] = [...ptrs.values()], d = Math.hypot(a[0] - b[0], a[1] - b[1]);
        st.zoomed = true; st.goal.dist = clamp(st.goal.dist * pinch / Math.max(1, d), 2, st.dist0 * 2.2); st.dist = st.goal.dist; pinch = d;
        kick(); return;
      }
      if (down && ptrs.size === 1) {
        const dx = p[0] - down.at[0], dy = p[1] - down.at[1];
        if (!down.moved && Math.hypot(dx, dy) > 4) { down.moved = true; st.drag = true; canvas.setAttribute('data-drag', ''); }
        if (down.moved) { st.yaw = down.yaw - dx * 0.007; st.pitch = clamp(down.pitch + dy * 0.005, -1.25, 1.25); st.idleAt = performance.now(); kick(); }
        return;
      }
      const id = pick(p[0], p[1]);
      canvas.toggleAttribute('data-hot', !!id);
      if (id !== st.hover) { st.hover = id; opts.onHover(id); kick(); }
    });
    const end = ev => {
      ptrs.delete(ev.pointerId);
      if (ptrs.size < 2) pinch = 0;
      if (down && !down.moved && ev.type === 'pointerup') opts.onPick(pick(...local(ev)));
      if (!ptrs.size) { down = null; st.drag = false; canvas.removeAttribute('data-drag'); }
      st.idleAt = performance.now();
    };
    canvas.addEventListener('pointerup', end);
    canvas.addEventListener('pointercancel', end);
    canvas.addEventListener('pointerleave', () => { if (st.hover) { st.hover = ''; opts.onHover(''); } canvas.removeAttribute('data-hot'); });
    canvas.addEventListener('wheel', ev => {
      ev.preventDefault();
      touched();
      st.zoomed = true;
      st.goal.dist = clamp(st.goal.dist * Math.exp(ev.deltaY * 0.0012), 2, st.dist0 * 2.2);
      st.idleAt = performance.now(); kick();
    }, { passive: false });
    canvas.addEventListener('keydown', ev => {
      touched();
      if (opts.onKey && opts.onKey(ev)) { ev.preventDefault(); st.idleAt = performance.now(); kick(); if (!st.run || opts.reduced()) render(performance.now(), true); return; }
      if (!ev.shiftKey && /^Arrow/.test(ev.key)) return;
      const turn = { ArrowLeft: -0.2, ArrowRight: 0.2 }[ev.key], tilt = { ArrowUp: -0.12, ArrowDown: 0.12 }[ev.key];
      if (turn) st.yaw += turn;
      else if (tilt) st.pitch = clamp(st.pitch + tilt, -1.25, 1.25);
      else if (ev.key === '+' || ev.key === '=') { st.zoomed = true; st.goal.dist = clamp(st.goal.dist * 0.85, 2, st.dist0 * 2.2); }
      else if (ev.key === '-') { st.zoomed = true; st.goal.dist = clamp(st.goal.dist / 0.85, 2, st.dist0 * 2.2); }
      else return;
      ev.preventDefault(); st.idleAt = performance.now(); kick();
      if (!st.run || opts.reduced()) render(performance.now(), true);
    });
    canvas.addEventListener('blur', () => opts.onBlur && opts.onBlur());
    resetBtn.addEventListener('click', () => {
      touched();
      st.yaw = 0.7; st.pitch = 0.42; st.zoomed = false;
      if (st.focus) opts.onPick('');
      st.goal.target = (st.home || [0, 0, 0]).slice(); st.goal.dist = st.dist0;
      st.idleAt = performance.now(); kick();
      if (!st.run || opts.reduced()) render(performance.now(), true);
    });
    canvas.addEventListener('webglcontextlost', ev => { ev.preventDefault(); st.run = false; });
    canvas.addEventListener('webglcontextrestored', () => { init(); st.run = true; kick(); });

    function resize() {
      const r = stage.getBoundingClientRect();
      st.W = Math.max(1, r.width); st.H = Math.max(1, r.height);
      st.dpr = Math.min(2, devicePixelRatio || 1);
      for (const c of [canvas, overlay]) { c.width = Math.round(st.W * st.dpr); c.height = Math.round(st.H * st.dpr); }
      fontCache();
      if (st.m) fitDist();
      if (st.m) { st.last = 0; render(performance.now(), opts.reduced()); }
      kick();
    }
    const ro = new ResizeObserver(resize);
    ro.observe(stage);

    function flash(id, tone, kind) { st.flashes.set(id, { at: performance.now(), tone, kind }); kick(); }

    return {
      elements: [canvas, overlay, tools, hint],
      model(m) { scene(m); resize(); },
      focus(id, lit, pinned) {
        st.lit = lit; st.shown = id || '';
        // the camera eases to a pinned neuron; a hover only lights
        if ((pinned || '') !== st.focus) {
          st.focus = pinned || '';
          const q = st.nodes.get(st.focus);
          st.goal.target = q ? q.p.slice() : (st.home || [0, 0, 0]).slice();
          st.goal.dist = q ? Math.max(3.5, st.dist0 * 0.62) : st.dist0;
        }
        kick();
        if (opts.reduced()) render(performance.now(), true);
      },
      live(ids) { st.live = ids; kick(); },
      act(id, tone, kind) { if (st.nodes.has(id)) flash(id, tone, kind); },
      // a burst on a neuron: it breathes like a live one until the burst ends
      busy(id, on) { if (on) st.busy.add(id); else st.busy.delete(id); kick(); },
      // the keyboard's cursor: the camera eases to it without pinning it
      follow(id) {
        const q = st.nodes.get(id);
        if (!q) return;
        st.goal.target = q.p.slice();
        st.goal.dist = Math.min(st.goal.dist, st.dist0 * 0.85);
        st.idleAt = performance.now(); kick();
      },
      signal(kind, hops, id, tone) {
        const pairs = hops.map(h => h.split('>')).filter(([a, b]) => st.nodes.has(a) && st.nodes.has(b));
        const endAt = kind === 'up' ? (pairs.length ? pairs[0][0] : id) : id;
        const t = kind === 'up' ? (tone || 'ok') : 'down';
        if (opts.reduced() || !pairs.length) { flash(endAt, t); return; }
        st.pulses.push({ hops: kind === 'up' ? pairs.slice().reverse() : pairs, dir: kind, at: performance.now(), per: 700, end: endAt, tone: t });
        kick();
      },
      running(v) {
        st.run = v;
        if (v) { st.last = 0; resize(); kick(); }
        else if (st.raf) { cancelAnimationFrame(st.raf); st.raf = 0; }
      },
      resize,
      dispose() {
        st.run = false;
        if (st.raf) cancelAnimationFrame(st.raf);
        ro.disconnect(); mo.disconnect(); mq.removeEventListener('change', onScheme);
        const lose = gl.getExtension('WEBGL_lose_context');
        if (lose) lose.loseContext();
      },
    };
  }

  window.agentOneNet3D = { supported, create };
})();
