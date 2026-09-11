# Ревью промптов и кубиков repomap по реальным запросам (2026-09-10, HEAD `4447ebbe`)

## Метод и охват

Правило 7 из основного документа: промпт читается только рядом с отправленным запросом.
Взяты три сохранённых прогона одного дня: Go `20260910-135426-go-http-server`, Python
`20260910-090532-python-tutorial-game`, TypeScript `20260910-112213-webernetes`
(`work/runs-validation-final-real`). Это 235 запросов (28 + 28 + 179) по 18 стадиям, полные
system/user-сообщения из `.llm-cache/payloads`, ответы и `rejected.jsonl`. Прочитаны все
31 файл промптов (87,7 КБ): 13 в `internal/atlas/lines/prompts`, learning ×3, questionbatch,
orientation, documentationreduce ×2, terminology ×3, reporttranslation, targetportfolio ×4,
readmetargetscout ×2, llm-преамбула ×2.

Два скрипта в scratchpad `prompt-review/`: `enum_leak.py` (каждое значение полей
`kind/invocation/resolution/detail/…` в запросе ищется в тексте промпта, заметках `fill`
и суффиксе терминологии; отчёт `enum-leak.md`), `samples.py` (по одному запросу и ответу
на стадию, язык и вариант таблицы, 56 файлов в `samples/`). Всё ниже проверено по этим
файлам и по коду; где не проверено, написано.

## Итог одной таблицей

| # | Кубик | Что не так | Улика | Вес |
|---|---|---|---|---|
| 1 | все табличные (symbols, operations, joints, files, answer, learn, question, orientation, portfolio) | внутренний словарь уходит модели без определения: 20 значений в symbols, 14 в operations, 16 в orientation, 10 в portfolio, 15 в answer, 18 в learn | `enum-leak.md`; `invokes_external` в 71 запросе symbols и 30 operations на всех трёх языках | высокий |
| 2 | operations (Python, JS) | `name` свободный текст, закрытого `http_path` нет; модель сама сочиняет путь | игра: модель написала `GET /levels`, `GET /levels/{level_id}`, `POST /run_level`; факты знают `/api/levels`, `/api/level/{level_id}`, `/api/level/run`; в атласе обе версии, в joints шесть peers на три ручки | высокий |
| 3 | operations (все) | заметка `u* for an upstream operation` при `entry_options: [self, none]`; модель пишет буквально `u*`, строка отклоняется | Go: `InitHandler$1`, `ReinitHandler$1` отклонены («cell entry is u*, not one of the options»), операции потеряны | высокий |
| 4 | question (retrieval) | один конфликт в одном вопросе отклоняет всю партию | `questionbatch.go:634-636` `return Response{}`; Go: три партии по 28 вопросов отклонены и куплены заново | высокий, нарушает правило 1 |
| 5 | symbols, operations | таблица привязок с одной строкой рендерится как `"columns": null, "rows": [null]` | `evidence.go:71-88`; 560 запросов symbols, 365 operations | средний |
| 6 | symbols (отбор) | активация и исходящие спрашиваются у строк без единого вызова и вызывающего | 1 034 из 5 073 строк, из них 709 переменных/констант | средний, деньги |
| 7 | symbols (отбор) | к таблицам из одних закрытых выборов пристёгнут словарный придаток | 77 из 86 запросов symbols; TS: 60 отклонённых наборов терминов | средний, деньги |
| 8 | answer | восемь одинаковых фраз `scope` повторяются в каждой строке | 16–29 КБ на прогон, это инструкция внутри данных | средний, деньги |
| 9 | arrows, zones | тестовые файлы рисуют половину стрелок карты и попадают в части | TS: 169 из 342 стрелок с тестовым свидетелем; часть «Test Harnesses and Examples» | средний, качество карты |
| 10 | files | выбор `box` из одного варианта плюс свободный `new:` | 290 из 674 строк с `box_options: ["here"]` | низкий, деньги |
| 11 | joints | вход собран из модельных границ: `time.Sleep`, `findContainerStatusByName`, `beginPath` предлагаются как исходящие | Go peers r1 = `time.Sleep` → `none`; TS r1–r4 локальные хелперы | следствие дефекта 15 |
| 12 | orientation | `kind: dynamic_execution` в фактах вне закрытого списка промпта; `kind` связей это слаги подписей; 20 тем коммитов «fix readme» как claims | запрос Go | низкий |
| 13 | files.md, directories.md, symbols.md, targets.md | текст промпта описывает поля, которых нет, или не в той форме | `directory_facts` 0 из 674; `parent` = «209 files: apis, models, index.ts», а не «одна строка о родителе»; фраза про операции осталась в промпте подписей после разделения | низкий |
| 14 | portfolio, targets, readme scout | три словаря ролей одной сущности | `standalone/seed_of/shared_code/tool/example` против `product/library/fixture/tool/example` против `target_entry/example_entry/test_entry/support_tool_entry` | низкий, но путает |
| 15 | answer, learn, question (TS) | один запрос на 862 470 входных токенов, второй на 601 202; запрос retrieval 4 МБ упал у провайдера до деления | `exchange.v3.json` Webernetes | известно (U5), напоминание |

