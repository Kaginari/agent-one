// net3d.js — the neural net as a brain: a hand-written WebGL2 stage (no library). The orchestrator sits at
// the centre, the thalamus every signal relays through; each rank is a shell around it (the first
// rank inner, the next one further out, the last on the outer "cortex"), so rank reads as depth from
// the centre. Each lane — a first-rank member's subtree — is a region (a lobe), tinted by a faint
// glow; inside a region a seeded force layout on the brain's two hemispheres keeps a member near
// its parent and pulls related members together (the team's `relates`: code that imports code,
// skills both hold; and work commissioned together), so the neighbourhoods the work keeps show. The
// skills are a crown above the brain, their `holds` filaments descending to the members that hold
// them. Every rank has its own lit solid (instanced, one draw per shape): the orchestrator a star crystal
// in two orbit rings, coordinators slender bipyramids, domain owners hexagonal prisms, zone workers soft wobbling blobs,
// the service owner a tetrahedron, a promoted rank its base with a crown ring, a skill a torus. Synapses
// taper from the parent; a dispatch travels as a pulse that lights each synapse it crosses, the
// report comes back the same way, a firing neuron flashes and rings; particles drift down the tree
// and down a held skill's filament while its holder works. Drag orbits, the wheel or a pinch zooms, a
// click focuses (the camera eases there), arrows walk the tree (Shift+arrows turn it), "reset view"
// comes home; hovering a relates bond says why it holds; the last turn's signals can be replayed
// and scrubbed. It renders only while seen, at 30 frames a second while it breathes, 12 after ten
// idle seconds, stops turning after an idle minute, and is still (no particles, no spin, no
// settling) under prefers-reduced-motion. Colours are the css tokens, read through probe elements.
(() => {
  'use strict';
  const TAU = Math.PI * 2;
  const clamp = (v, a, b) => Math.max(a, Math.min(b, v));
  const lerp = (a, b, t) => a + (b - a) * t;
  const mix3 = (a, b, t) => [lerp(a[0], b[0], t), lerp(a[1], b[1], t), lerp(a[2], b[2], t)];
  // a stable [0,1) from a string: the layout's seed, so a workspace lands the same way on every load
  const hash = (s, k = 0) => { let h = 2166136261 ^ k; for (let i = 0; i < s.length; i++) { h ^= s.charCodeAt(i); h = Math.imul(h, 16777619); } return ((h >>> 0) % 100000) / 100000; };

  function supported() {
    if (supported.v !== undefined) return supported.v;
    try { const c = document.createElement('canvas'); supported.v = !!c.getContext('webgl2'); } catch (_) { supported.v = false; }
    return supported.v;
  }

  // ---------- tiny vector and matrix kit (column-major) ----------
  const sub = (a, b) => [a[0] - b[0], a[1] - b[1], a[2] - b[2]];
  const add = (a, b) => [a[0] + b[0], a[1] + b[1], a[2] + b[2]];
  const scl = (a, k) => [a[0] * k, a[1] * k, a[2] * k];
  const dot = (a, b) => a[0] * b[0] + a[1] * b[1] + a[2] * b[2];
  const cross = (a, b) => [a[1] * b[2] - a[2] * b[1], a[2] * b[0] - a[0] * b[2], a[0] * b[1] - a[1] * b[0]];
  const len = a => Math.hypot(a[0], a[1], a[2]);
  const unit = a => { const l = len(a) || 1; return [a[0] / l, a[1] / l, a[2] / l]; };
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
  // a growable float buffer, reused frame to frame (no garbage per frame)
  function fbuf(n) {
    const b = { a: new Float32Array(n), n: 0 };
    b.need = k => { if (b.n + k > b.a.length) { const x = new Float32Array(Math.max(b.a.length * 2, b.n + k)); x.set(b.a.subarray(0, b.n)); b.a = x; } };
    return b;
  }

  // ---------- the solids: unit-sized, centred, pos3 + normal3 per vertex ----------
  function flat(tris) {
    const o = [];
    for (let [a, b, c] of tris) {
      let n = unit(cross(sub(b, a), sub(c, a)));
      const m = scl(add(add(a, b), c), 1 / 3);
      if (dot(n, m) < 0) { n = scl(n, -1); [b, c] = [c, b]; }
      for (const p of [a, b, c]) o.push(p[0], p[1], p[2], n[0], n[1], n[2]);
    }
    return new Float32Array(o);
  }
  const ICO = (() => {
    const t = (1 + Math.sqrt(5)) / 2;
    const v = [[-1, t, 0], [1, t, 0], [-1, -t, 0], [1, -t, 0], [0, -1, t], [0, 1, t], [0, -1, -t], [0, 1, -t], [t, 0, -1], [t, 0, 1], [-t, 0, -1], [-t, 0, 1]].map(unit);
    const f = [[0, 11, 5], [0, 5, 1], [0, 1, 7], [0, 7, 10], [0, 10, 11], [1, 5, 9], [5, 11, 4], [11, 10, 2], [10, 7, 6], [7, 1, 8], [3, 9, 4], [3, 4, 2], [3, 2, 6], [3, 6, 8], [3, 8, 9], [4, 9, 5], [2, 4, 11], [6, 2, 10], [8, 6, 7], [9, 8, 1]];
    return { v, f };
  })();
  // the orchestrator: a stellated icosahedron, a spike on every face
  function starMesh() {
    const T = [], k = 1 / 1.5;
    for (const [i, j, l] of ICO.f) {
      const a = ICO.v[i], b = ICO.v[j], c = ICO.v[l], apex = scl(unit(add(add(a, b), c)), 1.5);
      T.push([scl(a, k), scl(b, k), scl(apex, k)], [scl(b, k), scl(c, k), scl(apex, k)], [scl(c, k), scl(a, k), scl(apex, k)]);
    }
    return flat(T);
  }
  // a bipyramid: n sides at the equator, slender (tall points)
  function bipyramid(n, r, h) {
    const T = [], top = [0, h, 0], bot = [0, -h, 0];
    for (let i = 0; i < n; i++) {
      const a0 = i / n * TAU + Math.PI / n, a1 = (i + 1) / n * TAU + Math.PI / n;
      const p = [Math.cos(a0) * r, 0, Math.sin(a0) * r], q = [Math.cos(a1) * r, 0, Math.sin(a1) * r];
      T.push([p, q, top], [q, p, bot]);
    }
    return flat(T);
  }
  function prism(n, r, h) {
    const T = [];
    for (let i = 0; i < n; i++) {
      const a0 = i / n * TAU, a1 = (i + 1) / n * TAU;
      const p0 = [Math.cos(a0) * r, -h, Math.sin(a0) * r], p1 = [Math.cos(a1) * r, -h, Math.sin(a1) * r];
      const q0 = [p0[0], h, p0[2]], q1 = [p1[0], h, p1[2]];
      T.push([p0, p1, q1], [p0, q1, q0], [[0, h, 0], q0, q1], [[0, -h, 0], p1, p0]);
    }
    return flat(T);
  }
  function tetra() {
    const a = [0, 1, 0], s = Math.sqrt(8 / 9), y = -1 / 3;
    const b = [s, y, 0], c = [-s / 2, y, s * Math.sqrt(3) / 2], d = [-s / 2, y, -s * Math.sqrt(3) / 2];
    return flat([[a, b, c], [a, c, d], [a, d, b], [b, d, c]]);
  }
  function cube(k) {
    const T = [];
    for (let ax = 0; ax < 3; ax++) for (const s of [-1, 1]) {
      const P = (u, v) => { const p = [0, 0, 0]; p[ax] = s * k; p[(ax + 1) % 3] = u * k; p[(ax + 2) % 3] = v * k; return p; };
      T.push([P(-1, -1), P(1, -1), P(1, 1)], [P(-1, -1), P(1, 1), P(-1, 1)]);
    }
    return flat(T);
  }
  // a smooth icosphere (the zone worker's agent), a little squashed
  function blobMesh(level, sy) {
    let v = ICO.v.map(p => p.slice()), f = ICO.f.map(x => x.slice());
    for (let l = 0; l < level; l++) {
      const mid = new Map(), nf = [];
      const m = (i, j) => { const k = i < j ? i + ',' + j : j + ',' + i; if (!mid.has(k)) { mid.set(k, v.length); v.push(unit(add(v[i], v[j]))); } return mid.get(k); };
      for (const [a, b, c] of f) { const ab = m(a, b), bc = m(b, c), ca = m(c, a); nf.push([a, ab, ca], [b, bc, ab], [c, ca, bc], [ab, bc, ca]); }
      f = nf;
    }
    const o = [];
    for (const tri of f) {
      let [a, b, c] = tri.map(i => v[i]);
      if (dot(cross(sub(b, a), sub(c, a)), a) < 0) [b, c] = [c, b];
      for (const p of [a, b, c]) { const n = unit([p[0], p[1] / (sy * sy), p[2]]); o.push(p[0], p[1] * sy, p[2], n[0], n[1], n[2]); }
    }
    return new Float32Array(o);
  }
  function torus(R, r, seg, side) {
    const o = [];
    const P = (i, j) => {
      const u = i / seg * TAU, w = j / side * TAU, c = Math.cos(w);
      return [[(R + r * c) * Math.cos(u), r * Math.sin(w), (R + r * c) * Math.sin(u)], [c * Math.cos(u), Math.sin(w), c * Math.sin(u)]];
    };
    for (let i = 0; i < seg; i++) for (let j = 0; j < side; j++) {
      const a = P(i, j), b = P(i + 1, j), c = P(i + 1, j + 1), d = P(i, j + 1);
      for (const [p, n] of [a, b, c, a, c, d]) o.push(p[0], p[1], p[2], n[0], n[1], n[2]);
    }
    return new Float32Array(o);
  }
  const SHAPES = {
    star: () => starMesh(),
    coord: () => bipyramid(4, 0.5, 1),
    coord6: () => bipyramid(6, 0.52, 1),
    domain: () => prism(6, 0.78, 0.5),
    zone: () => blobMesh(2, 0.86),
    service: () => tetra(),
    cube: () => cube(0.62),
    skill: () => torus(0.7, 0.27, 36, 14),
    crown: () => torus(1, 0.06, 48, 6),
  };
  // the rank's solid, and whether it holds a crown ring (a promoted rank)
  const FORM = { orchestrator: ['star', 0], coord: ['coord', 0], 'principal-coordinator': ['coord', 1], 'auditor': ['coord6', 1], domain: ['domain', 0], 'principal-domain-owner': ['domain', 1], zone: ['zone', 0], service: ['service', 0] };
  const formOf = n => n.kind === 'skill' ? ['skill', 0] : n.orchestrator ? ['star', 0] : FORM[n.rank] || ['cube', 0];
  // the key's glyphs (the same forms, flat), drawn in the rank's colour
  const GLYPH = {
    star: 'M12 1.5l2.6 6.9 7.4.3-5.8 4.6 2 7.2L12 16.4l-6.2 4.1 2-7.2L2 8.7l7.4-.3z',
    coord: 'M12 2l5 10-5 10-5-10z', coord6: 'M12 2l5 10-5 10-5-10z', domain: 'M12 3l7.8 4.5v9L12 21l-7.8-4.5v-9z',
    zone: 'M12 4.5c4.7 0 8 3 8 7.2S16.7 19.5 12 19.5 4 16 4 11.7 7.3 4.5 12 4.5z', service: 'M12 3l9 17H3z', cube: 'M5 5h14v14H5z',
  };

  // the brain's settle, on unit directions (one shell each): tree springs, region cohesion, relates,
  // repulsion on a shell, a fissure between the hemispheres; fixed-length and seeded by its start
  function settle(n, S, X, FF, RD, PI, CU, CK, HAS, RA, RB2, RK) {
    const F = new Float64Array(n * 3);
    const pull = (i, tx, ty, tz, k) => { F[i * 3] += (tx - X[i * 3]) * k; F[i * 3 + 1] += (ty - X[i * 3 + 1]) * k; F[i * 3 + 2] += (tz - X[i * 3 + 2]) * k; };
    const ITER = 240;
    for (let it = 0; it < ITER; it++) {
      const cool = 0.15 + 0.85 * (1 - it / ITER);
      F.fill(0);
      for (let i = 0; i < n; i++) {
        const p = PI[i];
        if (p >= 0) { pull(i, X[p * 3], X[p * 3 + 1], X[p * 3 + 2], 0.05); pull(p, X[i * 3], X[i * 3 + 1], X[i * 3 + 2], 0.015); }
        if (HAS[i]) pull(i, CU[i * 3], CU[i * 3 + 1], CU[i * 3 + 2], CK[i]);
      }
      for (let k = 0; k < RA.length; k++) {
        const a = RA[k], b = RB2[k];
        pull(a, X[b * 3], X[b * 3 + 1], X[b * 3 + 2], RK[k]); pull(b, X[a * 3], X[a * 3 + 1], X[a * 3 + 2], RK[k]);
      }
      // repulsion on a shell: long-range (so a shell spreads over the whole brain), stiff up close
      for (let i = 0; i < n; i++) {
        const fa = FF[i], ax = X[i * 3] * fa, ay = X[i * 3 + 1] * fa, az = X[i * 3 + 2] * fa;
        for (let j = i + 1; j < n; j++) {
          const fb = FF[j];
          if (Math.abs(fa - fb) > 0.18) continue;
          const dx = ax - X[j * 3] * fb, dy = ay - X[j * 3 + 1] * fb, dz = az - X[j * 3 + 2] * fb;
          const dd = Math.max(0.02, Math.sqrt(dx * dx + dy * dy + dz * dz)), want = (RD[i] + RD[j]) * 3 / S;
          const k = (0.004 * Math.min(4, (want / dd) * (want / dd)) + (dd < want ? 0.02 * (want - dd) / want : 0)) / Math.max(0.3, fa) / dd;
          F[i * 3] += dx * k; F[i * 3 + 1] += dy * k; F[i * 3 + 2] += dz * k; F[j * 3] -= dx * k; F[j * 3 + 1] -= dy * k; F[j * 3 + 2] -= dz * k;
        }
      }
      for (let i = 0; i < n; i++) {
        const x = X[i * 3], y = X[i * 3 + 1];
        // the fissure: nothing sits on the plane between the hemispheres; the brainstem's pole stays free
        const side = Math.sign(x || (HAS[i] ? CU[i * 3] : 1) || 1);
        if (Math.abs(x) < 0.16 && FF[i] > 0.3) F[i * 3] += side * 0.02;
        if (y < -0.8) F[i * 3 + 1] += 0.02;
        const nx = x + F[i * 3] * cool, ny = y + F[i * 3 + 1] * cool, nz = X[i * 3 + 2] + F[i * 3 + 2] * cool;
        const l = Math.hypot(nx, ny, nz) || 1;
        X[i * 3] = nx / l; X[i * 3 + 1] = ny / l; X[i * 3 + 2] = nz / l;
      }
    }
  }

  // ---------- shaders ----------
  const MESH_VS = `#version 300 es
  layout(location=0) in vec3 aP; layout(location=1) in vec3 aN;
  layout(location=2) in vec4 iA; layout(location=3) in vec4 iC; layout(location=4) in vec4 iQ;
  uniform mat4 uVP; uniform float uT; uniform vec2 uFog;
  out vec3 vN; out vec3 vW; out vec4 vC; out float vE; out float vFog;
  mat3 ry(float a) { float c = cos(a), s = sin(a); return mat3(c, 0.0, -s, 0.0, 1.0, 0.0, s, 0.0, c); }
  mat3 rx(float a) { float c = cos(a), s = sin(a); return mat3(1.0, 0.0, 0.0, 0.0, c, s, 0.0, -s, c); }
  void main() {
    mat3 R = ry(iQ.x) * rx(iQ.y);
    vec3 p = aP;
    if (iQ.w > 0.0) {
      float s = iA.x * 3.1 + iA.z * 1.7;
      p += aN * iQ.w * sin(aP.x * 3.0 + uT * 1.9 + s) * sin(aP.y * 2.6 + uT * 1.3 + s);
    }
    vec3 w = iA.xyz + R * (p * iA.w);
    vN = R * aN; vW = w; vC = iC; vE = iQ.z;
    vec4 c = uVP * vec4(w, 1.0);
    gl_Position = c;
    vFog = clamp((c.w - uFog.x) / max(uFog.y - uFog.x, 0.001), 0.0, 1.0);
  }`;
  // Lambert from a key light, a hemisphere fill, a rim, a small specular glint, and the emissive core
  const MESH_FS = `#version 300 es
  precision mediump float;
  in vec3 vN; in vec3 vW; in vec4 vC; in float vE; in float vFog;
  uniform vec3 uEye; uniform vec3 uL; uniform vec3 uGround; uniform vec4 uLook; uniform float uRimTo;
  out vec4 o;
  void main() {
    vec3 N = normalize(vN);
    if (!gl_FrontFacing) N = -N;
    vec3 V = normalize(uEye - vW);
    float lam = max(dot(N, uL), 0.0);
    float hemi = 0.5 + 0.5 * N.y;
    float rim = pow(1.0 - max(dot(N, V), 0.0), 2.2);
    float sp = pow(max(dot(N, normalize(uL + V)), 0.0), 40.0);
    vec3 base = vC.rgb;
    vec3 c = base * (uLook.x + uLook.y * lam + 0.2 * hemi) + base * vE;
    c += mix(base, vec3(1.0), uRimTo) * uLook.z * rim + vec3(sp * uLook.w);
    c = mix(uGround, c, vC.a);
    c = mix(c, uGround, vFog * 0.5);
    o = vec4(c, 1.0);
  }`;
  // sprites: a core disc (core > 0), a soft glow (core = 0), or a thin ring (core < 0, its radius)
  const POINT_VS = `#version 300 es
  in vec3 aPos; in vec4 aColor; in float aSize; in float aCore;
  uniform mat4 uVP; uniform float uScale; uniform vec2 uFog; uniform float uMax;
  out vec4 vColor; out float vCore;
  void main() {
    vec4 p = uVP * vec4(aPos, 1.0);
    gl_Position = p;
    gl_PointSize = clamp(aSize * uScale / max(p.w, 0.01), 1.0, uMax);
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
    float a;
    vec3 c = vColor.rgb;
    if (vCore < 0.0) {
      float R = -vCore, w = 0.09;
      a = exp(-pow((r - R) / w, 2.0)) + 0.25 * exp(-pow((r - R) / (w * 3.0), 2.0));
    } else if (vCore > 0.0) {
      float core = 1.0 - smoothstep(vCore - 0.12, vCore, r);
      float glow = exp(-r * r * 4.5) * (1.0 - core);
      c = mix(c, c * 1.2 + 0.1, core);
      a = clamp(core + glow * 0.6, 0.0, 1.0);
    } else {
      a = exp(-r * r * 4.5) * (1.0 - r * r);
    }
    a *= vColor.a;
    o = vec4(c * a, a);
  }`;
  // ribbons: screen-space quads between two points, tapering (a width and an alpha at each end),
  // thicker near the eye, with a bright core and an optional soft glow around it
  const LINE_VS = `#version 300 es
  in vec2 aCorner;
  in vec3 aA; in vec3 aB; in vec4 aCol; in vec4 aW;
  uniform mat4 uVP; uniform vec2 uView; uniform vec2 uFog; uniform float uRef;
  out vec4 vColor; out float vSide; out float vGlow;
  void main() {
    vec4 pa = uVP * vec4(aA, 1.0), pb = uVP * vec4(aB, 1.0);
    vec2 sa = pa.xy / pa.w, sb = pb.xy / pb.w;
    vec2 d = (sb - sa) * uView;
    vec2 dir = length(d) > 0.0001 ? normalize(d) : vec2(1.0, 0.0);
    vec2 n = vec2(-dir.y, dir.x);
    vec4 p = mix(pa, pb, aCorner.x);
    float w = mix(aW.x, aW.y, aCorner.x) * clamp(uRef / max(p.w, 0.01), 0.6, 1.8) * (1.0 + 1.6 * aW.w);
    p.xy += n * aCorner.y * w / uView * p.w;
    gl_Position = p;
    float fog = clamp((p.w - uFog.x) / max(uFog.y - uFog.x, 0.001), 0.0, 1.0);
    vColor = vec4(aCol.rgb, mix(aCol.a, aW.z, aCorner.x) * (1.0 - 0.55 * fog)); vSide = aCorner.y; vGlow = aW.w;
  }`;
  const LINE_FS = `#version 300 es
  precision mediump float;
  in vec4 vColor; in float vSide; in float vGlow;
  out vec4 o;
  void main() {
    float s = abs(vSide), f = 1.0 / (1.0 + 1.6 * vGlow);
    float core = 1.0 - smoothstep(f * 0.55, f, s);
    float halo = vGlow * 0.42 * exp(-s * s * 4.0);
    float a = vColor.a * clamp(core + halo, 0.0, 1.0);
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

  // ---------- colours: the tokens, through probes ----------
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
  const TONES = ['ink', 'muted', 'line', 'surface', 'ok', 'warn', 'danger', 'accent', 's1', 's2', 's3', 's4', 's5', 's6'];
  // a region's tint in the lane colouring, in turn (the series hues the ranks lean on least first)
  const REGION_TONES = ['accent', 's5', 's3', 's1', 's6', 's2', 's4'];
  // the css token each region tone reads (the legend's swatches name the token, never a colour)
  const TONE_VAR = { accent: '--accent', s1: '--series-1', s2: '--series-2', s3: '--series-3', s4: '--series-4', s5: '--series-5', s6: '--series-6' };
  const TINTS = ['lane', 'heat', 'health'];

  function create(stage, opts) {
    if (!supported()) return null;
    const canvas = document.createElement('canvas');
    canvas.className = 'net-canvas';
    canvas.tabIndex = 0;
    canvas.setAttribute('role', 'img');
    canvas.setAttribute('aria-label', 'the net in 3D, a brain: the orchestrator at the centre, each rank a shell further out, each lane a region, the skills a crown above — drag to orbit, wheel to zoom, click a neuron to focus it; arrow keys walk the tree and the camera follows, Enter pins, Shift and the arrows turn it, + and - zoom');
    canvas.setAttribute('aria-describedby', 'net-keys');
    const overlay = document.createElement('canvas');
    overlay.className = 'net-overlay';
    overlay.setAttribute('aria-hidden', 'true');
    const gl = canvas.getContext('webgl2', { antialias: true, premultipliedAlpha: true, alpha: true, depth: true });
    if (!gl) return null;
    const octx = overlay.getContext('2d');
    const el = (tag, cls, text) => { const e = document.createElement(tag); if (cls) e.className = cls; if (text) e.textContent = text; return e; };
    const btn = (text, title) => { const b = el('button', 'button', text); b.type = 'button'; b.dataset.size = 's'; b.dataset.variant = 'quiet'; if (title) b.title = title; return b; };
    // the view's own tools: the regions' colouring, the last turn's replay, home
    const tools = el('div', 'net-3d-tools');
    const tintBox = el('div', 'net-3d-tint');
    tintBox.setAttribute('role', 'group');
    tintBox.setAttribute('aria-label', 'colour the regions by');
    const tintBtns = TINTS.map(t => {
      const b = el('button', '', t);
      b.type = 'button'; b.dataset.tint = t;
      b.title = { lane: 'each region its own hue: which coordinator\'s subtree it is', heat: 'how much work each region did in four weeks', health: 'the value score of each region\'s ranked agents, banded by config (ui.meters.score)' }[t];
      tintBox.append(b);
      return b;
    });
    const replayBtn = btn('replay', 'replay the last turn\'s signals');
    replayBtn.disabled = true;
    const resetBtn = btn('reset view');
    tools.append(tintBox, replayBtn, resetBtn);
    // the tint's key, beside the switch: what the colour means now, and what a size means
    const legend = el('div', 'net-vkey net-3d-legend');
    legend.setAttribute('aria-live', 'polite');
    // the replay bar: play/pause, a scrubber, the time, close
    const bar = el('div', 'net-3d-replay');
    bar.hidden = true;
    const playBtn = btn('pause');
    const scrub = el('input');
    scrub.type = 'range'; scrub.min = '0'; scrub.step = '10'; scrub.value = '0';
    scrub.setAttribute('aria-label', 'the replay\'s moment');
    const clock = el('span', 'net-3d-clock', '0.0 s');
    const closeBtn = btn('close', 'back to live');
    bar.append(playBtn, scrub, clock, closeBtn);
    const key = el('div', 'net-3d-key');
    key.setAttribute('aria-hidden', 'true');
    const hint = el('p', 'net-3d-hint', 'drag to orbit · wheel or pinch to zoom · click a neuron to focus · arrows walk the tree');
    let seen = false;
    try { seen = localStorage.getItem('dash.net3d.seen') === '1'; } catch (_) { /* private mode */ }
    hint.hidden = seen;
    const touched = () => { if (hint.hidden) return; hint.hidden = true; try { localStorage.setItem('dash.net3d.seen', '1'); } catch (_) { /* private mode */ } };

    let MP, P, LP, quad, pbuf, lbuf, vaoP, vaoL, meshes, maxPoint = 256;
    const LS = 14; // ribbon instance: a3 b3 col4 (wA wB alphaB glow)
    function init() {
      MP = program(gl, MESH_VS, MESH_FS);
      P = program(gl, POINT_VS, POINT_FS);
      LP = program(gl, LINE_VS, LINE_FS);
      const range = gl.getParameter(gl.ALIASED_POINT_SIZE_RANGE);
      maxPoint = range ? Math.min(1024, range[1]) : 256;
      meshes = {};
      for (const [k, mk] of Object.entries(SHAPES)) {
        const data = mk(), vao = gl.createVertexArray(), vb = gl.createBuffer(), ib = gl.createBuffer();
        gl.bindVertexArray(vao);
        gl.bindBuffer(gl.ARRAY_BUFFER, vb);
        gl.bufferData(gl.ARRAY_BUFFER, data, gl.STATIC_DRAW);
        gl.enableVertexAttribArray(0); gl.vertexAttribPointer(0, 3, gl.FLOAT, false, 24, 0);
        gl.enableVertexAttribArray(1); gl.vertexAttribPointer(1, 3, gl.FLOAT, false, 24, 12);
        gl.bindBuffer(gl.ARRAY_BUFFER, ib);
        for (let i = 0; i < 3; i++) { gl.enableVertexAttribArray(2 + i); gl.vertexAttribPointer(2 + i, 4, gl.FLOAT, false, 48, i * 16); gl.vertexAttribDivisor(2 + i, 1); }
        gl.bindVertexArray(null);
        meshes[k] = { vao, ib, count: data.length / 6, inst: fbuf(12 * 16) };
      }
      pbuf = gl.createBuffer(); lbuf = gl.createBuffer(); quad = gl.createBuffer();
      gl.bindBuffer(gl.ARRAY_BUFFER, quad);
      gl.bufferData(gl.ARRAY_BUFFER, new Float32Array([0, -1, 1, -1, 1, 1, 0, -1, 1, 1, 0, 1]), gl.STATIC_DRAW);
      vaoP = gl.createVertexArray();
      gl.bindVertexArray(vaoP);
      gl.bindBuffer(gl.ARRAY_BUFFER, pbuf);
      const pl = n => gl.getAttribLocation(P, n);
      const PS = 10 * 4; // pos3 col4 size core (+ depth, for the light theme's sort)
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
      const inst = (n, size, off) => { const loc = ll(n); gl.enableVertexAttribArray(loc); gl.vertexAttribPointer(loc, size, gl.FLOAT, false, LS * 4, off); gl.vertexAttribDivisor(loc, 1); };
      inst('aA', 3, 0); inst('aB', 3, 12); inst('aCol', 4, 24); inst('aW', 4, 40);
      gl.bindVertexArray(null);
      U = {};
      for (const [name, p] of [['m', MP], ['p', P], ['l', LP]]) {
        const n = gl.getProgramParameter(p, gl.ACTIVE_UNIFORMS);
        for (let i = 0; i < n; i++) { const u = gl.getActiveUniform(p, i).name; U[name + u] = gl.getUniformLocation(p, u); }
      }
    }
    let U = {};
    init();
    const RB = fbuf(14 * 4096), SB = fbuf(10 * 2048), SB2 = fbuf(10 * 2048);

    const st = {
      m: null, nodes: new Map(), edges: [], holds: [], relates: [], regions: [], crown: null, focus: '', shown: '', lit: null, live: new Set(), hover: '',
      yaw: 0.62, pitch: 0.26, dist: 10, dist0: 10, target: [0, 0, 0], goal: { target: [0, 0, 0], dist: 10 },
      run: false, raf: 0, last: 0, t: 0, spinT: 0, idleAt: performance.now(), pulses: [], flashes: new Map(), lights: new Map(), pal: null, dark: true, W: 1, H: 1, dpr: 1, VP: null,
      slow: 0, busy: new Set(), S: 4, A: [4, 3, 4], font: null, stars: null, dendrites: [], settle: 1, settleAt: 0, sig: '',
      tint: 'lane', hoverRel: null, rec: null, replay: null, eye: [0, 0, 10],
    };
    try { const t = localStorage.getItem('dash.net3d.tint'); if (TINTS.includes(t)) st.tint = t; } catch (_) { /* private mode */ }
    const setTint = t => {
      st.tint = t;
      for (const b of tintBtns) b.setAttribute('aria-pressed', String(b.dataset.tint === t));
      try { localStorage.setItem('dash.net3d.tint', t); } catch (_) { /* private mode */ }
      regionTints(); legendFill(); kick();
      if (!st.run || opts.reduced()) render(performance.now(), true);
    };
    // the key: lane — each region's swatch and its head; heat — the cold→hot ramp over four weeks;
    // health — the score bands config sets (ui.meters.score); and always, what a size is
    function legendFill() {
      const sw = (attrs, text, title) => { const sp = el('span'); const i = el('i'); for (const [k, v] of Object.entries(attrs)) i.setAttribute(k, v); sp.append(i, text); if (title) sp.title = title; return sp; };
      const items = [];
      if (st.tint === 'lane') {
        for (const r of (st.regions || []).slice(0, 6)) items.push(sw({ 'data-swatch': 'lane', style: '--net-swatch: var(' + TONE_VAR[REGION_TONES[r.i % REGION_TONES.length]] + ')' }, r.caption.toLowerCase(), 'the region under ' + r.caption.toLowerCase()));
        if (!items.length) items.push(el('span', '', 'one region per coordinator'));
      } else if (st.tint === 'heat') {
        items.push(sw({ 'data-swatch': 'heat' }, 'cold → hot', 'runs over the last four weeks'));
        items.push(el('span', '', 'work, 4 weeks'));
      } else {
        const sm = ((opts.meters && opts.meters()) || {}).score || { bands: { ok: 70, warn: 40 } };
        const b = sm.bands || { ok: 70, warn: 40 };
        items.push(sw({ 'data-band': 'ok' }, 'score ≥ ' + b.ok), sw({ 'data-band': 'warn' }, '≥ ' + b.warn), sw({ 'data-band': 'bad' }, '< ' + b.warn));
      }
      items.push(el('span', '', 'size = code owned (lines)'));
      legend.replaceChildren(...items);
    }
    for (const b of tintBtns) b.addEventListener('click', () => setTint(b.dataset.tint));
    for (const b of tintBtns) b.setAttribute('aria-pressed', String(b.dataset.tint === st.tint));

    // ---------- the scene: a brain ----------
    function scene(m) {
      const prev = st.nodes;
      st.m = m;
      st.nodes = new Map();
      const D = Math.max(1, m.maxDepth);
      const orchestratorId = m.orchestrator ? m.orchestrator.id : '';
      const members = m.order.filter(id => id !== orchestratorId);
      const N = members.length;
      const S = clamp(1.7 + Math.sqrt(N) * 0.55, 2.6, 6.8);
      const A = [S * 1.2, S * 0.86, S];
      st.S = S; st.A = A;
      // the rank's shell, as a fraction of the brain: the orchestrator at the centre, a first-rank member
      // with nobody below it close by, then each rank one shell further out
      const kidsOf = id => m.kids.get(id) || [];
      const shell = id => {
        const d = m.depth.get(id) || 0;
        if (id === orchestratorId) return 0;
        if (D === 1) return 0.9;
        if (d === 1 && !kidsOf(id).length) return 0.3;
        return 0.46 + 0.5 * (d - 1) / (D - 1);
      };
      // a neuron is small, sized by the code it owns (the model's massK, 0..1 over the workspace): a leaf
      // on the square root of its own lines, a member with a subtree on the cube root of its
      // subtree's, the orchestrator capped so it never swallows the scene; the largest leaf is 2.5× the
      // smallest. Activity never moves a size (it glows and breathes instead).
      const mk = id => (m.sized && m.massK && m.massK.has(id)) ? m.massK.get(id) : 0.5;
      const radius = n => n.orchestrator ? 0.3 : kidsOf(n.id).length ? 0.15 * (1 + 0.8 * mk(n.id)) : 0.1 * (1 + 1.5 * mk(n.id));
      // regions: the orchestrator's children with a subtree; the rest sit near the centre
      const regionOf = new Map(), heads = [], small = [];
      for (const k of kidsOf(orchestratorId)) (kidsOf(k).length ? heads : small).push(k);
      const leafCount = id => { const ks = kidsOf(id); return ks.length ? ks.reduce((a, k) => a + leafCount(k), 0) : 1; };
      const mark = (id, r) => { regionOf.set(id, r); for (const k of kidsOf(id)) mark(k, r); };
      for (const h of heads) mark(h, h);
      for (const s of small) mark(s, '');
      for (const id of members) if (!regionOf.has(id)) regionOf.set(id, '');
      // each region a sector around the vertical axis, as wide as its leaves; regions alternate a
      // little up and down, so lobes stack the way a brain's do
      const total = heads.reduce((a, h) => a + leafCount(h), 0) || 1;
      const centre = new Map();
      let acc = 0;
      const ELEV = [0.5, 0.12, 0.62, 0.28, 0.72, 0.05];
      heads.forEach((h, i) => {
        const w = leafCount(h) / total * TAU, th = acc + w / 2 + 0.35;
        acc += w;
        const el = ELEV[i % ELEV.length];
        centre.set(h, { th, el, w, u: [Math.cos(el) * Math.cos(th), Math.sin(el), Math.cos(el) * Math.sin(th)] });
      });
      // the start: the tidy-tree angle within the region, a seeded elevation
      const leaves = [], slot = new Map();
      const place = id => { const ks = kidsOf(id); if (!ks.length) { slot.set(id, leaves.length); leaves.push(id); return; } ks.forEach(place); slot.set(id, ks.reduce((a, k) => a + slot.get(k), 0) / ks.length); };
      heads.forEach(place);
      const u = new Map(), f = new Map(), rad = new Map();
      for (const id of members) {
        const n = m.byId.get(id), d = m.depth.get(id) || 1;
        f.set(id, shell(id)); rad.set(id, radius(n));
        const r = regionOf.get(id), c = centre.get(r);
        let th, el;
        if (c) {
          const lo = slot.has(id) ? slot.get(id) : 0, ls = leafCount(r);
          const first = slot.get(leaves.find(x => regionOf.get(x) === r)) || 0;
          th = c.th - c.w / 2 + ((lo - first + 0.5) / Math.max(1, ls)) * c.w;
          el = c.el + (d >= 2 ? (hash(id) - 0.5) * 1.4 : 0);
        } else {
          const i = small.indexOf(id);
          th = (i + 0.5) / Math.max(1, small.length) * TAU + 1.1; el = -0.75 + hash(id) * 0.3;
        }
        u.set(id, [Math.cos(el) * Math.cos(th), Math.sin(el), Math.cos(el) * Math.sin(th)]);
      }
      // relatedness: the team's relates, and work done together (days both worked, runs in the
      // same minute of the last turns)
      const rel = [];
      const relSeen = new Map();
      const addRel = (a, b, w, why) => {
        if (!u.has(a) || !u.has(b) || a === b) return;
        const k = a < b ? a + '|' + b : b + '|' + a;
        if (relSeen.has(k)) { const r = relSeen.get(k); r.w = Math.min(1, r.w + w); Object.assign(r.why, why); return; }
        const r = { a, b, w, why: { ...why } };
        relSeen.set(k, r); rel.push(r);
      };
      const idByName = new Map();
      for (const [id, n] of m.byId) idByName.set(n.name, id);
      for (const r of m.relates || []) addRel(r.from, r.to, r.weight || 0, { imports: r.imports || 0, skills: r.skills || 0 });
      const act = m.act || {};
      const busyDays = id => { const a = act[m.byId.get(id).name]; return a && a.runs ? a.runs.map(x => x > 0) : null; };
      const leafIds = members.filter(id => (m.depth.get(id) || 0) >= 2);
      const days = new Map(leafIds.map(id => [id, busyDays(id)]));
      for (let i = 0; i < leafIds.length; i++) for (let j = i + 1; j < leafIds.length; j++) {
        const a = days.get(leafIds[i]), b = days.get(leafIds[j]);
        if (!a || !b) continue;
        let both = 0, any = 0;
        for (let k = 0; k < Math.min(a.length, b.length); k++) { if (a[k] && b[k]) both++; if (a[k] || b[k]) any++; }
        if (both >= 2 && any && !kin(leafIds[i], leafIds[j])) addRel(leafIds[i], leafIds[j], 0.35 * both / any, { days: both });
      }
      function kin(a, b) { for (const [x, y] of [[a, b], [b, a]]) for (let p = m.parent.get(x), g = 0; p && g < 64; p = m.parent.get(p), g++) if (p === y) return true; return false; }
      const recent = (m.recent || []).map(r => ({ id: idByName.get(r.as), at: Date.parse(r.at) })).filter(r => r.id && isFinite(r.at)).sort((a, b) => a.at - b.at);
      for (let i = 0; i < recent.length; i++) for (let j = i + 1; j < recent.length && recent[j].at - recent[i].at < 120000; j++) if (!kin(recent[i].id, recent[j].id)) addRel(recent[i].id, recent[j].id, 0.2, { turns: 1 });
      // the settle: tree springs, region cohesion, relates, repulsion on the shell, a fissure between
      // the hemispheres; seeded and fixed-length, so a workspace always lands the same way
      // (typed arrays, no allocation per step: 60 members settle in a few milliseconds); a workspace
      // whose shape and relates are unchanged keeps the layout it had
      const ids = members, n = ids.length, ix = new Map(ids.map((id, i) => [id, i]));
      const memo = ids.map(id => id + '<' + (m.parent.get(id) || '')).join(',') + '|' + rel.map(r => r.a + '~' + r.b + ':' + r.w.toFixed(2)).join(',');
      if (st.layout && st.layout.memo === memo) for (const id of ids) u.set(id, st.layout.u.get(id));
      else {
        const X = new Float64Array(n * 3), FF = new Float64Array(n), RD = new Float64Array(n), PI = new Int32Array(n).fill(-1), CU = new Float64Array(n * 3), CK = new Float64Array(n), HAS = new Uint8Array(n);
        ids.forEach((id, i) => {
          const q = u.get(id); X[i * 3] = q[0]; X[i * 3 + 1] = q[1]; X[i * 3 + 2] = q[2];
          FF[i] = f.get(id); RD[i] = rad.get(id);
          const p = m.parent.get(id);
          if (p && p !== orchestratorId && ix.has(p)) PI[i] = ix.get(p);
          const c = centre.get(regionOf.get(id));
          if (c) { HAS[i] = 1; CU[i * 3] = c.u[0]; CU[i * 3 + 1] = c.u[1]; CU[i * 3 + 2] = c.u[2]; CK[i] = (m.depth.get(id) || 0) === 1 ? 0.12 : 0.025; }
        });
        const RA = new Int32Array(rel.length), RB2 = new Int32Array(rel.length), RK = new Float64Array(rel.length);
        rel.forEach((r, k) => { RA[k] = ix.get(r.a); RB2[k] = ix.get(r.b); RK[k] = (regionOf.get(r.a) === regionOf.get(r.b) ? 0.04 : 0.015) * r.w; });
        settle(n, S, X, FF, RD, PI, CU, CK, HAS, RA, RB2, RK);
        ids.forEach((id, i) => u.set(id, [X[i * 3], X[i * 3 + 1], X[i * 3 + 2]]));
        st.layout = { memo, u: new Map(u) };
      }
      const workspace = id => { const q = u.get(id), k = f.get(id); return [q[0] * k * A[0], q[1] * k * A[1], q[2] * k * A[2]]; };
      if (orchestratorId) {
        const n = m.byId.get(orchestratorId);
        st.nodes.set(orchestratorId, { id: orchestratorId, n, p1: [0, 0, 0], d: 0, rad: radius(n), rank: n.rank || 'orchestrator', family: n.family || '', region: '', form: formOf(n), seed: hash(orchestratorId) });
      }
      for (const id of members) {
        const n = m.byId.get(id);
        st.nodes.set(id, { id, n, p1: workspace(id), d: m.depth.get(id) || 1, rad: rad.get(id), rank: n.rank || 'plain', family: n.family || '', region: regionOf.get(id), form: formOf(n), seed: hash(id), u: u.get(id), f: f.get(id) });
      }
      // the skills: a crown above the brain, each over the middle of its holders, spread apart
      st.crown = null;
      if (m.skills.length) {
        const my = A[1] * 0.92 + S * 0.42, mr = A[0] * 0.6;
        const want = m.skills.map(n => {
          let sx = 0, sz = 0, k = 0;
          for (const w of m.holds) if (w.to === n.id && st.nodes.has(w.from)) { const q = st.nodes.get(w.from).p1; if (Math.hypot(q[0], q[2]) > 0.01) { sx += q[0] / A[0]; sz += q[2] / A[2]; k++; } }
          return { n, a: k && Math.hypot(sx, sz) > 0.01 ? Math.atan2(sz, sx) : hash(n.id) * TAU };
        }).sort((x, y) => x.a - y.a);
        const gap = TAU / want.length * 0.72;
        for (let pass = 0; pass < 16; pass++) for (let i = 0; i < want.length; i++) {
          const a = want[i], b = want[(i + 1) % want.length], d = ((b.a - a.a) % TAU + TAU) % TAU;
          if (want.length > 1 && d < gap) { a.a -= (gap - d) / 2; b.a += (gap - d) / 2; }
        }
        want.forEach(w => {
          const lane = w.n.lane || 'skill:shared';
          st.nodes.set(w.n.id, { id: w.n.id, n: w.n, p1: [Math.cos(w.a) * mr, my + Math.sin(w.a * 3) * S * 0.05, Math.sin(w.a) * mr], d: -1, rad: 0.12 * (1 + mk(w.n.id)), rank: lane, region: '', form: ['skill', 0], seed: hash(w.n.id) });
        });
        st.crown = { y: my, r: mr, caption: String(opts.word ? opts.word('skills', 'skills') : 'skills').toUpperCase() + '  ' + m.skills.length };
      }
      // the settle animates once per shape of the workspace (a reload of the same workspace stays put)
      const sig = [...st.nodes.keys()].sort().join(',');
      const again = sig === st.sig;
      st.sig = sig;
      for (const q of st.nodes.values()) {
        const was = prev.get(q.id);
        q.p0 = again && was ? was.p.slice() : q.d === -1 ? [q.p1[0] * 0.2, q.p1[1], q.p1[2] * 0.2] : scl(q.p1, 0.08);
        q.p = (again || opts.reduced()) ? q.p1.slice() : q.p0.slice();
      }
      st.settle = again || opts.reduced() ? 1 : 0;
      st.settleAt = performance.now();
      // bonds
      const maxW = Math.max(1, ...m.tree.map(t => t.weight));
      st.edges = m.tree.filter(t => st.nodes.has(t.up) && st.nodes.has(t.down)).map((t, i) => ({ ...t, key: t.up + '>' + t.down, k: t.weight ? Math.log2(1 + t.weight) / Math.log2(1 + maxW) : 0, seed: (i * 0.6180339) % 1 }));
      st.holds = m.holds.filter(w => st.nodes.has(w.from) && st.nodes.has(w.to)).map((w, i) => ({ ...w, key: w.from + '>' + w.to, seed: (i * 0.381966) % 1 }));
      st.relates = rel.filter(r => r.w >= 0.12).sort((a, b) => b.w - a.w).slice(0, Math.max(24, N * 2)).map(r => ({ ...r, key: r.a + '~' + r.b }));
      // regions: their members, a caption (the head's own word), a tint
      st.regions = heads.map((h, i) => ({ id: h, i, members: members.filter(id => regionOf.get(id) === h), caption: shortName(m.byId.get(h)).toUpperCase() }));
      const word = opts.word || ((k, d) => d || k);
      // dendrites: a few short forked branches off each outer neuron, pointing away from the centre
      st.dendrites = [];
      for (const q of st.nodes.values()) {
        if (q.d < 2) continue;
        const out = unit(q.p1), side = unit(cross(out, [0, 1, 0])), up = unit(cross(side, out));
        const nb = 2 + Math.floor(hash(q.id, 7) * 3);
        for (let b = 0; b < nb; b++) {
          const a = hash(q.id, 11 + b) * TAU, tilt = 0.55 + hash(q.id, 23 + b) * 0.5;
          const dir = unit(add(scl(out, Math.cos(tilt)), scl(add(scl(side, Math.cos(a)), scl(up, Math.sin(a))), Math.sin(tilt))));
          const L = q.rad * (2.2 + hash(q.id, 31 + b) * 1.6);
          const mid = scl(dir, L * 0.6), fork = unit(add(dir, scl(side, 0.5 * (hash(q.id, 41 + b) - 0.5))));
          const fork2 = unit(add(dir, scl(up, 0.9 * (hash(q.id, 43 + b) - 0.5))));
          st.dendrites.push({ id: q.id, segs: [[scl(dir, q.rad * 0.8), mid, 1, 0.55], [mid, add(mid, scl(fork, L * 0.45)), 0.55, 0], [mid, add(mid, scl(fork2, L * 0.38)), 0.5, 0]] });
        }
      }
      // depth: a slow field of faint motes around the brain
      if (!st.stars || st.stars.S !== S) {
        const pts = [];
        for (let i = 0; i < 220; i++) {
          const a = hash('s' + i, 1) * TAU, z = hash('s' + i, 2) * 2 - 1, r = S * (1.9 + hash('s' + i, 3) * 2.6), h = Math.sqrt(1 - z * z);
          pts.push([Math.cos(a) * h * r, z * r * 0.7, Math.sin(a) * h * r, 0.04 + hash('s' + i, 4) * 0.08, hash('s' + i, 5)]);
        }
        st.stars = { S, pts };
      }
      // the key: the forms present, in the lexicon's rank words
      const seenForms = [];
      for (const q of st.nodes.values()) {
        const k = q.d === -1 ? 'skill' : q.form[0];
        const label = q.d === -1 ? word('skills', 'skills') : word('rank.' + q.n.rank, q.n.rank || 'agent') + (q.form[1] ? ' ◌' : '');
        const have = seenForms.find(s => s.label === label);
        if (have) have.n++;
        else seenForms.push({ k, label, rank: q.d === -1 ? 'skill:shared' : q.n.rank || 'plain', crown: q.form[1], d: q.d, n: 1 });
      }
      seenForms.sort((a, b) => (a.d < 0 ? 99 : a.d) - (b.d < 0 ? 99 : b.d));
      key.replaceChildren(...seenForms.map(s => {
        const sp = el('span');
        sp.dataset.rank = s.rank;
        // a skill's key is the board's brain (Lucide brain-circuit, the sprite); a rank's its solid
        sp.innerHTML = s.k === 'skill' ? '<svg viewBox="0 0 24 24" aria-hidden="true"><use href="web/icons/icons.svg#skill"/></svg>'
          : `<svg viewBox="0 0 24 24" aria-hidden="true"><path d="${GLYPH[s.k] || GLYPH.cube}"/>${s.crown ? '<ellipse cx="12" cy="6" rx="8" ry="2.2"/>' : ''}</svg>`;
        sp.append(s.label.replace(' ◌', ''));
        const b = el('b', '', String(s.n));
        sp.append(b);
        if (s.crown) sp.title = 'a promoted rank holds a crown ring';
        return sp;
      }));
      home();
      st.idleAt = performance.now();
      palette();
      kick();
    }
    const shortName = n => n.orchestrator ? (n.label || n.name) : n.kind === 'skill' ? n.name : (n.rank && n.name.startsWith(n.rank + '-') ? n.name.slice(n.rank.length + 1) : n.name);
    // the camera's home: the whole brain and its crown inside the view
    function home() {
      let lo = Infinity, hi = -Infinity;
      for (const q of st.nodes.values()) { lo = Math.min(lo, q.p1[1] - q.rad); hi = Math.max(hi, q.p1[1] + q.rad); }
      if (!isFinite(lo)) { lo = -1; hi = 1; }
      st.home = [0, (lo + hi) / 2, 0];
      st.halfH = (hi - lo) / 2;
      fitDist();
      if (!st.focus) { st.goal.dist = st.dist0; st.dist = st.dist0; st.goal.target = st.home.slice(); st.target = st.home.slice(); }
    }
    function fitDist() {
      const t = Math.tan(0.36), aspect = st.W / Math.max(1, st.H);
      const v = (st.halfH || 1) * Math.cos(st.pitch) + st.A[2] * Math.sin(st.pitch) * 0.7 + 0.3, h = st.A[0] * 1.02 + 0.3;
      st.dist0 = Math.max(v / t, h / (t * aspect)) + st.A[2] * 0.2;
      if (!st.focus && !st.zoomed) { st.goal.dist = st.dist0; st.dist = st.dist0; }
    }
    // the curves: a synapse bows a little outward; a relates bond hugs the shell it joins; a held
    // skill's filament falls from the crown and enters its holder from outside
    function quad3(A, C, B, t) { const u = 1 - t; return [u * u * A[0] + 2 * u * t * C[0] + t * t * B[0], u * u * A[1] + 2 * u * t * C[1] + t * t * B[1], u * u * A[2] + 2 * u * t * C[2] + t * t * B[2]]; }
    function cubic(A, B, C, D, t) { const u = 1 - t, a = u * u * u, b = 3 * u * u * t, c = 3 * u * t * t, d = t * t * t; return [a * A[0] + b * B[0] + c * C[0] + d * D[0], a * A[1] + b * B[1] + c * C[1] + d * D[1], a * A[2] + b * B[2] + c * C[2] + d * D[2]]; }
    function synapse(e) {
      const A = st.nodes.get(e.up).p, B = st.nodes.get(e.down).p, mid = scl(add(A, B), 0.5);
      const C = len(mid) > 0.01 ? add(mid, scl(unit(mid), len(sub(B, A)) * 0.12)) : add(mid, [0, 0.2, 0]);
      return t => quad3(A, C, B, t);
    }
    function relCurve(r) {
      const A = st.nodes.get(r.a).p, B = st.nodes.get(r.b).p, mid = scl(add(A, B), 0.5), R = (len(A) + len(B)) / 2;
      const C = len(mid) > 0.01 ? scl(unit(mid), R * 1.1 + len(sub(A, B)) * 0.08) : add(mid, [0, 0.3, 0]);
      return t => quad3(A, C, B, t);
    }
    function holdCurve(w) {
      const M = st.nodes.get(w.to).p, W = st.nodes.get(w.from).p, h = M[1] - W[1];
      const out = Math.hypot(W[0], W[2]) > 0.05 ? unit([W[0], 0, W[2]]) : unit([M[0], 0, M[2]]);
      const P1 = [M[0] * 1.05, M[1] - h * 0.42, M[2] * 1.05];
      const P2 = add(W, add(scl(out, st.S * 0.38), [0, h * 0.22, 0]));
      return t => cubic(M, P1, P2, W, t);
    }

    // ---------- the theme ----------
    function palette() {
      const probes = opts.probes;
      const ranks = new Set(), famOf = new Map();
      for (const q of st.nodes.values()) { ranks.add(q.rank); if (q.family) famOf.set(q.rank, q.family); }
      probes.replaceChildren();
      const read = (attr, v) => { const sp = document.createElement('span'); sp.className = 'net-probe'; sp.setAttribute(attr, v); probes.append(sp); return sp; };
      const els = {};
      for (const t of TONES) els['t:' + t] = read('data-tone', t);
      // a rank with no hue of its own reads its family's (the probe carries both)
      for (const r of ranks) { els['r:' + r] = read('data-rank', r); if (famOf.has(r)) els['r:' + r].setAttribute('data-family', famOf.get(r)); }
      const pal = { rank: {} };
      for (const t of TONES) pal[t] = rgb(getComputedStyle(els['t:' + t]).color);
      for (const r of ranks) pal.rank[r] = rgb(getComputedStyle(els['r:' + r]).color);
      // a shared skill is drawn in the accent: its filaments must read, and grey reads as "off"
      if (pal.rank['skill:shared']) pal.rank['skill:shared'] = pal.accent;
      const s = pal.surface;
      st.dark = 0.2126 * s[0] + 0.7152 * s[1] + 0.0722 * s[2] < 0.5;
      st.pal = pal;
      regionTints();
      legendFill();
      fontCache();
      kick();
    }
    // the regions' tint: their own hue, the work they did, or their ranked agents' value score
    function regionTints() {
      const pal = st.pal, m = st.m;
      if (!pal || !m) return;
      const act = m.act || {};
      const heat = st.regions.map(r => r.members.reduce((a, id) => a + ((act[m.byId.get(id).name] || {}).total || 0), 0));
      const top = Math.max(1, ...heat);
      st.regions.forEach((r, i) => {
        if (st.tint === 'heat') { const h = heat[i] / top; r.tint = mix3(pal.muted, pal.warn, h); r.k = 0.35 + 0.9 * h; r.note = heat[i] + ' runs in 4 weeks'; }
        else if (st.tint === 'health') {
          // the value score of the region's ranked agents, banded by the score meter (config)
          let sum = 0, n = 0;
          for (const id of r.members) { const v = m.byId.get(id).value; if (v && v.confidence === 'ok') { sum += v.score; n++; } }
          const sm = ((opts.meters && opts.meters()) || {}).score || { bands: { ok: 70, warn: 40 } };
          const mean = n ? sum / n : 0, b = (sm.bands || {});
          const band = !n ? 'none' : mean >= b.ok ? 'ok' : mean >= b.warn ? 'warn' : 'bad';
          r.tint = band === 'ok' ? pal.ok : band === 'warn' ? pal.warn : band === 'bad' ? pal.danger : pal.muted; r.k = n ? 1 : 0.4;
          r.note = n ? 'score ' + Math.round(mean) + ' · ' + n + ' ranked' : 'no ranked agent';
        } else { r.tint = pal[REGION_TONES[i % REGION_TONES.length]]; r.k = 1; r.note = ''; }
      });
    }
    function fontCache() {
      if (!st.pal) return;
      const fam = getComputedStyle(stage).getPropertyValue('--font-body').trim() || 'system-ui';
      const px = parseFloat(getComputedStyle(document.documentElement).fontSize) || 16;
      const css = c => 'rgb(' + c.map(v => Math.round(v * 255)).join(',') + ')';
      st.font = {
        px, fam, ink: css(st.pal.ink), muted: css(st.pal.muted), halo: css(st.pal.surface), surface: st.pal.surface, line: css(st.pal.line), css,
        orchestrator: '650 ' + px * 1.05 + 'px ' + fam, first: '600 ' + px * 0.8 + 'px ' + fam, rest: px * 0.78 + 'px ' + fam, skill: px * 0.72 + 'px ' + fam,
        caption: '650 ' + px * 0.62 + 'px ' + fam, region: '700 ' + px * 0.8 + 'px ' + fam, tip: px * 0.78 + 'px ' + fam,
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
      st.eye = eye;
      const proj = perspective(0.72, st.W / st.H, 0.1, 400);
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
      const rp = st.replay;
      const busy = st.drag || st.pulses.length || st.flashes.size || st.lights.size || st.busy.size || st.settle < 1 || (rp && rp.playing) || Math.abs(st.dist - st.goal.dist) > 0.01 || Math.hypot(st.target[0] - st.goal.target[0], st.target[1] - st.goal.target[1], st.target[2] - st.goal.target[2]) > 0.01;
      const ambient = !reduced;
      // the budget: 60 a second while something moves for a reason, 30 while it only breathes,
      // 12 once nobody has touched it for ten seconds; the turn and the spin stop after an idle minute
      const idle = now - st.idleAt;
      const budget = busy ? 15 : idle > 10000 ? 83 : 32;
      if (st.last && now - st.last < budget) { schedule(); return; }
      const dt = st.last ? Math.min(0.1, (now - st.last) / 1000) : 0.016;
      const spent = performance.now();
      st.last = now;
      st.t += dt;
      const calm = idle < 60000 || st.busy.size || st.live.size;
      if (ambient && calm) st.spinT += dt;
      if (ambient && !st.drag && !st.focus && idle > 3500 && idle < 60000) st.yaw += dt * 0.06;
      const k = 1 - Math.pow(0.0015, dt);
      st.dist += (st.goal.dist - st.dist) * k;
      for (let i = 0; i < 3; i++) st.target[i] += (st.goal.target[i] - st.target[i]) * k;
      if (st.settle < 1) {
        st.settle = clamp((now - st.settleAt) / 1800, 0, 1);
        const e = 1 - Math.pow(1 - st.settle, 3);
        for (const q of st.nodes.values()) for (let i = 0; i < 3; i++) q.p[i] = lerp(q.p0[i], q.p1[i], e);
      }
      if (rp && rp.playing) {
        rp.t = Math.min(rp.dur, rp.t + dt * 1000);
        if (rp.t >= rp.dur) { rp.playing = false; playBtn.textContent = 'play'; }
        scrub.value = String(Math.round(rp.t));
        clock.textContent = (rp.t / 1000).toFixed(1) + ' s';
      }
      render(now, reduced);
      st.slow = st.slow * 0.9 + (performance.now() - spent) * 0.1;
      if (ambient || busy) schedule();
    }
    function schedule() { if (st.run && !st.raf) st.raf = requestAnimationFrame(frame); }
    function kick() { if (st.run) { if (!st.raf) st.raf = requestAnimationFrame(frame); } }

    // a signal's state at a moment: which hop it is on and how far along, or done
    const ease = u => u < 0.5 ? 2 * u * u : 1 - Math.pow(-2 * u + 2, 2) / 2;
    function render(now, reduced) {
      if (!st.pal || !st.m) return;
      camera();
      const pal = st.pal, dark = st.dark, lit = st.lit, focus = st.shown || '';
      const cw = Math.round(st.W * st.dpr), ch = Math.round(st.H * st.dpr);
      gl.viewport(0, 0, cw, ch);
      gl.clearColor(0, 0, 0, 0);
      gl.clearDepth(1);
      gl.clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT);
      const fog = [Math.max(0.1, st.dist - st.S * 0.9), st.dist + st.S * 1.6];
      const dim = id => focus && lit && !lit.nodes.has(id);
      const col = r => pal.rank[r] || pal.muted;
      const T = reduced ? 0 : st.t;
      // the moment the signals are drawn at: now, or the replay's own clock
      const rp = st.replay;
      const sigs = rp ? rp.pulses : st.pulses, sigNow = rp ? rp.t : now;
      const flashes = rp ? rp.flashes.filter(f => f.at <= rp.t && rp.t - f.at < (f.kind === 'read' ? 700 : 1100)) : [...st.flashes.values()];
      // synapses lit by a passing signal: the stretch behind its head glows, then fades
      const glowOn = new Map();
      const signalAt = pl => {
        const t = (sigNow - pl.at) / (pl.per * pl.hops.length);
        if (t < 0 || t >= 1) return null;
        const h = Math.min(pl.hops.length - 1, Math.floor(t * pl.hops.length));
        return { h, lt: t * pl.hops.length - h };
      };
      for (const pl of sigs) {
        const s = signalAt(pl);
        const c = pl.tone === 'down' ? col((st.nodes.get(pl.end) || {}).rank) : pal[pl.tone] || pal.ok;
        pl.hops.forEach(([up, down], i) => {
          const keyE = up + '>' + down;
          let v = 0;
          const end = pl.at + pl.per * (i + 1);
          if (s && i === s.h) v = { head: ease(s.lt), dir: pl.dir };
          else if (sigNow > end && sigNow - end < 1400) v = { head: 1, dir: pl.dir, fade: 1 - (sigNow - end) / 1400 };
          else if (s && ((pl.dir === 'down' && i < s.h) || (pl.dir === 'up' && i < s.h))) v = { head: 1, dir: pl.dir, fade: 1 };
          if (v) glowOn.set(keyE, { ...v, c });
        });
      }
      // ---- the solids ----
      for (const k in meshes) meshes[k].inst.n = 0;
      const putMesh = (kind, p, s, c, litK, spin, tilt, emis, wob) => {
        const b = meshes[kind].inst; b.need(12); const a = b.a; let i = b.n;
        a[i++] = p[0]; a[i++] = p[1]; a[i++] = p[2]; a[i++] = s; a[i++] = c[0]; a[i++] = c[1]; a[i++] = c[2]; a[i++] = litK; a[i++] = spin; a[i++] = tilt; a[i++] = emis; a[i++] = wob; b.n = i;
      };
      const breath = reduced ? 0 : Math.sin(st.t * 2.4);
      const fl = new Map();
      for (const f of flashes) { const age = (rp ? rp.t : now) - f.at; fl.set(f.id, Math.max(fl.get(f.id) || 0, clamp(1 - age / 600, 0, 1))); }
      for (const q of st.nodes.values()) {
        const c = col(q.rank), d = dim(q.id), on = q.id === focus;
        const liveN = st.live.has(q.id) || st.busy.has(q.id);
        const fire = fl.get(q.id) || 0;
        const s = q.rad * (1 + (liveN ? 0.07 * breath : 0) + (on ? 0.12 : 0) + 0.25 * fire);
        const emis = (dark ? 0.32 : 0.12) + (liveN ? (dark ? 0.35 : 0.15) * (0.6 + 0.4 * breath) : 0) + (on ? 0.25 : 0) + fire * (dark ? 1.4 : 0.6);
        const spin = q.seed * TAU + st.spinT * (q.form[0] === 'star' ? 0.22 : q.form[0] === 'skill' ? 0.35 : q.form[0] === 'zone' ? 0.1 : 0.3) * (q.seed > 0.5 ? 1 : -1);
        const kind = q.form[0];
        const tilt = kind === 'skill' ? 1.05 + q.seed * 0.3 : kind === 'cube' || kind === 'domain' ? 0.18 : 0;
        putMesh(kind, q.p, s, c, d ? 0.16 : 1, spin, tilt, emis, kind === 'zone' && !reduced ? 0.07 : 0);
        if (kind === 'star') {
          putMesh('crown', q.p, s * 1.3, c, d ? 0.16 : 1, st.spinT * 0.5, 1.15, emis * 0.8, 0);
          putMesh('crown', q.p, s * 1.5, c, d ? 0.16 : 1, -st.spinT * 0.35 + 1.4, -1.2, emis * 0.8, 0);
        } else if (q.form[1]) {
          putMesh('crown', [q.p[0], q.p[1] + s * 1.05, q.p[2]], s * 0.62, c, d ? 0.16 : 1, st.spinT * 0.6, 0.12, emis + 0.2, 0);
        }
      }
      gl.enable(gl.DEPTH_TEST);
      gl.depthFunc(gl.LEQUAL);
      gl.depthMask(true);
      gl.disable(gl.BLEND);
      gl.useProgram(MP);
      gl.uniformMatrix4fv(U.muVP, false, st.VP);
      gl.uniform1f(U.muT, T);
      gl.uniform2f(U.muFog, fog[0], fog[1]);
      gl.uniform3fv(U.muEye, st.eye);
      // the key light rides with the camera: from above, over the viewer's left shoulder
      const fwd = unit(sub(st.target, st.eye)), right = unit(cross(fwd, [0, 1, 0]));
      gl.uniform3fv(U.muL, unit(add(add(scl(fwd, -0.55), [0, 0.8, 0]), scl(right, -0.35))));
      gl.uniform3fv(U.muGround, pal.surface);
      if (dark) { gl.uniform4f(U.muLook, 0.14, 0.72, 0.95, 0.45); gl.uniform1f(U.muRimTo, 0.25); }
      else { gl.uniform4f(U.muLook, 0.5, 0.55, 0.35, 0.3); gl.uniform1f(U.muRimTo, 0.6); }
      for (const k in meshes) {
        const M = meshes[k];
        if (!M.inst.n) continue;
        gl.bindBuffer(gl.ARRAY_BUFFER, M.ib);
        gl.bufferData(gl.ARRAY_BUFFER, M.inst.a.subarray(0, M.inst.n), gl.DYNAMIC_DRAW);
        gl.bindVertexArray(M.vao);
        gl.drawArraysInstanced(gl.TRIANGLES, 0, M.count, M.inst.n / 12);
      }
      gl.depthMask(false);
      gl.enable(gl.BLEND);
      if (dark) gl.blendFunc(gl.ONE, gl.ONE); else gl.blendFunc(gl.ONE, gl.ONE_MINUS_SRC_ALPHA);
      // ---- ribbons ----
      RB.n = 0;
      const seg = (a, b, c, alA, alB, wA, wB, glow) => {
        RB.need(LS); const r = RB.a; let i = RB.n;
        r[i++] = a[0]; r[i++] = a[1]; r[i++] = a[2]; r[i++] = b[0]; r[i++] = b[1]; r[i++] = b[2];
        r[i++] = c[0]; r[i++] = c[1]; r[i++] = c[2]; r[i++] = alA; r[i++] = wA * st.dpr; r[i++] = wB * st.dpr; r[i++] = alB; r[i++] = glow; RB.n = i;
      };
      // a curve as tapered segments; light(t) adds a passing signal's glow along it
      const curve = (fn, n, c, a0, a1, w0, w1, glow, light) => {
        let prev = fn(0), pt = 0;
        for (let i = 1; i <= n; i++) {
          const t = i / n, p = fn(t);
          const la = light ? light(pt) : 0, lb = light ? light(t) : 0;
          const cc = la || lb ? mix3(c, [1, 1, 1], dark ? 0.35 * Math.max(la, lb) : 0) : c;
          seg(prev, p, cc, lerp(a0, a1, pt) + la, lerp(a0, a1, t) + lb, lerp(w0, w1, pt) * (1 + 1.2 * la), lerp(w0, w1, t) * (1 + 1.2 * lb), glow + 0.8 * Math.max(la, lb));
          prev = p; pt = t;
        }
      };
      // the cortex outline: each hemisphere a few sagittal arcs over the top, a gap between them
      const A = st.A, wire = pal.muted, wa = (dark ? 0.1 : 0.12) * (focus ? 0.5 : 1);
      for (const side of [-1, 1]) for (const k of [0.08, 0.45, 0.78]) {
        const r = Math.sqrt(1 - k * k);
        curve(t => { const a = -0.35 + t * (Math.PI + 0.7); return [side * k * A[0], Math.sin(a) * r * A[1], Math.cos(a) * r * A[2]]; }, 40, wire, wa, wa, 1, 1, 0);
      }
      for (const y of [0, 0.5]) {
        const r = Math.sqrt(1 - y * y);
        for (const side of [-1, 1]) curve(t => { const a = -Math.PI / 2 + t * Math.PI; return [side * (0.08 * A[0] + Math.cos(a) * r * A[0] * 0.92), y * A[1], Math.sin(a) * r * A[2]]; }, 36, wire, wa * 0.8, wa * 0.8, 1, 1, 0);
      }
      // dendrites
      const da = dark ? 0.2 : 0.18;
      for (const dd of st.dendrites) {
        const q = st.nodes.get(dd.id);
        if (!q) continue;
        const c = col(q.rank), k = dim(q.id) ? 0.2 : 1;
        for (const [a, b, w0, w1] of dd.segs) seg(add(q.p, a), add(q.p, b), c, da * k * (w0 > 0.9 ? 0.9 : 0.6), da * k * 0.15, 1.6 * w0 + 0.4, 1.6 * w1 + 0.3, 0);
      }
      // the relates: faint arcs between peers; the one under the pointer lights and says why
      for (const r of st.relates) {
        const hot = st.hoverRel && st.hoverRel.r === r;
        const on = focus && lit && lit.nodes.has(r.a) && lit.nodes.has(r.b) && (focus === r.a || focus === r.b);
        const off = focus && !on;
        const a = hot ? 0.85 : off ? 0.015 : on ? 0.5 : (dark ? 0.1 : 0.14) + 0.22 * r.w;
        const c = hot || on ? pal.ink : mix3(pal.muted, pal.ink, 0.4);
        curve(relCurve(r), 14, c, a, a, 0.8 + 1.6 * r.w, 0.8 + 1.6 * r.w, hot ? 0.8 : 0.25);
      }
      // the tree's synapses: thick at the parent, thin at the child, lit as a signal passes
      for (const e of st.edges) {
        const c = col(st.nodes.get(e.down).rank);
        const on = focus && lit && lit.tree.has(e.key);
        const liveE = st.live.has(e.down);
        const base = (dark ? 0.4 : 0.55) + 0.35 * e.k;
        const alpha = focus ? (on ? 0.95 : 0.04) : liveE ? 0.95 : base * (e.implied ? 0.55 : 1);
        const w = (2.2 + 3 * e.k) * (on || liveE ? 1.4 : 1);
        const g = glowOn.get(e.key);
        const light = g ? t => { const head = g.head, behind = g.dir === 'up' ? t - (1 - head) : head - t; return behind < 0 ? 0 : Math.exp(-behind * 2.2) * (g.fade === undefined ? 1 : g.fade) * 0.9; } : null;
        const cc = g ? mix3(c, g.c, 0.6) : c;
        curve(synapse(e), 14, cc, alpha, alpha * 0.75, w * 1.6, w * 0.4, liveE || on ? 0.6 : 0.35, light);
      }
      // the held skills' filaments: the skill's colour, falling from the crown, glowing; brighter while
      // the holder works, quiet only while another neuron holds the focus
      for (const w of st.holds) {
        const skill = st.nodes.get(w.to), c = col(skill.rank);
        const on = focus && lit && lit.holds.has(w.key);
        const working = st.live.has(w.from) || st.busy.has(w.from);
        const a = focus ? (on ? 0.85 : 0.025) : working ? 0.75 : (dark ? 0.24 : 0.3);
        const a1 = focus ? (on ? 0.6 : 0.015) : working ? 0.6 : (dark ? 0.1 : 0.16);
        curve(holdCurve(w), 22, c, a, a1, on || working ? 3.2 : 2.4, on || working ? 1.4 : 0.9, on || working ? 1 : 0.6);
      }
      // a signal's trail: the stretch of bond just behind its head, bright, fading backwards
      const trails = [];
      for (const pl of sigs) {
        const s = signalAt(pl);
        if (!s) continue;
        const [up, down] = pl.hops[s.h];
        if (!st.nodes.has(up) || !st.nodes.has(down)) continue;
        const fn = synapse({ up, down });
        const c = pl.tone === 'down' ? col((st.nodes.get(pl.end) || {}).rank) : pal[pl.tone] || pal.ok;
        const u = ease(s.lt), at = pl.dir === 'up' ? 1 - u : u;
        const back = pl.dir === 'up' ? Math.min(1, at + 0.28) : Math.max(0, at - 0.28);
        curve(t => fn(lerp(back, at, t)), 8, c, 0, 0.95, 0.5, 4.5, 1.2);
        trails.push({ p: fn(at), c });
      }
      gl.useProgram(LP);
      gl.uniformMatrix4fv(U.luVP, false, st.VP);
      gl.uniform2f(U.luView, cw, ch);
      gl.uniform2f(U.luFog, fog[0], fog[1]);
      gl.uniform1f(U.luRef, st.dist);
      gl.bindBuffer(gl.ARRAY_BUFFER, lbuf);
      gl.bufferData(gl.ARRAY_BUFFER, RB.a.subarray(0, RB.n), gl.DYNAMIC_DRAW);
      gl.bindVertexArray(vaoL);
      gl.drawArraysInstanced(gl.TRIANGLES, 0, 6, RB.n / LS);
      // ---- sprites: motes, region glows, halos, particles, sparks, rings ----
      SB.n = 0;
      const put = (p, c, a, size, core) => {
        SB.need(10); const s = SB.a; let i = SB.n;
        s[i++] = p[0]; s[i++] = p[1]; s[i++] = p[2]; s[i++] = c[0]; s[i++] = c[1]; s[i++] = c[2]; s[i++] = a; s[i++] = size; s[i++] = core;
        const M = st.VP; s[i++] = M[3] * p[0] + M[7] * p[1] + M[11] * p[2] + M[15]; SB.n = i;
      };
      const drift = reduced ? 0 : st.t * 0.012;
      const mc = dark ? mix3(pal.accent, pal.ink, 0.5) : pal.muted;
      for (const s of st.stars.pts) {
        const a = drift * (0.5 + s[4]), ca = Math.cos(a), sa = Math.sin(a);
        const p = [s[0] * ca - s[2] * sa, s[1], s[0] * sa + s[2] * ca], M = st.VP, w = M[3] * p[0] + M[7] * p[1] + M[11] * p[2] + M[15];
        // a mote close to the eye would swell into a blot: only the far field drifts
        if (w < st.dist * 0.75) continue;
        put(p, mc, (dark ? 0.55 : 0.35) * (0.4 + s[4] * 0.6), s[3], 0.5);
      }
      // a region's volume: a broad faint glow on each member, so the lobe's own shape tints
      for (const r of st.regions) {
        const off = focus && lit && !r.members.some(id => lit.nodes.has(id));
        const a = (dark ? 0.11 : 0.08) * r.k * (off ? 0.3 : 1);
        let c = [0, 0, 0];
        for (const id of r.members) { const q = st.nodes.get(id); c = add(c, q.p); put(q.p, r.tint, a, st.S * (q.d === 1 ? 0.9 : 0.7), 0); }
        put(scl(c, 1 / r.members.length), r.tint, a * 0.8, st.S * 1.5, 0);
      }
      for (const q of st.nodes.values()) {
        const c = col(q.rank), d = dim(q.id), on = q.id === focus;
        const liveN = st.live.has(q.id) || st.busy.has(q.id);
        const k = d ? 0.15 : 1, halo = dark ? 1 : 0.55;
        put(q.p, c, (dark ? 0.5 : 0.4) * k * (liveN ? 1.3 + 0.3 * breath : on ? 1.4 : 1), q.rad * 5.2 * halo, 0);
        // the bloom: a second, wider and fainter pass
        if (dark) put(q.p, c, 0.12 * k * (liveN ? 1.6 : 1), q.rad * 11, 0);
        // the value band ring: its rank's own meter, banded by config; faint below its record
        const v = q.n && q.n.value;
        if (v && v.band && v.band !== 'none' && q.d !== -1) put(q.p, v.band === 'ok' ? pal.ok : v.band === 'warn' ? pal.warn : pal.danger, (v.confidence === 'ok' ? 0.85 : 0.35) * k, q.rad * 3.4, -0.86);
      }
      const thin = st.slow > 14 ? 2 : 1;
      if (!reduced) {
        // particles drift down the tree, the way work goes
        for (const e of st.edges) {
          if (focus && lit && !lit.tree.has(e.key)) continue;
          const n = Math.max(1, Math.round((1 + Math.min(3, Math.floor(Math.log2(1 + e.weight)))) / thin));
          const c = col(st.nodes.get(e.down).rank), fn = synapse(e);
          const speed = 0.14 + 0.1 * e.k + (st.live.has(e.down) ? 0.3 : 0);
          for (let j = 0; j < n; j++) {
            const u = (st.t * speed + j / n + e.seed) % 1;
            put(fn(u), c, (dark ? 0.85 : 0.7) * Math.sin(u * Math.PI), 0.12, 0.35);
          }
        }
        // and down a held skill's filament while its holder works
        for (const w of st.holds) {
          if (!(st.live.has(w.from) || st.busy.has(w.from))) continue;
          if (focus && lit && !lit.holds.has(w.key)) continue;
          const c = col(st.nodes.get(w.to).rank), fn = holdCurve(w);
          for (let j = 0; j < 3; j++) {
            const u = (st.t * 0.42 + j / 3 + w.seed) % 1;
            put(fn(u), c, Math.sin(u * Math.PI), 0.2, 0.4);
            put(fn(Math.max(0, u - 0.03)), c, 0.4 * Math.sin(u * Math.PI), 0.14, 0);
          }
        }
      }
      // the signals' heads, with a soft bloom
      for (const t of trails) { put(t.p, t.c, 1, 0.3, 0.55); put(t.p, t.c, 0.55, 1.1, 0); }
      // firing: a ring that widens, a brief bloom
      for (const f of flashes) {
        const q = st.nodes.get(f.id);
        if (!q) continue;
        const age = (rp ? rp.t : now) - f.at, life = f.kind === 'read' ? 700 : 1100, t = clamp(age / life, 0, 1);
        const c = f.tone === 'info' ? pal.accent : f.tone === 'rank' || f.tone === 'down' ? col(q.rank) : pal[f.tone] || col(q.rank);
        const big = f.kind === 'read' ? 0.6 : 1, sz = q.rad * (3 + 9 * t * big) * (dark ? 1 : 0.8);
        put(q.p, c, (1 - t) * (dark ? 0.95 : 0.85), sz, -0.82);
        if (t < 0.3) put(q.p, c, (0.3 - t) * 1.6 * big, q.rad * 4.5, 0);
      }
      // the light theme blends in order: far first
      let SBuf = SB;
      if (!dark) {
        const n = SB.n / 10, idx = new Array(n);
        for (let i = 0; i < n; i++) idx[i] = i;
        idx.sort((x, y) => SB.a[y * 10 + 9] - SB.a[x * 10 + 9]);
        SB2.n = 0; SB2.need(SB.n);
        for (let i = 0; i < n; i++) SB2.a.set(SB.a.subarray(idx[i] * 10, idx[i] * 10 + 10), i * 10);
        SB2.n = SB.n; SBuf = SB2;
      }
      gl.useProgram(P);
      gl.uniformMatrix4fv(U.puVP, false, st.VP);
      gl.uniform1f(U.puScale, st.scale);
      gl.uniform1f(U.puMax, maxPoint);
      gl.uniform2f(U.puFog, fog[0], fog[1]);
      gl.bindBuffer(gl.ARRAY_BUFFER, pbuf);
      gl.bufferData(gl.ARRAY_BUFFER, SBuf.a.subarray(0, SBuf.n), gl.DYNAMIC_DRAW);
      gl.bindVertexArray(vaoP);
      gl.drawArrays(gl.POINTS, 0, SBuf.n / 10);
      gl.bindVertexArray(null);
      gl.depthMask(true);
      gl.disable(gl.DEPTH_TEST);
      // a live signal that reached its end fires there
      if (!rp) st.pulses = st.pulses.filter(pl => { if ((now - pl.at) / (pl.per * pl.hops.length) >= 1) { flash(pl.end, pl.tone); return false; } return true; });
      for (const [id, f] of st.flashes) if (now - f.at > (f.kind === 'read' ? 700 : 1100)) st.flashes.delete(id);
      labels(focus, lit);
    }

    // the faces, loaded once per rank; a face that has not arrived yet is simply not drawn
    const faces = new Map();
    function face(rank) {
      if (!rank) return null;
      let f = faces.get(rank);
      if (!f) {
        f = { img: new Image(), ok: false };
        f.img.onload = () => { f.ok = true; kick(); };
        f.img.src = 'portraits/' + encodeURIComponent(rank);
        faces.set(rank, f);
      }
      return f.ok ? f.img : null;
    }

    // ---------- labels: the orchestrator and first rank always, the rest on hover, focus or while live
    // (every one once 24 or fewer are in view); the regions, the shells and the crown captioned ----------
    function labels(focus, lit) {
      const ctx = octx, W = st.W, H = st.H, dpr = st.dpr, F = st.font;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      ctx.clearRect(0, 0, W, H);
      if (!F) return;
      const px = F.px;
      const width = (text, font) => { const k = font + '|' + text; let w = F.widths.get(k); if (w === undefined) { ctx.font = font; w = ctx.measureText(text).width; F.widths.set(k, w); } return w; };
      ctx.textBaseline = 'middle';
      ctx.lineJoin = 'round';
      const taken = [];
      const free = b => !taken.some(o => !(b[2] < o[0] || b[0] > o[2] || b[3] < o[1] || b[1] > o[3]));
      const text = (t, x, y, font, fill, alpha, align) => {
        ctx.font = font; ctx.textAlign = align || 'left'; ctx.globalAlpha = alpha;
        ctx.lineWidth = 3.5; ctx.strokeStyle = F.halo; ctx.strokeText(t, x, y);
        ctx.fillStyle = fill; ctx.fillText(t, x, y);
      };
      if (st.crown) {
        // at the crown's left edge on screen
        let lo = null;
        for (let i = 0; i < 32; i++) { const a = i / 32 * TAU, p = project([Math.cos(a) * st.crown.r, st.crown.y, Math.sin(a) * st.crown.r]); if (p.w > 0 && (!lo || p.x < lo.x)) lo = p; }
        const w = lo ? width(st.crown.caption, F.caption) : 0;
        if (lo && lo.x - 28 - w > 4) { taken.push([lo.x - 28 - w, lo.y - px * 0.4, lo.x - 28, lo.y + px * 0.4]); text(st.crown.caption, lo.x - 28, lo.y, F.caption, F.muted, focus ? 0.3 : 0.75, 'right'); }
      }
      // the regions: the head's name at the outer edge of the lobe
      for (const r of st.regions) {
        let c = [0, 0, 0];
        for (const id of r.members) c = add(c, st.nodes.get(id).p);
        c = scl(c, 1 / r.members.length);
        const out = len(c) > 0.01 ? unit(c) : [0, 1, 0];
        const p = project(add(scl(out, st.S * 1.08), [0, st.S * 0.08, 0]));
        if (p.w <= 0 || p.x < 0 || p.x > W) continue;
        const t = r.caption + (r.note && st.tint !== 'lane' ? ' · ' + r.note : '');
        const w = width(t, F.region);
        const box = [p.x - w / 2 - 4, p.y - px * 0.5, p.x + w / 2 + 4, p.y + px * 0.5];
        if (!free(box)) continue;
        taken.push(box);
        text(t, p.x, p.y, F.region, F.css(mix3(r.tint, st.pal.ink, st.dark ? 0.35 : 0.5)), focus && lit && !r.members.some(id => lit.nodes.has(id)) ? 0.25 : 0.9, 'center');
      }
      const onScreen = [];
      for (const q of st.nodes.values()) {
        const s = project(q.p);
        if (s.w <= 0 || s.x < -40 || s.x > W + 40 || s.y < -20 || s.y > H + 20) continue;
        onScreen.push({ q, s });
      }
      // each member's face as a badge where it fits (its resolved portrait or family glyph,
      // /portraits/<rank> — the one resolver), scaled to the neuron's size on screen
      for (const { q, s } of onScreen) {
        if (q.d < 0 || !q.n.icon) continue;
        const r = q.rad * 1.2 * st.scale / s.w / dpr;
        if (r < 6) continue;
        const img = face(q.n.orchestrator ? 'orchestrator' : q.n.rank);
        if (!img) continue;
        const br = clamp(r * 0.62, 6, 16), bx = s.x + r * 0.75, by = s.y - r * 0.75;
        ctx.globalAlpha = focus && lit && !lit.nodes.has(q.id) ? 0.25 : 0.95;
        ctx.save();
        ctx.beginPath(); ctx.arc(bx, by, br, 0, TAU); ctx.fillStyle = F.halo; ctx.fill();
        ctx.clip(); ctx.drawImage(img, bx - br, by - br, br * 2, br * 2);
        ctx.restore();
        ctx.beginPath(); ctx.arc(bx, by, br, 0, TAU); ctx.lineWidth = 1.5; ctx.strokeStyle = F.css(st.pal.rank[q.rank] || st.pal.muted); ctx.stroke();
        ctx.globalAlpha = 1;
      }
      const few = onScreen.length <= 24;
      const shown = [];
      for (const { q, s } of onScreen) {
        const on = q.id === focus || (focus && lit && lit.nodes.has(q.id));
        const always = few || q.d === 0 || q.d === 1 || (q.d === -1 && st.m.skills.length <= 12);
        if (!(on || st.live.has(q.id) || st.busy.has(q.id) || q.id === st.hover || (!focus && always))) continue;
        shown.push({ q, s });
      }
      shown.sort((a, b) => a.q.id === focus ? -1 : b.q.id === focus ? 1 : a.q.d - b.q.d || a.s.w - b.s.w);
      for (const { q, s } of shown) {
        const t = shortName(q.n);
        const font = q.d === 0 ? F.orchestrator : q.d === 1 ? F.first : q.d < 0 ? F.skill : F.rest;
        const w = width(t, font);
        const r = q.rad * 1.2 * st.scale / s.w / dpr;
        const x = s.x + r + 5, y = s.y;
        const box = [x - 2, y - px * 0.55, x + w + 2, y + px * 0.55];
        if (q.id !== focus && !free(box)) continue;
        taken.push(box);
        const near = clamp(1.4 - (s.w - st.dist * 0.75) / (st.dist0 * 0.9), 0.5, 1);
        text(t, x, y, font, q.d < 0 ? F.muted : F.ink, q.id === focus ? 1 : near);
      }
      // a relates bond under the pointer says why it holds
      const hr = st.hoverRel;
      if (hr) {
        const a = st.m.byId.get(hr.r.a), b = st.m.byId.get(hr.r.b), why = hr.r.why, bits = [];
        if (why.imports) bits.push(why.imports + ' import' + (why.imports === 1 ? '' : 's'));
        if (why.skills) bits.push(why.skills + ' shared skill' + (why.skills === 1 ? '' : 's'));
        if (why.days) bits.push(why.days + ' days worked together');
        if (why.turns) bits.push('commissioned together');
        const t1 = (a ? a.name : hr.r.a) + ' ↔ ' + (b ? b.name : hr.r.b), t2 = bits.join(' · ') || 'related';
        ctx.font = F.tip;
        const w = Math.max(width(t1, F.tip), width(t2, F.tip)) + 16, h = px * 2.6;
        const x = clamp(hr.x + 14, 4, W - w - 4), y = clamp(hr.y + 14, 4, H - h - 4);
        ctx.globalAlpha = 0.94; ctx.fillStyle = F.halo; ctx.strokeStyle = F.line; ctx.lineWidth = 1;
        ctx.fillRect(x, y, w, h); ctx.strokeRect(x + 0.5, y + 0.5, w - 1, h - 1);
        ctx.globalAlpha = 1; ctx.textAlign = 'left';
        ctx.fillStyle = F.ink; ctx.fillText(t1, x + 8, y + h * 0.32);
        ctx.fillStyle = F.muted; ctx.fillText(t2, x + 8, y + h * 0.7);
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
        const r = Math.max(10, q.rad * 1.5 * st.scale / s.w / st.dpr);
        const d = Math.hypot(s.x - x, s.y - y);
        if (d < r && d - r * 0.2 * (s.w / st.dist) < bd) { bd = d - r * 0.2 * (s.w / st.dist); best = q.id; }
      }
      return best;
    }
    function pickRel(x, y) {
      let best = null, bd = 7;
      for (const r of st.relates) {
        const fn = relCurve(r);
        let prev = project(fn(0));
        for (let i = 1; i <= 10; i++) {
          const p = project(fn(i / 10));
          if (p.w > 0 && prev.w > 0) {
            const dx = p.x - prev.x, dy = p.y - prev.y, l2 = dx * dx + dy * dy || 1;
            const t = clamp(((x - prev.x) * dx + (y - prev.y) * dy) / l2, 0, 1);
            const d = Math.hypot(x - (prev.x + t * dx), y - (prev.y + t * dy));
            if (d < bd) { bd = d; best = r; }
          }
          prev = p;
        }
      }
      return best;
    }
    const ptrs = new Map();
    let down = null, pinch = 0;
    const local = ev => { const r = canvas.getBoundingClientRect(); return [ev.clientX - r.left, ev.clientY - r.top]; };
    const still = () => { if (!st.run || opts.reduced()) render(performance.now(), true); };
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
      const r = id ? null : pickRel(p[0], p[1]);
      canvas.toggleAttribute('data-hot', !!id);
      const was = st.hoverRel;
      st.hoverRel = r ? { r, x: p[0], y: p[1] } : null;
      if (id !== st.hover) { st.hover = id; opts.onHover(id); kick(); }
      if (r || was) { kick(); still(); }
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
    canvas.addEventListener('pointerleave', () => { if (st.hover) { st.hover = ''; opts.onHover(''); } if (st.hoverRel) { st.hoverRel = null; kick(); still(); } canvas.removeAttribute('data-hot'); });
    canvas.addEventListener('wheel', ev => {
      ev.preventDefault();
      touched();
      st.zoomed = true;
      st.goal.dist = clamp(st.goal.dist * Math.exp(ev.deltaY * 0.0012), 2, st.dist0 * 2.2);
      st.idleAt = performance.now(); kick();
    }, { passive: false });
    canvas.addEventListener('keydown', ev => {
      touched();
      if (opts.onKey && opts.onKey(ev)) { ev.preventDefault(); st.idleAt = performance.now(); kick(); still(); return; }
      if (!ev.shiftKey && /^Arrow/.test(ev.key)) return;
      const turn = { ArrowLeft: -0.2, ArrowRight: 0.2 }[ev.key], tilt = { ArrowUp: -0.12, ArrowDown: 0.12 }[ev.key];
      if (turn) st.yaw += turn;
      else if (tilt) st.pitch = clamp(st.pitch + tilt, -1.25, 1.25);
      else if (ev.key === '+' || ev.key === '=') { st.zoomed = true; st.goal.dist = clamp(st.goal.dist * 0.85, 2, st.dist0 * 2.2); }
      else if (ev.key === '-') { st.zoomed = true; st.goal.dist = clamp(st.goal.dist / 0.85, 2, st.dist0 * 2.2); }
      else return;
      ev.preventDefault(); st.idleAt = performance.now(); kick();
      still();
    });
    canvas.addEventListener('blur', () => opts.onBlur && opts.onBlur());
    resetBtn.addEventListener('click', () => {
      touched();
      st.yaw = 0.62; st.pitch = 0.26; st.zoomed = false;
      if (st.focus) opts.onPick('');
      fitDist();
      st.goal.target = (st.home || [0, 0, 0]).slice(); st.goal.dist = st.dist0;
      st.idleAt = performance.now(); kick();
      still();
    });
    canvas.addEventListener('webglcontextlost', ev => { ev.preventDefault(); st.run = false; if (st.raf) cancelAnimationFrame(st.raf); st.raf = 0; });
    canvas.addEventListener('webglcontextrestored', () => { init(); st.run = true; st.last = 0; kick(); });

    // ---------- the replay: the last turn's signals, again, at their own pace ----------
    // a turn is a run of signals with no gap longer than fifteen seconds; long pauses inside it are
    // shortened to a second and a half, so the replay keeps the rhythm, not the waiting
    function record(ev) {
      const now = performance.now();
      if (!st.rec || now - st.rec.last > 15000) st.rec = { events: [], last: now };
      if (st.rec.events.length < 400) st.rec.events.push({ ...ev, real: now });
      st.rec.last = now;
      replayBtn.disabled = false;
    }
    function openReplay() {
      const rec = st.rec;
      if (!rec || !rec.events.length) return;
      let vt = 0, prev = rec.events[0].real;
      const pulses = [], flashes = [];
      for (const e of rec.events) {
        vt += Math.min(1500, e.real - prev); prev = e.real;
        if (e.type === 'pulse') {
          pulses.push({ ...e.p, at: vt });
          flashes.push({ id: e.p.end, tone: e.p.tone, at: vt + e.p.per * e.p.hops.length });
        } else flashes.push({ id: e.id, tone: e.tone, kind: e.kind, at: vt });
      }
      const dur = Math.max(...pulses.map(p => p.at + p.per * p.hops.length), ...flashes.map(f => f.at)) + 1600;
      st.replay = { pulses, flashes, t: 0, dur, playing: !opts.reduced() };
      scrub.max = String(Math.round(dur)); scrub.value = '0';
      playBtn.textContent = st.replay.playing ? 'pause' : 'play';
      clock.textContent = '0.0 s';
      bar.hidden = false;
      st.idleAt = performance.now(); st.last = 0; kick(); still();
    }
    function closeReplay() { st.replay = null; bar.hidden = true; kick(); still(); }
    replayBtn.addEventListener('click', () => { touched(); if (st.replay) closeReplay(); else openReplay(); });
    closeBtn.addEventListener('click', closeReplay);
    playBtn.addEventListener('click', () => {
      const rp = st.replay;
      if (!rp) return;
      if (!rp.playing && rp.t >= rp.dur) rp.t = 0;
      rp.playing = !rp.playing;
      playBtn.textContent = rp.playing ? 'pause' : 'play';
      st.last = 0; kick();
    });
    scrub.addEventListener('input', () => {
      const rp = st.replay;
      if (!rp) return;
      rp.playing = false; playBtn.textContent = 'play';
      rp.t = +scrub.value; clock.textContent = (rp.t / 1000).toFixed(1) + ' s';
      kick(); render(performance.now(), opts.reduced());
    });

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

    function flash(id, tone, kind) { st.flashes.set(id, { id, at: performance.now(), tone, kind }); kick(); }

    return {
      elements: [canvas, overlay, tools, legend, key, bar, hint],
      model(m) { scene(m); resize(); },
      focus(id, lit, pinned) {
        st.lit = lit; st.shown = id || '';
        // the camera eases to a pinned neuron; a hover only lights
        if ((pinned || '') !== st.focus) {
          st.focus = pinned || '';
          const q = st.nodes.get(st.focus);
          st.goal.target = q ? q.p1.slice() : (st.home || [0, 0, 0]).slice();
          st.goal.dist = q ? Math.max(3.5, st.dist0 * 0.55) : st.dist0;
        }
        kick();
        if (opts.reduced()) render(performance.now(), true);
      },
      live(ids) { st.live = ids; kick(); },
      act(id, tone, kind) { if (st.nodes.has(id)) { flash(id, tone, kind); record({ type: 'flash', id, tone, kind }); } },
      // a burst on a neuron: it breathes like a live one until the burst ends
      busy(id, on) { if (on) st.busy.add(id); else st.busy.delete(id); kick(); },
      // the keyboard's cursor: the camera eases to it without pinning it
      follow(id) {
        const q = st.nodes.get(id);
        if (!q) return;
        st.goal.target = q.p1.slice();
        st.goal.dist = Math.min(st.goal.dist, st.dist0 * 0.8);
        st.idleAt = performance.now(); kick();
      },
      signal(kind, hops, id, tone) {
        const pairs = hops.map(h => h.split('>')).filter(([a, b]) => st.nodes.has(a) && st.nodes.has(b));
        const endAt = kind === 'up' ? (pairs.length ? pairs[0][0] : id) : id;
        const t = kind === 'up' ? (tone || 'ok') : 'down';
        if (opts.reduced() || !pairs.length) { flash(endAt, t); record({ type: 'flash', id: endAt, tone: t }); return; }
        const p = { hops: kind === 'up' ? pairs.slice().reverse() : pairs, dir: kind, at: performance.now(), per: 700, end: endAt, tone: t };
        st.pulses.push(p);
        record({ type: 'pulse', p: { ...p } });
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
