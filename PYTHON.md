# Prompt: Python analyzer vertical slice

## Контекст

`repomap` — local-first инструмент для изучения незнакомых репозиториев. Сейчас основной поддерживаемый язык — Go: локальный детерминированный слой собирает факты, компактный bounded bundle может быть передан LLM, а точечное исследование символа делается через gopls.

Цель этой ветки работы — понять, насколько естественно добавить Python как еще один анализатор, не превращая core в набор языковых `if` и не включая Python-анализ во весь default survey раньше времени.

Первый продуктовый срез должен быть узким: **точечное исследование Python-символа по `file:line[:column]`**, аналогичное текущему gopls-сценарию. Полноценный Python onboarding и репозиторная карта — отдельный следующий этап.

## Что уже установлено про текущую архитектуру

В проекте уже есть хороший language-neutral фундамент:

- `internal/analyzer/analyzer.go` содержит небольшие порты `Provider`, `LocationResolver` и `ExactSymbolAnalyzer`;
- `internal/evidence/evidence.go` описывает общий граф сущностей и отношений, certainty, provenance и scenario;
- `internal/reportserver/server.go` принимает анализаторы через интерфейсы (`LocationResolver`, `ExactSymbolAnalyzer`, `ReferenceFinder`);
- gopls реализует эти интерфейсы отдельным адаптером в `internal/analyzer/golang/gopls`.

То есть gopls уже в значительной степени является подключаемым «кубиком». Но разъем вокруг него пока местами имеет форму Go.

Найденные Go-specific протечки:

- `internal/componentprobe/collect.go` создает выбранную сущность с `Language: "go"`;
- `componentstudy.SymbolCandidate` не несет язык;
- `internal/sourcecard/card.go` жестко работает с Go-файлами и ищет границу исходника как следующий top-level `func`;
- `internal/testevidence/testevidence.go` распознает только `_test.go`;
- `internal/symbol/bundle.go` содержит gopls-specific diagnostics/regex и предпочитает `_test.go`;
- `internal/reportserver/investigation.go` импортирует конкретный gopls package ради версии collector и использует Go-specific freshness context;
- `internal/freshness/freshness.go` хранит `GoVersion`, `GOOS` и `GOARCH`;
- `cmd/repomap/main.go` создает конкретный `*goplsanalyzer.Analyzer`;
- default repository survey построен на `internal/gofacts`, поэтому точечный Python-анализ существенно проще полной Python-поддержки onboarding.

Вывод: **не нужно переписывать core**, но перед полноценным подключением Python понадобится постепенно обобщить несколько policy-слоев вокруг анализатора.

## Что можно получить от Pyright

Pyright language server поддерживает необходимые LSP-операции:

- `textDocument/documentSymbol`;
- `workspace/symbol`;
- definition и references;
- `textDocument/prepareCallHierarchy`;
- `callHierarchy/incomingCalls`;
- `callHierarchy/outgoingCalls`.

Официальные точки для проверки актуального поведения:

- [регистрация LSP capabilities в Pyright](https://github.com/microsoft/pyright/blob/main/packages/pyright-internal/src/languageServerBase.ts#L655-L688);
- [реализация call hierarchy](https://github.com/microsoft/pyright/blob/main/packages/pyright-internal/src/languageService/callHierarchyProvider.ts);
- [конфигурация Pyright](https://github.com/microsoft/pyright/blob/main/docs/configuration.md).

В отличие от удобного one-shot CLI режима текущего gopls-адаптера, code-navigation API Pyright предполагает LSP-сессию поверх JSON-RPC:

1. запустить `pyright-langserver --stdio`;
2. отправить `initialize` и `initialized`;
3. открыть документ через `textDocument/didOpen`;
4. запросить symbols / prepareCallHierarchy / incomingCalls / outgoingCalls / references;
5. корректно выполнить `shutdown` и `exit`.

Поэтому полезно рассмотреть маленький reusable `internal/lspclient` для stdio JSON-RPC. Он пригодится и для будущего rust-analyzer. При этом language-specific преобразование LSP-ответов в evidence должно оставаться в адаптере Pyright.

## Контракт установки Pyright

Для первого Python MVP нормально считать `pyright-langserver` опциональным внешним toolchain-компонентом, как gopls для Go-focused анализа.

Факты и решение:

- официальный open-source Pyright распространяется прежде всего через npm и запускается как `pyright-langserver --stdio`;
- Pylance поставляется вместе с Python extension для VS Code и использует Pyright внутри, но это отдельный продукт с собственной лицензией и внутренним layout;
- нельзя искать и запускать случайный JS-файл внутри VS Code/Pylance extension: это нестабильный непубличный installation contract;
- на тестовом Mac в `PATH` не было Node, npm, `pyright` или `pyright-langserver`;
- для эксперимента Pyright 1.1.411 был поставлен только во временный venv через Python wrapper; wrapper локально загрузил Node 26.5.0;
- глобальное окружение и shell profile в ходе эксперимента не менялись.

Рекомендуемый MVP discovery order:

1. явный CLI path, например `--pyright-langserver /path/to/pyright-langserver`;
2. отдельная переменная окружения, если она действительно понадобится для CI/company setup;
3. `pyright-langserver` из `PATH`;
4. при отсутствии — короткая actionable ошибка с официальной командой `npm install -g pyright` и ссылкой на документацию.

Не делать скрытый download при обычном `repomap` run. Позже можно отдельно обсудить явный `repomap doctor python --install` или pinned managed tool cache, но это отдельное решение про supply chain, offline use и обновления.

Ссылки:

- [Pyright repository](https://github.com/microsoft/pyright);
- [Pylance FAQ: отношения Pylance, Pyright и Python extension](https://github.com/microsoft/pylance-release/blob/main/FAQ.md);
- [Pylance marketplace: внешний Pyright version/path](https://marketplace.visualstudio.com/items?itemName=ms-python.vscode-pylance).

## Проведенный эксперимент: 2026-07-11

Эксперимент проводился вне tracked-кода repomap: временная fixture, одноразовый JSON-RPC client и shallow clone beets. Поэтому он проверяет техническую гипотезу, но еще не является реализацией `cmd/pyright-playground`.

### Минимальная fixture

Fixture содержала:

```text
main.py -> app/service.py::process -> normalize
                                  -> Repository.save
             ^
             |
      tests/test_service.py
```

Pyright корректно вернул:

- exact document symbol `process`;
- incoming calls из `main.py::run` и `tests/test_service.py::test_process`;
- outgoing calls в `normalize` и `Repository.save`;
- references отдельно для declaration, imports и call sites.

Динамический пример `getattr(target, method_name)(value)` подтвердил границу: Pyright не разрешил фактический target. В outgoing он вернул шесть overload locations встроенного `getattr` из typeshed. Правильная нормализация здесь:

- дедуплицировать overload noise;
- отделить repo-local target от stdlib/external target;
- сохранить сам вызов `getattr` как evidence;
- пометить вызываемую через него runtime-цель как `unresolved_dynamic`, а не пытаться угадать ее.

### Реальный репозиторий: beets

На shallow clone beets Pyright работал без установки зависимостей самого проекта и показал полезный первый слой:

- `pyproject.toml` детерминированно объявляет CLI entrypoint `beet = "beets.ui:main"`; это не LSP-факт и должно собираться отдельным local facts provider;
- для `beets/ui/__init__.py:954` (`main`) Pyright нашел вход из `beets/__main__.py` и тестового helper path;
- единственный важный repo-local следующий шаг из `main` — `_raw_main`; остальные raw outgoing в основном относятся к обработке ошибок и stdlib stubs;
- `_raw_main` статически раскрыл parser/config/logging/setup/library/plugin calls, но **не раскрыл** динамический dispatch `subcommand.func(...)` до конкретных command handlers;
- `_setup` раскрыл `plugins.load_plugins()`, `plugins.commands()`, `_open_library()` и event send;
- top-level `plugins.commands` раскрыл вызов базового `BeetsPlugin.commands`, но не 50+ override implementations в plugin classes;
- Pyright 1.1.411 не реализовал стандартный `textDocument/prepareTypeHierarchy`: сервер ответил `Method not found`;
- одно имя `commands` оказалось неоднозначным внутри одного файла: naïve lookup выбрал class method вместо top-level function. Exact `file:line` выбрал правильный символ.

Raw Pyright output также содержал повторяющиеся targets (`plugins.send`) и внешние typeshed paths. До evidence graph обязательны:

- canonical identity `(provider, repo-relative path, range, symbol kind)`;
- дедупликация одинакового target/callsite;
- явная классификация `repository`, `stdlib`, `dependency`, `outside_workspace`;
- перевод LSP coordinates с zero-based line/character в единый внутренний формат;
- объединение полного range из `documentSymbol` с более узким name range из call hierarchy;
- версия Pyright, полученная отдельной командой: initialize response в этом эксперименте не заполнил `serverInfo`.

### Стоимость одной сессии

Один локальный, не бенчмарковый прогон на beets дал:

- initialize: около 0.40 s;
- первый `documentSymbol` большого `beets/ui/__init__.py`: около 1.48 s;
- первый incoming-call запрос после индексации: около 0.91 s;
- следующие incoming requests: около 0.05–0.06 s;
- prepare/outgoing: примерно 1–2 ms;
- references: примерно 4–22 ms;
- три символа (`_setup`, `_raw_main`, `main`) в одной сессии: около 2.97 s до shutdown.

Отдельный cold process на каждый символ занимал примерно 3–4 s. Следовательно, для интерактивного drill-down нужен один bounded long-lived Pyright process на repository/run, а не новый процесс на каждый click. Предварительная оптимизация индекса не нужна: сначала достаточно lifecycle, timeout, лимитов и измерений.

### Вывод эксперимента

Техническая гипотеза подтверждена частично:

- Pyright хорошо подходит для exact symbol, direct static calls и references;
- одного Pyright недостаточно для Python system map, plugin registry и event-driven dispatch;
- рядом нужен дешевый deterministic Python facts/syntax provider для manifests, entrypoints, class inheritance, decorators и известных registry shapes;
- он не должен превращаться в repository-wide semantic call graph: Pyright остается focused evidence cube;
- динамические пробелы являются полезным результатом и должны визуализироваться, а не маскироваться моделью.

## Какие факты для Python реалистично получать

Для выбранного символа можно извлекать:

- точное объявление и диапазон `file:line:column`;
- тип символа: функция, метод, класс и т.п.;
- входящие вызовы и их call sites;
- исходящие вызовы;
- references;
- bounded source window или точный диапазон тела;
- ссылки из тестов;
- версию провайдера, операцию, конфигурацию и scenario.

Это достаточно похоже на текущий gopls vertical slice, чтобы использовать общий `evidence.Graph` и downstream pipeline.

## Границы достоверности

Статический Python-анализ по природе менее полный, чем Go. Он может не восстановить фактический runtime flow из-за:

- monkey patching;
- `getattr` / `setattr`;
- динамических импортов;
- декораторов и метаклассов;
- строковых регистраций обработчиков;
- dependency injection;
- framework magic в Django, FastAPI, Celery и подобных системах.

Нужно сохранять существующую модель доказательности:

- успешно разрешенная статическая связь — `static`;
- эвристическая или неоднозначная связь — `possible`;
- предположение только модели — `hypothesis`;
- отсутствие связи в Pyright не означает, что runtime-связи нет;
- call hierarchy нельзя называть полным или доказанным runtime trace.

Каждая связь должна иметь provenance: provider, version, operation, исходную location и scenario/config context.

## Следующий tracked vertical slice

Временный proof-of-concept подтвердил протокол. Следующий шаг — перенести только минимальный reusable результат в полностью изолированный `cmd/pyright-playground`, не меняя default survey, browser report и LLM pipeline.

Пример интерфейса:

```sh
go run ./cmd/pyright-playground \
  --repo ../python-project \
  --path app/service.py \
  --line 42
```

Playground должен:

1. найти явно указанный или установленный `pyright-langserver`, но ничего не устанавливать скрыто;
2. инициализировать workspace с корректным repository root;
3. разрешить символ строго по заданной location, а не только по имени;
4. подтвердить его точную identity/location;
5. запросить bounded incoming calls, outgoing calls и references;
6. преобразовать результат в общий `evidence.Graph`;
7. вывести JSON для ручного изучения и сравнения с gopls;
8. корректно завершить LSP-процесс;
9. вернуть partial result с понятным warning, если отдельная capability ничего не дала.

Для tracked-проверки достаточно сохранить крошечную fixture-репу:

```text
main.py -> service.py -> repository.py
             ^
             |
      tests/test_service.py
```

Стоит добавить один намеренно динамический вызов и убедиться, что инструмент не выдумывает для него точную связь.

## Acceptance criteria первого среза

- выбранный Python callable разрешается по `file:line[:column]`;
- два одноименных символа в одном файле разрешаются однозначно по location;
- location, symbol kind и диапазон объявления корректны;
- incoming/outgoing/references bounded и представлены общим evidence contract;
- все пути внутри результата repository-relative;
- stdlib/dependency/outside-workspace locations не смешиваются с repository entities;
- одинаковые overload/target/callsite records дедуплицируются;
- zero-based LSP coordinates не протекают в пользовательский `file:line` contract;
- provenance содержит Pyright version и название LSP operation;
- scenario учитывает Python/Pyright/config inputs, но не протекает чувствительными абсолютными путями в model-facing bundle;
- `evidence.Graph` проходит валидацию;
- отсутствие call hierarchy дает честный partial/warning, а не пустую «успешную» истину;
- никакого LLM-вызова;
- никакой интеграции в default repository survey;
- один analyzer instance переиспользует LSP-сессию для нескольких bounded запросов и корректно завершает процесс;
- `getattr`/registry dispatch сохраняется как unresolved dynamic boundary без выдуманного target;
- отсутствие binary дает actionable setup error и не запускает implicit download;
- минимальные тесты проверяют contract и могут быть дешево выброшены/переписаны вместе с playground.

## Tracked vertical slice: 2026-07-11

Первый изолированный срез теперь реализован без подключения к default survey,
browser report или LLM pipeline:

- `cmd/pyright-playground` принимает `--repo`, `--path`, `--line`, optional
  `--column` и explicit `--pyright-langserver`;
- `internal/lspclient` держит одну bounded stdio JSON-RPC session, отвечает на
  server-side `workspace/configuration` requests и корректно делает
  `shutdown` / `exit`;
- `internal/analyzer/python/pyright` реализует `LocationResolver` и
  `ExactSymbolAnalyzer`, а incoming/outgoing/references складывает в общий
  `evidence.Graph`;
- common evidence различает `repository`, `stdlib`, `dependency` и
  `outside_workspace`; внешние entities не сохраняют абсолютные toolchain
  paths;
- tracked fixture содержит production caller, test caller, два `process` в
  одном файле и отдельный `getattr` boundary;
- `make pyright-fixture PYRIGHT_LANGSERVER=/path/to/pyright-langserver`
  запускает ручной smoke без model call.

Реальный smoke на Pyright 1.1.411 подтвердил:

- `app/service.py:8` разрешается в top-level `process`, находит callers из
  `main.py` и `tests/test_service.py`, а также direct outgoing в `normalize`,
  `Repository` и `Repository.save`;
- `app/service.py:14` независимо разрешается в одноименный method;
- `app/service.py:18` сохраняет один дедуплицированный stdlib target `getattr`
  и warning `unresolved`; runtime method по строке не выдумывается;
- все repository locations в JSON one-based и repository-relative;
- provider version записывается как `pyright 1.1.411`.

Два важных технических уточнения эксперимента:

1. `pyright-langserver --version` не является version command: langserver
   требует `--stdio`/другой transport. Адаптер берет версию через соседний
   executable `pyright --version`, когда он доступен.
2. Pyright language server по умолчанию использует `openFilesOnly`, поэтому
   incoming calls и cross-file references сначала были пустыми. Playground
   явно передает `python.analysis.diagnosticMode=workspace` через стандартный
   `workspace/configuration` и делает response-sized `workspace/symbol` index
   barrier с заведомо отсутствующим query. Это дает полные fixture callers, но
   стоимость такого barrier еще нужно измерить на beets/qutebrowser до
   продуктовой интеграции.

Следующие пункты ниже остаются архитектурной очередью, а не частью уже
подключенного Python onboarding.

## Ordinary CLI orientation: 2026-07-11

Первый Python onboarding теперь подключен к обычному пользовательскому пути:

```bash
cd /path/to/python-repository
repomap
```

Этот запуск использует общий language-neutral tracked-file survey, определяет
Python по `language_hints`, строит bounded `candidate_file_index` и закрытый
`allowed_paths`, затем выполняет тот же один orientation-вызов и открывает тот
же browser report. В индекс попадают Python source/test files и conventional
entrypoint filenames (`__main__.py`, `main.py`, `cli.py`, `app.py`,
`manage.py`). Полный file tree и Python source модели не передаются.

Pyright для первого отчета не требуется и скрыто не устанавливается. Он остается
отдельным focused cube для точного `file:line` исследования. Поэтому текущий
Python report честно дает верхнеуровневые направления и grounded файлы, но пока
не обещает Python import graph, framework registries, inheritance map или
browser symbol drill-down. Для них по-прежнему нужен отдельный deterministic
Python facts provider и явное подключение focused analyzer.

## Следующие архитектурные шаги после успешного playground

Двигаться небольшими независимыми изменениями:

1. добавить `Language` в `componentstudy.SymbolCandidate`;
2. убрать hardcoded `go` из `componentprobe`;
3. добавить маленький stdio JSON-RPC lifecycle client с timeout/cancellation/limits;
4. добавить `internal/analyzer/python/pyright`, реализующий существующие consumer-owned порты;
5. выделить чтение bounded source window за отдельный маленький интерфейс; для Python использовать полный `documentSymbol` range, когда он доступен;
6. вынести определение test path в языковую policy (`_test.go` против `test_*.py`, `*_test.py`, `tests/`);
7. заменить парсинг текстов gopls warnings на структурированные provider diagnostics/codes;
8. обобщить freshness/toolchain context, не теряя Go-specific детали;
9. выбирать analyzer через явный language/toolchain registry;
10. только после этого подключать Python exact-symbol path к component study/report server.

Не следует создавать один огромный интерфейс `LanguageAnalyzer`. Лучше сохранять небольшие интерфейсы со стороны потребителей и компоновать только нужные capabilities.

Отдельная доказанная потребность — Python syntax facts, которых нет в стандартном Pyright LSP. Перед реализацией нужно маленьким экспериментом сравнить два варианта: вызов bounded Python `ast` helper и embedded parser. Не компенсировать отсутствие type hierarchy россыпью regex по production-коду.

## Продуктовый сценарий Вани: понять Python-утилиту и подготовить переписывание на Go

Исходный запрос пользователя:

> Понять структуру Python-утилиты, а затем по этой информации составить prompt для переписывания на Go.

Здесь repomap не должен решать, какой код «лишний». Это продуктово-контекстное решение человека, которого невозможно вывести только из отсутствия статических references.

Инструмент должен подготовить человеку:

1. entrypoints, CLI commands и способы запуска;
2. карту компонентов и их ответственности;
3. несколько главных flows от входа до side effects;
4. конфигурацию, форматы данных, файловый/сетевой I/O и внешние процессы/API;
5. plugin/registry/event boundaries и места, где static analyzer не видит runtime target;
6. используемые библиотеки с объяснением роли, а не просто dependency list;
7. тестовые сценарии как observable behavior evidence;
8. список открытых вопросов и decision slots, которые заполняет человек;
9. компактный migration brief, из которого можно собрать prompt для Go-реализации.

Migration prompt должен описывать сохраняемое поведение и контракты, а не просить модель построчно переводить Python-файлы. В нем нужно четко разделить:

- **local facts** — что найдено детерминированно;
- **static evidence** — что поддерживает Pyright в конкретном scenario;
- **unknowns** — что зависит от runtime/config/plugins;
- **user decisions** — что владелец решил сохранить, изменить или не переносить;
- **model proposal** — предлагаемая Go-архитектура, которую еще нужно проверить.

Никаких автоматических `unused`, `dead` или `remove` verdicts по умолчанию. References и reachability показываются как материал для решения человека.

Если доступна реальная утилита Вани, она должна стать главным product calibration repository: владелец сравнивает карту с собственным пониманием, отмечает пропуски и оценивает, достаточно ли migration brief для содержательного Go prompt.

## Лестница calibration repositories

Не использовать пять больших репозиториев как единый pass/fail suite. Каждый нужен для отдельной гипотезы:

1. **synthetic fixture** — точность location/calls/references и честная динамическая граница;
2. **утилита Вани** — полезность структуры и migration brief для реального владельца;
3. **beets** — CLI entrypoint, команды, модели, pipeline и plugin registry;
4. **qutebrowser** — command registry и event-driven GUI;
5. **mitmproxy** — несколько пользовательских интерфейсов поверх общего ядра и сетевые side effects;
6. **Home Assistant** — динамические integrations/events и большое количество runtime registration;
7. **Airflow** — поздний stress test распределенного lifecycle и множества исполняемых компонентов.

Home Assistant и Airflow особенно ценны не как требование нарисовать полный граф, а как negative calibration: инструмент должен показать, где его evidence заканчивается, и не превращать динамику в выдуманную static truth.

## Что понадобится для полноценного Python onboarding позже

Точечного Pyright-анализа недостаточно для паритета с текущим Go survey. Отдельный Python facts provider должен будет локально и детерминированно учитывать:

- `pyproject.toml`, `setup.cfg`, `setup.py`;
- package roots и `src/` layout;
- import graph;
- `if __name__ == "__main__"`;
- `__main__.py`;
- `[project.scripts]` и другие console entrypoints;
- ASGI/WSGI/Celery declarations;
- `test_*.py`, `*_test.py`, `tests/`, pytest/unittest conventions;
- multi-environment/monorepo configuration и Pyright execution environments.

Это отдельная продуктовая задача. Ее не нужно прятать внутрь первого адаптера.

## Не делать в первом срезе

- не включать Pyright в default repo-wide scan;
- не отправлять Python source в LLM;
- не менять существующий gopls adapter без необходимости;
- не обещать полный call graph или runtime truth;
- не давать verdict, что человеку следует удалить при миграции;
- не строить сразу поддержку Poetry, uv, Django, FastAPI и монореп;
- не добавлять тяжелый универсальный plugin framework;
- не оптимизировать производительность до появления измеренного bottleneck;
- не писать дорогие snapshot/browser tests для экспериментального UI, которого еще нет.

## Грубая оценка сложности

- протокол Pyright playground: технически подтвержден; tracked implementation остается примерно **4/10**;
- подключение пути component → symbol → calls → source: примерно **6/10**, порядок недели с учетом обобщения Go-specific policy;
- Python system map с manifests/inheritance/registries: отдельные **6–7/10**, потому что одного LSP недостаточно;
- надежная поддержка разных Python layouts/frameworks: примерно **8/10**, это продолжающаяся калибровка, а не одноразовая задача.

Это ориентиры для выбора масштаба, не обещание сроков.

## Главный вопрос эксперимента

Не «можем ли мы запустить Pyright», а:

> Можем ли мы тем же language-neutral evidence contract честно описать точный Python-символ, его статически видимых соседей и границы неизвестного — так, чтобы следующие слои repomap не знали, был источником gopls или Pyright?

Если playground дает положительный ответ, Python действительно подключается как следующий кубик. Если нет — сначала нужно поправить границы портов, а не размазывать Python-specific условия по приложению.