## Подробно

### 1. Утечка словаря (все табличные кубики)

Полная таблица в `enum-leak.md`. Опасные, то есть те, где модель наверняка придаёт слову
смысл, которого в коде нет:

- symbols, operations: `kind: invokes_external` (структурный факт «вне индекса», см. дефект
  15), `invocation: construct` (67 запросов Python/TS: конструктор объекта, модель может
  читать как «строит запрос»), `resolution: alternatives / unresolved`, `invocation:
  declared_interface_dispatch:*`, `interface_invoke:*`, `function_value_call:*`,
  `callback_transfer:*`, `callable_binding:field / jsx_attribute`, `deferred`, `kind:
  decorates` (Python: декоратор `get` без объекта роутера и пути, см. п. 2). Ни одно из этих
  слов промпт не определяет; `symbols.md` для подписей говорит только «calls это нейтральные
  наблюдения: имена, литералы, аргументы, строки».
- portfolio: `go_main`, `own_main_packages`, `own_main_consumers`, `other_module_imports`,
  `name_main_guard`, `python_shebang`, `launch_root`, `declared_distribution_membership` не
  определены; промпт определяет только префикс `documented_`, `launch_file_executable` и
  `module_level_relative_import`. Правила промпта («отсутствие своих main», «импорт из
  другого модуля репозитория») модель должна сама сопоставить с этими ключами.
- orientation: в промпте закрытый список видов фактов (`entrypoint, http_route, http_call,
  portal, config_read, risk, manifest, negative, dead_module, dependency`), а в запросе
  Python приходит `kind: dynamic_execution`. `connections[].kind` это слаг подписи
  (`registers_get_hello_handler`), не вид.
- answer, learn, question: `level: files_or_observed_entities`, `place_kind: source_fact`,
  `component_kind`, `entrypoint_kind: callable`, `known_kind: other` — служебные имена
  внутри улик. Низкий риск, но это тот же класс.
- report_translation: `role: reason/basis/label/summary/remaining/term-explanation` не
  определены, а промпт просит «короткие подписи оставлять короткими» — модель не знает, что
  из этого подпись.

Что делать: правило 8 основного документа плюс тест по `enum_leak.py` на фикстурах трёх
языков; для symbols/operations перевести `kind`/`invocation`/`resolution` в слова до
рендера (например `callee_scope: indexed | external_package | platform`, `how: call |
construct | defer | interface`) и определить каждое в одном абзаце промпта.

### 2. Python и JS операции: путь сочиняет модель

В Go таблица операций даёт закрытые `name_kind_options`, `registered_names` и
`registered_name_options` (`http_path` выбирается как `p*`, текст пути восстанавливает код).
В Python и TS `fill` это `entry, activation, name, description`: `name` свободный текст на
60 рун, регистраций нет, в `calls` только `{"kind":"decorates","name":"get","line":18}` —
без объекта роутера и без пути. Модель в игре написала `GET /levels`, `GET /levels/{level_id}`,
`POST /run_level`. Факты (`facts/http.go`) знают `/api/levels`, `/api/level/{level_id}`,
`/api/level/run`. В `atlas.json` цели `main` по каждой ручке две входящие границы: факт
`http_server /api/levels` и модельная `other` с `values: ["GET /levels", "get_levels_info"]`.
В HTML неверный путь не показан (0 вхождений `GET /levels`), то есть проекция сейчас его
прячет, но joints получает шесть peers на три ручки, где p4–p6 неверные, и следующая правка
проекции покажет обе версии.

