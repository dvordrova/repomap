#!/usr/bin/env node
"use strict";

const fs = require("fs");
const path = require("path");
const vm = require("vm");

const defaultQueries = [
  "как строится отчет",
  "deepseek",
  "как проверяется ответ модели",
  "runtime surfaces",
  "кеширование",
  "какие тут главные компоненты",
  "как добавить новый анализ",
  "как здесь используется go/packages",
];

const reportPath = process.argv[2];
if (!reportPath) {
  process.stderr.write("usage: semantic_search_eval.js REPORT_JSON [QUERY ...]\n");
  process.exit(2);
}

const absoluteReport = path.resolve(reportPath);
const report = JSON.parse(fs.readFileSync(absoluteReport, "utf8"));
const index = report.semantic_search;
if (!index || !Array.isArray(index.items)) {
  process.stderr.write("report has no semantic_search index\n");
  process.exit(1);
}

const assetPath = path.resolve(__dirname, "../internal/report/templates/semantic_search.js");
const window = { __REPOMAP_SEARCH_TEST__: {} };
vm.runInNewContext(fs.readFileSync(assetPath, "utf8"), { window });
const search = window.__REPOMAP_SEARCH_TEST__;
const queries = process.argv.length > 3 ? process.argv.slice(3) : defaultQueries;

const evaluation = queries.map((query) => {
  const ranked = search.rankSemanticSearchItems(index.items, query, 12);
  const first = ranked[0];
  return {
    query,
    status: first ? (first.complete ? "complete_match" : "partial_only") : "insufficient_evidence",
    first: first ? {
      id: first.item.id,
      kind: first.item.kind,
      title: first.item.title,
      summary: first.item.summary || "",
      target: first.item.target,
      score: first.score,
      coverage: first.coverage,
      exact: first.exact,
    } : null,
    exact_matches: ranked.filter((result) => result.exact).map((result) => ({
      id: result.item.id,
      kind: result.item.kind,
      title: result.item.title,
    })),
    related: ranked.slice(1, 4).map((result) => ({
      id: result.item.id,
      kind: result.item.kind,
      title: result.item.title,
      complete: result.complete,
      coverage: result.coverage,
    })),
  };
});

process.stdout.write(JSON.stringify({
  report: absoluteReport,
  index_version: index.version,
  item_count: index.items.length,
  truncated: Boolean(index.truncated),
  evaluation,
}, null, 2) + "\n");
