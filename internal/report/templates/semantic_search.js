(function (global) {
  "use strict";

  const MAX_RESULTS = 20;
  const STOP_WORDS = new Set([
    "a", "an", "and", "are", "as", "at", "be", "by", "does", "for", "from",
    "here", "how", "i", "in", "is", "it", "of", "on", "or", "the", "this",
    "to", "used", "uses", "using", "what", "where", "which", "with", "work", "works",
    "а", "в", "где", "для", "здесь", "и", "из", "как", "какие", "какой", "на",
    "по", "при", "с", "тут", "что", "эта", "это", "этот", "использовать", "используется",
  ]);
  const KIND_PRIORITY = Object.freeze({
    repository_story: 225,
			study_direction: 223,
		paved_path: 222,
    mechanism: 220,
    dependency_usage: 216,
    repository_pattern: 212,
    contribution_guide: 208,
    go_learning: 204,
    guided_tour: 190,
    guided_step: 180,
    direction: 165,
    map: 155,
    subsystem: 125,
    component: 145,
    member: 35,
    flow: 135,
    surface: 130,
    domain_term: 105,
    behavior_anchor: 100,
    unknown: 70,
    warning: 65,
    flow_step: 55,
    location: 25,
  });
  const KIND_LABELS = Object.freeze({
    repository_story: "Repository story",
			study_direction: "Study direction",
		paved_path: "How to run and verify",
    mechanism: "Mechanism",
    dependency_usage: "Dependency usage",
    repository_pattern: "Repository pattern",
    contribution_guide: "Contribution guide",
    go_learning: "Go in practice",
    guided_tour: "Story",
    guided_step: "Story step",
    direction: "Code path",
    map: "Map",
    subsystem: "Subsystem",
    component: "Component",
    member: "Package / symbol",
    flow: "Flow",
    surface: "Runtime entry",
    domain_term: "Domain term",
    behavior_anchor: "Behavior anchor",
    unknown: "Source note",
    warning: "Source note",
    flow_step: "Symbol / step",
    location: "Source",
  });
  const TOKEN_WEIGHT = Object.freeze({
    verify: 3.2,
    response: 1.6,
    cache: 2.2,
    build: 1.8,
    report: 1.6,
    surface: 1.6,
    analyzer: 1.4,
    extend: 1.4,
    model: 1.1,
    component: 1.1,
  });

  function array(value) {
    return Array.isArray(value) ? value : [];
  }

  function text(value) {
    return value == null ? "" : String(value).trim();
  }

  function fold(value) {
    value = text(value);
    if (!value) return "";
    if (typeof value.normalize === "function") value = value.normalize("NFKC");
    return value.toLocaleLowerCase().replaceAll("ё", "е").replace(/\s+/g, " ").trim();
  }

  function protectedTechnicalTerms(value) {
    return fold(value)
      .replace(/golang\.org\s*\/\s*x\s*\/\s*tools\s*\/\s*go\s*\/\s*packages/g, " go_packages ")
      .replace(/\bgo\s*\/\s*packages\b/g, " go_packages ")
      .replace(/\bpackages\s*\.\s*load\b/g, " packages_load ");
  }

  function canonicalToken(token) {
    token = fold(token).replace(/^_+|_+$/g, "");
    if (!token || STOP_WORDS.has(token)) return "";
    if (token === "go_packages" || token === "packages_load") return token;
    if (/^(отчет|отчёт|report|reporting|html)$/.test(token)) return "report";
    if (/^(стро|собир|генер|build|built|generate|generated|render|rendered|produce|produces|assemble)/.test(token)) return "build";
    if (/^(deepseek)$/.test(token)) return "deepseek";
    if (/^(llm|model|models|модел)/.test(token)) return "model";
    if (/^(ответ|response|responses|output|outputs|result|results)$/.test(token)) return "response";
    if (/^(провер|валид|verify|verified|validation|validate|validated|allowlist|normalize)/.test(token)) return "verify";
    if (/^(runtime|рантайм|surface|surfaces|trigger|triggers|registration|registrations|route|routes|worker|workers|async|discovery)$/.test(token)) return "surface";
    if (/^(кеш|кэш)/.test(token) || /^(cache|cached|caching|replay)$/.test(token)) return "cache";
    if (/^(компон|component|components|subsystem|subsystems)$/.test(token)) return "component";
    if (/^(главн|основн|main|primary)$/.test(token)) return "main";
    if (/^(архитект|architecture|landscape|map|карта)$/.test(token)) return "architecture";
    if (/^(анализ|analyzer|analyzers|analysis|engine)$/.test(token)) return "analyzer";
    if (/^(добав|расшир|нов)/.test(token) || /^(add|added|adding|extend|extension|new)$/.test(token)) return "extend";
    if (/^(пакет|package|packages)$/.test(token)) return "package";
    if (/^(поток|flow|flows|trace|traces)$/.test(token)) return "flow";
    if (/^(go|golang)$/.test(token)) return "go";
    return token;
  }

  function tokenize(value) {
    const prepared = protectedTechnicalTerms(value)
      .replace(/([a-z0-9])([A-Z])/g, "$1 $2")
      .replace(/[^\p{L}\p{N}_]+/gu, " ");
    const tokens = [];
    const seen = new Set();
    prepared.split(/\s+/).forEach((raw) => {
      const token = canonicalToken(raw);
      if (!token || seen.has(token)) return;
      seen.add(token);
      tokens.push(token);
    });
    return tokens;
  }

  function exactCandidate(value) {
    return protectedTechnicalTerms(value)
      .replace(/[^\p{L}\p{N}_./:-]+/gu, " ")
      .replace(/\s+/g, " ")
      .trim();
  }

  function searchableItem(item) {
    const title = text(item && item.title);
    const question = text(item && item.question);
    const summary = text(item && item.summary);
    const aliases = array(item && item.aliases).map(text).filter(Boolean);
    const titleTokens = new Set(tokenize(title));
    const questionTokens = new Set(tokenize(question));
    const aliasTokens = new Set(tokenize(aliases.join(" ")));
    const summaryTokens = new Set(tokenize(summary));
    const exactCandidates = [title, question].concat(aliases).map(exactCandidate).filter(Boolean);
    return { item: item, title: title, question: question, summary: summary, aliases: aliases,
      titleTokens: titleTokens, questionTokens: questionTokens, aliasTokens: aliasTokens, summaryTokens: summaryTokens,
      exactCandidates: exactCandidates };
  }

  function exactMatch(candidates, query) {
    const wanted = exactCandidate(query);
    if (!wanted) return false;
    return candidates.some((candidate) => {
      if (candidate === wanted) return true;
      const segments = candidate.split(/[\/.:#-]+/).filter(Boolean);
      return segments.length > 0 && segments[segments.length - 1] === wanted;
    });
  }

  function rankSemanticSearchItems(items, query, limit) {
    limit = Math.max(1, Number(limit) || MAX_RESULTS);
    const queryTokens = tokenize(query);
    if (queryTokens.length === 0) return [];
    const foldedQuery = exactCandidate(query);
    const ranked = [];

    array(items).forEach((item) => {
      if (!item || !item.id || !item.target) return;
      const searchable = searchableItem(item);
      const isExact = exactMatch(searchable.exactCandidates, query);
      let score = Number(KIND_PRIORITY[item.kind] || 0);
      let matched = 0;
      const matchedTokens = [];

      queryTokens.forEach((token) => {
        let fieldWeight = 0;
        if (searchable.titleTokens.has(token)) fieldWeight = 80;
        else if (searchable.questionTokens.has(token)) fieldWeight = 55;
        else if (searchable.aliasTokens.has(token)) fieldWeight = 38;
        else if (searchable.summaryTokens.has(token)) fieldWeight = 28;
        if (!fieldWeight) return;
        matched++;
        matchedTokens.push(token);
        score += fieldWeight * Number(TOKEN_WEIGHT[token] || 1);
      });
      if (matched === 0 && !isExact) return;

      const titlePhrase = exactCandidate(searchable.title).includes(foldedQuery);
      const questionPhrase = exactCandidate(searchable.question).includes(foldedQuery);
      const aliasPhrase = searchable.exactCandidates.some((candidate) => candidate.includes(foldedQuery));
      const summaryPhrase = exactCandidate(searchable.summary).includes(foldedQuery);
      if (isExact) score += 4000;
      else if (titlePhrase) score += 900;
      else if (questionPhrase) score += 780;
      else if (aliasPhrase) score += 720;
      else if (summaryPhrase) score += 360;

      const coverage = matched / queryTokens.length;
      score += Math.round(coverage * 260);
      if (coverage === 1) score += 420;
      else score -= Math.round((1 - coverage) * 180);
      if ((item.kind === "location" || item.kind === "flow_step") && !isExact) score -= 120;
      const queryNamesExample = /\b(example|playground|пример)\b/.test(fold(query));
      if (!queryNamesExample && /\b(example|playground)\b/.test(fold(item.title))) score -= 180;

      ranked.push({
        item: item,
        score: Math.round(score),
        exact: isExact,
        complete: coverage === 1,
        coverage: coverage,
        matched_tokens: matchedTokens,
        query_tokens: queryTokens,
      });
    });

    ranked.sort((left, right) => {
      if (left.complete !== right.complete) return left.complete ? -1 : 1;
      if (left.score !== right.score) return right.score - left.score;
      const leftKind = Number(KIND_PRIORITY[left.item.kind] || 0);
      const rightKind = Number(KIND_PRIORITY[right.item.kind] || 0);
      if (leftKind !== rightKind) return rightKind - leftKind;
      const titleOrder = fold(left.item.title).localeCompare(fold(right.item.title));
      if (titleOrder !== 0) return titleOrder;
      return text(left.item.id).localeCompare(text(right.item.id));
    });
    return ranked.slice(0, limit);
  }

  function nextSearchIndex(current, delta, count) {
    count = Math.max(0, Number(count) || 0);
    if (count === 0) return -1;
    current = Number.isFinite(Number(current)) ? Number(current) : 0;
    return (current + Number(delta || 0) + count) % count;
  }

  function resultGroup(result) {
    const kind = text(result && result.item && result.item.kind);
    if ([
				"repository_story", "study_direction", "paved_path", "mechanism", "dependency_usage", "repository_pattern",
      "contribution_guide", "go_learning", "guided_tour", "guided_step",
    ].includes(kind)) return "Ready explanations";
    if (result && result.exact) return "Exact names and references";
    if (result && !result.complete) return "Related results";
    if (kind === "location" || kind === "flow_step" || kind === "member") return "Source code";
    if (kind === "unknown" || kind === "warning") return "Source code";
    return "Repository concepts";
  }

  function createElement(tag, className, content) {
    const node = global.document.createElement(tag);
    if (className) node.className = className;
    if (content != null) node.textContent = String(content);
    return node;
  }

  class SemanticSearchView {
    constructor(root, index, options) {
      this.root = root;
      this.index = index || {};
      this.options = options || {};
      this.items = array(this.index.items).filter((item) => {
        return typeof this.options.targetAvailable !== "function" ||
          this.options.targetAvailable(item && item.target, item);
      });
      this.itemByID = new Map(this.items.map((item) => [text(item.id), item]));
      this.results = [];
      this.activeIndex = -1;
      this.restoreFocus = null;
      this.restoringFocus = false;
      this.abort = new AbortController();
    }

    listen(target, type, handler, options) {
      const settings = Object.assign({}, options || {}, { signal: this.abort.signal });
      target.addEventListener(type, handler, settings);
    }

    presentationTitle(item) {
      if (typeof this.options.presentationTitle === "function") {
        const projected = text(this.options.presentationTitle(item));
        if (projected) return projected;
      }
      return text(item && item.title);
    }

    start() {
      if (!this.root || this.items.length === 0) {
        if (this.root) this.root.hidden = true;
        return this;
      }
      this.entry = this.root.querySelector("[data-semantic-search-entry]");
      this.suggestions = this.root.querySelector("[data-semantic-search-suggestions]");
      this.renderSuggestions();
      this.buildModal();
      if (this.entry) {
        this.listen(this.entry, "focus", () => {
          if (!this.restoringFocus) this.open(this.entry.value);
        });
        this.listen(this.entry, "click", () => this.open(this.entry.value));
        this.listen(this.entry, "keydown", (event) => {
          if (event.key === "Enter" || event.key === "ArrowDown") {
            event.preventDefault();
            this.open(this.entry.value);
          }
        });
      }
      this.listen(global, "keydown", (event) => {
        if (event.isComposing || event.altKey) return;
        if ((event.metaKey || event.ctrlKey) && fold(event.key) === "k") {
          event.preventDefault();
          this.open("");
        }
      });
      return this;
    }

    renderSuggestions() {
      if (!this.suggestions) return;
      this.suggestions.replaceChildren();
      array(this.index.suggestions).slice(0, 8).forEach((suggestion) => {
        const itemID = typeof suggestion === "string" ? suggestion : suggestion && suggestion.item_id;
        const item = this.itemByID.get(text(itemID));
        if (!item) return;
        const question = typeof suggestion === "object" && suggestion.question
          ? suggestion.question : this.suggestionQuestion(item);
        const button = createElement("button", "rm-semantic-search__suggestion", question);
        button.type = "button";
        button.title = "Open: " + this.presentationTitle(item);
        button.dataset.semanticSearchItem = text(item.id);
        this.listen(button, "click", () => this.activateItem(item));
        this.suggestions.appendChild(button);
      });
      this.suggestions.hidden = this.suggestions.childElementCount === 0;
    }

    suggestionQuestion(item) {
      if (text(item && item.question)) return text(item.question);
      const title = this.presentationTitle(item);
      switch (text(item && item.kind)) {
		case "paved_path": return "How do I use “" + title + "”?";
      case "guided_tour": return "Where should I start with “" + title + "”?";
      case "flow": return "How does “" + title + "” work?";
      case "surface": return "Where does “" + title + "” begin?";
      case "map": return "What are the main components?";
      case "direction": return "How does “" + title + "” work?";
      case "domain_term": return "What does “" + title + "” mean here?";
      case "component": return "What is “" + title + "” responsible for?";
      case "subsystem": return "What belongs to “" + title + "”?";
      case "flow_step": return "Where is “" + title + "” used?";
      case "location": return "What does “" + title + "” show?";
      default: return "Open “" + title + "”?";
      }
    }

    buildModal() {
      this.modal = createElement("div", "rm-search-modal");
      this.modal.hidden = true;
      this.backdrop = createElement("button", "rm-search-modal__backdrop");
      this.backdrop.type = "button";
      this.backdrop.tabIndex = -1;
      this.backdrop.setAttribute("aria-label", "Close search");
      this.dialog = createElement("section", "rm-search-modal__dialog");
      this.dialog.setAttribute("role", "dialog");
      this.dialog.setAttribute("aria-modal", "true");
      this.dialog.setAttribute("aria-labelledby", "rm-semantic-search-title");
      const header = createElement("header", "rm-search-modal__header");
      const heading = createElement("div");
      const title = createElement("h2", null, "What do you want to understand?");
      title.id = "rm-semantic-search-title";
      heading.appendChild(title);
      heading.appendChild(createElement("p", null, "Search saved explanations, the architecture map, and source references."));
      this.closeButton = createElement("button", "rm-search-modal__close", "Esc");
      this.closeButton.type = "button";
      this.closeButton.setAttribute("aria-label", "Close search");
      header.append(heading, this.closeButton);

      this.input = createElement("input", "rm-search-modal__input");
      this.input.type = "search";
      this.input.placeholder = "For example: request dispatch, report building, runtime surfaces";
      this.input.setAttribute("role", "combobox");
      this.input.setAttribute("aria-autocomplete", "list");
      this.input.setAttribute("aria-controls", "rm-semantic-search-results");
      this.input.setAttribute("aria-expanded", "true");
      this.status = createElement("p", "rm-search-modal__status");
      this.status.setAttribute("aria-live", "polite");
      this.resultHost = createElement("div", "rm-search-modal__results");
      this.resultHost.id = "rm-semantic-search-results";
      this.resultHost.tabIndex = -1;
      this.resultHost.setAttribute("role", "listbox");
      const footer = createElement("footer", "rm-search-modal__footer");
      footer.append(
        createElement("span", null, "↑↓ select"),
        createElement("span", null, "Enter open"),
        createElement("span", null, "Esc close")
      );
      this.dialog.append(header, this.input, this.status, this.resultHost, footer);
      this.modal.append(this.backdrop, this.dialog);
      global.document.body.appendChild(this.modal);

      this.listen(this.backdrop, "click", () => this.close(true));
      this.listen(this.closeButton, "click", () => this.close(true));
      this.listen(this.input, "input", () => {
        if (this.entry) this.entry.value = this.input.value;
        this.renderResults(this.input.value);
      });
      this.listen(this.input, "keydown", (event) => this.handleInputKey(event));
      this.listen(this.modal, "keydown", (event) => {
        if (event.key === "Escape") {
          event.preventDefault();
          event.stopPropagation();
          this.close(true);
        } else if (event.key === "Tab") {
          event.preventDefault();
          (global.document.activeElement === this.input ? this.closeButton : this.input).focus();
        }
      });
    }

    open(query) {
      if (!this.modal || this.items.length === 0) return;
      if (this.modal.hidden) this.restoreFocus = global.document.activeElement;
      this.modal.hidden = false;
      global.document.body.classList.add("rm-search-open");
      this.input.value = text(query);
      if (this.entry) this.entry.value = this.input.value;
      this.renderResults(this.input.value);
      global.requestAnimationFrame(() => {
        this.input.focus();
        this.input.select();
      });
    }

    close(restore) {
      if (!this.modal || this.modal.hidden) return;
      this.modal.hidden = true;
      global.document.body.classList.remove("rm-search-open");
      this.input.removeAttribute("aria-activedescendant");
      if (restore && this.restoreFocus && typeof this.restoreFocus.focus === "function") {
        this.restoringFocus = true;
        this.restoreFocus.focus();
        this.restoringFocus = false;
      }
      this.restoreFocus = null;
    }

    initialResults() {
      const seen = new Set();
      const results = [];
      array(this.index.suggestions).forEach((suggestion) => {
        const itemID = typeof suggestion === "string" ? suggestion : suggestion && suggestion.item_id;
        const item = this.itemByID.get(text(itemID));
        if (!item || seen.has(item.id)) return;
        seen.add(item.id);
        results.push({ item: item, score: 0, exact: false, complete: true, coverage: 1 });
      });
      return results.slice(0, 8);
    }

    renderResults(query) {
      this.results = text(query) ? rankSemanticSearchItems(this.items, query, MAX_RESULTS) : this.initialResults();
      this.activeIndex = this.results.length > 0 ? 0 : -1;
      this.resultHost.replaceChildren();
      if (this.results.length === 0) {
        this.input.removeAttribute("aria-activedescendant");
        this.status.textContent = "No matches.";
        const empty = createElement("div", "rm-search-modal__empty");
        empty.appendChild(createElement("strong", null, "No matches"));
        empty.appendChild(createElement(
          "p", null,
          "Try an exact component, flow, runtime entry, package, symbol, or file name."
        ));
        this.resultHost.appendChild(empty);
        return;
      }
      this.status.textContent = text(query)
        ? "Found: " + this.results.length
        : "Suggested starting points";
      let currentGroup = "";
      this.results.forEach((result, index) => {
        const group = resultGroup(result);
        if (group !== currentGroup) {
          currentGroup = group;
          this.resultHost.appendChild(createElement("h3", "rm-search-modal__group", group));
        }
        const button = createElement("button", "rm-search-modal__result");
        button.type = "button";
        button.id = "rm-semantic-search-option-" + index;
        button.setAttribute("role", "option");
        button.setAttribute("aria-selected", index === this.activeIndex ? "true" : "false");
        button.tabIndex = -1;
        const top = createElement("span", "rm-search-modal__result-top");
        top.appendChild(createElement("strong", null, this.presentationTitle(result.item)));
        top.appendChild(createElement("span", "rm-search-modal__kind", KIND_LABELS[result.item.kind] || result.item.kind));
        button.appendChild(top);
        if (result.item.question && result.item.question !== result.item.title) {
          button.appendChild(createElement("span", "rm-search-modal__summary", result.item.question));
        }
        if (result.item.summary) button.appendChild(createElement("span", "rm-search-modal__summary", result.item.summary));
        const action = createElement("span", "rm-search-modal__action", this.actionLabel(result.item.target));
        button.appendChild(action);
        this.listen(button, "click", () => this.activateIndex(index));
        this.resultHost.appendChild(button);
      });
      this.syncActiveResult(false);
    }

    actionLabel(target) {
      switch (text(target && target.kind)) {
      case "semantic_artifact": return "Open explanation →";
			case "study_direction": return "Open reading path →";
		case "paved_path": return "Open instructions →";
      case "guided_step": return "Go to step →";
      case "component": return "Show component on map →";
      case "flow": return "Open flow →";
      case "flow_step": return "Open exact step →";
      case "surface": return "Open runtime entry →";
      case "location": return "Open source →";
      default: return "Open full map →";
      }
    }

    handleInputKey(event) {
      if (event.isComposing) return;
      if (event.key === "ArrowDown") {
        event.preventDefault();
        this.activeIndex = nextSearchIndex(this.activeIndex, 1, this.results.length);
        this.syncActiveResult(true);
      } else if (event.key === "ArrowUp") {
        event.preventDefault();
        this.activeIndex = nextSearchIndex(this.activeIndex, -1, this.results.length);
        this.syncActiveResult(true);
      } else if (event.key === "Home" && this.results.length > 0) {
        event.preventDefault();
        this.activeIndex = 0;
        this.syncActiveResult(true);
      } else if (event.key === "End" && this.results.length > 0) {
        event.preventDefault();
        this.activeIndex = this.results.length - 1;
        this.syncActiveResult(true);
      } else if (event.key === "Enter" && this.activeIndex >= 0) {
        event.preventDefault();
        this.activateIndex(this.activeIndex);
      }
    }

    resultButtons() {
      return Array.from(this.resultHost.querySelectorAll(".rm-search-modal__result"));
    }

    syncActiveResult(scroll) {
      const buttons = this.resultButtons();
      buttons.forEach((button, index) => {
        const active = index === this.activeIndex;
        button.classList.toggle("is-active", active);
        button.setAttribute("aria-selected", active ? "true" : "false");
      });
      const active = buttons[this.activeIndex];
      if (!active) {
        this.input.removeAttribute("aria-activedescendant");
        return;
      }
      this.input.setAttribute("aria-activedescendant", active.id);
      if (scroll && typeof active.scrollIntoView === "function") active.scrollIntoView({ block: "nearest" });
    }

    activateIndex(index) {
      const result = this.results[index];
      if (result) this.activateItem(result.item);
    }

    activateItem(item) {
      if (!item || !item.target) return;
      this.close(false);
      if (typeof this.options.openTarget === "function") this.options.openTarget(item.target, item);
    }

    destroy() {
      this.abort.abort();
      if (this.modal) this.modal.remove();
    }
  }

  function mount(root, index, options) {
    return new SemanticSearchView(root, index, options).start();
  }

  global.RepomapSemanticSearch = Object.freeze({ mount: mount });
  if (global.__REPOMAP_SEARCH_TEST__ && typeof global.__REPOMAP_SEARCH_TEST__ === "object") {
    Object.assign(global.__REPOMAP_SEARCH_TEST__, {
      canonicalToken: canonicalToken,
      tokenize: tokenize,
      rankSemanticSearchItems: rankSemanticSearchItems,
      nextSearchIndex: nextSearchIndex,
      resultGroup: resultGroup,
    });
  }
})(window);
