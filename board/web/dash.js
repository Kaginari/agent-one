// dash.js — the dashboard: the session's console, the workspace as a neural net, metrics and
// relations. No dependencies; every element is built with textContent or escaped markup; every
// colour and length comes from the css tokens (the page sets only --<component>-* inputs).
(() => {
  'use strict';

  // ---------- basics ----------
  const $ = (sel, root = document) => root.querySelector(sel);
  const TOKEN = $('meta[name="board-token"]').content;
  let WORDS = {};
  try { WORDS = JSON.parse(document.body.dataset.words || '{}'); } catch (_) { /* default words */ }
  const word = (k, d) => WORDS[k] || d || k;

  function el(tag, attrs, ...kids) {
    const n = document.createElement(tag);
    for (const [k, v] of Object.entries(attrs || {})) {
      if (v === false || v === null || v === undefined) continue;
      if (k === 'text') n.textContent = v;
      else if (k === 'html') n.innerHTML = v; // only ever given escaped markup (md())
      else if (k.startsWith('on')) n.addEventListener(k.slice(2), v);
      else if (k === 'style') for (const [p, val] of Object.entries(v)) n.style.setProperty(p, val);
      else n.setAttribute(k, v === true ? '' : v);
    }
    for (const kid of kids.flat()) if (kid !== null && kid !== undefined && kid !== false) n.append(kid);
    return n;
  }
  const SVG = 'http://www.w3.org/2000/svg';
  function svg(tag, attrs, ...kids) {
    const n = document.createElementNS(SVG, tag);
    for (const [k, v] of Object.entries(attrs || {})) if (v !== false && v !== null && v !== undefined) n.setAttribute(k, v === true ? '' : v);
    for (const kid of kids.flat()) if (kid) n.append(kid);
    return n;
  }
  const esc = s => String(s).replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));
  const num = n => (n || 0).toLocaleString('en-US');
  const usd = n => '$' + (n || 0).toFixed(4);
  const getJSON = url => fetch(url, { cache: 'no-store' }).then(r => { if (!r.ok) throw new Error(r.status + ''); return r.json(); });
  function post(url, body) {
    return fetch(url, { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-Board-Token': TOKEN }, body: JSON.stringify(body || {}) })
      .then(r => r.ok ? r.json() : r.text().then(t => { throw new Error(t.trim() || r.statusText); }));
  }
  const store = {
    get(k) { try { return localStorage.getItem('dash.' + k) || ''; } catch (_) { return ''; } },
    set(k, v) { try { localStorage.setItem('dash.' + k, v); } catch (_) { /* private mode */ } },
  };
  for (const n of document.querySelectorAll('[data-word]')) n.textContent = word(n.dataset.word, n.textContent);

  // ---------- the look: the palette (ui.theme) and its mode (ui.mode) — one choice with the terminal ----------
  // The page arrives holding the workspace's look (data-scheme, data-theme, /theme.css). The menu lists
  // every palette and the three modes; a pick is POSTed to the session, which writes the workspace's
  // config.local.yaml and tells every surface (the terminal follows; a "theme" event brings a pick
  // made in the terminal here). The palette's roles are a stylesheet swapped in place — no reload;
  // the 3D net repaints when data-scheme changes (its probes re-read the colours).
  const root = document.documentElement;
  const schemeSel = $('#scheme'), themeSel = $('#theme');
  function holdMode(m) {
    if (m === 'dark' || m === 'light') root.setAttribute('data-theme', m); else root.removeAttribute('data-theme');
    themeSel.value = m === 'dark' || m === 'light' ? m : 'system';
  }
  function holdScheme(name) {
    if (!name) return;
    schemeSel.value = name;
    const old = $('#scheme-css');
    if (root.dataset.scheme === name && old) return;
    const next = el('link', { rel: 'stylesheet', href: 'theme.css?name=' + encodeURIComponent(name) + '&v=' + Date.now() });
    next.addEventListener('load', () => {
      for (const l of document.querySelectorAll('link[data-scheme-css]')) if (l !== next) l.remove();
      if (old && old !== next) old.remove();
      next.id = 'scheme-css';
      root.dataset.scheme = name;
    }, { once: true });
    next.setAttribute('data-scheme-css', '');
    (old || $('link[href="web/theme.css"]')).after(next);
  }
  function look(name, mode) {
    if (mode) holdMode(mode);
    if (name) holdScheme(name);
  }
  holdMode(root.getAttribute('data-theme') || 'system');
  getJSON('api/theme').then(t => {
    schemeSel.replaceChildren(...t.themes.map(p => el('option', { value: p.name, text: p.label.toLowerCase(), title: p.swatches.map(s => s.name + ' ' + s.hex).join(' · ') })));
    schemeSel.value = t.name;
    holdMode(t.mode);
    if (!t.settable) { schemeSel.title = themeSel.title = 'this board previews a theme; the session keeps it'; }
  }).catch(() => { schemeSel.replaceChildren(el('option', { value: root.dataset.scheme, text: root.dataset.scheme })); });
  function pick(body) {
    post('api/theme', body).then(r => look(r.name, r.mode)).catch(err => {
      // a board that cannot keep it still shows it, for this page only
      look(body.name, body.mode);
      schemeSel.title = 'not kept: ' + err.message;
    });
  }
  schemeSel.addEventListener('change', () => pick({ name: schemeSel.value }));
  themeSel.addEventListener('change', () => pick({ mode: themeSel.value }));

  // ---------- views ----------
  const main = $('#main');
  // the views: the tabs name them; a deep link (/#memory, /#subagent/<run>) opens one; the root opens
  // the cockpit. Listeners (workspace.js) hear every change through the page's bus.
  const tabs = [...document.querySelectorAll('#tablist .tab')];
  const VIEWS = tabs.map(t => t.dataset.view);
  const bus = new EventTarget();
  // the hash: "#memory", "#subagent/<id>", or the older "#view=memory&theme=light" — a bare word is
  // the view, its tail after "/" the view's own argument, key=value pairs are settings
  function parseHash() {
    const out = { view: '', arg: '', params: new URLSearchParams() };
    for (const part of decodeURIComponent(location.hash.slice(1)).split('&')) {
      if (!part) continue;
      if (part.includes('=')) { const [k, ...v] = part.split('='); out.params.set(k, v.join('=')); continue; }
      const [vw, ...rest] = part.split('/');
      out.view = vw; out.arg = rest.join('/');
    }
    if (!out.view && out.params.get('view')) out.view = out.params.get('view');
    return out;
  }
  function setHash(v, arg) {
    const want = v === 'overview' && !arg ? '' : '#' + v + (arg ? '/' + encodeURIComponent(arg) : '');
    if (location.hash !== want) window.history.replaceState(null, '', want || location.pathname + location.search);
  }
  function view(v, arg) {
    if (!VIEWS.includes(v)) v = 'overview';
    const changed = main.dataset.view !== v;
    main.dataset.view = v;
    for (const t of tabs) { const on = t.dataset.view === v; t.setAttribute('aria-selected', String(on)); t.tabIndex = on ? 0 : -1; }
    setHash(v, arg);
    fitTabs();
    bus.dispatchEvent(new CustomEvent('view', { detail: { view: v, arg: arg || '', changed } }));
    // in the cockpit the focused neuron's card floats over the net's top-right corner; on the
    // net's own page it takes the side column
    const kc = document.getElementById('kcard'), slot = document.getElementById('net-float'), side = document.querySelector('.net-side');
    const nl = document.getElementById('net-layout');
    if (nl) nl.toggleAttribute('data-side', v !== 'overview');
    // the cockpit keeps the net's findings on the net's own page (the note counts them) and
    // shows the now strip
    const nf = document.getElementById('net-findings'), nw = document.getElementById('now');
    if (nf) nf.hidden = v === 'overview';
    if (nw) nw.hidden = v !== 'overview';
    const nn = document.getElementById('net-note');
    if (nn) nn.hidden = v === 'overview';
    if (kc && slot && side) { (v === 'overview' ? slot : side).append(kc); if (v !== 'overview') slot.hidden = true; try { if (team) knowledge(focus); } catch (_) { /* booting: the net's state is not declared yet */ } }
    if (v === 'tree' && document.querySelector('.shell[data-drawer]')) document.querySelector('.shell').removeAttribute('data-drawer');
    requestAnimationFrame(() => netDraw());
    try { leaderboard(); } catch (_) { /* booting: the board's state is not declared yet */ }
  }
  for (const t of tabs) t.addEventListener('click', () => view(t.dataset.view));
  // the tablist's keys: arrows walk the tabs in the row (Home, End), Enter or Space opens one
  $('#tablist').addEventListener('keydown', ev => {
    const shown = tabs.filter(t => !t.hidden);
    const i = shown.indexOf(document.activeElement);
    if (i < 0) return;
    let to = -1;
    if (ev.key === 'ArrowRight') to = (i + 1) % shown.length;
    else if (ev.key === 'ArrowLeft') to = (i - 1 + shown.length) % shown.length;
    else if (ev.key === 'Home') to = 0;
    else if (ev.key === 'End') to = shown.length - 1;
    if (to < 0) return;
    ev.preventDefault();
    for (const t of shown) t.tabIndex = -1;
    shown[to].tabIndex = 0; shown[to].focus();
  });
  // ten views do not fit every row: what does not fit folds, last first, into "more" — the
  // chosen view always stays in the row; the menu is a menu (arrows, Home, End, Esc)
  const moreBox = $('#tabs-more'), moreBtn = $('#tabs-more-btn'), moreMenu = $('#tabs-menu'), tabsNav = $('#tabs'), tablist = $('#tablist');
  function fitTabs() {
    for (const t of tabs) t.hidden = false;
    moreBox.hidden = true;
    const room = () => tablist.clientWidth;
    const need = () => { let w = 0; for (const t of tabs) if (!t.hidden) w += t.offsetWidth + (parseFloat(getComputedStyle(t).marginInlineStart) || 0) + 4; return w; };
    if (need() <= room() + 1) { moreMenu.hidden = true; moreBtn.setAttribute('aria-expanded', 'false'); return; }
    moreBox.hidden = false;
    const folded = [];
    for (let i = tabs.length - 1; i >= 0 && need() > room() + 1; i--) {
      if (tabs[i].getAttribute('aria-selected') === 'true') continue;
      tabs[i].hidden = true; folded.unshift(tabs[i]);
    }
    $('#tabs-more-count').textContent = String(folded.length);
    moreBtn.setAttribute('aria-label', 'more views: ' + folded.map(t => t.textContent).join(', '));
    moreMenu.replaceChildren(...folded.map(t => el('button', { class: 'tabs-menu-item', role: 'menuitemradio', type: 'button', tabindex: -1, 'aria-checked': String(t.getAttribute('aria-selected') === 'true'), 'data-view': t.dataset.view, text: t.textContent,
      onclick: () => { menu(false); view(t.dataset.view); const back = tabs.find(x => x.dataset.view === t.dataset.view); (back && !back.hidden ? back : moreBtn).focus(); } })));
  }
  function menu(open) {
    moreMenu.hidden = !open;
    moreBtn.setAttribute('aria-expanded', String(open));
    if (open) { const items = [...moreMenu.children]; (items.find(x => x.getAttribute('aria-checked') === 'true') || items[0])?.focus(); }
  }
  moreBtn.addEventListener('click', () => menu(moreMenu.hidden));
  moreBtn.addEventListener('keydown', ev => { if (ev.key === 'ArrowDown' || ev.key === 'ArrowUp') { ev.preventDefault(); menu(true); } });
  moreMenu.addEventListener('keydown', ev => {
    const items = [...moreMenu.children], i = items.indexOf(document.activeElement);
    let to = -1;
    if (ev.key === 'ArrowDown') to = (i + 1) % items.length;
    else if (ev.key === 'ArrowUp') to = (i - 1 + items.length) % items.length;
    else if (ev.key === 'Home') to = 0;
    else if (ev.key === 'End') to = items.length - 1;
    else if (ev.key === 'Escape') { ev.preventDefault(); ev.stopPropagation(); menu(false); moreBtn.focus(); return; }
    else if (ev.key === 'Tab') { menu(false); return; }
    if (to < 0) return;
    ev.preventDefault(); items[to].focus();
  });
  document.addEventListener('pointerdown', ev => { if (!moreMenu.hidden && !moreBox.contains(ev.target)) menu(false); });
  new ResizeObserver(() => fitTabs()).observe(tabsNav);
  if (document.fonts && document.fonts.ready) document.fonts.ready.then(() => fitTabs());
  // the cockpit fills the window under the header and the tabs, whatever height they wrap to
  const chrome = () => { const h = ($('.topbar').offsetHeight || 0) + ($('.tabs').offsetHeight || 0); document.querySelector('.shell').style.setProperty('--shell-chrome', h + 'px'); };
  new ResizeObserver(chrome).observe($('.topbar'));
  chrome();
  // a row that scrolls sideways says so: the edge with more past it fades
  function edgeFade(row) {
    if (!row) return;
    const f = () => { row.toggleAttribute('data-more-start', row.scrollLeft > 2); row.toggleAttribute('data-more-end', row.scrollLeft + row.clientWidth < row.scrollWidth - 2); };
    row.addEventListener('scroll', f, { passive: true });
    new ResizeObserver(f).observe(row);
    f();
    return f;
  }
  const metaFade = edgeFade($('.topbar-meta'));
  const HASH0 = parseHash(), HASH = HASH0.params;
  // a link may preview a look for this page alone (#…&scheme=synthwave&theme=light)
  if (HASH.get('theme')) holdMode(HASH.get('theme'));
  if (HASH.get('scheme')) holdScheme(HASH.get('scheme'));
  // the page the workspace exposes to its other scripts (workspace.js): the helpers and the bus
  window.agentOneDash = { el, getJSON, word, pill, esc, num, store, bus, view: (v, arg) => view(v, arg), current: () => main.dataset.view, hashArg: () => parseHash().arg, setArg: arg => setHash(main.dataset.view, arg) };
  view(HASH0.view || 'overview', HASH0.arg);
  addEventListener('hashchange', () => { const h = parseHash(); view(h.view || 'overview', h.arg); });

  // ---------- markdown, safely: escape first, then a small grammar ----------
  function inline(s) {
    return esc(s)
      .replace(/`([^`]+)`/g, '<code>$1</code>')
      .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
      .replace(/(^|[\s(])\*([^*\s][^*]*)\*/g, '$1<em>$2</em>')
      .replace(/\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/g, '<a href="$2" rel="noopener noreferrer" target="_blank">$1</a>');
  }
  function md(text) {
    const out = [];
    const parts = String(text).split(/```/);
    parts.forEach((part, i) => {
      if (i % 2 === 1) {
        const body = part.replace(/^[^\n]*\n/, '');
        out.push('<pre><code>' + esc(body.replace(/\n$/, '')) + '</code></pre>');
        return;
      }
      let list = null;
      const flush = () => { if (list) { out.push('<' + list.tag + '>' + list.items.map(x => '<li>' + inline(x) + '</li>').join('') + '</' + list.tag + '>'); list = null; } };
      let para = [];
      const endPara = () => { if (para.length) { out.push('<p>' + inline(para.join(' ')) + '</p>'); para = []; } };
      for (const line of part.split('\n')) {
        let m;
        if (!line.trim()) { endPara(); flush(); continue; }
        if ((m = line.match(/^(#{1,4})\s+(.*)$/))) { endPara(); flush(); out.push('<h' + (m[1].length + 1) + '>' + inline(m[2]) + '</h' + (m[1].length + 1) + '>'); continue; }
        if ((m = line.match(/^\s*[-*]\s+(.*)$/))) { endPara(); if (!list || list.tag !== 'ul') { flush(); list = { tag: 'ul', items: [] }; } list.items.push(m[1]); continue; }
        if ((m = line.match(/^\s*\d+[.)]\s+(.*)$/))) { endPara(); if (!list || list.tag !== 'ol') { flush(); list = { tag: 'ol', items: [] }; } list.items.push(m[1]); continue; }
        if ((m = line.match(/^>\s?(.*)$/))) { endPara(); flush(); out.push('<blockquote>' + inline(m[1]) + '</blockquote>'); continue; }
        flush(); para.push(line);
      }
      endPara(); flush();
    });
    return out.join('');
  }

  // ---------- the console ----------
  const log = $('#log'), form = $('#ask-form'), field = $('#ask'), sendBtn = $('#send'), stopBtn = $('#interrupt');
  let turn = null, answer = null, answerText = '', thinking = null, paint = 0;
  const tools = new Map(), subagents = new Map(), choices = new Map();
  let live = { session: false };
  const history = []; let hIdx = -1;

  const nearBottom = () => log.scrollHeight - log.scrollTop - log.clientHeight < 80;
  function add(node, to) {
    const stick = nearBottom();
    $('#log-empty')?.remove();
    (to || turn || log).append(node);
    if (stick) log.scrollTop = log.scrollHeight;
    return node;
  }
  function newTurn(text, attrs) {
    finishAnswer();
    turn = el('div', { class: 'turn' });
    add(turn, log);
    if (text !== undefined) add(el('div', Object.assign({ class: 'turn-ask', text }, attrs || {})), turn);
    answer = null; answerText = ''; thinking = null;
    return turn;
  }
  function ensureTurn() { return turn || newTurn(); }
  // the wire's own lines (@S DONE, @U …) are for the binary; the reader sees them as one quiet note
  const WIRE = /^@[SUEFVT?]\s/;
  const prose = t => t.split('\n').filter(l => !WIRE.test(l)).join('\n');
  const wire = t => t.split('\n').filter(l => WIRE.test(l));
  function paintAnswer() {
    paint = 0;
    if (!answer) return;
    const stick = nearBottom();
    answer.innerHTML = md(prose(answerText));
    const last = answer.lastElementChild;
    if (last) last.setAttribute('data-cursor', '');
    if (stick) log.scrollTop = log.scrollHeight;
  }
  function finishAnswer() {
    if (!answer) return;
    answer.innerHTML = md(prose(answerText));
    const w = wire(answerText);
    if (w.length) add(el('p', { class: 'console-note', text: w.join(' · ') }), answer.parentElement);
    answer = null;
  }
  function note(text, tone) { return add(el('p', { class: 'console-note', 'data-tone': tone || false, text })); }

  function pill(state) {
    const tone = { done: 'ok', pass: 'ok', running: 'info', thinking: 'info', tool: 'info', queued: 'muted', idle: 'muted', 'waiting on gate': 'warn', gating: 'warn', failed: 'danger', fail: 'danger', denied: 'danger', refused: 'danger' }[state] || 'muted';
    const live = ['running', 'thinking', 'tool', 'gating', 'waiting on gate'].includes(state);
    return el('span', { class: 'pill', 'data-tone': tone, 'data-live': live, text: state || '—' });
  }
  function toolCard(t, kind) {
    const card = el('details', { class: 'toolcard', 'data-class': t.class || false, 'data-kind': kind || false });
    fillTool(card, t);
    return card;
  }
  function fillTool(card, t) {
    card.replaceChildren();
    const sum = el('summary', {},
      el('span', { class: 'toolcard-name', text: t.name || 'tool' }),
      el('span', { class: 'toolcard-sum', text: t.summary || '', title: t.summary || '' }),
      pill(t.status || 'running'),
      t.ms ? el('span', { class: 'toolcard-ms', text: t.ms < 1000 ? t.ms + ' ms' : (t.ms / 1000).toFixed(1) + ' s' }) : null);
    const body = el('div', { class: 'toolcard-body' });
    if (t.why) body.append(el('p', { class: 'toolcard-why', text: t.why }));
    if (t.diff && t.diff.lines && t.diff.lines.length) {
      const d = el('div', { class: 'diff', role: 'table', 'aria-label': 'diff ' + (t.diff.path || '') });
      for (const l of t.diff.lines) d.append(el('div', { class: 'diff-line', 'data-kind': l.k, role: 'row' }, el('span', { text: l.o || '' }), el('span', { text: l.n || '' }), el('span', { text: (l.k === '~' ? '⋯ ' : l.k + ' ') + l.t })));
      if (t.diff.truncated) d.append(el('div', { class: 'diff-line', 'data-kind': '~' }, el('span'), el('span'), el('span', { text: 'more lines not shown' })));
      body.append(d);
    }
    if (t.output) body.append(el('pre', { class: 'toolcard-out', text: t.output }));
    if (t.wrote && t.wrote.length) body.append(el('p', { class: 'console-note', text: 'wrote ' + t.wrote.join(', ') }));
    card.append(sum, body);
  }
  function courtCard(e) {
    let c = subagents.get(e.name);
    if (!c) {
      c = { card: el('details', { class: 'toolcard', 'data-kind': 'court' }), log: el('div', { class: 'stack', 'data-gap': 's' }), stream: '' };
      subagents.set(e.name, c);
      add(c.card, ensureTurn());
    }
    c.last = e;
    const open = c.card.open;
    c.card.replaceChildren();
    const secs = e.elapsedMs ? Math.round(e.elapsedMs / 1000) + ' s' : '';
    c.card.append(el('summary', {},
      el('span', { class: 'toolcard-name', text: e.name }),
      el('span', { class: 'toolcard-sum', text: [e.rank, e.role, e.model].filter(Boolean).join(' · ') + (e.ask ? ' — ' + e.ask : ''), title: e.ask || '' }),
      pill(e.failed ? 'failed' : e.state), secs ? el('span', { class: 'toolcard-ms', text: secs }) : null));
    const body = el('div', { class: 'toolcard-body' });
    if (e.report) {
      const r = e.report, lines = [];
      if (r.Status) lines.push('@S ' + r.Status);
      for (const f of r.Findings || []) lines.push('@F ' + f);
      for (const v of r.Verdicts || []) lines.push('@V ' + v);
      for (const h of r.Holes || []) lines.push('@? ' + h);
      for (const o of r.Other || []) lines.push(o);
      body.append(el('pre', { class: 'toolcard-out', text: lines.join('\n') }));
    }
    if (c.stream) body.append(el('pre', { class: 'toolcard-out', text: c.stream.slice(-4000) }));
    body.append(c.log);
    c.card.append(body);
    c.card.open = open;
    netSignal(e);
    liveNames.add(e.name);
    if (e.state === 'done' || e.failed) liveNames.delete(e.name);
    netLive();
  }
  function choiceCard(e) {
    const opts = el('div', { class: 'choice-options' });
    const card = el('div', { class: 'choice', role: 'group', 'aria-label': e.title || 'choice' },
      el('div', { class: 'choice-title', text: (e.title || 'Choice') + (e.class ? ' · ' + e.class : '') + (e.agent ? ' · ' + e.agent : '') }),
      e.tool || e.summary ? el('div', { class: 'choice-what', text: [e.tool, e.summary].filter(Boolean).join(': ') }) : null,
      e.why ? el('div', { class: 'choice-why', text: e.why }) : null, opts);
    const answerWith = (index, text) => {
      for (const b of card.querySelectorAll('button, input')) b.disabled = true;
      post('api/answer', { id: e.id, index, text }).catch(err => note('the answer did not land: ' + err.message, 'danger'));
    };
    if (e.elsewhere) {
      for (const o of e.options || []) opts.append(el('span', { class: 'pill', text: o }));
      card.append(el('div', { class: 'choice-why', text: 'answer it in the terminal' }));
    } else {
      (e.options || []).forEach((o, i) => {
        const reason = (e.reasons || [])[i];
        opts.append(el('button', { class: 'button', 'data-variant': i === 0 ? 'primary' : (i === e.options.length - 1 ? 'quiet' : false), type: 'button', text: o, onclick: () => {
          if (!reason) return answerWith(i, '');
          const inp = el('input', { 'aria-label': reason, placeholder: reason });
          const go = () => answerWith(i, inp.value.trim());
          inp.addEventListener('keydown', ev => { if (ev.key === 'Enter') go(); });
          card.append(el('div', { class: 'choice-free' }, inp, el('button', { class: 'button', type: 'button', text: 'Send', onclick: go })));
          inp.focus();
        } }));
      });
      if (e.free) {
        const inp = el('input', { 'aria-label': e.prompt || 'your answer', placeholder: e.prompt || 'your answer' });
        const go = () => answerWith(-1, inp.value.trim());
        inp.addEventListener('keydown', ev => { if (ev.key === 'Enter') go(); });
        card.append(el('div', { class: 'choice-free' }, inp, el('button', { class: 'button', type: 'button', text: 'Send', onclick: go })));
      }
      if (e.id) choices.set(e.id, card);
    }
    add(card, ensureTurn());
    setAsk('waiting');
  }

  // every action pulses on its own neuron: a session step on the orchestrator, a Subagent's on its member
  const CLASSES = ['read', 'write', 'gate', 'outward', 'destructive'];
  function netEvent(e) {
    const bad = st => /^(failed|fail|denied|refused)$/.test(String(st || ''));
    switch (e.type) {
      case 'tool': {
        const t = e.tool || {};
        if (e.phase === 'start') net.act(t.agent || '', CLASSES.includes(t.class) ? t.class : 'read');
        else if (bad(t.status)) net.act(t.agent || '', 'fail');
        break;
      }
      case 'bodystep': {
        const t = e.tool || {};
        if (!e.end) net.act(e.agent, CLASSES.includes(t.class) ? t.class : 'read');
        else if (bad(t.status)) net.act(e.agent, 'fail');
        break;
      }
      case 'state': if (e.state === 'waiting on gate' || e.state === 'gating') net.act(e.agent === orchestratorName() ? '' : e.agent, 'gate'); break;
      case 'turndone': {
        // green and red are for real verdicts; a plain turn's end is a calm ping in the rank's colour
        const failed = /fail|denied/i.test(e.verdict || '') || /FAIL|DENIED/.test(e.status || '');
        if (e.verdict || e.status) timelineAdd({ as: orchestratorName() || 'session', ask: (lastAsk || 'turn') + (e.verdict ? ' — verdict: ' + e.verdict : ''), at: new Date().toISOString(), started: turnAt, status: failed && e.verdict ? 'FAIL' : (e.status || 'DONE') });
        if (failed) net.act('', 'fail');
        else if (e.verdict) net.act('', /pass/i.test(e.verdict) ? 'pass' : 'gate');
        else net.act('', 'done');
        break;
      }
    }
  }
  let lastAsk = '', turnAt = '';
  const orchestratorName = () => { const m = net.model(); return m && m.orchestrator ? m.orchestrator.name : ''; };
  // an agent as the reader knows it: the orchestrator by the lexicon's word, never its internal name
  const labelOf = name => { const n = team && team.nodes.find(x => x.name === name && x.kind !== 'skill'); return n && n.orchestrator ? (n.label || n.name) : name; };
  function onEvent(e) {
    if ((e.type === 'ask' && !e.queued) || e.type === 'turnstart') { lastAsk = String(e.text || '').slice(0, 160); turnAt = new Date().toISOString(); now(); }
    treeEvent(e);
    netEvent(e);
    switch (e.type) {
      case 'ask': newTurn(e.text, { 'data-queued': e.queued || false }); if (!e.queued) setAsk('running'); break;
      case 'turnstart': newTurn(e.auto ? '(a report came in) ' + String(e.text || '').slice(0, 200) : e.text); setAsk('running'); break;
      case 'delta':
        ensureTurn();
        if (!answer) { answer = add(el('div', { class: 'turn-answer prose' })); answerText = ''; }
        answerText += e.text;
        if (!paint) paint = requestAnimationFrame(paintAnswer);
        break;
      case 'stream': {
        const c = subagents.get(e.agent);
        if (c) { c.stream = (c.stream + e.text).slice(-8000); if (c.last) courtCard(c.last); break; }
        if (e.kind !== 'thinking') break;
        ensureTurn();
        if (!thinking) {
          thinking = { box: el('details', { class: 'thinking' }, el('summary', { text: 'thinking…' }), el('div', { class: 'thinking-text' })), started: Date.now() };
          add(thinking.box);
        }
        thinking.box.lastChild.textContent += e.text;
        thinking.box.firstChild.textContent = 'thinking… ' + Math.round((Date.now() - thinking.started) / 1000) + ' s';
        break;
      }
      case 'tool': {
        const t = e.tool;
        const target = t.agent && subagents.get(t.agent) ? subagents.get(t.agent).log : null;
        let card = tools.get(t.id);
        if (!card) { card = toolCard(t); tools.set(t.id, card); add(card, target || ensureTurn()); }
        else fillTool(card, t);
        if (thinking && !t.agent) { thinking.box.firstChild.textContent = 'thought for ' + Math.round((Date.now() - thinking.started) / 1000) + ' s'; thinking = null; }
        if (answer && !t.agent) { finishAnswer(); }
        break;
      }
      case 'bodystep': {
        const c = subagents.get(e.agent);
        if (!c) break;
        let card = tools.get(e.tool.id);
        if (!card) { card = toolCard(e.tool); tools.set(e.tool.id, card); c.log.append(card); } else fillTool(card, e.tool);
        break;
      }
      case 'subagent': courtCard(e); break;
      case 'state':
        if (subagents.has(e.agent)) break;
        if (thinking && e.state !== 'thinking') { thinking.box.firstChild.textContent = 'thought for ' + Math.round((Date.now() - thinking.started) / 1000) + ' s'; thinking = null; }
        if (e.state === 'waiting on gate' || e.state === 'gating') setAsk('running', 'weighing the verdict');
        break;
      case 'turndone': {
        if (thinking) { thinking.box.firstChild.textContent = 'thought for ' + Math.round((Date.now() - thinking.started) / 1000) + ' s'; thinking = null; }
        if (!e.streamed && e.text) { ensureTurn(); answer = add(el('div', { class: 'turn-answer prose' })); answerText = e.text; }
        finishAnswer();
        const bits = [];
        if (e.interrupted) bits.push('interrupted');
        if (e.verdict) bits.push('verdict: ' + e.verdict);
        if (bits.length) note(bits.join(' · '), /fail/.test(e.verdict || '') ? 'danger' : false);
        for (const h of e.holes || []) note('@? ' + h, 'warn');
        if (e.hint) note(e.hint);
        if (e.status && !/^(DONE|done|Done)$/.test(e.status) && !e.interrupted) note('status ' + e.status, /FAIL|fail/.test(e.status) ? 'danger' : 'warn');
        setAsk('idle');
        break;
      }
      case 'notice': note(e.text); break;
      case 'evolution': evolution(e); break;
      case 'theme': look(e.name, e.mode); break;
      case 'error': note(e.text, 'danger'); break;
      case 'lines': add(el('pre', { class: 'console-lines', text: (e.lines || []).join('\n') })); break;
      case 'choice': choiceCard(e); break;
      case 'answered': { const c = choices.get(e.id); if (c) { c.setAttribute('data-answered', ''); choices.delete(e.id); } setAsk('running'); break; }
    }
  }

  function setAsk(state, label) {
    form.dataset.state = state;
    const p = pill(label || ({ idle: 'idle', running: 'thinking', waiting: 'waiting on you' }[state] || state));
    p.id = 'ask-state';
    $('#ask-state').replaceWith(p);
    stopBtn.disabled = state === 'idle' || !live.session;
  }

  function stream() {
    const es = new EventSource('dash/stream');
    es.addEventListener('reset', () => {
      log.replaceChildren(el('p', { class: 'console-empty', id: 'log-empty', text: 'Ask anything. The transcript reads like a page; tools, subagents and verdicts fold into cards.' }));
      turn = answer = thinking = null; tools.clear(); subagents.clear(); choices.clear(); subagentPhase.clear();
    });
    es.addEventListener('nosession', () => {
      $('#log-empty').textContent = 'No session here — this board was started without one. Run `' + word('bin', 'agent-one') + ' dash` (or `' + word('bin', 'agent-one') + '`) to talk to it.';
      field.disabled = sendBtn.disabled = true;
    });
    es.onmessage = m => { try { onEvent(JSON.parse(m.data)); } catch (err) { console.error(err); } };
  }

  form.addEventListener('submit', ev => {
    ev.preventDefault();
    const text = field.value.trim();
    if (!text) return;
    history.unshift(text); hIdx = -1;
    field.value = '';
    post('api/ask', { text }).catch(err => note('not sent: ' + err.message, 'danger'));
  });
  // (the field grows by itself: field-sizing: content in console.css)
  field.addEventListener('keydown', ev => {
    if (ev.key === 'Enter' && !ev.shiftKey && !ev.isComposing) { ev.preventDefault(); form.requestSubmit(); }
    else if (ev.key === 'ArrowUp' && field.selectionStart === 0 && history.length) { ev.preventDefault(); hIdx = Math.min(hIdx + 1, history.length - 1); field.value = history[hIdx]; }
    else if (ev.key === 'ArrowDown' && hIdx >= 0 && field.selectionStart === field.value.length) { ev.preventDefault(); hIdx--; field.value = hIdx >= 0 ? history[hIdx] : ''; }
    else if (ev.key === 'Escape') { ev.preventDefault(); interrupt(); }
  });
  function interrupt() { if (!stopBtn.disabled) post('api/interrupt').catch(err => note('interrupt: ' + err.message, 'danger')); }
  stopBtn.addEventListener('click', interrupt);
  document.addEventListener('keydown', ev => { if (ev.key === 'Escape' && document.activeElement !== field) interrupt(); });
  $('#fold-all').addEventListener('click', () => { for (const d of log.querySelectorAll('details.toolcard[open]')) d.open = false; });

  // ---------- the header ----------
  function header() {
    getJSON('api/live').then(s => {
      live = s;
      $('#world').textContent = s.workspace || '—';
      $('#model').textContent = s.model || '—';
      const st = s.session ? (s.running ? 'running' : 'idle') : 'no session';
      const p = pill(st); p.id = 'state';
      if (s.budgetsOff) p.title = 'step/time budgets off by this workspace\'s policy (-1) — the heartbeat watchdog carries the safety';
      $('#state').replaceWith(p);
      const m = $('#ctx');
      if (s.ctxKnown) {
        m.style.setProperty('--meter-value', s.ctxPct + '%');
        m.dataset.level = ctxLevel(s.ctxPct) || '';
        m.setAttribute('aria-valuenow', s.ctxPct);
      }
      m.querySelector('.meter-label').textContent = s.ctx || 'ctx —';
      $('#cost').textContent = s.cost || '—';
      const lc = $('#live-count');
      lc.hidden = !s.live; lc.textContent = (s.live || 0) + ' ' + word('agents', 'agents') + ' live';
      // the wedge banner: a workspace that waits on you says it (freeze 2026-10-07 policy:
      // waiting is a state the human must be able to SEE)
      const gw = $('#gatewait');
      const waitMs = s.gateWaitMs | 0, queueN = s.queued | 0;
      if (waitMs > 0) {
        gw.hidden = false;
        const mins = Math.floor(waitMs / 60000), secs = Math.floor((waitMs % 60000) / 1000);
        gw.textContent = 'waiting on you · ' + (s.choices || 1) + ' card(s) · ' + (mins > 0 ? mins + 'm ' : '') + secs + 's' + (queueN > 0 ? ' · ' + queueN + ' queued' : '');
        gw.dataset.tone = waitMs > 5 * 60 * 1000 ? 'danger' : 'warn';
      } else if (queueN > 0) {
        gw.hidden = false;
        gw.textContent = queueN + ' ask(s) queued behind a running turn';
        gw.dataset.tone = 'info';
      } else {
        gw.hidden = true;
      }
      // the orchestrator breathes while the session runs
      const tn = orchestratorName();
      if (tn && liveNames.has(tn) !== !!s.running) { if (s.running) liveNames.add(tn); else liveNames.delete(tn); netLive(); }
      if (s.session && form.dataset.state !== 'waiting') setAsk(s.running ? 'running' : 'idle');
      if (!s.session) { field.disabled = sendBtn.disabled = true; }
      now();
      if (metaFade) metaFade();
    }).catch(() => { const p = pill('board offline'); p.id = 'state'; $('#state').replaceWith(p); });
  }

  // ---------- the cockpit's now strip: state, the current ask, live agents, a wait on you ----------
  function now() {
    const box = $('#now');
    if (!box || box.hidden) return;
    const st = form.dataset.state || 'idle';
    const running = live.session && (live.running || st === 'running');
    const kids = [pill(st === 'waiting' ? 'waiting on you' : running ? 'running' : live.session ? 'idle' : 'no session')];
    kids.push(el('span', Object.assign({ class: 'now-ask', 'data-idle': !running }, lastAsk ? { text: (running ? '' : 'last: ') + lastAsk, title: lastAsk } : { text: running ? '…' : 'nothing asked yet' })));
    const agents = subagent.filter(b => b.state && !/^(done|idle)$/.test(b.state) && b.name !== orchestratorName());
    if (agents.length) kids.push(el('span', { class: 'now-agents' }, agents.map(b => {
      const pct = b.contextLimit ? Math.round(100 * (b.contextTokens || 0) / b.contextLimit) : -1;
      return el('button', { class: 'now-chip', type: 'button', 'data-rank': rankOfName(b.name) || false, title: b.name + ' · ' + b.state + (pct >= 0 ? ' · context ' + pct + '%' : ''),
        onclick: () => { const n = team && team.nodes.find(x => x.name === b.name && x.kind !== 'skill'); if (n) setFocus(n.id); } },
        labelOf(b.name), pct >= 0 ? el('span', { class: 'now-ctx', 'data-level': ctxLevel(pct), text: pct + '%' }) : null);
    })));
    const gw = $('#gatewait');
    if (!gw.hidden) kids.push(el('button', { class: 'now-wait', type: 'button', 'data-tone': gw.dataset.tone || 'warn', text: gw.textContent, title: gw.title,
      onclick: () => { const c = log.querySelector('.choice:not([data-answered]) button, .choice:not([data-answered]) input'); if (c) { c.scrollIntoView({ block: 'center' }); c.focus(); } else field.focus(); } }));
    box.replaceChildren(...kids);
  }

  // ---------- the neural net: the hierarchy as layers, skills beside (components/net/net.js) ----------
  // The module draws; this file feeds it (the team joined with the registered agents and every
  // wearable skill, the activity journals, the live subagent) and opens the knowledge card.
  const netBox = $('#net');
  let team = null, activity = null, focus = '';
  const liveNames = new Set();
  const SKILL = 'skill';
  const isSkill = n => n.kind === SKILL;
  const persistent = b => b && (b.mode === 'all' || b.mode === 'primary');
  // in the cockpit the net is the hero: it fits the panel it is given instead of the window
  const netAgent = netBox.closest('.panel-body');
  // the box is measured, not guessed: the panel agent's content height less everything else in the
  // net's stack (the legend, and the gaps between); 0 when the cockpit is not laid out (narrow,
  // or another view), and the net then takes the height it wants
  const netFit = () => {
    if (main.dataset.view !== 'overview' || getComputedStyle($('#dock-split')).display === 'none') return 0;
    const cs = getComputedStyle(netAgent);
    const stack = netBox.parentElement;
    const others = stack.getBoundingClientRect().height - netBox.getBoundingClientRect().height;
    return Math.floor(netAgent.clientHeight - parseFloat(cs.paddingTop) - parseFloat(cs.paddingBottom) - others);
  };
  const net = window.agentOneNet.create(netBox, { word, onFocus: id => setFocus(id, true), fit: netFit, meters: () => meters.session });
  let fitT = 0, fitH = 0;
  new ResizeObserver(() => { const h = netAgent.clientHeight; if (Math.abs(h - fitH) < 8) return; fitH = h; clearTimeout(fitT); fitT = setTimeout(netDraw, 120); }).observe(netAgent);
  function netDraw() { if (team) net.redraw(); }
  function netLive() { net.live(liveNames); }
  // the 2D · 3D switch: remembered per reader; a page without WebGL2 stays 2D and says why
  const modeBtns = [...document.querySelectorAll('#net-mode button')];
  function netMode(v) {
    const got = net.mode(v);
    // in 3D the cockpit gives the stage the timeline's room: the band folds to two rows
    main.dataset.net = got;
    for (const b of modeBtns) b.setAttribute('aria-pressed', String(b.dataset.mode === got));
    if (v) store.set('netmode', got);
  }
  for (const b of modeBtns) b.addEventListener('click', () => netMode(b.dataset.mode));
  if (!net.can3d()) { const b = modeBtns.find(x => x.dataset.mode === '3d'); if (b) { b.disabled = true; b.title = 'WebGL2 is not available here — the net stays 2D'; } }
  function findingsList() {
    const box = $('#net-findings');
    const items = [...team.findings.map(t => ({ t, tone: 'warn' })), ...team.notes.map(t => ({ t, tone: '' }))];
    box.replaceChildren(...(items.length ? [el('h3', { class: 'panel-note', text: team.findings.length + ' findings · ' + team.notes.length + ' notes — what the ontology says is missing' }),
      el('ul', { class: 'findings' }, items.map(i => el('li', { 'data-tone': i.tone || false, text: i.t })))] : []));
  }
  // the card over the net can be put away (✕) while its neuron stays selected; picking another
  // neuron, or the card button in the net's tools, brings it back
  let cardHidden = false;
  function setFocus(id, fromNet) {
    if (id !== focus) cardHidden = false;
    focus = id;
    if (!fromNet) net.focus(id);
    for (const th of document.querySelectorAll('#matrix th[data-id]')) th.toggleAttribute('data-hot', !!focus && th.dataset.id === focus);
    knowledge(id);
  }
  $('#net-clear').addEventListener('click', () => setFocus(''));
  $('#net-card').addEventListener('click', () => { cardHidden = false; knowledge(focus); });
  // a short, stable code for a neuron's ID block: its kind, then four hex of its id's hash
  const idCode = n => {
    let h = 2166136261;
    for (const ch of n.id) h = Math.imul(h ^ ch.charCodeAt(0), 16777619);
    const kind = n.orchestrator ? 'ROOT' : isSkill(n) ? 'SKILL' : (n.rank || 'AGENT').replace(/[^a-z]/gi, '').slice(0, 4).toUpperCase();
    return kind + '-' + ((h >>> 0) & 0xffff).toString(16).toUpperCase().padStart(4, '0');
  };
  function knowledge(id) {
    const box = $('#kcard');
    box.replaceChildren();
    const slot = box.parentElement && box.parentElement.id === 'net-float' ? box.parentElement : null;
    if (slot) slot.hidden = !id || cardHidden;
    const reopen = $('#net-card');
    if (reopen) reopen.hidden = !(slot && id && cardHidden);
    if (!id || !team) { if (main.dataset.view === 'overview') return; box.append(el('p', { class: 'net-hint', text: 'Hover a neuron to light its path to the ' + word('rank.orchestrator', 'orchestrator') + ' and everything below it; click (or Enter) pins it here. Each tile is four weeks of that ' + word('agent', 'agent') + "'s work, a cell a day — blank where no instrument saw any." })); return; }
    const n = team.nodes.find(x => x.id === id);
    if (!n) return;
    const m = net.model();
    const rows = [];
    const row = (k, v) => { if (v) rows.push(el('div', { class: 'kcard-row' }, el('dt', { text: k }), el('dd', { text: v }))); };
    const a = activity && activity.agents && activity.agents[n.name];
    if (isSkill(n)) {
      row('serves', label2(n.lane));
      row('source', n.source);
      row('workingNotes', n.workingNotes ? n.workingNotes.notes + ' / ' + n.workingNotes.limit + ' notes' + (n.workingNotes.stress && n.workingNotes.stress !== 'ok' ? ' — ' + n.workingNotes.stress : '') + (n.workingNotes.heldBy ? ' · held by ' + n.workingNotes.heldBy : '') : 'no working notes yet');
      row('held by', team.edges.filter(e => e.bond === 'holds' && e.to === id).map(e => nameOf(e.from)).join(', ') || '—');
      row('what', n.description);
    } else {
      const p = m && m.parent.get(id);
      const path = [];
      for (let x = p, g = 0; x && g < 32; x = m.parent.get(x), g++) path.push(nameOf(x));
      if (path.length) row('answers to', path.join(' → '));
      const kids = (m && m.kids.get(id)) || [];
      if (kids.length) row('below', kids.length + ' — ' + kids.map(nameOf).join(', '));
      row(word('ownership', 'ownership'), (n.owns || []).join(', ') || '—');
      row(word('skills', 'skills'), team.edges.filter(e => e.bond === 'holds' && e.from === id).map(e => nameOf(e.to)).join(', ') || '—');
      row('agent', n.agent ? (persistent(n.agent) ? 'persistent' : 'subagent') + ' · ' + (n.agent.model || 'the session model') + ' · ' + n.agent.source : 'not registered — runs in the session');
      row('work', a ? a.total + ' run' + (a.total === 1 ? '' : 's') + (a.failed ? ' (' + a.failed + ' not done)' : '') + ' · ' + num(a.tokens.reduce((x, y) => x + y, 0)) + ' tokens in 4 weeks' + (a.last ? ' · last ' + a.last.slice(0, 16).replace('T', ' ') : '') : 'no journaled work in 4 weeks');
    }
    const bonds = team.edges.filter(e => (e.from === id || e.to === id) && e.bond !== 'holds').map(e => (e.from === id ? word('bond.' + e.bond, e.bond) + ' → ' + nameOf(e.to) : nameOf(e.from) + ' → ' + word('bond.' + e.bond, e.bond)) + (e.implied ? ' (implied)' : ''));
    row('bonds', bonds.join(' · ') || '—');
    const docPath = n.doc || (n.workingNotes && n.workingNotes.src) || '';
    const doc = el('pre', { class: 'kcard-doc', text: docPath ? 'reading ' + docPath + '…' : 'no doc — the ontology notes it' });
    const plan = el('pre', { class: 'kcard-doc', text: '' });
    if (!isSkill(n) && n.value) row('value', 'score ' + Math.round(n.value.score) + (n.value.confidence === 'low' ? ' (below its record — unranked)' : '') + ' · ' + n.value.meter.label + ' ' + n.value.meter.text + ' · strain ' + n.value.strain);
    if (n.mass) row('size', isSkill(n) ? n.mass + ' holding it' : num(n.mass) + ' lines of code owned');
    // the rank's face: its resolved portrait, else its family's glyph — large, ringed in the rank's hue
    const icon = n.icon || null;
    const face = icon ? el('figure', { class: 'kcard-face' },
      icon.kind === 'portrait' ? el('img', { class: 'kcard-portrait', src: icon.url, alt: word('rank.' + n.rank, n.rank || 'agent'), width: 112, height: 112 })
        : svg('svg', { class: 'kcard-glyph', viewBox: '0 0 24 24', 'aria-hidden': 'true' }, svg('use', { href: icon.url })),
      el('figcaption', { text: isSkill(n) ? word('skill', 'skill') : word('rank.' + n.rank, n.rank || 'agent') + (n.family ? ' · ' + n.family : '') })) : null;
    const floating = main.dataset.view === 'overview';
    const idBlock = floating ? el('div', { class: 'kcard-id' },
      el('span', { class: 'kcard-id-tag', text: 'ID //' }), el('span', { class: 'kcard-id-code', text: idCode(n) }),
      el('span', { class: 'kcard-id-state', 'data-live': liveNames.has(n.name), text: liveNames.has(n.name) ? 'live' : 'idle' }),
      el('span', { class: 'kcard-id-bars', 'aria-hidden': 'true' })) : null;
    const hide = floating ? el('button', { class: 'kcard-x', type: 'button', text: '✕', title: 'hide the card — the neuron stays selected',
      'aria-label': 'hide the card; the neuron stays selected', onclick: () => { cardHidden = true; knowledge(focus); } }) : null;
    box.append(el('article', { class: 'kcard', 'data-float': floating, 'data-rank': n.orchestrator ? 'orchestrator' : (isSkill(n) ? false : n.rank || false), 'data-family': n.family || false },
      hide, idBlock,
      face,
      el('div', { class: 'kcard-title' }, n.orchestrator ? (n.label || n.name) : n.name, el('span', { class: 'pill', 'data-tone': 'info', text: isSkill(n) ? word('skill', 'skill') : word('rank.' + n.rank, n.rank || 'agent') }), liveNames.has(n.name) ? pill('running') : null,
        floating ? null : el('button', { class: 'button kcard-close', 'data-size': 's', 'data-variant': 'quiet', type: 'button', text: 'close', 'aria-label': 'clear the focus', onclick: () => setFocus('') })),
      el('dl', {}, rows), doc, isSkill(n) ? null : plan));
    if (docPath) fetch('api/doc?path=' + encodeURIComponent(docPath)).then(r => r.ok ? r.text() : Promise.reject(new Error(r.status)))
      .then(t => { doc.textContent = t.split('\n').slice(0, 40).join('\n'); }).catch(() => { doc.textContent = docPath + ' — not readable here'; });
    if (!isSkill(n)) fetch('/api/plan?agent=' + encodeURIComponent(id)).then(r => r.ok ? r.json() : Promise.reject(new Error(r.status)))
      .then(p => {
        if (!p || p.empty || !(p.items || p.deck || []).length) { plan.remove(); return; }
        const head = 'plan ' + (p.agent || id) + (p.updated ? ' · ' + p.updated : '');
        plan.textContent = head + '\n' + (p.items || p.deck).map(it => `[${it.status || '·'}] ${it.text || ''}`).join('\n');
      }).catch(() => { plan.remove(); });
  }
  const label2 = id => ((team && team.labels.find(l => l.id === id)) || {}).label || id;
  const nameOf = id => { const n = team && team.nodes.find(x => x.id === id); return n ? (n.orchestrator ? n.label || n.name : n.name) : id; };
  // the ontology's graph, joined with the two planes it does not draw: registered agents ride their
  // members (the orchestrator's own agent rides the orchestrator, wherever it was registered), every wearable
  // skill joins (the binary's own included), and each skill carries its working notes
  function loadTeam() {
    return Promise.all([getJSON('api/team'), getJSON('api/workspace').catch(() => ({ agents: [], skills: [], workingNotes: [] })), getJSON('api/activity').catch(() => null)]).then(([c, w, act]) => {
      c.nodes = c.nodes || []; c.edges = c.edges || []; c.lanes = c.lanes || []; c.labels = c.labels || []; c.findings = c.findings || []; c.notes = c.notes || [];
      // a skill's lane is drawn as skill:<lane> (the lane order and labels speak that way)
      for (const n of c.nodes) if (n.kind === 'skill' && c.lanes.includes('skill:' + n.lane)) n.lane = 'skill:' + n.lane;
      const byName = new Map(c.nodes.map(n => [n.name, n]));
      const orchestrator = c.nodes.find(n => n.id === c.orchestrator || n.orchestrator);
      const ranks = c.lanes.filter(l => !l.includes(':'));
      for (const b of w.agents || []) {
        const n = byName.get(b.name);
        if (n) { n.body = b; continue; }
        let rank = ranks.find(r => b.name === r || b.name.startsWith(r + '-'));
        // a machine-wide agent (~/.config/…) is another workspace's unless it rides a member here or
        // carries one of this workspace's rank prefixes — the net draws this team, not the machine
        const prefix = b.name.split('-')[0].replace(/_/g, '');
        if (/-global$/.test(b.source || '') && !rank && !WORDS['rank.' + prefix]) continue;
        if (!rank && WORDS['rank.' + prefix]) rank = prefix;
        const node = { id: 'agent-' + b.name, name: b.name, kind: 'member', rank: rank || '', lane: rank || 'agents:other', owns: [], body: b, description: b.description, parent: orchestrator ? orchestrator.id : '' };
        c.nodes.push(node); byName.set(b.name, node);
      }
      const shared = c.lanes.find(l => l.endsWith(':shared')) || 'skill:shared';
      for (const s of w.skills || []) {
        const n = byName.get(s.name);
        if (n) { n.source = n.source || s.source; n.description = n.description || s.description; continue; }
        if (!c.lanes.includes(shared)) c.lanes.push(shared);
        const node = { id: 'skill-' + s.name, name: s.name, kind: 'skill', lane: shared, shared: true, source: s.source, description: s.description, owns: [] };
        c.nodes.push(node); byName.set(s.name, node);
      }
      // a working notes sits in a skill, or in a member's own hat (<name>@hat, held by that member)
      for (const d of w.workingNotes || []) { const n = byName.get(String(d.skill).replace(/@hat$/, '')); if (n && (d.notes || !n.workingNotes)) n.workingNotes = d; }
      team = c; activity = act;
      const m = net.set(c, act);
      netLive();
      const agents = m.members.length, skills = m.skills.length;
      const lit = act && act.lit ? Object.keys(act.agents || {}).length + ' with journaled work' : 'no activity journal yet';
      const nn = $('#net-note');
      nn.textContent = agents + ' ' + word('members', 'members') + ' · ' + skills + ' ' + word('skills', 'skills') + ' · ' + m.tree.length + ' tree bonds · ' + (m.maxDepth + 1) + ' layers · ' + lit + (c.findings.length ? ' · ' + c.findings.length + ' findings' : '');
      nn.title = nn.textContent;
      findingsList();
      if (focus) knowledge(focus); else knowledge('');
      timeline();
      relations();
      leaderboard();
    }).catch(err => { console.error(err); $('#net-note').textContent = 'the ontology is silent'; });
  }
  // a Subagent on the stream: a pulse down when it is commissioned, back up when it reports
  const subagentPhase = new Map();
  function netSignal(e) {
    const was = subagentPhase.get(e.name);
    const done = e.state === 'done' || !!e.report || e.failed;
    if (!done && !was) { subagentPhase.set(e.name, 'down'); net.signal('down', e.name); const at = new Date().toISOString(); timelineAdd({ as: e.name, ask: e.ask, at, started: at, live: true }); }
    if (done && was !== 'up') {
      subagentPhase.set(e.name, 'up');
      const st = String((e.report && e.report.Status) || e.exit || '').toUpperCase();
      net.signal('up', e.name, e.failed || /FAIL|DENIED/.test(st) ? 'danger' : /ESCALATE|INTERRUPT/.test(st) || (st && !/DONE|PASS/.test(st)) ? 'warn' : 'ok');
      setTimeout(() => { if (subagentPhase.get(e.name) === 'up') subagentPhase.delete(e.name); }, 1500);
      timelineAdd({ as: e.name, ask: e.ask, at: new Date().toISOString(), status: st || (e.failed ? 'FAIL' : 'DONE') });
    }
  }

  // ---------- evolution (policy.evolution): a stressed member healed itself ----------
  // a timeline row ("zone-x evolved: split → zone-x-a, zone-x-b"), a ring in the arm's hue on
  // its neuron, and the team reloaded — the young (or the new hat) appear, and ring too
  const ARM_TONE = { split: 's1', mutate: 's2', rebirth: 's6' };
  function evolution(e) {
    const hole = e.status === 'hole';
    note('✦ ' + (e.line || e.agent + ' evolved') + (e.reason && !hole ? ' — ' + e.reason : ''), hole ? 'danger' : e.status === 'proposed' ? 'warn' : false);
    timelineAdd({ as: e.agent, ask: e.line || '', at: new Date().toISOString(), status: hole ? 'HOLE' : e.status === 'proposed' ? 'PROPOSED' : e.status === 'rejected' ? 'REJECTED' : 'EVOLVED' });
    const tone = hole ? 'danger' : ARM_TONE[e.arm] || 's6';
    net.act(e.agent, 'evolve', tone);
    if (e.status !== 'applied') return;
    loadTeam().then(() => { for (const n of e.into || []) if (n !== e.agent) net.act(n, 'evolve', tone); }).catch(() => {});
  }

  // ---------- the timeline: subagents and verdicts, newest first ----------
  // journaled runs (/api/activity's recent) beside what this page saw live (subagent starts and
  // reports, the session's turns), merged by time; a journal row is dropped only when it is the
  // same run as a live one (same agent, started within a few seconds of it). A row focuses its
  // neuron. Rows are keyed: a refresh rebuilds only the rows that changed, and a polite announcer
  // says only what is new — the list itself is not a live region.
  const liveRows = [];
  const SAME_RUN_MS = 8000;
  const tone = st => { const x = String(st || '').toUpperCase(); return !x ? 'info' : /^(DONE|PASS|EVOLVED)/.test(x) ? 'ok' : /FAIL|DENIED|HOLE/.test(x) ? 'danger' : /RUNNING/.test(x) ? 'info' : 'warn'; };
  const rankOfName = name => { const n = team && team.nodes.find(x => x.name === name && x.kind !== 'skill'); return n ? n.rank : ''; };
  function timelineAdd(row) {
    row.ask = String(row.ask || '').split('\n').filter(l => l.trim() && !/^\s*@/.test(l)).join(' ');
    const i = liveRows.findIndex(r => r.as === row.as && r.live);
    if (i >= 0) { row.started = row.started || liveRows[i].started; liveRows.splice(i, 1); }
    row.key = 'L' + row.as + '|' + (row.started || row.at);
    liveRows.unshift(row);
    if (liveRows.length > 40) liveRows.length = 40;
    timeline();
  }
  const tlEls = new Map();
  let tlSaid = new Set();
  const announcer = el('p', { class: 'visually-hidden', 'aria-live': 'polite' });
  document.body.append(announcer);
  function timeline() {
    const box = $('#timeline');
    if (!box) return;
    const t = x => { const v = Date.parse(x || ''); return isNaN(v) ? 0 : v; };
    const journal = ((activity && activity.recent) || []).filter(j => !liveRows.some(r => r.as === j.as && Math.abs(t(r.started || r.at) - t(j.at)) < SAME_RUN_MS))
      .map(j => Object.assign({ key: 'J' + (j.id || j.as + '|' + j.at) }, j));
    const rows = [...liveRows, ...journal].sort((a, b) => t(b.at) - t(a.at)).slice(0, 40);
    if (!rows.length) { tlEls.clear(); box.replaceChildren(el('li', {}, el('p', { class: 'timeline-empty', text: 'no journaled run in four weeks — a Subagent or a turn lands here as it happens' }))); return; }
    const hm = at => { const d = new Date(at); return isNaN(d) ? '' : (Date.now() - d < 864e5 ? d.toTimeString().slice(0, 5) : d.toISOString().slice(5, 10)); };
    const fresh = [];
    const items = rows.map(r => {
      const who = labelOf(r.as), end = r.live ? 'running' : (r.status || '—').split(/\s/)[0];
      const sig = [who, r.ask, end, hm(r.at), rankOfName(r.as)].join('|');
      let it = tlEls.get(r.key);
      if (it && it.sig === sig) return it.li;
      const nodeOf = () => team && team.nodes.find(x => x.name === r.as && x.kind !== 'skill');
      const li = el('li', {}, el('button', { class: 'timeline-item', type: 'button', 'data-rank': rankOfName(r.as) || false, 'data-live': !!r.live,
        'aria-label': who + ' — ' + (r.ask || '') + ' — ' + end,
        onclick: () => { const n = nodeOf(); if (n) setFocus(n.id); },
        onpointerenter: () => { const n = nodeOf(); net.hover(n ? n.id : ''); },
        onpointerleave: () => net.hover('') },
        el('span', { class: 'timeline-time', text: hm(r.at) }), el('span', { class: 'timeline-dot' }), el('span', { class: 'timeline-who', text: who }),
        el('span', { class: 'timeline-what', text: r.ask || '' }), el('span', { class: 'timeline-end', 'data-tone': r.live ? 'info' : tone(r.status), text: end })));
      tlEls.set(r.key, { li, sig });
      if (r.key[0] === 'L' && !tlSaid.has(r.key + end)) fresh.push(who + ' ' + end + (r.ask ? ': ' + r.ask : ''));
      return li;
    });
    const keep = new Set(rows.map(r => r.key));
    for (const k of [...tlEls.keys()]) if (!keep.has(k)) tlEls.delete(k);
    // move only what moved: walk the wanted order, inserting where the dom differs
    let at = box.firstChild;
    if (at && !items.includes(at)) { box.replaceChildren(); at = null; }
    for (const li of items) { if (li !== at) box.insertBefore(li, at); else at = at.nextSibling; }
    while (at) { const nx = at.nextSibling; at.remove(); at = nx; }
    for (const r of rows) if (r.key[0] === 'L') tlSaid.add(r.key + (r.live ? 'running' : (r.status || '—').split(/\s/)[0]));
    if (tlSaid.size > 400) tlSaid = new Set([...tlSaid].slice(-200));
    if (fresh.length) announcer.textContent = fresh.slice(0, 2).join(' · ');
    $('#timeline-note').textContent = rows.length + ' · subagents and verdicts, newest first';
  }
  // the instruments moved: tiles, bond weights and the timeline follow (at most every few seconds)
  let actT = 0;
  function refreshActivity() {
    clearTimeout(actT);
    actT = setTimeout(() => getJSON('api/activity').then(a => { activity = a; if (team) net.set(team, a); timeline(); leaderboard(); }).catch(() => {}), 2500);
  }

  // ---------- the cockpit: the console docks, folds and resizes ----------
  const shellMain = $('#main'), split = $('#dock-split'), dockBtn = $('#dock-toggle');
  function dock(open, save) {
    shellMain.dataset.dock = open ? 'open' : 'closed';
    dockBtn.setAttribute('aria-expanded', String(open));
    dockBtn.textContent = open ? '⇥' : '⇤';
    dockBtn.title = open ? 'fold the console to a rail' : 'open the console';
    if (save) store.set('dock', open ? 'open' : 'closed');
    requestAnimationFrame(() => {
      netDraw();
      // the separator's value is the console's real width, in rem
      const rem = parseFloat(getComputedStyle(document.documentElement).fontSize) || 16, w = $('#console-panel').getBoundingClientRect().width / rem;
      if (w > 0) split.setAttribute('aria-valuenow', String(Math.round(w)));
    });
  }
  function dockWidth(rem, save) {
    rem = Math.max(20, Math.min(60, rem));
    shellMain.style.setProperty('--shell-dock', rem.toFixed(1) + 'rem');
    split.setAttribute('aria-valuenow', String(Math.round(rem)));
    if (save) store.set('dockw', rem.toFixed(1));
  }
  dockBtn.addEventListener('click', () => dock(shellMain.dataset.dock === 'closed', true));
  if (store.get('dockw')) dockWidth(+store.get('dockw'));
  dock(store.get('dock') !== 'closed');
  split.addEventListener('pointerdown', ev => {
    const rem = parseFloat(getComputedStyle(document.documentElement).fontSize) || 16;
    const right = shellMain.getBoundingClientRect().right - parseFloat(getComputedStyle(shellMain).paddingRight);
    split.setPointerCapture(ev.pointerId); split.setAttribute('data-drag', '');
    const move = e => dockWidth((right - e.clientX) / rem);
    const up = e => { split.releasePointerCapture(e.pointerId); split.removeAttribute('data-drag'); split.removeEventListener('pointermove', move); split.removeEventListener('pointerup', up); dockWidth((right - e.clientX) / rem, true); netDraw(); };
    split.addEventListener('pointermove', move); split.addEventListener('pointerup', up);
  });
  split.addEventListener('keydown', ev => {
    const cur = parseFloat(shellMain.style.getPropertyValue('--shell-dock')) || 30;
    if (ev.key === 'ArrowLeft') dockWidth(cur + 2, true); else if (ev.key === 'ArrowRight') dockWidth(cur - 2, true); else return;
    ev.preventDefault();
  });

  // ---------- the folder tree as a drawer: the files button or `t`, Esc or a click away closes ----------
  const shellEl = $('.shell'), drawerBtn = $('#drawer-open');
  let drawerFrom = null;
  function drawer(open) {
    if (main.dataset.view === 'tree') open = false;
    shellEl.toggleAttribute('data-drawer', open);
    drawerBtn.setAttribute('aria-expanded', String(open));
    // open, the drawer is a modal dialog: the rest of the page is out of reach until it closes
    const panel = $('#tree-panel');
    if (open) { panel.setAttribute('role', 'dialog'); panel.setAttribute('aria-modal', 'true'); } else { panel.removeAttribute('role'); panel.removeAttribute('aria-modal'); }
    if (open) { drawerFrom = document.activeElement; setTimeout(() => ($('#tree').querySelector('.tree-node') || $('#drawer-close')).focus(), 50); }
    else { net.hover(''); if (drawerFrom && drawerFrom.focus) drawerFrom.focus(); drawerFrom = null; }
  }
  drawerBtn.addEventListener('click', () => drawer(!shellEl.hasAttribute('data-drawer')));
  $('#drawer-close').addEventListener('click', () => drawer(false));
  $('#scrim').addEventListener('click', () => drawer(false));
  document.addEventListener('keydown', ev => {
    const typing = /^(INPUT|TEXTAREA|SELECT)$/.test(document.activeElement && document.activeElement.tagName) || (document.activeElement && document.activeElement.isContentEditable);
    if (ev.key === 'Escape' && shellEl.hasAttribute('data-drawer')) { ev.preventDefault(); ev.stopImmediatePropagation(); drawer(false); return; }
    // the focus stays inside the open drawer: Tab and Shift+Tab wrap around it
    if (ev.key === 'Tab' && shellEl.hasAttribute('data-drawer')) {
      const panel = $('#tree-panel');
      const f = [...panel.querySelectorAll('button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])')].filter(x => !x.disabled && x.offsetParent !== null);
      if (!f.length) return;
      const first = f[0], last = f[f.length - 1], at = document.activeElement;
      if (!panel.contains(at)) { ev.preventDefault(); first.focus(); }
      else if (ev.shiftKey && at === first) { ev.preventDefault(); last.focus(); }
      else if (!ev.shiftKey && at === last) { ev.preventDefault(); first.focus(); }
      return;
    }
    // `t` anywhere but a field; Alt+T even from the prompt
    const key = ev.code === 'KeyT' || ev.key === 't' || ev.key === 'T';
    if (key && ((!typing && !ev.ctrlKey && !ev.metaKey && !ev.altKey) || (ev.altKey && !ev.ctrlKey && !ev.metaKey))) { ev.preventDefault(); drawer(!shellEl.hasAttribute('data-drawer')); }
  }, true);
  // a path in the tree lights the neuron that owns it (its ownership, longest claim wins)
  function ownerOf(path) {
    if (!team || !path) return '';
    const p = String(path).toLowerCase().replace(/\/+$/, '');
    let best = '', len = -1, deep = -1;
    const m = net.model();
    for (const n of team.nodes) for (const o of n.owns || []) {
      const t = String(o).toLowerCase().replace(/\/(\*\*?)?$/, '').replace(/^\.\//, '');
      const d = (m && m.depth.get(n.id)) || 0;
      // the longest claim wins; on a tie the deeper member (the one that does the work) does
      if (t && (p === t || p.startsWith(t + '/')) && (t.length > len || (t.length === len && d > deep))) { best = n.id; len = t.length; deep = d; }
    }
    return best;
  }
  const treeEl = $('#tree');
  treeEl.addEventListener('pointerover', ev => { const row = ev.target.closest('.tree-node'); if (row) net.hover(ownerOf(row.dataset.path)); });
  treeEl.addEventListener('pointerleave', () => net.hover(''));
  treeEl.addEventListener('focusin', ev => { const row = ev.target.closest('.tree-node'); if (row) net.hover(ownerOf(row.dataset.path)); });

  // ---------- the plan's progress in the header ----------
  function planMeter() {
    getJSON('/api/plan').then(p => {
      const m = $('#plan'), items = (p && (p.items || p.deck)) || [];
      if (!items.length) { m.hidden = true; return; }
      const done = items.filter(i => i.status === 'done').length;
      const pct = Math.round(100 * done / items.length);
      m.hidden = false;
      m.style.setProperty('--meter-value', pct + '%');
      m.setAttribute('aria-valuenow', String(pct));
      m.querySelector('.meter-label').textContent = 'plan ' + done + '/' + items.length;
      m.dataset.level = items.some(i => i.status === 'blocked') ? 'warn' : '';
    }).catch(() => { $('#plan').hidden = true; });
  }

  // ---------- relations: the matrix and its dimensions ----------
  function relations() {
    if (!team) return;
    const nodes = [...team.nodes].sort((a, b) => (team.lanes || []).indexOf(a.lane) - (team.lanes || []).indexOf(b.lane) || a.name.localeCompare(b.name));
    const bond = new Map();
    for (const e of team.edges) bond.set(e.from + '|' + e.to, e.bond);
    const table = el('table', { class: 'matrix' });
    const head = el('tr', {}, el('th'));
    for (const n of nodes) head.append(el('th', { scope: 'col', 'data-id': n.id, text: n.name }));
    table.append(el('thead', {}, head));
    const body = el('tbody');
    for (const r of nodes) {
      const tr = el('tr', {}, el('th', { scope: 'row', 'data-id': r.id, text: r.name }));
      for (const c of nodes) {
        const b = bond.get(r.id + '|' + c.id);
        tr.append(el('td', b ? { 'data-bond': b, title: r.name + ' — ' + b + ' → ' + c.name, onclick: () => { setFocus(r.id); } } : {}));
      }
      body.append(tr);
    }
    table.append(body);
    $('#matrix').replaceChildren(table);
    const facet = (title, counts) => {
      if (!Object.keys(counts).length) return el('div', { class: 'stack', 'data-gap': 's' }, el('h3', { class: 'panel-note', text: title }), el('p', { class: 'panel-note', text: 'none yet' }));
      const max = Math.max(1, ...Object.values(counts));
      return el('div', { class: 'stack', 'data-gap': 's' }, el('h3', { class: 'panel-note', text: title }),
        el('div', { class: 'bars' }, Object.entries(counts).sort((a, b) => b[1] - a[1]).map(([k, v]) =>
          el('div', { class: 'bars-row' }, el('span', { class: 'bars-label', text: k }), el('span', { class: 'bars-track' }, el('span', { class: 'bars-fill', style: { '--bars-value': (100 * v / max).toFixed(1) + '%' } })), el('span', { class: 'bars-figure', text: String(v) })))));
    };
    const count = (xs, f) => xs.reduce((m, x) => { const k = f(x) || '—'; m[k] = (m[k] || 0) + 1; return m; }, {});
    $('#facets').replaceChildren(
      facet('by ' + word('rank', 'rank'), count(team.nodes, n => n.kind === 'skill' ? word('skill', 'skill') : word('rank.' + n.rank, n.rank))),
      facet('by lane', count(team.nodes, n => ((team.labels || []).find(l => l.id === n.lane) || {}).label || n.lane)),
      facet('by bond', count(team.edges, e => word('bond.' + e.bond, e.bond))));
    $('#rel-note').textContent = nodes.length + ' × ' + nodes.length + ' · ' + team.edges.length + ' bonds' + ((team.findings || []).length ? ' · ' + team.findings.length + ' findings' : '');
  }

  // ---------- the value policy: meters, bands, the leaderboard ----------
  // /api/meters is the one source of every band: the session's ctx and plan meters (ctx derived
  // from policy.budget) and each rank's meter (policy.value's family, or the rank's own). Nothing here
  // carries a threshold of its own.
  let meters = { session: {}, ranks: {} }, valueRep = null;
  function loadValue() {
    return Promise.all([getJSON('api/meters').catch(() => null), getJSON('api/value').catch(() => null)]).then(([m, v]) => {
      if (m) meters = { session: m.session || {}, ranks: m.ranks || {} };
      valueRep = v && v.lit ? v.report : null;
      leaderboard(); rankKpis();
    });
  }
  // a reading against a meter's bands: ok · warn · bad (direction down: lower is better)
  function bandOf(def, v) {
    if (!def || !def.bands || v === null || v === undefined || isNaN(v)) return 'none';
    const b = def.bands;
    if (def.direction === 'down') return v <= b.ok ? 'ok' : v <= b.warn ? 'warn' : 'bad';
    return v >= b.ok ? 'ok' : v >= b.warn ? 'warn' : 'bad';
  }
  // the context level the header meter and the chips hold, from the ctx meter's bands
  function ctxLevel(pct) {
    const band = bandOf(meters.session.ctx, pct);
    return band === 'bad' ? 'danger' : band === 'warn' ? 'warn' : false;
  }
  const toneOf = band => band === 'bad' ? 'danger' : band === 'warn' ? 'warn' : band === 'ok' ? 'ok' : false;
  const pctText = v => (v === null || v === undefined || isNaN(v)) ? '—' : Math.round(v) + '%';
  // the per-rank KPI row: each rank by its own meter (the mean of its confident agents)
  function rankKpis() {
    const box = $('#rank-kpis');
    if (!box) return;
    if (!valueRep) { box.replaceChildren(kpi('value', '—', 'silent: no session keeps the value ledger', true)); return; }
    const ranks = (valueRep.ranks || []).filter(r => r.agents > 0);
    if (!ranks.length) { box.replaceChildren(kpi('value', '—', 'no agent on the roster yet', true)); return; }
    $('#rank-kpis-note').textContent = 'each rank, by its own measure — window ' + (valueRep.window || 'all');
    box.replaceChildren(...ranks.map(r => {
      const sure = r.confident > 0;
      return kpi(word('rank.' + r.rank, r.rank) + ' · ' + (r.meter.label || r.meter.signal), sure ? r.reading.text : '—',
        sure ? 'score ' + Math.round(r.mean) + ' · ' + r.confident + ' of ' + r.agents + ' ranked' : r.agents + ' below the ' + r.minRecord + '-report record', !sure, sure ? toneOf(r.reading.band) : false);
    }));
  }
  // the leaderboard: the members ranked by their rank's value score; an agent under its rank's
  // record is listed hatched and unranked, never placed by a number it has not earned
  function leaderboard() {
    const box = $('#leaderboard');
    if (!box || !team || main.dataset.view !== 'metrics') return;
    const m = net.model();
    const agents = (activity && activity.agents) || {};
    const vals = new Map(((valueRep && valueRep.agents) || []).map(b => [b.agent, b]));
    const rows = team.nodes.filter(n => n.kind !== 'skill' && (vals.has(n.name) && vals.get(n.name).signals && vals.get(n.name).signals.runs || agents[n.name] && agents[n.name].total))
      .map(n => { const a = agents[n.name] || { total: 0, tokens: [] }; const v = vals.get(n.name) || null; return { n, a, v, sure: !!v && v.confidence === 'ok', tokens: (a.tokens || []).reduce((x, y) => x + y, 0) || (v && v.signals.tokens) || 0 }; })
      .sort((x, y) => (y.sure - x.sure) || ((y.v ? y.v.score : -1) - (x.v ? x.v.score : -1)) || y.a.total - x.a.total || x.n.name.localeCompare(y.n.name));
    const quiet = team.nodes.filter(n => n.kind !== 'skill').length - rows.length;
    const window = (valueRep && valueRep.window) || 'four weeks';
    $('#leaderboard-note').textContent = 'the members, by value — ' + rows.filter(r => r.sure).length + ' ranked, ' + rows.filter(r => !r.sure).length + ' below their record' + (quiet > 0 ? ', ' + quiet + ' quiet' : '') + ' · window ' + window;
    if (!rows.length) { box.replaceChildren(el('p', { class: 'leaderboard-empty', text: 'no recorded work in the window — a Subagent lands here once it reports' })); return; }
    const th = (t, num, cls) => el('th', { scope: 'col', 'data-num': !!num, class: cls || false, text: t });
    let place = 0;
    box.replaceChildren(el('div', { class: 'leaderboard-wrap' }, el('table', { class: 'leaderboard' },
      el('thead', {}, el('tr', {}, th('Rank'), th('Member'), th('Score', true), th('Measure', true), th('Strain', true), th('Work', true), th('Tokens', true, 'leaderboard-col-tokens'), th('Status'))),
      el('tbody', {}, rows.map(r => {
        const p = m && m.parent.get(r.n.id);
        const liveNow = liveNames.has(r.n.name);
        const focusIt = () => setFocus(r.n.id);
        const meter = r.v ? r.v.meter : null;
        const work = r.v ? r.v.signals.runs : r.a.total;
        if (r.sure) place++;
        return el('tr', { 'data-rank': r.n.rank || false, 'data-family': r.n.family || false, 'data-confidence': r.sure ? 'ok' : 'low', tabindex: 0, onclick: focusIt, onkeydown: ev => { if (ev.key === 'Enter') focusIt(); },
          'aria-label': labelOf(r.n.name) + ', ' + (r.sure ? 'score ' + Math.round(r.v.score) : 'unranked, below its record') + (meter ? ', ' + meter.label + ' ' + meter.text : '') + ', ' + (liveNow ? 'live' : 'idle') },
          el('td', { class: 'leaderboard-rank', text: r.sure ? String(place).padStart(2, '0') : '—', title: r.sure ? false : 'below its rank\'s record: not ranked' }),
          el('td', {}, el('span', { class: 'leaderboard-name', text: labelOf(r.n.name) }), el('span', { class: 'leaderboard-sub', text: word('rank.' + r.n.rank, r.n.rank || 'agent') + (p ? ' · under ' + nameOf(p) : '') })),
          el('td', { class: 'leaderboard-num', 'data-lead': r.sure && place === 1, text: r.v ? String(Math.round(r.v.score)) : '—' }),
          el('td', { class: 'leaderboard-num', 'data-tone': r.sure && meter ? toneOf(meter.band) : false, title: meter ? meter.label : false },
            meter ? el('span', { class: 'leaderboard-unit', text: meter.label }) : null, meter ? meter.text : '—'),
          el('td', { class: 'leaderboard-num', 'data-tone': r.v && r.v.strain > 0 ? 'warn' : false, title: 'council strain (0 after a clean landing)', text: r.v ? String(r.v.strain) : '—' }),
          el('td', { class: 'leaderboard-num', text: num(work) }),
          el('td', { class: 'leaderboard-num leaderboard-col-tokens', text: r.tokens ? num(r.tokens) : '—' }),
          el('td', { class: 'leaderboard-status', 'data-live': liveNow, text: liveNow ? 'live' : 'idle' }));
      })))));
  }

  // ---------- metrics ----------
  function kpi(label, value, sub, silent, tone) {
    return el('div', { class: 'kpi', 'data-silent': !!silent, 'data-tone': tone || false }, el('span', { class: 'kpi-label', text: label }), el('span', { class: 'kpi-value', text: value }), el('span', { class: 'kpi-sub', text: sub }));
  }
  function bars(box, rows, figure) {
    const max = Math.max(1, ...rows.map(r => r.v));
    box.replaceChildren(...(rows.length ? rows.map(r => el('div', { class: 'bars-row' },
      el('span', { class: 'bars-label', text: r.k, title: r.k }),
      el('span', { class: 'bars-track' }, el('span', { class: 'bars-fill', style: { '--bars-value': (100 * r.v / max).toFixed(1) + '%' } })),
      el('span', { class: 'bars-figure', text: figure(r) }))) : [el('p', { class: 'panel-note', text: 'nothing measured in this range' })]));
  }
  const tokens = s => (s.input || 0) + (s.output || 0) + (s.cacheRead || 0) + (s.cacheWrite || 0);
  const priced = s => !s.usd && s.unpriced ? 'unpriced' : s.unpriced ? usd(s.usd) + '+' : usd(s.usd);
  let court = [];
  function metrics() {
    const range = $('#range').value;
    getJSON('api/usage?range=' + encodeURIComponent(range)).then(u => {
      const t = u.total || {};
      $('#metrics-note').textContent = u.reading || '';
      const ctx = live.ctxKnown ? live.ctxPct + '%' : '—';
      rankKpis();
      $('#kpis').replaceChildren(
        kpi('tokens', num(tokens(t)), num(t.input) + ' in · ' + num(t.output) + ' out', !t.calls),
        kpi('cost', t.calls ? priced(t) : '—', t.unpriced ? t.unpriced + ' calls unpriced (a floor)' : 'priced from config', !t.calls, 'second'),
        kpi('calls', num(t.calls), 'range ' + (u.range || range), !t.calls),
        kpi('context', ctx, live.ctxKnown ? 'the session window' : 'silent: no turn yet', !live.ctxKnown, ctxLevel(live.ctxPct) ? 'warn' : false),
        kpi(word('agents', 'agents') + ' live', String(live.live || 0), subagent.length + ' in this session', !live.session),
        cacheKpi(t));
      daySpark(u.byDay || []);
      bars($('#by-model'), (u.byModel || []).map(s => ({ k: s.key, v: tokens(s), s })), r => num(r.v) + ' · ' + priced(r.s));
      bars($('#by-body'), (u.byAgent || []).slice(0, 12).map(s => ({ k: s.key, v: tokens(s), s })), r => num(r.v) + ' · ' + priced(r.s));
    }).catch(() => { $('#metrics-note').textContent = 'the usage journal is silent'; });
  }
  // the prompt cache hit rate: the share of the INPUT the provider served from its cache (output
  // is never cached, so it is not in the denominator); what it saved is the reused tokens
  function cacheKpi(t) {
    const prompt = (t.input || 0) + (t.cacheRead || 0) + (t.cacheWrite || 0);
    if (!prompt) return kpi('prompt cache hit', '—', 'silent: no input measured', true);
    const hit = (t.cacheRead || 0) / prompt;
    const sub = t.cacheRead ? num(t.cacheRead) + ' input tokens reused' + (t.cacheWrite ? ' · ' + num(t.cacheWrite) + ' written' : '')
      : 'no cache hits reported — the provider may not cache or not report it';
    return kpi('prompt cache hit', Math.round(100 * hit) + '%', sub, !t.cacheRead && !t.cacheWrite, hit >= 0.5 ? 'ok' : hit >= 0.2 ? false : 'warn');
  }
  function daySpark(days) {
    const box = $('#by-day');
    if (!days.length) { box.replaceChildren(el('p', { class: 'panel-note', text: 'no days in this range' })); return; }
    const W = Math.max(240, box.clientWidth || 320), H = 96, pad = 18;
    const max = Math.max(1, ...days.map(tokens)), bw = (W - 2) / days.length;
    const g = svg('svg', { class: 'spark', viewBox: `0 0 ${W} ${H}`, role: 'img', 'aria-label': 'tokens by day' });
    days.forEach((d, i) => {
      const h = Math.max(1, (H - pad - 4) * tokens(d) / max);
      const bar = svg('rect', { class: 'spark-bar', x: 1 + i * bw + bw * 0.15, y: H - pad - h, width: bw * 0.7, height: h, rx: 2 });
      bar.append(svg('title', {}, document.createTextNode(d.key + ': ' + num(tokens(d)) + ' tokens · ' + priced(d))));
      g.append(bar);
    });
    g.append(svg('text', { class: 'spark-axis', x: 2, y: H - 3 }, document.createTextNode(days[0].key)));
    g.append(svg('text', { class: 'spark-axis', x: W - 2, y: H - 3, 'text-anchor': 'end' }, document.createTextNode(days[days.length - 1].key)));
    box.replaceChildren(g);
  }
  function subagentPoll() {
    return getJSON('api/subagent').then(c => {
      subagent = c.agents || [];
      for (const b of subagent) { if (b.state && !/^(done|idle)$/.test(b.state)) liveNames.add(b.name); else liveNames.delete(b.name); }
      netLive();
      now();
      leaderboard();
      bars($('#live-agents'), subagent.map(b => ({ k: b.name, v: (b.input || 0) + (b.output || 0) + (b.cacheRead || 0) + (b.cacheWrite || 0), b })),
        r => (r.b.state || '—') + (r.b.contextLimit ? ' · ctx ' + Math.round(100 * r.b.contextTokens / r.b.contextLimit) + '%' : ''));
    }).catch(() => { /* no live court */ });
  }
  $('#range').addEventListener('change', metrics);

  // ---------- the board's own events: files changed, the court moved ----------
  function boardEvents() {
    const es = new EventSource('events');
    const on = (name, f) => es.addEventListener(name, f);
    on('usage', () => { metrics(); refreshActivity(); });
    on('subagent', () => { subagentPoll(); refreshActivity(); });
    on('team', loadTeam);
    on('onto', loadTeam);
    // the workspace's tabs refresh on their own instruments
    for (const name of ['loop', 'subagent', 'memory', 'toolbox']) on(name, () => bus.dispatchEvent(new CustomEvent('board', { detail: name })));
  }

  // ────────── the folder tree: the workspace's ownership, live heat ──────────
  // Slice 1 laziness: one fetch of /api/tree/children per expanded dir —
  // the workspace is never walked at once. A dir expands on click; its children
  // come from the endpoint; heat on an unrendered path warms the nearest
  // rendered ancestor chain instead of a prebuilt index. Heat decays over
  // ~3.4s; paints are batched in one rAF.
  const treeBox = $('#tree');
  let treeIdx = new Map(), treeDirty = false, decayOn = false;
  const HEAT_LIFE = 3400;
  function rankOf(body) {
    if (!body) return 'other';
    const low = String(body).toLowerCase();
    for (const r of ['zone', 'auditor', 'principal-coordinator', 'principal-domain-owner', 'orchestrator', 'service', 'domain', 'coord']) if (low === r || low.startsWith(r + '-')) return r;
    for (const w of [WORDS['rank.zone'], WORDS['rank.domain'], WORDS['rank.coord'], WORDS['rank.service']].filter(Boolean)) if (low.includes(String(w).toLowerCase())) return w;
    return 'other';
  }
  function treeRow(depth, it) {
    return el('span', { class: 'tree-node', 'data-d': it.dir ? 'dir' : '', 'data-path': it.path, tabindex: 0, role: 'treeitem', 'aria-expanded': it.dir ? 'false' : false, style: { '--tree-depth': String(depth) } },
      svg('svg', { class: 'tree-icon', 'data-d': it.dir ? 'dir' : false, viewBox: '0 0 16 16', 'aria-hidden': 'true' }, svg('path', {
        d: it.dir ? 'M1 3a1 1 0 0 1 1-1h4l2 2h6a1 1 0 0 1 1 1v8a1 1 0 0 1-1 1H2a1 1 0 0 1-1-1z' : 'M3 1h7l3 3v11H3z',
        fill: 'none', stroke: 'currentColor', 'stroke-width': '1.4',
      })),
      el('span', { class: 'tree-name', text: it.name || '' }),
      el('span', { class: 'tree-path', text: it.path || '' }));
  }
  function treeFetch(path) {
    const q = path ? '?path=' + encodeURIComponent(path) : '';
    return fetch('api/tree/children' + q, { cache: 'no-store', headers: { 'X-Board-Token': TOKEN } })
      .then(r => { if (!r.ok) throw new Error(r.status + ''); return r.json(); });
  }
  function treeExpand(row, depth) {
    const path = row.getAttribute('data-path');
    if (!path) return;
    const li = row.closest('li');
    if (!li) return;
    if (row.getAttribute('aria-expanded') === 'true') {
      // Collapse: forget the subtree so a re-expand refetches fresh.
      const sub = li.querySelector('ul.tree-list');
      if (sub) { sub.querySelectorAll('.tree-node').forEach(n => treeIdx.delete(n.getAttribute('data-path'))); sub.remove(); }
      row.setAttribute('aria-expanded', 'false');
      const rec = treeIdx.get(path); if (rec) rec.expanded = false;
      return;
    }
    row.setAttribute('aria-expanded', 'true');
    treeFetch(path).then(d => {
      if (!d || !Array.isArray(d.children)) return;
      const ul = el('ul', { class: 'tree-list', role: 'group' });
      const rec = treeIdx.get(path);
      const parents = rec ? [...rec.parents, path] : [path];
      let dircount = 0;
      for (const n of d.children) {
        const nd = { name: n.name, path: n.path, dir: !!n.dir, parents };
        treeIdx.set(n.path, nd);
        if (nd.dir) dircount++;
        const kidRow = treeRow(depth + 1, nd);
        if (nd.dir) kidRow.addEventListener('click', () => treeExpand(kidRow, depth + 1));
        ul.append(el('li', { class: 'tree-li' }, kidRow));
      }
      li.append(ul);
      const r2 = treeIdx.get(path); if (r2) r2.expanded = true;
      if (d.truncated) li.append(el('p', { class: 'tree-truncated', text: 'folder capped — some entries not shown' }));
    }).catch(() => {
      row.setAttribute('aria-expanded', 'false');
    });
  }
  function treeLoad() {
    if (!treeBox) return;
    treeIdx = new Map();
    treeFetch('').then(d => {
      // the endpoint's face is {path, children, truncated} (api_face_test pins it)
      if (d && !d.nodes && Array.isArray(d.children)) d.nodes = d.children;
      if (!d || !Array.isArray(d.nodes)) { treeBox.replaceChildren(el('p', { class: 'tree-empty', text: 'no tree source' })); return; }
      const ul = el('ul', { class: 'tree-list', role: false });
      let dirs = 0;
      for (const n of d.nodes) {
        const nd = { name: n.name, path: n.path, dir: !!n.dir, parents: [] };
        treeIdx.set(n.path, nd);
        if (nd.dir) dirs++;
        const row = treeRow(0, nd);
        if (nd.dir) row.addEventListener('click', () => treeExpand(row, 0));
        ul.append(el('li', { class: 'tree-li' }, row));
      }
      treeBox.replaceChildren(ul,
        d.truncated ? el('p', { class: 'tree-truncated', text: 'tree capped — some files not shown' }) : null);
      $('.panel-note[data-note="tree"]').textContent = 'workspace ownership — ' + d.nodes.length + ' roots · ' + dirs + ' folders · lazy';
    }).catch(() => { treeIdx = new Map(); treeBox.replaceChildren(el('p', { class: 'tree-empty', text: 'no tree source' })); });
  }
  function treeHeat(path, className, body) {
    if (!treeIdx || !path) return;
    const rec = treeIdx.get(path);
    if (!rec) return;
    const rank = rankOf(body);
    const touch = (r, bubble) => {
      const row = treeBox.querySelector('[data-path="' + CSS.escape(r.path) + '"]');
      if (!row) return;
      row.dataset.class = className || 'read';
      row.dataset.agent = body || '';
      row.dataset.rank = rank;
      row.style.setProperty('--glow-opacity', bubble ? '0.22' : (className === 'read' ? '0.32' : '0.42'));
      if (bubble) row.setAttribute('data-bubble', ''); else row.removeAttribute('data-bubble');
      row.dataset.heatAt = String(Date.now());
      row.setAttribute('data-lit', bubble ? '' : 'hot');
    };
    touch(rec, false);
    for (const p of rec.parents) {
      const prec = treeIdx.get(p);
      if (prec) touch(prec, true);
    }
    if (!decayOn) { decayOn = true; requestAnimationFrame(() => treeDecay(Date.now())); }
  }
  function treeDecay(now) {
    let any = false;
    for (const row of treeBox.querySelectorAll('.tree-node[data-heat-at]')) {
      const a = +row.dataset.heatAt || 0;
      const k = Math.max(0, 1 - (now - a) / HEAT_LIFE);
      if (k <= 0.03) {
        row.removeAttribute('data-lit'); row.removeAttribute('data-bubble'); row.removeAttribute('data-heat-at');
        row.removeAttribute('data-class'); row.removeAttribute('data-agent'); row.removeAttribute('data-rank');
        row.style.setProperty('--glow-opacity', '0');
        continue;
      }
      const base = row.hasAttribute('data-bubble') ? 0.22 : (row.dataset.class === 'read' ? 0.32 : 0.42);
      row.style.setProperty('--glow-opacity', (base * k).toFixed(3));
      any = true;
    }
    if (any) requestAnimationFrame(() => treeDecay(Date.now())); else decayOn = false;
  }
  function treeEvent(e) {
    if (!e || !treeIdx) return;
    const cls = c => ['read', 'write', 'gate', 'outward', 'destructive'].includes(c) ? c : 'read';
    if (e.type === 'tool' || e.type === 'bodystep') {
      const t = e.tool || {};
      const body = e.agent || t.agent || '';
      const cx = cls(t.class || e.class || '');
      const paths = new Set();
      if (Array.isArray(t.wrote)) for (const w of t.wrote) if (typeof w === 'string') paths.add(w);
      if (t.diff && typeof t.diff.path === 'string' && t.diff.path) paths.add(t.diff.path);
      if (!paths.size) return;
      if (!treeDirty) { treeDirty = true; requestAnimationFrame(() => { treeDirty = false; [...paths].forEach(p => treeHeat(p, cx, body)); }); return; }
      [...paths].forEach(p => treeHeat(p, cx, body));
    } else if (e.type === 'state' && e.tool) {
      requestAnimationFrame(() => treeHeat(e.tool, 'read', e.agent || ''));
    }
  }

  netMode(HASH.get('net') || store.get('netmode') || '2d');
  // the vendored faces arrive after the first layout: the net measures its labels again
  if (document.fonts && document.fonts.ready) document.fonts.ready.then(() => netDraw());
  header(); setInterval(header, 1500);
  loadValue().finally(() => { loadTeam(); metrics(); subagentPoll(); treeLoad(); });
  setInterval(subagentPoll, 2000); setInterval(metrics, 15000); setInterval(loadValue, 10000);
  planMeter(); setInterval(planMeter, 15000);
  stream(); boardEvents();
  if (innerWidth >= 1024) field.focus({ preventScroll: true });
})();
