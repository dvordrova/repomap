(function () {
  'use strict';

  var DATA = JSON.parse(document.getElementById('rm-report-data').textContent);
  var REPORT_LANGUAGE = DATA.report_language === 'ru' ? 'ru' : 'en';
  // Archive 12 P0 (review): the default Study shelf is bounded and
  // core-first (cards already arrive in portfolio-rank order); the
  // complete shelf is one "Show all" press away, never truncated.
  var UI = window.RepomapUI;
  if (!UI || typeof UI.message !== 'function' || typeof UI.hasMessage !== 'function') {
    throw new Error('repomap UI message catalog is unavailable');
  }

  function msg(id, params) {
    return UI.message(id, params);
  }

  function bindFixedMessages(root) {
    var scope = root || document;
    Array.prototype.forEach.call(scope.querySelectorAll('[data-rm-message]'), function (node) {
      node.textContent = msg(node.getAttribute('data-rm-message'));
    });
    Array.prototype.forEach.call(scope.querySelectorAll('[data-rm-aria-message]'), function (node) {
      node.setAttribute('aria-label', msg(node.getAttribute('data-rm-aria-message')));
    });
  }

  var OPENABLE_PATHS = (DATA.openable_paths || []).slice().sort(function (a, b) {
    return b.length - a.length;
  });
  var OPENABLE_PATH_SET = {};
  OPENABLE_PATHS.forEach(function (path) { OPENABLE_PATH_SET[path] = true; });
	var SOURCE_IDS = DATA.source_ids || {};
	var SOURCE_CONTEXT_IDS = DATA.source_context_ids || {};
  var STATIC_SOURCE_HOST = DATA.github_source_links ? 'GitHub' : 'GitLab';
  var STATIC_SOURCE_LINKS = DATA.github_source_links || DATA.gitlab_source_links || null;
  var STATIC_WORKING_TREE_PATH_SET = {};
  ((STATIC_SOURCE_LINKS && STATIC_SOURCE_LINKS.working_tree_paths) || []).forEach(function (path) {
    STATIC_WORKING_TREE_PATH_SET[path] = true;
  });
  var toastTimer = null;
  var architectureCanvasView = null;
  var surfaceCatalogView = null;
  var DEBUG_MODE = /^(1|true)$/i.test(new URLSearchParams(window.location.search).get('debug') || '');
	var USER_SOURCES = Array.isArray(DATA.user_sources) ? DATA.user_sources : [];
	var ATLAS_FIRST = !!DATA.repository_atlas;
	var REPOSITORY_GUIDE = DATA.repository_guide || null;
	var STUDY_MAP = DATA.study_map || null;
	var ARCHITECTURE_COMPONENT_NAVIGATION = DATA.architecture_component_navigation || null;
	var STUDY_PUBLICATION = DATA.study_publication || null;
	var COMPLETE_STUDY_DIRECTIONS = STUDY_MAP && Array.isArray(STUDY_MAP.directions) ? STUDY_MAP.directions : [];
	var INCOMPLETE_STUDY = DATA.incomplete_study || null;
	var INCOMPLETE_STUDY_DIRECTIONS = INCOMPLETE_STUDY && Array.isArray(INCOMPLETE_STUDY.directions)
		? INCOMPLETE_STUDY.directions.map(function (direction) {
			return Object.assign({}, direction, { incomplete: true });
		})
		: [];
	var STUDY_DIRECTIONS = COMPLETE_STUDY_DIRECTIONS.concat(INCOMPLETE_STUDY_DIRECTIONS);
// repomap-source-episode:start
	var SOURCE_EPISODE = DATA.source_episode || null;
// repomap-source-episode:end
	var OPERATIONS = DATA.operations || null;
	var PAVED_PATHS = OPERATIONS && Array.isArray(OPERATIONS.paths) ? OPERATIONS.paths : [];
	var OPERATIONAL_LANDMARKS = OPERATIONS && Array.isArray(OPERATIONS.landmarks) ? OPERATIONS.landmarks : [];
	var TASK_INVESTIGATION = DATA.task_investigation || null;
	var ANALYSIS_TARGET = DATA.analysis_target || null;
	var TARGET_NAVIGATION = DATA.target_navigation || null;
  var workspaceState = {
		// Decision 236 (v11): Map is the default view for ordinary
		// workspaces.
		view: defaultWorkspaceView(),
		taskID: TASK_INVESTIGATION && TASK_INVESTIGATION.task_id || '',
		directionID: '',
		themeCardOrdinal: 0,
		operationID: '',
    sourceLocation: null,
    mapReturn: null,
    mapTarget: null,
  };
  var architectureCanvasHost = null;
  var mapEmptyInspectorHost = null;
  var mapPrimaryLayoutHost = null;
  var architectureReady = null;
  var architectureAppliedFocus = null;

	function studyDirectionByID(directionID) {
		for (var index = 0; index < STUDY_DIRECTIONS.length; index++) {
			if (STUDY_DIRECTIONS[index] && STUDY_DIRECTIONS[index].id === directionID) return STUDY_DIRECTIONS[index];
		}
		return null;
	}

	// D213: the source-grounded theme shelf. Cards are locally reduced from
	// the two semantic stages; readings carry exact source locations only.
	function themeCards() {
		var study = DATA.atlas_study;
		var themes = study && study.themes;
		return themes && Array.isArray(themes.cards) ? themes.cards : [];
	}

	function themeCardByOrdinal(ordinal) {
		var cards = themeCards();
		for (var index = 0; index < cards.length; index++) {
			if (cards[index] && Number(cards[index].ordinal) === Number(ordinal)) return cards[index];
		}
		return null;
	}

	function currentStudyInvestigation(card) {
		var investigation = card && card.investigation;
		if (!investigation || Number(investigation.version) !== 1 || !investigation.id ||
			['mechanism', 'prepared_investigation'].indexOf(String(investigation.outcome || '')) < 0 ||
			!Array.isArray(investigation.mechanisms)) return null;
		return investigation;
	}

	// Report42 publishes the exact component participation of every persisted
	// Study mechanism node. This is the sole current Study -> component join:
	// no path, symbol, package, component-name, or legacy Study-area matching.
	function componentStudyThemes(componentID) {
		componentID = String(componentID || '');
		if (!componentID) return [];
		var matches = [];
		themeCards().forEach(function (card) {
			var investigation = currentStudyInvestigation(card);
			if (!investigation || investigation.outcome !== 'mechanism') return;
			var exactParticipant = investigation.mechanisms.some(function (mechanism) {
				return (Array.isArray(mechanism && mechanism.nodes) ? mechanism.nodes : []).some(function (node) {
					return (Array.isArray(node && node.component_ids) ? node.component_ids : []).some(function (candidate) {
						return String(candidate || '') === componentID;
					});
				});
			});
			if (!exactParticipant) return;
			matches.push({
				route_kind: 'theme',
				ordinal: Number(card.ordinal) || 0,
				title: String(card.final_title || ''),
				question: String(card.final_question || card.final_title || ''),
				why_it_matters: String(card.why_it_matters || ''),
			});
		});
		return matches;
	}

	function studyMechanismForTarget(target) {
		if (!target || String(target.kind || '') !== 'study_mechanism') return null;
		var card = themeCardByOrdinal(Number(target.theme_ordinal));
		var investigation = currentStudyInvestigation(card);
		if (!card || !investigation || String(investigation.id) !== String(target.investigation_id || '')) return null;
		for (var index = 0; index < investigation.mechanisms.length; index++) {
			var mechanism = investigation.mechanisms[index];
			if (mechanism && String(mechanism.id || '') === String(target.mechanism_id || '')) {
				return { card: card, investigation: investigation, mechanism: mechanism };
			}
		}
		return null;
	}

	function pavedPathByID(pavedPathID) {
		for (var index = 0; index < PAVED_PATHS.length; index++) {
			if (PAVED_PATHS[index] && PAVED_PATHS[index].id === pavedPathID) return PAVED_PATHS[index];
		}
		return null;
	}

  function emptyWorkspaceState() {
    return {
			view: defaultWorkspaceView(),
			taskID: TASK_INVESTIGATION && TASK_INVESTIGATION.task_id || '',
			directionID: '',
			themeCardOrdinal: 0,
			operationID: '',
      sourceLocation: null,
      mapReturn: null,
      mapTarget: null,
    };
  }

  // Decision 236 (v11): Map is the primary product — the default view is
  // 'map' for every ordinary (non-task-investigation) workspace.
  function defaultWorkspaceView() {
    return TASK_INVESTIGATION ? 'investigate' : 'map';
  }

	function defaultWorkspaceHash() {
	if (TASK_INVESTIGATION && TASK_INVESTIGATION.task_id) {
	return '#/investigate/' + encodeRoutePart(TASK_INVESTIGATION.task_id);
	}
	return '#canvas';
	}

  function encodeRoutePart(value) {
    return encodeURIComponent(String(value == null ? '' : value));
  }

  function decodeRoutePart(value) {
    try {
      return decodeURIComponent(String(value || ''));
    } catch (_) {
      return '';
    }
  }

  function architectureFocusValue(target) {
    target = target || {};
    var kind = String(target && target.kind || '');
    if (kind === 'component' && target.component_id) {
      return 'component:' + encodeRoutePart(target.component_id);
    }
    if (kind === 'flow' && target.flow_id) {
      return 'flow:' + encodeRoutePart(target.flow_id);
    }
    if (kind === 'surface' && target.surface_id) {
      return 'surface:' + encodeRoutePart(target.surface_id);
    }
    if (kind === 'flow_step' && target.flow_id && target.step_id) {
      return 'flow_step:' + encodeRoutePart(target.flow_id) + ':' + encodeRoutePart(target.step_id);
    }
    return '';
  }

  function architectureTargetFromFocus(value) {
    var parts = String(value || '').split(':');
    var kind = parts.shift() || '';
    if (kind === 'component' && parts.length === 1) {
      var componentID = decodeRoutePart(parts[0]);
      return componentID ? { kind: kind, component_id: componentID } : null;
    }
    if (kind === 'flow' && parts.length === 1) {
      var flowID = decodeRoutePart(parts[0]);
      return flowID ? { kind: kind, flow_id: flowID } : null;
    }
    if (kind === 'surface' && parts.length === 1) {
      var surfaceID = decodeRoutePart(parts[0]);
      return surfaceID ? { kind: kind, surface_id: surfaceID } : null;
    }
    if (kind === 'flow_step' && parts.length === 2) {
      var stepFlowID = decodeRoutePart(parts[0]);
      var stepID = decodeRoutePart(parts[1]);
      return stepFlowID && stepID ? { kind: kind, flow_id: stepFlowID, step_id: stepID } : null;
    }
    return null;
  }

  function workspaceHashForState(state) {
    state = state || emptyWorkspaceState();
		if (state.view === 'investigate' && TASK_INVESTIGATION && TASK_INVESTIGATION.task_id) {
			return defaultWorkspaceHash();
		}
	if (state.themeCardOrdinal && themeCardByOrdinal(state.themeCardOrdinal)) {
		return '#study-theme-' + Number(state.themeCardOrdinal);
	}
	if (state.view === 'study_overview' && (STUDY_DIRECTIONS.length || themeCards().length)) return '#study';
    if (state.view === 'architecture') {
      var focus = architectureFocusValue(state.mapTarget);
      return '#canvas' + (focus ? '?focus=' + encodeURIComponent(focus) : '');
    }
		// Decision 236 (v11): map is the primary route — a map view (with
		// an optional focus target) hashes to #/map.
    if (state.view === 'map') {
      var mapFocus = architectureFocusValue(state.mapTarget);
      return '#canvas' + (mapFocus ? '?focus=' + encodeURIComponent(mapFocus) : '');
    }
		if (state.view === 'study' && state.directionID) {
			return '#/study/' + encodeRoutePart(state.directionID);
		}
		if (state.view === 'operate' && state.operationID) {
			return '#/operate/' + encodeRoutePart(state.operationID);
		}
    if (state.view === 'provenance' && DEBUG_MODE) return '#/provenance';
		// Decision 236 (v11): the overview view is legacy — its information
		// lives in the empty-selection map inspector, so its hash
		// canonicalizes to the map.
    if (state.view === 'overview') return defaultWorkspaceHash();
    return '#canvas';
  }

	function workspaceRouteFamily(state) {
		state = state || emptyWorkspaceState();
		if (state.view === 'investigate' && state.taskID) {
			return 'investigate:' + state.taskID;
		}
		if (state.view === 'study' && state.directionID) {
			return 'study:' + state.directionID;
		}
		if (state.view === 'study' && state.themeCardOrdinal) {
			return 'study:theme:' + state.themeCardOrdinal;
		}
		if (state.view === 'study_overview') return 'view:study_overview';
		if (state.view === 'operate' && state.operationID) {
			return 'operate:' + state.operationID;
		}
		// Decision 236 (v11): map is the primary route; architecture is
		// an alias to the map with the Landscape lens.
		if (state.view === 'architecture') return 'view:map';
		return 'view:' + (state.view || defaultWorkspaceView());
	}

	function resetWorkspaceScroll() {
		if (typeof window.scrollTo === 'function') window.scrollTo(0, 0);
	}

  function parseWorkspaceHash(hash, historyState) {
    hash = String(hash || '');
    var state = emptyWorkspaceState();
		var route = hash.replace(/^#/, '') || defaultWorkspaceHash().replace(/^#/, '');
    var queryIndex = route.indexOf('?');
    var query = queryIndex >= 0 ? route.slice(queryIndex + 1) : '';
    var path = queryIndex >= 0 ? route.slice(0, queryIndex) : route;
    var segments = path.replace(/^\/+|\/+$/g, '').split('/').filter(Boolean);
    var valid = true;
		var canonicalHash = defaultWorkspaceHash();

    if (segments.length === 1 && /^study-theme-\d+$/.test(segments[0])) {
			var inlineThemeOrdinal = Number(segments[0].slice('study-theme-'.length));
			if (!themeCardByOrdinal(inlineThemeOrdinal)) {
				valid = false;
			} else {
				state.view = 'map';
				state.themeCardOrdinal = inlineThemeOrdinal;
				canonicalHash = '#study-theme-' + inlineThemeOrdinal;
			}
		} else if (segments.length === 1 && segments[0] === 'canvas') {
			state.view = 'map';
			var canvasFocus = new URLSearchParams(query).get('focus') || '';
			state.mapTarget = architectureTargetFromFocus(canvasFocus);
			if (!state.mapTarget && historyState && studyMechanismForTarget(historyState.mapTarget)) {
				state.mapTarget = historyState.mapTarget;
			}
			canonicalHash = workspaceHashForState(state);
		} else if (segments.length === 1 && segments[0] === 'overview') {
			// Decision 236 (v11): Overview is no longer a competing
			// destination — its information lives in the empty-selection
			// Map inspector. The legacy route canonicalizes to the map.
			if (TASK_INVESTIGATION) {
				state = emptyWorkspaceState();
				valid = false;
			} else {
				state.view = 'map';
				canonicalHash = defaultWorkspaceHash();
			}
		} else if (segments.length === 1 && segments[0] === 'map') {
			state.view = 'map';
			var focus = new URLSearchParams(query).get('focus') || '';
			if (focus) {
				state.mapTarget = architectureTargetFromFocus(focus);
			} else if (historyState && studyMechanismForTarget(historyState.mapTarget)) {
				state.mapTarget = historyState.mapTarget;
			}
			state.mapReturn = historyState && historyState.mapReturn || null;
			canonicalHash = workspaceHashForState(state);
		} else if (segments.length === 2 && segments[0] === 'investigate') {
			var routeTaskID = decodeRoutePart(segments[1]);
			if (!TASK_INVESTIGATION || routeTaskID !== TASK_INVESTIGATION.task_id) {
				valid = false;
			} else {
				state.view = 'investigate';
				state.taskID = routeTaskID;
				canonicalHash = defaultWorkspaceHash();
			}
		} else if (segments.length === 1 && segments[0] === 'architecture') {
			// Decision 236 (v11): the Architecture route aliases to the map
			// with the Landscape lens (the map is the primary product).
      state.view = 'map';
      var focus = new URLSearchParams(query).get('focus') || '';
      state.mapTarget = architectureTargetFromFocus(focus);
      if (!state.mapTarget && historyState && studyMechanismForTarget(historyState.mapTarget)) {
        state.mapTarget = historyState.mapTarget;
      }
      state.mapReturn = historyState && historyState.mapReturn || null;
      canonicalHash = workspaceHashForState(state);
		} else if (segments.length === 1 && segments[0] === 'study') {
			state.view = 'map';
			valid = !!(STUDY_DIRECTIONS.length || themeCards().length);
			canonicalHash = valid ? '#study' : defaultWorkspaceHash();
    } else if (segments.length === 1 && segments[0] === 'provenance' && DEBUG_MODE) {
      state.view = 'provenance';
      canonicalHash = '#/provenance';
		} else if (segments.length === 2 && segments[0] === 'study') {
			var routeDirectionID = decodeRoutePart(segments[1]);
			var routeDirection = studyDirectionByID(routeDirectionID);
			if (!routeDirection) {
				valid = false;
			} else {
				state.view = 'map';
				canonicalHash = '#study';
			}
		} else if (segments.length === 3 && segments[0] === 'study' && segments[1] === 'theme') {
			var routeOrdinal = Number(segments[2]);
			if (!themeCardByOrdinal(routeOrdinal)) {
				valid = false;
			} else {
				state.view = 'map';
				state.themeCardOrdinal = routeOrdinal;
				state.directionID = '';
				canonicalHash = workspaceHashForState(state);
			}
		} else if (segments.length === 2 && segments[0] === 'operate') {
			var routeOperationID = decodeRoutePart(segments[1]);
			if (!pavedPathByID(routeOperationID)) {
				valid = false;
			} else {
				state.view = 'operate';
				state.operationID = routeOperationID;
				canonicalHash = workspaceHashForState(state);
			}
    } else {
      valid = false;
    }

		if (!valid) state = emptyWorkspaceState();
		return { valid: valid, canonicalHash: valid ? canonicalHash : defaultWorkspaceHash(), state: state };
  }

  function reduceWorkspaceState(state, action) {
    state = state || workspaceState;
    action = action || {};
    var next = {
		view: state.view || defaultWorkspaceView(),
			taskID: state.taskID || '',
		directionID: state.directionID || '',
		themeCardOrdinal: Number(state.themeCardOrdinal) || 0,
		operationID: state.operationID || '',
      sourceLocation: state.sourceLocation || null,
      mapReturn: state.mapReturn || null,
      mapTarget: state.mapTarget || null,
    };
    switch (action.type) {
    case 'view':
			// Decision 236 (v11): 'architecture' is an alias of the map
			// with the Landscape lens; the canonical view is 'map'.
      next.view = action.view === 'architecture' || action.view === 'overview'
				? 'map'
				: (action.view || defaultWorkspaceView());
      if (next.view !== 'map' && next.view !== 'architecture') {
        next.mapTarget = null;
        if (!action.keepReturn) next.mapReturn = null;
      }
      return next;
		case 'open_study':
			var direction = studyDirectionByID(action.directionID);
			if (!direction) return next;
			next.view = 'study';
			next.directionID = direction.id;
			next.themeCardOrdinal = 0;
			next.operationID = '';
			next.mapTarget = null;
			next.mapReturn = null;
			return next;
		case 'open_study_theme':
			if (!themeCardByOrdinal(action.ordinal)) return next;
			next.view = 'study';
			next.directionID = '';
			next.themeCardOrdinal = Number(action.ordinal) || 0;
			next.operationID = '';
			next.mapTarget = null;
			next.mapReturn = null;
			return next;
		case 'open_operation':
			var pavedPath = pavedPathByID(action.operationID);
			if (!pavedPath) return next;
			next.view = 'operate';
			next.operationID = pavedPath.id;
			next.directionID = '';
			next.mapTarget = null;
			next.mapReturn = null;
			return next;
	case 'open_source':
		if (!action.selection || !action.selection.snippet ||
			!Array.isArray(action.selection.snippet.lines) || !action.selection.snippet.lines.length) return next;
		next.sourceLocation = action.selection;
      return next;
    case 'close_source':
      next.sourceLocation = null;
      return next;
    case 'show_map':
      if (!action.target) return next;
		if (next.themeCardOrdinal && studyMechanismForTarget(action.target)) {
			next.mapReturn = { themeCardOrdinal: next.themeCardOrdinal };
		} else {
			next.mapReturn = null;
		}
      next.mapTarget = action.target;
			// Decision 236 (v11): the map is the primary product.
      next.view = 'map';
      return next;
    case 'return_from_map':
      if (!next.mapReturn) return next;
		if (next.mapReturn.themeCardOrdinal) {
			var returnTheme = themeCardByOrdinal(next.mapReturn.themeCardOrdinal);
			if (!returnTheme) return next;
			next.view = 'study';
			next.themeCardOrdinal = Number(returnTheme.ordinal) || Number(next.mapReturn.themeCardOrdinal);
			next.directionID = '';
			next.mapTarget = null;
			next.mapReturn = null;
			return next;
		}
		if (next.mapReturn.directionID) {
			var returnDirection = studyDirectionByID(next.mapReturn.directionID);
			if (!returnDirection) return next;
			next.view = 'study';
			next.directionID = returnDirection.id;
			next.mapTarget = null;
			next.mapReturn = null;
			return next;
		}
		next.mapReturn = null;
		return next;
    default:
      return next;
    }
  }

  // ── DOM builders ───────────────────────────────────────────────

  function el(tag, cls, attrs) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (attrs) Object.keys(attrs).forEach(function (k) { e.setAttribute(k, attrs[k]); });
    return e;
  }

  function txt(tag, cls, text) {
    // Archive 12 P0 (live casdoor run): an empty tag name creates a bare
    // text node — document.createElement('') throws InvalidCharacterError
    // and would abort the whole Study shelf render.
    if (!tag) {
      return document.createTextNode(text == null ? '' : String(text));
    }
    var e = el(tag, cls);
    var value = text == null ? '' : String(text);
    e.textContent = value;
    return e;
  }

  function serverMode() {
    if (staticSourceMode()) return false;
    var loopback = window.location.hostname === '127.0.0.1' ||
      window.location.hostname === 'localhost' ||
      window.location.hostname === '::1';
    return loopback && window.location.protocol === 'http:' && serverBasePath() !== '';
  }

  function staticSourceMode() {
    return !!(
      STATIC_SOURCE_LINKS &&
      /^https?:\/\/[^/]+\/.+/i.test(String(STATIC_SOURCE_LINKS.repository_url || '')) &&
      /^[0-9a-f]{40}(?:[0-9a-f]{24})?$/i.test(String(STATIC_SOURCE_LINKS.revision || ''))
    );
  }

  function staticSourceOpenLabel() {
    return msg('main.source.open_in_host', { host: STATIC_SOURCE_HOST });
  }

  function staticSourceCapturedRevisionTitle() {
    return msg('main.source.open_captured_revision', { host: STATIC_SOURCE_HOST });
  }

  function staticSourceDirtyMessage() {
    return msg('main.source.dirty_mismatch', { host: STATIC_SOURCE_HOST });
  }

  function staticSourceHintMessage() {
    if (STATIC_SOURCE_LINKS && STATIC_SOURCE_LINKS.working_tree_dirty) {
      return msg('main.source.stable_local_changes', { host: STATIC_SOURCE_HOST });
    }
    return msg('main.source.captured_revision_hint', { host: STATIC_SOURCE_HOST });
  }

  function encodeStaticSourcePathSegment(value) {
    return encodeURIComponent(value).replace(/[!'()*]/g, function (character) {
      return '%' + character.charCodeAt(0).toString(16).toUpperCase();
    });
  }

  function staticWorkingTreePath(filePath) {
    return !!(filePath && STATIC_WORKING_TREE_PATH_SET[filePath]);
  }

  function staticSourceLocationAvailable(filePath, line, endLine) {
    return !!(
      staticSourceURL(filePath, line, endLine) ||
      staticSourceMode() && OPENABLE_PATH_SET[filePath] && staticWorkingTreePath(filePath)
    );
  }

  function staticSourceURL(filePath, line, endLine) {
    if (!staticSourceMode() || !filePath || !OPENABLE_PATH_SET[filePath] || staticWorkingTreePath(filePath)) return '';
    var segments = [];
    var prefix = String(STATIC_SOURCE_LINKS.path_prefix || '').replace(/^\/+|\/+$/g, '');
    if (prefix) segments = segments.concat(prefix.split('/'));
    segments = segments.concat(String(filePath).split('/'));
    if (segments.some(function (segment) { return !segment || segment === '.' || segment === '..'; })) return '';
    var blobPrefix = STATIC_SOURCE_HOST === 'GitHub' ? '/blob/' : '/-/blob/';
    var url = String(STATIC_SOURCE_LINKS.repository_url).replace(/\/+$/g, '') +
      blobPrefix + encodeStaticSourcePathSegment(String(STATIC_SOURCE_LINKS.revision)) + '/' +
      segments.map(encodeStaticSourcePathSegment).join('/');
    var start = Math.floor(Number(line) || 0);
    var end = Math.floor(Number(endLine) || 0);
    if (start > 0) {
      var rangeSeparator = STATIC_SOURCE_HOST === 'GitHub' ? '-L' : '-';
      url += '#L' + start + (end > start ? rangeSeparator + end : '');
    }
    return url;
  }

  function staticSourceLink(label, cls, location, endLine) {
    var link = txt('a', cls || '', label);
    return configureStaticSourceLink(link, location || {}, endLine);
  }

  function configureStaticSourceLink(link, location, endLine) {
    if (!link) return null;
    var url = staticSourceURL(location && location.path, location && location.line, endLine);
    if (!url) return null;
    link.setAttribute('href', url);
    link.setAttribute('target', '_blank');
    link.setAttribute('rel', 'noopener noreferrer');
    link.setAttribute('title', staticSourceCapturedRevisionTitle());
    return link;
  }

  function openStaticSource(location, endLine) {
    if (staticSourceMode() && staticWorkingTreePath(location && location.path)) {
      showToast(staticSourceDirtyMessage(), true);
      return true;
    }
    var url = staticSourceURL(location && location.path, location && location.line, endLine);
    if (!url) return false;
    window.open(url, '_blank', 'noopener,noreferrer');
    return true;
  }

  function sourceActionElement(label, cls, location, endLine, action) {
    var link = staticSourceLink(label, cls, location, endLine);
    if (link) return link;
    if (staticSourceMode() && staticWorkingTreePath(location && location.path)) {
      var localOnly = txt('button', cls || '', msg('main.local.only.source'));
      localOnly.type = 'button';
      localOnly.setAttribute(
        'title',
        staticSourceDirtyMessage()
      );
      localOnly.onclick = function () {
        showToast(staticSourceDirtyMessage(), true);
      };
      return localOnly;
    }
    // Decision 222: without a GitHub/GitLab (or server) jump the source
    // action is not offered at all — never an inline code drawer.
    if (serverMode() && currentRunID() && location && SOURCE_IDS[location.path]) {
      var button = txt('button', cls || '', label);
      button.type = 'button';
      button.onclick = action;
      return button;
    }
    return null;
  }

  function serverBasePath() {
    var match = window.location.pathname.match(/^(\/_repomap\/[A-Za-z0-9_-]+)(?:\/|$)/);
    return match ? match[1] : '';
  }

  function currentRunID() {
    var base = serverBasePath().replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    var match = base && window.location.pathname.match(new RegExp('^' + base + '/runs/([^/]+)/report\\.html$'));
    return match ? decodeURIComponent(match[1]) : '';
  }

  function showToast(message, isError) {
    var toast = document.getElementById('rm-toast');
    if (!toast) return;
    toast.textContent = message;
    toast.className = 'rm-toast' + (isError ? ' rm-toast--error' : '');
    toast.hidden = false;
    if (toastTimer) window.clearTimeout(toastTimer);
    toastTimer = window.setTimeout(function () { toast.hidden = true; }, 2600);
  }

  function copyText(value) {
    if (!navigator.clipboard || !navigator.clipboard.writeText) return;
    navigator.clipboard.writeText(value).then(function () {
      showToast(msg('main.toast.copied', { value: value }), false);
    }).catch(function () {});
  }

  function showEditorUnavailable(filePath, line, column) {
    var toast = document.getElementById('rm-toast');
    if (!toast) return;
    var location = filePath + (line ? ':' + line : '') + (column ? ':' + column : '');
    toast.replaceChildren();
    toast.className = 'rm-toast rm-toast--error rm-toast--fallback';
    toast.appendChild(txt('strong', '', msg('main.vs.code.is.not.available')));
    var actions = el('span', 'rm-toast-actions');
    var relative = txt('button', '', msg('main.copy.repository.relative.path'));
    relative.type = 'button';
    relative.onclick = function () { copyText(filePath); };
    var exact = txt('button', '', msg('main.copy.path.line.column'));
    exact.type = 'button';
    exact.onclick = function () { copyText(location); };
    actions.append(relative, exact);
    toast.appendChild(actions);
    toast.hidden = false;
    if (toastTimer) window.clearTimeout(toastTimer);
    toastTimer = window.setTimeout(function () { toast.hidden = true; }, 8000);
  }

  function requestOpenFile(filePath, line, column) {
    var runID = currentRunID();
    var sourceID = SOURCE_IDS[filePath];
    if (!serverMode() || !runID || !sourceID) return Promise.resolve();
    var openingTimer = window.setTimeout(function () {
      showToast(msg('main.toast.opening_vscode'), false);
    }, 250);
    return fetch(serverBasePath() + '/api/open', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Repomap-Action': 'open-file',
      },
      body: JSON.stringify({ run_id: runID, source_id: sourceID, line: line || 0, column: column || 0 }),
    }).then(function (response) {
      return response.json().catch(function () { return {}; }).then(function (body) {
        if (!response.ok) {
          var error = new Error(msg('main.error.editor_action_failed'));
          error.code = body.code || '';
          throw error;
        }
        return body;
      });
    }).then(function (body) {
      window.clearTimeout(openingTimer);
      var stale = body.source_changed || DATA.freshness && (DATA.freshness.affected_paths || []).indexOf(filePath) >= 0;
      var location = filePath + (line ? ':' + line : '');
      var message = msg('main.toast.opened_vscode', { location: location });
      if (stale) message += msg('main.toast.source_changed_suffix');
      showToast(message, false);
    }).catch(function (error) {
      window.clearTimeout(openingTimer);
      if (error.code === 'editor_unavailable') {
        showEditorUnavailable(filePath, line || 0, column || 0);
        return;
      }
      showToast(msg('main.toast.could_not_open_vscode'), true);
    });
  }

  function renderFileReference(filePath, cls, line, label) {
    var text = label || filePath;
    var stale = DATA.freshness && (DATA.freshness.affected_paths || []).indexOf(filePath) >= 0;
    var staticSourceLinkNode = staticSourceLink(
      text,
      cls + ' rm-file-link' + (stale ? ' rm-file-link--stale' : ''),
      { path: filePath, line: line || 0 }
    );
    if (staticSourceLinkNode) return staticSourceLinkNode;
    if (staticSourceMode() && staticWorkingTreePath(filePath)) {
      var localReference = txt('span', cls + ' rm-file-link--stale', text);
      localReference.title = staticSourceDirtyMessage();
      return localReference;
    }
    if (!serverMode() || !OPENABLE_PATH_SET[filePath]) {
      var reference = txt('span', cls + (stale ? ' rm-file-link--stale' : ''), text);
      if (stale) reference.title = msg('main.this.source.changed.after.the.report.was.generated');
      return reference;
    }
    var button = txt('button', cls + ' rm-file-link', text);
    button.type = 'button';
    button.title = stale
      ? msg('main.source.changed_open_current')
      : msg('main.source.open_current_vscode');
    if (stale) button.classList.add('rm-file-link--stale');
    button.onclick = function (event) {
      event.preventDefault();
      event.stopPropagation();
      requestOpenFile(filePath, line || 0, 0);
    };
    return button;
  }

  function appendLinkifiedText(container, statement) {
    if ((!serverMode() && !staticSourceMode()) || !statement || OPENABLE_PATHS.length === 0) {
      container.textContent = statement || '';
      return;
    }
    var cursor = 0;
    while (cursor < statement.length) {
      var nextPath = '';
      var nextIndex = -1;
      OPENABLE_PATHS.forEach(function (filePath) {
        var index = statement.indexOf(filePath, cursor);
        if (index >= 0 && (nextIndex < 0 || index < nextIndex)) {
          nextIndex = index;
          nextPath = filePath;
        }
      });
      if (nextIndex < 0) {
        container.appendChild(document.createTextNode(statement.slice(cursor)));
        break;
      }
      if (nextIndex > cursor) {
        container.appendChild(document.createTextNode(statement.slice(cursor, nextIndex)));
      }
      var end = nextIndex + nextPath.length;
      var line = 0;
      var remainder = statement.slice(end);
      var suffix = remainder.match(/^:(\d+)/);
      var label = nextPath;
      if (suffix) {
        line = parseInt(suffix[1], 10) || 0;
        label += suffix[0];
        end += suffix[0].length;
      }
      container.appendChild(renderFileReference(nextPath, 'rm-inline-file', line, label));
      cursor = end;
    }
  }

  function linkified(tag, cls, statement) {
    var node = el(tag, cls);
    appendLinkifiedText(node, statement);
    return node;
  }

  // normalizeMarkdownProse renders repository README-derived prose as plain,
  // readable text: link syntax becomes "text (url)", emphasis/backticks are
  // stripped, blockquote markers and leading hashes are removed, and code
  // fences/HTML tags never leak into product copy. It is a display-only
  // projection: source material stays labeled, never translated in place.
  function normalizeMarkdownProse(value) {
    if (!value) return '';
    var text = String(value);
    text = text.replace(/```[\s\S]*?```/g, ' ').replace(/`([^`]*)`/g, '$1');
    text = text.replace(/!\[([^\]]*)\]\(([^)]+)\)/g, '$1 ($2)');
    text = text.replace(/\[([^\]]*)\]\(([^)]+)\)/g, '$1 ($2)');
    text = text.replace(/<[^>]+>/g, ' ');
    text = text.replace(/^\s{0,3}#{1,6}\s+/gm, '');
    text = text.replace(/^\s{0,3}(&gt;|>)\s?/gm, '');
    text = text.replace(/\*\*([^*]+)\*\*/g, '$1');
    text = text.replace(/(^|[^*])\*([^*]+)\*/g, '$1$2');
    text = text.replace(/__([^_]+)__/g, '$1');
    text = text.replace(/(^|[^_])_([^_]+)_/g, '$1$2');
    text = text.replace(/\s+/g, ' ').trim();
    return text;
  }

  function purposeSourceLabel() {
    // README-derived purpose is labeled source material, never presented as
    // translated product copy (Decision 217).
    return msg('main.provenance.readme_source');
  }

  function runPickerDate(run) {
    var parsed = new Date(String(run && run.created_at || ''));
    if (Number.isNaN(parsed.getTime())) {
      var runID = String(run && run.id || '');
      var match = runID.match(/^(\d{4})(\d{2})(\d{2})-(\d{2})(\d{2})(\d{2})/);
      if (match) {
        parsed = new Date(Date.UTC(
          Number(match[1]),
          Number(match[2]) - 1,
          Number(match[3]),
          Number(match[4]),
          Number(match[5]),
          Number(match[6])
        ));
      }
    }
    if (Number.isNaN(parsed.getTime())) {
      return REPORT_LANGUAGE === 'ru'
        ? '--.--.----, --:--:--'
        : '--/--/----, --:--:--';
    }
    var locale = REPORT_LANGUAGE === 'ru' ? 'ru-RU' : 'en-US';
    var date = new Intl.DateTimeFormat(locale, {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
    }).format(parsed);
    var time = new Intl.DateTimeFormat(locale, {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hourCycle: 'h23',
    }).format(parsed);
    return date + ', ' + time;
  }

  function runPickerShortID(run) {
    var supplied = String(run && run.short_id || '');
    if (supplied) return supplied;
    var runID = String(run && run.id || '');
    var separator = runID.lastIndexOf('-');
    var value = separator >= 0 ? runID.slice(separator + 1) : runID;
    return value.length > 12 ? value.slice(-12) : value;
  }

  function runPickerLabel(run) {
    var repository = String(run && run.repo_name || '') || msg('main.saved_repository_report');
    var language = run && run.report_language === 'ru' ? 'ru' : 'en';
    var cacheMode = run && run.cache_mode === 'no-cache' ? 'no-cache' : 'cache';
    return [
      repository,
      language,
      cacheMode,
      runPickerDate(run),
      runPickerShortID(run),
    ].join(' · ');
  }

  function setupServerFeatures() {
    if (staticSourceMode()) {
      var staticSourceHintNode = document.getElementById('rm-editor-hint');
      if (staticSourceHintNode) {
        staticSourceHintNode.textContent = staticSourceHintMessage();
        staticSourceHintNode.hidden = false;
      }
      return;
    }
    if (!serverMode()) return;
    var hint = document.getElementById('rm-editor-hint');
    if (hint) hint.hidden = false;
    fetch(serverBasePath() + '/api/runs').then(function (response) {
      if (!response.ok) throw new Error('report list unavailable');
      return response.json();
    }).then(function (payload) {
      var runs = payload.runs || [];
      if (runs.length === 0) return;
      var picker = document.getElementById('rm-run-picker');
      var selector = document.getElementById('rm-run-selector');
      var selected = currentRunID();
      selector.innerHTML = '';
      runs.forEach(function (run) {
        var option = document.createElement('option');
        option.value = run.id;
        option.textContent = runPickerLabel(run);
        option.selected = run.id === selected;
        selector.appendChild(option);
      });
      selector.onchange = function () {
        window.location.assign(serverBasePath() + '/runs/' + encodeURIComponent(selector.value) + '/report.html');
      };
      picker.hidden = false;
    }).catch(function () {
      // The report remains fully usable when the optional server API is absent.
    });
  }

  // ── Main render ─────────────────────────────────────────────────

  function viewSectionID(view) {
		if (view === 'investigate') return 'rm-task-investigation';
		if (view === 'operate') return 'rm-operate-detail';
		// Decision 236 (v11): map is the primary product — it reuses the
		// architecture canvas host; architecture is the Landscape lens.
    if (view === 'map' || view === 'architecture') return 'rm-architecture';
    if (view === 'provenance') return 'rm-provenance';
    return 'rm-architecture';
  }

  function renderViewHeading(kicker, title, copy) {
    var heading = el('div', 'rm-view-heading');
    if (kicker) heading.appendChild(txt('div', 'rm-view-kicker', kicker));
    heading.appendChild(txt('h2', '', title));
    if (copy) heading.appendChild(txt('p', '', copy));
    return heading;
  }

  function workspaceHistoryPayload(state, sourceDrawer) {
    var studyTarget = state && studyMechanismForTarget(state.mapTarget)
      ? {
        kind: 'study_mechanism',
        theme_ordinal: Number(state.mapTarget.theme_ordinal),
        investigation_id: String(state.mapTarget.investigation_id),
        mechanism_id: String(state.mapTarget.mechanism_id),
      }
      : null;
    return {
      repomapWorkspace: true,
      mapReturn: state && state.mapReturn || null,
      mapTarget: studyTarget,
      sourceDrawer: sourceDrawer || null,
    };
  }

  function writeWorkspaceHistory(hash, state, options) {
    options = options || {};
    if (!window.history || typeof window.history.pushState !== 'function') {
      if (window.location) window.location.hash = hash;
      return;
    }
    var method = options.replace ? 'replaceState' : 'pushState';
    window.history[method](workspaceHistoryPayload(state, options.sourceDrawer), '', hash);
  }

  function commitWorkspaceState(next, options) {
    options = options || {};
		var routeChanged = workspaceRouteFamily(workspaceState) !== workspaceRouteFamily(next);
    next.sourceLocation = options.keepSource ? next.sourceLocation : null;
    var hash = options.hash || workspaceHashForState(next);
    writeWorkspaceHistory(hash, next, { replace: !!options.replace });
    workspaceState = next;
    renderWorkspaceState();
		if (routeChanged) resetWorkspaceScroll();
  }

  function sourceDrawerHistoryReference(selection) {
    if (!selection || !selection.snippet) return null;
    return {
      path: selection.path || selection.snippet.path,
      line: Number(selection.line) || 0,
      column: Number(selection.column) || 0,
      presentation_sha256: selection.snippet.presentation_sha256 || '',
      expanded: !!selection.expanded,
			drawer_first: !!selection.drawerFirst,
    };
  }

  function restoreSourceDrawer(historyState) {
    var reference = historyState && historyState.sourceDrawer;
    if (!reference || !reference.path) return null;
    var snippets = allEmbeddedSourceSnippets();
    var snippet = null;
    for (var index = 0; index < snippets.length; index++) {
      if (!sourceSnippetHasCode(snippets[index]) || snippets[index].path !== reference.path) continue;
      if (reference.presentation_sha256 &&
          snippets[index].presentation_sha256 !== reference.presentation_sha256) continue;
      snippet = snippets[index];
      break;
    }
    if (!snippet) snippet = embeddedSourceForLocation(reference);
    if (!snippet) return null;
    return {
      path: reference.path,
      line: Number(reference.line) || sourceSnippetLocation(snippet).line,
      column: Number(reference.column) || 0,
      snippet: snippet,
      expanded: false,
			drawerFirst: !!reference.drawer_first,
    };
  }

  function restoreWorkspaceFromRoute(options) {
    options = options || {};
    var historyState = window.history && window.history.state || null;
    var parsed = parseWorkspaceHash(window.location && window.location.hash, historyState);
    if (parsed.state.view === 'architecture' && !userArchitectureAvailable()) {
			parsed = { state: emptyWorkspaceState(), valid: false, canonicalHash: defaultWorkspaceHash() };
    }
    workspaceState = parsed.state;
    workspaceState.sourceLocation = restoreSourceDrawer(historyState);
    var currentHash = String(window.location && window.location.hash || '');
    if (options.replace || !parsed.valid || currentHash !== parsed.canonicalHash) {
      writeWorkspaceHistory(parsed.canonicalHash, workspaceState, {
        replace: true,
        sourceDrawer: sourceDrawerHistoryReference(workspaceState.sourceLocation),
      });
    }
    renderWorkspaceState();
  }

  var workspaceRestoreScheduled = false;
  function scheduleWorkspaceRouteRestore() {
    if (workspaceRestoreScheduled) return;
    workspaceRestoreScheduled = true;
    Promise.resolve().then(function () {
      workspaceRestoreScheduled = false;
      restoreWorkspaceFromRoute();
    });
  }

  function navigateWorkspace(view, options) {
    options = options || {};
		// Decision 236 (v11): architecture is the Landscape lens of the
		// map — navigating to it opens the map with the given focus.
    if (view === 'architecture' || view === 'map') {
			if (view === 'architecture' && !userArchitectureAvailable()) {
				commitWorkspaceState(emptyWorkspaceState());
				return;
			}
      openArchitectureTarget(null, null, options);
      return;
    }
    var next = reduceWorkspaceState(workspaceState, { type: 'view', view: view });
    commitWorkspaceState(next, { replace: !!options.replace });
  }

  function addWorkspaceTab(label, view) {
    var tabs = document.getElementById('rm-tabs');
    if (!tabs) return;
    var button = txt('button', 'rm-tab', label);
    button.type = 'button';
    button.setAttribute('data-workspace-view', view);
    button.onclick = function () { navigateWorkspace(view); };
    tabs.appendChild(button);
  }

	function analysisTargetMenuItems() {
		function rootTargetLabel() {
			var identity = String(
				(ANALYSIS_TARGET && (ANALYSIS_TARGET.module_path || ANALYSIS_TARGET.package_path)) ||
				DATA.repo_name ||
				'',
			);
			var segments = identity.split('/').filter(Boolean);
			while (segments.length > 1 && /^v[0-9]+$/.test(segments[segments.length - 1])) {
				segments.pop();
			}
			var label = String(segments[segments.length - 1] || identity);
			label = label.replace(/\.v[0-9]+$/, '').replace(/\.git$/, '');
			return label || identity || '.';
		}
		function targetLabel(displayPath, kind) {
			if (kind === 'module_library') return msg('main.analysis_target.module_library_label');
			// The canonical repository-relative path remains the target identity,
			// but a literal dot is implementation notation rather than a useful
			// product label. Derive only the terminal module name, ignoring an exact
			// semantic-import major version suffix such as /v3 or .v3.
			return displayPath === '.' ? rootTargetLabel() : displayPath;
		}
		if (TARGET_NAVIGATION && Number(TARGET_NAVIGATION.version) === 2 &&
			Array.isArray(TARGET_NAVIGATION.targets)) {
			return TARGET_NAVIGATION.targets.map(function (item) {
				var moduleDir = String(item.module_dir || '.');
				var ref = String(item.target_ref || '');
				var displayPath = String(item.display_path || '');
				var kind = String(item.kind || '');
				var modulePath = String(item.module_path || '');
				return {
					ref: ref,
					label: targetLabel(displayPath, kind),
					title: kind === 'module_library' ? (modulePath || displayPath) : displayPath,
					goMod: moduleDir === '.' ? 'go.mod' : moduleDir + '/go.mod',
					isDefault: ref === String(TARGET_NAVIGATION.default_target_ref || ''),
					isActive: ref === String(TARGET_NAVIGATION.current_target_ref || ''),
					available: !!item.available,
					href: String(item.href || ''),
				};
			});
		}
		var target = ANALYSIS_TARGET || {};
		var kind = String(target.kind || '');
		var packagePath = String(target.package_path || target.module_path || target.package_dir || DATA.repo_name || msg('main.repository'));
		var packageDir = String(target.package_dir || '');
		var displayPath = kind === 'module_library'
			? String(target.module_dir || '.')
			: (packageDir || packagePath);
		var moduleDir = String(target.module_dir || '.');
		return [{
			ref: String(target.ref || ''),
			label: targetLabel(displayPath, kind),
			title: packagePath,
			goMod: moduleDir === '.' ? 'go.mod' : moduleDir + '/go.mod',
			isDefault: true,
			isActive: true,
			available: true,
			href: '#canvas',
			legacySingleTarget: true,
		}];
	}

	function renderAnalysisTargetMenu(tabs) {
		var items = analysisTargetMenuItems();
		var groups = [];
		items.forEach(function (item) {
			var group = groups.find(function (candidate) { return candidate.goMod === item.goMod; });
			if (!group) {
				group = { goMod: item.goMod, items: [] };
				groups.push(group);
			}
			group.items.push(item);
		});
		groups.forEach(function (group) {
			var section = el('section', 'rm-target-group');
			section.appendChild(txt('div', 'rm-target-group__label', group.goMod));
			var list = el('ul', 'rm-target-list');
			group.items.forEach(function (item) {
				var row = el('li', 'rm-target-list__item');
				var link = txt(
					item.available ? 'a' : 'span',
					'rm-tab rm-target-link' + (item.isActive ? ' rm-active' : '') +
						(item.available ? '' : ' rm-target-link--disabled'),
					'',
				);
				if (item.available) link.setAttribute('href', item.href);
				else link.setAttribute('aria-disabled', 'true');
				if (item.isActive) link.setAttribute('data-workspace-view', 'map');
				link.setAttribute('data-target-ref', item.ref);
				link.setAttribute('title', item.title || item.label);
				if (item.isActive) link.setAttribute('aria-current', 'page');
				if (item.isDefault) {
					var dot = el('span', 'rm-target-link__default-dot');
					dot.setAttribute('aria-hidden', 'true');
					link.appendChild(dot);
					link.appendChild(txt('span', 'rm-visually-hidden', msg('main.analysis_target.default') + ': '));
				}
				link.appendChild(txt('span', 'rm-target-link__label', item.label));
				if (item.legacySingleTarget) {
					link.onclick = function (event) {
						if (event && typeof event.preventDefault === 'function') event.preventDefault();
						var next = emptyWorkspaceState();
						commitWorkspaceState(next, { hash: '#canvas' });
						var canvas = document.getElementById('rm-architecture');
						if (canvas && typeof canvas.scrollIntoView === 'function') canvas.scrollIntoView({ block: 'start' });
					};
				}
				if (!item.available) {
					link.appendChild(txt(
						'span',
						'rm-visually-hidden',
						': ' + msg('main.analysis_target.unavailable'),
					));
				}
				row.appendChild(link);
				list.appendChild(row);
			});
			section.appendChild(list);
			tabs.appendChild(section);
		});
	}

	function userArchitectureAvailable() {
		if (ATLAS_FIRST) return !!DATA.architecture_canvas;
		if (DEBUG_MODE) return !!(DATA.architecture_canvas || (DATA.high_level_map || []).length);
		if (STUDY_MAP) return !!DATA.architecture_canvas;
		if (REPOSITORY_GUIDE) {
			return !!(REPOSITORY_GUIDE.architecture_useful && DATA.architecture_canvas);
		}
		return !!DATA.architecture_canvas;
	}

  function repositoryLocationAvailable(location) {
    if (!location || !location.path || !OPENABLE_PATH_SET[location.path]) return false;
    if (staticSourceLocationAvailable(location.path, location.line)) return true;
    if (embeddedSourceForLocation(location)) return true;
    return !!(serverMode() && currentRunID() && SOURCE_IDS[location.path]);
  }

  function repositoryAreaAction(area) {
    if (area && repositoryLocationAvailable(area.code_location)) return 'code';
    if (area && area.map_target && userArchitectureAvailable()) return 'map';
    return '';
  }

  function renderRepositoryArea(area) {
    var action = repositoryAreaAction(area);
    if (!action) return null;
    var remoteCard = action === 'code'
      ? staticSourceLink('', 'rm-repository-area rm-source-target-link', area.code_location)
      : null;
    var card = remoteCard;
    if (!card) {
      card = el('button', 'rm-repository-area');
      card.type = 'button';
    }
    card.appendChild(txt('strong', '', area.label || area.name || msg('main.code.area')));
    if (area.responsibility) card.appendChild(txt('span', 'rm-repository-area__responsibility', area.responsibility));
    if (action === 'code') {
      card.appendChild(txt('code', '', formatCodeLocation(area.code_location)));
      card.appendChild(txt('span', 'rm-repository-area__action', msg('main.open.code')));
      if (!remoteCard) {
        card.onclick = function () { openSourceLocation(area.code_location); };
      }
    } else {
      card.appendChild(txt('span', 'rm-repository-area__action', msg('main.view.on.map')));
      card.onclick = function () { openArchitectureTarget(area.map_target, null); };
    }
    return card;
  }

	function openStudyDirection(directionID) {
		var direction = studyDirectionByID(directionID);
		if (!direction) return;
		workspaceState.view = 'map';
		workspaceState.directionID = direction.id;
		workspaceState.themeCardOrdinal = 0;
		var id = 'study-direction-' + encodeRoutePart(direction.id);
		writeWorkspaceHistory('#' + id, workspaceState);
		var detail = document.getElementById(id);
		if (!detail) return;
		detail.open = true;
		if (typeof detail.scrollIntoView === 'function') detail.scrollIntoView({ block: 'start' });
	}

	function openThemeCard(ordinal, options) {
		options = options || {};
		ordinal = Number(ordinal) || 0;
		if (!themeCardByOrdinal(ordinal)) return;
		workspaceState.view = 'map';
		workspaceState.themeCardOrdinal = ordinal;
		workspaceState.directionID = '';
		if (options.history !== false) {
			writeWorkspaceHistory('#study-theme-' + ordinal, workspaceState, { replace: !!options.replace });
		}
		var detail = document.getElementById('study-theme-' + ordinal);
		if (!detail) {
			var studyRoot = document.getElementById('rm-study') || document.getElementById('rm-study-overview');
			var candidates = studyRoot && studyRoot.querySelectorAll
				? Array.prototype.slice.call(studyRoot.querySelectorAll('.rm-study-theme-card')) : [];
			detail = candidates.find(function (candidate) {
				return Number(candidate.getAttribute('data-study-theme-ordinal')) === ordinal;
			}) || null;
		}
		if (!detail) return;
		detail.open = true;
		if (typeof detail.scrollIntoView === 'function') detail.scrollIntoView({ block: 'start' });
		if (options.focus) {
			var summary = detail.querySelector && detail.querySelector('summary');
			if (summary && typeof summary.focus === 'function') summary.focus();
		}
	}

	function openPavedPath(operationID) {
		if (!pavedPathByID(operationID)) {
			commitWorkspaceState(emptyWorkspaceState());
			return;
		}
		var next = reduceWorkspaceState(workspaceState, {
			type: 'open_operation', operationID: operationID,
		});
		commitWorkspaceState(next);
	}

	function firstOperationalLiteral(pavedPath) {
		var actions = pavedPath && Array.isArray(pavedPath.actions) ? pavedPath.actions : [];
		for (var index = 0; index < actions.length; index++) {
			if (actions[index] && (actions[index].command || actions[index].endpoint)) {
				return actions[index].command || actions[index].endpoint;
			}
		}
		return '';
	}

	function renderPavedPathCard(pavedPath) {
		var card = el('button', 'rm-operation-card');
		card.type = 'button';
		card.appendChild(txt('span', 'rm-operation-card__label', msg('main.run.and.verify')));
		card.appendChild(txt('strong', '', pavedPath.title || msg('main.repository.operation')));
		if (pavedPath.goal) card.appendChild(txt('p', '', pavedPath.goal));
		var literal = firstOperationalLiteral(pavedPath);
		if (literal) card.appendChild(txt('code', 'rm-operation-card__literal', literal));
		var actionCount = Array.isArray(pavedPath.actions) ? pavedPath.actions.length : 0;
		if (actionCount) {
			card.appendChild(txt('span', 'rm-operation-card__meta', msg('main.count.steps', { count: actionCount })));
		}
		card.appendChild(txt('span', 'rm-operation-card__action', msg('main.open.instructions')));
		card.onclick = function () { openPavedPath(pavedPath.id); };
		return card;
	}

	function renderOperationalLandmark(landmark) {
		var card = el('article', 'rm-operation-landmark');
		card.appendChild(txt('span', 'rm-operation-card__label', msg('main.repository.reference')));
		card.appendChild(txt('strong', '', landmark.label || msg('main.operational.reference')));
		if (landmark.command) card.appendChild(txt('code', 'rm-operation-card__literal', landmark.command));
		if (landmark.endpoint) card.appendChild(txt('code', 'rm-operation-card__literal', landmark.endpoint));
		var actions = el('div', 'rm-overview-actions');
		if (landmark.copy_text) {
			var copy = txt('button', 'rm-secondary-action', msg('main.copy.command'));
			copy.type = 'button';
			copy.onclick = function () { copyText(landmark.copy_text); };
			actions.appendChild(copy);
		}
		if (landmark.reference && !landmark.reference.redacted && sourceSnippetAvailable(landmark.reference.source)) {
			var source = sourceActionElement(
				staticSourceMode() ? staticSourceOpenLabel() : msg('main.show.source'),
				'rm-quiet-action rm-source-action-link',
				landmark.reference.location || sourceSnippetLocation(landmark.reference.source),
				Number(landmark.reference.source.end_line) || 0,
				function () {
					openSourceSnippet(landmark.reference.source, landmark.reference.location, false);
				}
			);
			if (source) actions.appendChild(source);
		}
		if (actions.childNodes.length) card.appendChild(actions);
		return card;
	}

	function renderOperationsOverview(root) {
		if (!PAVED_PATHS.length && !OPERATIONAL_LANDMARKS.length) return;
		var section = el('section', 'rm-workspace-section rm-operations-section');
		section.appendChild(renderViewHeading(
			msg('main.how.to.run.and.verify'),
			msg('main.repository.backed.operating.paths'),
			msg('main.use.exact.commands.endpoints.and.checks.saved.from.this.repository.with.their.source.beside.them')
		));
		if (PAVED_PATHS.length) {
			var grid = el('div', 'rm-operation-grid');
			PAVED_PATHS.forEach(function (pavedPath) { grid.appendChild(renderPavedPathCard(pavedPath)); });
			section.appendChild(grid);
		} else {
			var landmarks = el('div', 'rm-operation-landmark-grid');
			OPERATIONAL_LANDMARKS.slice(0, 8).forEach(function (landmark) {
				landmarks.appendChild(renderOperationalLandmark(landmark));
			});
			section.appendChild(landmarks);
		}
		root.appendChild(section);
	}

	function renderStudyDirectionCard(direction, index) {
		var card = el('details', 'rm-study-direction-card');
		card.id = 'study-direction-' + encodeRoutePart(direction.id);
		var summary = el('summary', 'rm-study-direction-card__summary');
		summary.appendChild(txt('span', 'rm-study-direction-card__order', String(index + 1)));
		var body = el('div', 'rm-study-direction-card__body');
			if (direction.incomplete) {
				body.appendChild(txt(
					'span',
					'rm-study-reading-anchor__label',
					msg('main.study.stage_incomplete', {
						stage: studyStageLabel(direction.learning_stage),
					})
				));
		}
		var title = txt(
			'span',
			'rm-study-direction-card__title',
			direction.question || msg('main.chrome.explore.this.code.area')
		);
		summary.appendChild(title);
		if (direction.why_it_matters) summary.appendChild(txt('span', 'rm-study-direction-card__reason', direction.why_it_matters));
		if (direction.learning_outcome) summary.appendChild(txt('span', 'rm-study-direction-card__outcome', direction.learning_outcome));
		card.appendChild(summary);
		var anchors = el('div', 'rm-study-direction-card__anchors');
		(direction.reading_anchors || []).forEach(function (reading) {
			var location = studyReadingLocation(reading);
			if (!location) return;
			var action = renderStudySourceAction(reading, 'rm-study-direction-card__source', true);
			if (action) {
				anchors.appendChild(action);
				return;
			}
			// Decision 222: without a GitHub/GitLab (or server) jump the
			// reading stays a typed row — bare symbol + location — never a
			// dead button and never an inline code drawer.
			var row = el('div', 'rm-study-direction-card__source rm-study-direction-card__source--plain');
			row.appendChild(txt('strong', '', bareSourceSymbol(String(reading && reading.symbol || ''))));
			var kind = sourceKindFor(reading && reading.symbol, reading && reading.path);
			row.appendChild(txt('span', 'rm-study-direction-card__source-kind', sourceKindLabel(kind)));
			row.appendChild(txt('code', '', formatCodeLocation(location)));
			anchors.appendChild(row);
		});
		if (anchors.childNodes.length) body.appendChild(anchors);
		card.appendChild(body);
		return card;
	}

	var STUDY_STAGE_MESSAGE_IDS = {
		orientation: 'main.study.stage.orientation',
		central_operation: 'main.study.stage.central_operation',
		core_model: 'main.study.stage.core_model',
		integration: 'main.study.stage.integration',
		operations: 'main.study.stage.operations',
		contribution: 'main.study.stage.contribution',
	};

	function studyStageLabel(stage) {
		return msg(STUDY_STAGE_MESSAGE_IDS[String(stage || '')] || 'main.study.stage.study');
	}

	function renderIncompleteStudyOverview() {
		var root = document.getElementById('rm-study') || document.getElementById('rm-study-overview');
		if (!root) return;
		root.replaceChildren();
		// D213: the source-grounded theme shelf is the primary Study surface
		// when present; the legacy direction surface only renders when no
		// theme cards exist.
		var cards = themeCards();
		if (cards.length) {
			renderAtlasStudyThemeShelf(root, cards);
			return;
		}
		if (COMPLETE_STUDY_DIRECTIONS.length) {
			renderStudyMapOverview(root, false);
			return;
		}
		var directions = INCOMPLETE_STUDY_DIRECTIONS;
		root.appendChild(renderViewHeading(
			msg('main.study.incomplete.directions'),
			msg('main.what.is.worth.understanding.next'),
			msg('main.questions.backed.by.exact.starting.points.they.are.places.to.begin.not.answers.or.ordered.mechanisms')
		));
		var directionList = el('div', 'rm-study-direction-list');
		directions.forEach(function (direction, index) {
			directionList.appendChild(renderStudyDirectionCard(direction, index));
		});
		root.appendChild(directionList);
	}

	function appendInlineStudyWorkspace(root) {
		if (!root || !(STUDY_DIRECTIONS.length || themeCards().length)) return;
		var study = el('section', 'rm-target-study');
		study.id = 'rm-study';
		study.setAttribute('aria-labelledby', 'rm-study-heading');
		root.appendChild(study);
		renderIncompleteStudyOverview();
		var heading = study.querySelector && study.querySelector('h2');
		if (heading) heading.id = 'rm-study-heading';
	}

	// D213: renders the source-grounded theme shelf (cards with exact
	// readings), the diagnostics panel and the four-stage frontier browse.
	// themeCoverageState maps the stored badge to a user-facing coverage
	// state: "supported" for source-backed themes, "partial" otherwise.
	// It never presents "1 of 1 anchors passed" as failed evidence
	// (Decision 217).
	function themeCoverageState(card) {
		if (!card) return 'partial';
		var badge = String(card.badge || '');
		if (badge === 'editorial_source_backed' || badge === 'source_backed') return 'supported';
		return 'partial';
	}

	function themeCoverageLabel(card) {
		var state = themeCoverageState(card);
		return state === 'supported'
			? msg('main.study.theme.supported')
			: msg('main.study.theme.partial');
	}

	// themeScopeState derives the Decision 229 D6 scope axis
	// deterministically from the card readings: the theme scope is "exact"
	// when every reading carries an exact resolvable source location with a
	// direct role, and "partial" otherwise. Scope and exact-source attachment
	// are independent axes: partial scope does not remove an exact anchor, and
	// a narrow-but-exact theme is not failed evidence.
	function themeScopeState(card) {
		var readings = card && Array.isArray(card.readings) ? card.readings : [];
		if (!readings.length) return 'partial';
		for (var index = 0; index < readings.length; index++) {
			var reading = readings[index];
			if (!reading) return 'partial';
			if (!reading.path || !Number.isInteger(reading.line) || reading.line <= 0) return 'partial';
			if (reading.role === 'supporting') return 'partial';
		}
		return 'exact';
	}

	function themeScopeLabel(card) {
		return themeScopeState(card) === 'exact'
			? msg('main.study.theme.scope_exact')
			: msg('main.study.theme.scope_partial');
	}

	// themeKindLabel maps the stored theme kind to user-facing copy.
	// Unknown kinds fall back to the neutral "Learning theme" label — raw
	// enum values are never primary user copy (D229 D4).
	var THEME_KIND_MESSAGE_IDS = {
		user_journey: 'main.study.theme.kind.user_journey',
		lifecycle_concern: 'main.study.theme.kind.lifecycle_concern',
		cross_cutting_policy: 'main.study.theme.kind.cross_cutting_policy',
		sibling_implementation_family: 'main.study.theme.kind.sibling_family',
		integration_family: 'main.study.theme.kind.integration_family',
		shared_domain_responsibility: 'main.study.theme.kind.shared_domain',
	};
	function themeKindLabel(card) {
		var kind = String((card && card.theme_kind) || '');
		return msg(THEME_KIND_MESSAGE_IDS[kind] || 'main.study.theme.kind.learning');
	}

	// Every bounded reading stays visible on the Study shelf. A count such as
	// "+1 reading" is not navigation: the reader needs the exact destination.
	function themeReadingPreviewRows(card) {
		var readings = card && Array.isArray(card.readings) ? card.readings : [];
		return readings.filter(function (reading) {
			if (!reading || !reading.path || !Number.isInteger(reading.line)) return;
			return true;
		}).map(function (reading) {
			return { symbol: String(reading.symbol || ''), count: 1, first: reading };
		});
	}

	// themeCoverageExplanation renders the coverage limitation in user-facing
	// terms. A "1 of 1 anchors passed source review" limitation is explained
	// as supported-but-narrow rather than as failed evidence.
	function themeCoverageExplanation(card) {
		var state = themeCoverageState(card);
		if (state === 'supported') return msg('main.study.theme.supported_explanation');
		var limitation = String(card.limitation || '');
		var anchorsPassed = limitation.match(/(\d+)\s+of\s+(\d+)\s+anchors?\s+passed/);
		if (anchorsPassed) {
			return msg('main.study.theme.partial_explanation', {
				passed: anchorsPassed[1],
				total: anchorsPassed[2],
			});
		}
		return msg('main.study.theme.partial_explanation_generic');
	}

	function themeUnknowns(card) {
		var seen = new Set();
		var unknowns = [];
		(card && Array.isArray(card.unknowns) ? card.unknowns : []).forEach(function (unknown) {
			var value = String(unknown || '').trim();
			if (!value || seen.has(value)) return;
			seen.add(value);
			unknowns.push(value);
		});
		return unknowns;
	}

	function themeFirstLimitation(card) {
		var unknowns = themeUnknowns(card);
		if (unknowns.length) return unknowns[0];
		if (themeCoverageState(card) === 'partial' || String(card && card.limitation || '').trim()) {
			return msg('main.study.theme.partial_limitation_short');
		}
		return '';
	}

	function themeReadingExactKey(reading) {
		var path = String(reading && reading.path || '');
		var line = Number(reading && reading.line);
		if (!path || !Number.isInteger(line) || line <= 0) return '';
		return JSON.stringify([path, line, String(reading && reading.symbol || '')]);
	}

	// Alternate readings are backend-deduplicated by exact public identity.
	// Keep the presentation defensive as well: an exact primary source row is
	// never rendered a second time if a malformed projection repeats it.
	function distinctThemeAlternateReadings(card) {
		var seen = new Set();
		var result = [];
		(card && Array.isArray(card.readings) ? card.readings : []).forEach(function (reading) {
			var key = themeReadingExactKey(reading);
			if (key) seen.add(key);
		});
		(card && Array.isArray(card.alternate_readings) ? card.alternate_readings : []).forEach(function (reading) {
			var key = themeReadingExactKey(reading);
			if (!key || seen.has(key)) return;
			seen.add(key);
			result.push(reading);
		});
		return result;
	}

	// A role badge is useful only when one card actually asks the reader to
	// distinguish primary and supporting locations. Repeating "direct" beside
	// every source in a direct-only card adds no information.
	function themeReadingRolesCoexist(card) {
		var roles = new Set();
		var readings = (card && Array.isArray(card.readings) ? card.readings : [])
			.concat(distinctThemeAlternateReadings(card));
		readings.forEach(function (reading) {
			var role = String(reading && reading.role || '');
			if (role === 'direct' || role === 'supporting') roles.add(role);
		});
		return roles.has('direct') && roles.has('supporting');
	}

	function renderThemeAlternates(card) {
		var titles = card && Array.isArray(card.alternate_titles)
			? card.alternate_titles.map(function (value) { return String(value || '').trim(); }).filter(Boolean)
			: [];
		var questions = card && Array.isArray(card.alternate_questions)
			? card.alternate_questions.map(function (value) { return String(value || '').trim(); }).filter(Boolean)
			: [];
		var readings = distinctThemeAlternateReadings(card);
		if (!titles.length && !questions.length && !readings.length) return null;

		var section = el('section', 'rm-study-theme-alternates');
		section.appendChild(txt('h3', 'rm-study-theme-alternates__title', msg('main.study.theme.alternates.title')));
		section.appendChild(txt('p', 'rm-study-theme-alternates__copy', msg('main.study.theme.alternates.copy')));
		var perspectiveCount = Math.max(titles.length, questions.length);
		if (perspectiveCount) {
			var perspectives = el('ul', 'rm-study-theme-alternates__perspectives');
			for (var index = 0; index < perspectiveCount; index++) {
				var perspective = el('li', 'rm-study-theme-alternates__perspective');
				if (titles[index]) {
					perspective.appendChild(txt('p', 'rm-study-theme-alternates__wording',
						msg('main.study.theme.alternates.title_wording', { title: titles[index] })));
				}
				if (questions[index]) {
					perspective.appendChild(txt('p', 'rm-study-theme-alternates__wording',
						msg('main.study.theme.alternates.question_wording', { question: questions[index] })));
				}
				perspectives.appendChild(perspective);
			}
			section.appendChild(perspectives);
		}
		if (readings.length) {
			section.appendChild(txt('h4', 'rm-study-theme-alternates__readings-title', msg('main.study.theme.alternates.readings')));
			var readingList = el('div', 'rm-study-reading-list rm-study-theme-alternates__reading-list');
			var primaryCount = card && Array.isArray(card.readings) ? card.readings.length : 0;
			readings.forEach(function (reading, index) {
				var item = renderStudyReadingAnchor(reading, primaryCount + index, themeReadingRolesCoexist(card));
				if (item) readingList.appendChild(item);
			});
			if (readingList.childNodes.length) section.appendChild(readingList);
		}
		return section;
	}

	function renderThemeLimitations(card) {
		var unknowns = themeUnknowns(card);
		if (!unknowns.length) return null;
		var section = el('section', 'rm-study-theme-card__limitations');
		section.appendChild(txt('h3', 'rm-study-theme-card__limitations-title', msg('main.study.theme.limitations')));
		var list = el('ul', 'rm-study-theme-card__unknowns');
		unknowns.forEach(function (unknown) {
			list.appendChild(txt('li', 'rm-study-theme-card__unknowns-item', unknown));
		});
		section.appendChild(list);
		return section;
	}

	function renderAtlasStudyThemeShelf(root, cards) {
		var heading = renderViewHeading(
			msg('main.study'),
			msg('main.study.themes.title'),
			''
		);
		var headingTitle = heading.querySelector && heading.querySelector('h2');
		if (headingTitle) headingTitle.id = 'rm-study-heading';
		root.appendChild(heading);
		var contents = el('nav', 'rm-study-theme-contents');
		contents.setAttribute('aria-label', msg('main.study.contents'));
		contents.appendChild(txt('h3', 'rm-study-theme-contents__title', msg('main.study.contents')));
		var contentList = el('ol', 'rm-study-theme-contents__list');
		cards.forEach(function (card, index) {
			var item = el('li', 'rm-study-theme-contents__item');
			var ordinal = Number(card.ordinal) || (index + 1);
			var action = txt('a', 'rm-study-theme-contents__action', String(card.final_title || ''));
			action.setAttribute('href', '#study-theme-' + ordinal);
			action.onclick = function (event) {
				if (event && typeof event.preventDefault === 'function') event.preventDefault();
				openThemeCard(ordinal, { focus: true });
			};
			item.appendChild(action);
			contentList.appendChild(item);
		});
		contents.appendChild(contentList);
		root.appendChild(contents);
		var shelf = el('div', 'rm-study-theme-shelf');
		cards.forEach(function (card, index) {
			shelf.appendChild(renderThemeCard(card, index));
		});
		root.appendChild(shelf);
	}

	function renderThemeCard(card, index) {
		var ordinal = Number(card.ordinal) || (index + 1);
		var article = el('details', 'rm-study-theme-card' + (card.badge ? ' rm-study-theme-card--' + String(card.badge).toLowerCase() : ''));
		article.id = 'study-theme-' + ordinal;
		article.setAttribute('data-study-theme-ordinal', String(ordinal));
		if (Number(workspaceState.themeCardOrdinal) === ordinal) article.open = true;
		var summary = el('summary', 'rm-study-theme-card__summary');
		var titleRow = el('span', 'rm-study-theme-card__title-row');
		var title = txt('span', 'rm-study-theme-card__title', card.final_title || '');
		titleRow.appendChild(title);
		summary.appendChild(titleRow);
		if (card.final_question) summary.appendChild(txt('span', 'rm-study-theme-card__question', card.final_question));
		if (card.why_it_matters) summary.appendChild(txt('span', 'rm-study-theme-card__reason', card.why_it_matters));
		article.appendChild(summary);
		var body = el('div', 'rm-study-theme-card__body');
		var readings = el('div', 'rm-study-reading-list');
		var showReadingRoles = themeReadingRolesCoexist(card);
		(card.readings || []).forEach(function (reading, readingIndex) {
			var item = renderStudyReadingAnchor(reading, readingIndex, showReadingRoles);
			if (item) readings.appendChild(item);
		});
		if (readings.childNodes.length) body.appendChild(readings);
		var investigation = renderStudyInvestigation(card);
		if (investigation) body.appendChild(investigation);
		var alternates = renderThemeAlternates(card);
		if (alternates) body.appendChild(alternates);
		var limitations = renderThemeLimitations(card);
		if (limitations) body.appendChild(limitations);
		article.appendChild(body);
		return article;
	}

	function studyInvestigationLocation(value) {
		var path = String(value && value.path || '');
		var line = Number(value && value.line);
		if (!path || !Number.isInteger(line) || line <= 0) return null;
		return {
			path: path,
			line: line,
			column: Number.isInteger(Number(value.column)) && Number(value.column) > 0 ? Number(value.column) : 0,
		};
	}

	function renderStudyInvestigationSourceAction(location, label, cls) {
		var exact = studyInvestigationLocation(location);
		if (!exact) return null;
		var visible = String(label || '') + ' · ' + formatCodeLocation(exact);
		return renderStudyInvestigationExactAction(exact, visible, cls);
	}

	function renderStudyInvestigationExactAction(location, visible, cls) {
		var exact = studyInvestigationLocation(location);
		if (!exact) return null;
		visible = String(visible || formatCodeLocation(exact));
		var resolution = exactReportSourceActionResolutionForLocation(exact);
		var snippet = resolution && resolution.source && resolution.source.snippet;
		if (snippet) {
			var button = txt('button', (cls || '') + ' rm-source-action-link', visible);
			button.type = 'button';
			button.onclick = function () {
				openSourceSnippet(snippet, resolution.source.location, false, { drawerFirst: true });
			};
			return button;
		}
		if (resolution && resolution.source && resolution.source.location) {
			return sourceActionElement(
				visible,
				(cls || '') + ' rm-source-action-link',
				resolution.source.location,
				0,
				function () { openSourceLocation(resolution.source.location); }
			);
		}
		return txt('code', (cls || '') + ' rm-study-investigation__source--plain', visible);
	}

	function studyInvestigationInvocationLabel(invocation) {
		var ids = {
			goroutine: 'main.study.investigation.invocation.goroutine',
			deferred: 'main.study.investigation.invocation.deferred',
		};
		var id = ids[String(invocation || '')];
		return id ? msg(id) : '';
	}

	function showStudyMechanismOnMap(card, investigation, mechanism) {
		if (!card || !investigation || !mechanism || !userArchitectureAvailable()) return;
		activeMapLens = 'landscape';
		workspaceState.view = 'map';
		workspaceState.themeCardOrdinal = 0;
		workspaceState.mapReturn = null;
		workspaceState.mapTarget = {
			kind: 'study_mechanism',
			theme_ordinal: Number(card.ordinal),
			investigation_id: String(investigation.id || ''),
			mechanism_id: String(mechanism.id || ''),
		};
		writeWorkspaceHistory('#canvas', workspaceState);
		if (architectureCanvasView && typeof architectureCanvasView.setLens === 'function') {
			architectureCanvasView.setLens(activeMapLens);
		}
		var entryContext = document.querySelector && document.querySelector('.rm-map-entry-context');
		if (entryContext) entryContext.hidden = true;
		syncMapLensControl();
		applyStudyMechanismMapOverlay();
		renderStudyMechanismMapContextIntoHost();
		var canvas = document.getElementById('rm-architecture');
		if (canvas && typeof canvas.scrollIntoView === 'function') canvas.scrollIntoView({ block: 'start' });
	}

	function compareStudyInvestigationLocations(left, right) {
		left = studyInvestigationLocation(left);
		right = studyInvestigationLocation(right);
		if (!left && !right) return 0;
		if (!left) return 1;
		if (!right) return -1;
		if (left.path !== right.path) return left.path < right.path ? -1 : 1;
		if (left.line !== right.line) return left.line - right.line;
		return left.column - right.column;
	}

	function studyInvestigationRootKey(mechanism, index) {
		var nodes = Array.isArray(mechanism && mechanism.nodes) ? mechanism.nodes : [];
		var location = studyInvestigationLocation(nodes[0] && nodes[0].declaration);
		return location
			? JSON.stringify([location.path, location.line, location.column])
			: 'mechanism:' + String(mechanism && mechanism.id || index);
	}

	function groupedStudyInvestigationMechanisms(investigation) {
		var groups = [];
		var byRoot = Object.create(null);
		investigation.mechanisms.forEach(function (mechanism, index) {
			var key = studyInvestigationRootKey(mechanism, index);
			var group = byRoot[key];
			if (!group) {
				group = { root: mechanism.nodes && mechanism.nodes[0], mechanisms: [] };
				byRoot[key] = group;
				groups.push(group);
			}
			group.mechanisms.push({ mechanism: mechanism, index: index });
		});
		groups.forEach(function (group) {
			group.mechanisms.sort(function (left, right) {
				var leftEdges = Array.isArray(left.mechanism && left.mechanism.edges) ? left.mechanism.edges : [];
				var rightEdges = Array.isArray(right.mechanism && right.mechanism.edges) ? right.mechanism.edges : [];
				var order = compareStudyInvestigationLocations(
					leftEdges[0] && leftEdges[0].callsite,
					rightEdges[0] && rightEdges[0].callsite
				);
				return order || left.index - right.index;
			});
		});
		return groups;
	}

	function studyInvestigationCallLabel(node) {
		var label = bareSourceSymbol(node && node.label);
		if (!label) return '';
		return /\(\)$/.test(label) ? label : label + '()';
	}

	function studyInvestigationNodeByID(nodes, id) {
		id = String(id || '');
		for (var index = 0; index < nodes.length; index++) {
			if (String(nodes[index] && nodes[index].id || '') === id) return nodes[index];
		}
		return null;
	}

	function renderStudyInvestigationTraceAction(location, visible, cls) {
		var action = renderStudyInvestigationExactAction(
			location,
			visible,
			'rm-study-investigation__source ' + String(cls || '')
		);
		return action || txt('code', 'rm-study-investigation__source--plain ' + String(cls || ''), visible);
	}

	function renderStudyInvestigationRoot(rootNode) {
		var header = el('header', 'rm-study-investigation__root');
		var location = studyInvestigationLocation(rootNode && rootNode.declaration);
		var path = location ? location.path : '';
		var declaration = location
			? String(location.line) + ' ' + studyInvestigationCallLabel(rootNode)
			: studyInvestigationCallLabel(rootNode);
		header.appendChild(renderStudyInvestigationTraceAction(
			rootNode && rootNode.declaration,
			path,
			'rm-study-investigation__root-file'
		));
		header.appendChild(renderStudyInvestigationTraceAction(
			rootNode && rootNode.declaration,
			declaration,
			'rm-study-investigation__root-declaration'
		));
		return header;
	}

	function studyInvestigationCallsiteLabel(rootLocation, callsite) {
		var exact = studyInvestigationLocation(callsite);
		if (!exact) return '';
		return rootLocation && rootLocation.path === exact.path
			? String(exact.line)
			: formatCodeLocation(exact);
	}

	function renderStudyInvestigationTraceRow(nodes, edges, edgeIndex, rootLocation) {
		var edge = edges[edgeIndex];
		var row = el('div', 'rm-study-investigation__row');
		row.setAttribute('data-study-source-row', String(edgeIndex + 1));
		row.appendChild(renderStudyInvestigationTraceAction(
			edge && edge.callsite,
			studyInvestigationCallsiteLabel(rootLocation, edge && edge.callsite),
			'rm-study-investigation__line'
		));
		var invocation = studyInvestigationInvocationLabel(edge && edge.invocation);
		if (invocation) row.appendChild(txt('span', 'rm-study-investigation__invocation', invocation));
		row.appendChild(txt('span', 'rm-study-investigation__arrow', '→'));
		var callee = studyInvestigationNodeByID(nodes, edge && edge.to_node_id);
		row.appendChild(renderStudyInvestigationTraceAction(
			callee && callee.declaration,
			studyInvestigationCallLabel(callee),
			'rm-study-investigation__callee'
		));
		return row;
	}

	function renderStudyMechanismPath(card, investigation, mechanism, index, options) {
		options = options || {};
		var article = el('article', 'rm-study-investigation__mechanism');
		article.setAttribute('data-study-mechanism-id', String(mechanism && mechanism.id || ''));
		var nodes = Array.isArray(mechanism && mechanism.nodes) ? mechanism.nodes : [];
		var edges = Array.isArray(mechanism && mechanism.edges) ? mechanism.edges : [];
		if (options.showRoot !== false && nodes[0]) article.appendChild(renderStudyInvestigationRoot(nodes[0]));
		if (options.showMap !== false && userArchitectureAvailable()) {
			var showMap = txt('button', 'rm-study-investigation__map', msg('main.study.investigation.show_on_map'));
			showMap.type = 'button';
			showMap.onclick = function () { showStudyMechanismOnMap(card, investigation, mechanism); };
			article.appendChild(showMap);
		}
		var rows = el('div', 'rm-study-investigation__rows');
		var rootLocation = studyInvestigationLocation(nodes[0] && nodes[0].declaration);
		for (var edgeIndex = 0; edgeIndex < edges.length; edgeIndex += 1) {
			rows.appendChild(renderStudyInvestigationTraceRow(nodes, edges, edgeIndex, rootLocation));
		}
		article.appendChild(rows);
		return article;
	}

	function renderStudyInvestigationGroup(card, investigation, group) {
		var section = el('section', 'rm-study-investigation__group');
		section.setAttribute('data-study-trace-root', studyInvestigationRootKey(group.mechanisms[0].mechanism, 0));
		if (group.root) section.appendChild(renderStudyInvestigationRoot(group.root));
		group.mechanisms.forEach(function (entry) {
			section.appendChild(renderStudyMechanismPath(
				card,
				investigation,
				entry.mechanism,
				entry.index,
				{ showRoot: false }
			));
		});
		return section;
	}

	function renderStudyInvestigation(card) {
		var investigation = currentStudyInvestigation(card);
		if (!investigation || investigation.outcome !== 'mechanism' || !investigation.mechanisms.length) return null;
		var section = el('section', 'rm-study-investigation rm-study-investigation--' + String(investigation.outcome));
		section.appendChild(txt('h3', 'rm-study-investigation__title', msg('main.study.investigation.title')));
		section.appendChild(txt('p', 'rm-study-investigation__copy', msg('main.study.investigation.copy')));
		var paths = el('div', 'rm-study-investigation__paths');
		groupedStudyInvestigationMechanisms(investigation).forEach(function (group) {
			paths.appendChild(renderStudyInvestigationGroup(card, investigation, group));
		});
		if (paths.childNodes.length) section.appendChild(paths);
		return section;
	}

// repomap-source-episode:start
	function sourceEpisodeStateLabel(state) {
		switch (String(state || '').toLowerCase()) {
		case 'extracted': return msg('main.evidence_state.extracted');
		case 'corroborated': return msg('main.evidence_state.corroborated');
		case 'inferred': return msg('main.evidence_state.inferred');
		case 'unknown': return msg('main.evidence_state.unknown');
		default: return '';
		}
	}

	function renderSourceEpisodeState(state) {
		var label = sourceEpisodeStateLabel(state);
		if (!label) return null;
		var badge = txt('span', 'rm-source-episode__state rm-source-episode__state--' + String(state).toLowerCase(), label);
		badge.setAttribute('aria-label', msg('main.source.evidence_status', {
      status: label.toLowerCase(),
    }));
		return badge;
	}

	function sourceEpisodeSourceAvailable(source) {
		return !!(
			source && source.path && OPENABLE_PATH_SET[source.path] &&
			(staticSourceURL(source.path, source.start_line, source.end_line) ||
				typeof SOURCE_IDS[source.path] === 'string' && SOURCE_IDS[source.path])
		);
	}

	function sourceEpisodeSourceLabel(source) {
		if (!source) return '';
		var start = Number(source.start_line) || 0;
		var end = Number(source.end_line) || start;
		return source.path + (start ? ':' + start + (end > start ? '–' + end : '') : '');
	}

	function renderSourceEpisodeSources(sources) {
		var authorized = (Array.isArray(sources) ? sources : []).filter(sourceEpisodeSourceAvailable);
		if (!authorized.length) return null;
		var actions = el('div', 'rm-source-episode__sources');
		authorized.forEach(function (source) {
			var button = sourceActionElement(
				sourceEpisodeSourceLabel(source),
				'rm-source-episode__source rm-source-action-link',
				{ path: source.path, line: Number(source.start_line) || 0 },
				Number(source.end_line) || 0,
				function () {
					if (!sourceEpisodeSourceAvailable(source)) return;
					openSourceLocation({ path: source.path, line: Number(source.start_line) || 0 });
				}
			);
			// Decision 222: without a GitHub/GitLab (or server) jump the
			// source stays a typed row — never a dead button.
			if (!button) return;
			button.setAttribute('aria-label', msg('main.source.inspect_exact', {
        label: sourceEpisodeSourceLabel(source),
      }));
			actions.appendChild(button);
		});
		return actions;
	}

	function renderSourceEpisodeClaim(claim, index) {
		if (!claim || !sourceEpisodeStateLabel(claim.state)) return null;
		var card = el('article', 'rm-source-episode__claim');
		card.appendChild(txt('span', 'rm-source-episode__order', String(index + 1)));
		var body = el('div', 'rm-source-episode__claim-body');
		var heading = el('div', 'rm-source-episode__claim-heading');
		var badge = renderSourceEpisodeState(claim.state);
		if (badge) heading.appendChild(badge);
		heading.appendChild(txt('h3', '', claim.title || msg('main.chrome.explanation')));
		body.appendChild(heading);
		body.appendChild(txt('p', 'rm-source-episode__statement', claim.statement || ''));
		var sources = renderSourceEpisodeSources(claim.sources);
		if (sources) body.appendChild(sources);
		var limits = Array.isArray(claim.limits) ? claim.limits.filter(Boolean) : [];
		if (limits.length) {
			var boundary = el('div', 'rm-source-episode__limits');
			boundary.appendChild(txt('span', '', limits.length === 1 ? msg('main.boundary') : msg('main.boundaries')));
			var list = el('ul', '');
			limits.forEach(function (limit) { list.appendChild(txt('li', '', limit)); });
			boundary.appendChild(list);
			body.appendChild(boundary);
		}
		card.appendChild(body);
		return card;
	}

	function renderSourceEpisode(episode) {
		if (!episode || !episode.question || !Array.isArray(episode.claims) || !episode.claims.length) return null;
		var section = el('section', 'rm-source-episode');
		section.setAttribute('data-source-episode-id', episode.episode_id || '');
		var hero = el('div', 'rm-source-episode__hero');
		hero.appendChild(txt('div', 'rm-view-kicker', msg('main.how.this.works')));
		hero.appendChild(txt('h2', '', episode.question));
		var provenance = [episode.repository, episode.revision && String(episode.revision).slice(0, 12)]
			.filter(Boolean).join(' · ');
		if (provenance) hero.appendChild(txt('p', 'rm-source-episode__provenance', provenance));
		section.appendChild(hero);

		var claims = el('div', 'rm-source-episode__claims');
		episode.claims.forEach(function (claim, index) {
			var card = renderSourceEpisodeClaim(claim, index);
			if (card) claims.appendChild(card);
		});
		if (!claims.childNodes.length) return null;
		section.appendChild(claims);

		var uncertainties = Array.isArray(episode.uncertainties)
			? episode.uncertainties.filter(function (item) {
				return item && item.statement && sourceEpisodeStateLabel(item.state);
			})
			: [];
		if (uncertainties.length) {
			var frontier = el('section', 'rm-source-episode__uncertainties');
			frontier.appendChild(txt('h3', '', msg('main.what.remains.uncertain')));
			frontier.appendChild(txt('p', 'rm-source-episode__uncertainty-intro', msg('main.these.boundaries.stay.visible.because.they.change.how.the.explanation.should.be.used')));
			var list = el('div', 'rm-source-episode__uncertainty-list');
			uncertainties.forEach(function (item) {
				var card = el('article', 'rm-source-episode__uncertainty');
				var badge = renderSourceEpisodeState(item.state);
				if (badge) card.appendChild(badge);
				card.appendChild(txt('p', '', item.statement));
				var sources = renderSourceEpisodeSources(item.sources);
				if (sources) card.appendChild(sources);
				list.appendChild(card);
			});
			frontier.appendChild(list);
			section.appendChild(frontier);
		}
		return section;
	}
// repomap-source-episode:end

	function renderRepositoryBriefHero() {
		var brief = STUDY_MAP && STUDY_MAP.brief || {};
		if (!brief.what_it_is && !brief.problem && !brief.main_input &&
			!brief.central_responsibility && !brief.observable_result &&
			!(Array.isArray(brief.domain_terms) && brief.domain_terms.length)) return null;
		var hero = el('section', 'rm-overview-hero rm-purpose-hero');
		hero.appendChild(txt('div', 'rm-view-kicker', msg('main.repository.brief')));
		hero.appendChild(txt('h2', '', DATA.repo_name || msg('main.repository.overview')));
		if (brief.what_it_is) hero.appendChild(txt('p', 'rm-brief-lead', brief.what_it_is));
		if (brief.problem && brief.problem !== brief.what_it_is) hero.appendChild(txt('p', '', brief.problem));
		var briefFacts = el('div', 'rm-brief-facts');
		[
			[msg('main.main.input'), brief.main_input],
			[msg('main.central.responsibility'), brief.central_responsibility],
			[msg('main.observable.result'), brief.observable_result],
		].forEach(function (item) {
			if (!item[1]) return;
			var fact = el('div', 'rm-brief-fact');
			fact.appendChild(txt('span', '', item[0]));
			fact.appendChild(txt('strong', '', item[1]));
			briefFacts.appendChild(fact);
		});
		if (briefFacts.childNodes.length) hero.appendChild(briefFacts);
		var terms = Array.isArray(brief.domain_terms) ? brief.domain_terms : [];
		if (terms.length) {
			var termList = el('div', 'rm-brief-terms');
			terms.slice(0, 8).forEach(function (term) {
				var item = el('span', 'rm-brief-term');
				item.appendChild(txt('strong', '', term.term));
				item.appendChild(txt('span', '', term.meaning));
				termList.appendChild(item);
			});
			hero.appendChild(termList);
		}
		return hero;
	}

	function renderAtlasStudyDiagnostics() {
		var study = DATA.atlas_study;
		if (!study) return null;
		var stageCounts = [
			[msg('main.study.diagnostics.considered_spans'), study.considered_span_count],
			[msg('main.study.diagnostics.advertised_spans'), study.advertised_span_count],
			[msg('main.study.diagnostics.model_selected_spans'), study.model_selected_span_count],
			[msg('main.study.diagnostics.accepted_spans'), study.accepted_span_count],
		];
		var hasCounts = stageCounts.some(function (pair) { return Number(pair[1]) > 0; });
		var flags = [
			[msg('main.study.diagnostics.frontier_complete'), study.frontier_complete],
			[msg('main.study.diagnostics.selected_items_complete'), study.selected_items_complete],
			[msg('main.study.diagnostics.support_coverage_complete'), study.support_coverage_complete],
			[msg('main.study.diagnostics.portfolio_target_met'), study.portfolio_target_met],
		];
		var omissions = Array.isArray(study.omissions) ? study.omissions : [];
		if (!hasCounts && !omissions.length) return null;
		var panel = el('section', 'rm-workspace-section rm-study-diagnostics');
		panel.appendChild(renderViewHeading(
			msg('main.study'),
			msg('main.study.diagnostics'),
			msg('main.study.diagnostics_copy')
		));
		if (hasCounts) {
			var counts = el('div', 'rm-study-diagnostics-stages');
			stageCounts.forEach(function (pair) {
				if (!(Number(pair[1]) > 0)) return;
				var item = el('div', 'rm-study-diagnostics-stage');
				item.appendChild(txt('span', '', pair[0]));
				item.appendChild(txt('strong', '', String(pair[1])));
				counts.appendChild(item);
			});
			if (counts.childNodes.length) panel.appendChild(counts);
		}
		var flagList = el('ul', 'rm-study-diagnostics-flags');
		flags.forEach(function (pair) {
			var item = el('li', 'rm-study-diagnostics-flag');
			item.appendChild(txt('span', '', pair[0]));
			item.appendChild(txt('strong', '', pair[1] ? msg('main.study.diagnostics.yes') : msg('main.study.diagnostics.no')));
			flagList.appendChild(item);
		});
		panel.appendChild(flagList);
		if (omissions.length) {
			var omissionList = el('ul', 'rm-study-diagnostics-omissions');
			omissions.forEach(function (omission) {
				if (!omission || !omission.reason || !(Number(omission.count) > 0)) return;
				var item = el('li', 'rm-study-diagnostics-omission');
				item.appendChild(txt('p', 'rm-study-diagnostics-omission-sentence', msg('main.study.frontier.omission_sentence')));
				var showAll = txt('button', 'rm-secondary-action rm-study-diagnostics-show-all', msg('main.study.frontier.show_all', { count: omission.count }));
				showAll.type = 'button';
				showAll.onclick = function () { revealAtlasStudyLocalGroup(); };
				item.appendChild(showAll);
				omissionList.appendChild(item);
			});
			if (omissionList.childNodes.length) {
				var omissionHeading = el('h3', 'rm-study-diagnostics-subheading');
				omissionHeading.appendChild(document.createTextNode(msg('main.study.diagnostics.omissions')));
				panel.appendChild(omissionHeading);
				panel.appendChild(omissionList);
			}
		}
		// Decision 233 (Archive 9, AREA COVERAGE): exact missing-core-area
		// diagnostic — accepted principal Architecture components with no
		// published theme reading over their member paths. Never filler.
		if (Number(study.missing_core_area_count) > 0) {
			var missingList = el('ul', 'rm-study-diagnostics-missing-core');
			(Array.isArray(study.missing_core_areas) ? study.missing_core_areas : []).forEach(function (name) {
				var item = el('li', 'rm-study-diagnostics-missing-core-item');
				item.appendChild(document.createTextNode(msg('main.study.diagnostics.missing_core_area', { name: name })));
				missingList.appendChild(item);
			});
			var missingHeading = el('h3', 'rm-study-diagnostics-subheading');
			missingHeading.appendChild(document.createTextNode(msg('main.study.diagnostics.missing_core_heading', { count: study.missing_core_area_count })));
			panel.appendChild(missingHeading);
			panel.appendChild(missingList);
		}
		return panel;
	}

	// D213: provider-free browse of the complete considered Study question set
	// over the two semantic stages. Re-based four-stage membership:
	// published / scout_anchored / seed_advertised / considered. The failed
	// state renders the distinct neutral "Local question" label.
	function atlasStudyBrowseStageLabel(stage, failedState) {
		if (failedState) return msg('main.study.frontier.stage_local_failed');
		switch (String(stage || '')) {
			case 'published': return msg('main.study.frontier.stage_published');
			case 'scout_anchored': return msg('main.study.frontier.stage_scout_anchored');
			case 'seed_advertised': return msg('main.study.frontier.stage_seed_advertised');
			case 'considered': return msg('main.study.frontier.stage_considered');
			default: return msg('main.study.frontier.stage_local_failed');
		}
	}

	function atlasStudyBrowseSourceLocation(row) {
		if (!row || !row.source || !row.source.path) return null;
		var line = Number(row.source.line) || 0;
		return { path: row.source.path, line: line, column: Number(row.source.column) || 0, end_line: line };
	}

	function renderAtlasStudyBrowseRowSource(row) {
		var location = atlasStudyBrowseSourceLocation(row);
		if (!location || !sourceLocationActionAvailable(location)) return null;
		return sourceActionElement(
			row.question || row.title || '',
			'rm-study-browse-row__question rm-source-action-link',
			location,
			location.end_line,
			function () { openSourceLocation(location); }
		);
	}

	function renderAtlasStudyBrowseRow(row, failedState, statusState) {
		var item = el('li', 'rm-study-browse-row');
		var stageLabel = atlasStudyBrowseStageLabel(row.stage, failedState);
		if (!failedState && String(row.stage) === 'published' && row.theme_refs && row.theme_refs.length) {
			var badge = txt('button', 'rm-study-browse-row__stage rm-study-browse-row__stage-published', stageLabel);
			badge.type = 'button';
			var card = themeCardByOrdinal(row.theme_refs[0]);
			badge.title = card ? msg('main.study.frontier.open_theme', { count: card.ordinal }) : stageLabel;
			badge.onclick = function () { openThemeCard(row.theme_refs[0]); };
			item.appendChild(badge);
		} else {
			item.appendChild(txt('span', 'rm-study-browse-row__stage', stageLabel));
		}
		var question = renderAtlasStudyBrowseRowSource(row);
		if (question) {
			var sourceLocation = atlasStudyBrowseSourceLocation(row);
			var locationHint = sourceLocation ? formatCodeLocation(sourceLocation) : '';
			if (row.endpoint && row.endpoint.path && row.endpoint.path !== (row.source && row.source.path)) {
				locationHint = locationHint + ' → ' + formatCodeLocation({ path: row.endpoint.path, line: Number(row.endpoint.line) || 0, column: Number(row.endpoint.column) || 0 });
			}
			if (locationHint) question.title = locationHint;
			item.appendChild(question);
		} else {
			item.appendChild(txt('span', 'rm-study-browse-row__unavailable', msg('main.study.frontier.source_unavailable')));
		}
		item.appendChild(txt('code', 'rm-study-browse-row__title', row.title || ''));
		return item;
	}

	function atlasStudyBrowseRepresentativeCount() {
		var study = DATA.atlas_study;
		var total = 0;
		(study && Array.isArray(study.omissions) ? study.omissions : []).forEach(function (omission) {
			if (omission && Number(omission.representative_count) > 0) total += Number(omission.representative_count);
		});
		return total;
	}

	function revealAtlasStudyLocalGroup() {
		var group = document.querySelector('.rm-study-browse-group--local');
		if (!group) return;
		group.classList.remove('rm-study-browse-group--collapsed');
		group.scrollIntoView({ behavior: 'smooth', block: 'start' });
	}

	function renderAtlasStudyBrowse() {
		var study = DATA.atlas_study;
		if (!study || !study.frontier_browse) return null;
		var browse = study.frontier_browse;
		var failedState = study.state === 'failed';
		var section = el('section', 'rm-workspace-section rm-study-frontier-browse');
		section.appendChild(renderViewHeading(
			msg('main.study'),
			msg('main.study.frontier.browse_title'),
			msg('main.study.frontier.browse_copy')
		));
		if (Number(browse.shown) < Number(browse.total)) {
			section.appendChild(txt('p', 'rm-study-browse-ceiling', msg('main.study.frontier.ceiling_note', { shown: browse.shown, total: browse.total })));
		}
		var representativeCount = failedState ? 0 : atlasStudyBrowseRepresentativeCount();
		var list = el('ul', 'rm-study-browse-list');
		var currentStage = null;
		var group = null;
		var groupRows = null;
		var localRows = null;
		browse.spans.forEach(function (row) {
			var stageKey = failedState ? 'failed' : String(row.stage || '');
			if (stageKey !== currentStage) {
				currentStage = stageKey;
				group = el('li', 'rm-study-browse-group' + (stageKey === 'considered' ? ' rm-study-browse-group--local' : ''));
				groupRows = el('ul', 'rm-study-browse-group__rows');
				group.appendChild(txt('h3', 'rm-study-browse-group__heading', atlasStudyBrowseStageLabel(row.stage, failedState)));
				group.appendChild(groupRows);
				list.appendChild(group);
				localRows = stageKey === 'considered' ? [] : null;
			}
			var item = renderAtlasStudyBrowseRow(row, failedState, study.state);
			if (localRows) localRows.push(item);
			groupRows.appendChild(item);
		});
		if (localRows && localRows.length > representativeCount) {
			var showAll = txt('button', 'rm-secondary-action rm-study-browse-show-all', msg('main.study.frontier.show_all', { count: localRows.length }));
			showAll.type = 'button';
			showAll.onclick = function () { revealAtlasStudyLocalGroup(); };
			group.appendChild(showAll);
			group.classList.add('rm-study-browse-group--collapsed');
			localRows.slice(representativeCount).forEach(function (item) {
				item.classList.add('rm-study-browse-row--beyond');
			});
		}
		section.appendChild(list);
		return section;
	}

	function renderAtlasStudyFailedBrowse(root) {
		var study = DATA.atlas_study;
		if (!study || study.state !== 'failed' || !study.frontier_browse || !root) return;
		var browse = renderAtlasStudyBrowse();
		if (browse) root.appendChild(browse);
	}

	function renderStudyMapOverview(root, includeBrief) {

// repomap-source-episode:start
		var episode = renderSourceEpisode(SOURCE_EPISODE);
		if (episode) root.appendChild(episode);
// repomap-source-episode:end

		if (includeBrief !== false) {
			var briefHero = renderRepositoryBriefHero();
			if (briefHero) root.appendChild(briefHero);
		}

		if (COMPLETE_STUDY_DIRECTIONS.length) {
			var studySection = el('section', 'rm-workspace-section rm-study-map-section');
			studySection.appendChild(renderViewHeading(msg('main.what.to.study'), msg('main.a.useful.path.through.the.repository'), msg('main.choose.a.question.and.begin.with.concrete.source.anchors.the.order.is.for.learning.not.an.execution.trace')));
			var directionList = el('div', 'rm-study-direction-list');
			COMPLETE_STUDY_DIRECTIONS.forEach(function (direction, index) {
				directionList.appendChild(renderStudyDirectionCard(direction, index));
			});
			studySection.appendChild(directionList);
			root.appendChild(studySection);
		}

		renderOperationsOverview(root);

	}

	function studyReadingLocation(reading) {
		if (!reading) return null;
		// D213 theme readings carry a flat exact location ({path, line}).
		if (reading.path) {
			return {
				path: reading.path,
				line: Number(reading.line) || 0,
				column: 0,
				end_line: 0,
			};
		}
		var location = reading.location || sourceSnippetLocation(reading.source);
		if (!location || !location.path) return null;
		return {
			path: location.path,
			line: Number(location.line) || Number(reading.source && reading.source.start_line) || 0,
			column: Number(location.column) || 0,
			end_line: Number(reading.source && reading.source.end_line) || 0,
		};
	}

	// sourceKindFor derives the closed source kind for one reading/source
	// action deterministically from exact local fields (Decision 218 B).
	// It never infers a stronger kind than the evidence supports: a receiver
	// symbol is a method, a bare symbol with a line is a function, an empty
	// symbol over a path is a file, and explicit package markers are
	// packages. call_site/boundary are only used when the projection says so.
	function sourceKindFor(symbol, path) {
		var sym = String(symbol || '').trim();
		var filePath = String(path || '');
		if (!sym) {
			if (/(^|\/)\w+\.\w+$/.test(filePath)) return 'file';
			return 'package';
		}
		if (/\([^)]*\)\.[A-Za-z_]/.test(sym)) return 'method';
		if (/^type\s+/.test(sym)) return 'type';
		if (/^func\s+/.test(sym) || /\([^)]*\)/.test(sym)) return 'function';
		return 'function';
	}

	function sourceKindLabel(kind) {
		switch (String(kind || '')) {
		case 'method': return msg('main.source.kind.method');
		case 'type': return msg('main.source.kind.type');
		case 'call_site': return msg('main.source.kind.call_site');
		case 'package': return msg('main.source.kind.package');
		case 'file': return msg('main.source.kind.file');
		case 'document': return msg('main.source.kind.document');
		case 'boundary': return msg('main.source.kind.boundary');
		default: return msg('main.source.kind.function');
		}
	}

	function renderStudySourceAction(reading, cls, includeLocation) {
		var location = studyReadingLocation(reading);
		// Decision 222: the reading shows the bare symbol name only — the
		// package is already stated by the location (path:line) right next
		// to it. Full qualified paths are noise on the card.
		var symbol = bareSourceSymbol(String(reading && reading.symbol || ''));
		// Decision 230 D1: embedded snippets are resolved by exact
		// location too (atlas readings do not carry a .source field), so
		// the source action works in embedded reports without a server or
		// GitHub/GitLab jump — same contract as Overview object cards.
		var embedded = reading && sourceSnippetAvailable(reading.source) ? reading.source : (embeddedSourceForLocation(location) || null);
		if (!location || !symbol || (!embedded && !sourceLocationActionAvailable(location))) return null;
		// Decision 230 D1 priority: a pinned static link (GitHub/GitLab,
		// target=_blank) is the primary source action; the embedded
		// snippet drawer button is the fallback when no static or server
		// jump exists.
		var action;
		if (embedded && sourceSnippetHasCode(embedded) && !staticSourceMode() && !(serverMode() && currentRunID() && SOURCE_IDS[location.path])) {
			action = el('button', (cls || '') + ' rm-source-action-link');
			action.type = 'button';
			action.onclick = function () {
				openSourceSnippet(embedded, location, false, { drawerFirst: true });
			};
			// The embedded button carries the same visible label the
			// static/server action would: the bare symbol (or symbol +
			// location when includeLocation is set below).
			action.appendChild(txt('', '', symbol));
		} else {
			action = sourceActionElement(
				symbol,
				(cls || '') + ' rm-source-action-link',
				location,
				location.end_line,
				function () {
					openSourceLocation(location);
				}
			);
		}
		if (!action) return null;
		action.setAttribute('aria-label', symbol + ' · ' + formatCodeLocation(location));
		if (includeLocation) {
			action.textContent = '';
			action.appendChild(txt('strong', '', symbol));
			action.appendChild(txt('code', '', formatCodeLocation(location)));
		}
		return action;
	}

	// bareSourceSymbol reduces a fully-qualified symbol to its bare name:
	// the last dot-segment (keeping a (*Type) receiver prefix), because the
	// package is already shown by the location text next to the symbol.
	function bareSourceSymbol(symbol) {
		var value = String(symbol || '').trim();
		if (!value) return '';
		var last = value;
		var receiver = '';
		var open = value.indexOf('(');
		if (open >= 0) {
			var close = value.indexOf(')', open);
			if (close >= 0) {
				// A qualified Go method is commonly encoded as
				// "module/package.(*Type).Method". Keep the receiver, but
				// discard the package prefix just as we do for functions.
				receiver = value.slice(open, close + 1);
				last = value.slice(close + 1);
			}
		}
		if (last) {
			var dot = last.lastIndexOf('.');
			if (dot >= 0 && dot + 1 < last.length) last = last.slice(dot + 1);
		}
		return (receiver ? receiver + '.' : '') + last;
	}

	function renderStudyReadingAnchor(reading, index, showRole) {
		var location = studyReadingLocation(reading);
		if (!location) return null;
		var kind = sourceKindFor(reading && reading.symbol, reading && reading.path);
		var card = el('article', 'rm-study-reading-anchor');
		card.appendChild(txt('span', 'rm-study-reading-anchor__order', String(index + 1)));
		var copy = el('div', 'rm-study-reading-anchor__copy');
		var row = el('div', 'rm-study-reading-anchor__row');
		// Decision 218 (B): typed source rows — symbol, kind · path:line, and
		// explanation are separate DOM nodes with visible separation; a
		// package/file entry is explicitly labeled and never reads as a
		// function. The row renders even when no exact saved source is
		// available for this run: the reading is still visible and typed,
		// just without a click action.
		var symbol = String(reading && reading.symbol || '');
		var bare = bareSourceSymbol(symbol);
		// Decision 230 D1: symbol AND path:line form ONE coherent source
		// action (never a link plus an inert location beside it).
		var open = renderStudySourceAction(reading, 'rm-study-reading-anchor__open', true);
		if (open) {
			row.appendChild(open);
		} else {
			// Decision 222/230 D1: the plain fallback shows the bare
			// symbol name AND the exact location as a neutral non-link
			// row (never a dead button, never a silently missing path).
			var plain = el('span', 'rm-study-reading-anchor__open rm-study-reading-anchor__open--plain');
			plain.appendChild(txt('', '', bare));
			plain.appendChild(txt('code', 'rm-study-reading-anchor__location', formatCodeLocation(location)));
			row.appendChild(plain);
		}
		var meta = el('div', 'rm-study-reading-anchor__meta');
		meta.appendChild(txt('span', 'rm-study-reading-anchor__kind', sourceKindLabel(kind)));
		// Decision 229 D6: the expanded reading shows its support role
		// (direct/supporting) next to the typed kind.
		if (showRole && reading.role === 'direct') meta.appendChild(txt('span', 'rm-study-theme-card__reading-role', msg('main.study.theme.reading.role.direct')));
		else if (showRole && reading.role === 'supporting') meta.appendChild(txt('span', 'rm-study-theme-card__reading-role', msg('main.study.theme.reading.role.supporting')));
		// Decision 230 D1: path:line lives inside the single source
		// action above; it is not duplicated as an inert code node here.
		row.appendChild(meta);
		copy.appendChild(row);
		if (reading.what_to_look_for) copy.appendChild(txt('p', 'rm-study-reading-anchor__explain', reading.what_to_look_for));
		else if (reading.supported_observation) copy.appendChild(txt('p', 'rm-study-theme-card__reading-explain', reading.supported_observation));
		card.appendChild(copy);
		return card;
	}

	function operationOrderingCopy(pavedPath) {
		if (!pavedPath) return '';
		if (pavedPath.ordering_basis === 'documented_procedure') {
			return msg('main.operation_order.documented_procedure');
		}
		if (pavedPath.ordering_basis === 'script_sequence') {
			return msg('main.operation_order.script_sequence');
		}
		return msg('main.operation_order.practical_path');
	}

	function appendOperationalReferences(root, title, copy, references) {
		references = Array.isArray(references) ? references.filter(Boolean) : [];
		if (!references.length) return;
		var section = el('section', 'rm-workspace-section rm-operation-references');
		section.appendChild(renderViewHeading('', title, copy));
			references.forEach(function (reference) {
			if (!reference || !sourceSnippetAvailable(reference.source)) return;
			section.appendChild(renderSourceSnippetCard(reference.source, {
				roleLabel: reference.label || title,
				reason: reference.label || '',
				location: reference.location || sourceSnippetLocation(reference.source),
				redacted: !!reference.redacted,
			}));
		});
		root.appendChild(section);
	}

	function renderOperationAction(action, index) {
		var section = el('section', 'rm-operation-action');
		var heading = el('div', 'rm-operation-action__heading');
		heading.appendChild(txt('span', 'rm-operation-action__order', String(index + 1)));
		var copy = el('div', 'rm-operation-action__copy');
		copy.appendChild(txt('strong', '', action.instruction || msg('main.chrome.use.this.repository.reference')));
		if (action.command) {
			var literalRow = el('div', 'rm-operation-literal');
			literalRow.appendChild(txt('code', '', action.command));
			if (action.copy_text) {
				var copyButton = txt('button', 'rm-secondary-action', msg('main.copy.command'));
				copyButton.type = 'button';
				copyButton.onclick = function () { copyText(action.copy_text); };
				literalRow.appendChild(copyButton);
			}
			copy.appendChild(literalRow);
		}
		if (action.endpoint) {
			var endpointRow = el('div', 'rm-operation-literal');
			endpointRow.appendChild(txt('code', '', action.endpoint));
			copy.appendChild(endpointRow);
		}
		heading.appendChild(copy);
		section.appendChild(heading);
		if (action.reference && sourceSnippetAvailable(action.reference.source)) {
			section.appendChild(renderSourceSnippetCard(action.reference.source, {
				primary: index === 0,
				roleLabel: action.reference.label || msg('main.repository_source'),
				reason: action.instruction || '',
				location: action.reference.location || sourceSnippetLocation(action.reference.source),
				redacted: !!action.reference.redacted,
			}));
		}
		return section;
	}

	function operationalResultLabel(kind) {
		if (kind === 'command_output') return msg('main.operational_result.documented_output');
		if (kind === 'generated_artifact') return msg('main.operational_result.generated_artifact');
		return '';
	}

	function renderOperationalResults(pavedPath) {
		var actionCount = Array.isArray(pavedPath && pavedPath.actions) ? pavedPath.actions.length : 0;
		var results = Array.isArray(pavedPath && pavedPath.expected_results)
			? pavedPath.expected_results.filter(function (result) {
				var afterAction = Number(result && result.after_action) || 0;
				return !!(result && operationalResultLabel(result.kind) && String(result.value || '').trim() &&
					afterAction > 0 && afterAction <= actionCount && result.reference &&
					!result.reference.redacted && sourceSnippetAvailable(result.reference.source));
			})
			: [];
		if (!results.length) return null;

		var section = el('section', 'rm-workspace-section rm-operation-results');
		section.appendChild(renderViewHeading('', msg('main.expected.result'), msg('main.exact.output.or.generated.paths.retained.from.this.repository')));
		var list = el('div', 'rm-operation-result-list');
		results.forEach(function (result) {
			var card = el('article', 'rm-operation-result');
			var meta = el('div', 'rm-operation-result__meta');
			meta.appendChild(txt('span', 'rm-operation-result__action', msg('main.operation.after_action', {
				action: result.after_action,
			})));
			meta.appendChild(txt('span', 'rm-operation-result__kind', operationalResultLabel(result.kind)));
			card.appendChild(meta);
			if (result.kind === 'command_output') {
				var output = el('pre', 'rm-operation-result__value rm-operation-result__value--output');
				output.appendChild(txt('code', '', result.value));
				card.appendChild(output);
			} else {
				card.appendChild(txt('code', 'rm-operation-result__value', result.value));
			}
			var source = sourceActionElement(
				staticSourceMode() ? staticSourceOpenLabel() : msg('main.show.source'),
				'rm-quiet-action rm-source-action-link',
				result.reference.location || sourceSnippetLocation(result.reference.source),
				Number(result.reference.source.end_line) || 0,
				function () {
					openSourceSnippet(result.reference.source, result.reference.location, false);
				}
			);
			if (source) card.appendChild(source);
			list.appendChild(card);
		});
		section.appendChild(list);
		return section;
	}

	function renderOperateDetailWorkspace() {
		var root = document.getElementById('rm-operate-detail');
		if (!root) return;
		root.replaceChildren();
		var pavedPath = pavedPathByID(workspaceState.operationID);
		if (!pavedPath) return;
		var back = txt('button', 'rm-secondary-action rm-study-back', msg('main.repository.overview.2'));
		back.type = 'button';
		back.onclick = function () { navigateWorkspace('map', { replace: true }); };
		root.appendChild(back);
		root.appendChild(renderViewHeading(msg('main.how.to.run.and.verify'), pavedPath.title, pavedPath.goal));
		root.appendChild(txt('p', 'rm-operation-order-note', operationOrderingCopy(pavedPath)));

		appendOperationalReferences(
			root,
			msg('main.operate.before_start'),
			msg('main.operate.prerequisites_copy'),
			pavedPath.prerequisites
		);

		var actions = Array.isArray(pavedPath.actions) ? pavedPath.actions.filter(Boolean) : [];
		if (actions.length) {
			var actionSection = el('section', 'rm-workspace-section');
			actionSection.appendChild(renderViewHeading('', msg('main.actions'), msg('main.each.action.keeps.its.exact.command.or.endpoint.beside.the.repository.source.that.defines.it')));
			var actionList = el('div', 'rm-operation-action-list');
			actions.forEach(function (action, index) {
				actionList.appendChild(renderOperationAction(action, index));
			});
			actionSection.appendChild(actionList);
			root.appendChild(actionSection);
		}
		var expectedResults = renderOperationalResults(pavedPath);
		if (expectedResults) root.appendChild(expectedResults);

		appendOperationalReferences(
			root,
			msg('main.operate.what_to_verify'),
			msg('main.operate.verify_copy'),
			pavedPath.expected
		);
		appendOperationalReferences(
			root,
			msg('main.operate.if_not_working'),
			msg('main.operate.troubleshooting_copy'),
			pavedPath.troubleshooting
		);

		var relatedIDs = Array.isArray(pavedPath.related_study_direction_ids)
			? pavedPath.related_study_direction_ids : [];
		var relatedDirections = relatedIDs.map(studyDirectionByID).filter(Boolean);
		if (relatedDirections.length) {
			var related = el('section', 'rm-workspace-section rm-operation-related');
			related.appendChild(renderViewHeading(msg('main.study.the.implementation'), msg('main.related.reading.paths'), msg('main.follow.the.code.behind.this.operation.without.losing.this.page.from.browser.history')));
			var buttons = el('div', 'rm-overview-actions');
			relatedDirections.forEach(function (direction) {
				var button = txt('button', 'rm-secondary-action', direction.question);
				button.type = 'button';
				button.onclick = function () { openStudyDirection(direction.id); };
				buttons.appendChild(button);
			});
			related.appendChild(buttons);
			root.appendChild(related);
		}
	}

	var TASK_LENS_ENUM_MESSAGE_IDS = {
		task_kind: {
			bug: 'main.task_lens.enum.task_kind.bug',
			feature: 'main.task_lens.enum.task_kind.feature',
			extension: 'main.task_lens.enum.task_kind.extension',
			configuration: 'main.task_lens.enum.task_kind.configuration',
			operational: 'main.task_lens.enum.task_kind.operational',
			compatibility: 'main.task_lens.enum.task_kind.compatibility',
			unknown: 'main.task_lens.enum.task_kind.unknown',
		},
		locality: {
			local_exact: 'main.task_lens.enum.locality.local_exact',
			bounded_cross_file: 'main.task_lens.enum.locality.bounded_cross_file',
			extension_contribution: 'main.task_lens.enum.locality.extension_contribution',
			broad_dynamic: 'main.task_lens.enum.locality.broad_dynamic',
		},
		anchor_role: {
			symptom_site: 'main.task_lens.enum.anchor_role.symptom_site',
			public_or_cli_entry: 'main.task_lens.enum.anchor_role.public_or_cli_entry',
			state_owner: 'main.task_lens.enum.anchor_role.state_owner',
			state_mutation: 'main.task_lens.enum.anchor_role.state_mutation',
			configuration_source: 'main.task_lens.enum.anchor_role.configuration_source',
			configuration_copy: 'main.task_lens.enum.anchor_role.configuration_copy',
			error_creation: 'main.task_lens.enum.anchor_role.error_creation',
			error_mapping: 'main.task_lens.enum.anchor_role.error_mapping',
			integration_boundary: 'main.task_lens.enum.anchor_role.integration_boundary',
			representative_implementation: 'main.task_lens.enum.anchor_role.representative_implementation',
			generated_output: 'main.task_lens.enum.anchor_role.generated_output',
			reproduction_anchor: 'main.task_lens.enum.anchor_role.reproduction_anchor',
			verification_anchor: 'main.task_lens.enum.anchor_role.verification_anchor',
			documentation_contract: 'main.task_lens.enum.anchor_role.documentation_contract',
		},
		support_type: {
			locally_observed: 'main.task_lens.enum.support_type.locally_observed',
			document_supported: 'main.task_lens.enum.support_type.document_supported',
			model_hypothesis: 'main.task_lens.enum.support_type.model_hypothesis',
			unresolved: 'main.task_lens.enum.support_type.unresolved',
		},
		hypothesis_status: {
			supported: 'main.task_lens.enum.hypothesis_status.supported',
			plausible: 'main.task_lens.enum.hypothesis_status.plausible',
			unresolved: 'main.task_lens.enum.hypothesis_status.unresolved',
		},
		guidance_authority: {
			task_provided: 'main.task_lens.enum.guidance_authority.task_provided',
			repository_document: 'main.task_lens.enum.guidance_authority.repository_document',
			repository_test_or_example: 'main.task_lens.enum.guidance_authority.repository_test_or_example',
			repository_observation: 'main.task_lens.enum.guidance_authority.repository_observation',
			missing_evidence: 'main.task_lens.enum.guidance_authority.missing_evidence',
		},
		probe_action: {
			inspect_symbol: 'main.task_lens.enum.probe_action.inspect_symbol',
			resolve_reference: 'main.task_lens.enum.probe_action.resolve_reference',
			compare_config_copies: 'main.task_lens.enum.probe_action.compare_config_copies',
			inspect_fixture: 'main.task_lens.enum.probe_action.inspect_fixture',
			inspect_sibling_implementation: 'main.task_lens.enum.probe_action.inspect_sibling_implementation',
			search_task_terms: 'main.task_lens.enum.probe_action.search_task_terms',
		},
		relation_kind: {
			direct_call: 'main.task_lens.enum.relation_kind.direct_call',
			field_copy: 'main.task_lens.enum.relation_kind.field_copy',
			field_read: 'main.task_lens.enum.relation_kind.field_read',
			field_write: 'main.task_lens.enum.relation_kind.field_write',
			error_created: 'main.task_lens.enum.relation_kind.error_created',
			error_mapped: 'main.task_lens.enum.relation_kind.error_mapped',
			error_exposed: 'main.task_lens.enum.relation_kind.error_exposed',
			value_transformed: 'main.task_lens.enum.relation_kind.value_transformed',
			type_name_generated: 'main.task_lens.enum.relation_kind.type_name_generated',
			config_applied: 'main.task_lens.enum.relation_kind.config_applied',
			script_invokes: 'main.task_lens.enum.relation_kind.script_invokes',
			test_exercises: 'main.task_lens.enum.relation_kind.test_exercises',
			fixture_records: 'main.task_lens.enum.relation_kind.fixture_records',
			documented_uses: 'main.task_lens.enum.relation_kind.documented_uses',
			shared_state_alias: 'main.task_lens.enum.relation_kind.shared_state_alias',
			scope_unknown: 'main.task_lens.enum.relation_kind.scope_unknown',
			document_names_endpoints: 'main.task_lens.enum.relation_kind.document_names_endpoints',
			model_hypothesis: 'main.task_lens.enum.relation_kind.model_hypothesis',
			unresolved_relation: 'main.task_lens.enum.relation_kind.unresolved_relation',
		},
		stage: {
			architecture_synthesis: 'main.task_lens.enum.stage.architecture_synthesis',
			generic_orientation: 'main.task_lens.enum.stage.generic_orientation',
			guided_tour: 'main.task_lens.enum.stage.guided_tour',
			mechanism_opportunity: 'main.task_lens.enum.stage.mechanism_opportunity',
			paved_paths: 'main.task_lens.enum.stage.paved_paths',
			repository_study_map: 'main.task_lens.enum.stage.repository_study_map',
			runtime_surface_discovery: 'main.task_lens.enum.stage.runtime_surface_discovery',
		},
	};

	function taskLensEnumLabel(family, value) {
		var normalized = String(value || '');
		var familyMessages = TASK_LENS_ENUM_MESSAGE_IDS[family] || {};
		var messageID = familyMessages[normalized];
		return messageID
			? msg(messageID)
			: msg('main.task_lens.enum.unknown', { value: normalized || '—' });
	}

	function taskLensAnchor(index) {
		var anchors = TASK_INVESTIGATION && Array.isArray(TASK_INVESTIGATION.anchors)
			? TASK_INVESTIGATION.anchors
			: [];
		index = Number(index);
		return Number.isInteger(index) && index >= 0 && index < anchors.length ? anchors[index] : null;
	}

	function taskLensAnchorTitle(anchor) {
		if (!anchor) return msg('main.repository_anchor');
		return String(anchor.symbol || anchor.section || anchor.path || msg('main.repository_anchor'));
	}

	function taskLensBadge(value, label) {
		var normalized = String(value || 'unresolved');
		return txt(
			'span',
			'rm-task-support rm-task-support--' + normalized.replace(/[^a-z0-9_-]/gi, '-'),
			label
		);
	}

	function taskLensEnumBadge(family, value) {
		return taskLensBadge(value, taskLensEnumLabel(family, value));
	}

	function taskLensAnchorElementID(index) {
		return 'rm-task-anchor-' + (Number(index) + 1);
	}

	function scrollToTaskLensAnchor(index) {
		var anchor = taskLensAnchor(index);
		var target = anchor && document.getElementById(taskLensAnchorElementID(index));
		if (!target) return;
		if (typeof target.scrollIntoView === 'function') {
			target.scrollIntoView({ behavior: 'smooth', block: 'center' });
		}
		if (typeof target.focus === 'function') target.focus({ preventScroll: true });
	}

	function taskLensCitations(indexes, label) {
		indexes = Array.isArray(indexes) ? indexes : [];
		var valid = indexes.map(function (index) {
			index = Number(index);
			return taskLensAnchor(index) ? index : null;
		}).filter(function (index) { return index !== null; });
		if (!valid.length) return null;
		var citations = el('div', 'rm-task-citations');
		citations.appendChild(txt('span', 'rm-task-citations__label', label || msg('main.chrome.evidence')));
		valid.forEach(function (index) {
			var anchor = taskLensAnchor(index);
			var button = txt(
				'button',
				'rm-task-citation',
				msg('main.task.anchor_citation', {
					index: index + 1,
					title: taskLensAnchorTitle(anchor),
				})
			);
			button.type = 'button';
			button.onclick = function () { scrollToTaskLensAnchor(index); };
			citations.appendChild(button);
		});
		return citations;
	}

	function appendTaskLensCitations(root, indexes, label) {
		var citations = taskLensCitations(indexes, label);
		if (citations) root.appendChild(citations);
	}

	function taskLensTermGroup(label, values, className) {
		values = Array.isArray(values) ? values.filter(Boolean) : [];
		if (!values.length) return null;
		var group = el('div', 'rm-task-term-group ' + className);
		group.appendChild(txt('span', 'rm-task-term-label', label));
		var terms = el('div', 'rm-task-term-list');
		values.forEach(function (value) {
			terms.appendChild(txt('code', 'rm-task-term', value));
		});
		group.appendChild(terms);
		return group;
	}

	function taskLensJoinConnector(kind) {
		return kind === 'direct_call' || kind === 'direct_call_expression' ? ' → ' : ' ↔ ';
	}

	function taskLensGuidanceList(values) {
		var list = el('ol', 'rm-task-guidance-list');
		(Array.isArray(values) ? values : []).forEach(function (guidance) {
			if (!guidance || !guidance.text) return;
			var item = el('li', 'rm-task-guidance');
			item.appendChild(txt('p', '', guidance.text));
			item.appendChild(taskLensEnumBadge('guidance_authority', guidance.authority));
			appendTaskLensCitations(item, guidance.support_anchor_indexes, msg('main.chrome.evidence'));
			list.appendChild(item);
		});
		return list;
	}

	function renderTaskInvestigationWorkspace() {
		var root = document.getElementById('rm-task-investigation');
		if (!root) return;
		root.replaceChildren();
		if (!TASK_INVESTIGATION) return;

		var interpretation = TASK_INVESTIGATION.interpretation || {};
		var hero = el('section', 'rm-task-hero');
		hero.appendChild(renderViewHeading(
			msg('main.chrome.task.investigation'),
			interpretation.restatement || msg('main.repository_task'),
			interpretation.observable_or_outcome || ''
		));
		var taskText = linkified('p', 'rm-task-original', TASK_INVESTIGATION.task || '');
		if (taskText.textContent || taskText.childNodes && taskText.childNodes.length) hero.appendChild(taskText);
		var classification = el('div', 'rm-task-classification');
		classification.appendChild(taskLensEnumBadge('task_kind', interpretation.task_kind));
		classification.appendChild(taskLensEnumBadge('locality', TASK_INVESTIGATION.locality));
		classification.appendChild(taskLensBadge(
			TASK_INVESTIGATION.sufficient ? 'sufficient' : 'partial',
			TASK_INVESTIGATION.sufficient
				? msg('main.task_lens.bounded_evidence_sufficient')
				: msg('main.task_lens.partial_bounded_evidence')
		));
		hero.appendChild(classification);
		var termBoundary = el('div', 'rm-task-term-boundary');
		var foundTerms = taskLensTermGroup(
				msg('main.task_lens.found_in_repository_evidence'),
			interpretation.repository_terms_found,
			'rm-task-term-group--found'
		);
		var userOnlyTerms = taskLensTermGroup(
				msg('main.task_lens.task_provided_only'),
			interpretation.user_provided_only_terms,
			'rm-task-term-group--task'
		);
		if (foundTerms) termBoundary.appendChild(foundTerms);
		if (userOnlyTerms) termBoundary.appendChild(userOnlyTerms);
		if (termBoundary.childNodes.length) hero.appendChild(termBoundary);
		root.appendChild(hero);

		var warnings = Array.isArray(TASK_INVESTIGATION.warnings)
			? TASK_INVESTIGATION.warnings.filter(Boolean)
			: [];
		var warningPresentations = Array.isArray(TASK_INVESTIGATION.presentation_warnings)
			? TASK_INVESTIGATION.presentation_warnings
			: [];
		if (warnings.length) {
			var warningSection = el('section', 'rm-workspace-section rm-task-warnings');
			warningSection.appendChild(renderViewHeading(
					msg('main.task_lens.evidence_cautions'),
					msg('main.task_lens.limits_to_keep_in_view'),
					msg('main.task_lens.cautions_copy')
			));
			var warningList = el('ul', 'rm-task-warning-list');
			warnings.forEach(function (warning, warningIndex) {
				var presentation = warningPresentations[warningIndex] || null;
				var renderedWarning = warning;
				if (presentation && UI.hasMessage(presentation.message_id)) {
					var params = {};
					if (Number(presentation.index) > 0) params.index = Number(presentation.index);
					renderedWarning = msg(presentation.message_id, params);
				}
				warningList.appendChild(txt('li', '', renderedWarning));
			});
			warningSection.appendChild(warningList);
			root.appendChild(warningSection);
		}

		var areas = Array.isArray(TASK_INVESTIGATION.likely_areas) ? TASK_INVESTIGATION.likely_areas : [];
		if (areas.length) {
			var areaSection = el('section', 'rm-workspace-section rm-task-areas');
			areaSection.appendChild(renderViewHeading(
					msg('main.task_lens.likely_areas'),
					msg('main.task_lens.where_evidence_points'),
					msg('main.task_lens.areas_copy')
			));
			areas.forEach(function (area) {
				var areaCard = el('article', 'rm-task-area');
				areaCard.appendChild(txt('h3', '', area.label || msg('main.chrome.relevant.code.area')));
				if (area.why) areaCard.appendChild(txt('p', '', area.why));
				var areaAnchors = el('div', 'rm-task-area-anchors');
				(area.anchor_indexes || []).forEach(function (anchorIndex) {
					var anchor = taskLensAnchor(anchorIndex);
					if (!anchor) return;
					areaAnchors.appendChild(txt(
						'code',
						'rm-task-area-anchor',
						taskLensAnchorTitle(anchor) + ' · ' + anchor.path
					));
				});
				areaCard.appendChild(areaAnchors);
				areaSection.appendChild(areaCard);
			});
			root.appendChild(areaSection);
		}

		var hypotheses = Array.isArray(TASK_INVESTIGATION.working_hypothesis)
			? TASK_INVESTIGATION.working_hypothesis
			: [];
		if (hypotheses.length) {
			var hypothesisSection = el('section', 'rm-workspace-section rm-task-hypothesis');
			hypothesisSection.appendChild(renderViewHeading(
					msg('main.task_lens.working_hypothesis'),
					msg('main.task_lens.what_evidence_supports'),
					msg('main.task_lens.hypothesis_copy')
			));
			var hypothesisList = el('div', 'rm-task-hypothesis-list');
				hypotheses.forEach(function (clause) {
					var item = el('article', 'rm-task-hypothesis-clause');
					item.appendChild(taskLensEnumBadge('hypothesis_status', clause.status));
					var claim = el('div', 'rm-task-hypothesis-claim');
					claim.appendChild(txt('p', '', clause.text || ''));
					appendTaskLensCitations(claim, clause.support_anchor_indexes, msg('main.chrome.evidence'));
					item.appendChild(claim);
				hypothesisList.appendChild(item);
			});
			hypothesisSection.appendChild(hypothesisList);
			root.appendChild(hypothesisSection);
		}

		var anchors = Array.isArray(TASK_INVESTIGATION.anchors) ? TASK_INVESTIGATION.anchors : [];
		if (anchors.length) {
			var anchorSection = el('section', 'rm-workspace-section rm-task-anchors');
			anchorSection.appendChild(renderViewHeading(
					msg('main.task_lens.anchor_map'),
					msg('main.task_lens.files_and_symbols'),
					msg('main.task_lens.anchor_map_copy')
			));
			var anchorList = el('div', 'rm-task-anchor-list');
			anchors.forEach(function (anchor, index) {
				var card = el('article', 'rm-task-anchor');
				card.id = taskLensAnchorElementID(index);
				card.tabIndex = -1;
				var heading = el('div', 'rm-task-anchor__heading');
				var title = el('div', '');
				title.appendChild(txt('span', 'rm-task-anchor__index', String(index + 1)));
				title.appendChild(txt('strong', '', taskLensAnchorTitle(anchor)));
				title.appendChild(taskLensEnumBadge('anchor_role', anchor.role));
				heading.appendChild(title);
				var showSource = sourceActionElement(
					staticSourceMode() ? staticSourceOpenLabel() : msg('main.show.source'),
					'rm-secondary-action rm-source-action-link',
					{ path: anchor.path, line: anchor.start_line },
					anchor.end_line,
					function () {
						openSourceSnippet(anchor.source, { path: anchor.path, line: anchor.start_line });
					}
				);
				if (showSource) heading.appendChild(showSource);
				card.appendChild(heading);
				card.appendChild(renderFileReference(
					anchor.path,
					'rm-task-anchor__location',
					anchor.start_line,
					anchor.path + ':' + anchor.start_line + '–' + anchor.end_line
				));
				card.appendChild(txt('p', '', anchor.why || ''));
				anchorList.appendChild(card);
			});
			anchorSection.appendChild(anchorList);
			root.appendChild(anchorSection);
		}

		var joins = Array.isArray(TASK_INVESTIGATION.evidence_joins)
			? TASK_INVESTIGATION.evidence_joins
			: [];
		if (joins.length) {
			var joinSection = el('section', 'rm-workspace-section rm-task-joins');
			joinSection.appendChild(renderViewHeading(
					msg('main.task_lens.evidence_joins'),
					msg('main.task_lens.how_anchors_connect'),
					msg('main.task_lens.joins_copy')
			));
			var joinList = el('div', 'rm-task-join-list');
			joins.forEach(function (join) {
				var left = taskLensAnchor(join.left_anchor);
				var right = taskLensAnchor(join.right_anchor);
				if (!left || !right) return;
				var card = el('article', 'rm-task-join');
				var title = el('div', 'rm-task-join__title');
				title.appendChild(txt('strong', '', taskLensAnchorTitle(left)));
				var connector = taskLensJoinConnector(join.kind);
				title.appendChild(txt(
					'span',
					'',
					connector + taskLensEnumLabel('relation_kind', join.kind) + connector
				));
				title.appendChild(txt('strong', '', taskLensAnchorTitle(right)));
				card.appendChild(title);
				card.appendChild(taskLensEnumBadge('support_type', join.support));
				card.appendChild(txt('p', '', join.explanation || ''));
				card.appendChild(txt('p', 'rm-task-scope', msg('main.task.scope', {
	          scope: join.scope_non_guarantees || msg('main.task_lens.bounded_local_evidence_only'),
	        })));
					appendTaskLensCitations(card, join.support_anchor_indexes, msg('main.task_lens.supporting_anchors'));
				joinList.appendChild(card);
			});
			joinSection.appendChild(joinList);
			root.appendChild(joinSection);
		}

		var guidanceGrid = el('div', 'rm-task-guidance-grid');
		var reproduce = el('section', 'rm-workspace-section');
		reproduce.appendChild(renderViewHeading(
				msg('main.task_lens.reproduce_or_observe'),
				msg('main.task_lens.collect_signal'),
				msg('main.task_lens.authority_copy')
		));
		reproduce.appendChild(taskLensGuidanceList(TASK_INVESTIGATION.reproduce_or_observe));
		guidanceGrid.appendChild(reproduce);

		var verification = TASK_INVESTIGATION.verify || {};
		var verify = el('section', 'rm-workspace-section');
		verify.appendChild(renderViewHeading(msg('main.chrome.verify'), msg('main.chrome.confirm.the.intended.effect'), verification.effect_to_observe || ''));
		verify.appendChild(taskLensGuidanceList(verification.steps));
		guidanceGrid.appendChild(verify);
		root.appendChild(guidanceGrid);

		var probes = Array.isArray(TASK_INVESTIGATION.next_probes) ? TASK_INVESTIGATION.next_probes : [];
		if (probes.length) {
			var probeSection = el('section', 'rm-workspace-section rm-task-probes');
			probeSection.appendChild(renderViewHeading(
					msg('main.task_lens.next_probes'),
					msg('main.what.remains.unresolved'),
					msg('main.task_lens.probes_copy')
			));
			var probeList = el('ul', '');
			probes.forEach(function (probe) {
				var item = el('li', '');
				var probeBody = el('div', 'rm-task-probe-body');
				probeBody.appendChild(txt('span', '', probe.text || ''));
					appendTaskLensCitations(probeBody, probe.anchor_indexes, msg('main.task_lens.inspect'));
				item.appendChild(taskLensEnumBadge('probe_action', probe.action));
				item.appendChild(probeBody);
				probeList.appendChild(item);
			});
			probeSection.appendChild(probeList);
			root.appendChild(probeSection);
		}

		var details = el('details', 'rm-task-retrieval');
		details.appendChild(txt('summary', '', msg('main.chrome.bounded.retrieval.details')));
		var budget = TASK_INVESTIGATION.budget || {};
      details.appendChild(txt(
        'p',
        '',
        msg('main.task.budget_summary', {
          files: Number(budget.read_files || 0),
          bytes: Number(budget.read_bytes || 0),
          calls: Number(TASK_INVESTIGATION.provider && TASK_INVESTIGATION.provider.calls || 0),
        })
      ));
		if ((TASK_INVESTIGATION.stages_skipped || []).length) {
			details.appendChild(txt('p', '', msg('main.task.skipped', {
				stages: TASK_INVESTIGATION.stages_skipped.map(function (stage) {
					return taskLensEnumLabel('stage', stage);
				}).join(', '),
      })));
		}
		root.appendChild(details);
	}

	function exactOverviewSourcePath(value) {
		if (typeof value !== 'string' || !value || !OPENABLE_PATH_SET[value] ||
			value.charAt(0) === '/' || value.charAt(value.length - 1) === '/' ||
			value.indexOf('\\') >= 0 || value.indexOf('\u0000') >= 0) return '';
		var segments = value.split('/');
		for (var index = 0; index < segments.length; index++) {
			if (!segments[index] || segments[index] === '.' || segments[index] === '..') return '';
		}
		return value;
	}

	function overviewSnippetContentIdentity(snippet) {
		if (typeof snippet.content_sha256 === 'string' && snippet.content_sha256) {
			return 'sha256:' + snippet.content_sha256;
		}
		return 'lines:' + JSON.stringify(snippet.lines || []);
	}

	function overviewSnippetStableKey(snippet) {
		return [
			snippet.path,
			String(snippet.start_line),
			String(snippet.end_line),
			String(snippet.revision || ''),
			overviewSnippetContentIdentity(snippet),
		].join('\u0000');
	}

	// Overlapping saved excerpts are normal. Prefer the smallest exact
	// containing interval, then stable persisted fields. Only two views of the
	// same interval/revision with different source content are ambiguous.
	function exactOverviewSourceResolutionFromSnippets(location, snippets) {
		if (!location || exactOverviewSourcePath(location.path) !== location.path ||
			!Number.isInteger(location.line) || location.line <= 0) return { source: null, conflict: false };
		var equivalent = {};
		(Array.isArray(snippets) ? snippets : []).forEach(function (snippet) {
			if (!sourceSnippetHasCode(snippet) ||
				exactOverviewSourcePath(snippet.path) !== location.path ||
				!Number.isInteger(snippet.start_line) || !Number.isInteger(snippet.end_line) ||
				snippet.start_line <= 0 || snippet.end_line < snippet.start_line ||
				location.line < snippet.start_line || location.line > snippet.end_line) return;
			var key = overviewSnippetStableKey(snippet);
			var current = equivalent[key];
			if (!current || String(snippet.presentation_sha256 || '').localeCompare(
				String(current.presentation_sha256 || '')
			) < 0) equivalent[key] = snippet;
		});
		var matches = Object.keys(equivalent).map(function (key) { return equivalent[key]; });
		if (!matches.length) return { source: null, conflict: false };
		matches.sort(function (left, right) {
			return (left.end_line - left.start_line) - (right.end_line - right.start_line) ||
				left.start_line - right.start_line || left.end_line - right.end_line ||
				String(left.revision || '').localeCompare(String(right.revision || '')) ||
				overviewSnippetContentIdentity(left).localeCompare(overviewSnippetContentIdentity(right)) ||
				String(left.presentation_sha256 || '').localeCompare(String(right.presentation_sha256 || ''));
		});
		var smallestSpan = matches[0].end_line - matches[0].start_line;
		var survivors = matches.filter(function (snippet) {
			return snippet.end_line - snippet.start_line === smallestSpan;
		});
		var intervalIdentities = {};
		var conflict = survivors.some(function (snippet) {
			var interval = snippet.path + '\u0000' + String(snippet.start_line) + '\u0000' +
				String(snippet.end_line);
			var identity = String(snippet.revision || '') + '\u0000' + overviewSnippetContentIdentity(snippet);
			if (intervalIdentities[interval] && intervalIdentities[interval] !== identity) return true;
			intervalIdentities[interval] = identity;
			return false;
		});
		if (conflict) return { source: null, conflict: true };
		var selected = survivors[0];
		return { source: {
			snippet: selected,
			location: {
				path: location.path,
				line: location.line,
				column: Number.isInteger(location.column) && location.column > 0 ? location.column : 0,
			},
		}, conflict: false };
	}

	function exactOverviewSourceResolutionForLocation(location) {
		return exactOverviewSourceResolutionFromSnippets(location, allEmbeddedSourceSnippets());
	}

	function exactOverviewSourceForLocation(location) {
		return exactOverviewSourceResolutionForLocation(location).source;
	}

	function exactOverviewActionResolutionForLocation(location) {
		var sourceResolution = exactOverviewSourceResolutionForLocation(location);
		if (sourceResolution.conflict || sourceResolution.source) return sourceResolution;
		if (!location || exactOverviewSourcePath(location.path) !== location.path ||
			!Number.isInteger(location.line) || location.line <= 0 ||
			!overviewLocationOnlyActionAvailable(location)) {
			return { source: null, conflict: false };
		}
		return { source: {
			snippet: null,
			location: {
				path: location.path,
				line: location.line,
				column: Number.isInteger(location.column) && location.column > 0 ? location.column : 0,
			},
		}, conflict: false };
	}

	function overviewLocationOnlyActionAvailable(location) {
		if (allEmbeddedSourceSnippets().some(sourceSnippetHasCode)) return false;
		if (serverMode() && currentRunID() && SOURCE_IDS[location && location.path]) {
			return sourceLocationActionAvailable(location);
		}
		if (!staticSourceMode()) return false;
		return sourceLocationActionAvailable(location);
	}

	// overviewLocationOnlyText builds a location-only reference from a
	// surface location when no snippet/source is resolvable (Decision 222:
	// no-snippet locations stay text spans, never vanish).
	function overviewLocationOnlyText(location) {
		if (!location) return null;
		var path = String(location.path || '');
		var line = Number(location.line) || 0;
		if (!path || line <= 0) return null;
		return {
			path: path,
			line: line,
			column: Number.isInteger(location.column) && location.column > 0 ? location.column : 0,
		};
	}

	function overviewSurfaceLocation(trigger) {
		if (!trigger) return null;
		return trigger.handler_location || trigger.registration_site || trigger.descriptor_site ||
			trigger.server_start_site || (trigger.process_entrypoint && trigger.process_entrypoint.location) || null;
	}

	function overviewSurfaceTitle(trigger) {
		var identity = trigger && trigger.identity || {};
		var method = typeof identity.method === 'string' ? identity.method : '';
		var pathValue = identity.path && identity.path.known && typeof identity.path.text === 'string'
			? identity.path.text : '';
		var route = [method, pathValue].filter(Boolean).join(' ');
		if (route) return route;
		if (typeof identity.name === 'string' && identity.name) return identity.name;
		if (trigger && trigger.handler && trigger.handler.known && typeof trigger.handler.text === 'string' &&
			trigger.handler.text) return trigger.handler.text;
		if (trigger && trigger.process_entrypoint && typeof trigger.process_entrypoint.name === 'string' &&
			trigger.process_entrypoint.name) return trigger.process_entrypoint.name;
		return String(trigger && trigger.id || '');
	}

	// overviewSurfaceTitleIsValueShaped implements the Decision 229 D3
	// presentation-quality gate: a title that names a local value or callback
	// local (amount, payer, application_context, unresolved value, TrimSpace/
	// Sprintf results, …) is never a primary surface title. Deterministic,
	// closed-list based; case-insensitive; prefix/suffix tolerant.
	var OVERVIEW_VALUE_SHAPED_TITLE_WORDS = [
		'amount', 'payer', 'application_context', 'unresolved value',
		'unresolved_value', 'result of strings.trimspace', 'result of fmt.sprintf',
		'result of strings.trim', 'result of fmt.sprintf',
		'error', 'err', 'result', 'response', 'request', 'ctx', 'context',
		'value', 'val', 'ret', 'payload', 'body', 'params', 'args',
	];
	function overviewSurfaceTitleIsValueShaped(title) {
		if (!title) return true;
		var lower = String(title).toLowerCase().trim();
		if (!lower) return true;
		var words = lower.split(/[^a-z0-9_]+/).filter(Boolean);
		var joined = words.join(' ');
		if (OVERVIEW_VALUE_SHAPED_TITLE_WORDS.indexOf(joined) >= 0) return true;
		// Multi-word phrases that are pure value shapes: "result of …",
		// "unresolved …", single-word callback locals.
		if (/^result of /.test(lower)) return true;
		if (/^unresolved /.test(lower)) return true;
		if (words.length === 1 && OVERVIEW_VALUE_SHAPED_TITLE_WORDS.indexOf(words[0]) >= 0) return true;
		return false;
	}

	var OVERVIEW_SURFACE_KIND_MESSAGE_IDS = {
		async_task: 'main.overview.anatomy.surface_kind.async_task',
		cli_command: 'main.overview.anatomy.surface_kind.cli_command',
		http_route: 'main.overview.anatomy.surface_kind.http_route',
		http_route_descriptor: 'main.overview.anatomy.surface_kind.http_route_descriptor',
		http_route_frontier: 'main.overview.anatomy.surface_kind.http_route_frontier',
		http_server: 'main.overview.anatomy.surface_kind.http_server',
		process_entry: 'main.overview.anatomy.surface_kind.process_entry',
		worker: 'main.overview.anatomy.surface_kind.worker',
	};

	function overviewSurfaceKindLabel(kind) {
		return msg(
			OVERVIEW_SURFACE_KIND_MESSAGE_IDS[String(kind || '')] ||
				'main.overview.anatomy.surface_kind.other'
		);
	}

	function overviewEntrySurfaceObjects() {
		var triggers = DATA.discovered_surfaces && Array.isArray(DATA.discovered_surfaces.triggers)
			? DATA.discovered_surfaces.triggers : [];
		var eligible = triggers.filter(function (trigger) {
			return !!(trigger && typeof trigger.id === 'string' && trigger.id &&
				trigger.surface_role === 'entry_surface' &&
				trigger.application_classification === 'application_surface' &&
				trigger.availability === 'available' && trigger.executable_role !== 'test_or_helper' &&
				trigger.provisional_id !== true);
		});
		var counts = {};
		eligible.forEach(function (trigger) { counts[trigger.id] = (counts[trigger.id] || 0) + 1; });
		var objects = [];
		eligible.forEach(function (trigger) {
			if (counts[trigger.id] !== 1) return;
			var title = overviewSurfaceTitle(trigger);
			// Decision 229 D3 presentation-quality gate: value-shaped local
			// names never become primary surface titles. They remain in
			// DATA (never lost); they are counted in `omitted` below.
			if (overviewSurfaceTitleIsValueShaped(title)) return;
			var surfaceLocation = overviewSurfaceLocation(trigger);
			var resolved = exactOverviewActionResolutionForLocation(surfaceLocation);
			var source = resolved.source;
			if (!source) {
				// Decision 229 D3 / 222: without a resolvable source the
				// entry still answers "how work enters" as a location-only
				// text span (D222: no-snippet locations stay text spans).
				// Only a resolved snippet makes it an actionable button.
				// This applies ONLY when no embedded snippet exists at all
				// (plain offline report): with saved snippets present,
				// unsourced surfaces stay excluded (ambiguous/dead never
				// render, Decision 217).
				var hasAnyEmbeddedSnippet = allEmbeddedSourceSnippets().some(sourceSnippetHasCode);
				if (hasAnyEmbeddedSnippet || resolved.conflict) return;
				var fallbackLocation = surfaceLocation && overviewLocationOnlyText(surfaceLocation);
				if (!fallbackLocation) return;
				objects.push({
					id: trigger.id,
					title: title,
					detail: overviewSurfaceKindLabel(trigger.kind),
					snippet: null,
					location: fallbackLocation,
					entryKind: String(trigger.kind || ''),
					entryRole: String(trigger.executable_role || ''),
					entryExecutable: String(trigger.owning_executable ||
						(trigger.process_entrypoint && trigger.process_entrypoint.package) || ''),
					entryFramework: String(trigger.framework || ''),
					locationOnly: true,
				});
				return;
			}
			objects.push({
				id: trigger.id,
				title: title,
				detail: overviewSurfaceKindLabel(trigger.kind),
				snippet: source.snippet,
				location: source.location,
				entryKind: String(trigger.kind || ''),
				entryRole: String(trigger.executable_role || ''),
				entryExecutable: String(trigger.owning_executable ||
					(trigger.process_entrypoint && trigger.process_entrypoint.package) || ''),
				entryFramework: String(trigger.framework || ''),
				locationOnly: false,
			});
		});
		return {
			objects: objects,
			total: Object.keys(counts).length,
			omitted: Object.keys(counts).length - objects.length,
		};
	}

	// overviewEntryClassifications groups eligible entry surfaces into
	// user-facing classes derived from existing fields only. Decision 233
	// (Archive 9): the grouping is repository-shape + product-role aware —
	// cli_command is PRIMARY on CLI-shaped repositories, exact HTTP routes
	// group as primary on service/application shapes, library APIs on
	// library shapes, jobs/consumers on worker shapes — instead of the old
	// kind-only collapse that treated every cli_command as tooling.
	function overviewEntryClassifications(objects) {
		var archetype = String(DATA.repository_archetype ||
			(DATA.architecture_canvas && DATA.architecture_canvas.repository_archetype) || '');
		var groups = [];
		var byClass = {};
		var push = function (key, labelMessageID, items) {
			if (!items.length) return;
			byClass[key] = { key: key, labelMessageID: labelMessageID, items: items };
			groups.push(byClass[key]);
		};
		var primary = [], service = [], tooling = [], library = [], worker = [], other = [];
		var isPrimaryProduct = function (kind, role) {
			// Shape-specific priority (Decision 233): the primary entry
			// family depends on the repository shape, never on the kind
			// alone.
			if (kind === 'process_entry' && role === 'primary_application') return true;
			if (archetype === 'cli_tool' && (kind === 'cli_command' || role === 'tooling')) return true;
			// F1 (fresh review): a CLI tree on a daemon/worker repository is
			// still the primary operational surface for a CLI-shaped product
			// (restic: 36 cli_command on daemon_worker_system).
			if (archetype === 'daemon_worker_system' && kind === 'cli_command') return true;
			if ((archetype === 'application' || archetype === 'modular_platform_server') &&
				(kind === 'http_server' || kind === 'grpc_server' || kind === 'service' ||
				 kind === 'http_route' || kind === 'http_handler')) return true;
			if (archetype === 'library_framework' &&
				(kind === 'library_api' || kind === 'exported_api')) return true;
			if (archetype === 'daemon_worker_system' &&
				(kind === 'http_server' || kind === 'grpc_server' || kind === 'service' ||
				 kind === 'http_route' || kind === 'http_handler')) return true;
			// F2 (fresh review): monorepo_mixed promotes its per-app
			// process/routes the same way an application repository does.
			if (archetype === 'monorepo_mixed' &&
				(kind === 'http_server' || kind === 'grpc_server' || kind === 'service' ||
				 kind === 'http_route' || kind === 'http_handler')) return true;
			return false;
		};
		objects.forEach(function (object) {
			var kind = object.entryKind;
			var role = object.entryRole;
			if (isPrimaryProduct(kind, role)) { primary.push(object); return; }
			if (kind === 'http_server' || kind === 'grpc_server' || kind === 'service' ||
				(kind === 'process_entry' && role === 'secondary_service')) { service.push(object); return; }
			if (kind === 'cli_command' || kind === 'tool' || role === 'tooling') { tooling.push(object); return; }
			if (kind === 'library_api' || kind === 'exported_api') { library.push(object); return; }
			other.push(object);
		});
		push('primary', 'main.overview.entry.primary', primary);
		push('service', 'main.overview.entry.service', service);
		push('tooling', 'main.overview.entry.tooling', tooling);
		push('library', 'main.overview.entry.library', library);
		push('worker', 'main.overview.entry.worker', worker);
		push('other', 'main.overview.entry.other', other);
		return groups;
	}

	// overviewEntryGroupIsPrimaryTooling reports whether a tooling group is
	// the repository's primary product surface on a CLI-shaped repository
	// (Decision 233): cli_command entries are PRIMARY on cli_tool
	// repositories and on daemon/worker repositories whose CLI is the
	// operational product (F1), never a collapsed secondary.
	function overviewEntryGroupIsPrimaryTooling(group) {
		var archetype = String(DATA.repository_archetype ||
			(DATA.architecture_canvas && DATA.architecture_canvas.repository_archetype) || '');
		if (archetype !== 'cli_tool' && archetype !== 'daemon_worker_system') return false;
		return group.items.some(function (object) {
			return object.entryKind === 'cli_command';
		});
	}

	// componentEvidenceTier derives a small non-numeric presentation tier from
	// existing fields (Decision 217):
	//   exact_source   — exact symbol/path sources present in navigation
	//   package_backed — packages present but no exact symbol start
	//   hypothesis     — model grouping/hypothesis with limited support
	//   unmapped       — locally observed members not assigned to the map
	function componentEvidenceTier(component) {
		var context = (component && component.id) ? (architectureComponentContexts()[component.id] || {}) : {};
		var navigation = ARCHITECTURE_COMPONENT_NAVIGATION && Number(ARCHITECTURE_COMPONENT_NAVIGATION.version) === 1
			? (ARCHITECTURE_COMPONENT_NAVIGATION.components || []).filter(function (entry) {
				return entry && entry.component_id === component.id;
			})[0] : null;
		var exactSources = (context.sources && context.sources.length) ||
			(navigation && navigation.symbol_sources && navigation.symbol_sources.length) ||
			(component.symbol_sources && component.symbol_sources.length);
		if (exactSources) return 'exact_source';
		var packages = (navigation && navigation.package_participant_ids && navigation.package_participant_ids.length) ||
			(component.package_ids && component.package_ids.length) ||
			(context.packageIDs && context.packageIDs.length);
		if (packages) return 'package_backed';
		return 'hypothesis';
	}

	var EVIDENCE_TIER_MESSAGE_IDS = {
		exact_source: 'main.evidence.tier.exact_source',
		package_backed: 'main.evidence.tier.package_backed',
		hypothesis: 'main.evidence.tier.hypothesis',
		unmapped: 'main.evidence.tier.unmapped',
	};

	function componentEvidenceTierLabel(tier) {
		return msg(EVIDENCE_TIER_MESSAGE_IDS[tier] || 'main.evidence.tier.hypothesis');
	}

	function currentArchitectureComponents(canvas) {
		canvas = canvas || DATA.architecture_canvas || {};
		var remainderComponentID = String(canvas.local_remainder_component_id || '');
		return (Array.isArray(canvas.components) ? canvas.components : []).filter(function (component) {
			return !!(component && (!remainderComponentID || component.id !== remainderComponentID));
		});
	}

	function overviewComponentObjects() {
		var canvas = DATA.architecture_canvas || {};
		var remainderComponentID = String(canvas.local_remainder_component_id || '');
		// Decision 229 D3: a diagnostic subsystem (category === 'diagnostic',
		// e.g. "Supporting repository evidence") is unclassified exact scope —
		// never a principal product area. It is counted and rendered as a
		// collapsed disclosure, not as a principal component card.
		var diagnosticSubsystemIDs = {};
		(Array.isArray(canvas.subsystems) ? canvas.subsystems : []).forEach(function (subsystem) {
			if (subsystem && subsystem.category === 'diagnostic' && typeof subsystem.id === 'string') {
				diagnosticSubsystemIDs[subsystem.id] = true;
			}
		});
		var components = currentArchitectureComponents().filter(function (component) {
			if (diagnosticSubsystemIDs[component.subsystem_id]) return false;
			return true;
		});
		var remainderComponents = (Array.isArray(canvas.components) ? canvas.components : []).filter(function (component) {
			if (!component) return false;
			if (remainderComponentID && component.id === remainderComponentID) return true;
			return !!diagnosticSubsystemIDs[component.subsystem_id];
		});
		var remainderCounts = { packages: 0, symbols: 0, other: 0 };
		remainderComponents.forEach(function (component) {
			(Array.isArray(component.members) ? component.members : []).forEach(function (member) {
				var kind = String(member && member.id && member.id.kind || '');
				if (kind === 'package') remainderCounts.packages += 1;
				else if (kind === 'symbol' || kind === 'entrypoint') remainderCounts.symbols += 1;
				else remainderCounts.other += 1;
			});
		});
		var contexts = architectureComponentContexts();
		var navigationByComponent = {};
		if (ARCHITECTURE_COMPONENT_NAVIGATION && Number(ARCHITECTURE_COMPONENT_NAVIGATION.version) === 1) {
			(ARCHITECTURE_COMPONENT_NAVIGATION.components || []).forEach(function (entry) {
				if (entry && entry.component_id) navigationByComponent[entry.component_id] = entry;
			});
		}
		var counts = {};
		components.forEach(function (component) {
			if (component && typeof component.id === 'string' && component.id) {
				counts[component.id] = (counts[component.id] || 0) + 1;
			}
		});
		var objects = [];
		components.forEach(function (component) {
			if (!component || !component.id || counts[component.id] !== 1) return;
			var context = contexts[component.id] || {};
			var navigation = navigationByComponent[component.id] || {};
			var resolved = [];
			var resolvedSeen = {};
			(Array.isArray(context.sources) ? context.sources : []).forEach(function (source) {
				var resolution = exactOverviewActionResolutionForLocation(source && source.location);
				if (resolution.conflict) return;
				if (!resolution.source) return;
				var location = resolution.source.location;
				var key = location.path + '\u0000' + String(location.line) + '\u0000' +
					String(location.column || 0) + '\u0000' + (resolution.source.snippet
						? overviewSnippetStableKey(resolution.source.snippet)
						: 'exact-location');
				if (resolvedSeen[key]) return;
				resolvedSeen[key] = true;
				resolved.push({
					symbol: String(source.symbol || source.label || source.detail || ''),
					snippet: resolution.source.snippet,
					location: resolution.source.location,
				});
			});
			// Decision 218 (C): a component participates in the Overview
			// system spine even without exact symbol sources — the spine is
			// about roles, not just evidence. Package-backed components use
			// their navigation entry (package participants, map target) and
			// keep an empty source list; the evidence tier stays honest.
			var mapTarget = context.map_target ||
				(navigation.map_target || { kind: 'component', component_id: component.id });
			var packageCount = Array.isArray(navigation.package_participant_ids)
				? navigation.package_participant_ids.length
				: (Array.isArray(component.package_ids) ? component.package_ids.length : 0);
			objects.push({
				id: component.id,
				title: String(component.name || component.id),
				detail: String(component.description || ''),
				mapTarget: mapTarget,
				sources: resolved,
				packageCount: packageCount,
			});
		});
		return {
			objects: objects,
			total: Object.keys(counts).length,
			omitted: Object.keys(counts).length - objects.length,
			remainderCounts: remainderCounts,
			hasDiagnosticRemainder: remainderComponents.length > 0,
		};
	}

	function repositoryOverviewAnatomy() {
		if (!DATA.architecture_canvas) return null;
		var entries = overviewEntrySurfaceObjects();
		var components = overviewComponentObjects();
		if (!entries.objects.length && !components.objects.length) return null;
		return { entries: entries, components: components, integrations: [] };
	}

	// overviewSystemSpine derives the Overview system spine (Decision 218 C):
	// at most one representative card per supported role — entry/consumption,
	// core coordination/domain, state/resource, extension/integration,
	// operations/support — targeting 3-5 cards with one explicit primary card
	// when evidence supports it. Selection is deterministic and evidence
	// based (exact-source count, package count, then title), never array
	// order or canonical hash alone. Every other component remains reachable
	// through Architecture; nothing is deleted from report data.
	function overviewSystemSpine(anatomy) {
		var entryObjects = anatomy && anatomy.entries && anatomy.entries.objects || [];
		var componentObjects = anatomy && anatomy.components && anatomy.components.objects || [];
		var spine = { roles: [], primary: null, totalComponents: componentObjects.length };
		var roleGroups = {
			entry: [],
			core: [],
			state: [],
			extension: [],
			operations: [],
		};
		entryObjects.forEach(function (object) {
			roleGroups.entry.push({ object: object, isEntry: true });
		});
		componentObjects.forEach(function (object) {
			var role = overviewComponentRole(object);
			if (role && roleGroups[role]) roleGroups[role].push({ object: object, isEntry: false });
		});
		var evidenceScore = function (candidate) {
			var object = candidate.object;
			var score = 0;
			if (object.isEntry) score += 1000;
			score += Math.min(Array.isArray(object.sources) ? object.sources.length : 0, 8) * 100;
			score += Math.min(object.packageCount || 0, 12) * 10;
			return score;
		};
		var pickRole = function (role, isPrimary) {
			var group = roleGroups[role];
			if (!group.length) return;
			group.sort(function (left, right) {
				var byEvidence = evidenceScore(right) - evidenceScore(left);
				if (byEvidence) return byEvidence;
				return String(left.object.title || '').localeCompare(String(right.object.title || ''));
			});
			var winner = group[0];
			var card = {
				role: role,
				object: winner.object,
				isEntry: winner.isEntry,
				primary: !!isPrimary,
			};
			spine.roles.push(card);
			if (isPrimary) spine.primary = card;
		};
		// Entry/consumption is the natural primary card when present.
		if (roleGroups.entry.length) {
			pickRole('entry', true);
		}
		pickRole('core', false);
		pickRole('state', false);
		pickRole('extension', false);
		pickRole('operations', false);
		// Without any entry surface, the strongest core component becomes
		// primary.
		if (!spine.primary && roleGroups.core.length) {
			spine.primary = spine.roles.filter(function (card) { return card.role === 'core'; })[0] || null;
			if (spine.primary) spine.primary.primary = true;
		}
		return spine;
	}

	// overviewComponentRole classifies one component into a closed spine role
	// from exact local fields only (Decision 218 C): name/description
	// keywords in the report language, then evidence. It never invents a
	// role; a component that matches no role returns null.
	function overviewComponentRole(object) {
		var title = String(object.title || '').toLowerCase();
		var detail = String(object.detail || '').toLowerCase();
		var text = title + ' ' + detail;
		var has = function (words) {
			return words.some(function (word) { return text.indexOf(word) >= 0; });
		};
		if (has(['api', 'sdk', 'core', 'domain', 'kernel', 'ядро', 'основн', 'координац', 'coordination', 'сервер', 'server', 'движок', 'engine'])) {
			if (has(['интеграц', 'integration', 'plugin', 'плагин', 'адаптер', 'adapter', 'расширени', 'extension', 'провайдер', 'provider'])) return 'extension';
			return 'core';
		}
		if (has(['storage', 'store', 'cache', 'database', 'db', 'state', 'resource', 'хранилищ', 'баз', 'кэш', 'состояни', 'ресурс', 'бэкенд', 'backend', 'repo', 'репозитор'])) return 'state';
		if (has(['интеграц', 'integration', 'plugin', 'плагин', 'адаптер', 'adapter', 'расширени', 'extension', 'провайдер', 'provider', 'connector', 'коннектор'])) return 'extension';
		if (has(['tool', 'util', 'cli', 'debug', 'script', 'build', 'инструмент', 'утилит', 'отладк', 'сборк', 'скрипт', 'терминал', 'terminal', 'diagnostic', 'диагност'])) return 'operations';
		return null;
	}

	function renderOverviewSystemSpine(spine) {
		if (!spine || !spine.roles.length) return null;
		var section = el('section', 'rm-workspace-section rm-overview-spine');
		section.appendChild(renderViewHeading(
			msg('main.overview.spine.kicker'),
			msg('main.overview.spine.title'),
			msg('main.overview.spine.copy')
		));
		var grid = el('div', 'rm-overview-spine-grid');
		spine.roles.forEach(function (card) {
			var wrapper = el('div', 'rm-overview-spine-card' + (card.primary ? ' rm-overview-spine-card--primary' : ''));
			var roleMessageID = 'main.overview.spine.role.operations';
			if (card.role === 'entry') roleMessageID = 'main.overview.spine.role.entry';
			else if (card.role === 'core') roleMessageID = 'main.overview.spine.role.core';
			else if (card.role === 'state') roleMessageID = 'main.overview.spine.role.state';
			else if (card.role === 'extension') roleMessageID = 'main.overview.spine.role.extension';
			wrapper.appendChild(txt('span', 'rm-overview-spine-role', msg(roleMessageID)));
			wrapper.appendChild(renderOverviewObjectCard(card.object, card.isEntry ? 'surface' : 'component'));
			grid.appendChild(wrapper);
		});
		section.appendChild(grid);
		if (spine.totalComponents > spine.roles.length) {
			var remaining = spine.totalComponents - spine.roles.filter(function (card) { return !card.isEntry; }).length;
			if (remaining > 0) {
				var allAction = txt('button', 'rm-secondary-action rm-overview-spine-all', msg('main.overview.spine.all', { count: remaining }));
				allAction.type = 'button';
				allAction.onclick = function () { navigateWorkspace('architecture'); };
				section.appendChild(allAction);
			}
		}
		return section;
	}

	function renderOverviewObjectCard(object, kind) {
		var card = el('article', 'rm-overview-object-card');
		card.setAttribute('data-rm-object-kind', kind);
		card.setAttribute('data-rm-object-id', object.id);
		if (kind === 'component') {
			var tier = componentEvidenceTier(object);
			card.setAttribute('data-rm-evidence-tier', tier);
			var mapAction = el('button', 'rm-overview-object-primary rm-overview-object-map');
			mapAction.type = 'button';
			mapAction.appendChild(txt('strong', '', object.title));
			if (object.detail) mapAction.appendChild(txt('span', 'rm-overview-object-detail', object.detail));
			mapAction.onclick = function () { openArchitectureTarget(object.mapTarget, null); };
			card.appendChild(mapAction);
			var meta = el('div', 'rm-overview-object-meta');
			meta.appendChild(txt('span', 'rm-evidence-tier rm-evidence-tier--' + tier, componentEvidenceTierLabel(tier)));
			if (object.packageCount > 0) {
				meta.appendChild(txt('span', 'rm-overview-object-count', msg('main.overview.object.package_count', { count: object.packageCount })));
			}
			card.appendChild(meta);
			var sources = el('div', 'rm-overview-object-sources');
			var representative = Array.isArray(object.sources) ? object.sources.slice(0, 1) : [];
			representative.forEach(function (source) {
				if (!source || !source.symbol || !source.location) return;
				var hasEmbeddedSource = sourceSnippetHasCode(source.snippet);
				var sourceAction = hasEmbeddedSource
					? el('button', 'rm-overview-object-source rm-source-action-link')
					: sourceActionElement(
						'',
						'rm-overview-object-source rm-source-action-link',
						source.location,
						0,
						function () { openSourceLocation(source.location); }
					);
				// Decision 222: without a GitHub/GitLab (or server) jump the
				// source action is not offered at all.
				if (!sourceAction) return;
				if (hasEmbeddedSource) sourceAction.type = 'button';
				sourceAction.appendChild(txt('strong', '', source.symbol));
				sourceAction.appendChild(txt('code', '', formatCodeLocation(source.location)));
				if (hasEmbeddedSource) {
					sourceAction.onclick = function () {
						openSourceSnippet(source.snippet, source.location, false, { drawerFirst: true });
					};
				}
				sources.appendChild(sourceAction);
			});
			if (sources.childNodes.length) card.appendChild(sources);
			if (Array.isArray(object.sources) && object.sources.length > 1) {
				card.appendChild(txt('span', 'rm-overview-object-more', msg('main.overview.object.more_sources', { count: object.sources.length - 1 })));
			}
			return card;
		}
		var hasEmbeddedSource = sourceSnippetHasCode(object.snippet);
		var primary;
		if (object.locationOnly || !hasEmbeddedSource && !sourceLocationActionAvailable(object.location)) {
			// Decision 222/229 D3: a location-only entry stays a plain
			// text span — the answer "how work enters" is preserved even
			// when no exact-source jump can be offered.
			primary = el('div', 'rm-overview-object-primary rm-overview-object-primary--location-only');
			primary.appendChild(txt('strong', '', object.title));
			if (object.detail) primary.appendChild(txt('span', 'rm-overview-object-detail', object.detail));
			primary.appendChild(txt('code', 'rm-overview-object-location', formatCodeLocation(object.location)));
			card.appendChild(primary);
			return card;
		}
		primary = hasEmbeddedSource
			? el('button', 'rm-overview-object-primary')
			: sourceActionElement(
				'',
				'rm-overview-object-primary',
				object.location,
				0,
				function () { openSourceLocation(object.location); }
			);
		if (!primary) {
			// Decision 222: without a resolvable jump the entry stays a
			// location-only text span rather than vanishing.
			primary = el('div', 'rm-overview-object-primary rm-overview-object-primary--location-only');
			primary.appendChild(txt('strong', '', object.title));
			if (object.detail) primary.appendChild(txt('span', 'rm-overview-object-detail', object.detail));
			primary.appendChild(txt('code', 'rm-overview-object-location', formatCodeLocation(object.location)));
			card.appendChild(primary);
			return card;
		}
		if (hasEmbeddedSource) primary.type = 'button';
		primary.appendChild(txt('strong', '', object.title));
		if (object.detail) primary.appendChild(txt('span', 'rm-overview-object-detail', object.detail));
		primary.appendChild(txt('code', 'rm-overview-object-location', formatCodeLocation(object.location)));
		primary.appendChild(txt('span', 'rm-overview-object-action', msg('main.open.exact.source')));
		if (hasEmbeddedSource) {
			primary.onclick = function () {
				openSourceSnippet(object.snippet, object.location, false, { drawerFirst: true });
			};
		}
		card.appendChild(primary);
		return card;
	}

	function renderOverviewAnatomyZone(kicker, title, copy, collection, kind) {
		var section = el('section', 'rm-workspace-section rm-overview-anatomy-zone');
		section.appendChild(renderViewHeading(kicker, title, copy));
		var grid = el('div', 'rm-overview-object-grid');
		collection.objects.forEach(function (object) {
			grid.appendChild(renderOverviewObjectCard(object, kind));
		});
		section.appendChild(grid);
		if (collection.omitted > 0) {
			section.appendChild(txt('p', 'rm-overview-anatomy-omitted', msg(
				'main.overview.anatomy.omitted_ambiguous_or_unavailable',
				{ count: collection.omitted }
			)));
		}
		return section;
	}

	function renderRepositoryOverviewLead(root) {
		var thesis = REPOSITORY_GUIDE || DATA.repository_thesis || {};
		var hero = renderRepositoryBriefHero();
		if (!hero) {
			var story = Array.isArray(thesis.system_story) ? thesis.system_story : [];
			// Decision 221 A: the hero purpose is the backend-filtered thesis
			// purpose (README warnings/quotes/lists removed deterministically)
			// or a neutral local fallback — never raw README residue as the
			// sole purpose. Raw README stays labeled source material.
			var purpose = thesis.purpose || '';
			var hasDocumented = !!DATA.documented_purpose;
			var useReadmeSource = hasDocumented && purpose !== DATA.documented_purpose;
			hero = el('section', 'rm-overview-hero rm-purpose-hero');
			hero.appendChild(txt('div', 'rm-view-kicker', msg('main.overview')));
			hero.appendChild(txt('h2', '', DATA.repo_name || msg('main.repository.overview')));
			if (story.length) {
				// Localized repository orientation first (Decision 217): the
				// system story is product copy, the README quote is source
				// material shown separately and labeled.
				var storyNode = el('div', 'rm-hero-story');
				story.forEach(function (entry) {
					storyNode.appendChild(txt('p', 'rm-hero-story__entry', String(entry || '')));
				});
				hero.appendChild(storyNode);
			}
			if (purpose) {
				hero.appendChild(txt('p', 'rm-brief-lead', normalizeMarkdownProse(purpose)));
			} else {
				hero.appendChild(txt('p', 'rm-purpose-fallback', neutralPurposeFallback()));
			}
			if (hasDocumented) {
				var readmeQuote = el('blockquote', 'rm-hero-readme');
				readmeQuote.appendChild(txt('p', '', normalizeMarkdownProse(DATA.documented_purpose)));
				readmeQuote.appendChild(txt('span', 'rm-purpose-source', purposeSourceLabel()));
				hero.appendChild(readmeQuote);
			}
		}
		root.appendChild(hero);
	}

	// readmePurposeHasResidue detects README heads that begin with
	// warning/badge/ASCII-art/marketing residue rather than product copy
	// (Decision 218 G). It is a conservative local heuristic over the first
	// non-empty line: images, badge URLs, ASCII-art borders, and generic
	// marketing imperatives never become a claimed purpose.
	function readmePurposeHasResidue(purpose) {
		var text = String(purpose || '').trim();
		if (!text) return false;
		var firstLine = text.split('\n')[0].trim();
		var lower = firstLine.toLowerCase();
		if (/^!\[/.test(firstLine)) return true;
		if (/^(https?:\/\/|!\[[^\]]*\]\(|<img|\[![^\]]*\]\()/.test(firstLine)) return true;
		if (/^[#>*\s_-]{3,}$/.test(firstLine)) return true;
		if (/^[-=#*|+]{4,}$/.test(firstLine)) return true;
		if (/^┌|^╔|^┏|^\x1b\[/.test(firstLine)) return true;
		if (/(badge|travis|ci status|coverage|codacy|gitter|license[-\s]?badge|build[-\s]?status|downloads?[-\s]?badge)/.test(lower)) return true;
		if (/^(welcome to|install|quick start|getting started|status|build status|releases?|downloads?)$/.test(lower)) return true;
		return false;
	}

	// neutralPurposeFallback assembles a neutral local purpose answer from
	// repository name and archetype (Decision 218 G) — never README prose.
	function neutralPurposeFallback() {
		var archetype = DATA.repository_archetype || DATA.architecture_canvas && DATA.architecture_canvas.repository_archetype || '';
		var name = DATA.repo_name || '';
		var pieces = [];
		if (name) pieces.push(name);
		// Decision 221 A: a localized closed archetype label, never raw
		// README residue. Unknown archetypes collapse to a neutral noun.
		// Literal keys keep the typed catalog acceptance test satisfiable.
		var archetypeLabels = {
			application: msg('main.overview.archetype.application'),
			library_framework: msg('main.overview.archetype.library_framework'),
			modular_platform_server: msg('main.overview.archetype.modular_platform_server'),
			daemon_worker_system: msg('main.overview.archetype.daemon_worker_system'),
			cli_tool: msg('main.overview.archetype.cli_tool'),
		};
		var archetypeLabel = archetypeLabels[String(archetype || '')] || msg('main.overview.archetype.unknown');
		if (archetype) pieces.push(archetypeLabel);
		if (!pieces.length) return msg('main.overview.glance.what_fallback');
		return pieces.join(' · ');
	}

	// renderOverviewAtAGlance answers the four orientation questions above the
	// fold from existing grounded data only (Decision 217): what the repository
	// is, where it is entered, what its main areas are, and what to open first.
	// It reuses the strong "at a glance" idea of the older Casdoor report
	// without restoring the old Overview wall.
	function renderOverviewAtAGlance(anatomy) {
		var rows = [];
		var thesis = REPOSITORY_GUIDE || DATA.repository_thesis || {};
		var story = Array.isArray(thesis.system_story) ? thesis.system_story : [];
		// Decision 218 (G): the glance "what" answer never repeats raw
		// README/documented-purpose text (already shown as labeled source
		// material in the hero). It uses the localized story when present,
		// otherwise a neutral local fallback assembled from repository name
		// and archetype.
		var what = story.length ? story[0] : neutralPurposeFallback();
		if (what) {
			rows.push([msg('main.overview.glance.what'), what]);
		}
		var entryObjects = anatomy && anatomy.entries && anatomy.entries.objects || [];
		if (entryObjects.length) {
			// Decision 221 B: the entry answer leads with production entries
			// (primary process, service), never tests/tooling first.
			var entryOrder = entryObjects.slice().sort(function (left, right) {
				var rank = function (object) {
					if (object.entryKind === 'process_entry' && object.entryRole === 'primary_application') return 0;
					if (object.entryKind === 'http_server' || object.entryKind === 'grpc_server' ||
						object.entryKind === 'service' || object.entryRole === 'secondary_service') return 1;
					if (object.entryKind === 'library_api' || object.entryKind === 'exported_api') return 2;
					if (object.entryKind === 'cli_command' || object.entryRole === 'tooling') return 3;
					return 4;
				};
				var leftRank = rank(left), rightRank = rank(right);
				if (leftRank !== rightRank) return leftRank - rightRank;
				return String(left.title || '').localeCompare(String(right.title || ''));
			});
			var entryLabels = entryOrder.slice(0, 3).map(function (object) {
				return object.title;
			});
			rows.push([msg('main.overview.glance.entry'), entryLabels.join(' · ')]);
		}
		var componentObjects = anatomy && anatomy.components && anatomy.components.objects || [];
		if (componentObjects.length) {
			var componentLabels = componentObjects.slice(0, 4).map(function (object) {
				return object.title;
			});
			rows.push([msg('main.overview.glance.areas'), componentLabels.join(' · ')]);
		}
		if (!rows.length) return null;
		var glance = el('section', 'rm-overview-glance');
		glance.appendChild(txt('h3', 'rm-overview-glance__title', msg('main.overview.glance.title')));
		rows.forEach(function (row) {
			var line = el('div', 'rm-overview-glance__row');
			line.appendChild(txt('span', 'rm-overview-glance__label', row[0]));
			line.appendChild(txt('p', 'rm-overview-glance__value', row[1]));
			glance.appendChild(line);
		});
		// Decision 221 C: the first action is an independent, clickable,
		// keyboard-accessible source action — never a theme-ordinal pick.
		var firstAction = overviewFirstAction(anatomy, atlantisWorkspaceShelf());
		if (firstAction) {
			glance.appendChild(renderOverviewFirstAction(firstAction));
		} else if (window.firstActionAvailable === false || !anatomy.entries.objects.length) {
			glance.appendChild(txt('p', 'rm-overview-first-action rm-overview-first-action--unavailable',
				msg('main.overview.first_action.unavailable')));
		}
		return glance;
	}

	function atlantisWorkspaceShelf() {
		try {
			return repositoryAtlasWorkspaceShelf && repositoryAtlasWorkspaceShelf() || null;
		} catch (error) {
			return null;
		}
	}

	// renderOverviewFirstAction renders the independent first action as a
	// prominent clickable row with its reason and authority (Decision 221 C).
	// Decision 230 D1: the action label AND exact path live inside the one
	// coherent action control — never a button plus inert source text.
	function renderOverviewFirstAction(action) {
		var block = el('div', 'rm-overview-first-action');
		block.appendChild(txt('span', 'rm-overview-first-action__label', msg('main.overview.first_action.title')));
		var button = txt('button', 'rm-overview-first-action__button', action.label);
		button.type = 'button';
		button.onclick = action.action;
		if (action.path) {
			var location = action.line ? action.path + ':' + action.line : action.path;
			if (action.symbol) location += ' · ' + action.symbol;
			button.appendChild(txt('code', 'rm-overview-first-action__path', location));
		}
		block.appendChild(button);
		if (action.reason) block.appendChild(txt('p', 'rm-overview-first-action__reason', action.reason));
		if (action.authority) block.appendChild(txt('p', 'rm-overview-first-action__authority', action.authority));
		return block;
	}

	// overviewFirstAction implements the independent «Open first» selector
	// (Decision 221 C). It never derives the action from theme array order.
	// Priority: backend-designated primary production process entry, library
	// constructor/start/use entry, one deterministic exact non-test process
	// entry when no primary was designated, a core Study theme's first exact
	// reading, then an explicit unavailable state.
	function overviewFirstAction(anatomy, atlasShelf) {
		// 1. Primary production process entry (not tests/tooling).
		var entries = anatomy && anatomy.entries && anatomy.entries.objects || [];
		var primary = entries.filter(function (object) {
			return object.entryKind === 'process_entry' && object.entryRole === 'primary_application';
		});
		if (primary.length) {
			var action = overviewFirstActionFromEntry(primary[0]);
			if (action) {
				action.label = msg('main.overview.first_action.process_entry');
				action.reason = msg('main.overview.first_action.reason.process_entry');
				action.authority = msg('main.overview.first_action.authority.process_entry');
				action.kind = 'process_entry';
				return action;
			}
		}
		// 2. Library constructor/start/use entry.
		var library = entries.filter(function (object) {
			return object.entryKind === 'library_api' || object.entryKind === 'exported_api';
		});
		if (library.length) {
			var libAction = overviewFirstActionFromEntry(library[0]);
			if (libAction) {
				libAction.label = msg('main.overview.first_action.library');
				libAction.reason = msg('main.overview.first_action.reason.library');
				libAction.authority = msg('main.overview.first_action.authority.library');
				libAction.kind = 'library';
				return libAction;
			}
		}
		// 3. Exact process-entry fallback. The backend remains authoritative
		// about whether an entry is primary; this tier does not relabel a
		// secondary entry as primary. Prefer an executable whose basename
		// matches the repository/module basename, then use only bounded stable
		// source-locator tie-breakers.
		var processFallback = overviewProcessEntryFallback(entries);
		if (processFallback) {
			var processFallbackAction = overviewFirstActionFromEntry(processFallback);
			if (processFallbackAction) {
				processFallbackAction.label = msg('main.overview.first_action.process_entry_fallback');
				processFallbackAction.reason = msg('main.overview.first_action.reason.process_entry_fallback');
				processFallbackAction.authority = msg('main.overview.first_action.authority.process_entry');
				processFallbackAction.kind = 'process_entry_fallback';
				return processFallbackAction;
			}
		}
		// 4. A core Study theme's first exact reading (theme ordinal is
		// irrelevant — the first reading with an exact source wins, with
		// constructor/start symbols ranked ahead deterministically).
		var cards = themeCards();
		var ranked = [];
		for (var cardIndex = 0; cardIndex < cards.length; cardIndex++) {
			var card = cards[cardIndex];
			if (!card || !Array.isArray(card.readings)) continue;
			for (var readingIndex = 0; readingIndex < card.readings.length; readingIndex++) {
				var reading = card.readings[readingIndex];
				if (!reading || !reading.path || !Number.isInteger(reading.line)) continue;
				var readingLocation = {
					path: reading.path,
					line: reading.line,
					column: Number.isInteger(reading.column) && reading.column > 0 ? reading.column : 0,
				};
				var resolution = exactOverviewActionResolutionForLocation(readingLocation);
				if (!resolution || resolution.conflict || !resolution.source) continue;
				ranked.push({
					reading: reading,
					location: readingLocation,
					source: resolution.source,
				});
			}
		}
		ranked.sort(function (left, right) {
			var rank = function (entry) {
				var symbol = String(entry.reading.symbol || entry.reading.label || '');
				if (/^(New|Create|Start|Run|Serve|Launch|Init|Open)/.test(symbol)) return 0;
				return 1;
			};
			var leftRank = rank(left), rightRank = rank(right);
			if (leftRank !== rightRank) return leftRank - rightRank;
			// Decision 221 C: permutation invariance — a deterministic
			// tie-breaker (symbol, then path, then line) so reordering theme
			// cards never changes the first action.
			var leftSymbol = String(left.reading.symbol || left.reading.label || '');
			var rightSymbol = String(right.reading.symbol || right.reading.label || '');
			if (leftSymbol !== rightSymbol) return leftSymbol < rightSymbol ? -1 : 1;
			if (left.location.path !== right.location.path) {
				return left.location.path < right.location.path ? -1 : 1;
			}
			return left.location.line - right.location.line;
		});
		if (ranked.length) {
			var chosen = ranked[0];
			var chosenReading = chosen.reading;
			var themeAction = {
				label: msg('main.overview.first_action.study_reading'),
				path: chosen.location.path,
				line: chosen.location.line,
				symbol: chosenReading.symbol || chosenReading.label || '',
				reason: msg('main.overview.first_action.reason.study_reading'),
				action: function () {
					openSourceSnippet(chosen.source.snippet, chosen.location, false, { drawerFirst: true });
				},
				authority: msg('main.overview.first_action.authority.study_reading'),
				kind: 'study_reading',
			};
			return themeAction;
		}
		// 5. Explicit unavailable state — never an invented path.
		return null;
	}

	function overviewProcessEntryFallback(entries) {
		var repositoryBasenames = Object.create(null);
		var addRepositoryBasename = function (value) {
			var segments = String(value || '').replace(/\\/g, '/').split('/').filter(function (segment) {
				return !!segment && segment !== '.';
			});
			while (segments.length && /^v[0-9]+$/i.test(segments[segments.length - 1])) segments.pop();
			if (!segments.length) return;
			var basename = segments[segments.length - 1].replace(/\.git$/i, '').toLowerCase();
			if (basename) repositoryBasenames[basename] = true;
		};
		addRepositoryBasename(DATA.repo_name);
		var graph = DATA.repository_graph || {};
		(Array.isArray(graph.modules) ? graph.modules : []).forEach(function (module) {
			if (!module) return;
			addRepositoryBasename(module.path || module.module_path || module.name || '');
		});
		var pathParts = function (value) {
			return String(value || '').replace(/\\/g, '/').split('/').filter(function (segment) {
				return !!segment && segment !== '.';
			});
		};
		var executableMatchesRepository = function (entry) {
			var parts = pathParts(entry && entry.entryExecutable);
			if (!parts.length) return false;
			return !!repositoryBasenames[String(parts[parts.length - 1]).replace(/\.git$/i, '').toLowerCase()];
		};
		var candidates = (Array.isArray(entries) ? entries : []).filter(function (entry) {
			return entry && entry.entryKind === 'process_entry' &&
				entry.entryRole !== 'primary_application' && entry.entryRole !== 'test_or_helper';
		});
		candidates.sort(function (left, right) {
			var leftMatch = executableMatchesRepository(left), rightMatch = executableMatchesRepository(right);
			if (leftMatch !== rightMatch) return leftMatch ? -1 : 1;
			var leftPath = String(left.location && left.location.path || '');
			var rightPath = String(right.location && right.location.path || '');
			var leftDepth = pathParts(leftPath).length, rightDepth = pathParts(rightPath).length;
			if (leftDepth !== rightDepth) return leftDepth - rightDepth;
			if (leftPath.length !== rightPath.length) return leftPath.length - rightPath.length;
			if (leftPath !== rightPath) return leftPath < rightPath ? -1 : 1;
			var leftLine = Number(left.location && left.location.line) || 0;
			var rightLine = Number(right.location && right.location.line) || 0;
			if (leftLine !== rightLine) return leftLine - rightLine;
			var leftID = String(left.id || ''), rightID = String(right.id || '');
			if (leftID === rightID) return 0;
			return leftID < rightID ? -1 : 1;
		});
		return candidates[0] || null;
	}

	function overviewFirstActionFromEntry(object) {
		if (!object || !object.location) return null;
		var resolution = exactOverviewActionResolutionForLocation(object.location);
		if (!resolution || resolution.conflict || !resolution.source) return null;
		return {
			label: msg('main.overview.first_action.generic'),
			path: object.location.path,
			line: object.location.line,
			symbol: object.title || '',
			reason: '',
			action: function () {
				openSourceSnippet(object.snippet || resolution.source.snippet, object.location, false, { drawerFirst: true });
			},
			authority: '',
			kind: 'entry',
		};
	}

	// Decision 236: the default Map selection is the repository itself. Keep
	// that answer compact and beside the Canvas instead of reviving the former
	// Overview wall. Every collection rendered here has a closed DOM bound;
	// full component, Study and package collections remain in their existing
	// product surfaces.
	function renderMapEmptyInspectorStart(action) {
		var block = el('section', 'rm-map-empty-inspector__start');
		if (!action || !action.path || !Number.isInteger(action.line) || action.line < 1 || typeof action.action !== 'function') {
			block.appendChild(txt(
				'p',
				'rm-map-empty-inspector__unavailable',
				msg('main.overview.first_action.unavailable')
			));
			return block;
		}
		var location = { path: action.path, line: Number(action.line), column: 0 };
		var control = sourceActionElement(
			action.label,
			'rm-map-empty-inspector__start-action',
			location,
			0,
			action.action
		);
		if (!control) {
			block.appendChild(txt(
				'p',
				'rm-map-empty-inspector__unavailable',
				msg('main.overview.first_action.unavailable')
			));
			return block;
		}
		control.setAttribute('data-rm-map-start-here', String(action.kind || 'exact_source'));
		var label = action.path + ':' + Number(action.line);
		if (action.symbol) label += ' · ' + action.symbol;
		control.appendChild(txt('code', 'rm-map-empty-inspector__start-location', label));
		block.appendChild(control);
		return block;
	}

	function renderMapEmptyInspector() {
		var anatomy = repositoryOverviewAnatomy() || {
			entries: { objects: [], total: 0, omitted: 0 },
			components: {
				objects: [], total: 0,
				remainderCounts: { packages: 0, symbols: 0, other: 0 },
				hasDiagnosticRemainder: false,
			},
		};
		var atlasShelf = repositoryAtlasWorkspaceShelf();
		var root = el('div', 'rm-map-empty-inspector');
		root.id = 'rm-map-empty-inspector';
		root.setAttribute('aria-label', msg('architecture.nav.start_here'));
		root.appendChild(renderMapEmptyInspectorStart(overviewFirstAction(anatomy, atlasShelf)));
		return root;
	}

	function setMapEmptyInspectorDetailVisible(detailVisible) {
		if (!mapEmptyInspectorHost) return;
		mapEmptyInspectorHost.hidden = !!detailVisible;
		if (mapPrimaryLayoutHost) {
			mapPrimaryLayoutHost.classList.toggle('has-detail-inspector', !!detailVisible);
		}
	}

	// recommendedNextReading derives one next action from existing Study data:
	// the first source-backed theme card, else the first exact component
	// source, else nothing. It never invents a path.
	function recommendedNextReading(anatomy) {
		var cards = themeCards();
		if (cards.length) {
			var card = cards[0];
			if (card && card.readings && card.readings.length && card.readings[0].label) {
				return card.readings[0].label;
			}
			if (card && card.final_title) return card.final_title;
		}
		var componentObjects = anatomy && anatomy.components && anatomy.components.objects || [];
		for (var index = 0; index < componentObjects.length; index++) {
			var sources = componentObjects[index].sources || [];
			if (sources.length && sources[0].symbol) return sources[0].symbol;
		}
		return '';
	}

	// renderRepositoryPerimeter projects the repository perimeter
	// (Decision 229 D3, design contract v4): observed use/entry →
	// analyzed repository scope ⋯ observed touchpoints. It is not C4 System
	// Context: every touchpoint is an observation in exact member scope from
	// the D225 association rows, never a runtime dependency or external
	// system identity.
	function renderRepositoryPerimeter(anatomy) {
		var entries = anatomy && anatomy.entries && anatomy.entries.objects || [];
		if (!entries.length) return null;
		var associations = DATA.architecture_associations || {};
		var touchpointFamilies = {};
		(Array.isArray(associations.components) ? associations.components : []).forEach(function (component) {
			(Array.isArray(component.associations) ? component.associations : []).forEach(function (row) {
				if (!row) return;
				var family = String(row.imported_family || row.name || '');
				if (!family) return;
				var key = String(row.kind || '') + '\u0000' + family;
				if (!touchpointFamilies[key]) {
					touchpointFamilies[key] = {
						kind: String(row.kind || ''),
						family: family,
						count: 0,
						componentCount: 0,
						components: {},
					};
				}
				touchpointFamilies[key].count += Number(row.observation_count) || 1;
				var componentName = String(component.name || component.component_id || '');
				if (!touchpointFamilies[key].components[componentName]) {
					touchpointFamilies[key].components[componentName] = true;
					touchpointFamilies[key].componentCount += 1;
				}
			});
		});
		var families = Object.keys(touchpointFamilies).map(function (key) {
			return touchpointFamilies[key];
		}).sort(function (left, right) {
			return right.count - left.count || String(left.family).localeCompare(String(right.family));
		}).slice(0, 8);
		if (!families.length) return null;
		var scope = DATA.architecture_canvas || {};
		var packageCount = 0;
		(Array.isArray(scope.components) ? scope.components : []).forEach(function (component) {
			packageCount += Array.isArray(component.members) ? component.members.length : 0;
		});
		var section = el('section', 'rm-overview-perimeter');
		section.appendChild(renderViewHeading(
			msg('main.overview.perimeter.kicker'),
			msg('main.overview.perimeter.title'),
			msg('main.overview.perimeter.copy')
		));
		var flow = el('div', 'rm-overview-perimeter__flow');
		flow.appendChild(txt('div', 'rm-overview-perimeter__entry', msg('main.overview.perimeter.entry')));
		flow.appendChild(txt('div', 'rm-overview-perimeter__arrow', '\u2193'));
		flow.appendChild(txt('div', 'rm-overview-perimeter__scope',
			msg('main.overview.perimeter.scope', { count: packageCount })));
		flow.appendChild(txt('div', 'rm-overview-perimeter__dots', '\u22ef'));
		flow.appendChild(txt('div', 'rm-overview-perimeter__touchpoints', msg('main.overview.perimeter.touchpoints')));
		section.appendChild(flow);
		var list = el('ul', 'rm-overview-perimeter__families');
		families.forEach(function (family) {
			var item = el('li', 'rm-overview-perimeter__family rm-overview-perimeter__family--' + family.kind);
			item.appendChild(txt('span', 'rm-overview-perimeter__family-kind', msg(family.kind === 'boundary' ? 'main.overview.perimeter.kind.boundary' : 'main.overview.perimeter.kind.resource')));
			item.appendChild(txt('span', 'rm-overview-perimeter__family-name', family.family));
			item.appendChild(txt('span', 'rm-overview-perimeter__family-count',
				msg('main.overview.perimeter.observed_count', { count: family.count })));
			list.appendChild(item);
		});
		section.appendChild(list);
		return section;
	}

	// renderRepositoryOverviewAnatomy renders the anatomy zones in fixed
	// order: glance, perimeter, entries, system spine, diagnostics remainder
	// disclosure, integrations, study directions.
	function renderRepositoryOverviewAnatomy(root, anatomy, includeBrief) {
		if (includeBrief !== false) renderRepositoryOverviewLead(root);

		var glance = renderOverviewAtAGlance(anatomy);
		if (glance) root.appendChild(glance);

		// Decision 229 D3: repository-perimeter projection (not C4) —
		// observed use/entry → analyzed repository scope ⋯ observed
		// touchpoints. Touchpoints are observations in exact member scope
		// (D225 association rows), never runtime dependencies.
		var perimeter = renderRepositoryPerimeter(anatomy);
		if (perimeter) root.appendChild(perimeter);

		if (anatomy.entries.objects.length) {
			var entryGroups = overviewEntryClassifications(anatomy.entries.objects);
			if (entryGroups.length) {
				var entrySection = el('section', 'rm-workspace-section rm-overview-anatomy-zone');
				entrySection.appendChild(renderViewHeading(
					msg('main.overview.anatomy.entry_surfaces_kicker'),
					msg('main.overview.anatomy.entry_surfaces'),
					msg('main.overview.anatomy.entry_surfaces_copy')
				));
				entryGroups.forEach(function (group) {
					var groupBlock = el('div', 'rm-overview-entry-group');
					groupBlock.appendChild(txt('h4', 'rm-overview-entry-group__label',
						msg('main.overview.entry.group_label', {
							label: msg(group.labelMessageID),
							count: group.items.length,
						})));
					// Decision 233 (Archive 9): every category summary shows
					// the exact total count, 1-3 representative exact
					// entries, and a "Show all N" action; the full set is
					// one disclosure away. Do not render hundreds of handler
					// cards above the fold.
					var representativeCount = Math.min(group.items.length, 3);
					var grid = el('div', 'rm-overview-object-grid');
					group.items.slice(0, representativeCount).forEach(function (object) {
						grid.appendChild(renderOverviewObjectCard(object, 'surface'));
					});
					groupBlock.appendChild(grid);
					var isVisible = group.key === 'primary' || group.key === 'service' ||
						(group.key === 'tooling' && overviewEntryGroupIsPrimaryTooling(group)) ||
						group.key === 'library' || group.key === 'worker';
					if (group.items.length > representativeCount) {
						var allButton = el('button', 'rm-quiet-action rm-overview-entry-group__show-all');
						allButton.type = 'button';
						allButton.textContent = msg('main.overview.entry.show_all', { count: group.items.length });
						// F4 (fresh review): the hidden full grid is built
						// LAZILY on first click — a hundreds-of-routes
						// category pays no upfront render cost for cards
						// that are not yet visible.
						var allGrid = null;
						allButton.addEventListener('click', function () {
							if (allGrid === null) {
								allGrid = el('div', 'rm-overview-object-grid rm-overview-entry-group__all');
								group.items.slice(representativeCount).forEach(function (object) {
									allGrid.appendChild(renderOverviewObjectCard(object, 'surface'));
								});
							}
							allGrid.hidden = !allGrid.hidden;
							allButton.textContent = allGrid.hidden
								? msg('main.overview.entry.show_all', { count: group.items.length })
								: msg('main.overview.entry.hide_all');
							if (allGrid.parentNode !== groupBlock) {
								groupBlock.appendChild(allGrid);
							}
						});
						groupBlock.appendChild(allButton);
					}
					if (isVisible) {
						entrySection.appendChild(groupBlock);
					} else {
						var disclosure = el('details', 'rm-overview-entry-disclosure');
						var summary = el('summary', '');
						summary.appendChild(txt('span', 'rm-overview-entry-disclosure__count',
							msg('main.overview.entry.disclosure_count', { count: group.items.length })));
						disclosure.appendChild(summary);
						disclosure.appendChild(groupBlock);
						entrySection.appendChild(disclosure);
					}
				});
				// F3 (fresh review): value-shaped/unresolved/unavailable
				// entries are dropped before classification; they stay
				// COUNTED and reachable under a bounded disclosure so the
				// category summary reconciles to the complete surface
				// catalog (entries.total = Σ groups + omitted).
				if (anatomy.entries && anatomy.entries.omitted) {
					var omittedBlock = el('details', 'rm-overview-entry-disclosure');
					var omittedSummary = el('summary', '');
					omittedSummary.appendChild(txt('span', 'rm-overview-entry-disclosure__count',
						msg('main.overview.entry.unclassified_count', { count: anatomy.entries.omitted })));
					var omittedNote = el('p', 'rm-overview-entry-omitted-note');
					omittedNote.textContent = msg('main.overview.entry.unclassified_copy');
					omittedBlock.appendChild(omittedSummary);
					omittedBlock.appendChild(omittedNote);
					entrySection.appendChild(omittedBlock);
				}
				root.appendChild(entrySection);
			} else {
				root.appendChild(renderOverviewAnatomyZone(
					msg('main.overview.anatomy.entry_surfaces_kicker'),
					msg('main.overview.anatomy.entry_surfaces'),
					msg('main.overview.anatomy.entry_surfaces_copy'),
					anatomy.entries,
					'surface'
				));
			}
		}
		if (anatomy.components.objects.length) {
			// Decision 218 (C): Overview shows the system spine — at most one
			// representative card per supported role — instead of every
			// component with equal weight. All other components stay
			// reachable through the Architecture route; nothing is deleted
			// from report data. When no component classifies into a spine
			// role, the full component zone remains (never hide content).
			var spine = overviewSystemSpine(anatomy);
			var spineHasComponentRoles = spine && spine.roles.some(function (card) { return !card.isEntry; });
			var spineView = spineHasComponentRoles ? renderOverviewSystemSpine(spine) : null;
			if (spineView) {
				root.appendChild(spineView);
			} else {
				root.appendChild(renderOverviewAnatomyZone(
					msg('main.overview.anatomy.components_kicker'),
					msg('main.overview.anatomy.components'),
					msg('main.overview.anatomy.components_copy'),
					anatomy.components,
					'component'
				));
			}
		}

		// Decision 229 D3: unclassified exact scope (diagnostic remainder,
		// e.g. "Supporting repository evidence") is a collapsed disclosure —
		// never a principal product area. Count-reconciled, always reachable.
		var remainderCounts = anatomy.components && anatomy.components.remainderCounts ||
			{ packages: 0, symbols: 0, other: 0 };
		var remainderCount = (Number(remainderCounts.packages) || 0) +
			(Number(remainderCounts.symbols) || 0) + (Number(remainderCounts.other) || 0);
		if (anatomy.components && anatomy.components.hasDiagnosticRemainder && remainderCount > 0) {
			var remainderSection = el('section', 'rm-workspace-section rm-overview-remainder');
			var remainderDetails = el('details', 'rm-overview-remainder__disclosure');
			remainderSection.appendChild(remainderDetails);
			remainderDetails.appendChild(txt('summary', 'rm-overview-remainder__summary',
				msg('main.overview.remainder.summary', remainderCounts)));
			remainderDetails.appendChild(txt('p', 'rm-overview-remainder__note',
				msg('main.overview.remainder.copy')));
			root.appendChild(remainderSection);
		}

		if (anatomy.integrations && anatomy.integrations.length) {
			root.appendChild(renderOverviewAnatomyZone(
				msg('main.overview.anatomy.integrations_kicker'),
				msg('main.overview.anatomy.integrations'),
				msg('main.overview.anatomy.integrations_copy'),
				anatomy.integrations,
				'integration'
			));
		}

		if (COMPLETE_STUDY_DIRECTIONS.length) {
			var directions = el('section', 'rm-workspace-section rm-overview-study-routes');
			directions.appendChild(renderViewHeading(
				msg('main.overview.anatomy.study_kicker'),
				msg('main.overview.anatomy.study_directions'),
				msg('main.overview.anatomy.study_directions_copy')
			));
			var directionGrid = el('div', 'rm-mechanism-grid');
			COMPLETE_STUDY_DIRECTIONS.forEach(function (direction, index) {
				directionGrid.appendChild(renderStudyDirectionCard(direction, index));
			});
			directions.appendChild(directionGrid);
			root.appendChild(directions);
		}

		// Decision 220 E: honest zero-handler copy. When a surface analysis
		// ran but no exact HTTP handler or server start was resolved, the
		// surfaces area says so with the unresolved-candidate count instead
		// of reading as proof that no handlers exist.
		var surfaces = DATA.discovered_surfaces;
		if (surfaces && surfaces.total_count === 0 && !anatomy.entries.objects.length) {
			var unresolved = Number(surfaces.unresolved_handler_count) || 0;
			var possible = Number(surfaces.possible_registration_count) || 0;
			var candidates = unresolved + possible;
			var copy = candidates > 0
				? msg('main.overview.surfaces.zero_handlers_candidates', { count: candidates })
				: msg('main.overview.surfaces.zero_handlers_none');
			root.appendChild(txt('p', 'rm-overview-zero-handlers', copy));
		}
	}

	var REPOSITORY_ATLAS_UNIT_KIND_MESSAGE_IDS = {
		app: 'main.atlas.workspace.unit_kind.app',
		module: 'main.atlas.workspace.unit_kind.module',
		package: 'main.atlas.workspace.unit_kind.package',
		repository: 'main.atlas.workspace.unit_kind.repository',
		service: 'main.atlas.workspace.unit_kind.service',
	};

	function repositoryAtlasUnitKindLabel(kind) {
		return msg(
			REPOSITORY_ATLAS_UNIT_KIND_MESSAGE_IDS[String(kind || '')] ||
				'main.atlas.workspace.unit_kind.other'
		);
	}

	var REPOSITORY_ATLAS_COALESCED_UNIT_KINDS = {
		app: true,
		module: true,
		repository: true,
	};

	var REPOSITORY_ATLAS_UNIT_KIND_ORDER = {
		repository: 0,
		module: 1,
		app: 2,
	};

	function repositoryAtlasTopologyDisplayUnits(units) {
		var groupsByName = Object.create(null);
		units.forEach(function (unit) {
			if (!REPOSITORY_ATLAS_COALESCED_UNIT_KINDS[unit.kind]) return;
			if (!groupsByName[unit.name]) groupsByName[unit.name] = [];
			groupsByName[unit.name].push(unit);
		});
		// Decision 218 (F): repository and module units become structural
		// headers when child applications/libraries exist under them. A unit
		// with a same-name peer of a different container kind (repository +
		// module) coalesces into one structural header; the children render
		// under it, never as equal-weight peer cards.
		var emitted = Object.create(null);
		var displayUnits = [];
		var childByParent = Object.create(null);
		units.forEach(function (unit) {
			if (unit.parentID && !childByParent[unit.parentID]) childByParent[unit.parentID] = [];
			if (unit.parentID) childByParent[unit.parentID].push(unit);
		});
		var isStructuralHeader = function (unit) {
			var children = childByParent[unit.id] || [];
			return children.some(function (child) {
				return child.kind === 'app' || child.kind === 'service';
			});
		};
		units.forEach(function (unit) {
			var peers = groupsByName[unit.name] || [];
			var kinds = Object.create(null);
			var coalescible = peers.length > 1 && peers.every(function (peer) {
				if (kinds[peer.kind]) return false;
				kinds[peer.kind] = true;
				return true;
			});
			if (!coalescible) {
				displayUnits.push({
					name: unit.name, kinds: [unit.kind], unitIDs: [unit.id],
					structural: isStructuralHeader(unit),
					childCount: (childByParent[unit.id] || []).length,
				});
				return;
			}
			if (emitted[unit.name]) return;
			emitted[unit.name] = true;
			displayUnits.push({
				name: unit.name,
				kinds: peers.map(function (peer) { return peer.kind; }).sort(function (left, right) {
					return (REPOSITORY_ATLAS_UNIT_KIND_ORDER[left] || 0) -
						(REPOSITORY_ATLAS_UNIT_KIND_ORDER[right] || 0) || left.localeCompare(right);
				}),
				unitIDs: peers.map(function (peer) { return peer.id; }),
				structural: peers.some(isStructuralHeader),
				childCount: peers.reduce(function (total, peer) {
					return total + (childByParent[peer.id] || []).length;
				}, 0),
			});
		});
		return displayUnits;
	}

	function repositoryAtlasPackageRepresentativeLocation(packageName) {
		var graph = DATA.repository_graph || {};
		var packages = Array.isArray(graph.packages) ? graph.packages : [];
		var pkg = null;
		packages.some(function (candidate) {
			if (candidate && candidate.canonical_package_path === packageName) {
				pkg = candidate;
				return true;
			}
			return false;
		});
		if (!pkg) return null;
		var files = (Array.isArray(pkg.files) ? pkg.files : []).map(String).filter(function (filePath) {
			return !!(filePath && OPENABLE_PATH_SET[filePath]);
		}).sort();
		if (!files.length) return null;
		var location = { path: files[0], line: 1, column: 0 };
		return sourceLocationActionAvailable(location) ? location : null;
	}

	function repositoryAtlasPackageGroups(packageUnits, unitsByID) {
		var groups = [];
		var groupsByKey = Object.create(null);
		packageUnits.forEach(function (unit) {
			var parent = unitsByID[unit.parentID];
			var exactModuleParent = parent && parent.kind === 'module' ? parent : null;
			var key = exactModuleParent ? 'module\u0000' + exactModuleParent.id : 'unparented';
			if (!groupsByKey[key]) {
				groupsByKey[key] = { module: exactModuleParent, units: [] };
				groups.push(groupsByKey[key]);
			}
			groupsByKey[key].units.push(unit);
		});
		groups.forEach(function (group) {
			var prefix = group.module && group.module.name || '';
			group.prefix = prefix && group.units.every(function (unit) {
				return unit.name === prefix || unit.name.indexOf(prefix + '/') === 0;
			}) ? prefix : '';
			// Owner preference: packages are a plain sorted list. The module's
			// root package comes first, its sub-packages follow lexically, and
			// unrelated (external) package names sort last — even when the
			// prefix is not factored (mixed module, e.g. one external package).
			var moduleName = group.module && group.module.name || '';
			group.units.sort(function (left, right) {
				var lm = moduleName && (left.name === moduleName);
				var rm = moduleName && (right.name === moduleName);
				if (lm && !rm) return -1;
				if (!lm && rm) return 1;
				var li = moduleName ? left.name.indexOf(moduleName + '/') === 0 : false;
				var ri = moduleName ? right.name.indexOf(moduleName + '/') === 0 : false;
				if (li && !ri) return -1;
				if (!li && ri) return 1;
				return left.name < right.name ? -1 : left.name > right.name ? 1 : 0;
			});
		});
		// Groups themselves are sorted by module name so the shelf is stable;
		// unparented packages (no exact module parent) always sort last.
		groups.sort(function (left, right) {
			var l = left.module && left.module.name || '';
			var r = right.module && right.module.name || '';
			if (!l && !r) return 0;
			if (!l) return 1;
			if (!r) return -1;
			return l < r ? -1 : l > r ? 1 : 0;
		});
		return groups;
		}

	function repositoryAtlasWorkspaceShelf() {
		var atlas = DATA.repository_atlas;
		if (!atlas || !Array.isArray(atlas.units) || !atlas.units.length) return null;
		var unitCounts = Object.create(null);
		atlas.units.forEach(function (unit) {
			if (unit && typeof unit.id === 'string' && unit.id) {
				unitCounts[unit.id] = (unitCounts[unit.id] || 0) + 1;
			}
		});
		var units = [];
		var topologyUnits = [];
		var packageUnits = [];
		var unitsByID = Object.create(null);
		atlas.units.forEach(function (unit) {
			if (!unit || typeof unit.id !== 'string' || !unit.id || unitCounts[unit.id] !== 1 ||
				typeof unit.name !== 'string' || !unit.name) return;
			var view = {
				id: unit.id,
				parentID: typeof unit.parent_id === 'string' ? unit.parent_id : '',
				name: unit.name,
				kind: String(unit.kind || ''),
			};
			units.push(view);
			if (view.kind === 'package') packageUnits.push(view);
			else topologyUnits.push(view);
			unitsByID[unit.id] = view;
		});

		var entityCounts = Object.create(null);
		(Array.isArray(atlas.entities) ? atlas.entities : []).forEach(function (entity) {
			if (entity && typeof entity.id === 'string' && entity.id) {
				entityCounts[entity.id] = (entityCounts[entity.id] || 0) + 1;
			}
		});
		var entitiesByID = Object.create(null);
		(Array.isArray(atlas.entities) ? atlas.entities : []).forEach(function (entity) {
			if (!entity || typeof entity.id !== 'string' || !entity.id || entityCounts[entity.id] !== 1 ||
				!unitsByID[entity.unit_id]) return;
			entitiesByID[entity.id] = entity;
		});

		var evidenceCounts = Object.create(null);
		(Array.isArray(atlas.evidence) ? atlas.evidence : []).forEach(function (evidence) {
			if (evidence && typeof evidence.id === 'string' && evidence.id) {
				evidenceCounts[evidence.id] = (evidenceCounts[evidence.id] || 0) + 1;
			}
		});
		var evidenceByID = Object.create(null);
		(Array.isArray(atlas.evidence) ? atlas.evidence : []).forEach(function (evidence) {
			if (!evidence || typeof evidence.id !== 'string' || !evidence.id ||
				evidenceCounts[evidence.id] !== 1 || !unitsByID[evidence.unit_id]) return;
			evidenceByID[evidence.id] = evidence;
		});
		var unitSources = Object.create(null);
		var unitSourceConflicts = Object.create(null);
		Object.keys(evidenceByID).forEach(function (evidenceID) {
			var evidence = evidenceByID[evidenceID];
			if (!evidence.location || !evidence.provenance ||
				evidence.provenance.provider !== 'gofacts' ||
				evidence.provenance.version !== 'package-declaration-v1' ||
				evidence.provenance.operation !== 'package_declaration') return;
			var resolution = exactOverviewActionResolutionForLocation(evidence.location);
			if (resolution.conflict) {
				unitSourceConflicts[evidence.unit_id] = true;
				return;
			}
			if (!resolution.source) return;
			var relatedEvidenceIDs = resolution.source.snippet &&
				Array.isArray(resolution.source.snippet.related_evidence_ids)
				? resolution.source.snippet.related_evidence_ids : [];
			if (resolution.source.snippet && relatedEvidenceIDs.indexOf(evidenceID) < 0) return;
			if (!unitSources[evidence.unit_id]) unitSources[evidence.unit_id] = Object.create(null);
			var location = resolution.source.location;
			var key = location.path + '\u0000' + String(location.line) + '\u0000' +
				String(location.column || 0) + '\u0000' + (resolution.source.snippet
					? overviewSnippetStableKey(resolution.source.snippet) : 'exact-location');
			unitSources[evidence.unit_id][key] = resolution.source;
		});
		packageUnits.forEach(function (unit) {
			var sources = Object.keys(unitSources[unit.id] || {}).sort().map(function (key) {
				return unitSources[unit.id][key];
			});
			unit.source = !unitSourceConflicts[unit.id] && sources.length === 1 ? sources[0] : null;
			unit.sourceState = unitSourceConflicts[unit.id] || sources.length > 1
				? 'conflict' : unit.source ? 'available' : 'unavailable';
			// P7-B: without an exact saved source, a package row still
			// navigates to its representative file — the first openable
			// file of the package in the saved repository graph, in sorted
			// path order. The package itself has no single line; the file
			// is the exact boundary a reader can open. No openable file →
			// the row stays a plain unavailable reference.
			if (!unit.source && unit.sourceState === 'unavailable') {
				unit.representativeLocation = repositoryAtlasPackageRepresentativeLocation(unit.name);
			}
		});

		var eligible = 0;
		var relations = [];
		(Array.isArray(atlas.relations) ? atlas.relations : []).forEach(function (relation) {
			if (!relation || relation.kind !== 'exposes' || relation.phase !== 'startup' ||
				relation.authority !== 'resolved' || !unitsByID[relation.unit_id] ||
				!relation.source || relation.source.kind !== 'surface' ||
				!relation.target || relation.target.kind !== 'operation') return;
			var sourceEntity = entitiesByID[relation.source.id];
			var targetEntity = entitiesByID[relation.target.id];
			if (!sourceEntity || sourceEntity.kind !== 'surface' ||
				!targetEntity || targetEntity.kind !== 'operation') return;
			eligible += 1;
			var source = null;
			(Array.isArray(relation.evidence_refs) ? relation.evidence_refs : []).some(function (ref) {
				var evidence = evidenceByID[ref];
				if (!evidence || !evidence.location) return false;
				var resolution = exactOverviewSourceResolutionFromSnippets(evidence.location, USER_SOURCES);
				if (!resolution.source || resolution.conflict) return false;
				source = resolution.source;
				return true;
			});
			if (!source) return;
			relations.push({
				id: relation.id,
				surfaceID: relation.source.id,
				applicationID: relation.target.id,
				evidenceIDs: (relation.evidence_refs || []).slice(),
				unit: unitsByID[relation.unit_id],
				authority: relation.authority,
				snippet: source.snippet,
				location: source.location,
			});
		});
		return {
			units: units,
			topologyUnits: topologyUnits,
			topologyDisplayUnits: repositoryAtlasTopologyDisplayUnits(topologyUnits),
			packageUnits: packageUnits,
			packageGroups: repositoryAtlasPackageGroups(packageUnits, unitsByID),
			relations: relations,
			omittedRelations: eligible - relations.length,
		};
	}

	function renderRepositoryAtlasUnitGrid(units) {
		var unitGrid = el('div', 'rm-atlas-unit-grid');
		units.forEach(function (unit) {
			if (unit.structural) {
				// Decision 218 (F): repository/module containers with child
				// applications render as structural section headers with a
				// child count, not equal-weight peer cards.
				var header = el('div', 'rm-atlas-unit-header');
				var heading = el('h4', 'rm-atlas-unit-header__name');
				heading.appendChild(txt('span', '', unit.name));
				if (unit.childCount > 0) {
					heading.appendChild(txt('span', 'rm-atlas-unit-header__count', msg(
						'main.atlas.workspace.children_count',
						{ count: unit.childCount }
					)));
				}
				header.appendChild(heading);
				var tags = el('div', 'rm-atlas-unit-tags');
				(Array.isArray(unit.kinds) ? unit.kinds : [unit.kind]).forEach(function (kind) {
					tags.appendChild(txt('span', 'rm-atlas-unit-tag', repositoryAtlasUnitKindLabel(kind)));
				});
				header.appendChild(tags);
				unitGrid.appendChild(header);
				return;
			}
			var card = el('article', 'rm-atlas-unit-card');
			card.appendChild(txt('strong', '', unit.name));
			var tags = el('div', 'rm-atlas-unit-tags');
			(Array.isArray(unit.kinds) ? unit.kinds : [unit.kind]).forEach(function (kind) {
				tags.appendChild(txt('span', 'rm-atlas-unit-tag', repositoryAtlasUnitKindLabel(kind)));
			});
			card.appendChild(tags);
			unitGrid.appendChild(card);
		});
		return unitGrid;
	}

	function repositoryAtlasPackageSourceStateLabel(state, packageName) {
		return msg(
			state === 'conflict'
				? 'main.atlas.workspace.package_source_conflict_label'
				: 'main.atlas.workspace.package_source_unavailable_label',
			{ package: packageName }
		);
	}

	function renderRepositoryAtlasPackageList(groups) {
		var container = el('div', 'rm-atlas-package-groups');
		groups.forEach(function (group) {
			var packageGroup = el('section', 'rm-atlas-package-group');
			var unavailableCount = 0;
			var conflictCount = 0;
			if (group.prefix) {
				packageGroup.appendChild(txt('code', 'rm-atlas-package-prefix', group.prefix + '/'));
			}
			var list = el('ul', 'rm-atlas-package-list');
			group.units.forEach(function (unit) {
				var item = el('li', 'rm-atlas-package-row');
				var label = group.prefix
					? (unit.name === group.prefix ? msg('main.atlas.workspace.package_root') :
						unit.name.slice(group.prefix.length + 1))
					: unit.name;
				if (unit.source) {
					// Decision 222: source actions are GitHub/GitLab jumps
					// (new tab) or server open actions — never an inline
					// code drawer. Without either, no action is offered.
					var action = sourceActionElement(
						'',
						'rm-atlas-package-action',
						unit.source.location,
						0,
						function () { openSourceLocation(unit.source.location); }
					);
					if (!action) {
						item.appendChild(txt('code', 'rm-atlas-package-name', label));
						list.appendChild(item);
						return;
					}
					action.setAttribute('aria-label', msg(
						'main.atlas.workspace.open_package_source',
						{ package: unit.name }
					));
					action.appendChild(txt('code', 'rm-atlas-package-name', label));
					action.appendChild(txt('span', 'rm-atlas-package-open', '↗'));
					item.appendChild(action);
				} else {
					// P7-B: a package without an exact saved source still
					// opens its representative file when the repository
					// graph proves one (GitHub/GitLab jump or server open).
					// Without either, the row stays a plain unavailable
					// reference — no dead button, no drawer.
					var representative = unit.representativeLocation || null;
					var representativeAction = representative
						? sourceActionElement(
							'',
							'rm-atlas-package-action',
							representative,
							0,
							function () { openSourceLocation(representative); }
						)
						: null;
					if (representativeAction) {
						representativeAction.setAttribute('aria-label', msg(
							'main.atlas.workspace.open_package_source',
							{ package: unit.name }
						));
						representativeAction.appendChild(txt('code', 'rm-atlas-package-name', label));
						representativeAction.appendChild(txt('span', 'rm-atlas-package-open', '↗'));
						item.appendChild(representativeAction);
						list.appendChild(item);
						return;
					}
					var unavailable = el('span', 'rm-atlas-package-unavailable');
					var sourceStateLabel = repositoryAtlasPackageSourceStateLabel(unit.sourceState, unit.name);
					if (unit.sourceState === 'conflict') conflictCount += 1;
					else unavailableCount += 1;
					unavailable.setAttribute('aria-disabled', 'true');
					unavailable.setAttribute('aria-label', sourceStateLabel);
					unavailable.setAttribute('title', sourceStateLabel);
					unavailable.setAttribute('data-rm-source-state', unit.sourceState || 'unavailable');
					unavailable.appendChild(txt('code', 'rm-atlas-package-name', label));
					item.appendChild(unavailable);
				}
				list.appendChild(item);
			});
			packageGroup.appendChild(list);
			if (unavailableCount || conflictCount) {
				var summary = el('p', 'rm-atlas-package-source-summary');
				if (unavailableCount) summary.appendChild(txt(
					'span',
					'',
					msg('main.atlas.workspace.package_sources_unavailable_count', { count: unavailableCount })
				));
				if (conflictCount) summary.appendChild(txt(
					'span',
					'',
					msg('main.atlas.workspace.package_sources_conflict_count', { count: conflictCount })
				));
				packageGroup.appendChild(summary);
			}
			container.appendChild(packageGroup);
		});
		return container;
	}

	function renderRepositoryAtlasCompactDiagnostics(shelf) {
		var startupUnavailable = !shelf.relations.length;
		if (!startupUnavailable) return null;
		var diagnostics = el('section', 'rm-atlas-compact-status');
		diagnostics.appendChild(txt('strong', 'rm-atlas-compact-status__title', msg(
			'main.atlas.workspace.compact_status'
		)));
		var list = el('ul', 'rm-atlas-compact-status__list');
		if (startupUnavailable) {
			var startupItem = el('li', 'rm-atlas-compact-status__item');
			startupItem.setAttribute('data-rm-status-code', 'no_exact_source_backed_relations');
			startupItem.appendChild(txt('span', 'rm-atlas-compact-status__label', msg(
				'main.atlas.workspace.startup_relations'
			)));
			startupItem.appendChild(txt('span', '', msg(
				'main.atlas.workspace.no_source_backed_relations'
			)));
			if (shelf.omittedRelations > 0) startupItem.appendChild(txt('span', '', msg(
				'main.atlas.workspace.omitted_relations',
				{ count: shelf.omittedRelations }
			)));
			list.appendChild(startupItem);
		}
		diagnostics.appendChild(list);
		return diagnostics;
	}

	function renderRepositoryAtlasWorkspaceShelf(root, compactOverview) {
		var shelf = repositoryAtlasWorkspaceShelf();
		var section = el(
			'section',
			'rm-workspace-section rm-atlas-shelf' + (compactOverview ? ' rm-atlas-shelf--overview' : '')
		);
		if (!compactOverview) {
			section.appendChild(renderViewHeading(
				msg('main.atlas.workspace.kicker'),
				msg('main.atlas.workspace.title'),
				msg('main.atlas.workspace.copy')
			));
		}
		if (!shelf || !shelf.units.length) {
			if (compactOverview) section.appendChild(txt(
				'h3', 'rm-atlas-shelf-heading', msg('main.atlas.workspace.units')
			));
			section.appendChild(txt('p', 'rm-empty-state', msg('main.atlas.workspace.unavailable')));
			root.appendChild(section);
			return;
		}
		section.appendChild(txt('h3', 'rm-atlas-shelf-heading rm-atlas-units-heading', msg('main.atlas.workspace.units')));
		if (shelf.topologyDisplayUnits.length) {
			section.appendChild(renderRepositoryAtlasUnitGrid(shelf.topologyDisplayUnits));
		}
		if (shelf.packageUnits.length) {
			var packageDisclosure = el('details', 'rm-atlas-package-disclosure');
			packageDisclosure.appendChild(txt(
				'summary',
				'rm-atlas-package-summary',
				msg('main.atlas.workspace.packages', { count: shelf.packageUnits.length })
			));
			packageDisclosure.appendChild(renderRepositoryAtlasPackageList(shelf.packageGroups));
			section.appendChild(packageDisclosure);
		}

		if (!compactOverview && shelf.relations.length) {
			section.appendChild(txt(
				'h3',
				'rm-atlas-shelf-heading rm-atlas-relations-heading',
				msg('main.atlas.workspace.startup_relations')
			));
			var relationGrid = el('div', 'rm-atlas-relation-grid');
			shelf.relations.forEach(function (relation) {
				var card = el('article', 'rm-atlas-relation-card');
				card.appendChild(txt('strong', 'rm-atlas-relation-unit', relation.unit.name));
				var roles = el('div', 'rm-atlas-relation-roles');
				roles.appendChild(txt('span', '', msg('main.atlas.workspace.process_entry')));
				roles.appendChild(txt('span', '', msg('main.atlas.workspace.application_start')));
				card.appendChild(roles);
				card.appendChild(txt('span', 'rm-atlas-authority', msg(
					'main.atlas.workspace.authority',
					{ authority: msg('main.atlas.workspace.authority.resolved') }
				)));
				// Decision 222: source actions are GitHub/GitLab jumps (new
				// tab) or server open actions — never an inline code drawer.
				// Without either, the exact source stays a plain reference.
				var sourceButton = sourceActionElement(
					msg('main.open.exact.source'),
					'rm-atlas-source-action',
					relation.location || sourceSnippetLocation(relation.snippet),
					relation.snippet && relation.snippet.end_line || 0,
					function () {
						openSourceSnippet(relation.snippet, relation.location, false, { drawerFirst: true });
					}
				);
				if (sourceButton) card.appendChild(sourceButton);
				relationGrid.appendChild(card);
			});
			section.appendChild(relationGrid);
		}
		if (!compactOverview && shelf.relations.length && shelf.omittedRelations > 0) {
			section.appendChild(txt('p', 'rm-atlas-omitted', msg(
				'main.atlas.workspace.omitted_relations',
				{ count: shelf.omittedRelations }
			)));
		}
		if (!compactOverview) {
			var compactDiagnostics = renderRepositoryAtlasCompactDiagnostics(shelf);
			if (compactDiagnostics) section.appendChild(compactDiagnostics);
		}
		root.appendChild(section);
	}

	// Decision 236 (v11): the repository summary (thesis/brief/atlas shelf,
	// entry perimeter, entries, remainder — the former Overview content)
	// renders into the Map workspace when no architecture canvas exists.
	function renderMapSummaryInto(sectionID) {
		var root = document.getElementById(sectionID);
		if (!root) return;
		root.replaceChildren();
		renderAtlasStudyFailedBrowse(root);
		var anatomy = repositoryOverviewAnatomy();
		if (anatomy) {
			renderRepositoryOverviewLead(root);
			renderRepositoryOverviewAnatomy(root, anatomy, false);
			// The Atlas unit ontology is demoted below the user-facing
			// orientation (Decision 217): units describe the local package
			// structure, they do not precede entry points and components.
			renderRepositoryAtlasWorkspaceShelf(root, true);
			return;
		}
		if (STUDY_MAP && COMPLETE_STUDY_DIRECTIONS.length) {
			renderRepositoryOverviewLead(root);
			renderStudyMapOverview(root, false);
			return;
		}
		renderRepositoryAtlasWorkspaceShelf(root);
		if (STUDY_MAP) {
			renderStudyMapOverview(root);
			return;
		}

// repomap-source-episode:start
		var sourceEpisode = renderSourceEpisode(SOURCE_EPISODE);
		if (sourceEpisode) root.appendChild(sourceEpisode);
// repomap-source-episode:end

		var thesis = REPOSITORY_GUIDE || DATA.repository_thesis || {};
		var overviewPurpose = thesis.purpose || DATA.project_guess;
    var hero = el('section', 'rm-overview-hero rm-purpose-hero');
    hero.appendChild(txt('div', 'rm-view-kicker', msg('main.purpose')));
    hero.appendChild(txt('h2', '', DATA.repo_name || msg('main.repository.overview')));
    hero.appendChild(txt('p', '', overviewPurpose ? normalizeMarkdownProse(overviewPurpose) : msg('main.explore.how.this.repository.is.organized.and.implemented')));
    if (DATA.documented_purpose && overviewPurpose === DATA.documented_purpose) {
      hero.appendChild(txt('span', 'rm-purpose-source', purposeSourceLabel()));
    }
    root.appendChild(hero);

    var areas = Array.isArray(thesis.areas) ? thesis.areas.slice(0, 7) : [];
    var areaCards = areas.map(renderRepositoryArea).filter(Boolean);
    if (areaCards.length) {
      var shapeSection = el('section', 'rm-workspace-section');
      shapeSection.appendChild(renderViewHeading(msg('main.repository.shape'), msg('main.code.areas.to.know'), msg('main.open.a.concrete.source.location.or.continue.on.the.architecture.map')));
      var areaGrid = el('div', 'rm-repository-area-grid');
      areaCards.forEach(function (card) { areaGrid.appendChild(card); });
      shapeSection.appendChild(areaGrid);
      root.appendChild(shapeSection);
    }

		renderOperationsOverview(root);

		var systemStory = Array.isArray(thesis.system_story) ? thesis.system_story.filter(Boolean) : [];
		if (systemStory.length) {
			var storySection = el('section', 'rm-workspace-section');
			storySection.appendChild(renderViewHeading(msg('main.system.story'), msg('main.how.the.parts.fit.together'), msg('main.a.compact.orientation.assembled.from.repository.documentation.and.existing.code.areas')));
			var story = el('ol', 'rm-system-story');
			systemStory.forEach(function (item) { story.appendChild(txt('li', '', item)); });
			storySection.appendChild(story);
			root.appendChild(storySection);
		}

		var hasArchitecture = userArchitectureAvailable();
    if (hasArchitecture) {
      var exploreSection = el('section', 'rm-workspace-section rm-overview-explore');
      exploreSection.appendChild(renderViewHeading(msg('main.explore'), msg('main.inspect.the.wider.repository'), msg('main.open.the.repository.map.for.additional.context')));
      var actions = el('div', 'rm-overview-actions');
      if (hasArchitecture) {
        var architecture = txt('button', 'rm-secondary-action', msg('main.explore.architecture'));
        architecture.type = 'button';
        architecture.onclick = function () { openArchitectureTarget(null, null); };
        actions.appendChild(architecture);
      }
      exploreSection.appendChild(actions);
      root.appendChild(exploreSection);
    }
  }

  function formatCodeLocation(location) {
    if (!location) return '';
    return location.path + (location.line ? ':' + location.line : '') + (location.column ? ':' + location.column : '');
  }

  function sourceSnippetHasCode(snippet) {
    return !!(snippet && snippet.path && Array.isArray(snippet.lines) && snippet.lines.length);
  }

  function sourceSnippetAvailable(snippet) {
    if (!snippet || !snippet.path) return false;
    if (staticSourceLocationAvailable(snippet.path, snippet.start_line, snippet.end_line)) return true;
    return sourceSnippetHasCode(snippet);
  }

  function sourceSnippetIdentity(snippet) {
    if (!snippet) return '';
    return String(snippet.path || '') + '\u0000' + String(snippet.enclosing_symbol || '');
  }

  function uniqueSourceSnippets(snippets, excluded, limit, byPath) {
    var result = [];
    var seen = {};
    Object.keys(excluded || {}).forEach(function (key) { seen[key] = true; });
    (Array.isArray(snippets) ? snippets : []).forEach(function (snippet) {
      if (!sourceSnippetAvailable(snippet) || result.length >= limit) return;
      var key = byPath ? String(snippet.path || '') : sourceSnippetIdentity(snippet);
      if (!key || seen[key]) return;
      seen[key] = true;
      result.push(snippet);
    });
    return result;
  }

  function overviewSourceReason(snippet) {
    if (!snippet) return '';
    return String(
      snippet.reason ||
      snippet.presentation_landmark_reason ||
      snippet.landmark_reason ||
      snippet.selection_reason ||
      ''
    ).trim();
  }

  function overviewSourceRoleLabel(snippet) {
    var kind = String(snippet && (snippet.landmark_kind || snippet.role) || 'source');
    var labels = {
      cli_entrypoint: msg('main.overview.role.cli_entrypoint'),
      public_api: msg('main.overview.role.public_api'),
      quickstart: msg('main.overview.role.quickstart'),
      quickstart_example: msg('main.overview.role.quickstart'),
      start_here: msg('main.start.here'),
      orientation_start: msg('main.start.here'),
      constructor: msg('main.overview.role.constructor'),
      handler: msg('main.overview.role.handler'),
      test: msg('main.overview.role.test'),
      example: msg('main.overview.role.example'),
      core: msg('main.overview.role.core'),
      implementation: msg('main.implementation'),
      entrypoint: msg('main.start.here'),
    };
    return labels[kind] || msg('main.source.code');
  }

  function overviewSourceIsStrong(snippet) {
    var kind = String(snippet && (snippet.landmark_kind || snippet.role) || '');
    return ['cli_entrypoint', 'public_api', 'quickstart', 'quickstart_example'].indexOf(kind) >= 0;
  }

  function sourceSnippetLocation(snippet, fallback) {
    fallback = fallback || {};
    var line = Number(fallback.line) || 0;
    if (!line && snippet && Array.isArray(snippet.highlight_ranges) && snippet.highlight_ranges.length) {
      line = Number(snippet.highlight_ranges[0].start_line) || 0;
    }
    if (!line) line = Number(snippet && snippet.start_line) || 0;
    return {
      path: fallback.path || snippet.path,
      line: line,
      column: Number(fallback.column) || 0,
    };
  }

  function allEmbeddedSourceSnippets() {
    var result = USER_SOURCES.slice();
		if (TASK_INVESTIGATION) {
			(TASK_INVESTIGATION.anchors || []).forEach(function (anchor) {
				if (anchor && anchor.source) result.push(anchor.source);
			});
		}
		STUDY_DIRECTIONS.forEach(function (direction) {
			(direction.reading_anchors || []).forEach(function (reading) {
				if (reading && reading.source) result.push(reading.source);
			});
			(direction.documents || []).forEach(function (document) {
				if (document && document.source) result.push(document.source);
			});
		});
		// D213: theme card readings carry exact source; embedded snippets stay
		// available for the browse source actions.
		themeCards().forEach(function (card) {
			(card.readings || []).forEach(function (reading) {
				if (reading && reading.source) result.push(reading.source);
			});
		});
		((STUDY_MAP && STUDY_MAP.shape) || []).forEach(function (area) {
			if (area && area.source) result.push(area.source);
		});
		function appendOperationalReference(reference) {
			if (reference && !reference.redacted && reference.source) result.push(reference.source);
		}
		PAVED_PATHS.forEach(function (pavedPath) {
			(pavedPath.prerequisites || []).forEach(appendOperationalReference);
			(pavedPath.actions || []).forEach(function (action) {
				if (action) appendOperationalReference(action.reference);
			});
			(pavedPath.expected_results || []).forEach(function (result) {
				if (result) appendOperationalReference(result.reference);
			});
			(pavedPath.expected || []).forEach(appendOperationalReference);
			(pavedPath.troubleshooting || []).forEach(appendOperationalReference);
		});
		OPERATIONAL_LANDMARKS.forEach(function (landmark) {
			if (landmark) appendOperationalReference(landmark.reference);
		});
    return result;
  }

  function embeddedSourceForLocation(location) {
    if (!location || !location.path) return null;
    var snippets = allEmbeddedSourceSnippets().filter(function (snippet) {
      return sourceSnippetHasCode(snippet) && snippet.path === location.path;
    });
    if (!snippets.length) return null;
    var line = Number(location.line) || 0;
    if (line) {
      for (var index = 0; index < snippets.length; index++) {
        if (line >= Number(snippets[index].start_line) && line <= Number(snippets[index].end_line)) {
          return snippets[index];
        }
      }
      return null;
    }
    return snippets[0];
  }

  function sourceLineHighlighted(snippet, line) {
    if (line && typeof line.highlight === 'boolean') return line.highlight;
    var lineNumber = Number(line && line.line) || 0;
    return (snippet.highlight_ranges || []).some(function (range) {
      return lineNumber >= Number(range.start_line) && lineNumber <= Number(range.end_line);
    });
  }

  function sourceHighlightRanges(snippet) {
    var ranges = Array.isArray(snippet && snippet.highlight_ranges)
      ? snippet.highlight_ranges.slice()
      : [];
    if (ranges.length) return ranges;
    (snippet && snippet.lines || []).forEach(function (line) {
      if (line && line.highlight) {
        ranges.push({ start_line: Number(line.line), end_line: Number(line.line) });
      }
    });
    return ranges;
  }

  function remainingExactReferences(locations, primary) {
    if (!Array.isArray(locations)) return [];
    if (!sourceSnippetAvailable(primary)) return locations.slice();
    var ranges = sourceHighlightRanges(primary);
    return locations.filter(function (location) {
      if (!location || location.path !== primary.path) return true;
      var start = Number(location.line || location.start_line) || 0;
      var end = Number(location.end_line || location.line || location.start_line) || start;
      if (!start) return true;
      return !ranges.some(function (range) {
        return start >= Number(range.start_line) && end <= Number(range.end_line);
      });
    });
  }

  function renderSourceCode(snippet, sourceLines) {
    var code = el('div', 'rm-source-code');
    code.setAttribute('data-source-path', snippet.path || '');
    code.setAttribute('data-source-content', 'true');
    (sourceLines || snippet.lines || []).forEach(function (line) {
      if (line.gap_before) {
        var gap = el('div', 'rm-source-code__gap');
        gap.appendChild(txt('span', '', '⋮'));
        gap.appendChild(txt('span', '', msg('main.lines_omitted')));
        code.appendChild(gap);
      }
      var row = el('div', 'rm-source-code__line' + (sourceLineHighlighted(snippet, line) ? ' is-highlighted' : ''));
      row.setAttribute('data-source-line', String(line.line || ''));
      row.appendChild(txt('span', 'rm-source-code__number', String(line.line || '')));
      row.appendChild(txt('code', 'rm-source-code__text', line.text == null ? '' : String(line.text)));
      code.appendChild(row);
    });
    return code;
  }

	function isMarkdownDocumentSource(snippet) {
		return !!(snippet && /\.(md|mdx)$/i.test(String(snippet.path || '')));
	}

	function studyDocumentForSnippet(direction, snippet) {
		if (!direction || !snippet) return null;
		var documents = Array.isArray(direction.documents) ? direction.documents : [];
		for (var index = 0; index < documents.length; index++) {
			var source = documents[index] && documents[index].source;
			if (!sourceSnippetHasCode(source) || source.path !== snippet.path) continue;
			if (source.presentation_sha256 && snippet.presentation_sha256 &&
				source.presentation_sha256 !== snippet.presentation_sha256) continue;
			return documents[index];
		}
		return null;
	}

	function appendReadableDocumentInline(root, value) {
		var textValue = String(value || '');
		var tokenPattern = /(`[^`\n]+`|\[[^\]\n]+\]\([^)]+\))/g;
		var offset = 0;
		var match;

		function appendPlainText(plain) {
			var safe = String(plain || '')
				.replace(/<!--.*?-->/g, '')
				.replace(/<\/?[A-Za-z][^>]*>/g, '');
			if (safe) root.appendChild(txt('span', '', safe));
		}

		while ((match = tokenPattern.exec(textValue)) !== null) {
			appendPlainText(textValue.slice(offset, match.index));
			var token = match[0];
			if (token.charAt(0) === '`') {
				root.appendChild(txt('code', 'rm-readable-document__inline-code', token.slice(1, -1)));
			} else {
				var link = token.match(/^\[([^\]]+)\]\(([^)]+)\)$/);
				if (link) {
					var safeHref = /^https?:\/\//i.test(link[2]) ? link[2] : '';
					var label = txt(safeHref ? 'a' : 'span', 'rm-readable-document__link', link[1]);
					label.setAttribute('title', link[2]);
					if (safeHref) {
						label.setAttribute('href', safeHref);
						label.setAttribute('target', '_blank');
						label.setAttribute('rel', 'noreferrer noopener');
					}
					root.appendChild(label);
				}
			}
			offset = match.index + token.length;
		}
		appendPlainText(textValue.slice(offset));
	}

	function renderReadableDocument(snippet) {
		var article = el('article', 'rm-readable-document');
		var paragraphLines = [];
		var quoteLines = [];
		var list = null;
		var codeLines = null;
		var fenceCharacter = '';
		var insideHTMLComment = false;
		var insideHTMLTag = false;

		function flushParagraph() {
			if (!paragraphLines.length) return;
			var paragraph = el('p', '');
			appendReadableDocumentInline(paragraph, paragraphLines.join(' '));
			if (paragraph.childNodes.length) article.appendChild(paragraph);
			paragraphLines = [];
		}

		function flushQuote() {
			if (!quoteLines.length) return;
			var quote = el('blockquote', '');
			var paragraph = el('p', '');
			appendReadableDocumentInline(paragraph, quoteLines.join(' '));
			if (paragraph.childNodes.length) {
				quote.appendChild(paragraph);
				article.appendChild(quote);
			}
			quoteLines = [];
		}

		function flushList() {
			if (list) article.appendChild(list.node);
			list = null;
		}

		function flushTextBlocks() {
			flushParagraph();
			flushQuote();
			flushList();
		}

		function flushCode() {
			if (codeLines === null) return;
			var pre = el('pre', '');
			pre.appendChild(txt('code', '', codeLines.join('\n')));
			article.appendChild(pre);
			codeLines = null;
			fenceCharacter = '';
		}

		(snippet.lines || []).forEach(function (sourceLine) {
			var raw = sourceLine && sourceLine.text != null ? String(sourceLine.text) : '';
			var trimmed = raw.trim();
			if (codeLines !== null) {
				var closesBackticks = fenceCharacter === '`' && /^```+\s*$/.test(trimmed);
				var closesTildes = fenceCharacter === '~' && /^~~~+\s*$/.test(trimmed);
				if (closesBackticks || closesTildes) flushCode();
				else codeLines.push(raw);
				return;
			}

			if (insideHTMLComment) {
				if (trimmed.indexOf('-->') >= 0) insideHTMLComment = false;
				return;
			}
			if (trimmed.indexOf('<!--') === 0) {
				flushTextBlocks();
				insideHTMLComment = trimmed.indexOf('-->') < 0;
				return;
			}
			if (insideHTMLTag) {
				if (trimmed.indexOf('>') >= 0) insideHTMLTag = false;
				return;
			}
			if (trimmed.charAt(0) === '<' && trimmed.indexOf('>') < 0) {
				flushTextBlocks();
				insideHTMLTag = true;
				return;
			}

			var fence = trimmed.match(/^(```+|~~~+)/);
			if (fence) {
				flushTextBlocks();
				codeLines = [];
				fenceCharacter = fence[1].charAt(0);
				return;
			}
			if (!trimmed) {
				flushTextBlocks();
				return;
			}
			var isMDXDirective = /^(import|export)\s/.test(trimmed);
			var isHTMLTag = /^<\/?[A-Za-z][^>]*>\s*$/.test(trimmed);
			if (isMDXDirective || isHTMLTag) {
				flushTextBlocks();
				return;
			}

			var heading = trimmed.match(/^(#{1,6})\s+(.+)$/);
			if (heading) {
				flushTextBlocks();
				var level = Math.min(6, heading[1].length + 2);
				var headingNode = el('h' + level, '');
				appendReadableDocumentInline(headingNode, heading[2].replace(/\s+#+\s*$/, ''));
				if (headingNode.childNodes.length) article.appendChild(headingNode);
				return;
			}

			var quote = trimmed.match(/^>\s?(.*)$/);
			if (quote) {
				flushParagraph();
				flushList();
				quoteLines.push(quote[1]);
				return;
			}

			var unordered = trimmed.match(/^[-*+]\s+(.+)$/);
			var ordered = trimmed.match(/^\d+[.)]\s+(.+)$/);
			if (unordered || ordered) {
				flushParagraph();
				flushQuote();
				var orderedList = !!ordered;
				if (!list || list.ordered !== orderedList) {
					flushList();
					list = { ordered: orderedList, node: el(orderedList ? 'ol' : 'ul', '') };
				}
				var item = el('li', '');
				appendReadableDocumentInline(item, (ordered || unordered)[1]);
				if (item.childNodes.length) list.node.appendChild(item);
				return;
			}

			flushQuote();
			flushList();
			paragraphLines.push(trimmed);
		});
		flushTextBlocks();
		flushCode();
		return article;
	}

	function renderReadableDocumentCard(snippet, location, reference) {
		var card = el('section', 'rm-source-card rm-readable-document-card');
		var heading = el('div', 'rm-source-card__heading');
		var title = el('div', '');
		title.appendChild(txt('div', 'rm-source-card__role', reference.label || msg('main.chrome.documentation')));
		title.appendChild(txt('strong', '', snippet.path));
		title.appendChild(txt(
			'code',
			'rm-source-card__location',
			msg('main.source.location_lines', {
				path: snippet.path,
				start: snippet.start_line,
				end: snippet.end_line,
			})
		));
		heading.appendChild(title);
		if (snippet.revision && DEBUG_MODE) {
			heading.appendChild(txt('span', 'rm-source-card__snapshot', msg('main.saved.snapshot')));
		}
		card.appendChild(heading);

		var readable = renderReadableDocument(snippet);
		card.appendChild(readable);
		var rawHost = el('div', 'rm-readable-document-card__raw');
		rawHost.hidden = true;
		card.appendChild(rawHost);

		var actions = renderSourceActions(snippet, sourceSnippetLocation(snippet, location), {});
		var raw = txt('button', 'rm-secondary-action', msg('main.show.raw.exact.source'));
		raw.type = 'button';
		var showingRaw = false;
		raw.onclick = function () {
			showingRaw = !showingRaw;
			readable.hidden = showingRaw;
			rawHost.hidden = !showingRaw;
			rawHost.replaceChildren();
			if (showingRaw) rawHost.appendChild(renderSourceCode(snippet));
			raw.textContent = showingRaw ? msg('main.show.readable.document') : msg('main.show.raw.exact.source');
		};
		actions.appendChild(raw);
		card.appendChild(actions);
		return card;
	}

  function requestSourceContext(snippet) {
    var runID = currentRunID();
    var contextID = SOURCE_CONTEXT_IDS[snippet.presentation_sha256];
    if (!serverMode() || !runID || !contextID) return Promise.resolve(null);
    return fetch(serverBasePath() + '/api/source-context', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Repomap-Action': 'read-source-context',
      },
      body: JSON.stringify({ run_id: runID, context_id: contextID }),
    }).then(function (response) {
      return response.json().catch(function () { return {}; }).then(function (body) {
        if (!response.ok) throw new Error(msg('main.error.source_context_unavailable'));
        return body;
      });
    }).then(function (body) {
      if (!body || !body.source || !Array.isArray(body.source.lines) || !body.source.lines.length) return null;
      var expanded = {
        path: body.source.path,
        language: snippet.language,
        enclosing_symbol: snippet.enclosing_symbol,
        start_line: body.source.start_line,
        end_line: body.source.end_line,
        highlight_ranges: snippet.highlight_ranges || [],
        content_sha256: snippet.content_sha256,
        presentation_sha256: snippet.presentation_sha256,
        role: snippet.role,
        revision: snippet.revision,
        source_complete: !!body.source.source_complete,
        lines: body.source.lines.map(function (line) {
          var highlighted = (snippet.highlight_ranges || []).some(function (range) {
            return line.line >= range.start_line && line.line <= range.end_line;
          });
          return { line: line.line, text: line.text, highlight: highlighted };
        }),
      };
      return expanded;
    });
  }

  function renderSourceActions(snippet, location, options) {
    options = options || {};
    var actions = el('div', 'rm-source-actions');
    var staticSourceOpen = staticSourceMode() ? sourceActionElement(
      staticSourceOpenLabel(),
      'rm-primary-action rm-source-action-link',
      location,
      Number(snippet && snippet.end_line) || 0,
      function () { openStaticSource(location, Number(snippet && snippet.end_line) || 0); }
    ) : null;
    if (staticSourceOpen) {
      actions.appendChild(staticSourceOpen);
    }
    if (serverMode() && currentRunID() && SOURCE_IDS[snippet.path]) {
      var open = txt('button', 'rm-primary-action', msg('main.open.in.editor'));
      open.type = 'button';
      open.onclick = function () { requestOpenFile(snippet.path, location.line, location.column); };
      actions.appendChild(open);
    }
    if (!options.expanded && serverMode() && SOURCE_CONTEXT_IDS[snippet.presentation_sha256]) {
      var more = txt('button', 'rm-secondary-action', msg('main.show.more.context'));
      more.type = 'button';
      more.onclick = function () {
        more.disabled = true;
        requestSourceContext(snippet).then(function (expanded) {
          more.disabled = false;
          if (expanded) openSourceSnippet(expanded, location, true);
        }).catch(function () {
          more.disabled = false;
          showToast(msg('main.error.source_context_unavailable'), true);
        });
      };
      actions.appendChild(more);
    }
    if (typeof options.toggleFullFunction === 'function') {
      var full = txt('button', 'rm-secondary-action', msg('main.show.full.function'));
      full.type = 'button';
      full.onclick = function () { options.toggleFullFunction(full); };
      actions.appendChild(full);
    }
    var copyLocation = txt('button', 'rm-secondary-action', msg('main.copy.file.line'));
    copyLocation.type = 'button';
    copyLocation.onclick = function () { copyText(formatCodeLocation(location)); };
    actions.appendChild(copyLocation);
    var copyPath = txt('button', 'rm-quiet-action', msg('main.copy.path'));
    copyPath.type = 'button';
    copyPath.onclick = function () { copyText(snippet.path); };
    actions.appendChild(copyPath);
    return actions;
  }

  function sourceNoticeRanges(notice, snippet) {
    if (!notice || notice.path !== snippet.path || !Array.isArray(notice.supporting_ranges)) return [];
    var highlights = Array.isArray(snippet.highlight_ranges) ? snippet.highlight_ranges : [];
    var ranges = notice.supporting_ranges.filter(function (range) {
      var start = Number(range && range.start_line) || 0;
      var end = Number(range && range.end_line) || 0;
      if (!start || end < start) return false;
      return highlights.some(function (highlight) {
        return start >= Number(highlight.start_line) && end <= Number(highlight.end_line);
      });
    });
    return ranges.length === notice.supporting_ranges.length ? ranges : [];
  }

  function sourceNoticeLineLabel(ranges) {
    var labels = ranges.map(function (range) {
      return Number(range.start_line) === Number(range.end_line)
        ? msg('main.source.notice_line', { line: range.start_line })
        : msg('main.source.notice_lines', {
          start: range.start_line,
          end: range.end_line,
        });
    });
    return labels.join(', ');
  }

  function renderSourceNotices(notices, snippet, card) {
    var safe = (Array.isArray(notices) ? notices : []).map(function (notice) {
      return { notice: notice, ranges: sourceNoticeRanges(notice, snippet) };
    }).filter(function (item) {
      return item.ranges.length && String(item.notice.text || '').trim();
    }).slice(0, 2);
    if (!safe.length) return null;
    var section = el('aside', 'rm-source-notices');
    section.appendChild(txt('h4', '', msg('main.chrome.what.to.notice')));
    var list = el('ul', '');
    safe.forEach(function (item) {
      var listItem = el('li', '');
      var jump = txt('button', 'rm-source-notice__line', sourceNoticeLineLabel(item.ranges));
      jump.type = 'button';
      jump.onclick = function () {
        if (!card || typeof card.querySelector !== 'function') return;
        var firstLine = item.ranges[0].start_line;
        var row = card.querySelector('[data-source-line="' + firstLine + '"]');
        if (row && typeof row.scrollIntoView === 'function') {
          row.scrollIntoView({ behavior: 'smooth', block: 'center' });
        }
      };
      listItem.appendChild(jump);
      listItem.appendChild(txt('span', '', item.notice.text));
      list.appendChild(listItem);
    });
    section.appendChild(list);
    return section;
  }

  function renderSourceSnippetCard(snippet, options) {
    options = options || {};
    var card = el(
      'section',
      'rm-source-card' + (options.primary ? ' is-primary' : '') + (options.overviewLandmark ? ' is-overview-landmark' : '')
    );
    var heading = el('div', 'rm-source-card__heading');
    var title = el('div', '');
    title.appendChild(txt(
      'div',
      'rm-source-card__role',
      options.roleLabel || (options.primary ? msg('main.primary_implementation') : msg('main.source.code'))
    ));
    title.appendChild(txt('strong', '', snippet.enclosing_symbol || snippet.path));
    var locationText = msg('main.source.location_lines', {
      path: snippet.path,
      start: snippet.start_line,
      end: snippet.end_line,
    });
    var locationLabel = staticSourceLink(
      locationText,
      'rm-source-card__location rm-file-link',
      { path: snippet.path, line: snippet.start_line },
      snippet.end_line
    ) || txt('code', 'rm-source-card__location', locationText);
    title.appendChild(locationLabel);
    heading.appendChild(title);
    if (!staticSourceMode() && snippet.revision && (DEBUG_MODE || options.showSnapshot)) {
      heading.appendChild(txt('span', 'rm-source-card__snapshot', msg('main.saved.snapshot')));
    }
    card.appendChild(heading);
    if (String(options.reason || '').trim()) {
      card.appendChild(txt('p', 'rm-source-card__reason', String(options.reason).trim()));
    }
    var location = sourceSnippetLocation(snippet, options.location);
    var codeHost = null;
    if (!staticSourceMode()) {
      var notices = renderSourceNotices(options.notices, snippet, card);
      if (notices) card.appendChild(notices);
      codeHost = el('div', 'rm-source-card__code');
      codeHost.appendChild(renderSourceCode(snippet));
      card.appendChild(codeHost);
    }
    var fullLines = Array.isArray(snippet.full_function_lines) ? snippet.full_function_lines : [];
    var showingFullFunction = false;
    var toggleFullFunction = null;
    if (fullLines.length && codeHost) {
      toggleFullFunction = function (button) {
        showingFullFunction = !showingFullFunction;
        codeHost.replaceChildren(renderSourceCode(snippet, showingFullFunction ? fullLines : snippet.lines));
        button.textContent = showingFullFunction ? msg('main.show_focused_excerpt') : msg('main.show.full.function');
        var start = showingFullFunction ? snippet.full_function_start_line : snippet.start_line;
        var end = showingFullFunction ? snippet.full_function_end_line : snippet.end_line;
        locationLabel.textContent = msg('main.source.location_lines', {
          path: snippet.path,
          start: start,
          end: end,
        });
      };
    }
		if (!options.redacted) {
			card.appendChild(renderSourceActions(snippet, location, {
				expanded: !!options.expanded,
				toggleFullFunction: toggleFullFunction,
			}));
		}
    return card;
  }

  function renderRelatedSourceTarget(snippet) {
    var card = el('article', 'rm-related-source');
    var meta = el('div', '');
    meta.appendChild(txt('strong', '', snippet.enclosing_symbol || snippet.path));
    meta.appendChild(renderFileReference(
      snippet.path,
      'rm-related-source__location',
      snippet.start_line,
      snippet.path + ':' + snippet.start_line + '–' + snippet.end_line
    ));
    var reason = overviewSourceReason(snippet);
    if (reason) meta.appendChild(txt('span', 'rm-related-source__reason', reason));
    card.appendChild(meta);
    var show = sourceActionElement(
      staticSourceMode() ? staticSourceOpenLabel() : msg('main.source.show_code'),
      'rm-secondary-action rm-source-action-link',
      sourceSnippetLocation(snippet),
      snippet.end_line,
      function () { openSourceSnippet(snippet, sourceSnippetLocation(snippet)); }
    );
    // Decision 222: without a GitHub/GitLab (or server) jump the related
    // source stays a plain reference — never a dead button.
    if (show) card.appendChild(show);
    return card;
  }

  function renderExactReferences(locations) {
    if (!Array.isArray(locations) || !locations.length) return null;
    var details = el('details', 'rm-exact-references');
    details.appendChild(txt('summary', '', msg('main.source.show_exact_references', {
      count: locations.length,
    })));
    var list = el('div', 'rm-exact-references__list');
    locations.forEach(function (location) {
      list.appendChild(renderFileReference(
        location.path,
        'rm-exact-reference',
        Number(location.line || location.start_line) || 0,
        formatCodeLocation(location)
      ));
    });
    details.appendChild(list);
    return details;
  }

	function sourceLocationActionAvailable(location) {
		if (!location || !location.path || !OPENABLE_PATH_SET[location.path]) return false;
		if (staticSourceMode()) {
			return !!staticSourceURL(
				location.path,
				Number(location.line) || 0,
				Number(location.end_line) || 0
			);
		}
		if (serverMode() && currentRunID() && SOURCE_IDS[location.path]) return true;
		return !!embeddedSourceForLocation(location);
	}

	function openSourceLocation(location) {
	  if (!location || !location.path || !OPENABLE_PATH_SET[location.path]) return;
	  if (openStaticSource(location, location.end_line)) return;
	  if (serverMode() && currentRunID() && SOURCE_IDS[location.path]) {
	    requestOpenFile(location.path, Number(location.line) || 0, Number(location.column) || 0);
	    return;
	  }
	  var snippet = embeddedSourceForLocation(location);
	  if (snippet) openSourceSnippet(snippet, location);
	}

  // Static source hosts and the manifest-authorized local server stay the
  // preferred source actions. A generic host has neither authority, so an
  // already embedded exact snippet opens in the existing source drawer.
  function openSourceSnippet(snippet, location, expanded, options) {
    options = options || {};
    var resolved = sourceSnippetLocation(snippet, location);
    if (openStaticSource(resolved, snippet && snippet.end_line)) return;
    if (serverMode() && currentRunID() && snippet && OPENABLE_PATH_SET[snippet.path]) {
      requestOpenFile(snippet.path, Number(resolved.line) || 0, Number(resolved.column) || 0);
	  return;
    }
	if (!sourceSnippetHasCode(snippet) || !OPENABLE_PATH_SET[snippet.path]) return;
	var next = reduceWorkspaceState(workspaceState, {
	  type: 'open_source',
	  selection: {
	    path: snippet.path,
	    line: resolved.line,
	    column: resolved.column,
	    snippet: snippet,
	    expanded: !!expanded,
	    drawerFirst: !!options.drawerFirst,
	  },
	});
	var reference = sourceDrawerHistoryReference(next.sourceLocation);
	var replacingDrawer = !!(window.history && window.history.state && window.history.state.sourceDrawer);
	writeWorkspaceHistory(
	  String(window.location && window.location.hash || workspaceHashForState(next)),
	  next,
	  { replace: replacingDrawer, sourceDrawer: reference }
	);
	workspaceState = next;
	renderSourceDrawer();
  }

  function closeSourceDrawer() {
    // Decision 217: focus returns to the trigger that opened the drawer.
    var drawer = document.getElementById('rm-source-drawer');
    if (drawer) {
      var trigger = drawer._rmTrigger;
      drawer._rmTrigger = null;
      if (trigger && typeof trigger.focus === 'function') trigger.focus();
    }
    if (window.history && window.history.state && window.history.state.sourceDrawer &&
        typeof window.history.back === 'function') {
      window.history.back();
      return;
    }
    workspaceState = reduceWorkspaceState(workspaceState, { type: 'close_source' });
    writeWorkspaceHistory(workspaceHashForState(workspaceState), workspaceState, { replace: true });
    renderSourceDrawer();
  }

	function activeSourceDrawerStudy() {
		if (workspaceState.view === 'study') {
			if (workspaceState.themeCardOrdinal) {
				var theme = themeCardByOrdinal(workspaceState.themeCardOrdinal);
				return theme ? {
					question: theme.final_question || theme.final_title || msg('main.study'),
					learning_outcome: theme.expected_learning || theme.why_it_matters || '',
				} : null;
			}
			return studyDirectionByID(workspaceState.directionID);
		}
		if ((workspaceState.view === 'map' || workspaceState.view === 'architecture') &&
			workspaceState.mapReturn && workspaceState.mapReturn.themeCardOrdinal) {
			var mapTheme = themeCardByOrdinal(workspaceState.mapReturn.themeCardOrdinal);
			return mapTheme ? {
				question: mapTheme.final_question || mapTheme.final_title || msg('main.study'),
				learning_outcome: mapTheme.expected_learning || mapTheme.why_it_matters || '',
			} : null;
		}
		return null;
	}

	function activeSourceDrawerOperation() {
		if (workspaceState.view !== 'operate') return null;
		return pavedPathByID(workspaceState.operationID);
	}

  function renderSourceDrawer() {
    var workspace = document.querySelector('.rm-workspace');
    var drawer = document.getElementById('rm-source-drawer');
    var content = document.getElementById('rm-source-drawer-content');
    var close = document.getElementById('rm-source-drawer-close');
    if (!workspace || !drawer || !content) return;
    if (staticSourceMode() && !(workspaceState.sourceLocation && workspaceState.sourceLocation.drawerFirst)) {
      workspaceState.sourceLocation = null;
      drawer.hidden = true;
      workspace.classList.remove('has-source-drawer');
      content.replaceChildren();
      return;
    }
    if (!workspaceState.sourceLocation) {
      drawer.hidden = true;
      workspace.classList.remove('has-source-drawer');
      content.replaceChildren();
      return;
    }
    var selection = workspaceState.sourceLocation;
    var snippet = selection.snippet;
    if (!sourceSnippetHasCode(snippet)) {
      closeSourceDrawer();
      return;
    }
    drawer.hidden = false;
    workspace.classList.add('has-source-drawer');
    if (close) close.onclick = closeSourceDrawer;
    // Decision 217: the source drawer is a real dialog — descriptive label,
    // aria-modal, focus trap, Escape closes, focus returns to the trigger.
    drawer.setAttribute('role', 'dialog');
    drawer.setAttribute('aria-modal', 'true');
    drawer.setAttribute('aria-label', msg('main.source.locations'));
    if (!drawer._rmDialogInstalled) {
      drawer._rmDialogInstalled = true;
      drawer.addEventListener('keydown', function (event) {
        if (event.key === 'Escape') {
          event.preventDefault();
          closeSourceDrawer();
          return;
        }
        if (event.key !== 'Tab') return;
        var focusables = drawer.querySelectorAll(
          'a[href], button:not([disabled]), textarea, input, select, [tabindex]:not([tabindex="-1"])'
        );
        if (!focusables.length) return;
        var first = focusables[0];
        var last = focusables[focusables.length - 1];
        if (event.shiftKey && document.activeElement === first) {
          event.preventDefault();
          last.focus();
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault();
          first.focus();
        }
      });
    }
    // Capture the opening trigger for focus return (only when one is active).
    if (!drawer._rmTrigger || !document.body.contains(drawer._rmTrigger)) {
      var active = document.activeElement;
      if (active && active !== document.body && active !== drawer && !drawer.contains(active)) {
        drawer._rmTrigger = active;
      }
    }
    content.replaceChildren();
			var study = activeSourceDrawerStudy();
			var operation = activeSourceDrawerOperation();
			var task = workspaceState.view === 'investigate' ? TASK_INVESTIGATION : null;
			content.appendChild(txt('div', 'rm-view-kicker', study ? msg('main.source.in.this.reading.path') : operation ? msg('main.source.in.this.operating.path') : task ? msg('main.source.in.this.task.investigation') : msg('main.saved.source')));
			if (study) {
			content.appendChild(txt('h2', '', study.question));
			content.appendChild(txt('p', '', study.learning_outcome || study.why_it_matters || msg('main.chrome.repository.reading.path')));
			}
			if (operation) {
				content.appendChild(txt('h2', '', operation.title));
				content.appendChild(txt('p', '', operation.goal || msg('main.chrome.repository.operating.path')));
			}
			if (task) {
				content.appendChild(txt('h2', '', task.interpretation && task.interpretation.restatement || msg('main.chrome.task.investigation')));
				content.appendChild(txt('p', '', task.interpretation && task.interpretation.observable_or_outcome || msg('main.chrome.bounded.saved.source')));
			}
			var documentReference = study ? studyDocumentForSnippet(study, snippet) : null;
			if (documentReference && isMarkdownDocumentSource(snippet)) {
				content.appendChild(renderReadableDocumentCard(
					snippet,
					{ path: selection.path, line: selection.line, column: selection.column },
					documentReference
				));
				return;
			}
    content.appendChild(renderSourceSnippetCard(snippet, {
      location: { path: selection.path, line: selection.line, column: selection.column },
      expanded: !!selection.expanded,
      showSnapshot: true,
    }));
  }

  // Decision 230 D7: closed deterministic fallback component names are
  // localized for the RU product. Only the exact deterministic set from
  // the local fallback (componentmap deterministicLocalComponents) is
  // translated; everything else passes through untouched.
  var DETERMINISTIC_COMPONENT_NAMES_RU = {
    'Primary application': 'Основное приложение',
    'Secondary services': 'Вторичные сервисы',
    'Tool entrypoints': 'Точки входа инструментов',
    'Test and helper entrypoints': 'Тестовые и вспомогательные точки входа',
    'Other process entrypoints': 'Прочие точки входа процессов',
    'Command dispatch': 'Диспетчеризация команд',
    'Extension registration': 'Регистрация расширений',
    'Lifecycle startup': 'Запуск жизненного цикла',
    'Request dispatch': 'Диспетчеризация запросов',
    'TLS and security boundary': 'TLS и граница безопасности',
    'Supporting repository evidence': 'Вспомогательные свидетельства репозитория',
  };
  var DETERMINISTIC_COMPONENT_DESCRIPTIONS_RU = {
    'Primary application': 'Точка входа процесса, названная по репозиторию, подтверждённая точным объявлением main.',
    'Secondary services': 'Сервисные точки входа репозитория, отличные от основного приложения.',
    'Tool entrypoints': 'Точки входа для разработки, сборки, релизов и обслуживания.',
    'Test and helper entrypoints': 'Тестовые, примерные и вспомогательные точки входа процессов.',
    'Other process entrypoints': 'Точные точки входа процессов, чья продуктовая роль не определена.',
    'Command dispatch': 'Детерминированная группировка по точным якорям command_dispatch.',
    'Extension registration': 'Детерминированная группировка по точным якорям registry_write.',
    'Lifecycle startup': 'Детерминированная группировка по точным якорям lifecycle_start.',
    'Request dispatch': 'Детерминированная группировка по точным якорям request_dispatch_root.',
    'TLS and security boundary': 'Детерминированная группировка по точным якорям tls_or_security_boundary.',
    'Supporting repository evidence': 'Точные локальные элементы, не отнесённые ограниченным набором якорей.',
  };

  function localizeDeterministicComponentName(name) {
    if (typeof name !== 'string') return name;
    return DETERMINISTIC_COMPONENT_NAMES_RU[name] || name;
  }

  function localizeDeterministicComponentDescription(name, description) {
    if (typeof description !== 'string' || typeof name !== 'string') return description;
    return DETERMINISTIC_COMPONENT_DESCRIPTIONS_RU[name] || description;
  }

  // Closed deterministic local fallback subsystem names (anchorFirstLocalLandscape).
  var DETERMINISTIC_SUBSYSTEM_NAMES_RU = {
    'Entry and dispatch': 'Вход и диспетчеризация',
    'Configuration': 'Конфигурация',
    'Runtime and extensions': 'Среда выполнения и расширения',
    'Control plane': 'Плоскость управления',
    'Request and data plane': 'Плоскость запросов и данных',
    'Security': 'Безопасность',
    'Supporting evidence': 'Вспомогательные свидетельства',
  };
  var DETERMINISTIC_SUBSYSTEM_DESCRIPTIONS_RU = {
    'Entry and dispatch': 'Якоря точек входа и диспетчеризации команд.',
    'Configuration': 'Якоря входа, адаптации и применения конфигурации.',
    'Runtime and extensions': 'Якоря реестра, расширений и жизненного цикла.',
    'Control plane': 'Административные якоря плоскости управления.',
    'Request and data plane': 'Якоря диспетчеризации запросов и плоскости данных приложения.',
    'Security': 'Якоря TLS и границ безопасности.',
    'Supporting evidence': 'Свидетельства пакетов, файлов, символов и потоков, сохранённые вне остальных групп.',
  };

  function localizeDeterministicSubsystemName(name) {
    if (typeof name !== 'string') return name;
    return DETERMINISTIC_SUBSYSTEM_NAMES_RU[name] || name;
  }

  function localizeDeterministicSubsystemDescription(name, description) {
    if (typeof description !== 'string' || typeof name !== 'string') return description;
    return DETERMINISTIC_SUBSYSTEM_DESCRIPTIONS_RU[name] || description;
  }

  function userArchitectureData() {
		if (!DATA.architecture_canvas || !userArchitectureAvailable()) return null;
    if (DEBUG_MODE) return DATA.architecture_canvas;
		var sourceCanvas = DATA.architecture_canvas;
		var architectureSource = String(sourceCanvas.architecture_source || '');
		var presentationProjector = window.RepomapArchitectureCanvas &&
			typeof window.RepomapArchitectureCanvas.projectUserPresentation === 'function'
			? window.RepomapArchitectureCanvas.projectUserPresentation : null;
		if (presentationProjector) sourceCanvas = presentationProjector(sourceCanvas, msg);
    var canvas = JSON.parse(JSON.stringify(sourceCanvas));
		if (String(canvas.validation_outcome || '') !== 'accepted_partial') {
			delete canvas.local_remainder_component_id;
		}
    delete canvas.suggested_investigations;
    delete canvas.frontiers;
    delete canvas.diagnostics;
    delete canvas.normalizations;
    delete canvas.hash;
    delete canvas.architecture_source;
    (canvas.behavior_anchors || []).forEach(function (anchor) {
      delete anchor.limitations;
      delete anchor.diagnostics;
      delete anchor.hash;
    });
    (canvas.components || []).forEach(function (component) {
      delete component.diagnostics;
      delete component.hash;
      delete component.source_component_ids;
      // Decision 230 D7: deterministic local fallback component names
      // and descriptions are localized for the RU product (audit §10).
      // Only the closed deterministic set is translated; model-authored
      // names and exact technical identifiers never change.
      if (REPORT_LANGUAGE === 'ru' && architectureSource === 'local_anchors') {
        var originalName = component.name;
        component.name = localizeDeterministicComponentName(component.name);
        component.description = localizeDeterministicComponentDescription(originalName, component.description);
      }
    });
    (canvas.subsystems || []).forEach(function (subsystem) {
      delete subsystem.diagnostics;
      delete subsystem.hash;
      delete subsystem.source_subsystem_ids;
      // Decision 230 D7: deterministic local fallback subsystem names
      // and descriptions are localized for the RU product (audit §10).
      if (REPORT_LANGUAGE === 'ru' && architectureSource === 'local_anchors') {
        var originalSubsystemName = subsystem.name;
        subsystem.name = localizeDeterministicSubsystemName(subsystem.name);
        subsystem.description = localizeDeterministicSubsystemDescription(originalSubsystemName, subsystem.description);
      }
    });
    (canvas.flows || []).forEach(function (flow) {
      delete flow.frontier;
      delete flow.flow_status;
      delete flow.status;
      delete flow.diagnostics;
      delete flow.hash;
    });
    (canvas.surfaces || []).forEach(function (surface) {
      delete surface.unavailable_reason;
      delete surface.readiness;
      delete surface.status;
      delete surface.diagnostics;
      delete surface.hash;
    });
    return canvas;
  }

	function architecturePackagePathForMember(member, packageByPath) {
		if (!member || !member.id || String(member.id.kind || '') !== 'package') return '';
		var facts = Array.isArray(member.facts) ? member.facts : [];
		for (var index = 0; index < facts.length; index++) {
			var fact = facts[index];
			if (!fact || String(fact.kind || '') !== 'declaration') continue;
			var packagePath = String(fact.value || '');
			if (packagePath && packageByPath[packagePath]) return packagePath;
		}
		return '';
	}

	function architectureComponentContexts() {
		var canvas = DATA.architecture_canvas || {};
		var knownComponentIDs = {};
		var memberNames = {};
		var navigationByComponentID = {};
		if (ARCHITECTURE_COMPONENT_NAVIGATION &&
			Number(ARCHITECTURE_COMPONENT_NAVIGATION.version) === 1) {
			(ARCHITECTURE_COMPONENT_NAVIGATION.components || []).forEach(function (entry) {
				var componentID = String(entry && entry.component_id || '');
				if (componentID && !navigationByComponentID[componentID]) {
					navigationByComponentID[componentID] = entry;
				}
			});
		}
		function memberIdentityKey(memberID) {
			if (!memberID || typeof memberID !== 'object') return '';
			var kind = String(memberID.kind || '');
			var value = String(memberID.value || '');
			return kind && value ? kind + '\u0000' + value : '';
		}
		(canvas.components || []).forEach(function (component) {
			if (!component || !component.id) return;
			knownComponentIDs[String(component.id)] = true;
			(component.members || []).forEach(function (member) {
				var key = memberIdentityKey(member && member.id);
				if (key && member && member.name) memberNames[key] = String(member.name);
			});
		});
		(canvas.structural_locators || []).forEach(function (entry) {
			var locator = entry && entry.locator;
			var key = memberIdentityKey(locator && locator.id);
			if (key && locator && locator.name) memberNames[key] = String(locator.name);
		});
		var graph = DATA.repository_graph || {};
		var graphPackages = Array.isArray(graph.packages) ? graph.packages : [];
		var packageByPath = {};
			graphPackages.forEach(function (pkg) {
				var packagePath = String(pkg && pkg.canonical_package_path || '');
				if (packagePath) packageByPath[packagePath] = pkg;
			});
			var behaviorAnchorByID = {};
			(canvas.behavior_anchors || []).forEach(function (anchor) {
				var anchorID = String(anchor && anchor.id || '');
				if (anchorID) behaviorAnchorByID[anchorID] = anchor;
			});
			var architectureSurfaceByID = {};
			(canvas.surfaces || []).forEach(function (surface) {
				var surfaceID = String(surface && surface.id || '');
				if (surfaceID) architectureSurfaceByID[surfaceID] = surface;
			});
			var discoveredTriggerByID = {};
			(((DATA.discovered_surfaces || {}).triggers) || []).forEach(function (trigger) {
				var triggerID = String(trigger && trigger.id || '');
				if (triggerID) discoveredTriggerByID[triggerID] = trigger;
			});

			var contexts = {};
			(canvas.components || []).forEach(function (component) {
			if (!component || !component.id) return;
			var packagePaths = [];
			var packageSeen = {};
			(component.members || []).forEach(function (member) {
				var packagePath = architecturePackagePathForMember(member, packageByPath);
				if (!packagePath || packageSeen[packagePath]) return;
				packageSeen[packagePath] = true;
				packagePaths.push(packagePath);
			});
			packagePaths.sort();

			var componentFiles = {};
			packagePaths.forEach(function (packagePath) {
				(packageByPath[packagePath].files || []).forEach(function (filePath) {
					filePath = String(filePath || '');
					if (filePath && OPENABLE_PATH_SET[filePath]) componentFiles[filePath] = true;
				});
			});
				(component.members || []).forEach(function (member) {
					(member && member.facts || []).forEach(function (fact) {
						var filePath = String(fact && fact.location && fact.location.path || '');
						if (filePath && OPENABLE_PATH_SET[filePath]) componentFiles[filePath] = true;
					});
				});
				var hasExactComponentFiles = Object.keys(componentFiles).length > 0;
				(component.anchor_ids || []).forEach(function (anchorID) {
					var anchor = behaviorAnchorByID[String(anchorID || '')];
					var filePath = String(anchor && anchor.location && anchor.location.path || '');
					if (!filePath || !OPENABLE_PATH_SET[filePath]) return;
					if (hasExactComponentFiles && !componentFiles[filePath]) return;
					componentFiles[filePath] = true;
				});

			// Persisted Study mechanisms already carry the exact validated
			// component IDs for their nodes. Legacy direction areas are not an
			// authority for the current component inspector.
			var studies = componentStudyThemes(component.id);

				var sources = [];
				var navigation = navigationByComponentID[String(component.id)] || null;
				(navigation && Array.isArray(navigation.symbol_sources)
					? navigation.symbol_sources : []).forEach(function (source) {
					var location = source && source.location;
					var symbol = String(source && source.symbol || '');
					if (!symbol || !location || !location.path || Number(location.line) <= 0) return;
					sources.push({
						member_id: source.member_id,
						symbol: symbol,
						label: symbol,
						detail: symbol,
						location: {
							path: String(location.path),
							line: Number(location.line),
							column: Number(location.column) || 0,
						},
						actionable: sourceLocationActionAvailable(location),
					});
				});
				var surfaceStarts = [];
				var surfaceStartKeys = {};
				function addSurfaceStart(start, semanticKind) {
					var location = start && start.location;
					var processEntry = semanticKind === 'process_entry';
					var key = processEntry
						? 'process_entry\u0000' + String(location && location.path || '') + '\u0000' +
							String(Number(location && location.line) || 0)
						: 'surface\u0000' + String(start && start.id || '');
					if (!location || !location.path || Number(location.line) <= 0 || surfaceStartKeys[key]) return;
					surfaceStartKeys[key] = true;
					surfaceStarts.push(start);
				}
				var componentSurfaceIDs = Array.from(new Set(
					(component.owned_surface_ids || [])
						.concat(component.participating_surface_ids || [])
						.map(String)
						.filter(Boolean)
				));
				componentSurfaceIDs.forEach(function (surfaceID) {
					surfaceID = String(surfaceID || '');
					var surface = architectureSurfaceByID[surfaceID];
					var trigger = discoveredTriggerByID[surfaceID];
					if (!trigger) return;
					var handlerLocation = trigger.handler_location;
					var registrationLocation = trigger.registration_site || trigger.descriptor_site ||
						trigger.server_start_site;
					var entryLocation = trigger.process_entrypoint && trigger.process_entrypoint.location;
					var processEntry = String(trigger.kind || '') === 'process_entry';
					var location = processEntry && entryLocation
						? entryLocation : (handlerLocation || registrationLocation || entryLocation);
					var filePath = String(location && location.path || '');
					var line = Number(location && location.line) || 0;
					if (!filePath || !OPENABLE_PATH_SET[filePath] || line <= 0) return;
					var surfaceName = String(surface && surface.name || trigger.identity && trigger.identity.name || '');
					var handlerName = String(trigger.handler && trigger.handler.known && trigger.handler.text || '');
					var label = surfaceName || handlerName ||
						String(trigger.process_entrypoint && trigger.process_entrypoint.name || filePath);
					if (!processEntry && handlerName && handlerName !== surfaceName) {
						label = surfaceName ? surfaceName + ' → ' + handlerName : handlerName;
					}
					if (processEntry && entryLocation) {
						label += ' · ' + msg('main.surface.process_entry_suffix');
					} else if (!handlerLocation && registrationLocation) {
						label += ' · ' + msg('main.surface.registration_suffix');
					} else if (!handlerLocation && !registrationLocation && entryLocation) {
						label += ' · ' + msg('main.surface.process_entry_suffix');
					}
					var projectedLocation = {
						path: filePath,
						line: line,
						column: Number(location.column) || 0,
					};
					addSurfaceStart({
						id: surfaceID,
						label: label,
						location: projectedLocation,
						actionable: sourceLocationActionAvailable(projectedLocation),
					}, processEntry ? 'process_entry' : String(trigger.kind || 'surface'));
				});
				// Exact process-entry anchors remain valid entry context even when
				// the surface catalog has no matching trigger. They are backend-
				// attached to the component; this is a bounded local join, not an
				// inferred entrypoint.
				(component.anchor_ids || []).forEach(function (anchorID) {
					var anchor = behaviorAnchorByID[String(anchorID || '')];
					if (!anchor || String(anchor.kind || '') !== 'process_entry') return;
					var location = anchor.location;
					var filePath = String(location && location.path || '');
					var line = Number(location && location.line) || 0;
					if (!filePath || !OPENABLE_PATH_SET[filePath] || line <= 0) return;
					var projectedLocation = {
						path: filePath,
						line: line,
						column: Number(location.column) || 0,
					};
					addSurfaceStart({
						id: String(anchor.id || anchorID || ''),
						label: String(anchor.label || msg('main.surface.process_entry_suffix')),
						location: projectedLocation,
						actionable: sourceLocationActionAvailable(projectedLocation),
					}, 'process_entry');
				});
				surfaceStarts.sort(function (left, right) {
					return String(left && left.label || '').localeCompare(String(right && right.label || '')) ||
						String(left && left.location && left.location.path || '').localeCompare(
							String(right && right.location && right.location.path || '')
						) ||
						Number(left && left.location && left.location.line) -
							Number(right && right.location && right.location.line);
				});
			var packageTargets = packagePaths.map(function (packagePath) {
				var packageFiles = (packageByPath[packagePath].files || []).map(String).filter(function (filePath) {
					return !!(filePath && OPENABLE_PATH_SET[filePath]);
				}).sort();
				// P7-B: a package navigates to its first openable file as a
				// deterministic representative target (sorted path order).
				// The package itself has no single line; the file is the
				// exact boundary a reader can open. No file openable → the
				// target stays non-actionable (honest unavailable state).
				var location = null;
				var actionable = false;
				if (packageFiles.length > 0) {
					location = { path: packageFiles[0], line: 1, column: 0 };
					actionable = sourceLocationActionAvailable(location);
				}
				return {
					path: packagePath,
					file_count: packageFiles.length,
					location: location,
					actionable: actionable,
				};
			});

			contexts[String(component.id)] = {
				map_target: navigation && navigation.map_target || {
					kind: 'component', component_id: String(component.id),
				},
				package_paths: packagePaths,
				package_targets: packageTargets,
				file_count: Object.keys(componentFiles).length,
				sources: sources,
				surface_starts: surfaceStarts,
				member_count: Array.isArray(component.members) ? component.members.length : 0,
				// Decision 222: authority and evidence composition travel with
				// the component so the inspector can state them truthfully.
				authority: architectureGroupingAuthority(),
				evidence_composition: componentEvidenceComposition(component),
				studies: studies,
			};
		});
		(Array.isArray(canvas.structural_edges) ? canvas.structural_edges : []).forEach(function (edge) {
			if (!edge || !edge.id || !edge.witness) return;
			var participantComponentIDs = [];
			var seenComponentIDs = {};
			(Array.isArray(edge.from_component_ids) ? edge.from_component_ids : []).concat(
				Array.isArray(edge.to_component_ids) ? edge.to_component_ids : []
			).forEach(function (componentID) {
				componentID = String(componentID || '');
				if (!componentID || seenComponentIDs[componentID]) return;
				seenComponentIDs[componentID] = true;
				participantComponentIDs.push(componentID);
			});
			participantComponentIDs.forEach(function (componentID) {
				var context = contexts[componentID];
				if (!context && !knownComponentIDs[componentID]) return;
				if (!context) {
					context = contexts[componentID] = {
						package_paths: [], package_targets: [], file_count: 0,
						sources: [], surface_starts: [], studies: [],
					};
				}
				if (!Array.isArray(context.structural_relations)) context.structural_relations = [];
				context.structural_relations.push({
					id: String(edge.id),
					from: edge.witness.from,
					to: edge.witness.to,
					from_label: memberNames[memberIdentityKey(edge.witness.from)] || '',
					to_label: memberNames[memberIdentityKey(edge.witness.to)] || '',
					location: edge.witness.location || null,
				});
			});
		});
		Object.keys(contexts).forEach(function (componentID) {
			var relations = contexts[componentID].structural_relations || [];
			relations.sort(function (left, right) { return left.id.localeCompare(right.id); });
		});
		return contexts;
	}

  // The selected executable target opens on its exact Entrypoints context.
  // Libraries keep the neutral landscape because no process entry is
  // invented for their unresolved public-API boundary.
  var activeMapLens = defaultTargetMapLens();
  var selectedEntrypointHandoffGroupID = defaultEntrypointHandoffGroupID();

	function defaultTargetMapLens() {
		return ANALYSIS_TARGET && String(ANALYSIS_TARGET.kind || '') === 'executable_package'
			? 'entrypoints' : 'landscape';
	}

	function restoreDefaultTargetCanvasContext() {
		activeMapLens = defaultTargetMapLens();
		selectedEntrypointHandoffGroupID = defaultEntrypointHandoffGroupID();
		if (architectureCanvasView && typeof architectureCanvasView.setLens === 'function') {
			architectureCanvasView.setLens(activeMapLens);
		}
		if (selectedEntrypointHandoffGroupID && architectureCanvasView &&
			typeof architectureCanvasView.selectEntrypointHandoffGroup === 'function') {
			architectureCanvasView.selectEntrypointHandoffGroup(selectedEntrypointHandoffGroupID);
		}
		if (architectureCanvasView && typeof architectureCanvasView.clearStudyMechanismOverlay === 'function') {
			architectureCanvasView.clearStudyMechanismOverlay();
		}
		var context = document.querySelector && document.querySelector('.rm-map-entry-context');
		if (context) context.hidden = activeMapLens !== 'entrypoints';
		renderMapLensObjects();
	}

	function defaultEntrypointHandoffGroupID() {
		if (!ANALYSIS_TARGET || String(ANALYSIS_TARGET.kind || '') !== 'executable_package') return '';
		var roots = Array.isArray(ANALYSIS_TARGET.roots) ? ANALYSIS_TARGET.roots : [];
		var groups = DATA.architecture_canvas && Array.isArray(DATA.architecture_canvas.entry_handoff_groups)
			? DATA.architecture_canvas.entry_handoff_groups : [];
		for (var rootIndex = 0; rootIndex < roots.length; rootIndex++) {
			var root = roots[rootIndex];
			var matches = groups.filter(function (group) {
				var entry = group && group.entry;
				return String(root && root.path || '') === String(entry && entry.path || '') &&
					Number(root && root.line) === Number(entry && entry.line);
			});
			if (matches.length === 1) return String(matches[0].id || '');
		}
		return '';
	}

  // Decision 236 (v11): the lens objects panel makes the FIRST-CLASS
  // backend objects of the active lens visibly present next to the map:
  // entry categories (how work enters), exact first-hop context for process
  // entries, and touchpoint families (what external state is observed). The
  // objects come from the SAME DOM-free projection the canvas
  // emphasis uses (projectArchitectureLens) — never guessed from dimmed
  // boxes and never from renderer state.
  function renderMapLensObjects() {
    var host = document.getElementById('rm-map-lens-objects');
    if (!host) return;
    host.replaceChildren();
    host.setAttribute('data-lens', activeMapLens);
    var api = window.RepomapArchitectureCanvas && window.RepomapArchitectureCanvas.projectArchitectureLens;
    if (typeof api !== 'function') return;
    var projection = api(DATA, activeMapLens);
    if (!projection) return;
    var entryHandoffGroups = projection.objects && projection.objects.entry_handoff_groups ||
      projection.entry_handoff_groups || [];
    if (activeMapLens === 'landscape') return;
    if (activeMapLens === 'entrypoints') {
			var entrypointGroups = projection.objects && projection.objects.entrypoints ||
				projection.entrypoints || [];
      if (selectedEntrypointHandoffGroupID && !entryHandoffGroups.some(function (projected) {
        return projected && projected.id === selectedEntrypointHandoffGroupID;
      })) selectedEntrypointHandoffGroupID = '';
      entryHandoffGroups.forEach(function (projected) {
        if (!projected || projected.kind !== 'entry_handoff_group' || !projected.group) return;
        host.appendChild(renderEntrypointHandoffSelector(projected.group, host, entryHandoffGroups));
      });
			entrypointGroups.forEach(function (group) {
				(group.entries || []).forEach(function (entry) {
					// A zero-hop exact Surface has no handoff group to select. It
					// remains a first-class Entrypoint even when conceptual grouping
					// could not assign an owner. Do not dim or emphasize a component;
					// expose only its backend-owned exact source locations.
					if (entryHandoffGroups.length &&
						(Array.isArray(entry.component_ids) && entry.component_ids.length > 0 ||
						 entrypointSurfaceHasExactHandoffGroup(entry, entryHandoffGroups))) return;
					host.appendChild(renderZeroHopEntrypointSurface(entry, group.kind));
				});
			});
      var overflowHost = el('div', 'rm-map-entry-overflow-host');
      host.appendChild(overflowHost);
      if (selectedEntrypointHandoffGroupID) {
        activateEntrypointHandoffGroup(
          selectedEntrypointHandoffGroupID,
          host,
          entryHandoffGroups
        );
      }
      return;
    }
  }

	function activeStudyMechanismMapSelection() {
		if (workspaceState.view !== 'map' && workspaceState.view !== 'architecture') return null;
		if (activeMapLens !== 'landscape') return null;
		return studyMechanismForTarget(workspaceState.mapTarget);
	}

	function architectureComponentName(componentID) {
		var components = DATA.architecture_canvas && Array.isArray(DATA.architecture_canvas.components)
			? DATA.architecture_canvas.components : [];
		for (var index = 0; index < components.length; index++) {
			if (components[index] && String(components[index].id || '') === String(componentID || '')) {
				return String(components[index].name || components[index].id || '');
			}
		}
		return '';
	}

	function studyMechanismSideRowLabel(row) {
		if (!row) return '';
		if (row.reason === 'same_component') {
			var componentID = row.from_component_ids && row.from_component_ids[0];
			return msg('main.study.investigation.map.same_component', {
				component: architectureComponentName(componentID) || msg('main.study.investigation.map.one_area'),
			});
		}
		if (row.reason === 'plural_components') {
			return msg('main.study.investigation.map.plural_components');
		}
		return msg('main.study.investigation.map.zero_component');
	}

	function renderStudyMechanismMapContext() {
		var selection = activeStudyMechanismMapSelection();
		if (!selection) return null;
		var project = window.RepomapArchitectureCanvas &&
			window.RepomapArchitectureCanvas.projectStudyMechanismOverlay;
		var componentIDs = (DATA.architecture_canvas && DATA.architecture_canvas.components || []).map(function (component) {
			return String(component && component.id || '');
		}).filter(Boolean);
		var projection = typeof project === 'function'
			? project(selection.mechanism, componentIDs)
			: { side_rows: [] };
		var sideRows = (Array.isArray(projection.side_rows) ? projection.side_rows : []).filter(function (row) {
			return row && ['same_component', 'zero_component', 'plural_components'].indexOf(row.reason) >= 0;
		});
		// Desktop carries only exact transitions that cannot be represented by
		// one honest Canvas arrow. The Canvas itself is hidden on mobile, so a
		// responsive sibling restores the complete ordered path there. CSS
		// keeps exactly one representation exposed at every breakpoint.
		var aside = el('aside', 'rm-study-investigation-map' +
			(sideRows.length ? '' : ' rm-study-investigation-map--mobile-only'));
		aside.setAttribute('aria-label', msg('main.study.investigation.title'));
		var desktopContext = el('div', 'rm-study-investigation-map__desktop');
		desktopContext.appendChild(txt('h3', 'rm-study-investigation-map__side-title', msg('main.study.investigation.map.side_title')));
		sideRows.forEach(function (row) {
			var item = el('div', 'rm-study-investigation-map__side-row rm-study-investigation-map__side-row--' + String(row.reason));
			item.appendChild(txt('strong', '', [
				String(row.from_node && row.from_node.label || ''),
				String(row.to_node && row.to_node.label || ''),
			].filter(Boolean).join(' → ')));
			item.appendChild(txt('span', '', studyMechanismSideRowLabel(row)));
			var source = renderStudyInvestigationSourceAction(
				row.edge && row.edge.callsite,
				msg('main.study.investigation.callsite'),
				'rm-study-investigation__source'
			);
			if (source) item.appendChild(source);
			desktopContext.appendChild(item);
		});
		aside.appendChild(desktopContext);
		var mobileContext = el('div', 'rm-study-investigation-map__mobile-path');
		mobileContext.appendChild(txt('h3', 'rm-study-investigation-map__side-title', msg('main.study.investigation.title')));
		mobileContext.appendChild(renderStudyMechanismPath(
			selection.card,
			selection.investigation,
			selection.mechanism,
			Math.max(0, Number(selection.mechanism.ordinal) - 1),
			{ showMap: false }
		));
		aside.appendChild(mobileContext);
		return aside;
	}

	function renderStudyMechanismMapContextIntoHost() {
		var host = document.getElementById('rm-study-investigation-map-context');
		if (!host) return;
		host.replaceChildren();
		var context = renderStudyMechanismMapContext();
		if (context) host.appendChild(context);
		host.hidden = !context;
	}

  function renderArchitectureReturn() {
    var root = document.getElementById('rm-architecture');
    if (!root) return;
    var existing = root.querySelector('.rm-architecture-return');
    if (existing) existing.remove();
    if (!workspaceState.mapReturn) return;
		if (workspaceState.mapReturn.themeCardOrdinal) {
			var theme = themeCardByOrdinal(workspaceState.mapReturn.themeCardOrdinal);
			if (!theme) return;
			var themeBanner = el('div', 'rm-architecture-return');
			themeBanner.appendChild(txt('span', '', msg('main.map.context', { title: theme.final_title || theme.final_question || msg('main.study') })));
			var themeBack = txt('button', 'rm-secondary-action', msg('main.study.investigation.back_to_study'));
			themeBack.type = 'button';
			themeBack.onclick = returnFromArchitecture;
			themeBanner.appendChild(themeBack);
			root.prepend(themeBanner);
			return;
		}
		if (workspaceState.mapReturn.directionID) {
			var direction = studyDirectionByID(workspaceState.mapReturn.directionID);
			if (!direction) return;
			var directionBanner = el('div', 'rm-architecture-return');
			directionBanner.appendChild(txt('span', '', msg('main.map.context', { title: direction.question })));
			var directionBack = txt('button', 'rm-secondary-action', msg('main.back.to.reading.path'));
			directionBack.type = 'button';
			directionBack.onclick = returnFromArchitecture;
			directionBanner.appendChild(directionBack);
			root.prepend(directionBanner);
			return;
		}
  }

	function bindArchitectureListDisclosureMobileDefault(disclosure, summary) {
		if (!disclosure || !summary || typeof window.matchMedia !== 'function') return;
		var media = window.matchMedia('(max-width: 560px)');
		if (!media) return;
		var userChoseState = false;
		var autoOpened = false;
		function applyMobileDefault(matches) {
			if (matches) {
				if (!userChoseState && !disclosure.open) {
					disclosure.open = true;
					autoOpened = true;
				}
				return;
			}
			if (!userChoseState && autoOpened) {
				disclosure.open = false;
				autoOpened = false;
			}
		}
		summary.addEventListener('click', function () {
			userChoseState = true;
			autoOpened = false;
		});
		applyMobileDefault(!!media.matches);
		var onChange = function (event) { applyMobileDefault(!!(event && event.matches)); };
		if (typeof media.addEventListener === 'function') media.addEventListener('change', onChange);
		else if (typeof media.addListener === 'function') media.addListener(onChange);
	}

  function renderArchitectureWorkspace() {
    var root = document.getElementById('rm-architecture');
    if (!root) return;
    root.replaceChildren();
    var componentList = renderArchitectureComponentList();
    var canvasCard = null;
    if (DATA.architecture_canvas && window.RepomapArchitectureCanvas) {
      canvasCard = el('section', 'rm-card rm-architecture-canvas-card');
      var mapToolbar = el('div', 'rm-map-toolbar');
      mapEmptyInspectorHost = renderMapEmptyInspector();
      mapToolbar.appendChild(mapEmptyInspectorHost);
      canvasCard.appendChild(mapToolbar);
		if (activeMapLens === 'entrypoints') {
			var entryContext = el('section', 'rm-map-entry-context');
			entryContext.appendChild(txt('h2', 'rm-map-entry-context__title', msg('main.map.lens.entrypoints')));
			var entryObjects = el('div', 'rm-map-lens-objects');
			entryObjects.id = 'rm-map-lens-objects';
			entryContext.appendChild(entryObjects);
			canvasCard.appendChild(entryContext);
		}
		var canvasLayout = el('div', 'rm-map-primary-layout');
		mapPrimaryLayoutHost = canvasLayout;
		var canvasStage = el('div', 'rm-architecture-canvas-stage');
      architectureCanvasHost = el('div', 'rm-architecture-canvas-host');
		canvasStage.appendChild(architectureCanvasHost);
		var studyMechanismContext = el('div', 'rm-study-investigation-map-context');
		studyMechanismContext.id = 'rm-study-investigation-map-context';
		canvasStage.appendChild(studyMechanismContext);
		canvasLayout.appendChild(canvasStage);
		canvasCard.appendChild(canvasLayout);
      root.appendChild(canvasCard);
		renderStudyMechanismMapContextIntoHost();
    } else {
      var systemMap = renderUserSystemMap(DATA.high_level_map || []);
      if (systemMap) root.appendChild(systemMap);
    }
    if (componentList) {
      // With no supported relation evidence the structured list is the
      // primary representation (it already follows the map in layout).
      var listDisclosure = el('details', 'rm-architecture-disclosure rm-architecture-list-disclosure');
      var listSummary = el('summary', 'rm-architecture-disclosure__summary');
      listSummary.appendChild(txt('span', 'rm-architecture-disclosure__title', msg('main.architecture.component_list.title')));
      var componentCount = currentArchitectureComponents().length;
      listSummary.appendChild(txt('span', 'rm-architecture-disclosure__count', msg('main.architecture.disclosure.count', { count: componentCount })));
      listDisclosure.appendChild(listSummary);
      listDisclosure.appendChild(componentList);
			// The viewport may settle after the initial report render. Keep the
			// mobile default synchronized until the reader explicitly chooses an
			// open/closed state; never carry an automatic open state to desktop.
			bindArchitectureListDisclosureMobileDefault(listDisclosure, listSummary);
      root.appendChild(listDisclosure);
    }
    // Decision 217: compact unmapped-evidence disclosure preserving every
    // exact item behind an expand action.
    var unmapped = renderArchitectureUnmappedDisclosure();
    if (unmapped) root.appendChild(unmapped);
		appendInlineStudyWorkspace(root);
  }

  // Resolve an already-authorized exact report location without inferring a
  // path, symbol, or source range. Entrypoints and Study mechanisms share
  // this presentation-only source action; their graph joins remain separate.
  function exactReportSourceActionResolutionForLocation(location) {
   var sourceResolution = exactOverviewSourceResolutionForLocation(location);
   if (sourceResolution.conflict || sourceResolution.source) return sourceResolution;
   if (!location || exactOverviewSourcePath(location.path) !== location.path ||
    !Number.isInteger(location.line) || location.line <= 0 ||
    !sourceLocationActionAvailable(location)) {
    return { source: null, conflict: false };
   }
   return { source: {
    snippet: null,
    location: {
     path: location.path,
     line: location.line,
     column: Number.isInteger(location.column) && location.column > 0 ? location.column : 0,
    },
   }, conflict: false };
  }

  // Canvas 15 publishes exact D210 first-hop groups as context for a process
  // Entrypoint. The list selects ONE group for the Canvas overlay; backend
  // transition.component_ids are the only join. Diagnostic archaeology stays
  // in saved evidence, while exact source actions remain directly available.
  function renderEntrypointLocationAction(location, label) {
   var exact = {
    path: String(location && location.path || ''),
    line: Number(location && location.line) || 0,
    column: Number(location && location.column) || 0,
   };
   var visible = String(label || '') + ' · ' + formatCodeLocation(exact);
   var resolution = exactReportSourceActionResolutionForLocation(exact);
   var snippet = resolution && resolution.source && resolution.source.snippet;
   if (snippet) {
    var action = el('button', 'rm-map-entry-context__source rm-source-action-link');
    action.type = 'button';
    action.textContent = visible;
    action.onclick = function () {
     openSourceSnippet(snippet, resolution.source.location, false, { drawerFirst: true });
    };
    return action;
   }
   if (resolution && resolution.source && resolution.source.location) {
    var staticAction = sourceActionElement(
     visible,
     'rm-map-entry-context__source rm-source-action-link',
     resolution.source.location,
     0,
     function () { openSourceLocation(resolution.source.location); }
    );
    if (staticAction) return staticAction;
   }
   return txt('code', 'rm-map-entry-context__source rm-map-entry-context__source--text', visible);
  }

	function entrypointSurfaceHasExactHandoffGroup(entry, projectedGroups) {
		var locations = Array.isArray(entry && entry.locations) ? entry.locations : [];
		return locations.some(function (location) {
			return projectedGroups.some(function (projected) {
				var groupEntry = projected && projected.group && projected.group.entry;
				if (!groupEntry || String(location.path || '') !== String(groupEntry.path || '') ||
					Number(location.line) !== Number(groupEntry.line)) return false;
				var leftColumn = Number(location.column) || 0;
				var rightColumn = Number(groupEntry.column) || 0;
				return leftColumn === 0 || rightColumn === 0 || leftColumn === rightColumn;
			});
		});
	}

	function renderZeroHopEntrypointSurface(entry, kind) {
		var row = el('div', 'rm-map-lens-object rm-map-entry-zero-hop');
		var locations = Array.isArray(entry && entry.locations) ? entry.locations : [];
		var label = String(entry && entry.label || msg('main.map.lens.entry.process'));
		if (locations.length) {
			locations.forEach(function (location) {
				row.appendChild(renderEntrypointLocationAction(location, label));
			});
		} else {
			row.appendChild(txt('strong', '', label));
		}
		row.appendChild(txt('span', 'rm-map-lens-object__detail', String(kind || '')));
		return row;
	}

  function entryHandoffProjectionByID(projectedGroups, groupID) {
   for (var index = 0; index < projectedGroups.length; index++) {
    var projected = projectedGroups[index];
    if (projected && String(projected.id || '') === String(groupID || '')) return projected;
   }
   return null;
  }

  function entryHandoffSelectorLabel(group) {
   var entry = group && group.entry || {};
   var ownerIDs = Array.from(new Set((Array.isArray(entry.component_ids) ? entry.component_ids : [])
    .map(function (value) { return String(value || ''); }).filter(Boolean)));
   var ownerName = '';
   if (ownerIDs.length === 1) {
    (DATA.architecture_canvas && DATA.architecture_canvas.components || []).some(function (component) {
     if (!component || String(component.id || '') !== ownerIDs[0]) return false;
     ownerName = String(component.name || '').trim();
     return true;
    });
   }
   var exactPath = String(entry.path || '');
   var pathParts = exactPath.split('/').filter(Boolean);
   var pathLeaf = pathParts.length ? pathParts[pathParts.length - 1] : '';
   var symbol = bareSourceSymbol(entry.symbol);
   return [ownerName || pathLeaf, symbol].filter(Boolean).join(' · ') ||
    msg('main.map.lens.entry.process');
  }

  function activateEntrypointHandoffGroup(groupID, host, projectedGroups) {
   var projected = entryHandoffProjectionByID(projectedGroups, groupID);
   if (!projected || !projected.group) return false;
   selectedEntrypointHandoffGroupID = String(projected.id || '');
   if (architectureCanvasView && typeof architectureCanvasView.selectEntrypointHandoffGroup === 'function') {
    architectureCanvasView.selectEntrypointHandoffGroup(selectedEntrypointHandoffGroupID);
   }
   host.querySelectorAll('[data-entry-handoff-group-id]').forEach(function (node) {
    var selected = node.getAttribute('data-entry-handoff-group-id') === selectedEntrypointHandoffGroupID;
    node.classList.toggle('is-selected', selected);
    node.setAttribute('aria-pressed', selected ? 'true' : 'false');
   });
   var overflowHost = host.querySelector('.rm-map-entry-overflow-host');
   if (overflowHost) {
    overflowHost.replaceChildren(renderEntrypointHandoffOverflow(projected.group));
   }
   return true;
  }

  function renderEntrypointHandoffSelector(group, host, projectedGroups) {
   var entry = group && group.entry || {};
   var handoffCount = Array.isArray(group && group.entry_handoffs) ? group.entry_handoffs.length : 0;
   var row = el('div', 'rm-map-entry-selector-row');
   var selector = el('button', 'rm-map-entry-selector');
   selector.type = 'button';
   selector.setAttribute('data-entry-handoff-group-id', String(group && group.id || ''));
   selector.setAttribute('aria-pressed', String(group && group.id || '') === selectedEntrypointHandoffGroupID ? 'true' : 'false');
   selector.appendChild(txt('strong', 'rm-map-entry-selector__entry',
    entryHandoffSelectorLabel(group)));
   selector.appendChild(txt('span', 'rm-map-lens-object__detail', '— ' + handoffCount));
   var activate = function () {
    activateEntrypointHandoffGroup(String(group && group.id || ''), host, projectedGroups);
   };
   selector.onclick = activate;
   row.appendChild(selector);
   row.appendChild(renderEntrypointLocationAction(entry, msg('main.map.entry.context.entry_source')));
   return row;
  }

  function renderEntrypointHandoffOverflow(group) {
   var strip = el('div', 'rm-map-entry-overflow');
   var project = window.RepomapArchitectureCanvas &&
    window.RepomapArchitectureCanvas.projectEntrypointHandoffOverlay;
   var componentIDs = (DATA.architecture_canvas && DATA.architecture_canvas.components || []).map(function (component) {
    return String(component && component.id || '');
   }).filter(Boolean);
   var projection = typeof project === 'function'
    ? project(group, componentIDs)
    : { overflow: [], frontier: null };
   var overflow = Array.isArray(projection.overflow) ? projection.overflow : [];
   if (overflow.length) {
    strip.appendChild(txt('span', 'rm-map-entry-overflow__heading',
     msg('main.map.entry.context.not_drawn_calls', { count: overflow.length })));
    overflow.forEach(function (item) {
     var handoff = item && item.handoff || {};
     var target = handoff.target || {};
     var row = el('div', 'rm-map-entry-overflow__call');
     row.appendChild(txt('strong', 'rm-map-entry-context__target',
      bareSourceSymbol(target.symbol || target.label || handoff.symbol) || msg('main.map.entry.context.direct_call')));
     var actions = el('div', 'rm-map-entry-context__actions');
     actions.appendChild(renderEntrypointLocationAction(handoff, msg('main.map.entry.context.callsite')));
     actions.appendChild(renderEntrypointLocationAction(target, msg('main.map.entry.context.target_source')));
     row.appendChild(actions);
     strip.appendChild(row);
    });
   }
   if (projection.frontier) {
    strip.appendChild(txt('p', 'rm-map-entry-context__limit', msg('main.map.entry.context.limit')));
   }
   return strip;
  }

  function renderArchitectureUnmappedDisclosure() {
    var canvas = DATA.architecture_canvas || {};
    var remainderID = String(canvas.local_remainder_component_id || '');
    var remainder = null;
    (Array.isArray(canvas.components) ? canvas.components : []).forEach(function (component) {
      if (component && component.id === remainderID) remainder = component;
    });
    var members = remainder && Array.isArray(remainder.members) ? remainder.members : [];
    if (!members.length) return null;
    var details = el('details', 'rm-architecture-unmapped');
    details.appendChild(txt('summary', 'rm-architecture-unmapped__summary', msg(
      'main.architecture.unmapped.title',
      { count: members.length }
    )));
    details.appendChild(txt('p', 'rm-architecture-unmapped__note', msg('main.architecture.unmapped.copy')));
    var list = el('ul', 'rm-architecture-unmapped__list');
    members.forEach(function (member) {
      var name = String(member && member.name || member && member.id && member.id.value || '');
      var item = txt('li', 'rm-architecture-unmapped__item', name);
      list.appendChild(item);
    });
    details.appendChild(list);
    return details;
  }

  // Decision 221/216 authority axis: the grouping authority of the whole
  // canvas, derived exclusively from the exact closed canvas source and
  // synthesis state — never from component membership.
  //   validated_model      -> validated model hypothesis
  //   partial_model        -> partial validated model hypothesis
  //   local_anchors / local_* / package_fallback -> local deterministic fallback
  function architectureGroupingAuthority() {
  	var canvas = DATA.architecture_canvas || {};
  	var source = String(canvas.architecture_source || canvas.source || '');
  	if (source === 'validated_model') return 'validated';
  	if (source === 'partial_model') return 'partial';
  	return 'local';
  }

  // architectureAuthorityMessageID maps the closed authority axis to copy.
  function architectureAuthorityMessageID(authority) {
  	if (authority === 'validated') return 'main.architecture.authority.validated';
  	if (authority === 'partial') return 'main.architecture.authority.partial';
  	return 'main.architecture.authority.local';
  }

  // architectureCoverageState compiles the exact coverage axis: complete,
  // partial (covered/requested), or no synthesis (local remainder only).
  function architectureCoverageState() {
  	var synthesis = DATA.architecture_synthesis || {};
  	var state = String(synthesis.state || '');
  	if (state === 'succeeded' || state === 'accepted') {
  		var covered = Number(synthesis.covered_conceptual_count) || 0;
  		var requested = Number(synthesis.requested_conceptual_count) || 0;
  		if (requested > 0 && covered >= requested) return { kind: 'complete' };
  		if (requested > 0) return { kind: 'partial', covered: covered, requested: requested };
  	}
  	return { kind: 'local' };
  }

  // componentEvidenceComposition classifies a component's member evidence
  // into exact-only, mixed exact + package, or package/structure only.
  // This is the member-evidence axis and is independent of grouping
  // authority: a local deterministic component with exact sources is still
  // a local grouping, and a validated component with package-only members
  // is still a validated hypothesis about structure.
  function componentEvidenceComposition(component) {
  	var navigation = ARCHITECTURE_COMPONENT_NAVIGATION && Number(ARCHITECTURE_COMPONENT_NAVIGATION.version) === 1
  		? (ARCHITECTURE_COMPONENT_NAVIGATION.components || []).filter(function (entry) {
  			return entry && entry.component_id === component.id;
  		})[0] : null;
  	var exactSources = (component.symbol_sources && component.symbol_sources.length) ||
  		(navigation && navigation.symbol_sources && navigation.symbol_sources.length) ||
  		(component.members || []).filter(function (member) {
  			return member && member.id && String(member.id.kind) === 'symbol';
  		}).length;
  	var packages = (navigation && navigation.package_participant_ids && navigation.package_participant_ids.length) ||
  		(component.package_ids && component.package_ids.length) ||
  		(component.members || []).filter(function (member) {
  			return member && member.id && String(member.id.kind) === 'package';
  		}).length;
  	if (exactSources && packages) return 'mixed';
  	if (exactSources) return 'exact';
  	return 'package';
  }

  function renderArchitectureAuthorityStrip() {
  	var authority = architectureGroupingAuthority();
  	var coverage = architectureCoverageState();
  	var items = [];
  	items.push(msg(architectureAuthorityMessageID(authority)));
  	if (coverage.kind === 'complete') {
  		items.push(msg('main.architecture.coverage.complete'));
  	} else if (coverage.kind === 'partial') {
  		items.push(msg('main.architecture.coverage.partial', { covered: coverage.covered, requested: coverage.requested }));
  	} else {
  		items.push(msg('main.architecture.coverage.local'));
  	}
  	var strip = el('div', 'rm-architecture-truth-strip');
  	items.forEach(function (item) {
  		strip.appendChild(txt('span', 'rm-architecture-truth-strip__item', item));
  	});
  	return strip;
  }

  function renderArchitectureComponentList() {
		var canvas = userArchitectureData() || DATA.architecture_canvas || {};
    var components = currentArchitectureComponents(canvas);
    if (!components.length) return null;
    var section = el('section', 'rm-workspace-section rm-architecture-list');
    section.appendChild(renderViewHeading(
      msg('main.architecture.list.kicker'),
      msg('main.architecture.list.title'),
      msg('main.architecture.list.copy')
    ));
    var list = el('ol', 'rm-architecture-list__items');
    components.forEach(function (component) {
      var item = txt('li', 'rm-architecture-list__item', '');
      var primary = el('button', 'rm-architecture-list__primary');
      primary.type = 'button';
      var heading = el('span', 'rm-architecture-list__heading');
      heading.appendChild(txt('strong', '', String(component.name || component.id)));
      heading.appendChild(txt('span', 'rm-architecture-list__arrow', '›'));
      primary.appendChild(heading);
      if (component.description) primary.appendChild(txt('span', 'rm-architecture-list__description', String(component.description)));
      var memberCount = Array.isArray(component.members) ? component.members.length : 0;
      if (memberCount > 0) {
        primary.appendChild(txt('span', 'rm-architecture-list__count', msg('main.architecture.list.member_count', { count: memberCount })));
      }
      primary.onclick = function () {
        if (architectureCanvasView) {
          architectureCanvasView.openComponent(String(component.id));
        } else {
          openArchitectureTarget({ kind: 'component', component_id: component.id }, null);
        }
      };
      item.appendChild(primary);
      list.appendChild(item);
    });
    section.appendChild(list);
    return section;
  }

  function renderUserSystemMap(subsystems) {
    if (!Array.isArray(subsystems) || !subsystems.length) return null;
    var grid = el('div', 'rm-mechanism-grid');
    subsystems.forEach(function (subsystem) {
      if (!subsystem || !subsystem.name) return;
      var card = el('article', 'rm-card');
      card.appendChild(txt('h3', '', subsystem.name));
      if (subsystem.why_it_matters) card.appendChild(txt('p', 'rm-step-explanation', subsystem.why_it_matters));
      var sourceLines = el('div', 'rm-code-location-list');
      (subsystem.evidence || []).forEach(function (statement) {
        var line = el('div', 'rm-summary-line');
        appendLinkifiedText(line, statement);
        sourceLines.appendChild(line);
      });
      if (sourceLines.childNodes.length) card.appendChild(sourceLines);
      grid.appendChild(card);
    });
    return grid.childNodes.length ? grid : null;
  }

  function mountArchitectureCanvas() {
    if (!architectureCanvasHost || !DATA.architecture_canvas || !window.RepomapArchitectureCanvas) {
      return Promise.resolve(null);
    }
    if (architectureCanvasView) {
      architectureCanvasView.destroy();
      architectureCanvasView = null;
    }
    architectureAppliedFocus = null;
    var options = {
      userMode: !DEBUG_MODE,
      message: msg,
      presentationText: DEBUG_MODE ? (DATA.architecture_debug_presentation || {}) : {},
      stalePaths: new Set((DATA.freshness && DATA.freshness.affected_paths) || []),
		onInspectorVisibilityChange: function (visible) {
			setMapEmptyInspectorDetailVisible(visible);
		},
    };
	if (!DEBUG_MODE) {
		options.componentContexts = architectureComponentContexts();
		options.openStudyDirection = openStudyDirection;
		options.openStudyTheme = openThemeCard;
		options.openSourceLocation = openSourceLocation;
		options.associations = DATA.architecture_associations || null;
	}
    if (staticSourceMode()) {
      options.openLocation = function (filePath, line, column) {
        return openStaticSource({ path: filePath, line: line || 0, column: column || 0 });
      };
      // Decision 230 (fresh review task-4 minor): static reports render
      // witness/edge jumps as real links (pinned revision, target=_blank,
      // rel=noopener noreferrer) instead of buttons that call window.open.
      options.staticSourceURL = function (filePath, line) {
        return staticSourceURL(filePath, line || 0, 0);
      };
    } else if (serverMode() && currentRunID()) {
      options.openLocation = function (filePath, line, column) {
        if (!OPENABLE_PATH_SET[filePath]) return;
        return requestOpenFile(filePath, line || 0, column || 0);
      };
    }
    architectureCanvasView = window.RepomapArchitectureCanvas.mount(
      architectureCanvasHost,
      userArchitectureData(),
      options
    );
    architectureReady = Promise.resolve(architectureCanvasView.ready).then(function () {
      return architectureCanvasView;
    });
    return architectureReady;
  }

  function architectureFocusNeedsReset(appliedFocus, target) {
    return appliedFocus !== null && appliedFocus !== architectureFocusValue(target);
  }

  function focusArchitectureTarget(target) {
    if (!architectureCanvasView) return Promise.resolve(null);
		// D246 Study paths are transient overlays, not focus targets. Keeping
		// them out of the focus/reset path preserves the existing layout and
		// Canvas transform exactly.
		if (target && String(target.kind || '') === 'study_mechanism') {
			return Promise.resolve(architectureCanvasView);
		}
    if (architectureFocusNeedsReset(architectureAppliedFocus, target)) {
      return mountArchitectureCanvas().then(function () {
        return focusArchitectureTarget(target);
      });
    }
    var kind = target ? String(target.kind || '') : '';
    if (target) {
      if (kind === 'component' && target.component_id) architectureCanvasView.openComponent(target.component_id);
      else if (kind === 'flow' && target.flow_id) architectureCanvasView.openTrace(target.flow_id);
      else if (kind === 'flow_step' && target.flow_id && target.step_id) {
        architectureCanvasView.openFlowStep(target.flow_id, target.step_id);
      } else if (kind === 'surface' && target.surface_id) architectureCanvasView.openSurface(target.surface_id);
    }
    architectureAppliedFocus = architectureFocusValue(target);
    return Promise.resolve(architectureCanvasView);
  }

	function applyStudyMechanismMapOverlay() {
		if (!architectureCanvasView) return false;
		var selection = activeStudyMechanismMapSelection();
		if (selection && typeof architectureCanvasView.setStudyMechanismOverlay === 'function') {
			return architectureCanvasView.setStudyMechanismOverlay(selection.mechanism);
		}
		if (typeof architectureCanvasView.clearStudyMechanismOverlay === 'function') {
			architectureCanvasView.clearStudyMechanismOverlay();
		}
		return false;
	}

	function syncMapLensControl() {
		document.querySelectorAll('[data-map-lens]').forEach(function (node) {
			var active = node.getAttribute('data-map-lens') === activeMapLens;
			node.classList.toggle('rm-active', active);
			node.setAttribute('aria-pressed', active ? 'true' : 'false');
		});
	}

  function showStudyMechanismTargetOnMap(target) {
		if (!target || !userArchitectureAvailable()) return;
    var next = reduceWorkspaceState(workspaceState, { type: 'show_map', target: target });
    commitWorkspaceState(next);
  }

  function openArchitectureTarget(target, returnTarget, options) {
		options = options || {};
		if (!userArchitectureAvailable()) {
			commitWorkspaceState(emptyWorkspaceState(), { replace: !!options.replace });
			return;
		}
    var next = reduceWorkspaceState(workspaceState, { type: 'view', view: 'map' });
    next.mapReturn = returnTarget || null;
    next.mapTarget = target || null;
		commitWorkspaceState(next, { replace: !!options.replace });
  }

  function returnFromArchitecture() {
    var returnTarget = workspaceState.mapReturn;
    if (!returnTarget) return;
    var next = reduceWorkspaceState(workspaceState, { type: 'return_from_map' });
    commitWorkspaceState(next, { replace: true });
  }

  function renderProvenanceWorkspace() {
    var root = document.getElementById('rm-provenance');
    if (!root || !DEBUG_MODE) return;
    root.replaceChildren();
    root.appendChild(renderViewHeading(
      msg('main.debug_mode'),
      msg('main.validation_and_provenance'),
      msg('main.provenance_copy')
    ));
    var payload = {
      semantic_artifacts: DATA.semantic_artifacts || [],
      semantic_coverage: DATA.semantic_coverage || null,
      warnings: DATA.warnings || [],
      architecture_synthesis: DATA.architecture_synthesis || null,
      architecture_grounding: DATA.architecture_grounding || null,
      run: DATA.run || null,
    };
    root.appendChild(txt('pre', 'rm-provenance-card', JSON.stringify(payload, null, 2)));
    mountDebugSurfaceCatalog(root);
  }

  function mountDebugSurfaceCatalog(root) {
    if (!DEBUG_MODE || !root || !DATA.discovered_surfaces || !window.RepomapSurfaceCatalog) return;
    if (surfaceCatalogView) {
      surfaceCatalogView.destroy();
      surfaceCatalogView = null;
    }
    var host = el('div', 'rm-surface-catalog-host');
    root.appendChild(host);
    var options = {
      message: msg,
      architectureSurfaces: (DATA.architecture_canvas && DATA.architecture_canvas.surfaces) || [],
      architectureAnchorCount: DATA.run && DATA.run.architecture_anchor_count || 0,
      // Decision 236 (v11): backend association view-model for the
      // Integrations lens (closed family touchpoint objects).
      associations: DATA.architecture_associations || [],
      openTrace: function (flowID) {
        openArchitectureTarget({ kind: 'flow', flow_id: flowID }, null);
      },
      openSurface: function (surfaceID) {
        openArchitectureTarget({ kind: 'surface', surface_id: surfaceID }, null);
      },
      openComponent: function (componentID) {
        openArchitectureTarget({ kind: 'component', component_id: componentID }, null);
      },
    };
    if (staticSourceMode()) {
      options.openLocation = function (location) {
        if (!location) return false;
        return openStaticSource(location, location.end_line);
      };
    } else if (serverMode() && currentRunID()) {
      options.openLocation = function (location) {
        if (!location || !OPENABLE_PATH_SET[location.path]) return;
        return requestOpenFile(location.path, location.line || 0, location.column || 0);
      };
    }
    surfaceCatalogView = window.RepomapSurfaceCatalog.mount(host, DATA.discovered_surfaces, options);
  }

	function activateWorkspaceView(view) {
    var sectionID = viewSectionID(view);
    document.querySelectorAll('.rm-main-content > .rm-tab-content').forEach(function (section) {
      section.classList.toggle('rm-active', section.id === sectionID);
    });
		var navView = view === 'study'
				? ((STUDY_DIRECTIONS.length || themeCards().length) ? 'study_overview' : 'overview')
				: view === 'operate'
					? 'map'
					: view;
    document.querySelectorAll('[data-workspace-view]').forEach(function (button) {
      var active = button.getAttribute('data-workspace-view') === navView;
      button.classList.toggle('rm-active', active);
      if (active) button.setAttribute('aria-current', 'page');
      else button.removeAttribute('aria-current');
    });
  }

	function syncInlineStudyLocation() {
		if (workspaceState.themeCardOrdinal) {
			openThemeCard(workspaceState.themeCardOrdinal, { history: false });
			return;
		}
		if (String(window.location && window.location.hash || '') !== '#study') return;
		var study = document.getElementById('rm-study');
		if (study && typeof study.scrollIntoView === 'function') study.scrollIntoView({ block: 'start' });
	}

  function renderWorkspaceState() {
		if (workspaceState.view === 'investigate') renderTaskInvestigationWorkspace();
		if (workspaceState.view === 'operate') renderOperateDetailWorkspace();
		// Decision 236 (v11): map is the primary product — it renders the
		// architecture workspace (Landscape lens); architecture remains an
		// accepted alias. A canvas-less report still shows the map
		// workspace with the repository summary (thesis/brief/atlas shelf).
    if ((workspaceState.view === 'map' || workspaceState.view === 'architecture') && !DATA.architecture_canvas) {
      // Decision 236 (v11): a canvas-less report still shows the map
      // workspace — the repository summary renders into the map section
      // (thesis/brief/atlas shelf) from report data that exists without a
      // canvas.
      renderMapSummaryInto('rm-architecture');
			appendInlineStudyWorkspace(document.getElementById('rm-architecture'));
      activateWorkspaceView(workspaceState.view);
			syncInlineStudyLocation();
    } else if (workspaceState.view === 'map' || workspaceState.view === 'architecture') {
      if (!architectureCanvasHost) renderArchitectureWorkspace();
		if (!workspaceState.mapTarget) restoreDefaultTargetCanvasContext();
		renderStudyMechanismMapContextIntoHost();
      activateWorkspaceView(workspaceState.view);
		syncInlineStudyLocation();
      var ready = architectureCanvasView ? (architectureReady || Promise.resolve(architectureCanvasView)) : mountArchitectureCanvas();
      ready.then(function () {
			applyStudyMechanismMapOverlay();
        if (architectureCanvasView && typeof architectureCanvasView.setLens === 'function') {
          architectureCanvasView.setLens(activeMapLens);
        }
			if (selectedEntrypointHandoffGroupID && architectureCanvasView &&
				typeof architectureCanvasView.selectEntrypointHandoffGroup === 'function') {
				architectureCanvasView.selectEntrypointHandoffGroup(selectedEntrypointHandoffGroupID);
			}
			syncMapLensControl();
        renderMapLensObjects();
        focusArchitectureTarget(workspaceState.mapTarget);
      });
    } else {
      activateWorkspaceView(workspaceState.view);
    }
    renderSourceDrawer();
  }

  function renderRunDetails() {
    var details = document.getElementById('rm-run-details');
    if (!details) return;
    // The "About this report" disclosure is a compact provenance row, never
    // a debug-only telemetry dump (Decision 217).
    details.hidden = false;
    var provenanceRow = document.getElementById('rm-provenance-row');
    if (provenanceRow) {
      provenanceRow.replaceChildren();
      var chips = [];
      if (DATA.captured_revision) {
        chips.push(msg('main.provenance.revision', { revision: DATA.captured_revision.slice(0, 12) }));
      }
      if (DATA.freshness && DATA.freshness.state) {
        chips.push(msg('main.provenance.freshness', { state: provenanceFreshnessLabel(DATA.freshness.state) }));
      }
			if (STATIC_SOURCE_LINKS && STATIC_SOURCE_LINKS.working_tree_dirty) {
				chips.push(msg('main.provenance.local_changes_captured'));
			}
      if (DATA.report_language) {
        chips.push(msg('main.provenance.language', { language: DATA.report_language }));
      }
      chips.forEach(function (chip) {
        provenanceRow.appendChild(txt('span', 'rm-provenance-chip', chip));
      });
    }
    if (DATA.artifacts_dir) {
      document.getElementById('rm-artifacts-dir').textContent = msg('main.run.artifacts', {
        path: DATA.artifacts_dir,
      });
    }
    if (DATA.feedback_path) {
      document.getElementById('rm-feedback-path').textContent = msg('main.run.feedback_notes', {
        path: DATA.feedback_path,
      });
    }
    if (DATA.captured_revision) {
      document.getElementById('rm-snapshot-detail').textContent = msg('main.captured_snapshot', {
        revision: DATA.captured_revision.slice(0, 12),
        count: DATA.captured_input_count || 0,
      });
    }
    if (DATA.freshness) {
      document.getElementById('rm-freshness-detail').textContent = msg('main.current_freshness', {
        state: provenanceFreshnessLabel(DATA.freshness.state),
      });
    }
    var submodules = DATA.repository_submodules || [];
    if (submodules.length) {
      document.getElementById('rm-submodule-detail').textContent = msg('main.run.excluded_submodules', {
        count: submodules.length,
      });
    }
  }

  function provenanceFreshnessLabel(state) {
    var value = String(state || '');
    switch (value) {
      case 'fresh': return msg('main.freshness.state.fresh');
      case 'dirty': return msg('main.freshness.state.dirty');
      case 'stale': return msg('main.freshness.state.stale');
      case 'missing': return msg('main.freshness.state.missing');
      case 'unavailable': return msg('main.freshness.state.unavailable');
      default: return value;
    }
  }

	function renderWorkspaceTabs() {
		var tabs = document.getElementById('rm-tabs');
		if (!tabs) return;
		tabs.replaceChildren();
		if (TASK_INVESTIGATION) {
			addWorkspaceTab(msg('main.task'), 'investigate');
		} else {
			renderAnalysisTargetMenu(tabs);
		}
		if (DEBUG_MODE) addWorkspaceTab(msg('main.provenance'), 'provenance');
	}

	function analysisTargetWorkspacePurpose() {
		if (!ANALYSIS_TARGET || Number(ANALYSIS_TARGET.version) !== 2) return '';
		var kind = String(ANALYSIS_TARGET.kind || '');
		var packageDir = String(ANALYSIS_TARGET.package_dir || '');
		var packagePath = String(ANALYSIS_TARGET.package_path || '');
		if (kind === 'executable_package' && packageDir) {
			return msg('main.analysis_target.executable_scope', { scope: packageDir });
		}
		if (kind === 'library_package') {
			var libraryScope = packageDir && packageDir !== '.' ? packageDir : packagePath;
			if (libraryScope) return msg('main.analysis_target.library_scope', { scope: libraryScope });
		}
		if (kind === 'module_library') {
			var moduleScope = String(ANALYSIS_TARGET.module_path || ANALYSIS_TARGET.module_dir || '');
			if (moduleScope) return msg('main.analysis_target.module_library_scope', { scope: moduleScope });
		}
		return '';
	}

	function workspacePurposeText() {
		var targetPurpose = analysisTargetWorkspacePurpose();
		if (targetPurpose) return targetPurpose;
		if (TASK_INVESTIGATION) return msg('main.chrome.task.investigation');
		return msg('main.repository.onboarding');
	}

  function render() {
    var repoName = document.getElementById('rm-repo-name');
    if (repoName) repoName.textContent = DATA.repo_name || msg('main.repository');
		var workspacePurpose = document.getElementById('rm-workspace-purpose');
		if (workspacePurpose) workspacePurpose.textContent = workspacePurposeText();
    setupServerFeatures();
    renderRunDetails();

		renderWorkspaceTabs();

		if (TASK_INVESTIGATION) {
			renderTaskInvestigationWorkspace();
		} else {
			renderOperateDetailWorkspace();
		}
    renderProvenanceWorkspace();
    restoreWorkspaceFromRoute();
  }

  if (window.__REPOMAP_WORKSPACE_TEST__ && typeof window.__REPOMAP_WORKSPACE_TEST__ === 'object') {
    Object.assign(window.__REPOMAP_WORKSPACE_TEST__, {
      architectureFocusValue: architectureFocusValue,
		analysisTargetWorkspacePurpose: analysisTargetWorkspacePurpose,
		analysisTargetMenuItems: analysisTargetMenuItems,
		renderAnalysisTargetMenu: renderAnalysisTargetMenu,
		defaultEntrypointHandoffGroupID: defaultEntrypointHandoffGroupID,
		workspacePurposeText: workspacePurposeText,
      architectureFocusNeedsReset: architectureFocusNeedsReset,
      architectureTargetFromFocus: architectureTargetFromFocus,
		architectureComponentContexts: architectureComponentContexts,
      parseWorkspaceHash: parseWorkspaceHash,
      workspaceHashForState: workspaceHashForState,
			workspaceRouteFamily: workspaceRouteFamily,
      userArchitectureAvailable: userArchitectureAvailable,
		userArchitectureData: userArchitectureData,
      reduceWorkspaceState: reduceWorkspaceState,
      renderExactReferences: renderExactReferences,
      renderSourceCode: renderSourceCode,
      renderSourceSnippetCard: renderSourceSnippetCard,
      sourceSnippetIdentity: sourceSnippetIdentity,
      uniqueSourceSnippets: uniqueSourceSnippets,
      remainingExactReferences: remainingExactReferences,
      overviewSourceIsStrong: overviewSourceIsStrong,
      overviewSourceRoleLabel: overviewSourceRoleLabel,
			renderMapSummaryInto: renderMapSummaryInto,
			renderMapEmptyInspector: renderMapEmptyInspector,
			repositoryOverviewAnatomy: repositoryOverviewAnatomy,
			overviewEntrySurfaceObjects: overviewEntrySurfaceObjects,
			overviewComponentObjects: overviewComponentObjects,
			overviewSurfaceKindLabel: overviewSurfaceKindLabel,
			exactOverviewSourceForLocation: exactOverviewSourceForLocation,
			renderRepositoryOverviewAnatomy: renderRepositoryOverviewAnatomy,
			repositoryAtlasWorkspaceShelf: repositoryAtlasWorkspaceShelf,
			renderRepositoryAtlasWorkspaceShelf: renderRepositoryAtlasWorkspaceShelf,
// repomap-source-episode:start
			renderSourceEpisode: renderSourceEpisode,
			renderSourceEpisodeClaim: renderSourceEpisodeClaim,
			sourceEpisodeStateLabel: sourceEpisodeStateLabel,
			sourceEpisodeSourceAvailable: sourceEpisodeSourceAvailable,
// repomap-source-episode:end
		renderTaskInvestigationWorkspace: renderTaskInvestigationWorkspace,
				taskLensEnumLabel: taskLensEnumLabel,
      renderIncompleteStudyOverview: renderIncompleteStudyOverview,
		renderStudyInvestigation: renderStudyInvestigation,
		renderStudyMechanismMapContext: renderStudyMechanismMapContext,
		studyMechanismForTarget: studyMechanismForTarget,
		renderStudyDirectionCard: renderStudyDirectionCard,
		renderStudyReadingAnchor: renderStudyReadingAnchor,
		renderReadableDocument: renderReadableDocument,
		renderReadableDocumentCard: renderReadableDocumentCard,
		renderOperateDetailWorkspace: renderOperateDetailWorkspace,
      renderPavedPathCard: renderPavedPathCard,
      sourceNoticeRanges: sourceNoticeRanges,
      embeddedSourceForLocation: embeddedSourceForLocation,
		openStudyDirection: openStudyDirection,
		openPavedPath: openPavedPath,
      openSourceSnippet: openSourceSnippet,
      closeSourceDrawer: closeSourceDrawer,
		activeSourceDrawerOperation: activeSourceDrawerOperation,
      navigateWorkspace: navigateWorkspace,
		showStudyMechanismOnMap: showStudyMechanismOnMap,
      returnFromArchitecture: returnFromArchitecture,
      restoreWorkspaceFromRoute: restoreWorkspaceFromRoute,
      workspaceStateSnapshot: function () { return JSON.parse(JSON.stringify(workspaceState)); },
		pavedPathByID: pavedPathByID,
      viewSectionID: viewSectionID,
      staticSourceMode: staticSourceMode,
      staticSourceURL: staticSourceURL,
      gitLabSourceMode: staticSourceMode,
      gitLabSourceURL: staticSourceURL,
      renderFileReference: renderFileReference,
      openSourceLocation: openSourceLocation,
      sourceLocationActionAvailable: sourceLocationActionAvailable,
      sourceSnippetAvailable: sourceSnippetAvailable,
      renderRepositoryArea: renderRepositoryArea,
      renderOperationalLandmark: renderOperationalLandmark,
      serverMode: serverMode,
      setupServerFeatures: setupServerFeatures,
      runPickerDate: runPickerDate,
      runPickerShortID: runPickerShortID,
      runPickerLabel: runPickerLabel,
      renderArchitectureWorkspace: renderArchitectureWorkspace,
		renderWorkspaceTabs: renderWorkspaceTabs,
		renderRunDetails: renderRunDetails,
		provenanceFreshnessLabel: provenanceFreshnessLabel,
      mountArchitectureCanvas: mountArchitectureCanvas,
      mountDebugSurfaceCatalog: mountDebugSurfaceCatalog,
      txt: txt,
    });
  }

  window.addEventListener('hashchange', scheduleWorkspaceRouteRestore);
  window.addEventListener('popstate', scheduleWorkspaceRouteRestore);
  window.addEventListener('DOMContentLoaded', function () {
    bindFixedMessages(document);
    render();
  });
})();