Что делать: Python/JS получают тот же контракт, что Go: маршрут из фактов как закрытый
`http_path`, `name` без путей вообще; модельная граница на объявлении, у которого уже есть
фактическая граница того же направления, не создаётся, а привязывается (то, что заявлено в
`c123befd` для страницы, должно быть и в атласе).

### 3. Заметка с шаблоном `u*`

`operations.go:15`: `Note: "self when this declaration is directly activated; u* for an
upstream operation; none without an operation"`. Строка `InitHandler$1` пришла с
`entry_options: ["self","none"]` и `observed_callers: null`, хотя `callers: 1`
(`callable_binding:field` от `InitHandler`). Модель написала `u*`, строка отклонена, объявление
осталось без операции. Второй такой же случай `ReinitHandler$1`. Отклонение построчное (правило 1
соблюдено), но сама причина — шаблон в заметке, которого нет среди вариантов.

Что делать: заметка перечисляет реальные варианты строки (генерируется из `entry_options`);
привязанные через `callable_binding` объявления получают привязывающего как кандидата `u1`,
либо строка не задаётся вовсе.

### 4. Партия вопросов отклоняется целиком

`questionbatch.go:634-636`: если модель выбрала одну и ту же строку улик дважды с разной
`relevance` или `why`, `return Response{}, fmt.Errorf("conflicting selection …")` — вся партия
из 28 вопросов отклоняется и покупается заново (Go: три раза, 14 записей в `rejected.jsonl`,
каждая с `count: 28`). Это нарушение правила 1 «отклонять строку, не окно»; для вопросов
строка это один вопрос. На Webernetes партия retrieval стоит 584–610 тыс. входных токенов,
повтор из-за одного дубля это заметные деньги.

Что делать: при дубле оставлять первое выделение (или объединять якоря), отклонять только
этот вопрос, остальные принимать; тест на это.

### 5. `"rows": [null]`

`evidence.go:71-88`: столбец считается общим, если совпадает во всех строках после первой;
при одной привязке все столбцы «общие», `Columns` пуст, `Rows` = `[nil]`. Модель видит
`"columns": null, "rows": [null]` с заметкой «Row numbers start at 1». 560 запросов symbols и
365 operations. Исправление: при одной строке отдавать обычный объект без разложения.

### 6–8, 10. Деньги без смысла

- Отбор символов: 714 строк переменных/констант получают вопросы `activation` и `outbound`;
  у 709 из них нет ни вызывающих, ни вызовов. Всего 1 034 из 5 073 строк с нулевыми
  уликами. Пропускать ячейки по условию (`When` уже есть) или не отправлять такие строки.
- Придаток терминологии (`terminology/prompts/adjunct.md` плюс каталог `REPOMAP_TERMINOLOGY_
  CATALOG_V3`) пристёгнут к 77 из 86 запросов symbols, где нет ни одной прозаической ячейки;
  результат: 60 отклонённых наборов терминов на TS, ноль пользы, лишние ~2 КБ входа и
  выходные токены на каждое окно (на Airflow 1 208 окон). Придаток только там, где есть
  `text`/`prose` ячейки.
- answer: поле `scope` из восьми одинаковых фраз («Names, signatures and author documentation
  guide reading; function bodies … were not inspected by this pass» и т. д.) повторяется в
  каждой строке: 15,9 / 24,4 / 28,7 КБ на прогон. Это инструкция, ей место один раз в
  `context` или в промпте.
- files: 290 из 674 строк имеют единственный вариант `box_options: ["here"]`, при этом
  разрешён `new:` — выбор без выбора, но с приглашением придумать. Не запрашивать `box`,
  когда вариант один.
- orientation: 20 тем коммитов в `claims` у Go, из них «fix readme» ×4 и «better readme» ×2.
  Шум; темы коммитов без содержания отфильтровать.

### 9. Тесты на карте

Webernetes: 169 из 342 строк arrows имеют свидетеля из `*.test.ts` / `testing/` / `test/`
(«Node status → Container Test Fakes: setters.test calls FakeVersion.constructor»). В zones
среди коробок «Client Tests (16 files)», «Test Harnesses (15 files)», в семи частях
«Test Harnesses and Examples». Половина связей на карте это тестовые рёбра — вот откуда
«85 связей на 21 узле». Тесты исключать из arrows и zones детерминированно (по конфигурации
тестов, как в критерии закрытия из раздела 3), оставляя их в инвентаре и в ответах.

