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
  const reduced = matchMedia('(prefers-reduced-motion: reduce)').matches;

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

  // ---------- the theme: one design, light and dark; the system decides unless the reader does ----------
  const THEMES = [['', 'theme · system', 'follows the system'], ['light', 'theme · light', 'light'], ['dark', 'theme · dark', 'dark']];
  const themeBtn = $('#theme');
  function theme(v) {
    const t = THEMES.find(x => x[0] === v) || THEMES[0];
    if (t[0]) document.documentElement.setAttribute('data-theme', t[0]); else document.documentElement.removeAttribute('data-theme');
    themeBtn.textContent = t[1];
    themeBtn.setAttribute('aria-label', 'theme: ' + t[2] + ' — change');
    store.set('theme', t[0]);
  }
  themeBtn.addEventListener('click', () => {
    const i = THEMES.findIndex(x => x[0] === (document.documentElement.getAttribute('data-theme') || ''));
    theme(THEMES[(i + 1) % THEMES.length][0]);
  });
  theme(store.get('theme'));

  // ---------- views ----------
  const main = $('#main');
  function view(v) {
    main.dataset.view = v;
    for (const t of document.querySelectorAll('.tab')) t.setAttribute('aria-selected', String(t.dataset.view === v));
    store.set('view', v);
    requestAnimationFrame(netDraw);
  }
  for (const t of document.querySelectorAll('.tab')) t.addEventListener('click', () => view(t.dataset.view));
  view(new URLSearchParams(location.hash.slice(1)).get('view') || store.get('view') || 'overview');

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
  const tools = new Map(), courts = new Map(), choices = new Map();
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
    let c = courts.get(e.name);
    if (!c) {
      c = { card: el('details', { class: 'toolcard', 'data-kind': 'court' }), log: el('div', { class: 'stack', 'data-gap': 's' }), stream: '' };
      courts.set(e.name, c);
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

  function onEvent(e) {
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
        const c = courts.get(e.agent);
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
        const target = t.agent && courts.get(t.agent) ? courts.get(t.agent).log : null;
        let card = tools.get(t.id);
        if (!card) { card = toolCard(t); tools.set(t.id, card); add(card, target || ensureTurn()); }
        else fillTool(card, t);
        if (thinking && !t.agent) { thinking.box.firstChild.textContent = 'thought for ' + Math.round((Date.now() - thinking.started) / 1000) + ' s'; thinking = null; }
        if (answer && !t.agent) { finishAnswer(); }
        break;
      }
      case 'bodystep': {
        const c = courts.get(e.agent);
        if (!c) break;
        let card = tools.get(e.tool.id);
        if (!card) { card = toolCard(e.tool); tools.set(e.tool.id, card); c.log.append(card); } else fillTool(card, e.tool);
        break;
      }
      case 'subagent': courtCard(e); break;
      case 'state':
        if (courts.has(e.agent)) break;
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
      turn = answer = thinking = null; tools.clear(); courts.clear(); choices.clear();
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
    field.value = ''; grow();
    post('api/ask', { text }).catch(err => note('not sent: ' + err.message, 'danger'));
  });
  function grow() { /* the field grows by itself (field-sizing: content in console.css) */ }
  field.addEventListener('keydown', ev => {
    if (ev.key === 'Enter' && !ev.shiftKey && !ev.isComposing) { ev.preventDefault(); form.requestSubmit(); }
    else if (ev.key === 'ArrowUp' && field.selectionStart === 0 && history.length) { ev.preventDefault(); hIdx = Math.min(hIdx + 1, history.length - 1); field.value = history[hIdx]; grow(); }
    else if (ev.key === 'ArrowDown' && hIdx >= 0 && field.selectionStart === field.value.length) { ev.preventDefault(); hIdx--; field.value = hIdx >= 0 ? history[hIdx] : ''; grow(); }
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
      const p = pill(st); p.id = 'state'; $('#state').replaceWith(p);
      const m = $('#ctx');
      if (s.ctxKnown) {
        m.style.setProperty('--meter-value', s.ctxPct + '%');
        m.dataset.level = s.ctxPct >= 85 ? 'danger' : s.ctxPct >= 65 ? 'warn' : '';
        m.setAttribute('aria-valuenow', s.ctxPct);
      }
      m.querySelector('.meter-label').textContent = s.ctx || 'ctx —';
      $('#cost').textContent = s.cost || '—';
      const lc = $('#live-count');
      lc.hidden = !s.live; lc.textContent = (s.live || 0) + ' ' + word('bodies', 'agents') + ' live';
      if (s.session && form.dataset.state !== 'waiting') setAsk(s.running ? 'running' : 'idle');
      if (!s.session) { field.disabled = sendBtn.disabled = true; }
    }).catch(() => { const p = pill('board offline'); p.id = 'state'; $('#state').replaceWith(p); });
  }

  // ---------- the neural net: bodies and minds, two planes on one grid ----------
  // A member is a BODY in its rank's layer (portrait, halo, K/C badge for a keeper or court
  // vessel); a MIND is a dashed ring with a brain tinted by the lane it serves, carrying its desk
  // (thoughts / limit — stress glows). Bonds are synapses; a live body's synapses carry a pulse.
  const netBox = $('#net');
  let team = null, focus = '';
  const liveNames = new Set();
  const PORTRAITS = new Set(['orchestrator', 'coord', 'domain', 'zone', 'service']);
  const BRAIN = 'M0 -6 C-3.2 -6 -5.6 -4.4 -5.6 -1.6 C-7.2 -0.6 -7.2 1.8 -5.4 2.8 C-6 4.6 -4.2 6.2 -2.2 6.2 C-1.1 6.2 -0.4 5.4 0 4.4 C0.4 5.4 1.1 6.2 2.2 6.2 C4.2 6.2 6 4.6 5.4 2.8 C7.2 1.8 7.2 -0.6 5.6 -1.6 C5.6 -4.4 3.2 -6 0 -6 Z';
  const MIND = 'skill';
  const isMind = n => n.kind === MIND;
  const tone = n => isMind(n) ? n.lane : (n.rank || 'plain'); // what data-rank colours
  const keeper = b => b && (b.mode === 'all' || b.mode === 'primary');
  function netDraw() {
    if (!team || !netBox.isConnected || netBox.offsetParent === null) return;
    const W = Math.max(320, netBox.clientWidth);
    const nodes = team.nodes, edges = team.edges, lanes = team.lanes;
    const label = id => (team.labels.find(l => l.id === id) || {}).label || id.replace(/^[a-z]+:/, '') ;
    const byLane = new Map(lanes.map(l => [l, []]));
    for (const n of nodes) { if (!byLane.has(n.lane)) byLane.set(n.lane, []); byLane.get(n.lane).push(n); }
    const deg = new Map();
    for (const e of edges) { deg.set(e.from, (deg.get(e.from) || 0) + 1); deg.set(e.to, (deg.get(e.to) || 0) + 1); }
    const pad = 28, gap = W < 560 ? 84 : 112, perRow = Math.max(1, Math.floor((W - 2 * pad) / gap));
    const rows = [];
    for (const [lane, list] of byLane) {
      if (!list.length) continue;
      for (let i = 0; i < list.length; i += perRow) rows.push({ lane, nodes: list.slice(i, i + perRow), first: i === 0 });
    }
    rows.reverse(); // the input layer at the bottom, the crown on top
    const RH = 112, top = 8, H = top + rows.length * RH + 8;
    const pos = new Map();
    const g = svg('svg', { viewBox: `0 0 ${W} ${H}`, role: 'img', 'aria-label': 'the ' + word('world', 'workspace') + ' as a neural net' });
    const defs = svg('defs');
    g.append(defs);
    rows.forEach((row, i) => {
      const y = top + i * RH;
      // a layer wears its rank's colour (net.css maps data-rank → --node); a mind layer its lane's
      g.append(svg('g', { 'data-rank': row.lane, 'data-plane': row.lane.includes(':') ? 'minds' : 'bodies' },
        svg('rect', { class: 'net-row', x: 4, y, width: W - 8, height: RH - 10, rx: 12 }),
        row.first ? svg('text', { class: 'net-row-label', x: 16, y: y + 16 }, document.createTextNode(label(row.lane) + ' · ' + byLane.get(row.lane).length)) : null));
      row.nodes.forEach((n, j) => pos.set(n.id, { x: pad + (W - 2 * pad) * (j + 0.5) / row.nodes.length, y: y + RH / 2 + 4, n }));
    });
    const edgeLayer = svg('g', { 'data-layer': 'edges' }), pulseLayer = svg('g', { 'data-layer': 'pulses' }), nodeLayer = svg('g', { 'data-layer': 'nodes' });
    g.append(edgeLayer, pulseLayer, nodeLayer);
    for (const e of edges) {
      const A = pos.get(e.from), B = pos.get(e.to);
      if (!A || !B) continue;
      const d = A.y === B.y
        ? `M${A.x} ${A.y} Q ${(A.x + B.x) / 2} ${A.y - 34} ${B.x} ${B.y}`
        : `M${A.x.toFixed(1)} ${A.y} C ${A.x.toFixed(1)} ${(A.y + B.y) / 2} ${B.x.toFixed(1)} ${(A.y + B.y) / 2} ${B.x.toFixed(1)} ${B.y}`;
      edgeLayer.append(svg('path', { class: 'net-edge', 'data-bond': e.bond, 'data-from': e.from, 'data-to': e.to, d }));
    }
    let k = 0;
    for (const [id, p] of pos) {
      const n = p.n, r = (isMind(n) ? 12 : 17) + Math.min(7, 2.5 * Math.sqrt(deg.get(id) || 0));
      const desk = n.desk;
      const node = svg('g', { class: 'net-node', 'data-id': id, 'data-rank': tone(n), 'data-kind': isMind(n) ? 'skill' : 'body',
        'data-stress': desk && desk.stress && desk.stress !== 'ok' ? desk.stress : false, tabindex: 0, role: 'button',
        'aria-label': n.name + ' (' + (isMind(n) ? word('skill', 'skill') : (n.rank || 'body')) + ')' });
      node.append(svg('circle', { class: 'net-node-halo', cx: p.x, cy: p.y, r: r * 1.7 }), svg('circle', { class: 'net-node-core', cx: p.x, cy: p.y, r }));
      if (isMind(n)) {
        const s = (r * 0.13).toFixed(2);
        node.append(svg('g', { transform: `translate(${p.x.toFixed(1)} ${p.y.toFixed(1)}) scale(${s})` },
          svg('path', { class: 'net-brain', d: BRAIN }), svg('path', { class: 'net-brain-fold', d: 'M0 -6 L0 4.4' })));
      } else if (PORTRAITS.has(n.rank)) {
        const cid = 'np-' + (k++);
        defs.append(svg('clipPath', { id: cid }, svg('circle', { cx: p.x, cy: p.y, r: r * 0.86 })));
        const img = svg('image', { class: 'net-portrait', href: 'web/portraits/' + n.rank + '.png', x: p.x - r * 0.86, y: p.y - r * 0.86, width: r * 1.72, height: r * 1.72,
          preserveAspectRatio: 'xMidYMid slice', 'clip-path': `url(#${cid})` });
        img.addEventListener('error', () => img.remove());
        node.append(img);
      } else {
        node.append(svg('circle', { class: 'net-node-dot', cx: p.x, cy: p.y, r: r * 0.38 }));
      }
      if (n.body) node.append(svg('g', { class: 'net-badge' },
        svg('circle', { cx: p.x + r * 0.8, cy: p.y - r * 0.8, r: 6.5 }),
        svg('text', { x: p.x + r * 0.8, y: p.y - r * 0.8 + 3 }, document.createTextNode(keeper(n.body) ? 'K' : 'C'))));
      if (desk) node.append(svg('text', { class: 'net-desk', x: p.x + r + 6, y: p.y + 4 }, document.createTextNode(desk.notes + '/' + desk.limit)));
      node.append(svg('text', { class: 'net-node-label', x: p.x, y: p.y + r * 1.7 + 10 }, document.createTextNode(n.name.length > 18 ? n.name.slice(0, 17) + '…' : n.name)));
      node.addEventListener('click', () => setFocus(focus === id ? '' : id));
      node.addEventListener('keydown', ev => { if (ev.key === 'Enter' || ev.key === ' ') { ev.preventDefault(); setFocus(focus === id ? '' : id); } });
      nodeLayer.append(node);
    }
    netBox.replaceChildren(g);
    const bodies = nodes.filter(n => !isMind(n)).length, minds = nodes.length - bodies;
    $('#net-note').textContent = bodies + ' ' + word('bodies', 'agents') + ' · ' + minds + ' ' + word('minds', 'skills') + ' · ' + edges.length + ' bonds · ' + rows.length + ' layers';
    findingsList();
    netLive(); applyFocus();
  }
  function findingsList() {
    const box = $('#net-findings');
    const items = [...team.findings.map(t => ({ t, tone: 'warn' })), ...team.notes.map(t => ({ t, tone: '' }))];
    box.replaceChildren(...(items.length ? [el('h3', { class: 'panel-note', text: team.findings.length + ' findings · ' + team.notes.length + ' notes — what the ontology says is missing' }),
      el('ul', { class: 'findings' }, items.map(i => el('li', { 'data-tone': i.tone || false, text: i.t })))] : []));
  }
  function netLive() {
    const g = netBox.querySelector('svg');
    if (!g) return;
    for (const n of g.querySelectorAll('.net-node')) n.toggleAttribute('data-live', liveNames.has(n.dataset.id));
    const pulses = g.querySelector('[data-layer="pulses"]');
    pulses.replaceChildren();
    let k = 0;
    for (const p of g.querySelectorAll('.net-edge')) {
      const on = liveNames.has(p.dataset.from) || liveNames.has(p.dataset.to);
      p.toggleAttribute('data-live', on);
      if (on && !reduced && k++ < 40) {
        const dot = svg('circle', { class: 'net-pulse', r: 3 });
        dot.append(svg('animateMotion', { dur: '1.8s', repeatCount: 'indefinite', path: p.getAttribute('d'), keyPoints: liveNames.has(p.dataset.from) ? '0;1' : '1;0', keyTimes: '0;1', calcMode: 'linear' }));
        pulses.append(dot);
      }
    }
  }
  function setFocus(id) { focus = id; applyFocus(); knowledge(id); }
  function applyFocus() {
    const g = netBox;
    g.toggleAttribute('data-focus', !!focus);
    const near = new Set([focus]);
    for (const p of g.querySelectorAll('.net-edge')) {
      const lit = !!focus && (p.dataset.from === focus || p.dataset.to === focus);
      p.toggleAttribute('data-lit', lit);
      if (lit) { near.add(p.dataset.from); near.add(p.dataset.to); }
    }
    for (const n of g.querySelectorAll('.net-node')) n.toggleAttribute('data-near', !!focus && near.has(n.dataset.id));
    for (const th of document.querySelectorAll('#matrix th[data-id]')) th.toggleAttribute('data-hot', !!focus && th.dataset.id === focus);
  }
  $('#net-clear').addEventListener('click', () => setFocus(''));
  function knowledge(id) {
    const box = $('#kcard');
    box.replaceChildren();
    if (!id || !team) return;
    const n = team.nodes.find(x => x.id === id);
    if (!n) return;
    const bonds = team.edges.filter(e => e.from === id || e.to === id).map(e => (e.from === id ? e.bond + ' → ' + nameOf(e.to) : nameOf(e.from) + ' → ' + e.bond));
    const rows = [];
    const row = (k, v) => { if (v) rows.push(el('dt', { text: k }), el('dd', { text: v })); };
    if (isMind(n)) {
      row('serves', label2(n.lane));
      row('source', n.source);
      row('notes', n.desk ? n.desk.notes + ' / ' + n.desk.limit + ' notes' + (n.desk.stress && n.desk.stress !== 'ok' ? ' — ' + n.desk.stress : '') + (n.desk.heldBy ? ' · held by ' + n.desk.heldBy : '') : 'no notes yet');
      row('what', n.description);
    } else {
      row(word('territory', 'territory'), (n.owns || []).join(', ') || '—');
      row('agent', n.body ? (keeper(n.body) ? 'keeper' : 'court') + ' · ' + (n.body.model || 'the session model') + ' · ' + n.body.source : 'not defined — runs in the session');
    }
    row('bonds', bonds.join(' · ') || '—');
    const docPath = n.doc || (n.desk && n.desk.src) || '';
    const doc = el('pre', { class: 'kcard-doc', text: docPath ? 'reading ' + docPath + '…' : 'no doc — the ontology notes it' });
    box.append(el('article', { class: 'kcard' },
      el('div', { class: 'kcard-title' }, n.name, el('span', { class: 'pill', 'data-tone': 'info', text: isMind(n) ? word('skill', 'skill') : (n.rank || 'body') }), liveNames.has(id) ? pill('running') : null),
      el('dl', {}, rows), doc));
    if (docPath) fetch('api/doc?path=' + encodeURIComponent(docPath)).then(r => r.ok ? r.text() : Promise.reject(new Error(r.status)))
      .then(t => { doc.textContent = t.split('\n').slice(0, 60).join('\n'); }).catch(() => { doc.textContent = docPath + ' — not readable here'; });
  }
  const label2 = id => ((team && team.labels.find(l => l.id === id)) || {}).label || id;
  const nameOf = id => ((team && team.nodes.find(n => n.id === id)) || {}).name || id;
  // the ontology's graph, joined with the two planes it does not draw: minted bodies ride their
  // members (or stand in their rank's layer), every wearable skill joins (the binary's own
  // included), and each mind carries its desk
  function loadTeam() {
    return Promise.all([getJSON('api/team'), getJSON('api/workspace').catch(() => ({ agents: [], skills: [], workingNotes: [] }))]).then(([c, w]) => {
      c.nodes = c.nodes || []; c.edges = c.edges || []; c.lanes = c.lanes || []; c.labels = c.labels || []; c.findings = c.findings || []; c.notes = c.notes || [];
      // a skill's lane is drawn as skill:<lane> (the lane order and labels speak that way)
      for (const n of c.nodes) if (n.kind === 'skill' && c.lanes.includes('skill:' + n.lane)) n.lane = 'skill:' + n.lane; // a lane may share a rank's name
      const byName = new Map(c.nodes.map(n => [n.name, n]));
      const ranks = c.lanes.filter(l => !l.includes(':'));
      for (const b of w.agents || []) {
        const n = byName.get(b.name);
        if (n) { n.body = b; continue; }
        let rank = ranks.find(r => b.name === r || b.name.startsWith(r + '-'));
        const prefix = b.name.split('-')[0].replace(/_/g, '');
        if (!rank && WORDS['rank.' + prefix]) {
          // a rank born in time (service, dark coord …): its first body opens its layer, above the crown
          rank = prefix;
          c.lanes.splice(Math.max(0, c.lanes.findIndex(l => l.endsWith(':shared'))), 0, rank);
          c.labels.push({ id: rank, label: WORDS['rank.' + prefix], kind: 'rank' }); ranks.push(rank);
        }
        const lane = rank || 'bodies:other';
        if (!rank && !c.lanes.includes(lane)) { c.lanes.push(lane); c.labels.push({ id: lane, label: 'other agents', kind: 'rank' }); }
        const node = { id: 'body-' + b.name, name: b.name, kind: 'member', rank: rank || '', lane, owns: [], body: b, description: b.description };
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
      // a desk sits in a mind, or in a member's own hat (<name>@hat, worn by that member)
      for (const d of w.workingNotes || []) { const n = byName.get(String(d.skill).replace(/@skill$/, '')); if (n && (d.notes || !n.desk)) n.desk = d; }
      team = c; netDraw(); relations();
    }).catch(() => { $('#net-note').textContent = 'the ontology is silent'; });
  }
  new ResizeObserver(() => { clearTimeout(netDraw.t); netDraw.t = setTimeout(netDraw, 120); }).observe(netBox);

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
      $('#kpis').replaceChildren(
        kpi('tokens', num(tokens(t)), num(t.input) + ' in · ' + num(t.output) + ' out', !t.calls),
        kpi('cost', t.calls ? priced(t) : '—', t.unpriced ? t.unpriced + ' calls unpriced (a floor)' : 'priced from config', !t.calls, 'second'),
        kpi('calls', num(t.calls), 'range ' + (u.range || range), !t.calls),
        kpi('context', ctx, live.ctxKnown ? 'the session window' : 'silent: no turn yet', !live.ctxKnown, live.ctxPct >= 85 ? 'warn' : false),
        kpi(word('bodies', 'agents') + ' live', String(live.live || 0), court.length + ' in this session', !live.session),
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
  function courtPoll() {
    return getJSON('api/subagent').then(c => {
      court = c.agents || [];
      for (const b of court) { if (b.state && b.state !== 'done') liveNames.add(b.name); else liveNames.delete(b.name); }
      netLive();
      bars($('#live-bodies'), court.map(b => ({ k: b.name, v: (b.input || 0) + (b.output || 0) + (b.cacheRead || 0) + (b.cacheWrite || 0), b })),
        r => (r.b.state || '—') + (r.b.contextLimit ? ' · ctx ' + Math.round(100 * r.b.contextTokens / r.b.contextLimit) + '%' : ''));
    }).catch(() => { /* no live court */ });
  }
  $('#range').addEventListener('change', metrics);

  // ---------- the board's own events: files changed, the court moved ----------
  function boardEvents() {
    const es = new EventSource('events');
    const on = (name, f) => es.addEventListener(name, f);
    on('usage', metrics);
    on('subagent', courtPoll);
    on('team', loadTeam);
    on('onto', loadTeam);
  }

  // ---------- start ----------
  header(); setInterval(header, 1500);
  loadTeam(); metrics(); courtPoll();
  setInterval(courtPoll, 2000); setInterval(metrics, 15000);
  stream(); boardEvents();
  field.focus();
})();
