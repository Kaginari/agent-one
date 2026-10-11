// workspace.js — the dashboard's workspace tabs: Subagent (every journaled run, and one run whole), Memory
// (the three tiers, a read-only recall, the review), Toolbox (the registry, what was offered and
// loaded) and Config (the resolved config with its origins, the off-list). Read-only, all of it.
// No dependencies; it builds its DOM with dash.js's helpers (window.agentOneDash) and reads the
// board's JSON (/api/runs, /api/run, /api/memory, /api/memory/recall, /api/memory/review,
// /api/toolbox, /api/workspace, /api/subagent, /api/config). A tab loads when it is first shown and
// refreshes when its instrument moves.
(() => {
  'use strict';
  const D = window.agentOneDash;
  if (!D) return;
  const { el, getJSON, word, pill, num } = D;
  const $ = s => document.querySelector(s);

  // ---------- shared bits ----------
  const tone = st => { const x = String(st || '').toUpperCase(); return !x ? 'muted' : /^(DONE|PASS|OK)/.test(x) ? 'ok' : /FAIL|DENIED|REFUSED/.test(x) ? 'danger' : /RUNNING|LIVE/.test(x) ? 'info' : 'warn'; };
  const status = st => { const p = pill(String(st || '—').split(/\s/)[0].toLowerCase()); p.dataset.tone = tone(st); return p; };
  const when = at => { const d = new Date(at); if (isNaN(d)) return at || '—'; return d.toISOString().slice(0, 16).replace('T', ' '); };
  const short = at => { const d = new Date(at); if (isNaN(d)) return ''; return Date.now() - d < 864e5 ? d.toTimeString().slice(0, 5) : d.toISOString().slice(5, 10); };
  const ms = n => !n ? '—' : n < 1000 ? n + ' ms' : n < 60000 ? (n / 1000).toFixed(1) + ' s' : Math.round(n / 60000) + ' min';
  const section = (title, ...kids) => el('section', { class: 'stack', 'data-gap': 's' }, el('h3', { class: 'panel-note', text: title }), ...kids);
  const empty = (cls, text) => el('p', { class: cls, text });
  const rankOf = name => { const low = String(name || '').toLowerCase(); for (const r of ['orchestrator', 'auditor', 'principal-coordinator', 'principal-domain-owner', 'service', 'zone', 'domain', 'coord']) if (low === r || low.startsWith(r + '-')) return r; return ''; };
  function feed(items, emptyText) {
    if (!items.length) return empty('feed-empty', emptyText);
    return el('ol', { class: 'feed' }, items.map(i => el('li', { class: 'feed-item', 'data-tone': i.tone || false },
      i.meta && i.meta.length ? el('div', { class: 'feed-meta' }, i.meta) : null,
      el('p', { class: 'feed-text', text: i.text }),
      i.sub ? el('p', { class: 'feed-sub', text: i.sub }) : null)));
  }
  function ledger(heads, rows, emptyText, max) {
    if (!rows.length) return el('div', { class: 'ledger-wrap' }, empty('ledger-empty', emptyText));
    return el('div', { class: 'ledger-wrap', style: max ? { '--ledger-max': max } : false },
      el('table', { class: 'ledger' },
        el('thead', {}, el('tr', {}, heads.map(h => el('th', { scope: 'col', 'data-num': !!h.num, 'data-wide': !!h.wide, text: h.t })))),
        el('tbody', {}, rows)));
  }
  const td = (kids, attrs) => el('td', attrs || {}, kids);
  const name = (n, sub) => [el('span', { class: 'ledger-name', text: n }), sub ? el('span', { class: 'ledger-sub', text: sub }) : null];
  const tag = t => el('span', { class: 'ledger-tag', text: t });
  function readingNote(id, r) { const n = $(id); if (n) n.textContent = r ? r.text : '—'; }
  function fail(box, what, err) { box.replaceChildren(el('p', { class: 'net-hint', text: what + ' did not answer: ' + (err && err.message || err) })); }

  // ---------- Subagent: the history and one run, whole ----------
  const subagent = { runs: [], live: [], filter: { q: '', as: '', st: 'all' }, open: '', loaded: false };
  function subagentLoad() {
    return Promise.all([getJSON('api/runs'), getJSON('api/subagent').catch(() => ({ agents: [], live: false }))]).then(([r, c]) => {
      subagent.runs = r.runs || []; subagent.live = c.agents || []; subagent.liveWired = !!c.live; subagent.reading = r.reading;
      // drawn once; a refresh redraws the live line and the list, never the filter being typed in
      if (subagent.loaded && $('#subagent-runs')) { readingNote('#subagent-note', subagent.reading); subagentLive(); subagentList(); return; }
      subagent.loaded = true;
      subagentRender();
    }).catch(err => fail($('#subagent-view'), 'the loop journals', err));
  }
  function subagentLive() {
    const host = $('#subagent-live');
    if (!host) return;
    const running = subagent.live.filter(b => b.state && !/^(done|idle)$/.test(b.state));
    host.replaceChildren(subagent.liveWired
      ? (running.length ? ledger([{ t: word('agent', 'agent') }, { t: 'state' }, { t: word('role', 'role'), wide: true }, { t: 'model', wide: true }, { t: 'context', num: true }, { t: 'tokens', num: true }],
        running.map(b => el('tr', { 'data-rank': b.rank || rankOf(b.name) || false },
          td(name(b.name, word('rank.' + b.rank, b.rank || ''))), td(pill(b.state)), td(b.role || '—', { 'data-wide': true }), td(b.model || '—', { 'data-wide': true }),
          td(b.contextLimit ? Math.round(100 * b.contextTokens / b.contextLimit) + '%' : '—', { 'data-num': true }),
          td(num((b.input || 0) + (b.output || 0) + (b.cacheRead || 0) + (b.cacheWrite || 0)), { 'data-num': true }))), '')
        : empty('panel-note', 'no ' + word('subagent_agent', 'ephemeral subagent') + ' running now — the history below is every one that ran'))
      : empty('panel-note', 'no live feed on this board (it runs without a session) — the history below reads the journals'));
  }
  function subagentRender() {
    const box = $('#subagent-view');
    readingNote('#subagent-note', subagent.reading);
    const f = subagent.filter;
    const agents = [...new Set(subagent.runs.map(r => r.as).filter(Boolean))].sort();
    const count = st => subagent.runs.filter(r => (!f.as || r.as === f.as) && (st === 'all' || (st === 'ok' ? tone(r.status) === 'ok' : tone(r.status) !== 'ok'))).length;

    const field = el('input', { class: 'filter-field', type: 'search', 'aria-label': 'filter the runs', placeholder: 'filter: agent, ask, status, id', value: f.q });
    field.addEventListener('input', () => { f.q = field.value.trim().toLowerCase(); subagentList(); });
    const sel = el('select', { class: 'filter-select', 'aria-label': word('agent', 'agent') }, el('option', { value: '', text: 'every ' + word('agent', 'agent') }), agents.map(b => el('option', { value: b, text: b, selected: b === f.as })));
    sel.addEventListener('change', () => { f.as = sel.value; subagentRender(); });
    const chip = (st, label) => el('button', { class: 'filter-chip', type: 'button', 'aria-pressed': String(f.st === st), onclick: () => { f.st = st; subagentRender(); } }, label, el('span', { class: 'filter-count', text: String(count(st)) }));
    const list = el('ol', { class: 'timeline', id: 'subagent-runs', 'aria-label': 'journaled runs' });
    const detail = el('div', { id: 'subagent-detail', class: 'stack', 'data-gap': 's' });
    box.replaceChildren(
      section('live now', el('div', { id: 'subagent-live' })),
      el('div', { class: 'sidebar', style: { '--sidebar-width': '34rem', '--sidebar-content-min': '55%' } },
        el('div', { class: 'stack', 'data-gap': 's' },
          el('div', { class: 'filter', role: 'search' }, field, sel, el('div', { class: 'filter-chips', role: 'group', 'aria-label': 'how it ended' }, chip('all', 'all'), chip('ok', 'passed'), chip('bad', 'not passed')),
            el('span', { class: 'filter-note', id: 'subagent-count' })),
          el('div', { class: 'ledger-wrap', style: { '--ledger-max': '62vh' } }, list)),
        detail));
    subagentLive();
    subagentList();
    if (subagent.open) subagentOpen(subagent.open, false);
    // wide, the newest run opens beside the list (the hash stays the tab's until the reader picks)
    else if (subagent.runs.length && matchMedia('(min-width: 64rem)').matches) subagentOpen(subagent.runs[0].id, false, true);
    else detail.replaceChildren(el('p', { class: 'net-hint', text: 'Pick a run to read it whole: its commission, every step the gate weighed, how it ended and what it left unsaid.' }));
  }
  function subagentList() {
    const f = subagent.filter, list = $('#subagent-runs');
    if (!list) return;
    const shown = subagent.runs.filter(r => (!f.as || r.as === f.as) &&
      (f.st === 'all' || (f.st === 'ok' ? tone(r.status) === 'ok' : tone(r.status) !== 'ok')) &&
      (!f.q || (r.as + ' ' + r.ask + ' ' + r.status + ' ' + r.id).toLowerCase().includes(f.q)));
    $('#subagent-count').textContent = shown.length + ' of ' + subagent.runs.length;
    if (!shown.length) { list.replaceChildren(el('li', {}, empty('timeline-empty', subagent.runs.length ? 'no run matches' : 'no journaled run yet — a Subagent lands here when it runs'))); return; }
    list.replaceChildren(...shown.slice(0, 400).map(r => el('li', {}, el('button', { class: 'timeline-item', type: 'button', 'data-rank': rankOf(r.as) || false, 'data-id': r.id,
      'aria-current': String(r.id === subagent.open), 'aria-label': r.as + ' — ' + (r.ask || 'no ask') + ' — ' + (r.status || '—') + ' — ' + when(r.at),
      onclick: () => subagentOpen(r.id, true) },
    el('span', { class: 'timeline-time', text: short(r.at) }), el('span', { class: 'timeline-dot' }), el('span', { class: 'timeline-who', text: r.as }),
    el('span', { class: 'timeline-what', text: r.ask || '—', title: r.ask || '' }), el('span', { class: 'timeline-end', 'data-tone': tone(r.status), text: (r.status || '—').split(/\s/)[0] })))));
  }
  function subagentOpen(id, focus, auto) {
    subagent.open = id;
    if (!auto) D.setArg(id);
    for (const b of document.querySelectorAll('#subagent-runs .timeline-item')) { b.setAttribute('aria-current', String(b.dataset.id === id)); if (b.dataset.id === id && !auto) b.scrollIntoView({ block: 'nearest' }); }
    const box = $('#subagent-detail');
    if (!box) return;
    box.replaceChildren(el('p', { class: 'panel-note', text: 'reading ' + id + '…' }));
    getJSON('api/run?id=' + encodeURIComponent(id)).then(d => {
      const facts = [['run', d.id], ['started', when(d.at)], ['ended', d.ended ? when(d.ended) : '—'], ['length', ms(d.ms)], ['provider', d.provider || '—'], ['turns', String(d.turns)], ['steps', String(d.steps.length)], ['events', String(d.events)]];
      if (d.context && d.context.available) facts.push(['context', num(d.context.tokens) + ' / ' + num(d.context.limit) + ' · ' + (d.context.zone || '')]);
      if (d.exit) facts.push(['cut by', d.exit]);
      const head = el('article', { class: 'kcard', 'aria-labelledby': 'run-title' },
        el('div', { class: 'kcard-title', id: 'run-title' }, el('span', { text: d.as }), status(d.status), el('span', { class: 'panel-note', text: word('rank.' + rankOf(d.as), rankOf(d.as)) })),
        el('dl', {}, facts.map(([k, v]) => el('div', { class: 'kcard-row' }, el('dt', { text: k }), el('dd', { text: v })))));
      const steps = d.steps.map(s => {
        const card = el('details', { class: 'toolcard', 'data-class': s.class || false });
        card.append(el('summary', {}, el('span', { class: 'toolcard-name', text: s.tool || s.id }), el('span', { class: 'toolcard-sum', text: s.summary || s.why || '', title: s.summary || '' }), pill(s.status), s.ms ? el('span', { class: 'toolcard-ms', text: ms(s.ms) }) : null),
          el('div', { class: 'toolcard-body' }, el('pre', { class: 'toolcard-out', text: [
            'step ' + s.id + (s.attempts ? ' · ' + s.attempts + ' attempt' + (s.attempts > 1 ? 's' : '') : ''),
            s.class ? 'class: ' + s.class : '', s.decision ? 'gate: ' + s.decision + (s.why ? ' — ' + s.why : '') : '',
            s.summary ? 'asked: ' + s.summary : ''].filter(Boolean).join('\n') })));
        return card;
      });
      box.replaceChildren(head,
        section('the commission', el('pre', { class: 'toolcard-out', text: d.ask || '—' })),
        section('steps · ' + d.steps.length, ...(steps.length ? steps : [empty('panel-note', 'no step — the run answered in words')])),
        section('holes · ' + d.holes.length, feed(d.holes.map(h => ({ text: '@? ' + h, tone: 'warn' })), 'nothing left unsaid')));
      if (focus) head.querySelector('.kcard-title').setAttribute('tabindex', '-1');
    }).catch(err => box.replaceChildren(el('p', { class: 'net-hint', text: 'that run is not in the journals any more (' + err.message + ')' })));
  }

  // ---------- Memory: the tiers, a recall, the review ----------
  const mem = { q: '', tier: 'all', kind: '' };
  function memoryLoad() {
    return getJSON('api/memory').then(m => { mem.data = m; memoryRender(); }).catch(err => fail($('#memory-view'), 'the memory', err));
  }
  function tier(kind, label, asks, figure, unit, facts, silent) {
    return el('article', { class: 'tier', 'data-tier': kind, 'data-silent': !!silent },
      el('header', { class: 'tier-head' }, el('h3', { class: 'tier-name', text: label }), el('span', { class: 'tier-asks', text: asks })),
      el('div', { class: 'tier-figure' }, figure, el('small', { text: unit })),
      el('dl', { class: 'tier-facts' }, facts.filter(Boolean).flatMap(([k, v, t]) => [el('dt', { text: k }), el('dd', { 'data-tone': t || false, text: v })])));
  }
  function memoryRender() {
    const m = mem.data, box = $('#memory-view');
    readingNote('#memory-note', m.reading);
    const st = m.status;
    const kids = [];
    if (!m.reading.lit) kids.push(el('p', { class: 'net-hint', text: 'silent — ' + (m.reading.why || 'no memory instrument has spoken') }));
    if (st) {
      const cw = st.short.contextWindow || {}, wm = st.short.workingMemory || {}, sc = st.short.semanticCache || {};
      const lg = st.long || {}, sh = st.shared || {};
      const longN = (lg.episodic || 0) + (lg.procedural || 0) + (lg.semantic || 0);
      kids.push(el('div', { class: 'tiers' },
        tier('short', word('tier.short', 'short'), 'attention: what agents hold and keep asking',
          cw.available ? Math.round(100 * cw.tokens / (cw.limit || 1)) + '%' : '—', cw.available ? 'of the window' : 'window unmeasured', [
            cw.available ? ['context', num(cw.tokens) + ' / ' + num(cw.limit), /STRESS|LIMIT/i.test(cw.zone || '') ? 'warn' : 'ok'] : ['context', cw.why || 'unmeasured'],
            [word('workingNotes', 'working notes'), String(wm.workingNotes || 0)],
            (wm.stressed || []).length ? ['stressed', wm.stressed.join(', '), 'warn'] : null,
            (wm.atLimit || []).length ? ['at limit', wm.atLimit.join(', '), 'warn'] : null,
            ['semantic cache', num(sc.entries) + ' entries · ' + num(sc.hits) + ' hits']], !cw.available && !wm.workingNotes),
        tier('long', word('tier.long', 'long'), 'genomic: what the workspace did, and what is true',
          lg.indexed ? num(longN) : '—', lg.indexed ? 'memories indexed' : 'no index', lg.indexed ? [
            ['episodic', num(lg.episodic)], ['procedural', num(lg.procedural)], ['semantic', num(lg.semantic)],
            ['index', lg.stale ? 'stale — reindex' : 'fresh', lg.stale ? 'warn' : 'ok'], ['built', lg.builtAt ? when(lg.builtAt) : '—'], lg.head ? ['head', lg.head.slice(0, 10)] : null]
            : [['source', lg.src || '—']], !lg.indexed),
        tier('shared', word('tier.shared', 'shared'), 'lessons: what the workspace resolved to remember',
          num(sh.workspace || 0), word('workspace', 'workspace') + ' notes', [
            ['machine notes', num(sh.machine)],
            [word('unsaid', 'the unsaid'), word('kind.policy', 'policy') + ' ' + ((sh.unsaid || {}).policy || 0) + ' · ' + word('kind.team', 'team') + ' ' + ((sh.unsaid || {}).team || 0) + ' · ' + word('kind.ownership', 'ownership') + ' ' + ((sh.unsaid || {}).ownership || 0)],
            ['read from', (sh.src || []).join(' · ') || '—']], !(sh.workspace || sh.machine))));
    }
    if ((m.holes || []).length) kids.push(section('what the instrument cannot see · ' + m.holes.length, feed(m.holes.map(h => ({ text: '@? ' + h, tone: 'warn' })), '')));

    // recall: the memory searched as an agent would, read-only (no cache line written)
    const q = el('input', { class: 'filter-field', type: 'search', name: 'q', 'aria-label': 'recall: ask the memory', placeholder: 'recall — ask the memory anything', value: mem.q });
    const tierSel = el('select', { class: 'filter-select', 'aria-label': 'tier' }, ['all', 'long', 'shared'].map(t => el('option', { value: t, text: t === 'all' ? 'every tier' : t, selected: t === mem.tier })));
    const kindSel = el('select', { class: 'filter-select', 'aria-label': 'kind' }, el('option', { value: '', text: 'any kind' }), ['episodic', 'procedural', 'semantic', 'policy', 'team', 'ownership'].map(k => el('option', { value: k, text: k, selected: k === mem.kind })));
    const hits = el('div', { id: 'memory-hits', 'aria-live': 'polite' });
    const form = el('form', { class: 'filter', role: 'search', onsubmit: ev => { ev.preventDefault(); mem.q = q.value.trim(); mem.tier = tierSel.value; mem.kind = kindSel.value; recall(); } },
      q, tierSel, kindSel, el('button', { class: 'button', 'data-variant': 'primary', 'data-size': 's', type: 'submit', text: 'Recall' }));
    kids.push(section('recall', form, hits));

    // working notes and notes, side by side when there is room
    const workingNotes = ledger([{ t: word('skill', 'skill') }, { t: 'held by' }, { t: word('notes', 'notes'), num: true }, { t: word('stress', 'stress') }],
      (m.workingNotes || []).map(d => el('tr', { 'data-rank': rankOf(d.heldBy) || false }, td(name(d.skill, d.src)), td(d.heldBy || '—'), td(d.notes + ' / ' + d.limit, { 'data-num': true, 'data-lead': d.notes >= d.limit }),
        td(pill(String(d.stress || 'ok').toLowerCase())))), 'no ' + word('workingNotes', 'working notes') + ' on any ' + word('skill', 'skill'), '40vh');
    for (const p of workingNotes.querySelectorAll('.pill')) p.dataset.tone = /stress/i.test(p.textContent) ? 'danger' : /limit|near/i.test(p.textContent) ? 'warn' : 'ok';
    const kinds = Object.entries(m.byKind || {}).sort((a, b) => b[1] - a[1]).map(([k, n]) => k + ' ' + n).join(' · ');
    const notes = feed((m.notes || []).map(n => ({ text: n.text, meta: [el('span', { text: when(n.at) }), el('span', { class: 'feed-who', text: n.by || '—' }), n.kind ? el('span', { class: 'feed-tag', text: word('kind.' + n.kind, n.kind) }) : null, n.tag ? el('span', { text: '#' + n.tag }) : null].filter(Boolean) })), 'no shared note yet');
    if (notes.classList.contains('feed')) notes.style.setProperty('--feed-max', '40vh');
    kids.push(el('div', { class: 'switcher', style: { '--switch-at': '56rem' } },
      section(word('workingNotes', 'working notes') + 's · ' + (m.workingNotes || []).length, workingNotes),
      section('shared notes · ' + (m.notes || []).length + (kinds ? ' — ' + kinds : ''), notes)));

    // the review: each tier read with its own lens, every signal routed to the act it calls for
    const review = el('div', { id: 'memory-review', class: 'stack', 'data-gap': 's' }, empty('panel-note', 'The review reads each tier with its own lens and routes every recurring signal: provisioning a member, craft a skill, consolidate a lesson into policy, or fix the instrument. It reads, never writes.'));
    kids.push(section('review', el('div', { class: 'cluster' }, el('button', { class: 'button', 'data-size': 's', type: 'button', text: 'Read the review', onclick: reviewLoad })), review));
    box.replaceChildren(...kids);
    if (mem.q) recall();
  }
  function recall() {
    const box = $('#memory-hits');
    if (!mem.q) { box.replaceChildren(); return; }
    box.replaceChildren(empty('hits-empty', 'recalling…'));
    getJSON('api/memory/recall?q=' + encodeURIComponent(mem.q) + '&tier=' + mem.tier + (mem.kind ? '&kind=' + mem.kind : '')).then(r => {
      const top = Math.max(0.0001, ...r.results.map(x => x.score));
      const list = r.results.length ? el('ol', { class: 'hits', 'aria-label': r.results.length + ' memories for ' + mem.q }, r.results.map((x, i) => el('li', { class: 'hits-item' },
        el('span', { class: 'hits-n', text: String(i + 1).padStart(2, '0') }),
        el('div', { class: 'hits-head' }, el('span', { class: 'hits-kind', text: x.kind }), el('span', { class: 'hits-title', text: x.title || x.src }), el('span', { class: 'hits-anchor', text: x.src + (x.sec ? '#' + x.sec : '') + (x.when ? ' · ' + x.when : '') })),
        el('p', { class: 'hits-snippet', text: x.snippet || '' }),
        el('div', { class: 'hits-score', style: { '--hits-score': (100 * x.score / top).toFixed(0) + '%' }, title: 'score ' + x.score + ' · similarity ' + x.sim + ' · relation ' + x.rel + ' · recency ' + x.rec })))) : empty('hits-empty', 'no memory matched "' + mem.q + '"');
      box.replaceChildren(list, ...(r['@?'] || []).length ? [feed(r['@?'].map(h => ({ text: '@? ' + h, tone: 'warn' })), '')] : []);
    }).catch(err => box.replaceChildren(empty('hits-empty', 'recall refused: ' + err.message)));
  }
  function reviewLoad() {
    const box = $('#memory-review');
    box.replaceChildren(empty('panel-note', 'reading every tier…'));
    getJSON('api/memory/review').then(r => {
      if (r.silent || !(r.tiers || []).length) { box.replaceChildren(empty('panel-note', 'silent — ' + (r.silent || 'no tier to review'))); return; }
      box.replaceChildren(...r.tiers.map(t => section(t.name + ' — reads ' + t.reads,
        feed([...t.facts.map(f => ({ text: f })), ...t.findings.map(f => ({ text: f.signal + ' ×' + f.n, sub: [f.route, ...(f.evidence || []).slice(0, 2)].join(' · '), tone: 'warn' }))], 'nothing measured'))));
    }).catch(err => box.replaceChildren(empty('panel-note', 'the review did not answer: ' + err.message)));
  }

  // ---------- Toolbox: the registry, the session's skills and agents, what loaded ----------
  const tb = { q: '', kind: '', state: '' };
  function toolboxLoad() {
    return Promise.all([getJSON('api/toolbox'), getJSON('api/workspace').catch(() => ({ agents: [], skills: [] }))]).then(([t, w]) => { tb.data = t; tb.workspace = w; toolboxRender(); })
      .catch(err => fail($('#toolbox-view'), 'the toolbox', err));
  }
  const kpi = (label, value, sub, silent, t) => el('div', { class: 'kpi', 'data-silent': !!silent, 'data-tone': t || false }, el('span', { class: 'kpi-label', text: label }), el('span', { class: 'kpi-value', text: value }), el('span', { class: 'kpi-sub', text: sub }));
  function toolboxRender() {
    const t = tb.data, w = tb.workspace, box = $('#toolbox-view');
    readingNote('#toolbox-note', t.reading);
    const ins = t.instrument || {}, reg = t.registry || {};
    const kids = [];
    if (!t.reading.lit) kids.push(el('p', { class: 'net-hint', text: 'silent — ' + (t.reading.why || 'the registry is empty') }));
    kids.push(el('div', { class: 'kpis' },
      kpi('registry', num(t.entries.length), (reg.live ? 'live harvest' : 'on disk' + (reg.builtAt ? ' · built ' + when(reg.builtAt) : '')) + (reg.stale ? ' · stale' : ''), !t.entries.length, reg.stale ? 'warn' : false),
      kpi(word('toolbox.offered', 'offered'), num(ins.offeredDistinct), num(ins.offers) + ' briefs · ' + num(Math.round(ins.residentTokens || 0)) + ' resident tokens', !ins.offers),
      kpi(word('toolbox.loaded', 'loaded'), num(ins.loadedDistinct), num(ins.loads) + ' loads · ' + num(Math.round(ins.loadedTokens || 0)) + ' tokens', !ins.loads, 'second'),
      kpi('skills to hold', num((w.skills || []).length), 'every skill the session can hold', !(w.skills || []).length),
      kpi(word('agents', 'agents') + ' registered', num((w.agents || []).length), 'agents a ' + word('subagent_agent', 'ephemeral subagent') + ' rides', !(w.agents || []).length),
      kpi('external', num(t.external || 0), (t.missing || []).length ? 'missing: ' + t.missing.join(', ') : 'none missing', !t.external, (t.missing || []).length ? 'warn' : false)));
    if ((t.holes || []).length) kids.push(section('holes · ' + t.holes.length, feed(t.holes.map(h => ({ text: '@? ' + h, tone: 'warn' })), '')));

    const kinds = {}; for (const e of t.entries) kinds[e.kind] = (kinds[e.kind] || 0) + 1;
    const field = el('input', { class: 'filter-field', type: 'search', 'aria-label': 'filter the registry', placeholder: 'filter: name, description, holder', value: tb.q });
    const chips = el('div', { class: 'filter-chips', role: 'group', 'aria-label': 'kind' },
      el('button', { class: 'filter-chip', type: 'button', 'aria-pressed': String(!tb.kind), onclick: () => { tb.kind = ''; toolboxRender(); } }, 'all', el('span', { class: 'filter-count', text: String(t.entries.length) })),
      Object.keys(kinds).sort().map(k => el('button', { class: 'filter-chip', type: 'button', 'aria-pressed': String(tb.kind === k), onclick: () => { tb.kind = k; toolboxRender(); } }, word('toolbox.kind.' + k, k), el('span', { class: 'filter-count', text: String(kinds[k]) }))));
    const stateSel = el('select', { class: 'filter-select', 'aria-label': 'state' }, [['', 'any state'], ['loaded', word('toolbox.loaded', 'loaded')], ['offered', word('toolbox.offered', 'offered')], ['idle', 'idle']].map(([v, l]) => el('option', { value: v, text: l, selected: v === tb.state })));
    stateSel.addEventListener('change', () => { tb.state = stateSel.value; regRows(); });
    field.addEventListener('input', () => { tb.q = field.value.trim().toLowerCase(); regRows(); });
    kids.push(section('the registry', el('div', { class: 'filter', role: 'search' }, field, chips, stateSel, el('span', { class: 'filter-note', id: 'toolbox-count' })), el('div', { id: 'toolbox-reg' })));

    const skills = ledger([{ t: 'skill' }, { t: 'from' }, { t: 'what it is for', wide: true }],
      (w.skills || []).map(s => el('tr', {}, td(name(s.name)), td(tag(s.source)), td(s.description || '—', { class: 'ledger-text', 'data-wide': true }))), 'no skill found for the session', '44vh');
    const agents = ledger([{ t: word('agent', 'agent') }, { t: 'mode' }, { t: 'model', wide: true }, { t: 'from', wide: true }],
      (w.agents || []).map(b => el('tr', { 'data-rank': rankOf(b.name) || false }, td(name(b.name, b.description || '')), td(tag(b.mode)), td(b.model || '—', { 'data-wide': true }), td(b.source || '—', { 'data-wide': true }))), 'no registered agent', '44vh');
    kids.push(el('div', { class: 'switcher', style: { '--switch-at': '60rem' } }, section('what the session can hold · ' + (w.skills || []).length, skills), section(word('agents', 'agents') + ' registered · ' + (w.agents || []).length, agents)));
    const loads = feed((t.recent || []).map(e => ({ tone: e.ev === 'load' ? 'info' : false, text: (e.names || []).join(', ') || '—',
      sub: e.tokens ? num(Math.round(e.tokens)) + ' tokens' + (e.cmd ? ' · ' + e.cmd : '') : (e.cmd || ''),
      meta: [el('span', { text: e.at ? when(e.at) : '—' }), el('span', { class: 'feed-who', text: e.by || '—' }), el('span', { class: 'feed-tag', text: e.ev === 'load' ? word('toolbox.loaded', 'loaded') : word('toolbox.offered', 'offered') })] })), 'nothing offered or loaded yet');
    if (loads.classList.contains('feed')) loads.style.setProperty('--feed-max', '40vh');
    kids.push(section('what was offered and loaded, newest first', loads));
    box.replaceChildren(...kids);
    regRows();
  }
  function regRows() {
    const t = tb.data, host = $('#toolbox-reg');
    if (!host) return;
    const rows = t.entries.filter(e => (!tb.kind || e.kind === tb.kind) && (!tb.state || e.state === tb.state) &&
      (!tb.q || (e.name + ' ' + e.description + ' ' + (e.holders || []).join(' ') + ' ' + (e.path || '')).toLowerCase().includes(tb.q)));
    $('#toolbox-count').textContent = rows.length + ' of ' + t.entries.length;
    const maxFull = Math.max(0, ...rows.map(e => e.full));
    host.replaceChildren(ledger([{ t: 'name' }, { t: 'kind' }, { t: 'what it is for', wide: true }, { t: 'held by', wide: true }, { t: 'resident', num: true }, { t: 'full', num: true }, { t: 'offers', num: true, wide: true }, { t: 'loads', num: true }, { t: 'state' }],
      rows.map(e => { const p = pill(e.state === 'loaded' ? word('toolbox.loaded', 'loaded') : e.state === 'offered' ? word('toolbox.offered', 'offered') : 'idle'); p.dataset.tone = e.state === 'loaded' ? 'ok' : e.state === 'offered' ? 'info' : 'muted';
        return el('tr', {}, td(name(e.name, e.path || '')), td(tag(e.kind)), td(e.description || '—', { class: 'ledger-text', 'data-wide': true }),
          td((e.holders || []).join(', ') || '—', { 'data-wide': true }), td(num(Math.round(e.resident)), { 'data-num': true }), td(num(Math.round(e.full)), { 'data-num': true, 'data-lead': e.full === maxFull && maxFull > 0 }),
          td(num(e.offers), { 'data-num': true, 'data-wide': true }), td(num(e.loads), { 'data-num': true }), td(p)); }),
      t.entries.length ? 'no entry matches' : 'the registry is empty — no skill, command, tool or agent found', '60vh'));
  }

  // ---------- Config: the resolved config, layered with origins; the off-list ----------
  const cfg = { q: '', set: false };
  const isLeaf = v => v && typeof v === 'object' && !Array.isArray(v) && Object.keys(v).length === 2 && 'value' in v && 'origin' in v;
  const layerOf = o => { const s = String(o || ''); return !s ? 'no origin' : /^(\(unset\)|default|built-?in|policy)/i.test(s) ? 'default' : /^flag:/.test(s) ? 'flags (--set)' : /^env/i.test(s) ? 'environment' : home(s).replace(/:\d+$/, ''); };
  const home = s => String(s).replace(/^\/(home|Users)\/[^/]+/, '~');
  function configLoad() {
    return getJSON('api/config').then(c => { cfg.data = c; configRender(); }).catch(err => fail($('#config-view'), 'the config', err));
  }
  function leaves(v, path, out) {
    if (isLeaf(v)) { out.push({ path, value: v.value, origin: v.origin }); return out; }
    if (Array.isArray(v)) { v.forEach((x, i) => leaves(x, path + '[' + i + ']', out)); if (!v.length) out.push({ path, value: [], origin: '' }); return out; }
    if (v && typeof v === 'object') { for (const k of Object.keys(v).sort()) leaves(v[k], path ? path + '.' + k : k, out); return out; }
    out.push({ path, value: v, origin: '' }); return out;
  }
  function fmtVal(v) {
    if (v === true) return ['on', 'on'];
    if (v === false) return ['off', 'off'];
    if (v === null || v === undefined) return ['null', 'null'];
    if (Array.isArray(v)) return [v.length ? v.map(x => typeof x === 'object' ? JSON.stringify(x) : String(x)).join(', ') : '[]', v.length ? false : 'empty'];
    if (typeof v === 'object') return [JSON.stringify(v), false];
    if (v === '') return ['""', 'empty'];
    return [String(v), false];
  }
  function configRender() {
    const c = cfg.data, box = $('#config-view');
    readingNote('#config-note', c.reading);
    const kids = [];
    const off = c.off || [];
    kids.push(section(word('off_list', 'off-list') + ' — policy features this workspace switched off',
      !c.offKnown ? empty('panel-note', 'unknown — no config feed on this board') :
        off.length ? feed(off.map(o => ({ text: o.feature, sub: o.origin, tone: 'warn' })), '') : el('div', { class: 'cluster' }, (() => { const p = pill('everything on'); p.dataset.tone = 'ok'; return p; })())));
    if (!c.live || !c.tree) {
      kids.push(el('p', { class: 'net-hint', text: 'silent — ' + (c.reading.why || 'no config feed') }));
      box.replaceChildren(...kids);
      return;
    }
    const all = leaves(c.tree, '', []);
    const layers = {}; for (const l of all) { const k = layerOf(l.origin) || '—'; layers[k] = (layers[k] || 0) + 1; }
    const max = Math.max(1, ...Object.values(layers));
    kids.push(section('layers — where each value was written · ' + all.length + ' values',
      el('div', { class: 'bars' }, Object.entries(layers).sort((a, b) => b[1] - a[1]).map(([k, n]) => el('div', { class: 'bars-row' }, el('span', { class: 'bars-label', text: k, title: k }),
        el('span', { class: 'bars-track' }, el('span', { class: 'bars-fill', style: { '--bars-value': (100 * n / max).toFixed(1) + '%' } })), el('span', { class: 'bars-figure', text: String(n) }))))));
    const field = el('input', { class: 'filter-field', type: 'search', 'aria-label': 'filter the config', placeholder: 'filter: key, value or origin', value: cfg.q });
    field.addEventListener('input', () => { cfg.q = field.value.trim().toLowerCase(); tree(); });
    const setChip = el('button', { class: 'filter-chip', type: 'button', 'aria-pressed': String(cfg.set), onclick: () => { cfg.set = !cfg.set; setChip.setAttribute('aria-pressed', String(cfg.set)); tree(); } }, 'only what a file set');
    const fold = open => () => { for (const d of document.querySelectorAll('#config-tree details')) d.open = open; };
    kids.push(section('the resolved config — read-only', el('div', { class: 'filter', role: 'search' }, field, el('div', { class: 'filter-chips' }, setChip),
      el('button', { class: 'button', 'data-size': 's', 'data-variant': 'quiet', type: 'button', text: 'open all', onclick: fold(true) }),
      el('button', { class: 'button', 'data-size': 's', 'data-variant': 'quiet', type: 'button', text: 'fold all', onclick: fold(false) }),
      el('span', { class: 'filter-note', id: 'config-count' })), el('div', { class: 'cfgtree', id: 'config-tree' })));
    box.replaceChildren(...kids);
    tree();
  }
  function tree() {
    const host = $('#config-tree'), c = cfg.data;
    // docs: a line per key saying what it does (the binary's config.Describe)
    const docs = c.docs || {};
    const want = l => (!cfg.set || layerOf(l.origin) !== 'default') && (!cfg.q || (l.path + ' ' + fmtVal(l.value)[0] + ' ' + l.origin + ' ' + (docs[l.path] || '')).toLowerCase().includes(cfg.q));
    let shown = 0;
    const filtering = !!cfg.q || cfg.set;
    function node(key, v, path, depth) {
      if (isLeaf(v) || v === null || typeof v !== 'object' || (Array.isArray(v) && (!v.length || v.every(x => isLeaf(x) || typeof x !== 'object')))) {
        // a leaf, or a list of plain values: one row
        let value = v, origin = '';
        if (isLeaf(v)) { value = v.value; origin = v.origin; }
        else if (Array.isArray(v)) { value = v.map(x => isLeaf(x) ? x.value : x); origin = (v.find(isLeaf) || {}).origin || ''; }
        const l = { path, value, origin };
        if (!want(l)) return null;
        shown++;
        const [txt, kind] = fmtVal(value);
        return el('div', { class: 'cfgtree-leaf', 'data-match': !!cfg.q && path.toLowerCase().includes(cfg.q) },
          el('span', { class: 'cfgtree-key', text: key, title: path }), el('span', { class: 'cfgtree-val', 'data-kind': kind || false, text: txt }),
          origin ? el('span', { class: 'cfgtree-origin', 'data-layer': layerOf(origin) === 'default' ? 'default' : false, text: home(origin), title: origin }) : null,
          docs[path] ? el('span', { class: 'cfgtree-desc', text: docs[path] }) : null);
      }
      const keys = Array.isArray(v) ? v.map((_, i) => i) : Object.keys(v).sort();
      const kids = keys.map(k => node(Array.isArray(v) ? '[' + k + ']' : k, v[k], path ? path + (Array.isArray(v) ? '[' + k + ']' : '.' + k) : String(k), depth + 1)).filter(Boolean);
      if (!kids.length) return null;
      return el('details', { open: filtering || depth < 1 }, el('summary', {}, key, el('span', { class: 'cfgtree-count', text: String(kids.length) }),
        docs[path] ? el('span', { class: 'cfgtree-desc', text: docs[path] }) : null), kids);
    }
    const top = Object.keys(c.tree).sort().map(k => node(k, c.tree[k], k, 0)).filter(Boolean);
    host.replaceChildren(...(top.length ? top : [empty('cfgtree-empty', 'no value matches')]));
    $('#config-count').textContent = shown + ' values shown';
  }

  // ---------- wiring: load a tab when it shows, refresh it when its instrument moves ----------
  const tabsOf = { subagent: subagentLoad, memory: memoryLoad, toolbox: toolboxLoad, config: configLoad };
  const loaded = {};
  function show(v, arg) {
    if (!tabsOf[v]) return;
    if (v === 'subagent' && arg && arg !== subagent.open) { subagent.open = arg; if (subagent.loaded) { subagentRender(); return; } }
    if (!loaded[v]) { loaded[v] = true; tabsOf[v](); }
  }
  D.bus.addEventListener('view', e => show(e.detail.view, e.detail.arg));
  const refresh = { loop: 'subagent', subagent: 'subagent', memory: 'memory', toolbox: 'toolbox' };
  const timers = {};
  D.bus.addEventListener('board', e => {
    const v = refresh[e.detail];
    if (!v || !loaded[v]) return;
    clearTimeout(timers[v]);
    // a tab the reader is not looking at reloads when it is shown next
    timers[v] = setTimeout(() => { if (D.current() === v) tabsOf[v](); else loaded[v] = false; }, 1200);
  });
  show(D.current(), D.hashArg());
})();
