package main

const reviewCockpitHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Repomap Morning Review</title>
  <style>
    :root {
      color-scheme: light;
      --ink: #172033;
      --muted: #5d687b;
      --line: #dfe4ee;
      --paper: #ffffff;
      --wash: #f5f7fb;
      --fact: #165dba;
      --proposal: #8a4b08;
      --canonical: #087443;
      --mixed: #6d48b5;
      --rejected: #b42318;
      --unknown: #667085;
    }
    * { box-sizing: border-box; }
    body { margin: 0; background: var(--wash); color: var(--ink); font: 15px/1.55 ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
    main { width: min(1500px, calc(100% - 32px)); margin: 24px auto 80px; }
    h1 { font-size: clamp(32px, 5vw, 56px); line-height: 1.03; margin: 0 0 16px; letter-spacing: -0.04em; }
    h2 { margin: 0 0 14px; font-size: 26px; letter-spacing: -0.025em; }
    h3 { margin: 0 0 8px; font-size: 20px; }
    h4 { margin: 20px 0 8px; }
    p { margin: 6px 0 12px; }
    code { overflow-wrap: anywhere; font-size: .9em; }
    a { color: #155eef; text-decoration-thickness: 1px; text-underline-offset: 3px; }
    .hero, .section, .experiment, .frontier-card, .funnel, .stage, .goal { background: var(--paper); border: 1px solid var(--line); border-radius: 16px; }
    .hero { padding: clamp(24px, 5vw, 52px); box-shadow: 0 16px 44px rgba(16,24,40,.07); }
    .hero-copy { max-width: 880px; font-size: 19px; }
    .policy { margin-top: 20px; padding: 12px 16px; background: #eef4ff; border-left: 4px solid var(--fact); border-radius: 8px; }
    .section { margin-top: 22px; padding: 24px; }
    .grid { display: grid; gap: 16px; }
    .two { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    .three { grid-template-columns: repeat(3, minmax(0, 1fr)); }
    .pipeline { display: flex; gap: 10px; overflow-x: auto; padding: 4px 2px 12px; align-items: stretch; }
    .stage { min-width: 205px; padding: 14px; position: relative; }
    .stage:not(:last-child)::after { content: "→"; position: absolute; right: -9px; top: 48%; z-index: 2; color: #98a2b3; font-weight: 800; }
    .stage small, .muted { color: var(--muted); }
    .badge { display: inline-flex; align-items: center; padding: 3px 9px; border-radius: 999px; font-size: 12px; font-weight: 800; letter-spacing: .045em; text-transform: uppercase; border: 1px solid currentColor; }
    .fact { color: var(--fact); background: #eff8ff; }
    .proposal { color: var(--proposal); background: #fff6e8; }
    .canonical, .verified { color: var(--canonical); background: #ecfdf3; }
    .mixed { color: var(--mixed); background: #f4f0ff; }
    .rejected { color: var(--rejected); background: #fff1f0; }
    .unknown, .detected { color: var(--unknown); background: #f2f4f7; }
    .possible { color: #9a6700; background: #fffaeb; }
    .grammar { display: flex; flex-wrap: wrap; gap: 10px; }
    .grammar > div { border: 1px solid var(--line); border-radius: 10px; padding: 10px; min-width: 175px; }
    .goal { padding: 14px; }
    .experiment { padding: 20px; min-width: 0; }
    .experiment-head { display: flex; justify-content: space-between; gap: 18px; align-items: flex-start; }
    .meta { display: grid; grid-template-columns: 145px minmax(0, 1fr); gap: 5px 10px; margin: 16px 0; }
    .meta dt { color: var(--muted); }
    .meta dd { margin: 0; min-width: 0; overflow-wrap: anywhere; }
    .links { display: flex; gap: 10px; flex-wrap: wrap; margin: 14px 0; }
    .button { display: inline-block; border-radius: 9px; padding: 8px 12px; color: #fff; background: #155eef; font-weight: 700; text-decoration: none; }
    .proposal-warning { border: 2px solid #f4b740; background: #fff8e8; border-radius: 12px; padding: 12px; margin: 16px 0; font-weight: 800; color: #713b05; }
    details { border-top: 1px solid var(--line); padding: 12px 0; }
    summary { cursor: pointer; font-weight: 750; }
    .fact-card, .claim-card, .timeline-item { border: 1px solid var(--line); border-radius: 10px; padding: 12px; margin: 10px 0; scroll-margin-top: 16px; }
    .fact-card:target { outline: 3px solid #84adff; background: #f5f8ff; }
    .claim-title { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
    .chips { display: flex; gap: 6px; flex-wrap: wrap; margin-top: 8px; }
    .chip { background: #eef2f6; border-radius: 7px; padding: 2px 7px; font: 12px/1.6 ui-monospace, SFMono-Regular, Menlo, monospace; overflow-wrap: anywhere; }
    .lens { margin: 18px 0; border: 1px solid #c7d7fe; background: #f4f7ff; border-radius: 12px; padding: 14px; }
    .lens-strip { display: flex; gap: 8px; overflow-x: auto; padding: 8px 0; }
    .lens-button { min-width: 170px; border: 1px solid #84adff; border-radius: 9px; background: #fff; color: var(--ink); padding: 9px; text-align: left; cursor: pointer; }
    .lens-button.is-active { background: #155eef; color: #fff; }
    .lens-panel { min-height: 82px; border-top: 1px dashed #84adff; padding-top: 10px; }
    .aspect { display: grid; grid-template-columns: auto 1fr; gap: 8px; margin: 6px 0; }
    .timeline { border-left: 3px solid #d0d5dd; padding-left: 14px; }
    .timeline-item { margin-left: 4px; }
    .funnel { padding: 16px; }
    .funnel-numbers { display: grid; grid-template-columns: repeat(4, 1fr); gap: 8px; margin: 12px 0; }
    .number { padding: 8px; background: var(--wash); border-radius: 8px; text-align: center; }
    .number strong { display: block; font-size: 24px; }
    .frontier-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
    .frontier-card { padding: 16px; min-width: 0; }
    .frontier-card h3 { margin-top: 8px; }
    .ground { border-left: 3px solid #84adff; padding-left: 9px; margin: 7px 0; }
    .empty { border: 1px dashed #98a2b3; color: var(--muted); padding: 14px; border-radius: 10px; }
    .error { padding: 20px; color: var(--rejected); background: #fff; border: 2px solid var(--rejected); border-radius: 12px; }
    @media (max-width: 980px) { .two, .three, .frontier-grid { grid-template-columns: 1fr; } }
    @media (max-width: 640px) { main { width: min(100% - 16px, 1500px); margin-top: 8px; } .hero, .section, .experiment { padding: 16px; } .meta { grid-template-columns: 1fr; } .funnel-numbers { grid-template-columns: repeat(2, 1fr); } }
  </style>
</head>
<body>
<main>
  <div id="app"><div class="hero"><h1>Repomap Morning Review</h1><p>Loading saved artifacts…</p></div></div>
</main>
<script>
const esc = value => String(value == null ? "" : value)
  .replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;")
  .replaceAll('"', "&quot;").replaceAll("'", "&#39;");
const badge = status => '<span class="badge ' + esc(status) + '">' + esc(status) + '</span>';
const link = item => '<a class="button" href="' + esc(item.href) + '" target="_blank" rel="noreferrer">' + esc(item.label) + '</a>';
const chips = values => values && values.length ? '<div class="chips">' + values.map(v => '<span class="chip">' + esc(v) + '</span>').join('') + '</div>' : '';
const locationText = e => esc(e.path || "unknown") + (e.line ? ':' + e.line : '') + (e.column ? ':' + e.column : '');
let reviewData;

function renderPipeline(data) {
  return '<section class="section"><h2>Evidence → explanation → product</h2><div class="pipeline">' +
    data.pipeline.map(stage => '<div class="stage">' + badge(stage.authority.toLowerCase()) + '<h3>' + esc(stage.name) + '</h3><small>' + esc(stage.explanation) + '</small></div>').join('') +
    '</div></section>';
}

function renderGrammar(data) {
  return '<section class="section"><h2>Permanent status grammar</h2><div class="grammar">' +
    data.grammar.map(item => '<div>' + badge(item.status.toLowerCase()) + '<p>' + esc(item.explanation) + '</p></div>').join('') +
    '</div></section>';
}

function renderGoals(data) {
  return '<section class="section"><h2>Queue status</h2><div class="grid three">' +
    data.goals.map(goal => '<div class="goal"><div class="muted">Goal ' + goal.number + '</div><h3>' + esc(goal.name) + '</h3>' + badge(goal.status === 'passed' || goal.status === 'canonical' ? 'canonical' : goal.status) + '<p>' + esc(goal.result) + '</p></div>').join('') +
    '</div></section>';
}

function renderFact(exp, fact) {
  const evidence = (fact.evidence || []).map(e => '<span class="chip">' + locationText(e) + (e.label ? ' · ' + esc(e.label) : '') + '</span>').join('');
  const source = fact.source ? '<p><strong>Saved window:</strong> ' + esc(fact.source.path) + ':' + fact.source.start_line + '–' + fact.source.end_line + (fact.source.enclosing_symbol ? ' · ' + esc(fact.source.enclosing_symbol) : '') + '</p>' : '';
  return '<article class="fact-card" id="fact-' + esc(exp.slug) + '-' + esc(fact.id) + '">' +
    '<div class="claim-title">' + badge('fact') + '<code>' + esc(fact.id) + '</code>' + (fact.used ? badge('canonical') : '') + '</div>' +
    '<p>' + esc(fact.statement) + '</p>' + source +
    '<p><strong>Manifest role:</strong> ' + esc(fact.role) + '</p>' + chips(fact.capabilities || []) + '<div class="chips">' + evidence + '</div></article>';
}

function renderClaim(exp, claim) {
  const status = claim.validation_status === 'retained_canonical' ? 'canonical' : claim.validation_status === 'retained_unknown' ? 'unknown' : 'rejected';
  const support = (claim.support_ids || []).map(id => '<a class="chip" href="#fact-' + esc(exp.slug) + '-' + esc(id) + '">' + esc(id) + '</a>').join('');
  const evidence = (claim.evidence || []).map(e => '<span class="chip">' + locationText(e) + '</span>').join('');
  return '<article class="claim-card"><div class="claim-title">' + badge('proposal') + badge(status) + '<strong>Claim ' + (claim.index + 1) + ': ' + esc(claim.title) + '</strong></div>' +
    '<p>' + esc(claim.text) + '</p><p><strong>Basis:</strong> ' + esc(claim.basis) + '</p>' +
    '<p><strong>Support IDs:</strong></p><div class="chips">' + support + '</div>' +
    (claim.answer_aspect_ids && claim.answer_aspect_ids.length ? '<p><strong>Answer aspects:</strong></p>' + chips(claim.answer_aspect_ids) : '') +
    '<p><strong>Exact evidence:</strong></p><div class="chips">' + evidence + '</div></article>';
}

function renderLens(exp) {
  const targets = exp.trace_targets || [];
  return '<div class="lens"><div class="claim-title"><strong>Presentation-only evidence lens</strong>' + badge('fact') + '</div>' +
    '<p class="muted">Each target is derived from existing canonical Step evidence. The left-to-right order is editorial; it does not assert a new runtime relation.</p>' +
    '<div class="lens-strip">' + targets.map((target, index) => '<button class="lens-button' + (index === 0 ? ' is-active' : '') + '" data-exp="' + esc(exp.slug) + '" data-trace="' + index + '"><strong>' + (index + 1) + '. ' + esc(target.step_title) + '</strong><br><code>' + locationText(target.target) + '</code></button>').join('') + '</div>' +
    '<div class="lens-panel" id="lens-panel-' + esc(exp.slug) + '"></div></div>';
}

function renderCoverage(exp) {
  return '<p><strong>' + exp.coverage.covered + '/' + exp.coverage.required.length + '</strong> aspects covered; <strong>' + exp.coverage.key_covered + '/' + exp.coverage.key_total + '</strong> key aspects covered.</p>' +
    (exp.coverage.required || []).map(a => '<div class="aspect">' + badge(a.covered ? 'canonical' : 'unknown') + '<div><strong>' + esc(a.label) + '</strong><br><code>' + esc(a.id) + '</code>' + (a.key ? ' · key' : '') + '</div></div>').join('');
}

function renderTimeline(exp) {
  return '<div class="timeline">' + exp.validation.timeline.map(item => '<div class="timeline-item"><div class="claim-title">' + badge(item.status) + '<strong>' + esc(item.label) + '</strong></div><p>' + esc(item.detail) + '</p></div>').join('') + '</div>';
}

function renderExperiment(exp) {
  const diagnostics = (exp.validation.diagnostics || []).map(d => '<div class="claim-card">' + badge(d.code.startsWith('historical_') ? 'rejected' : 'mixed') + '<strong> ' + esc(d.code) + '</strong>' + (d.proposed || d.derived ? '<p>Proposed: <code>' + esc(d.proposed) + '</code> → derived: <code>' + esc(d.derived) + '</code></p>' : '') + chips(d.reasons || []) + '</div>').join('') || '<p>No aggregate verdict mismatch.</p>';
  const raw = exp.raw_artifacts.map(item => '<li><a href="' + esc(item.href) + '" target="_blank" rel="noreferrer">' + esc(item.label) + '</a></li>').join('');
  return '<article class="experiment" id="experiment-' + esc(exp.slug) + '"><div class="experiment-head"><div><div class="muted">' + esc(exp.repository_namespace) + '</div><h2>' + esc(exp.name) + '</h2></div><div>' + badge('canonical') + ' ' + badge(exp.canonical.verdict) + '</div></div>' +
    '<p>' + esc(exp.state_explanation) + '</p>' +
    '<dl class="meta"><dt>Repository</dt><dd>' + esc(exp.repository) + (exp.repository_path ? ' · <code>' + esc(exp.repository_path) + '</code>' : '') + '</dd><dt>Revision</dt><dd><code>' + esc(exp.revision) + '</code></dd><dt>Question</dt><dd>' + esc(exp.question) + '</dd><dt>Mechanism</dt><dd><code>' + esc(exp.canonical.mechanism_id) + '</code></dd><dt>Semantic hash</dt><dd><code>' + esc(exp.canonical.semantic_content_hash) + '</code></dd><dt>Artifact</dt><dd><code>' + esc(exp.canonical.artifact_id) + '</code></dd><dt>Artifact hash</dt><dd><code>' + esc(exp.canonical.artifact_sha256) + '</code></dd></dl>' +
    '<div class="links">' + exp.links.map(link).join('') + '</div>' +
    '<div class="proposal-warning">' + esc(exp.model_proposal.notice) + '</div>' +
    '<div class="grid two"><div><strong>Model-proposed verdict</strong><p>' + badge(exp.model_proposal.proposed_verdict) + '</p></div><div><strong>Locally derived canonical verdict</strong><p>' + badge(exp.validation.derived_verdict) + '</p></div></div>' +
    renderLens(exp) +
    '<details open><summary>Validation timeline</summary>' + renderTimeline(exp) + diagnostics + '</details>' +
    '<details><summary>Question</summary><p>' + esc(exp.question) + '</p></details>' +
    '<details><summary>Facts (' + exp.fact_summary.total + ' total; ' + exp.fact_summary.claim_support + ' claim-support; ' + exp.fact_summary.candidate_seeds + ' seeds; ' + exp.fact_summary.available_unused + ' available-unused)</summary>' + exp.facts.map(f => renderFact(exp, f)).join('') + '</details>' +
    '<details><summary>Model claims and claim support (' + exp.model_proposal.claims.length + ')</summary><p>' + esc(exp.model_proposal.summary) + '</p>' + exp.model_proposal.claims.map(c => renderClaim(exp, c)).join('') + '</details>' +
    '<details><summary>Covered aspects</summary>' + renderCoverage(exp) + '</details>' +
    '<details><summary>Unknowns (' + exp.unknowns.length + ')</summary>' + (exp.unknowns.map(u => '<p>' + badge('unknown') + ' ' + esc(u) + '</p>').join('') || '<p>None retained.</p>') + '</details>' +
    '<details><summary>Validation result</summary><p>Current status: ' + badge(exp.validation.current_status === 'accepted' ? 'canonical' : exp.validation.current_status) + '</p>' + exp.validation.checks.map(c => '<p><strong>' + esc(c.name) + ':</strong> ' + esc(c.status) + '</p>').join('') + diagnostics + '</details>' +
    '<details><summary>Canonical artifact</summary><p>' + badge('canonical') + ' ' + badge(exp.canonical.verdict) + '</p><pre><code>' + esc(JSON.stringify(exp.canonical, null, 2)) + '</code></pre></details>' +
    '<details><summary>No-model replay</summary><p>' + esc(exp.replay.result) + '</p><pre><code>' + esc(JSON.stringify(exp.replay, null, 2)) + '</code></pre></details>' +
    '<details><summary>Raw artifacts</summary><ul>' + raw + '</ul></details></article>';
}

function renderFunnels(data) {
  return '<section class="section" id="funnels"><h2>Experiment funnel</h2><p>One clean card is not semantic coverage. Broad artifacts and canonical Mechanisms are deliberately counted separately.</p><div class="grid two">' +
    data.funnels.map(f => '<div class="funnel"><h3>' + esc(f.repository) + '</h3><div class="funnel-numbers">' +
      [['opportunities',f.opportunities],['selected',f.selected],['investigated',f.investigated],['proposals',f.model_proposals],['broad accepted',f.accepted_broad_artifacts],['rejected',f.rejected_broad_proposals],['unexplained',f.unexplained],['canonical follow-up',f.follow_up_canonical_mechanisms]].map(n => '<div class="number"><strong>' + n[1] + '</strong>' + esc(n[0]) + '</div>').join('') +
      '</div><p>' + esc(f.explanation) + '</p></div>').join('') + '</div></section>';
}

function renderFrontierCard(card) {
  const grounds = (card.why_suspected || []).map(g => '<div class="ground"><code>' + esc(g.fact_id) + '</code><p>' + esc(g.statement || 'Saved supporting fact') + '</p>' + chips(g.locations || []) + '</div>').join('');
  return '<article class="frontier-card"><div class="claim-title">' + badge(card.status) + '<span class="muted">' + esc(card.repository) + '</span></div><h3>' + esc(card.question) + '</h3><p>' + esc(card.status_explanation) + '</p>' +
    '<details><summary>Why suspected</summary>' + grounds + '</details>' +
    '<details><summary>Evidence and gaps</summary><h4>Available evidence</h4>' + chips(card.available_evidence || []) + '<h4>Missing evidence</h4>' + (card.missing_evidence || []).map(x => '<p>' + badge('unknown') + ' ' + esc(x) + '</p>').join('') + chips(card.missing_capabilities || []) + (card.missing_source ? '<p class="muted">Source: ' + esc(card.missing_source) + '</p>' : '') + ((card.diagnostics || []).length ? '<h4>Saved rejection diagnostics</h4>' + chips(card.diagnostics || []) : '') + '</details>' +
    '<h4>Suggested next bounded probe</h4><p><strong>' + esc(card.suggested_next_probe) + '</strong></p><p class="muted">Question only — never Start Here, Search truth, or coding-agent authority.</p></article>';
}

function renderFrontier(data) {
  const collapsed = data.frontier_collapsed.map(c => '<p><strong>' + esc(c.repository) + ':</strong> ' + c.count + ' lower-ranked saved candidates collapsed. <a href="' + esc(c.raw_link) + '" target="_blank">Open raw opportunities</a>.</p>').join('');
  return '<section class="section" id="frontier"><h2>Explore possibilities</h2><p>Recall-first research remains visible without turning hypotheses into knowledge. Candidate text is always a question; model planning notes are labelled separately from locally retained gaps.</p>' +
    '<div class="frontier-grid">' + data.coverage_frontier.map(renderFrontierCard).join('') + '</div>' +
    '<div class="empty"><strong>Unavailable</strong><p>' + esc(data.unavailable_note) + '</p></div>' + collapsed + '</section>';
}

function activateTrace(expSlug, index) {
  const exp = reviewData.experiments.find(item => item.slug === expSlug);
  if (!exp || !exp.trace_targets[index]) return;
  document.querySelectorAll('[data-exp="' + expSlug + '"]').forEach((button, current) => button.classList.toggle('is-active', current === index));
  const trace = exp.trace_targets[index];
  const panel = document.getElementById('lens-panel-' + expSlug);
  panel.innerHTML = '<div class="claim-title">' + badge('fact') + '<strong>' + esc(trace.step_title) + '</strong></div><p><strong>Current target:</strong> <code>' + locationText(trace.target) + '</code>' + (trace.target.label ? ' · ' + esc(trace.target.label) : '') + '</p><p><strong>Inspector evidence for this step:</strong></p>' + chips((trace.evidence || []).map(locationText)) + '<p class="muted">' + esc(trace.disclaimer) + '</p>';
}

function render(data) {
  reviewData = data;
  document.getElementById('app').innerHTML = '<header class="hero"><div class="muted">DEV-ONLY · SAVED ARTIFACTS</div><h1>What are we testing?</h1><div class="hero-copy"><p><strong>Repomap tries to turn bounded repository evidence into reusable explanations.</strong></p><p>Deterministic code establishes facts. The model groups and explains those facts. Local validators decide which claims are acceptable. Canonical Mechanisms power Start Here, Search and evidence navigation.</p></div><div class="policy">' + esc(data.source_policy) + '</div></header>' +
    renderPipeline(data) + renderGrammar(data) + renderGoals(data) +
    '<section class="section"><h2>Caddy and chi, side by side</h2><p>The explanation itself may be useful. It is not canonical until every claim passes validation and the local system derives a consistent verdict.</p><div class="grid two">' + data.experiments.map(renderExperiment).join('') + '</div></section>' +
    renderFunnels(data) + renderFrontier(data);
  document.querySelectorAll('[data-trace]').forEach(button => button.addEventListener('click', () => activateTrace(button.dataset.exp, Number(button.dataset.trace))));
  data.experiments.forEach(exp => activateTrace(exp.slug, 0));
}

fetch('data.json', {cache: 'no-store'}).then(response => {
  if (!response.ok) throw new Error('HTTP ' + response.status);
  return response.json();
}).then(render).catch(error => {
  document.getElementById('app').innerHTML = '<div class="error"><h1>Serve this directory over HTTP</h1><p>The review could not load <code>data.json</code>: ' + esc(error.message) + '</p><p><code>python3 -m http.server 8765 --bind 127.0.0.1 --directory /Users/dvordrova/git/repomap/tmp/repomap-review</code></p></div>';
});
</script>
</body>
</html>`