### 11. Joints ест мусор

Peers-запрос Go: единственная строка `a = time.Sleep` (`line: ""`, `source: model`) с
кандидатом `p1`; модель ответила `none` — верно, но вызов куплен зря. TS: `r1–r4` это
локальные хелперы `findContainerStatusByName`, `findMatchingContainerRestartRule` как
«исходящие границы» библиотеки. Python: peers p1–p3 (факты) и p4–p6 (модель, неверные пути)
на те же три ручки. Всё это следствия дефекта 15 и п. 2; после них joints должен получать
только фактические и принятые модельные границы, без дублей на одно объявление.

### 12–14. Текст промптов разошёлся с данными

- `files.md` описывает `directory_facts` («extracted description») — ни одна из 674 строк
  его не несёт.
- `directories.md`: «one line about its parent directory», в данных `parent` это
  «209 files: apis, models, index.ts».
- `symbols.md` (подписи) сохранил абзац «A function returning a command/router object
  constructs an operation; the callback doing its work implements it» — после разделения
  отбора и подписей (`db66ac09`) он относится к другой таблице.
- `boundaries.md` называет поле `kind_given`, в уликах answer/learn то же называется
  `known_kind`.
- Роли целей: portfolio решает `standalone / seed_of / shared_code / tool / example`;
  `targets.md` просит `role` из `product / library / fixture / tool / example`; в строках
  приходит `selected_role: product`, которого `targets.md` не определяет (частично
  `target_descriptions.md`); readme scout классифицирует `target_entry / example_entry /
  test_entry / support_tool_entry / …`. Три словаря на одно понятие, и `fixture` в одном
  против `seed_of` в другом.
- `detail` привязок Go: `server.Handler.HelloWorld <- func() (…) func() (…)` — стрелка и
  дважды сигнатура без объяснения; в TS `invocation: ""` и `detail: ""`.
- `documentationreduce`: `content_trust: "untrusted_repository_text"` токен вместо фразы, в
  orientation та же мысль дана предложением.

### 15. Размер запросов

Webernetes: answer 2,89 МБ и 862 470 входных токенов за один вызов (153 с), learn 2,2 МБ и
601 202 токена, question 4,03 МБ упал у провайдера (`provider_failed`, 0 токенов) и потом
прошёл двумя партиями по 584 и 610 тыс. Один отказ = потеря всего окна; деление сейчас
только по таймауту/500 (U5). Вопросы и ответы паковать по бюджету заранее, как делает
`table.Windows` для остальных таблиц.

## Что в порядке

Закрытые ссылки (`c*`, `p*`, `u*`, `e*`, `f*`) и запрет копировать текст из данных;
независимость строк и построчное отклонение в табличных кубиках; `When` для условных
ячеек; преамбула языка ответа; явный список видов фактов в orientation (кроме
`dynamic_execution`); строгий `documentationreduce`; `answer.md` длинный, но внутренне
согласованный; валидатор действительно отклонил `u*` вместо того, чтобы принять.

## Правила, которых не хватает (к разделу 8 основного документа)

9. Заметки и опции не содержат шаблонов (`u*`): перечисляются реальные варианты строки.
10. Ячейка не запрашивается у строки без улик для неё: `activation`/`outbound` без вызовов и
    вызывающих, `box` при одном варианте.
11. Словарный придаток только к таблицам с прозаическими ячейками.
12. Любая ошибка проверки партии деградирует до отклонения строки (вопроса); тест.
13. Текст промпта о полях сверяется с отрендеренным запросом тестом: поле, которого нет в
    запросе, не описывается; поле, которое есть, определено.

## Как проверить без модели

`repomap read --through <stage>` на сохранённых входах трёх прогонов, затем по
отрендеренным запросам: `enum_leak.py` даёт ноль неопределённых значений; `"rows":[null]`
ноль; придаток отсутствует в закрытых таблицах; число строк отбора с пустыми
`call_options` и запрошенным `outbound` ноль; в arrows/zones нет тестовых свидетелей;
`scope` встречается один раз на запрос. Потом один модельный прогон на тех же трёх
репозиториях: отклонённых строк operations по `u*` ноль, партии вопросов не
перепокупаются, у Python-ручек нет модельных путей, отличных от фактов.
