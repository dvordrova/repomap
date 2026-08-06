# MORNING-229 — D229 «полезная проекция существующих данных» завершён

**Дата:** 2026-08-06 · **Статус:** PHASE 0–8 выполнены, 11 коммитов на `main` (a31e1dc..fbb4d8c), push — по сигналу владельца.

## Что сделано

**Decision 229** (product projection vertical) — decision-документ + 10 implementation-коммитов.
D1–D9 все реализованы и верифицированы на живом отчёте casdoor (run `20260806-085142-casdoor-edc27c5754c1`, 22 компонента, 8 групп, 11 study-тем):

| Phase | Что | Ключевые артефакты |
|---|---|---|
| 0 | baseline, дефекты воспроизведены | casdoor 041149 |
| 1 | decision + 11 bounded правок + final red-team **PASS** | `docs/agent-room/decisions/229-product-projection-vertical.md` |
| 2 | Overview: периметр (observed use/entry → analyzed scope → touchpoints), value-shaped gate, remainder-not-principal | `overview_projection_d229_test.go` |
| 3 | Study: collapsed-by-default, ≤2 previews, независимые Evidence/Scope бейджи, группировка символов | `study_progressive_disclosure_d229_test.go` |
| 4 | Architecture: SVG-рёбра passive (0 tabindex), B/R коалесценция 8→4, инспектор | `architecture_canvas.js` |
| 5 | Mechanism: отдельные lane-карточки, human copy, raw enums под Evidence details | `mechanism_fragment_asset_test.go` |
| 6 | D9 палитра: navy #14213a / blue #3157d5 / teal #0f766e / amber #a16207 / violet #f0edff | `style.css` |
| 7 | D7 item-scope salvage: unknown member/anchor → component skip, duplicate → local dedup, exact twin → skip; провайдер-фри матрица 5 репо | `landscape.go`, `validation.go` |
| 8 | 3 fresh-ревью (code/semantic/product-ux) → все findings применены | см. ниже |

## PHASE 7 — провайдер-фри матрица

Offline-прогоны точным бинарём: **casdoor, etcd, restic, telebot, chatto** — все `state: ready`, manifest + report.json + report.html верифицированы. (Ночные run'ы v12 не реплеятся на контракте v14 — это ожидаемый digest-инвариант, не регрессия.)

## PHASE 8 — ревью и repair

Три fresh-context ревьюера (`deleg_f4b9b4ed`, полные вердикты получены после завершения батча):
- **Code/regression — NOT PASS (4 fixes):** `::root` → `:root` (палитра D9 не применялась!), D4 mechanism fixture, **sibling-poisoning** (пустая после salvage подсистема резала валидные sibling-подсистемы), **missing componentSalvaged** в duplicate-member ветке. Все применены.
- **Semantic/authority — CONDITIONAL PASS (2 fixes):** **R1 wire-path whole-reject** — resolveSynthesisWireProposal возвращал whole-response ошибки на unknown refs, делая D7 item-scope salvage недостижимым из live-пайплайна; теперь ref-resolution failures дропают только компонент (recoverable), valid siblings публикуются accepted_partial, zero-valid → whole reject с точной причиной. **R2 mixed-component monotonic regression** — valid members, собранные до item-scope drop, исчезали из ландшафта; reference-counted release возвращает их в remainder.
- **Product/UX + a11y — NOT PASS (5 fixes):** truthful-empty инспектор (9 вопросов), hero-дубликат README (Go-fix), raw label'ы operation/surface → human copy, **R1 Overview entry surfaces ванишали** без github links → location-only text spans (только когда нет embedded snippets), **R3 relations inventory raw enums** → human group labels («Настраивает границу безопасности», «Статическая структурная поддержка») с raw под details.

Итог: **74/74 Go-тестов green, 0 FAIL; vet чист; golden актуален; 2 новых детерминированных теста** (subsystem sibling-poisoning, mixed-member remainder release).

## Верификация (реальный бинарь)

- `make build` → `.bin/repomap`; offline-gate: `repomap --offline --no-open --no-serve --debug-dir …` на 5 репо.
- Браузер 1440×1000: Overview (5 ответов, периметр, remainder), Study (11 тем collapsed), Architecture (инспектор 9 вопросов, 4 merged rows, 0 focusable edges), Mechanism (48 lanes, 0 raw-утечек).
- Мобильный 390×844 (headless Chrome): hero-текст переносится, инспектор видим (не скрыт canvas media-query), overflow отсутствует.
- Скриншоты: `tmp/hermes-product-projection-20260806-094500/screenshots/after/` (10 шт).

## Файлы

- Decision: `docs/agent-room/decisions/229-product-projection-vertical.md`; CURRENT.md указывает на 229.
- Тесты: `overview_projection_d229_test.go`, `study_progressive_disclosure_d229_test.go`, обновлённые asset-тесты.
- Артефакты: `tmp/hermes-product-projection-20260806-094500/` (STATE.json, RUN_LOG.md, checks/, screenshots/).

## Коммиты (13)

a31e1dc decision · c648df5 P2 · f84efe1 P3 · 9575358 P4 · e1d8c44 P5 · 9a0f088 P6 · da57888 P7 · 2ce476c mobile · 53ad898 mechanism-human · 035efa5 :root · fbb4d8c PHASE 8 repairs · NEW 9412a72 full review verdicts (semantic wire salvage, monotonic release, location-only entries, relations labels)

## Ожидает владельца

- Push (`git push origin main`) — только по твоему сигналу.
- Финальный просмотр отчёта: `~/Library/Caches/repomap/runs/20260806-085142-casdoor-edc27c5754c1/report.html?t=review#/overview`
