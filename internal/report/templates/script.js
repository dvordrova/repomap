(function () {
  'use strict';

  var DATA = JSON.parse(document.getElementById('rm-report-data').textContent);
  var REPORT_LANGUAGE = DATA.report_language === 'ru' ? 'ru' : 'en';
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
  var symbolLookupStates = {};
  var symbolLookupViews = {};
  var componentSelectionViews = {};
  var architectureCanvasView = null;
  var surfaceCatalogView = null;
  var resumeInvestigationStarted = false;
  var maxSymbolCandidates = 8;
  var maxStaticCalls = 5;
  var maxSourceLines = 20;
  var maxTestReferences = 5;
  var DEBUG_MODE = /^(1|true)$/i.test(new URLSearchParams(window.location.search).get('debug') || '');
	var USER_MECHANISMS = Array.isArray(DATA.user_mechanisms) ? DATA.user_mechanisms : [];
	var USER_TOPICS = Array.isArray(DATA.user_topics) ? DATA.user_topics : [];
	var USER_SOURCES = Array.isArray(DATA.user_sources) ? DATA.user_sources : [];
	var NAVIGATOR = DATA.navigator || null;
	var ATLAS_FIRST = !!(
		NAVIGATOR && Number(NAVIGATOR.version) === 1 &&
		(NAVIGATOR.state === 'selected' || NAVIGATOR.state === 'empty' ||
			NAVIGATOR.state === 'unavailable')
	);
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
  var workspaceState = {
		view: TASK_INVESTIGATION ? 'investigate' : 'overview',
		taskID: TASK_INVESTIGATION && TASK_INVESTIGATION.task_id || '',
    artifactID: '',
		directionID: '',
		themeCardOrdinal: 0,
		operationID: '',
    stepIndex: 0,
    sourceLocation: null,
    mapReturn: null,
    mapTarget: null,
  };
  var architectureCanvasHost = null;
  var architectureReady = null;
  var architectureAppliedFocus = null;
	var activeOverviewTopicID = '';

  var LABELS = {
    purpose: msg('main.project.purpose.model.orientation'),
    explore: msg('main.explore.this.repository'),
    systemMap: msg('main.components.model.orientation'),
    startFiles: msg('main.start.here'),
    terms: msg('main.important.terms'),
    questions: msg('main.questions.for.a.teammate'),
    model: msg('main.model'),
    localContext: msg('main.compact.local.context'),
    externalRequest: msg('main.provider.request.bodies'),
    providerRequests: msg('main.provider.requests'),
    providerLatency: msg('main.orientation.latency'),
    surfaceAnalysis: msg('main.generic.surface.scan'),
    architectureAnchors: msg('main.architecture.anchors'),
    snapshotFreshness: msg('main.analyzed.input.freshness'),
    architectureGrouping: msg('main.architecture.grouping'),
    directionsFound: msg('main.suggested.investigations'),
    rejectedDirections: msg('main.rejected.suggestions'),
    savedFlows: msg('main.saved.traces'),
    candidateFlows: msg('main.saved.traces'),
    candidateDirections: msg('main.suggested.investigations'),
    directionHint: msg('main.choose.a.direction.to.get.a.focused.starting.point.in.the.repository'),
    trigger: msg('main.starts.when'),
    likelyEntrypoint: msg('main.likely.entrypoint'),
    likelyFiles: msg('main.likely.files'),
    orientationEvidence: msg('main.why.the.model.suggested.this'),
    verifiedEvidence: msg('main.locally.verified'),
    missingEvidence: msg('main.missing.evidence'),
    verifiedFlow: msg('main.evidence.backed.flow'),
    proofSlots: msg('main.proof.coverage'),
    proofStop: msg('main.current.proof.boundary'),
    filesToRead: msg('main.read.order.open.these.files.in.sequence'),
    testsToRead: msg('main.tests'),
    executionChain: msg('main.execution.chain'),
    knownUnknowns: msg('main.known.unknowns'),
    unverified: msg('main.unverified.paths'),
    unknowns: msg('main.unknowns'),
    warnings: msg('main.warnings'),
    retrievalDetails: msg('main.retrieval.details'),
    noFlows: msg('main.no.candidate.directions.were.produced'),
    startHere: msg('main.start.here'),
    suggestedStart: msg('main.suggested.start'),
    quickStart: msg('main.quick.start'),
    errorUnavailable: msg('main.analysis.unavailable'),
    openLocalEvidence: msg('main.explore.this.direction'),
    localEvidenceIntro: msg('main.suggested.files.are.selected.from.repository.facts.treat.them.as.a.starting.point.not.a.verified.runtime.trace'),
    localEvidenceLegacyIntro: msg('main.suggested.files.come.from.a.saved.repository.snapshot.treat.them.as.a.starting.point.not.a.verified.runtime.trace'),
    suggestedFiles: msg('main.suggested.files.to.inspect'),
    evidenceFiles: msg('main.evidence.files'),
    showAll: function (count) { return msg('main.list.show_all', { count: count }); },
    showMore: function (count) { return msg('main.list.show_more', { count: count }); },
    showLess: msg('main.show.less'),
  };

  function userMechanismByID(mechanisms, artifactID) {
    mechanisms = Array.isArray(mechanisms) ? mechanisms : [];
    for (var index = 0; index < mechanisms.length; index++) {
      if (mechanisms[index] && mechanisms[index].artifact_id === artifactID) return mechanisms[index];
    }
    return null;
  }

	function userTopicByID(candidateID) {
		for (var index = 0; index < USER_TOPICS.length; index++) {
			if (USER_TOPICS[index] && USER_TOPICS[index].candidate_id === candidateID) {
				return USER_TOPICS[index];
			}
		}
		return null;
	}

	function mixedShelfAvailable() {
		return !!(USER_MECHANISMS.length || USER_TOPICS.length || INCOMPLETE_STUDY_DIRECTIONS.length);
	}

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

	function pavedPathByID(pavedPathID) {
		for (var index = 0; index < PAVED_PATHS.length; index++) {
			if (PAVED_PATHS[index] && PAVED_PATHS[index].id === pavedPathID) return PAVED_PATHS[index];
		}
		return null;
	}

  function mechanismNarrativeItems(mechanism) {
    var phases = mechanism && Array.isArray(mechanism.phases)
      ? mechanism.phases.filter(Boolean)
      : [];
    if (phases.length) return phases;
    return mechanism && Array.isArray(mechanism.steps) ? mechanism.steps.filter(Boolean) : [];
  }

  function mechanismUsesPhases(mechanism) {
    return !!(mechanism && Array.isArray(mechanism.phases) && mechanism.phases.length);
  }

  function mechanismImplementationSteps(mechanism, phase) {
    if (!mechanismUsesPhases(mechanism) || !phase) return [];
    var steps = Array.isArray(mechanism.steps) ? mechanism.steps : [];
    var seen = {};
    var indexed = (Array.isArray(phase.implementation_step_indexes)
      ? phase.implementation_step_indexes
      : []).map(function (index) {
        index = Number(index);
        if (!Number.isInteger(index) || index < 0 || index >= steps.length || seen[index]) return null;
        seen[index] = true;
        return steps[index];
      }).filter(Boolean);
    var attached = Array.isArray(phase.implementation_details)
      ? phase.implementation_details.filter(Boolean)
      : [];
    return indexed.concat(attached);
  }

  function narrativeIndexForImplementationStep(mechanism, stepIndex) {
    var items = mechanismNarrativeItems(mechanism);
    if (!items.length || !mechanismUsesPhases(mechanism)) {
      return boundedMechanismStep(mechanism, stepIndex);
    }
    stepIndex = Number(stepIndex);
    for (var index = 0; index < items.length; index++) {
      var members = Array.isArray(items[index].implementation_step_indexes)
        ? items[index].implementation_step_indexes
        : [];
      if (members.some(function (candidate) { return Number(candidate) === stepIndex; })) return index;
    }
    return 0;
  }

  function boundedMechanismStep(mechanism, requested) {
    var steps = mechanismNarrativeItems(mechanism);
    if (!steps.length) return 0;
    requested = Number.isFinite(Number(requested)) ? Math.trunc(Number(requested)) : 0;
    return Math.max(0, Math.min(steps.length - 1, requested));
  }

  function emptyWorkspaceState() {
    return {
			view: TASK_INVESTIGATION ? 'investigate' : 'overview',
			taskID: TASK_INVESTIGATION && TASK_INVESTIGATION.task_id || '',
      artifactID: '',
		directionID: '',
		themeCardOrdinal: 0,
		operationID: '',
      stepIndex: 0,
      sourceLocation: null,
      mapReturn: null,
      mapTarget: null,
    };
  }

	function defaultWorkspaceHash() {
		if (TASK_INVESTIGATION && TASK_INVESTIGATION.task_id) {
			return '#/investigate/' + encodeRoutePart(TASK_INVESTIGATION.task_id);
		}
		return '#/overview';
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

  function workspaceHashForState(state, mechanismRoot) {
    state = state || emptyWorkspaceState();
		if (state.view === 'investigate' && TASK_INVESTIGATION && TASK_INVESTIGATION.task_id) {
			return defaultWorkspaceHash();
		}
    if (state.view === 'mechanisms') return '#/mechanisms';
    if (state.view === 'study_overview' && (STUDY_DIRECTIONS.length || themeCards().length)) return '#/study';
    if (state.view === 'architecture') {
      var focus = architectureFocusValue(state.mapTarget);
      return '#/architecture' + (focus ? '?focus=' + encodeURIComponent(focus) : '');
    }
    if (state.view === 'mechanism' && state.artifactID) {
      var mechanismHash = '#/mechanism/' + encodeRoutePart(state.artifactID);
      return mechanismRoot ? mechanismHash : mechanismHash + '/step/' + (Number(state.stepIndex) + 1);
    }
		if (state.view === 'study' && state.directionID) {
			return '#/study/' + encodeRoutePart(state.directionID);
		}
		if (state.view === 'study' && state.themeCardOrdinal) {
			return '#/study/theme/' + Number(state.themeCardOrdinal);
		}
		if (state.view === 'operate' && state.operationID) {
			return '#/operate/' + encodeRoutePart(state.operationID);
		}
    if (state.view === 'provenance' && DEBUG_MODE) return '#/provenance';
    return '#/overview';
  }

	function workspaceRouteFamily(state) {
		state = state || emptyWorkspaceState();
		if (state.view === 'investigate' && state.taskID) {
			return 'investigate:' + state.taskID;
		}
		if (state.view === 'mechanism' && state.artifactID) {
			return 'mechanism:' + state.artifactID;
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
		return 'view:' + (state.view || 'overview');
	}

	function resetWorkspaceScroll() {
		if (typeof window.scrollTo === 'function') window.scrollTo(0, 0);
	}

  function parseWorkspaceHash(hash, mechanisms, historyState) {
    hash = String(hash || '');
    var state = emptyWorkspaceState();
		var route = hash.replace(/^#/, '') || (TASK_INVESTIGATION ? defaultWorkspaceHash().replace(/^#/, '') : '/overview');
    var queryIndex = route.indexOf('?');
    var query = queryIndex >= 0 ? route.slice(queryIndex + 1) : '';
    var path = queryIndex >= 0 ? route.slice(0, queryIndex) : route;
    var segments = path.replace(/^\/+|\/+$/g, '').split('/').filter(Boolean);
    var valid = true;
		var canonicalHash = defaultWorkspaceHash();

    if (segments.length === 1 && segments[0] === 'overview') {
			if (TASK_INVESTIGATION) {
				state = emptyWorkspaceState();
				valid = false;
			} else {
				canonicalHash = '#/overview';
			}
		} else if (segments.length === 2 && segments[0] === 'investigate') {
			var routeTaskID = decodeRoutePart(segments[1]);
			if (!TASK_INVESTIGATION || routeTaskID !== TASK_INVESTIGATION.task_id) {
				valid = false;
			} else {
				state.view = 'investigate';
				state.taskID = routeTaskID;
				canonicalHash = defaultWorkspaceHash();
			}
    } else if (segments.length === 1 && segments[0] === 'mechanisms') {
      state.view = mechanisms && mechanisms.length ? 'mechanisms' : 'overview';
      valid = state.view === 'mechanisms';
      canonicalHash = valid ? '#/mechanisms' : '#/overview';
		} else if (segments.length === 1 && segments[0] === 'study') {
			state.view = (STUDY_DIRECTIONS.length || themeCards().length) ? 'study_overview' : 'overview';
			valid = state.view === 'study_overview';
			canonicalHash = valid ? '#/study' : '#/overview';
    } else if (segments.length === 1 && segments[0] === 'architecture') {
      state.view = 'architecture';
      var focus = new URLSearchParams(query).get('focus') || '';
      state.mapTarget = architectureTargetFromFocus(focus);
      state.mapReturn = historyState && historyState.mapReturn || null;
      canonicalHash = workspaceHashForState(state);
    } else if (segments.length === 1 && segments[0] === 'provenance' && DEBUG_MODE) {
      state.view = 'provenance';
      canonicalHash = '#/provenance';
    } else if (segments.length === 2 && segments[0] === 'mechanism') {
      var rootArtifactID = decodeRoutePart(segments[1]);
      var rootMechanism = userMechanismByID(mechanisms, rootArtifactID);
      if (!rootMechanism || !mechanismNarrativeItems(rootMechanism).length) {
        valid = false;
      } else {
        state.view = 'mechanism';
        state.artifactID = rootArtifactID;
        canonicalHash = workspaceHashForState(state, true);
      }
		} else if (segments.length === 2 && segments[0] === 'study') {
			var routeDirectionID = decodeRoutePart(segments[1]);
			var routeDirection = studyDirectionByID(routeDirectionID);
			if (!routeDirection) {
				valid = false;
			} else if (routeDirection.mechanism_id && userMechanismByID(mechanisms, routeDirection.mechanism_id)) {
				state.view = 'mechanism';
				state.artifactID = routeDirection.mechanism_id;
				state.stepIndex = 0;
				canonicalHash = workspaceHashForState(state, true);
			} else {
				state.view = 'study';
				state.directionID = routeDirectionID;
				canonicalHash = workspaceHashForState(state);
			}
		} else if (segments.length === 3 && segments[0] === 'study' && segments[1] === 'theme') {
			var routeOrdinal = Number(segments[2]);
			if (!themeCardByOrdinal(routeOrdinal)) {
				valid = false;
			} else {
				state.view = 'study';
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
    } else if (segments.length === 4 && segments[0] === 'mechanism' && segments[2] === 'step') {
      var artifactID = decodeRoutePart(segments[1]);
      var mechanism = userMechanismByID(mechanisms, artifactID);
      var humanStep = Number(segments[3]);
      if (!mechanism || !mechanismNarrativeItems(mechanism).length || !Number.isInteger(humanStep) || humanStep < 1) {
        valid = false;
      } else {
        state.view = 'mechanism';
        state.artifactID = artifactID;
        state.stepIndex = boundedMechanismStep(mechanism, humanStep - 1);
        canonicalHash = workspaceHashForState(state);
      }
    } else {
      valid = false;
    }

		if (!valid) state = emptyWorkspaceState();
		return { valid: valid, canonicalHash: valid ? canonicalHash : defaultWorkspaceHash(), state: state };
  }

  function reduceWorkspaceState(state, action, mechanisms) {
    state = state || workspaceState;
    action = action || {};
    var next = {
      view: state.view || 'overview',
			taskID: state.taskID || '',
      artifactID: state.artifactID || '',
		directionID: state.directionID || '',
		themeCardOrdinal: Number(state.themeCardOrdinal) || 0,
		operationID: state.operationID || '',
      stepIndex: Number(state.stepIndex) || 0,
      sourceLocation: state.sourceLocation || null,
      mapReturn: state.mapReturn || null,
      mapTarget: state.mapTarget || null,
    };
    var mechanism;
    switch (action.type) {
    case 'view':
      next.view = action.view || 'overview';
      if (next.view !== 'architecture') {
        next.mapTarget = null;
        if (!action.keepReturn) next.mapReturn = null;
      }
      return next;
    case 'open_mechanism':
      mechanism = userMechanismByID(mechanisms, action.artifactID);
      if (!mechanism) return next;
      next.view = 'mechanism';
      next.artifactID = mechanism.artifact_id;
		next.directionID = '';
		next.operationID = '';
      next.stepIndex = boundedMechanismStep(mechanism, action.stepIndex);
      next.mapTarget = null;
      next.mapReturn = null;
      return next;
		case 'open_study':
			var direction = studyDirectionByID(action.directionID);
			if (!direction || direction.mechanism_id) return next;
			next.view = 'study';
			next.directionID = direction.id;
			next.themeCardOrdinal = 0;
			next.artifactID = '';
			next.operationID = '';
			next.stepIndex = 0;
			next.mapTarget = null;
			next.mapReturn = null;
			return next;
		case 'open_study_theme':
			if (!themeCardByOrdinal(action.ordinal)) return next;
			next.view = 'study';
			next.directionID = '';
			next.themeCardOrdinal = Number(action.ordinal) || 0;
			next.artifactID = '';
			next.operationID = '';
			next.stepIndex = 0;
			next.mapTarget = null;
			next.mapReturn = null;
			return next;
		case 'open_operation':
			var pavedPath = pavedPathByID(action.operationID);
			if (!pavedPath) return next;
			next.view = 'operate';
			next.operationID = pavedPath.id;
			next.artifactID = '';
			next.directionID = '';
			next.stepIndex = 0;
			next.mapTarget = null;
			next.mapReturn = null;
			return next;
    case 'select_step':
      mechanism = userMechanismByID(mechanisms, next.artifactID);
      if (!mechanism) return next;
      next.stepIndex = boundedMechanismStep(mechanism, action.stepIndex);
      return next;
    case 'move_step':
      mechanism = userMechanismByID(mechanisms, next.artifactID);
      if (!mechanism) return next;
      next.stepIndex = boundedMechanismStep(mechanism, next.stepIndex + Number(action.delta || 0));
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
      next.mapReturn = { artifactID: next.artifactID, stepIndex: next.stepIndex };
      next.mapTarget = action.target;
      next.view = 'architecture';
      return next;
    case 'return_from_map':
      if (!next.mapReturn) return next;
		if (next.mapReturn.directionID) {
			var returnDirection = studyDirectionByID(next.mapReturn.directionID);
			if (!returnDirection) return next;
			next.view = 'study';
			next.directionID = returnDirection.id;
			next.artifactID = '';
			next.mapTarget = null;
			next.mapReturn = null;
			return next;
		}
      mechanism = userMechanismByID(mechanisms, next.mapReturn.artifactID);
      if (!mechanism) return next;
      next.view = 'mechanism';
      next.artifactID = mechanism.artifact_id;
      next.stepIndex = boundedMechanismStep(mechanism, next.mapReturn.stepIndex);
      next.mapTarget = null;
      next.mapReturn = null;
      return next;
    default:
      return next;
    }
  }

  function esc(s) {
    if (!s) return '';
    return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }

  function evidenceClass(c) {
    var v = Math.max(0, c || 0);
    if (v <= 0) return 'rm-ev-none';
    return v >= 0.7 ? 'rm-ev-strong' : v >= 0.4 ? 'rm-ev-medium' : 'rm-ev-weak';
  }

  function evidenceLabel(c, verification) {
    var v = Math.max(0, c || 0);
    if (verification) {
      if (v <= 0) return msg('main.evidence_confidence.not_estimated');
      if (v >= 0.7) return msg('main.evidence_confidence.high');
      if (v >= 0.4) return msg('main.evidence_confidence.medium');
      return msg('main.evidence_confidence.low');
    }
    if (v <= 0) return msg('main.model_confidence.not_estimated');
    if (v >= 0.7) return msg('main.model_confidence.high');
    if (v >= 0.4) return msg('main.model_confidence.medium');
    return msg('main.model_confidence.low');
  }

  function chainCircleClass(c) {
    var v = Math.max(0, c || 0);
    if (v <= 0) return 'rm-chain-circle--none';
    return v >= 0.7 ? 'rm-chain-circle--hi' : v >= 0.4 ? 'rm-chain-circle--md' : 'rm-chain-circle--lo';
  }

  function kindClass(kind) {
    if (!kind) return '';
    return 'rm-kind rm-kind--' + kind;
  }

  // ── DOM builders ───────────────────────────────────────────────

  function el(tag, cls, attrs) {
    var e = document.createElement(tag);
    if (cls) e.className = cls;
    if (attrs) Object.keys(attrs).forEach(function (k) { e.setAttribute(k, attrs[k]); });
    return e;
  }

  function txt(tag, cls, text) {
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

  function packageDisplayName(pkg) {
    var packages = ((DATA.repository_graph || {}).packages || []);
    for (var packageIndex = 0; packageIndex < packages.length; packageIndex++) {
      if (packages[packageIndex].canonical_package_path === pkg) {
        return packages[packageIndex].display_path || packages[packageIndex].name || pkg;
      }
    }
    var modules = ((DATA.repository_graph || {}).modules || []).slice().sort(function (a, b) {
      return (b.path || '').length - (a.path || '').length;
    });
    for (var i = 0; i < modules.length; i++) {
      var modulePath = modules[i].path || '';
      if (pkg === modulePath) return DATA.repo_name || modules[i].display_name || modulePath;
      if (modulePath && pkg.indexOf(modulePath + '/') === 0) return pkg.slice(modulePath.length + 1);
    }
    return pkg;
  }

  function packageSourceTarget(pkg) {
    var packages = ((DATA.repository_graph || {}).packages || []);
    for (var packageIndex = 0; packageIndex < packages.length; packageIndex++) {
      var candidate = packages[packageIndex];
      if (!candidate || candidate.canonical_package_path !== pkg) continue;
      var files = (candidate.files || []).map(String).filter(function (filePath) {
        return !!(filePath && OPENABLE_PATH_SET[filePath]);
      }).sort();
      return files.length ? files[0] : '';
    }
    return '';
  }

  function renderPackageReference(pkg) {
    var label = packageDisplayName(pkg);
    var filePath = packageSourceTarget(pkg);
    if (!filePath) {
      var code = txt('code', '', label);
      code.title = pkg;
      return code;
    }
    var reference = renderFileReference(filePath, 'rm-component-package-link', 0, label);
    if (!reference.title) reference.title = pkg;
    return reference;
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

  function formatBytes(value) {
    if (!value || value < 0) return '';
    if (value < 1024) return value + ' B';
    if (value < 1024 * 1024) return (value / 1024).toFixed(1) + ' KiB';
    return (value / (1024 * 1024)).toFixed(1) + ' MiB';
  }

  function humanizeReason(reason) {
    if (!reason) return '';
    if (reason === 'likely_file from candidate_flow') {
      return msg('main.reason.initial_orientation');
    }

    var retrievalReason = reason.indexOf('exact basename match') >= 0 ||
      reason.indexOf('filename contains term') >= 0 ||
      reason.indexOf('directory segment') >= 0 ||
      reason.indexOf('path contains term') >= 0;
    if (!retrievalReason) return reason;

    var lower = reason.toLowerCase();
    if (lower.indexOf('raft') >= 0) return msg('main.reason.raft_signals');
    if (lower.indexOf('grpc') >= 0 || lower.indexOf('rpc') >= 0) return msg('main.reason.rpc_signals');
    if (lower.indexOf('lease') >= 0) return msg('main.reason.lease_signals');
    if (lower.indexOf('wal') >= 0 || lower.indexOf('backend') >= 0 || lower.indexOf('mvcc') >= 0) {
      return msg('main.reason.storage_signals');
    }
    return msg('main.reason.repository_signals');
  }

  // ── Components ──────────────────────────────────────────────────

  function renderEvidenceBadge(confidence, verification) {
    var badge = el('span', 'rm-evidence ' + evidenceClass(confidence));
    badge.textContent = evidenceLabel(confidence, verification);
    return badge;
  }

  function renderLocalVerification(verification) {
    if (!verification || (!verification.verified || !verification.verified.length) && (!verification.missing || !verification.missing.length)) {
      return null;
    }
    var box = el('div', 'rm-local-verification rm-local-verification--' + (verification.status || 'partial'));
    if (verification.verified && verification.verified.length) {
      box.appendChild(txt('div', 'rm-direction-label', LABELS.verifiedEvidence));
      verification.verified.forEach(function (statement) {
        var item = el('div', 'rm-verification-item rm-verification-item--verified');
        appendLinkifiedText(item, statement);
        box.appendChild(item);
      });
    }
    if (verification.missing && verification.missing.length) {
      box.appendChild(txt('div', 'rm-direction-label rm-verification-missing-label', LABELS.missingEvidence));
      verification.missing.forEach(function (statement) {
        box.appendChild(txt('div', 'rm-verification-item rm-verification-item--missing', statement));
      });
    }
    return box;
  }

  function proofAnchorMap(proof) {
    var result = {};
    (proof.anchors || []).forEach(function (anchor) { result[anchor.id] = anchor; });
    return result;
  }

  function proofStatusCounts(proof) {
    var counts = { verified: 0, partial: 0, missing: 0, unresolved: 0 };
    (proof.slots || []).forEach(function (slot) {
      counts[slot.status] = (counts[slot.status] || 0) + 1;
    });
    return counts;
  }

  function renderProofLocation(location, cls) {
    if (!location || !location.path) return null;
    var label = location.path + (location.line ? ':' + location.line : '');
    return renderFileReference(location.path, cls || 'rm-proof-location', location.line || 0, label);
  }

  function proofLocationMatches(left, right) {
    if (!left || !right) return false;
    return left.path === right.path && left.line === right.line && left.column === right.column;
  }

  function proofAnchorLabel(anchor, fallback) {
    return anchor && (anchor.label || anchor.qualified_name || anchor.id) || fallback;
  }

  function isCommandSetupTransition(transition, anchors) {
    var from = anchors[transition.from];
    var to = anchors[transition.to];
    return (transition.id || '').indexOf('dispatch-') === 0 ||
      transition.relation === 'registers_command' ||
      transition.relation === 'dispatches' ||
      from && from.kind === 'command' ||
      to && to.kind === 'command';
  }

  function proofStaticRelationGroups(proof) {
    var anchors = proofAnchorMap(proof);
    var transitions = (proof.transitions || []).slice();
    var transitionsByID = {};
    var assigned = {};
    transitions.forEach(function (transition) { transitionsByID[transition.id] = transition; });

    var commandSetup = transitions.filter(function (transition) {
      if (!isCommandSetupTransition(transition, anchors)) return false;
      assigned[transition.id] = true;
      return true;
    });

    var taskGroups = [];
    (proof.anchors || []).forEach(function (anchor) {
      if (anchor.kind !== 'task') return;
      var taskRelations = transitions.filter(function (transition) {
        if (assigned[transition.id]) return false;
        var belongsToTask = transition.from === anchor.id || transition.to === anchor.id;
        if (belongsToTask) assigned[transition.id] = true;
        return belongsToTask;
      });
      taskGroups.push({
        title: msg('main.proof.task_group', { task: proofAnchorLabel(anchor, anchor.id) }),
        anchor: anchor,
        relations: taskRelations,
      });
    });

    var handlerAnchors = {};
    var applicationSlot = (proof.slots || []).filter(function (slot) {
      return slot.kind === 'application_callable';
    })[0];
    (applicationSlot && applicationSlot.evidence_ids || []).forEach(function (id) {
      var transition = transitionsByID[id];
      if (transition) handlerAnchors[transition.to] = true;
      if (anchors[id]) handlerAnchors[id] = true;
    });
    if (!Object.keys(handlerAnchors).length && commandSetup.length) {
      handlerAnchors[commandSetup[commandSetup.length - 1].to] = true;
    }

    var mainHandler = [];
    var foundRelation = true;
    while (foundRelation) {
      foundRelation = false;
      transitions.forEach(function (transition) {
        if (assigned[transition.id] || !handlerAnchors[transition.from]) return;
        if (anchors[transition.to] && anchors[transition.to].kind === 'task') return;
        assigned[transition.id] = true;
        mainHandler.push(transition);
        handlerAnchors[transition.to] = true;
        foundRelation = true;
      });
    }

    var other = transitions.filter(function (transition) { return !assigned[transition.id]; });
    var groups = [];
    if (commandSetup.length) groups.push({ title: msg('main.command_setup'), relations: commandSetup });
    if (mainHandler.length) groups.push({ title: msg('main.main_handler_branch'), relations: mainHandler });
    taskGroups.forEach(function (group) { groups.push(group); });
    if (other.length) groups.push({ title: msg('main.other_static_relations'), relations: other });
    return groups;
  }

  function appendProofNamedLocation(parent, label, location, duplicate) {
    if (!location || !location.path || duplicate && proofLocationMatches(location, duplicate)) return;
    var row = el('div', 'rm-proof-target');
    row.appendChild(document.createTextNode(label + ' '));
    row.appendChild(renderProofLocation(location, 'rm-proof-location'));
    parent.appendChild(row);
  }

  function renderProofStaticRelation(transition, anchors) {
    var from = anchors[transition.from];
    var to = anchors[transition.to];
    var row = el('div', 'rm-direction-evidence-item');
    var top = el('div', 'rm-proof-step-top');
    top.appendChild(txt('span', 'rm-proof-symbol', proofAnchorLabel(from, transition.from)));
    var relation = transition.relation || 'related';
    var invocation = transition.invocation || 'unknown';
    top.appendChild(txt('span', 'rm-proof-relation', '→ ' + relation + ' / ' + invocation + ' →'));
    top.appendChild(txt('span', 'rm-proof-symbol', proofAnchorLabel(to, transition.to)));
    row.appendChild(top);

    var semantics = [];
    if (transition.resolution) {
      semantics.push(msg('main.proof.resolution', {
        resolution: transition.resolution,
      }));
    }
    if (transition.certainty) {
      semantics.push(msg('main.proof.evidence_quality', { certainty: transition.certainty }));
    }
    if (semantics.length) row.appendChild(txt('div', 'rm-proof-relation', semantics.join(' · ')));

    appendProofNamedLocation(row, msg('main.proof.evidence'), transition.evidence, null);
    appendProofNamedLocation(row, msg('main.proof.from_declaration'), from && from.location, transition.evidence);
    appendProofNamedLocation(row, msg('main.proof.target_declaration'), to && to.location, transition.evidence);
    if (transition.condition) {
      var condition = el('div', 'rm-proof-target');
      condition.appendChild(document.createTextNode(msg('main.proof.condition', {
        expression: transition.condition.expression || '',
      })));
      if (transition.condition.location && transition.condition.location.path) {
        condition.appendChild(document.createTextNode(msg('main.proof.at')));
        condition.appendChild(renderProofLocation(transition.condition.location, 'rm-proof-location'));
      }
      row.appendChild(condition);
    }
    return row;
  }

  function renderLocalProof(session, compact) {
    if (!session || !session.proof) return null;
    var proof = session.proof;
    var counts = proofStatusCounts(proof);
    var box = el('section', 'rm-flow-proof' + (compact ? ' rm-flow-proof--compact' : ''));
    var header = el('div', 'rm-proof-header');
    header.appendChild(txt('div', 'rm-direction-label', LABELS.verifiedFlow + ' · ' + (
      proof.archetype || msg('main.proof.flow')
    )));
    header.appendChild(txt(
      'div',
      'rm-proof-summary',
      msg('main.proof.trace_summary', {
        quality: proof.trace_quality || msg('main.proof.partial'),
        verified: counts.verified,
        total: (proof.slots || []).length,
      })
    ));
    box.appendChild(header);

    var slots = el('div', 'rm-proof-slots');
    (proof.slots || []).forEach(function (slot) {
      var slotNode = txt('span', 'rm-proof-slot rm-proof-slot--' + (slot.status || 'missing'), slot.kind);
      slotNode.title = slot.summary || slot.missing || slot.status;
      slots.appendChild(slotNode);
    });
    box.appendChild(slots);

    if (!compact) {
      var anchors = proofAnchorMap(proof);
      var groups = proofStaticRelationGroups(proof);
      if (groups.length) {
        box.appendChild(txt('div', 'rm-direction-label rm-proof-path-label', msg('main.chrome.static.relation.groups')));
        box.appendChild(txt('div', 'rm-direction-hint', msg('main.chrome.grouped.by.static.scope.runtime.order.is.not.inferred')));
        groups.forEach(function (group) {
          var groupNode = el('div', 'rm-direction-field');
          groupNode.appendChild(txt('div', 'rm-direction-label', group.title));
          if (group.anchor) {
            appendProofNamedLocation(groupNode, msg('main.proof.task_anchor'), group.anchor.location, null);
          }
          if (!group.relations.length) {
            groupNode.appendChild(txt('div', 'rm-proof-relation', msg('main.chrome.no.static.relations.captured.for.this.task')));
          }
          group.relations.forEach(function (transition) {
            groupNode.appendChild(renderProofStaticRelation(transition, anchors));
          });
          box.appendChild(groupNode);
        });
      }

      var stats = session.stats || {};
      var meta = el('div', 'rm-proof-meta');
      meta.appendChild(txt('span', '', msg('main.count.tasks', { count: stats.tasks_completed || 0 })));
      meta.appendChild(txt('span', '', msg('main.count.evidence_files', { count: (stats.files || []).length })));
      meta.appendChild(txt('span', '', msg('main.count.symbols', { count: (stats.symbols || []).length })));
      if (stats.wall_millis) meta.appendChild(txt('span', '', msg('main.local_analysis_seconds', {
        seconds: (stats.wall_millis / 1000).toFixed(1),
      })));
      box.appendChild(meta);
    }

    if (session.stop && session.stop.reason !== 'complete') {
      var boundary = el('div', 'rm-proof-boundary');
      boundary.appendChild(txt('span', 'rm-proof-boundary-label', LABELS.proofStop + ': '));
      boundary.appendChild(document.createTextNode(session.stop.reason + (session.stop.message ? ' — ' + session.stop.message : '')));
      box.appendChild(boundary);
    }
    if (proof.current_frontier) {
      var frontier = el('div', 'rm-proof-boundary');
      frontier.appendChild(txt('span', 'rm-proof-boundary-label', msg('main.debug.current_frontier', {
        value: '',
      })));
      frontier.appendChild(document.createTextNode(proof.current_frontier));
      box.appendChild(frontier);
    }
    return box;
  }

  function renderPill(text, kind) {
    var pill = el('span', 'rm-pill rm-pill--' + (kind || 'accent'));
    pill.textContent = text;
    return pill;
  }

  function renderFlowTypePill(flow) {
    var flowType = flow && flow.flow_type;
    if (flowType !== 'request' && flowType !== 'operational') return null;
    var label = flowType === 'operational' ? msg('main.operational') : msg('main.request');
    return renderPill(label, flowType);
  }

  function renderKindBadge(kind) {
    if (!kind) return null;
    var badge = el('span', 'rm-kind rm-kind--' + kind);
    badge.textContent = kind;
    return badge;
  }

  function renderFlowCard(flow, isRecommended) {
    var card = el('div', 'rm-ov-flow');
    if (isRecommended) card.classList.add('rm-ov-flow--recommended');

    if (flow.error) {
      card.classList.add('rm-ov-flow--error');
      var eh = el('div', 'rm-ov-flow-header');
      var eh3 = txt('h3', '', flow.name || flow.id);
      eh.appendChild(eh3);
      var errorFlowType = renderFlowTypePill(flow);
      if (errorFlowType) eh.appendChild(errorFlowType);
      eh.appendChild(renderPill(LABELS.errorUnavailable, 'error'));
      card.appendChild(eh);
      if (flow.bundle_summary) {
        var meta = txt('div', 'rm-meta', msg('main.flow.bundle_stats_compact', {
          source: Number(flow.bundle_summary.selected_files_count) || 0,
          test: Number(flow.bundle_summary.selected_tests_count) || 0,
          doc: Number(flow.bundle_summary.selected_docs_count) || 0,
        }));
        card.appendChild(meta);
      }
      card.onclick = function () { showTab('rm-flow-' + flow.id); };
      return card;
    }

    var header = el('div', 'rm-ov-flow-header');
    var h3 = txt('h3', '', flow.name || flow.id);
    header.appendChild(h3);
    var flowType = renderFlowTypePill(flow);
    if (flowType) header.appendChild(flowType);
    if (isRecommended) header.appendChild(renderPill(LABELS.startHere));
    card.appendChild(header);

    if (flow.summary) {
      var truncated = flow.summary.length > 100 ? flow.summary.slice(0, 100) + '...' : flow.summary;
      card.appendChild(linkified('div', 'rm-summary-line', truncated));
    }

    var preview = el('div', 'rm-ov-flow-preview');
    var previewFiles = flow.files_to_read_in_order;
    if ((!previewFiles || previewFiles.length === 0) && flow.bundle_files) {
      previewFiles = flow.bundle_files;
    }
    if (previewFiles && previewFiles.length > 0) {
      previewFiles.slice(0, 3).forEach(function (fi) {
        var p = el('div', 'rm-ov-flow-file');
        p.appendChild(renderFileReference(fi.path, '', 0, fi.path));
        preview.appendChild(p);
      });
    }
    card.appendChild(preview);

    var footer = el('div', 'rm-ov-flow-footer');
    if (!flow.evidence_only) footer.appendChild(renderEvidenceBadge(flow.confidence));
    if (flow.warnings && flow.warnings.length > 0) {
      footer.appendChild(txt('span', 'rm-ov-flow-stat', msg('main.count.warnings', { count: flow.warnings.length })));
    }
    if (flow.tests_to_read && flow.tests_to_read.length > 0) {
      footer.appendChild(txt('span', 'rm-ov-flow-stat', msg('main.count.tests', { count: flow.tests_to_read.length })));
    }
    card.appendChild(footer);

    card.onclick = function () { showTab('rm-flow-' + flow.id); };
    return card;
  }

  function candidateDirections() {
    if (DATA.candidate_directions && DATA.candidate_directions.length > 0) {
      return DATA.candidate_directions;
    }
    if (!DATA.candidate_flows) return [];
    return DATA.candidate_flows.map(function (candidate, index) {
      if (typeof candidate === 'string') {
        return { id: 'candidate-' + index, name: candidate };
      }
      return candidate;
    });
  }

  function architectureSourceLabel(value) {
    return {
      validated_model: msg('architecture.value.validated_model'),
      partial_model: msg('architecture.value.partial_model'),
      normalized_model: msg('architecture.value.normalized_model'),
      local_anchors: msg('architecture.value.local_anchors'),
      package_fallback: msg('architecture.value.package_fallback')
    }[value] || msg('architecture.value.unspecified');
  }

  function renderDirectionField(label, value, code) {
    if (!value) return null;
    var row = el('div', 'rm-direction-field');
    row.appendChild(txt('div', 'rm-direction-label', label));
    var body = el('div', code ? 'rm-direction-code' : 'rm-direction-value');
    if (code) appendLinkifiedText(body, value);
    else body.textContent = value;
    row.appendChild(body);
    return row;
  }

  function flowByID(id) {
    if (!id || !DATA.flows) return null;
    for (var i = 0; i < DATA.flows.length; i++) {
      if (DATA.flows[i].id === id) return DATA.flows[i];
    }
    return null;
  }

  function directionByID(id) {
    if (!id) return null;
    var directions = candidateDirections();
    for (var i = 0; i < directions.length; i++) {
      if (directions[i].id === id) return directions[i];
    }
    return null;
  }

  function renderDirectionContext(direction) {
    if (!direction) return null;

    var context = el('section', 'rm-direction-context');
    if (direction.why_interesting) {
      context.appendChild(linkified('p', 'rm-direction-purpose', direction.why_interesting));
    }

    var facts = el('div', 'rm-direction-context-facts');
    var trigger = renderDirectionField(LABELS.trigger, direction.trigger, false);
    if (trigger) facts.appendChild(trigger);
    var entrypoint = renderDirectionField(LABELS.likelyEntrypoint, direction.likely_entrypoint, true);
    if (entrypoint) facts.appendChild(entrypoint);
    if (facts.children.length > 0) context.appendChild(facts);

    var verification = renderLocalVerification(direction.local_verification);
    if (verification) context.appendChild(verification);

    var proof = renderLocalProof(direction.local_proof, false);
    if (proof) context.appendChild(proof);

    if (direction.evidence && direction.evidence.length > 0) {
      var evidence = el('div', 'rm-direction-context-evidence');
      evidence.appendChild(txt('div', 'rm-direction-label', LABELS.orientationEvidence));
      direction.evidence.forEach(function (statement) {
        var item = el('div', 'rm-direction-evidence-item');
        appendLinkifiedText(item, statement);
        evidence.appendChild(item);
      });
      context.appendChild(evidence);
    }

    return context;
  }

  function renderCandidateDirectionCard(direction, isSuggestedStart) {
    var card = el('div', 'rm-ov-flow rm-candidate-direction');

    var header = el('div', 'rm-ov-flow-header');
    header.appendChild(txt('h3', '', direction.name || direction.id));
    var directionFlowType = renderFlowTypePill(direction);
    if (directionFlowType) header.appendChild(directionFlowType);
    if (isSuggestedStart) header.appendChild(renderPill(LABELS.suggestedStart));
    if (direction.disposition === 'rejected') header.appendChild(renderPill(msg('main.chrome.not.used.as.a.flow')));
    header.appendChild(renderEvidenceBadge(direction.confidence, direction.local_verification));
    card.appendChild(header);

    if (direction.why_interesting) {
      card.appendChild(linkified('div', 'rm-summary-line', direction.why_interesting));
    }
    if (direction.disposition === 'rejected' && direction.disposition_reason) {
      card.appendChild(txt('div', 'rm-direction-hint', direction.disposition_reason));
    }

    var trigger = renderDirectionField(LABELS.trigger, direction.trigger, false);
    if (trigger) card.appendChild(trigger);

    var entrypoint = renderDirectionField(LABELS.likelyEntrypoint, direction.likely_entrypoint, true);
    if (entrypoint) card.appendChild(entrypoint);

    var verification = renderLocalVerification(direction.local_verification);
    if (verification) card.appendChild(verification);

    var proof = renderLocalProof(direction.local_proof, true);
    if (proof) card.appendChild(proof);

    if (direction.likely_files && direction.likely_files.length > 0) {
      var files = el('div', 'rm-direction-field');
      files.appendChild(txt('div', 'rm-direction-label', LABELS.likelyFiles));
      direction.likely_files.forEach(function (path) {
        var file = el('div', 'rm-direction-code');
        file.appendChild(renderFileReference(path, '', 0, path));
        files.appendChild(file);
      });
      card.appendChild(files);
    }

    if (direction.evidence && direction.evidence.length > 0) {
      var evidence = el('div', 'rm-direction-evidence');
      evidence.appendChild(txt('div', 'rm-direction-label', LABELS.orientationEvidence));
      direction.evidence.forEach(function (statement) {
        var item = el('div', 'rm-direction-evidence-item');
        appendLinkifiedText(item, statement);
        evidence.appendChild(item);
      });
      card.appendChild(evidence);
    }

    var focused = flowByID(direction.id);
    if (focused && direction.disposition !== 'rejected') {
      card.classList.add('rm-candidate-direction--clickable');
      card.setAttribute('role', 'button');
      card.setAttribute('tabindex', '0');
      card.appendChild(txt('div', 'rm-direction-action', LABELS.openLocalEvidence));
      var openFocused = function () { showTab('rm-flow-' + focused.id); };
      card.onclick = openFocused;
      card.onkeydown = function (event) {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault();
          openFocused();
        }
      };
    }

    return card;
  }

  function appendOrientationEvidence(container, evidence) {
    if (!evidence || evidence.length === 0) return;

    var evidenceBlock = el('div', 'rm-direction-evidence');
    evidenceBlock.appendChild(txt('div', 'rm-direction-label', LABELS.orientationEvidence));
    evidence.forEach(function (statement) {
      var item = el('div', 'rm-direction-evidence-item');
      appendLinkifiedText(item, statement);
      evidenceBlock.appendChild(item);
    });
    container.appendChild(evidenceBlock);
  }

  function renderPurposeCard() {
    var card = el('div', 'rm-card');
    var header = el('div', 'rm-flow-header');
    header.appendChild(txt('h2', '', LABELS.purpose));
    if (DATA.project_guess) {
      header.appendChild(renderEvidenceBadge(DATA.orientation_confidence));
    }
    card.appendChild(header);
    card.appendChild(txt(
      'div',
      DATA.project_guess ? 'rm-summary' : 'rm-placeholder',
      DATA.project_guess || msg('main.no_project_purpose')
    ));

    if (DATA.run) {
      var facts = el('div', 'rm-run-facts');
      var addFact = function (label, value) {
        if (!value) return;
        var fact = el('div', 'rm-run-fact');
        fact.appendChild(txt('span', 'rm-run-fact-label', label));
        fact.appendChild(txt('span', 'rm-run-fact-value', value));
        facts.appendChild(fact);
      };
      addFact(LABELS.model, DATA.run.model);
      addFact(LABELS.localContext, formatBytes(DATA.run.compact_context_bytes));
      addFact(LABELS.externalRequest, formatBytes(DATA.run.external_request_bytes));
      if (DATA.run.provider_request_count) {
        addFact(LABELS.providerRequests, String(DATA.run.provider_request_count));
      }
      if (DATA.run.provider_latency_ms !== undefined && DATA.run.provider_latency_ms !== null) {
        addFact(LABELS.providerLatency, DATA.run.provider_latency_ms + ' ms');
      }
      addFact(LABELS.directionsFound, String(DATA.run.suggested_investigation_count || 0));
      addFact(LABELS.rejectedDirections, String(DATA.run.rejected_direction_count || 0));
      var surfaceTotal = String(DATA.run.discovered_surface_count || 0);
      var surfaceBreakdown = [];
      if (DATA.run.application_surface_count) surfaceBreakdown.push(msg('main.count.application_surfaces', { count: DATA.run.application_surface_count }));
      if (DATA.run.secondary_service_surface_count) surfaceBreakdown.push(msg('main.count.secondary_service_surfaces', { count: DATA.run.secondary_service_surface_count }));
      if (DATA.run.tooling_surface_count) surfaceBreakdown.push(msg('main.count.tooling_surfaces', { count: DATA.run.tooling_surface_count }));
      if (DATA.run.test_helper_surface_count) surfaceBreakdown.push(msg('main.count.test_helper_surfaces', { count: DATA.run.test_helper_surface_count }));
      if (DATA.run.unavailable_surface_count) surfaceBreakdown.push(msg('main.count.unavailable_surfaces', { count: DATA.run.unavailable_surface_count }));
      if (DATA.run.supporting_dependency_surface_count) surfaceBreakdown.push(msg('main.count.supporting_dependency_surfaces', { count: DATA.run.supporting_dependency_surface_count }));
      if (DATA.run.dependency_only_surface_count) surfaceBreakdown.push(msg('main.count.dependency_only_surfaces', { count: DATA.run.dependency_only_surface_count }));
      if (DATA.run.unassigned_surface_count) surfaceBreakdown.push(msg('main.count.unassigned_surfaces', { count: DATA.run.unassigned_surface_count }));
      addFact(msg('main.discovered_surfaces'), surfaceTotal + (surfaceBreakdown.length ? ' · ' + surfaceBreakdown.join(' · ') : ''));
      addFact(LABELS.savedFlows, String(DATA.run.saved_trace_count || 0));
      addFact(msg('main.evidence_bundles'), String(DATA.run.evidence_bundle_count || 0));
      addFact(msg('main.complete_traces'), String(DATA.run.complete_trace_count || 0));
      addFact(msg('main.partial_traces'), String(DATA.run.partial_trace_count || 0));
      addFact(msg('main.unresolved_traces'), String(DATA.run.unresolved_trace_count || 0));
      addFact(msg('main.failed_trace_attempts'), String(DATA.run.failed_trace_attempt_count || 0));
      if (DATA.run.surface_discovery_ran) {
        var surfaceValue = formatMillis(DATA.run.surface_discovery_ms);
        surfaceValue += ' · ' + msg('main.count.found', { count: DATA.run.surface_discovery_count || 0 });
        addFact(LABELS.surfaceAnalysis, surfaceValue);
      }
      if (DATA.freshness) {
        var freshnessValue = String(DATA.freshness.state || 'unavailable');
        if ((DATA.freshness.affected_paths || []).length > 0) {
          freshnessValue += ' · ' + msg('main.count.analyzed_inputs_changed', { count: DATA.freshness.affected_paths.length });
        } else if ((DATA.freshness.affected_submodules || []).length > 0) {
          freshnessValue += ' · ' + msg('main.count.excluded_submodule_changes', { count: DATA.freshness.affected_submodules.length });
        }
        addFact(LABELS.snapshotFreshness, freshnessValue);
      }
      addFact(LABELS.architectureAnchors, msg('main.count.static_families', { count: DATA.run.architecture_anchor_count || 0 }));
      if (DEBUG_MODE && DATA.architecture_synthesis) {
        var architectureState = DATA.architecture_synthesis.state || 'unknown';
        var architectureValue = msg('main.unavailable');
        if (DATA.architecture_synthesis.proposal_partial) {
          architectureValue = msg('main.partial_model');
        } else if (DATA.architecture_synthesis.proposal_normalized) {
          architectureValue = msg('main.normalized_model');
        } else if (DATA.architecture_synthesis.proposal_accepted) {
          architectureValue = msg('main.validated_model');
        } else if (DATA.architecture_synthesis.proposal_rejected && DATA.architecture_synthesis.fallback_selected) {
          architectureValue = msg('main.proposal_rejected_local_fallback');
        } else if (architectureState === 'cached') {
          architectureValue = msg('main.cached_outcome_unavailable');
        } else if (architectureState === 'succeeded') {
          architectureValue = msg('main.completed_outcome_unavailable');
        }
        if (architectureState === 'cached' && DATA.architecture_synthesis.proposal_accepted) {
          architectureValue += ' · ' + msg('main.cached_response');
        }
        if (DATA.architecture_synthesis.latency_ms) {
          architectureValue += ' · ' + formatMillis(DATA.architecture_synthesis.latency_ms);
        }
        addFact(LABELS.architectureGrouping, architectureValue);
      }
      if (facts.children.length > 0) card.appendChild(facts);
    }

    var research = renderModelResearchDetails(DATA.model_research);
    if (research) card.appendChild(research);

    var warnings = renderRunWarnings(DATA.warnings);
    if (warnings) card.appendChild(warnings);
    return card;
  }

  function semanticArtifactGroup(kind) {
    if (kind === 'contribution_guide') return msg('main.semantic_group.contribute');
    if (kind === 'dependency_usage' || kind === 'repository_pattern' || kind === 'go_learning') {
      return msg('main.semantic_group.learn_from_code');
    }
    return msg('main.semantic_group.understand');
  }

  function semanticArtifactKindLabel(kind) {
    return {
      repository_story: msg('main.semantic_kind.repository_story'),
      mechanism: msg('main.semantic_kind.mechanism'),
      dependency_usage: msg('main.semantic_kind.dependency_usage'),
      repository_pattern: msg('main.semantic_kind.repository_pattern'),
      contribution_guide: msg('main.semantic_kind.contribution_guide'),
      go_learning: msg('main.semantic_kind.go_learning'),
    }[kind] || String(kind || msg('main.chrome.explanation'));
  }

  function semanticArtifactBasisCounts(artifact) {
    var counts = { direct: 0, compositional: 0, interpretive: 0, unresolved: 0 };
    (artifact && artifact.statements || []).forEach(function (statement) {
      var basis = statement && statement.basis;
      if (Object.prototype.hasOwnProperty.call(counts, basis)) counts[basis]++;
    });
    counts.unresolved += (artifact && artifact.unknowns || []).length;
    return counts;
  }

  function renderSemanticArtifactCard(artifact) {
    var button = el('button', 'rm-explore-artifact');
    button.type = 'button';
    button.setAttribute('data-semantic-artifact-id', artifact.id || '');
    button.appendChild(txt('span', 'rm-explore-artifact__kind', semanticArtifactKindLabel(artifact.kind)));
    button.appendChild(txt('strong', 'rm-explore-artifact__title', artifact.title || artifact.question || msg('main.chrome.repository.explanation')));
    if (artifact.question && artifact.question !== artifact.title) {
      button.appendChild(txt('span', 'rm-explore-artifact__question', artifact.question));
    }
    if (artifact.summary) button.appendChild(txt('span', 'rm-explore-artifact__summary', artifact.summary));

    var counts = semanticArtifactBasisCounts(artifact);
    var meta = el('span', 'rm-explore-artifact__meta');
    var supported = counts.direct + counts.compositional;
    var requiredAspects = artifact.required_answer_aspects || [];
    var coveredAspects = artifact.covered_answer_aspects || [];
    if (supported) meta.appendChild(txt('span', 'is-supported', msg('main.count.evidence_linked', { count: supported })));
    if (requiredAspects.length) {
      meta.appendChild(txt(
        'span',
        'rm-explore-artifact__coverage',
        msg('main.covered_aspects', { covered: coveredAspects.length, required: requiredAspects.length })
      ));
    }
    if (counts.interpretive) meta.appendChild(txt('span', 'is-interpretive', msg('main.count.interpretive', {
      count: counts.interpretive,
    })));
    if (counts.unresolved || artifact.verdict === 'insufficient_evidence') {
      meta.appendChild(txt(
        'span',
        'is-unresolved',
        msg('main.count.known_gaps', { count: counts.unresolved || 1 })
      ));
    }
    if (artifact.verdict) {
      meta.appendChild(txt('span', 'rm-explore-artifact__verdict', String(artifact.verdict)));
    }
    if (meta.childElementCount) button.appendChild(meta);
    button.appendChild(txt('span', 'rm-explore-artifact__action', msg('main.chrome.open.explanation.on.the.map')));
    button.addEventListener('click', function () {
      openReportTarget({ kind: 'semantic_artifact', artifact_id: artifact.id });
    });
    return button;
  }

  function renderSemanticCoverageSummary(coverage) {
    if (!coverage || typeof coverage !== 'object') return null;
    var summary = el('div', 'rm-semantic-coverage');
    summary.appendChild(txt('strong', 'rm-semantic-coverage__title', msg('main.chrome.publication.coverage')));
    var items = el('div', 'rm-semantic-coverage__items');
    items.appendChild(txt('span', '', msg('main.coverage.opportunities_attempted', {
      count: String(coverage.opportunities_attempted || 0),
    })));
    items.appendChild(txt('span', '', msg('main.coverage.candidates_investigated', {
      count: String(coverage.candidates_investigated || 0),
    })));
    items.appendChild(txt('span', '', msg('main.coverage.mechanisms_published', {
      count: String(coverage.canonical_mechanisms_published || 0),
    })));
    if (coverage.central_routing_mechanism) {
      items.appendChild(txt(
        'span',
        'rm-semantic-coverage__routing is-' + String(coverage.central_routing_mechanism),
        msg('main.central_routing_mechanism', { value: String(coverage.central_routing_mechanism) })
      ));
    }
    summary.appendChild(items);
    return summary;
  }

  function renderExploreCard(artifacts) {
    artifacts = (artifacts || []).filter(function (artifact) {
      return artifact && artifact.id && (artifact.title || artifact.question);
    });
    if (!artifacts.length) return null;

    var card = el('section', 'rm-card rm-explore');
    card.appendChild(txt('h2', 'rm-explore__title', LABELS.explore));
    card.appendChild(txt(
      'p',
      'rm-explore__intro',
      msg('main.evidence_linked_explanations')
    ));
    var coverage = renderSemanticCoverageSummary(DATA.semantic_coverage);
    if (coverage) card.appendChild(coverage);
    var order = [
      msg('main.semantic_group.understand'),
      msg('main.semantic_group.learn_from_code'),
      msg('main.semantic_group.contribute'),
    ];
    order.forEach(function (groupName) {
      var groupArtifacts = artifacts.filter(function (artifact) {
        return semanticArtifactGroup(artifact.kind) === groupName;
      });
      if (!groupArtifacts.length) return;
      var group = el('section', 'rm-explore__group');
      group.appendChild(txt('h3', 'rm-explore__group-title', groupName));
      var grid = el('div', 'rm-explore__grid');
      groupArtifacts.forEach(function (artifact) {
        grid.appendChild(renderSemanticArtifactCard(artifact));
      });
      group.appendChild(grid);
      card.appendChild(group);
    });
    return card;
  }

  function renderModelResearchDetails(research) {
    if (!research || !research.policy) return null;
    var details = el('details', 'rm-analysis-details rm-model-research');
    details.appendChild(txt('summary', '', msg('main.chrome.model.research')));

    var policy = research.policy;
    var usage = research.usage || {};
    var overview = el('div', 'rm-model-research-overview');
    overview.appendChild(txt('div', '', msg('main.research.provider_calls', {
      used: String(usage.semantic_calls || 0),
      limit: String(policy.max_semantic_calls || 0),
    })));
    overview.appendChild(txt('div', '', msg('main.research.external_request_bytes', {
      used: formatBytes(usage.request_bytes || 0),
      limit: formatBytes(policy.max_total_request_bytes || 0),
    })));
    details.appendChild(overview);

    var stages = el('div', 'rm-model-research-stages');
    appendResearchStage(stages, msg('main.orientation'), research.orientation);
    (research.targeted_rounds || []).forEach(function (round, index) {
      appendResearchRound(stages, msg('main.targeted_research_round', { round: String(index + 1) }), round);
    });
    (research.skipped_targeted_rounds || []).forEach(function (round, index) {
      appendResearchRound(stages, msg('main.skipped_targeted_round', { round: String(index + 1) }), round);
    });
	appendResearchStage(stages, msg('main.architecture_synthesis'), research.architecture_synthesis);
	appendResearchStage(stages, msg('main.guided_tour_editor'), research.guided_tour);
    details.appendChild(stages);

    var coverage = research.coverage || {};
    var coverageList = el('div', 'rm-model-research-coverage');
    coverageList.appendChild(txt('div', '', msg('main.research.local_authorized_files', {
      count: String(coverage.local_authorized_files || 0),
    })));
    coverageList.appendChild(txt('div', '', msg('main.research.initial_model_summaries', {
      count: String(coverage.initial_model_summaries || 0),
    })));
    coverageList.appendChild(txt('div', '', msg('main.research.focused_local_evidence', {
      count: String(coverage.focused_local_evidence_inspected || 0),
    })));
    coverageList.appendChild(txt('div', '', msg('main.research.targeted_model_windows', {
      count: String(coverage.targeted_model_evidence_windows || 0),
    })));
    details.appendChild(coverageList);
    return details;
  }

  function appendResearchStage(container, label, stage) {
    if (!stage || !stage.status) return;
    var row = el('div', 'rm-model-research-stage');
    var status = String(stage.status);
    if (stage.cache_hit) status += ' · ' + msg('main.research.cached');
    if (stage.request_bytes) status += ' · ' + formatBytes(stage.request_bytes);
    row.appendChild(txt('strong', '', label));
    row.appendChild(txt('span', '', status));
    container.appendChild(row);
  }

  function researchSelectionReasonLabel(reason) {
    var code = String(reason || '');
    var messageIDs = {
      planned: 'main.research.selection_reason.planned',
      runtime_only_frontier: 'main.research.selection_reason.runtime_only_frontier',
      unknown_candidate_ids: 'main.research.selection_reason.unknown_candidate_ids',
      no_code_bearing_bounded_window: 'main.research.selection_reason.no_code_bearing_bounded_window',
      no_new_exact_evidence: 'main.research.selection_reason.no_new_exact_evidence',
      no_bounded_local_evidence: 'main.research.selection_reason.no_bounded_local_evidence',
      new_exact_evidence_and_high_value_frontier: 'main.research.selection_reason.new_exact_evidence_and_high_value_frontier',
      targeted_round_limit: 'main.research.selection_reason.targeted_round_limit',
    };
    return messageIDs[code] ? msg(messageIDs[code]) : code;
  }

  function appendResearchRound(container, label, round) {
    if (!round) return;
    var row = el('div', 'rm-model-research-round');
    var heading = el('div', 'rm-model-research-stage');
    heading.appendChild(txt('strong', '', label));
    var status = round.status
      ? String(round.status)
      : msg('main.research.unknown_status');
    if (round.cached) status += ' · ' + msg('main.research.cached');
    heading.appendChild(txt('span', '', status));
    row.appendChild(heading);
    if (round.question) row.appendChild(txt('div', '', round.question));
    if (round.selection_reason) {
      row.appendChild(txt('div', 'rm-muted', msg('main.research.why', {
        reason: researchSelectionReasonLabel(round.selection_reason),
      })));
    }
    row.appendChild(txt('div', 'rm-muted', msg('main.research.exact_evidence', {
      evidence: String((round.input_evidence_ids || []).length),
      facts: String(round.new_grounded_facts_count || 0),
    })));
    if ((round.rejected_findings || []).length) {
      row.appendChild(txt('div', 'rm-muted', msg('main.research.rejected_claims', {
        count: String(round.rejected_findings.length),
      })));
    }
    if ((round.unresolved_frontiers || []).length) {
      row.appendChild(txt('div', 'rm-muted', msg('main.research.frontier', {
        value: String(round.unresolved_frontiers[0].question || 'unresolved'),
      })));
    } else if (round.stop_reason) {
      row.appendChild(txt('div', 'rm-muted', msg('main.research.result', {
        result: String(round.stop_reason),
      })));
    }
    container.appendChild(row);
  }

  function componentReferences(statements) {
    var seen = {};
    var references = [];
    (statements || []).forEach(function (statement) {
      OPENABLE_PATHS.forEach(function (filePath) {
        var cursor = 0;
        while (cursor < statement.length) {
          var index = statement.indexOf(filePath, cursor);
          if (index < 0) break;
          var before = index > 0 ? statement[index - 1] : '';
          var afterIndex = index + filePath.length;
          var after = afterIndex < statement.length ? statement[afterIndex] : '';
          var pathChar = /[A-Za-z0-9_./-]/;
          if ((!before || !pathChar.test(before)) && (!after || !pathChar.test(after))) {
            var suffix = statement.slice(afterIndex).match(/^:(\d+)/);
            var line = suffix ? parseInt(suffix[1], 10) || 0 : 0;
            var key = filePath + ':' + line;
            if (!seen[key]) {
              seen[key] = true;
              references.push({ path: filePath, line: line, statement: statement });
            }
          }
          cursor = afterIndex;
        }
      });
    });
    return references;
  }

  function directionPaths(direction) {
    var paths = [];
    if (direction.likely_entrypoint) paths.push(direction.likely_entrypoint);
    (direction.likely_files || []).forEach(function (path) { paths.push(path); });
    componentReferences(direction.evidence || []).forEach(function (ref) { paths.push(ref.path); });
    return paths;
  }

  function packageForFile(filePath) {
    var graph = DATA.repository_graph || {};
    var slash = filePath.lastIndexOf('/');
    var fileDir = slash < 0 ? '' : filePath.slice(0, slash);
    var packages = graph.packages || [];
    if (packages.length > 0) {
      for (var packageIndex = 0; packageIndex < packages.length; packageIndex++) {
        if ((packages[packageIndex].package_directory || '') === fileDir) {
          return packages[packageIndex].canonical_package_path || '';
        }
      }
      return '';
    }
    var modules = graph.modules || [];
    var best = null;
    modules.forEach(function (module) {
      var moduleDir = module.dir || '';
      var matches = !moduleDir || fileDir === moduleDir || fileDir.indexOf(moduleDir + '/') === 0;
      if (matches && (!best || moduleDir.length > (best.dir || '').length)) best = module;
    });
    if (!best || !best.path) return '';
    var relativeDir = fileDir;
    if (best.dir) {
      relativeDir = fileDir === best.dir ? '' : fileDir.slice(best.dir.length + 1);
    }
    return best.path + (relativeDir ? '/' + relativeDir : '');
  }

  function groupComponentFiles(references) {
    var groups = [];
    var byPath = {};
    var add = function (path, line, statement) {
      if (!path || !OPENABLE_PATH_SET[path]) return;
      var group = byPath[path];
      if (!group) {
        group = { path: path, lines: [], statements: [] };
        byPath[path] = group;
        groups.push(group);
      }
      if (line && group.lines.indexOf(line) < 0) group.lines.push(line);
      if (statement && group.statements.indexOf(statement) < 0) group.statements.push(statement);
    };
    references.forEach(function (ref) { add(ref.path, ref.line, ref.statement); });
    groups.forEach(function (group) { group.lines.sort(function (a, b) { return a - b; }); });
    return groups;
  }

  function componentFileLinks(group) {
    if (!group.lines || group.lines.length === 0) {
      return [{ path: group.path, line: 0, label: group.path }];
    }
    return group.lines.map(function (line) {
      return { path: group.path, line: line, label: group.path + ':' + line };
    });
  }

  function boundedText(value, maxLength) {
    if (typeof value !== 'string') return '';
    value = value.trim();
    if (value.length <= maxLength) return value;
    return value.slice(0, Math.max(0, maxLength - 1)) + '…';
  }

  function exactSymbolDetail(value) {
    return typeof value === 'string' ? value : '';
  }

  function symbolLookupLine(anchor) {
    if (!anchor.lines || anchor.lines.length === 0) return 0;
    return anchor.lines[0] || 0;
  }

  function symbolLookupKey(component, anchor, line) {
    return component.id + ':' + anchor.id + ':' + line;
  }

  function symbolLookupElementID(key) {
    return 'rm-symbol-results-' + key.replace(/[^A-Za-z0-9_-]/g, '-');
  }

  function renderEntityLocation(entity, cls) {
    if (!entity || typeof entity.path !== 'string') return txt('code', cls, msg('main.unknown_location'));
    var line = Number.isInteger(entity.line) && entity.line > 0 ? entity.line : 0;
    var label = boundedText(entity.path, 320) + (line ? ':' + line : '');
    return renderFileReference(entity.path, cls, line, label);
  }

  function renderSymbolCandidate(candidate, key, state) {
    var row = el('div', 'rm-symbol-candidate');
    row.setAttribute('role', 'listitem');
    var inspection = state.inspection || { status: 'idle' };
    var selected = inspection.candidateID === candidate.id;
    if (selected) row.classList.add('rm-symbol-candidate--selected');

    var heading = el('div', 'rm-symbol-candidate-heading');
    heading.appendChild(txt('code', 'rm-symbol-candidate-name', boundedText(candidate.name, 160) || msg('main.chrome.unnamed.go.symbol')));
    var kind = exactSymbolDetail(candidate.kind);
    if (kind) heading.appendChild(txt('span', 'rm-symbol-candidate-kind', kind));
    row.appendChild(heading);

    var path = boundedText(candidate.path, 320) || msg('main.unknown_location');
    var line = Number.isInteger(candidate.line) && candidate.line > 0 ? candidate.line : 0;
    row.appendChild(txt('code', 'rm-symbol-candidate-location', path + (line ? ':' + line : '')));

    var details = [];
    var match = exactSymbolDetail(candidate.match);
    var certainty = exactSymbolDetail(candidate.certainty);
    if (match) details.push(match);
    if (certainty) details.push(certainty);
    if (details.length > 0) {
      row.appendChild(txt('div', 'rm-symbol-candidate-meta', details.join(' · ')));
    }

    var reasonCount = Array.isArray(candidate.rank_reasons) ? candidate.rank_reasons.length : 0;
    if (reasonCount > 0) {
      row.appendChild(txt('div', 'rm-symbol-candidate-reasons', msg('main.symbol.ranked_by', {
        count: reasonCount,
      })));
    }
    if (selected && state.persisted) {
      row.appendChild(txt('div', 'rm-investigation-saved-label', msg('main.chrome.saved.selection')));
      return row;
    }
    var inspect = txt('button', 'rm-symbol-inspect-button', selected && inspection.status === 'loading' ? msg('main.chrome.inspecting') : msg('main.chrome.inspect.symbol'));
    inspect.type = 'button';
    inspect.disabled = inspection.status === 'loading' || !state.candidateSetID || typeof candidate.id !== 'string' || !candidate.id;
    inspect.setAttribute('aria-pressed', selected && inspection.status === 'ready' ? 'true' : 'false');
    inspect.setAttribute('aria-controls', symbolInspectionElementID(key));
    inspect.onclick = function () {
      requestSymbolInspection(key, candidate);
    };
    row.appendChild(inspect);
    return row;
  }

  function symbolInspectionElementID(key) {
    return 'rm-symbol-inspection-' + key.replace(/[^A-Za-z0-9_-]/g, '-');
  }

  function renderStaticCallList(title, calls, omitted) {
    var section = el('section', 'rm-symbol-call-section');
    var label = msg('main.static_calls_shown', { title: title, count: calls.length });
    if (omitted > 0) label += ' · ' + msg('main.additional_returned_calls_omitted', { count: omitted });
    section.appendChild(txt('div', 'rm-symbol-detail-label', label));
    if (calls.length === 0) {
      section.appendChild(txt('div', 'rm-symbol-detail-empty', msg('main.chrome.no.calls.were.returned.within.this.bounded.static.view.this.does.not.prove.that.none.exist')));
      return section;
    }
    var list = el('div', 'rm-symbol-call-list');
    list.setAttribute('role', 'list');
    calls.slice(0, maxStaticCalls).forEach(function (call) {
      var item = el('div', 'rm-symbol-call');
      item.setAttribute('role', 'listitem');
      var symbol = call && call.symbol ? call.symbol : {};
      item.appendChild(txt('code', 'rm-symbol-call-name', boundedText(symbol.name, 160) || msg('main.chrome.unnamed.go.symbol')));
      item.appendChild(renderEntityLocation(symbol, 'rm-symbol-call-location'));
      if (call.callsite && call.callsite.path) {
        var callsite = el('div', 'rm-symbol-callsite');
        callsite.appendChild(document.createTextNode(msg('main.callsite_prefix')));
        callsite.appendChild(renderEntityLocation(call.callsite, 'rm-symbol-callsite-location'));
        item.appendChild(callsite);
      }
      var certainty = exactSymbolDetail(call.certainty);
      if (certainty) item.appendChild(txt('div', 'rm-symbol-candidate-meta', msg('main.evidence_suffix', {
        certainty: certainty,
      })));
      list.appendChild(item);
    });
    section.appendChild(list);
    return section;
  }

  function renderSourceWindow(source) {
    var section = el('section', 'rm-symbol-source');
    section.appendChild(txt('div', 'rm-symbol-detail-label', msg('main.chrome.bounded.source')));
    if (!source || !Array.isArray(source.lines) || source.lines.length === 0) {
      section.appendChild(txt('div', 'rm-symbol-detail-empty', msg('main.chrome.no.source.lines.returned')));
      return section;
    }
    if (source.path) {
      section.appendChild(renderEntityLocation({ path: source.path, line: source.start_line }, 'rm-symbol-source-location'));
    }
    var summary = [];
    if (source.start_line > 0 && source.end_line >= source.start_line) {
      summary.push(msg('main.source.line_range', {
        start: source.start_line,
        end: source.end_line,
      }));
    }
    if (source.stop_reason) {
      summary.push(msg('main.source.stop_reason', {
        reason: exactSymbolDetail(source.stop_reason),
      }));
    }
    if (source.truncated) summary.push(msg('main.source.truncated'));
    if (summary.length > 0) section.appendChild(txt('div', 'rm-symbol-source-summary', summary.join(' · ')));

    var lines = el('div', 'rm-symbol-source-lines');
    lines.setAttribute('aria-label', msg('main.chrome.bounded.source.lines'));
    source.lines.slice(0, maxSourceLines).forEach(function (sourceLine) {
      var row = el('div', 'rm-symbol-source-line');
      var number = Number.isInteger(sourceLine.line) && sourceLine.line > 0 ? String(sourceLine.line) : '·';
      row.appendChild(txt('span', 'rm-symbol-source-number', number));
      var code = el('code', 'rm-symbol-source-text');
      code.textContent = typeof sourceLine.text === 'string' ? sourceLine.text : '';
      row.appendChild(code);
      if (sourceLine.truncated) row.appendChild(txt('span', 'rm-symbol-source-line-note', msg('main.line_truncated')));
      lines.appendChild(row);
    });
    section.appendChild(lines);
    return section;
  }

	function renderNeighborhoodEntity(entity, location, basis) {
	  var card = el('div', 'rm-symbol-neighborhood-node');
	  card.appendChild(txt('code', 'rm-symbol-neighborhood-name', boundedText(entity && entity.name, 120) || msg('main.chrome.unnamed.go.symbol')));
	  card.appendChild(renderEntityLocation(location || entity || {}, 'rm-symbol-neighborhood-location'));
	  card.appendChild(txt('span', 'rm-symbol-neighborhood-basis', basis));
	  return card;
	}

	function renderNeighborhoodSide(label, calls, omitted) {
	  var side = el('div', 'rm-symbol-neighborhood-side');
	  side.appendChild(txt('div', 'rm-symbol-neighborhood-label', label));
	  if (calls.length === 0) {
			side.appendChild(txt('div', 'rm-symbol-neighborhood-empty', msg('main.none_returned_bounded_view')));
	  } else {
		calls.slice(0, 2).forEach(function (call) {
		  var symbol = call && call.symbol ? call.symbol : {};
			  side.appendChild(renderNeighborhoodEntity(symbol, call.callsite || symbol, msg('main.static_active_build')));
		});
	  }
	  var beyond = Math.max(0, calls.length - 2) + omitted;
	  if (beyond > 0) {
      side.appendChild(txt('div', 'rm-symbol-neighborhood-frontier', msg('main.symbol.outside_focused_view', {
        count: beyond,
      })));
    }
	  return side;
	}

	function renderSymbolNeighborhood(target, incoming, outgoing, incomingOmitted, outgoingOmitted) {
	  var section = el('section', 'rm-symbol-neighborhood');
	  section.appendChild(txt('div', 'rm-symbol-detail-label', msg('main.chrome.focused.static.neighborhood')));
	  section.appendChild(txt('p', 'rm-symbol-neighborhood-caption', msg('main.chrome.a.navigation.projection.for.the.active.build.not.observed.runtime.order')));
	  var graph = el('div', 'rm-symbol-neighborhood-graph');
	  graph.appendChild(renderNeighborhoodSide(msg('main.arrives_from'), incoming, incomingOmitted));
	  graph.appendChild(txt('div', 'rm-symbol-neighborhood-arrow', '→'));
	  var center = el('div', 'rm-symbol-neighborhood-center');
	  center.appendChild(txt('div', 'rm-symbol-neighborhood-label', msg('main.chrome.selected.symbol')));
	  center.appendChild(renderNeighborhoodEntity(target, target, msg('main.static_active_build')));
	  graph.appendChild(center);
	  graph.appendChild(txt('div', 'rm-symbol-neighborhood-arrow', '→'));
	  graph.appendChild(renderNeighborhoodSide(msg('main.calls_next'), outgoing, outgoingOmitted));
	  section.appendChild(graph);
	  return section;
	}

  function renderInspectionTruncation(truncated) {
    if (!truncated || typeof truncated !== 'object' || Array.isArray(truncated)) return null;
    var items = Object.keys(truncated).sort().filter(function (key) {
      return key !== 'incoming_calls' && key !== 'outgoing_calls' &&
        Number.isInteger(truncated[key]) && truncated[key] > 0;
    }).slice(0, 5).map(function (key) {
      return exactSymbolDetail(key) + ' +' + truncated[key];
    });
    if (items.length === 0) return null;
    return txt('div', 'rm-symbol-truncation', msg('main.symbol.evidence_omitted', {
      items: items.join(' · '),
    }));
  }

  function renderTestReferences(response) {
    var section = el('section', 'rm-investigation-tests');
    section.appendChild(txt('div', 'rm-symbol-detail-label', msg('main.chrome.related.test.references')));
    var references = Array.isArray(response.test_references) ? response.test_references.slice(0, maxTestReferences) : [];
    if (references.length === 0) {
      section.appendChild(txt('div', 'rm-symbol-detail-empty', msg('main.chrome.no.test.go.references.were.found.in.the.active.build.this.does.not.prove.that.no.relevant.tests.exist')));
    } else {
      var list = el('div', 'rm-investigation-test-list');
      list.setAttribute('role', 'list');
      references.forEach(function (reference) {
        if (!reference || typeof reference.path !== 'string') return;
        var path = boundedText(reference.path, 320);
        var line = Number.isInteger(reference.line) && reference.line > 0 ? reference.line : 0;
        if (!path || !line) return;
        var item = el('div', 'rm-investigation-test-reference');
        item.setAttribute('role', 'listitem');
        item.appendChild(txt('code', '', path + ':' + line));
        item.appendChild(txt('span', '', msg('main.chrome.direct.static.reference.to.the.exact.symbol')));
        list.appendChild(item);
      });
      section.appendChild(list);
    }
    section.appendChild(txt('p', 'rm-investigation-caveat', msg('main.chrome.navigation.evidence.only.this.does.not.prove.coverage.or.what.a.test.asserts.test.paths.stay.non.clickable.until.they.are.part.of.saved.run.authority')));
    var warningCount = Array.isArray(response.test_warnings) ? response.test_warnings.length : 0;
    if (warningCount > 0) {
      section.appendChild(txt('div', 'rm-investigation-test-warning', msg('main.count.warnings', {
        count: warningCount,
      })));
    }
    return section;
  }

  function renderInvestigationCheckpoint(response, key) {
    var status = response && typeof response.investigation_status === 'string' ? response.investigation_status : '';
    if (status !== 'source_ready' && status !== 'tests_ready') return null;
    var section = el('section', 'rm-investigation-checkpoint');
    if (status === 'tests_ready') {
      section.appendChild(txt('div', 'rm-investigation-checkpoint-status', msg('main.chrome.local.checkpoint.complete')));
      section.appendChild(renderTestReferences(response));
      return section;
    }
    section.appendChild(txt('div', 'rm-investigation-checkpoint-status', msg('main.chrome.saved.locally')));
    if (response.can_find_test_references) {
      var state = symbolLookupStates[key] || {};
      var loading = state.testStatus === 'loading';
      var button = txt('button', 'rm-investigation-test-button', loading ? msg('main.chrome.finding.test.references') : msg('main.chrome.find.related.test.references'));
      button.type = 'button';
      button.disabled = loading;
      button.onclick = function () { requestTargetTestReferences(key); };
      section.appendChild(button);
      section.appendChild(txt('div', 'rm-investigation-local-hint', msg('main.chrome.local.gopls.only.no.model.request')));
    }
    return section;
  }

  function renderSymbolInspection(inspection, key) {
    var detail = el('section', 'rm-symbol-detail');
    detail.id = symbolInspectionElementID(key);
    detail.setAttribute('aria-label', msg('main.chrome.exact.go.symbol.details'));
    if (inspection.status === 'loading') {
      var loading = txt('div', 'rm-symbol-status', msg('main.chrome.inspecting.exact.go.symbol.locally'));
      loading.setAttribute('role', 'status');
      loading.setAttribute('aria-live', 'polite');
      detail.appendChild(loading);
      return detail;
    }
    if (inspection.status === 'error') {
      var error = txt('div', 'rm-symbol-status rm-symbol-status--error', msg('main.chrome.could.not.inspect.this.go.symbol'));
      error.setAttribute('role', 'alert');
      detail.appendChild(error);
      return detail;
    }
    if (inspection.status !== 'ready' || !inspection.detail) return detail;

    var response = inspection.detail;
    var target = response.target || {};
    detail.appendChild(txt('div', 'rm-symbol-detail-label', msg('main.chrome.exact.symbol')));
    var targetHeading = el('div', 'rm-symbol-detail-heading');
    targetHeading.appendChild(txt('code', 'rm-symbol-detail-name', boundedText(target.name, 160) || msg('main.chrome.unnamed.go.symbol')));
    var targetKind = exactSymbolDetail(target.kind);
    if (targetKind) targetHeading.appendChild(txt('span', 'rm-symbol-candidate-kind', targetKind));
    detail.appendChild(targetHeading);
    detail.appendChild(renderEntityLocation(target, 'rm-symbol-detail-location'));

    var evidenceLevel = exactSymbolDetail(response.evidence_level) || 'static';
    detail.appendChild(txt('p', 'rm-symbol-static-note', msg('main.symbol.static_hierarchy', {
      level: evidenceLevel,
    })));

    var incoming = Array.isArray(response.incoming_calls) ? response.incoming_calls.slice(0, maxStaticCalls) : [];
    var outgoing = Array.isArray(response.outgoing_calls) ? response.outgoing_calls.slice(0, maxStaticCalls) : [];
    var truncated = response.truncated && typeof response.truncated === 'object' ? response.truncated : {};
    var incomingOmitted = Number.isInteger(truncated.incoming_calls) && truncated.incoming_calls > 0 ? truncated.incoming_calls : 0;
    var outgoingOmitted = Number.isInteger(truncated.outgoing_calls) && truncated.outgoing_calls > 0 ? truncated.outgoing_calls : 0;
	detail.appendChild(renderSymbolNeighborhood(target, incoming, outgoing, incomingOmitted, outgoingOmitted));
	detail.appendChild(renderSourceWindow(response.source || {}));
	if (incoming.length > 2 || outgoing.length > 2 || incomingOmitted > 0 || outgoingOmitted > 0) {
	  var callDetails = el('details', 'rm-symbol-call-details');
	  callDetails.appendChild(txt('summary', 'rm-symbol-call-details-summary', msg('main.chrome.all.returned.static.relations')));
		  callDetails.appendChild(renderStaticCallList(msg('main.incoming_static_calls'), incoming, incomingOmitted));
		  callDetails.appendChild(renderStaticCallList(msg('main.outgoing_static_calls'), outgoing, outgoingOmitted));
	  detail.appendChild(callDetails);
	}

    var truncation = renderInspectionTruncation(truncated);
    if (truncation) detail.appendChild(truncation);
    var checkpoint = renderInvestigationCheckpoint(response, key);
    if (checkpoint) detail.appendChild(checkpoint);
    var warningCount = Array.isArray(response.warnings) ? response.warnings.length : 0;
    if (warningCount > 0) {
      var warningSection = el('section', 'rm-symbol-warnings');
      warningSection.appendChild(txt('div', 'rm-symbol-detail-label', msg('main.warnings')));
      var warningList = el('ul', 'rm-symbol-warning-list');
      warningList.appendChild(txt('li', '', msg('main.count.warnings', { count: warningCount })));
      warningSection.appendChild(warningList);
      detail.appendChild(warningSection);
    }
    return detail;
  }

  function paintSymbolLookup(key) {
    var state = symbolLookupStates[key] || { status: 'idle', candidates: [] };
    var view = symbolLookupViews[key];
    if (!view) return;

    var inspection = state.inspection || { status: 'idle' };
    view.button.disabled = state.status === 'loading' || inspection.status === 'loading';
    view.button.textContent = state.status === 'ready' ? msg('main.refresh_go_symbols') : msg('main.chrome.find.go.symbols');
    view.button.setAttribute('aria-expanded', state.status === 'ready' ? 'true' : 'false');
    view.results.setAttribute('aria-busy', state.status === 'loading' || inspection.status === 'loading' ? 'true' : 'false');
    view.results.innerHTML = '';

    if (state.status === 'loading') {
      var loading = txt('div', 'rm-symbol-status', msg('main.chrome.finding.go.symbols.locally'));
      loading.setAttribute('role', 'status');
      loading.setAttribute('aria-live', 'polite');
      view.results.appendChild(loading);
      return;
    }
    if (state.status === 'error') {
      var error = txt('div', 'rm-symbol-status rm-symbol-status--error', msg('main.chrome.could.not.find.go.symbols'));
      error.setAttribute('role', 'alert');
      view.results.appendChild(error);
      return;
    }
    if (state.status !== 'ready') return;

    var candidates = state.candidates.slice(0, maxSymbolCandidates);
    if (candidates.length === 0) {
      var empty = txt('div', 'rm-symbol-status', msg('main.chrome.no.go.functions.or.methods.found.near.this.anchor'));
      empty.setAttribute('role', 'status');
      empty.setAttribute('aria-live', 'polite');
      view.results.appendChild(empty);
      return;
    }
    var title = txt('div', 'rm-symbol-results-title', msg('main.symbol.count', {
      count: candidates.length,
    }));
    title.setAttribute('role', 'status');
    title.setAttribute('aria-live', 'polite');
    view.results.appendChild(title);
    var list = el('div', 'rm-symbol-candidates');
    list.setAttribute('role', 'list');
    candidates.forEach(function (candidate) {
      list.appendChild(renderSymbolCandidate(candidate, key, state));
      if (inspection.status !== 'idle' && inspection.candidateID === candidate.id) {
        list.appendChild(renderSymbolInspection(inspection, key));
      }
    });
    view.results.appendChild(list);
  }

  function revealSymbolInspection(key) {
    var detail = document.getElementById(symbolInspectionElementID(key));
    if (!detail || typeof detail.scrollIntoView !== 'function') return;
    detail.scrollIntoView({ block: 'nearest' });
  }

  function requestSymbols(component, anchor, line, key) {
    var runID = currentRunID();
    if (!runID) {
      symbolLookupStates[key] = { status: 'error', candidates: [], error: msg('main.error.saved_run_unavailable') };
      paintSymbolLookup(key);
      return;
    }

    symbolLookupStates[key] = { status: 'loading', candidates: [] };
    paintSymbolLookup(key);
    fetch(serverBasePath() + '/api/symbols', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Repomap-Action': 'list-symbols',
      },
      body: JSON.stringify({
        run_id: runID,
        component_id: component.id,
        anchor_id: anchor.id,
        line: line,
      }),
    }).then(function (response) {
      return response.json().catch(function () { return {}; }).then(function (body) {
        if (!response.ok) throw new Error(msg('main.error.go_symbol_lookup_failed'));
        return body;
      });
    }).then(function (payload) {
      var candidates = Array.isArray(payload.candidates) ? payload.candidates.slice(0, maxSymbolCandidates) : [];
      var candidateSetID = typeof payload.candidate_set_id === 'string' ? payload.candidate_set_id : '';
      if (candidates.length > 0 && !candidateSetID) throw new Error(msg('main.error.go_symbol_candidates_uninspectable'));
      symbolLookupStates[key] = {
        status: 'ready',
        candidates: candidates,
        candidateSetID: candidateSetID,
        inspection: { status: 'idle' },
      };
      paintSymbolLookup(key);
    }).catch(function () {
      symbolLookupStates[key] = {
        status: 'error',
        candidates: [],
        error: msg('main.chrome.could.not.find.go.symbols'),
      };
      paintSymbolLookup(key);
    });
  }

  function requestSymbolInspection(key, candidate) {
    var state = symbolLookupStates[key];
    var runID = currentRunID();
    if (!state || state.status !== 'ready' || !runID || !state.candidateSetID || typeof candidate.id !== 'string' || !candidate.id) {
      if (state) {
        state.inspection = {
          status: 'error',
          candidateID: candidate.id,
          error: msg('main.error.symbol_candidate_unavailable'),
        };
        paintSymbolLookup(key);
      }
      return;
    }

    var candidateSetID = state.candidateSetID;
    state.inspection = { status: 'loading', candidateID: candidate.id };
    paintSymbolLookup(key);
    revealSymbolInspection(key);
    fetch(serverBasePath() + '/api/symbol', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Repomap-Action': 'inspect-symbol',
      },
      body: JSON.stringify({
        run_id: runID,
        candidate_set_id: candidateSetID,
        candidate_id: candidate.id,
      }),
    }).then(function (response) {
      return response.json().catch(function () { return {}; }).then(function (body) {
        if (!response.ok) throw new Error(msg('main.error.go_symbol_inspection_failed'));
        return body;
      });
    }).then(function (payload) {
      var current = symbolLookupStates[key];
      if (!current || current.candidateSetID !== candidateSetID) return;
      current.inspection = { status: 'ready', candidateID: candidate.id, detail: payload };
      current.persisted = payload.investigation_status === 'source_ready' || payload.investigation_status === 'tests_ready';
      paintSymbolLookup(key);
      revealSymbolInspection(key);
    }).catch(function () {
      var current = symbolLookupStates[key];
      if (!current || current.candidateSetID !== candidateSetID) return;
      current.inspection = {
        status: 'error',
        candidateID: candidate.id,
        error: msg('main.chrome.could.not.inspect.this.go.symbol'),
      };
      paintSymbolLookup(key);
      revealSymbolInspection(key);
    });
  }

  function requestTargetTestReferences(key) {
    var state = symbolLookupStates[key];
    var runID = currentRunID();
    if (!state || !state.inspection || state.inspection.status !== 'ready' || !runID || state.testStatus === 'loading') return;
    state.testStatus = 'loading';
    paintSymbolLookup(key);
    revealSymbolInspection(key);
    fetch(serverBasePath() + '/api/investigation/target-tests', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Repomap-Action': 'find-test-references',
      },
      body: JSON.stringify({ run_id: runID }),
    }).then(function (response) {
      return response.json().catch(function () { return {}; }).then(function (body) {
        if (!response.ok) throw new Error(msg('main.error.related_test_references_failed'));
        return body;
      });
    }).then(function (payload) {
      var current = symbolLookupStates[key];
      if (current) current.testStatus = 'idle';
      if (!applyResumedInvestigation(payload)) {
        if (current) paintSymbolLookup(key);
        throw new Error(msg('main.error.saved_investigation_mismatch'));
      }
    }).catch(function () {
      var current = symbolLookupStates[key];
      if (current) current.testStatus = 'idle';
      paintSymbolLookup(key);
      showToast(msg('main.error.related_test_references_failed'), true);
      revealSymbolInspection(key);
    });
  }

  function rawComponentAnchor(componentID, anchorID) {
    var components = Array.isArray(DATA.components) ? DATA.components : [];
    for (var componentIndex = 0; componentIndex < components.length; componentIndex++) {
      if (components[componentIndex].id !== componentID) continue;
      var anchors = Array.isArray(components[componentIndex].anchor_groups) ? components[componentIndex].anchor_groups : [];
      for (var anchorIndex = 0; anchorIndex < anchors.length; anchorIndex++) {
        if (anchors[anchorIndex].id === anchorID) {
          return { component: components[componentIndex], anchor: anchors[anchorIndex] };
        }
      }
    }
    return null;
  }

  function applyResumedInvestigation(payload) {
    if (!payload || typeof payload.component_id !== 'string' || typeof payload.anchor_id !== 'string' || !payload.target) return false;
    var raw = rawComponentAnchor(payload.component_id, payload.anchor_id);
    var selectComponent = componentSelectionViews[payload.component_id];
    if (!raw || typeof selectComponent !== 'function') return false;
    var line = 0;
    if (Array.isArray(raw.anchor.locations)) {
      raw.anchor.locations.forEach(function (location) {
        if (!location || !Number.isInteger(location.line) || location.line <= 0) return;
        if (line === 0 || location.line < line) line = location.line;
      });
    }
    var key = symbolLookupKey(raw.component, raw.anchor, line);
    var target = payload.target;
    var savedCandidateID = 'saved-investigation';
    symbolLookupStates[key] = {
      status: 'ready',
      candidates: [{
        id: savedCandidateID,
        name: boundedText(target.name, 160),
        kind: target.kind,
        path: boundedText(target.path, 320),
        line: target.line,
        column: target.column,
        match: msg('main.saved_exact_selection'),
        certainty: payload.evidence_level || 'static',
        distance_lines: 0,
        rank_reasons: [],
      }],
      candidateSetID: '',
      persisted: true,
      inspection: { status: 'ready', candidateID: savedCandidateID, detail: payload },
    };
    showTab('rm-overview');
    selectComponent();
    revealSymbolInspection(key);
    return true;
  }

  function resumeLatestInvestigation() {
    if (resumeInvestigationStarted || !serverMode()) return;
    var runID = currentRunID();
    if (!runID) return;
    resumeInvestigationStarted = true;
    fetch(serverBasePath() + '/api/investigation/latest', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Repomap-Action': 'resume-investigation',
      },
      body: JSON.stringify({ run_id: runID }),
    }).then(function (response) {
      if (response.status === 204 || response.status === 404) return null;
      return response.json().catch(function () { return {}; }).then(function (body) {
        if (!response.ok) throw new Error(msg('main.error.saved_investigation_unavailable'));
        return body;
      });
    }).then(function (payload) {
      if (payload) applyResumedInvestigation(payload);
    }).catch(function () {
      showToast(msg('main.error.saved_investigation_unavailable'), true);
    });
  }

  function showArchitectureTarget() {
    showTab('rm-overview');
    var target = document.querySelector('.rm-architecture-canvas-card') ||
      document.querySelector('.rm-orientation-map') || document.getElementById('rm-overview');
    if (target && typeof target.scrollIntoView === 'function') {
      target.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }
  }

  function openReportTarget(target) {
    target = target || {};
    var kind = String(target.kind || 'map');
    if (kind === 'semantic_artifact') {
      var hasArtifactStep = target.step_index != null || target.index != null;
      var artifactStepIndex = Number(target.step_index != null ? target.step_index : target.index);
      var targetMechanism = userMechanismByID(USER_MECHANISMS, target.artifact_id || target.id);
      var narrativeStepIndex = Number.isFinite(artifactStepIndex) && targetMechanism
        ? narrativeIndexForImplementationStep(targetMechanism, artifactStepIndex)
        : 0;
      openUserMechanism(
        target.artifact_id || target.id,
        narrativeStepIndex,
        hasArtifactStep
      );
      return;
    }
		if (kind === 'study_direction') {
			openStudyDirection(target.direction_id || target.id);
			return;
		}
		if (kind === 'paved_path') {
			openPavedPath(target.paved_path_id || target.id);
			return;
		}
    if (kind === 'component') {
      openArchitectureTarget({ kind: 'component', component_id: target.component_id || target.id }, null);
      return;
    }
    if (kind === 'flow') {
      openArchitectureTarget({ kind: 'flow', flow_id: target.flow_id || target.id }, null);
      return;
    }
    if (kind === 'flow_step') {
      openArchitectureTarget({
        kind: 'flow_step',
        flow_id: target.flow_id || target.scope_id,
        step_id: target.step_id || target.id,
      }, null);
      return;
    }
    if (kind === 'surface') {
      openArchitectureTarget({ kind: 'surface', surface_id: target.surface_id || target.id }, null);
      return;
    }
    if (kind === 'guided_step' && DEBUG_MODE && architectureCanvasView) {
      var stepIndex = Number(target.step_index != null ? target.step_index : target.index);
      architectureCanvasView.openGuidedTourStep(Number.isFinite(stepIndex) ? stepIndex : 0);
      showArchitectureTarget();
      return;
    }
    var location = target.location || target;
    if (kind === 'location' && location.path) {
      openSourceLocation(location);
      return;
    }
    openArchitectureTarget(null, null);
  }

  function reportTargetAvailable(target) {
    target = target || {};
    var kind = target ? String(target.kind || '') : '';
    if (kind === 'map' || kind === 'component' || kind === 'flow' || kind === 'flow_step' || kind === 'surface') {
      return userArchitectureAvailable();
    }
		if (kind === 'paved_path') return !!pavedPathByID(target.paved_path_id || target.id);
		if (kind === 'study_direction') return !!studyDirectionByID(target.direction_id || target.id);
    if (kind !== 'location') return true;
    var location = target.location || target;
    if (!location.path || !OPENABLE_PATH_SET[location.path]) return false;
    if (staticSourceLocationAvailable(location.path, location.line)) return true;
    if (embeddedSourceForLocation(location)) return true;
    return !!(serverMode() && currentRunID() && SOURCE_IDS[location.path]);
  }

  function renderSymbolLookup(component, anchor) {
    var line = symbolLookupLine(anchor);
    var key = symbolLookupKey(component, anchor, line);
    var container = el('div', 'rm-symbol-lookup');
    var button = txt('button', 'rm-symbol-find-button', msg('main.chrome.find.go.symbols'));
    button.type = 'button';
    var results = el('div', 'rm-symbol-results');
    results.id = symbolLookupElementID(key);
    button.setAttribute('aria-controls', results.id);
    button.onclick = function () {
      requestSymbols(component, anchor, line, key);
    };
    container.appendChild(button);
    container.appendChild(results);
    symbolLookupViews[key] = { button: button, results: results };
    paintSymbolLookup(key);
    return container;
  }

  function buildComponentModel(item, index, directions, flows) {
    var references = componentReferences(item.evidence || []);
    var componentPathSet = {};
    references.forEach(function (ref) { componentPathSet[ref.path] = true; });
    var relatedDirections = (directions || []).filter(function (direction) {
      return directionPaths(direction).some(function (path) { return componentPathSet[path]; });
    });
    var relatedIDs = {};
    relatedDirections.forEach(function (direction) { relatedIDs[direction.id] = true; });
    var relatedFlows = (flows || []).filter(function (flow) { return relatedIDs[flow.id]; });

    var files = groupComponentFiles(references);

    var packages = [];
    var packageSet = {};
    var tests = [];
    var testSet = {};
    files.forEach(function (file) {
      var pkg = packageForFile(file.path);
      if (pkg && !packageSet[pkg]) {
        packageSet[pkg] = true;
        packages.push(pkg);
      }
    });
    relatedFlows.forEach(function (flow) {
      (flow.bundle_tests || []).concat(flow.tests_to_read || []).forEach(function (test) {
        if (test.path && !testSet[test.path]) {
          testSet[test.path] = true;
          tests.push(test);
        }
      });
    });

    return {
      id: 'component-' + index,
      name: item.name || msg('main.unnamed_component'),
	  role: normalizeComponentRole(item.role),
	  role_basis: normalizeComponentRole(item.role) === 'unknown' ? 'unknown' : 'hypothesis',
      purpose: item.why_it_matters || '',
      evidence: item.evidence || [],
      references: references,
      files: files,
      directions: relatedDirections,
      flows: relatedFlows,
      packages: packages,
      tests: tests,
    };
  }

  function buildStructuredComponentModel(item, index, directions, flows) {
    var directionIDs = {};
    (item.related_flow_ids || []).forEach(function (id) { directionIDs[id] = true; });
    var relatedDirections = (directions || []).filter(function (direction) { return directionIDs[direction.id]; });
    var relatedFlows = (flows || []).filter(function (flow) { return directionIDs[flow.id]; });
    var files = (item.anchor_groups || []).map(function (anchor) {
      var lines = [];
      (anchor.locations || []).forEach(function (location) {
        if (location.line && lines.indexOf(location.line) < 0) lines.push(location.line);
      });
      lines.sort(function (a, b) { return a - b; });
      return {
        id: anchor.id,
        path: anchor.path,
        lines: lines,
        statements: anchor.model_notes || [],
        grounding: anchor.grounding,
        context: anchor.local_context || [],
        can_list_symbols: !!anchor.can_list_symbols,
      };
    });
    var references = [];
    files.forEach(function (file) {
      if (file.lines.length === 0) references.push({ path: file.path, line: 0 });
      file.lines.forEach(function (line) { references.push({ path: file.path, line: line }); });
    });
    var tests = [];
    var testSet = {};
    relatedFlows.forEach(function (flow) {
      (flow.bundle_tests || []).concat(flow.tests_to_read || []).forEach(function (test) {
        if (test.path && !testSet[test.path]) {
          testSet[test.path] = true;
          tests.push(test);
        }
      });
    });
    var evidence = [];
    files.forEach(function (file) {
      file.statements.forEach(function (statement) {
        if (evidence.indexOf(statement) < 0) evidence.push(statement);
      });
    });
    return {
      id: item.id || 'component-' + index,
      name: item.name || msg('main.unnamed_component'),
	  role: normalizeComponentRole(item.role),
	  role_basis: item.role_basis || 'hypothesis',
      purpose: item.model_purpose || '',
      evidence: evidence,
      references: references,
      files: files,
      directions: relatedDirections,
      flows: relatedFlows,
      packages: item.packages || [],
      tests: tests,
    };
  }

	var componentRoleOrder = ['entry', 'boundary', 'coordination', 'domain', 'state', 'support', 'unknown'];
	var componentRoleLabels = {
	  entry: msg('main.component_role.entrypoints'),
	  boundary: msg('main.boundaries'),
	  coordination: msg('main.component_role.coordination'),
	  domain: msg('main.component_role.core_domain'),
	  state: msg('main.component_role.state'),
	  support: msg('main.component_role.support'),
	  unknown: msg('main.component_role.unclassified'),
	};

	function normalizeComponentRole(role) {
	  role = typeof role === 'string' ? role.toLowerCase() : '';
	  return componentRoleOrder.indexOf(role) >= 0 ? role : 'unknown';
	}

  function componentAnchorGroundingLabel(grounding) {
    if (grounding === 'verified_line') return msg('main.grounding.verified_line');
    if (grounding === 'verified_direction_path') return msg('main.grounding.verified_direction_path');
    if (grounding === 'verified_path') return msg('main.grounding.verified_path');
    return msg('main.repository_anchor');
  }

  function renderComponentInspector(component) {
    var inspector = el('aside', 'rm-component-inspector');
    inspector.setAttribute('aria-label', msg('main.chrome.selected.component.details'));
    inspector.appendChild(txt('div', 'rm-direction-label', msg('main.component.inspector')));
    inspector.appendChild(txt('h3', '', component.name));
    if (component.purpose) inspector.appendChild(linkified('p', 'rm-component-purpose', component.purpose));

    if (component.files.length > 0) {
      inspector.appendChild(txt('div', 'rm-section-title', msg('main.start.here')));
      var hasSymbolAnchors = component.files.some(function (group) {
        return group.id && group.can_list_symbols;
      });
      if (hasSymbolAnchors && !serverMode() && !staticSourceMode()) {
        inspector.appendChild(txt('p', 'rm-symbol-static-hint', msg('main.chrome.run.repomap.serve.to.find.go.symbols.near.these.anchors')));
      }
      var files = el('div', 'rm-component-inspector-list');
      component.files.slice(0, 8).forEach(function (group, index) {
        var row = el('div', 'rm-component-inspector-row');
        if (index === 0) row.classList.add('rm-component-inspector-row--recommended');
        var links = el('div', 'rm-component-anchor-links');
        if (index === 0) {
          links.appendChild(txt('span', 'rm-component-anchor-recommended', msg('main.recommended.start')));
        }
        componentFileLinks(group).forEach(function (ref) {
          links.appendChild(renderFileReference(ref.path, 'rm-component-file', ref.line, ref.label));
        });
        row.appendChild(links);
        if (DATA.freshness && (DATA.freshness.affected_paths || []).indexOf(group.path) >= 0) {
          row.appendChild(txt('div', 'rm-component-anchor-context', msg('main.this.source.changed.after.the.report.was.generated')));
        }
        if (group.grounding) {
          row.appendChild(txt('span', 'rm-component-anchor-grounding', componentAnchorGroundingLabel(group.grounding)));
        }
        if (group.context && group.context.length > 0) {
          var context = group.context[0];
          var contextText = context.reason || context.category || msg('main.local_source_signal');
          if (context.snippet) contextText += ': ' + context.snippet;
          row.appendChild(txt('div', 'rm-component-anchor-context', contextText));
        }
        if (serverMode() && group.id && group.can_list_symbols) {
          row.appendChild(renderSymbolLookup(component, group));
        }
        files.appendChild(row);
      });
      inspector.appendChild(files);
    }

    if (component.directions.length > 0) {
      inspector.appendChild(txt('div', 'rm-section-title', msg('main.related.flows')));
      var directions = el('div', 'rm-component-related-list');
      component.directions.forEach(function (direction) {
        var flow = flowByID(direction.id);
        var row = el('div', 'rm-component-related-row');
        if (flow) {
          var button = txt('button', 'rm-component-related-button', direction.name || direction.id);
          button.type = 'button';
          button.onclick = function () { showTab('rm-flow-' + flow.id); };
          row.appendChild(button);
        } else {
          row.appendChild(txt('span', 'rm-component-related-name', direction.name || direction.id));
        }
        if (direction.trigger) {
          row.appendChild(txt('span', 'rm-component-related-trigger', LABELS.trigger + ': ' + direction.trigger));
        }
        directions.appendChild(row);
      });
      inspector.appendChild(directions);
    }

    if (component.packages.length > 0) {
      inspector.appendChild(txt('div', 'rm-section-title', msg('main.packages')));
      var packages = el('div', 'rm-component-packages');
      component.packages.slice(0, 8).forEach(function (pkg) {
        packages.appendChild(renderPackageReference(pkg));
      });
      inspector.appendChild(packages);
    }

    if (component.tests.length > 0) {
      inspector.appendChild(txt('div', 'rm-section-title', msg('main.tests')));
      var tests = el('div', 'rm-component-inspector-list');
      component.tests.slice(0, 6).forEach(function (test) {
        var row = el('div', 'rm-component-inspector-row');
        row.appendChild(renderFileReference(test.path, 'rm-component-file', 0, test.path));
        tests.appendChild(row);
      });
      inspector.appendChild(tests);
    }

    if (component.evidence.length > 0) {
      inspector.appendChild(txt('div', 'rm-section-title', LABELS.orientationEvidence));
      var evidence = el('div', 'rm-component-inspector-evidence');
      component.evidence.forEach(function (statement) {
        evidence.appendChild(linkified('div', 'rm-direction-evidence-item', statement));
      });
      inspector.appendChild(evidence);
    }
    return inspector;
  }

  // Reports without a saved Architecture Canvas keep a compact orientation
  // fallback. This is deliberately a list, not a second graph renderer: the
  // model-oriented cards remain useful while graph semantics stay owned by
  // ArchitectureCanvas.
  function renderSystemMapCard(items, directions, flows) {
    var structured = DATA.components && DATA.components.length > 0;
    var componentItems = structured ? DATA.components : (items || []);
    if (componentItems.length === 0) return null;

    var components = componentItems.map(function (item, index) {
      return structured ? buildStructuredComponentModel(item, index, directions, flows) :
        buildComponentModel(item, index, directions, flows);
    });
    var card = el('section', 'rm-card rm-orientation-map');
    card.appendChild(txt('h2', '', LABELS.systemMap));
    card.appendChild(txt(
      'p',
      'rm-orientation-map-hint',
      DATA.architecture_synthesis && DATA.architecture_synthesis.state === 'failed' ?
        msg('main.orientation_map.generation_failed') :
        msg('main.orientation_map.saved_orientation')
    ));

    var grid = el('div', 'rm-orientation-component-grid');
    var buttons = [];
    var inspectorHost = el('div', 'rm-orientation-inspector');
    function selectComponent(index) {
      buttons.forEach(function (button, buttonIndex) {
        var selected = buttonIndex === index;
        button.classList.toggle('is-selected', selected);
        button.setAttribute('aria-pressed', selected ? 'true' : 'false');
      });
      inspectorHost.innerHTML = '';
      inspectorHost.appendChild(renderComponentInspector(components[index]));
    }

    components.forEach(function (component, index) {
      var button = el('button', 'rm-orientation-component');
      button.type = 'button';
      button.setAttribute('aria-pressed', 'false');
      button.appendChild(txt('strong', '', component.name));
      if (component.purpose) button.appendChild(txt('span', '', component.purpose));
      button.appendChild(txt(
        'small',
        '',
        msg('main.component.role_anchor_count', {
          role: componentRoleLabels[component.role] || componentRoleLabels.unknown,
          count: component.files.length,
        })
      ));
      button.onclick = function () { selectComponent(index); };
      buttons.push(button);
      componentSelectionViews[component.id] = function () { selectComponent(index); };
      grid.appendChild(button);
    });
    card.appendChild(grid);
    card.appendChild(inspectorHost);
    selectComponent(0);
    return card;
  }

  function renderStartHereCard(files) {
    if (!files || files.length === 0) return null;

    var card = el('div', 'rm-card');
    card.appendChild(txt('h2', '', LABELS.startFiles));
    var list = el('div', 'rm-read-order');
    files.forEach(function (file, index) {
      list.appendChild(renderReadOrderItem(file, index + 1, file.priority || index + 1));
    });
    card.appendChild(list);
    return card;
  }

  function renderTermsCard(words) {
    if (!words || words.length === 0) return null;

    var card = el('div', 'rm-card');
    card.appendChild(txt('h2', '', LABELS.terms));
    var grid = el('div', 'rm-overview-flows');
    words.forEach(function (word) {
      var term = el('div', 'rm-ov-flow rm-candidate-direction');
      term.appendChild(txt('h3', '', word.word));
      if (word.guess) {
        term.appendChild(linkified('div', 'rm-summary-line', word.guess));
      }
      appendOrientationEvidence(term, word.evidence);
      grid.appendChild(term);
    });
    card.appendChild(grid);
    return card;
  }

  function renderQuestionsCard(questions, unverifiedPaths) {
    var hasQuestions = questions && questions.length > 0;
    var hasUnverified = unverifiedPaths && unverifiedPaths.length > 0;
    if (!hasQuestions && !hasUnverified) return null;

    var card = el('div', 'rm-card');
    card.appendChild(txt('h2', '', LABELS.questions));
    if (hasQuestions) {
      var list = el('ul', 'rm-file-list');
      questions.forEach(function (question) {
        list.appendChild(linkified('li', '', question));
      });
      card.appendChild(list);
    }
    var unverified = renderUnverifiedPaths(unverifiedPaths);
    if (unverified) card.appendChild(unverified);
    return card;
  }

  function renderDirectionsCard(directions, flows) {
    var card = el('div', 'rm-card');
    card.appendChild(txt('h2', '', LABELS.candidateDirections));
    var acceptedDirections = directions.filter(function (direction) { return direction.disposition !== 'rejected'; });
    var rejectedDirections = directions.filter(function (direction) { return direction.disposition === 'rejected'; });
    if (acceptedDirections.length === 0) {
      card.appendChild(renderPlaceholder(LABELS.noFlows));
    } else {
      card.appendChild(txt('p', 'rm-direction-hint', LABELS.directionHint));
      var directionsGrid = el('div', 'rm-overview-flows');
      var highestConfidence = Math.max.apply(null, acceptedDirections.map(function (direction) {
        return direction.confidence || 0;
      }));
      acceptedDirections.forEach(function (direction) {
        directionsGrid.appendChild(renderCandidateDirectionCard(
          direction,
          (direction.confidence || 0) === highestConfidence
        ));
      });
      card.appendChild(directionsGrid);
    }

    if (rejectedDirections.length > 0) {
      var rejected = el('details', 'rm-details');
      rejected.appendChild(txt('summary', '', LABELS.rejectedDirections + ' · ' + rejectedDirections.length));
      var rejectedGrid = el('div', 'rm-overview-flows');
      rejectedDirections.forEach(function (direction) {
        rejectedGrid.appendChild(renderCandidateDirectionCard(direction, false));
      });
      rejected.appendChild(rejectedGrid);
      card.appendChild(rejected);
    }

    var expandedFlows = flows.filter(function (flow) {
      return !flow.evidence_only && !flow.error && flow.flow_status !== 'local_only';
    });
    if (expandedFlows.length > 0) {
      card.appendChild(txt('div', 'rm-section-title', LABELS.candidateFlows));
      var flowsGrid = el('div', 'rm-overview-flows');
      expandedFlows.forEach(function (flow) {
        var isRecommended = DATA.recommended_flow && flow.id === DATA.recommended_flow;
        flowsGrid.appendChild(renderFlowCard(flow, isRecommended));
      });
      card.appendChild(flowsGrid);

      var quickStart = el('div', 'rm-quick-start');
      quickStart.appendChild(txt('div', 'rm-section-title', LABELS.quickStart));
      expandedFlows.forEach(function (flow) {
        if (!flow.files_to_read_in_order || flow.files_to_read_in_order.length === 0) return;

        var item = el('div', 'rm-quick-start-item');
        var flowLink = el('span', 'rm-quick-start-flow');
        flowLink.textContent = flow.name || flow.id;
        flowLink.onclick = function () { showTab('rm-flow-' + flow.id); };
        item.appendChild(flowLink);
        item.appendChild(document.createTextNode('→ '));
        item.appendChild(renderFileReference(
          flow.files_to_read_in_order[0].path,
          '',
          0,
          flow.files_to_read_in_order[0].path
        ));
        quickStart.appendChild(item);
      });
      card.appendChild(quickStart);
    }

    return card;
  }

  function buildFileReasonIndex(filesToRead) {
    var idx = {};
    if (!filesToRead) return idx;
    filesToRead.forEach(function (fi) {
      if (fi.reason) idx[fi.path] = fi.reason;
    });
    return idx;
  }

  function renderChainSteps(chain, filesToRead) {
    var reasonIndex = buildFileReasonIndex(filesToRead);

    var section = el('div');
    section.appendChild(txt('div', 'rm-section-title', LABELS.executionChain));

    var container = el('div', 'rm-chain');

    chain.forEach(function (s) {
      var hasName = s.name && s.name.length > 0;
      var hasWhat = s.what_happens && s.what_happens.length > 0;
      var hasEvidence = s.evidence_files && s.evidence_files.length > 0;

      var stepWhat = s.what_happens;
      var stepName = s.name;

      if (!hasWhat && hasEvidence) {
        for (var i = 0; i < s.evidence_files.length; i++) {
          var r = reasonIndex[s.evidence_files[i]];
          if (r) { stepWhat = r; break; }
        }
      }
      if (!hasName && hasEvidence) {
        var fname = s.evidence_files[0];
        var last = fname.split('/').pop();
        stepName = last || fname;
      }

      if (!hasEvidence && !stepWhat) return;

      var step = el('div', 'rm-chain-step');

      var circle = el('div', 'rm-chain-circle ' + chainCircleClass(s.confidence));
      circle.textContent = s.step;
      step.appendChild(circle);

      var body = el('div', 'rm-chain-body');

      if (stepName) {
        body.appendChild(txt('div', 'rm-chain-name', msg('main.flow.step_named', {
          step: s.step,
          name: stepName,
        })));
      } else {
        body.appendChild(txt('div', 'rm-chain-name', msg('main.flow.step', { step: s.step })));
      }

      if (stepWhat) {
        body.appendChild(linkified('div', 'rm-chain-desc', stepWhat));
      }

      if (hasEvidence) {
        var filesDiv = el('div', 'rm-chain-files');
        filesDiv.appendChild(txt('span', 'rm-chain-files-label', LABELS.evidenceFiles + ': '));
        s.evidence_files.forEach(function (ef) {
          var fileRow = el('div', 'rm-chain-file-row');
          var pathSpan = renderFileReference(ef, 'rm-chain-file', 0, ef);
          fileRow.appendChild(pathSpan);
          var eReason = reasonIndex[ef];
          if (eReason) {
            var reasonSpan = linkified('span', 'rm-chain-file-reason', eReason);
            fileRow.appendChild(reasonSpan);
          }
          filesDiv.appendChild(fileRow);
        });
        body.appendChild(filesDiv);
      }

      step.appendChild(body);
      container.appendChild(step);
    });

    section.appendChild(container);
    return section;
  }

  function renderReadOrder(files, maxShow, title) {
    if (!files || !files.length) return null;

    var section = el('div');
    section.appendChild(txt('div', 'rm-section-title', (title || LABELS.filesToRead) + ' (' + files.length + ')'));

    var limit = maxShow || files.length;
    var visible = files.slice(0, limit);
    var hidden = files.slice(limit);

    var ol = el('div', 'rm-read-order');
    visible.forEach(function (fi, i) {
      ol.appendChild(renderReadOrderItem(fi, i + 1, fi.priority));
    });
    section.appendChild(ol);

    if (hidden.length > 0) {
      var expandDiv = el('div', 'rm-read-order-expand');
      var btn = el('button', 'rm-expand-btn');
      btn.textContent = LABELS.showMore(hidden.length);
      var expanded = false;
      btn.onclick = function () {
        if (expanded) {
          while (ol.children.length > limit) ol.removeChild(ol.lastChild);
          btn.textContent = LABELS.showMore(hidden.length);
        } else {
          hidden.forEach(function (fi, j) {
            ol.appendChild(renderReadOrderItem(fi, limit + j + 1, fi.priority));
          });
          btn.textContent = LABELS.showLess;
        }
        expanded = !expanded;
      };
      expandDiv.appendChild(btn);
      section.appendChild(expandDiv);
    }

    return section;
  }

  function renderReadOrderItem(fi, num, priority) {
    var item = el('div', 'rm-read-order-item');

    var numEl = el('div', 'rm-read-order-num');
    if (priority >= 3) numEl.classList.add('rm-read-order-num--p3');
    else if (priority >= 2) numEl.classList.add('rm-read-order-num--p2');
    numEl.textContent = num;
    item.appendChild(numEl);

    var body = el('div', 'rm-read-order-body');
    var pathRow = el('div', 'rm-read-order-path-row');
    pathRow.appendChild(renderFileReference(fi.path, 'rm-read-order-path', 0, fi.path));
    if (fi.kind) pathRow.appendChild(renderKindBadge(fi.kind));
    body.appendChild(pathRow);
    if (DATA.freshness && (DATA.freshness.affected_paths || []).indexOf(fi.path) >= 0) {
      body.appendChild(txt('div', 'rm-read-order-reason', msg('main.this.source.changed.after.the.report.was.generated')));
    }
    if (fi.reason) {
      var displayedReason = fi.presentation_reason || humanizeReason(fi.reason);
      var reason = linkified('div', 'rm-read-order-reason', displayedReason);
      reason.title = displayedReason;
      body.appendChild(reason);
    }
    item.appendChild(body);

    return item;
  }

  function renderFileList(title, files) {
    if (!files || !files.length) return null;

    var section = el('div');
    section.appendChild(txt('div', 'rm-section-title', title + ' (' + files.length + ')'));

    var ul = el('ul', 'rm-file-list');
    files.forEach(function (f) {
      var li = el('li', 'rm-file-list-item');
      var pathSpan = renderFileReference(f.path, 'rm-file-path', 0, f.path);
      li.appendChild(pathSpan);
      if (DATA.freshness && (DATA.freshness.affected_paths || []).indexOf(f.path) >= 0) {
        li.appendChild(txt('span', 'rm-file-reason', msg('main.this.source.changed.after.the.report.was.generated')));
      }
      if (f.reason) {
        var displayedReason = f.presentation_reason || humanizeReason(f.reason);
        var reason = linkified('span', 'rm-file-reason', displayedReason);
        reason.title = displayedReason;
        li.appendChild(reason);
      }
      ul.appendChild(li);
    });
    section.appendChild(ul);
    return section;
  }

  function renderBoundedFileList(title, files, maxShow) {
    if (!files || !files.length) return null;
    var section = el('div');
    section.appendChild(txt('div', 'rm-section-title', title + ' (' + files.length + ')'));
    var list = el('ul', 'rm-file-list');
    var limit = Math.min(maxShow || files.length, files.length);
    var renderItem = function (file) {
      var li = el('li', 'rm-file-list-item');
      li.appendChild(renderFileReference(file.path, 'rm-file-path', 0, file.path));
      if (DATA.freshness && (DATA.freshness.affected_paths || []).indexOf(file.path) >= 0) {
        li.appendChild(txt('span', 'rm-file-reason', msg('main.this.source.changed.after.the.report.was.generated')));
      }
      if (file.reason) {
        var displayedReason = file.presentation_reason || humanizeReason(file.reason);
        var reason = linkified('span', 'rm-file-reason', displayedReason);
        reason.title = displayedReason;
        li.appendChild(reason);
      }
      return li;
    };
    files.slice(0, limit).forEach(function (file) { list.appendChild(renderItem(file)); });
    section.appendChild(list);
    if (files.length > limit) {
      var controls = el('div', 'rm-read-order-expand');
      var button = txt('button', 'rm-expand-btn', LABELS.showMore(files.length - limit));
      var expanded = false;
      button.type = 'button';
      button.onclick = function () {
        if (expanded) {
          while (list.children.length > limit) list.removeChild(list.lastChild);
          button.textContent = LABELS.showMore(files.length - limit);
        } else {
          files.slice(limit).forEach(function (file) { list.appendChild(renderItem(file)); });
          button.textContent = LABELS.showLess;
        }
        expanded = !expanded;
      };
      controls.appendChild(button);
      section.appendChild(controls);
    }
    return section;
  }

  function renderExpandableList(title, items, boxClass, maxShow) {
    if (!items || !items.length) return null;
    var limit = maxShow || 3;
    var visible = items.slice(0, limit);
    var hidden = items.slice(limit);

    var box = el('div', boxClass);
    var titleEl = el('strong');
    titleEl.textContent = title + ': ';
    box.appendChild(titleEl);

    visible.forEach(function (u, i) {
      var p = el('div');
      p.className = 'rm-exp-item';
      p.appendChild(document.createTextNode('• '));
      appendLinkifiedText(p, u);
      box.appendChild(p);
    });

    if (hidden.length > 0) {
      var hiddenDiv = el('div', 'rm-exp-hidden');
      hidden.forEach(function (u) {
        var p = el('div');
        p.className = 'rm-exp-item';
        p.appendChild(document.createTextNode('• '));
        appendLinkifiedText(p, u);
        hiddenDiv.appendChild(p);
      });
      box.appendChild(hiddenDiv);

      var btn = el('button', 'rm-expand-btn-inline');
      btn.textContent = LABELS.showMore(hidden.length);
      btn.onclick = function () {
        var showing = hiddenDiv.style.display === 'block';
        hiddenDiv.style.display = showing ? 'none' : 'block';
        btn.textContent = showing ? LABELS.showMore(hidden.length) : LABELS.showLess;
      };
      box.appendChild(btn);
    }

    return box;
  }

  function renderUnknowns(unknowns) {
    return renderExpandableList(LABELS.knownUnknowns, unknowns, 'rm-info-box', 3);
  }

  function renderWarnings(warnings) {
    return renderExpandableList(LABELS.warnings, warnings, 'rm-warn-box', 3);
  }

  function formatMillis(value) {
    var millis = Number(value || 0);
    if (!Number.isFinite(millis) || millis <= 0) return '0 ms';
    if (millis < 1000) return Math.round(millis) + ' ms';
    return (millis / 1000).toFixed(millis < 10000 ? 1 : 0) + ' s';
  }

  function summarizeRunWarnings(warnings, presentationWarnings, presentationWarningKinds, presentationWarningMessages) {
    var primary = [];
    var fixedMessages = new Set([
      'main.warning.study_editing_did_not_finish',
      'main.warning.study_checks_failed',
      'main.warning.study_no_source_adapter',
      'main.warning.study_no_source_functions',
      'main.warning.confidence_candidate_capped',
      'main.warning.confidence_orientation_capped_incomplete',
    ]);
    var structuredMessages = new Map();
    (presentationWarningMessages || []).forEach(function (presentation) {
      if (!presentation || !Number.isInteger(presentation.warning_index)) return;
      if (!fixedMessages.has(presentation.message_id)) return;
      structuredMessages.set(presentation.warning_index, presentation);
    });
    (warnings || []).forEach(function (warning, warningIndex) {
      var value = String(warning || '').trim();
      var presentationValue = String(
        presentationWarnings && presentationWarnings[warningIndex] || value
      ).trim();
      var presentationMessageID = String(
        presentationWarningKinds && presentationWarningKinds[warningIndex] || ''
      );
      if (!value) return;
      var structured = structuredMessages.get(warningIndex);
      if (structured) {
        if (structured.message_id === 'main.warning.confidence_candidate_capped') {
          primary.push(msg(structured.message_id, {
            index: structured.candidate_index,
            proposed: structured.proposed,
            capped: structured.capped,
          }));
        } else {
          primary.push(msg(structured.message_id, {
            proposed: structured.proposed,
            capped: structured.capped,
          }));
        }
        return;
      }
      if (fixedMessages.has(presentationMessageID)) {
        primary.push(msg(presentationMessageID));
        return;
      }
      primary.push(presentationValue);
    });
    return { primary: primary, modelContext: [], details: [] };
  }

  function renderRunWarnings(warnings) {
    var summarized = summarizeRunWarnings(
      warnings,
      DATA.presentation_warnings,
      DATA.presentation_warning_kinds,
      DATA.presentation_warning_messages
    );
    if (summarized.primary.length === 0 && summarized.modelContext.length === 0 && summarized.details.length === 0) return null;
    var container = el('div', 'rm-run-warning-stack');
    if (summarized.modelContext.length > 0) {
      var contextItems = [
        msg('main.model_context_limit_copy')
      ].concat(summarized.modelContext.map(function (warning) {
        return msg('main.model_reported_note', { note: warning });
      }));
      var context = renderExpandableList(msg('main.model_context_limit'), contextItems, 'rm-info-box', 1);
      if (context) container.appendChild(context);
    }
    var primary = renderExpandableList(LABELS.warnings, summarized.primary, 'rm-warn-box', 3);
    if (primary) container.appendChild(primary);
    if (summarized.details.length > 0) {
      var details = el('details', 'rm-analysis-details');
      details.appendChild(txt('summary', '', msg('main.analysis.details', {
        count: summarized.details.length,
      })));
      var list = el('ul');
      summarized.details.forEach(function (item) { list.appendChild(txt('li', '', item)); });
      details.appendChild(list);
      container.appendChild(details);
    }
    return container;
  }

  function renderBundleStats(stats) {
    var container = el('div', 'rm-collapsible');

    var toggle = el('button', 'rm-collapsible-toggle');
    toggle.textContent = LABELS.retrievalDetails;
    toggle.onclick = function () {
      container.classList.toggle('rm-collapsible--open');
    };
    container.appendChild(toggle);

    var body = el('div', 'rm-collapsible-body');
    var list = [
      msg('main.bundle.source_files_selected', { count: stats.selected_files_count }),
      msg('main.bundle.test_files_selected', { count: stats.selected_tests_count }),
      msg('main.bundle.docs_selected', { count: stats.selected_docs_count }),
      msg('main.bundle.packages_selected', { count: stats.selected_packages_count }),
      msg('main.bundle.related_import_edges', { count: stats.related_edges_count }),
    ];
    list.forEach(function (line) {
      var div = txt('div', 'rm-bundle-stat', line);
      body.appendChild(div);
    });
    container.appendChild(body);
    return container;
  }

  function renderStringList(title, items) {
    if (!items || !items.length) return null;

    var section = el('div');
    section.appendChild(txt('div', 'rm-section-title', title + ' (' + items.length + ')'));

    var ul = el('ul', 'rm-file-list');
    items.forEach(function (s) {
      var li = el('li');
      li.textContent = s;
      ul.appendChild(li);
    });
    section.appendChild(ul);
    return section;
  }

  function renderEdgeList(title, edges) {
    if (!edges || !edges.length) return null;

    var section = el('div');
    section.appendChild(txt('div', 'rm-section-title', title + ' (' + edges.length + ')'));

    var ul = el('ul', 'rm-file-list');
    edges.forEach(function (e) {
      var li = el('li');
      li.textContent = e.from + ' → ' + e.to;
      ul.appendChild(li);
    });
    section.appendChild(ul);
    return section;
  }

  function renderErrorBox(error) {
    var box = el('div', 'rm-error-box');
    box.textContent = error;
    return box;
  }

  function renderCollapsedError(error) {
    var container = el('div', 'rm-collapsible');

    var toggle = el('button', 'rm-collapsible-toggle');
    toggle.textContent = msg('main.chrome.show.parse.error');
    toggle.onclick = function () {
      container.classList.toggle('rm-collapsible--open');
    };
    container.appendChild(toggle);

    var body = el('div', 'rm-collapsible-body');
    body.appendChild(renderErrorBox(error));
    container.appendChild(body);
    return container;
  }

  function renderUnverifiedPaths(paths) {
    if (!paths || !paths.length) return null;

    var section = el('div');
    section.appendChild(txt('div', 'rm-section-title', LABELS.unverified));
    var tags = el('div', 'rm-tags');
    paths.forEach(function (up) {
      var tag = el('span', 'rm-tag');
      tag.textContent = up.path;
      if (up.reason) tag.title = up.reason;
      tags.appendChild(tag);
    });
    section.appendChild(tags);
    return section;
  }

  function renderFlowPage(flow) {
    var page = el('div');
    var direction = directionByID(flow.id);

    var header = el('div', 'rm-flow-header');
    header.appendChild(txt('h2', '', flow.name || flow.id));
    var detailFlowType = renderFlowTypePill(direction || flow);
    if (detailFlowType) header.appendChild(detailFlowType);
    if (!flow.error && !flow.evidence_only) {
      header.appendChild(renderEvidenceBadge(flow.confidence));
    } else if (!flow.error && direction) {
      header.appendChild(renderEvidenceBadge(direction.confidence, direction.local_verification));
    }
    page.appendChild(header);
    var directionContext = renderDirectionContext(direction);
    if (directionContext) page.appendChild(directionContext);

    if (flow.error) {
      page.appendChild(txt('div', 'rm-fallback-heading', msg('main.chrome.flow.explanation.failed.but.local.context.was.collected')));

      var filesSection = renderFileList(msg('main.flow.files_deterministic'), flow.bundle_files);
      if (filesSection) page.appendChild(filesSection);

      var testsSection = renderFileList(msg('main.flow.tests_deterministic'), flow.bundle_tests);
      if (testsSection) page.appendChild(testsSection);

      var docsSection = renderFileList(msg('main.flow.docs_selected'), flow.bundle_docs);
      if (docsSection) page.appendChild(docsSection);

      var pkgsSection = renderStringList(msg('main.flow.packages_selected'), flow.bundle_packages);
      if (pkgsSection) page.appendChild(pkgsSection);

      var edgesSection = renderEdgeList(msg('main.flow.related_import_edges'), flow.bundle_edges);
      if (edgesSection) page.appendChild(edgesSection);

      if (flow.bundle_summary) {
        page.appendChild(renderBundleStats(flow.bundle_summary));
      }

      page.appendChild(renderCollapsedError(flow.error));
      return page;
    }

    if (flow.evidence_only) {
      page.appendChild(txt(
        'div',
        'rm-method-note',
        flow.flow_status === 'local_only' ? LABELS.localEvidenceIntro : LABELS.localEvidenceLegacyIntro
      ));

      var modelAnchors = (flow.bundle_files || []).filter(function (file) {
        return file.reason === 'likely_file from candidate_flow';
      });
      var localNeighbors = (flow.bundle_files || []).filter(function (file) {
        return file.reason !== 'likely_file from candidate_flow';
      });
      var anchorsSection = renderBoundedFileList(msg('main.flow.orientation_anchors'), modelAnchors, 5);
      if (anchorsSection) page.appendChild(anchorsSection);
      var neighborsSection = renderBoundedFileList(msg('main.flow.related_local_files'), localNeighbors, 5);
      if (neighborsSection) page.appendChild(neighborsSection);

      var localTests = renderFileList(msg('main.flow.tests_selected_locally'), (flow.bundle_tests || []).slice(0, 8));
      if (localTests) page.appendChild(localTests);

      var localDocs = renderFileList(msg('main.flow.docs_selected_locally'), (flow.bundle_docs || []).slice(0, 8));
      if (localDocs) page.appendChild(localDocs);

      var localPackages = renderStringList(
        msg('main.flow.packages_focused_neighborhood'),
        (flow.bundle_packages || []).slice(0, 12)
      );
      if (localPackages) page.appendChild(localPackages);

      var localEdges = renderEdgeList(
        msg('main.flow.related_import_edges'),
        (flow.bundle_edges || []).slice(0, 12)
      );
      if (localEdges) page.appendChild(localEdges);

      if (flow.bundle_summary) page.appendChild(renderBundleStats(flow.bundle_summary));
      return page;
    }

    if (flow.summary) {
      page.appendChild(linkified('div', 'rm-summary', flow.summary));
    }

    if (flow.files_to_read_in_order && flow.files_to_read_in_order.length > 0) {
      page.appendChild(renderReadOrder(flow.files_to_read_in_order, 5));
    }

    if (flow.likely_chain && flow.likely_chain.length > 0) {
      var chainSection = renderChainSteps(flow.likely_chain, flow.files_to_read_in_order);
      if (chainSection.children.length > 1) {
        page.appendChild(chainSection);
      }
    }

    var testsSection = renderFileList(LABELS.testsToRead, flow.tests_to_read);
    if (testsSection) page.appendChild(testsSection);

    var unknownsSection = renderUnknowns(flow.unknowns);
    if (unknownsSection) page.appendChild(unknownsSection);

    var warningsSection = renderWarnings(flow.warnings);
    if (warningsSection) page.appendChild(warningsSection);

    var uvSection = renderUnverifiedPaths(flow.unverified_paths);
    if (uvSection) page.appendChild(uvSection);

    if (flow.bundle_summary) {
      page.appendChild(renderBundleStats(flow.bundle_summary));
    }

    return page;
  }

  function renderPlaceholder(text) {
    return txt('div', 'rm-placeholder', text);
  }

  // ── Tab management ──────────────────────────────────────────────

  function renderSavedTraceMenu(flows) {
    if (!flows || flows.length === 0) return null;
    var menu = el('details', 'rm-guided-flow-menu');
    var defaultLabel = msg('main.saved_trace_count', { count: flows.length });
    var summary = txt('summary', 'rm-guided-flow-summary', defaultLabel);
    summary.id = 'rm-guided-flow-summary';
    summary.setAttribute('data-default-label', defaultLabel);
    menu.appendChild(summary);
    var list = el('div', 'rm-guided-flow-list');
    flows.forEach(function (flow) {
      var label = flow.name || flow.id;
      if (flow.error) {
        label = msg('main.flow.analysis_unavailable', { name: label });
      }
      var option = txt('button', 'rm-guided-flow-option', label);
      option.type = 'button';
      option.setAttribute('data-flow-target', 'rm-flow-' + flow.id);
      option.setAttribute('data-flow-label', flow.name || flow.id);
      option.onclick = function () {
        menu.open = false;
        showTab('rm-flow-' + flow.id);
      };
      list.appendChild(option);
    });
    menu.appendChild(list);
    return menu;
  }

  function showTab(id) {
    document.querySelectorAll('.rm-tab, .rm-tab-content, .rm-guided-flow-option').forEach(function (e) {
      e.classList.remove('rm-active');
      e.removeAttribute('aria-current');
    });
    var content = document.getElementById(id);
    if (content) content.classList.add('rm-active');
    var tab = document.querySelector('.rm-tab[data-tab="' + id + '"]');
    if (tab) {
      tab.classList.add('rm-active');
      tab.setAttribute('aria-current', 'page');
    }
    var activeFlowLabel = '';
    document.querySelectorAll('.rm-guided-flow-option').forEach(function (option) {
      if (option.getAttribute('data-flow-target') !== id) return;
      option.classList.add('rm-active');
      option.setAttribute('aria-current', 'page');
      activeFlowLabel = option.getAttribute('data-flow-label') || '';
    });
    var flowSummary = document.getElementById('rm-guided-flow-summary');
    if (flowSummary) {
      flowSummary.textContent = activeFlowLabel
        ? msg('main.saved_trace_named', { label: activeFlowLabel })
        : flowSummary.getAttribute('data-default-label');
    }
  }

  // ── Main render ─────────────────────────────────────────────────

  function viewSectionID(view) {
		if (view === 'investigate') return 'rm-task-investigation';
    if (view === 'mechanisms') return 'rm-mechanisms';
    if (view === 'mechanism') return 'rm-mechanism-detail';
		if (view === 'study_overview') return 'rm-study-overview';
		if (view === 'study') return 'rm-study-detail';
		if (view === 'operate') return 'rm-operate-detail';
    if (view === 'architecture') return 'rm-architecture';
    if (view === 'provenance') return 'rm-provenance';
    return 'rm-overview';
  }

  function renderViewHeading(kicker, title, copy) {
    var heading = el('div', 'rm-view-heading');
    if (kicker) heading.appendChild(txt('div', 'rm-view-kicker', kicker));
    heading.appendChild(txt('h2', '', title));
    if (copy) heading.appendChild(txt('p', '', copy));
    return heading;
  }

  function mechanismPresentationTitle(mechanism) {
    mechanism = mechanism || {};
    var projected = String(mechanism.presentation_title || mechanism.presentationTitle || '').trim();
    if (projected) return projected.replace(/\?$/, '');
    var title = String(mechanism.title || '').trim();
    if (/^How\b/i.test(title)) return title.replace(/\?$/, '');
    var question = String(mechanism.question || '').trim();
    if (question) return question.replace(/\?$/, '');
    return title || msg('main.how_this_code_works');
  }

  function mechanismShortAnswer(mechanism) {
    var answer = String(mechanism && mechanism.answer || '').trim();
    if (/^Source-backed path\s*:/i.test(answer)) return '';
    return answer;
  }

  function workspaceHistoryPayload(state, sourceDrawer) {
    return {
      repomapWorkspace: true,
      mapReturn: state && state.mapReturn || null,
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
    var hash = options.hash || workspaceHashForState(next, !!options.mechanismRoot);
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
    var parsed = parseWorkspaceHash(window.location && window.location.hash, USER_MECHANISMS, historyState);
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

  function navigateWorkspace(view) {
    if (view === 'architecture') {
		if (!userArchitectureAvailable()) {
			commitWorkspaceState(emptyWorkspaceState());
			return;
		}
      openArchitectureTarget(null, null);
      return;
    }
    var next = reduceWorkspaceState(workspaceState, { type: 'view', view: view }, USER_MECHANISMS);
    commitWorkspaceState(next);
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

  function primaryUserMechanism() {
		var guideID = REPOSITORY_GUIDE && REPOSITORY_GUIDE.start_here_artifact_id || '';
		var guided = userMechanismByID(USER_MECHANISMS, guideID);
		if (guided) return guided;
    var thesis = DATA.repository_thesis || {};
    var recommended = userMechanismByID(USER_MECHANISMS, thesis.recommended_artifact_id || '');
    if (recommended) return recommended;
    for (var index = 0; index < USER_MECHANISMS.length; index++) {
      if (USER_MECHANISMS[index] && USER_MECHANISMS[index].role === 'primary_behavior') {
        return USER_MECHANISMS[index];
      }
    }
    return null;
  }

  function mechanismIsPrimary(mechanism) {
    var primary = primaryUserMechanism();
    return !!(primary && mechanism && primary.artifact_id === mechanism.artifact_id);
  }

  function mechanismRoleLabel(mechanism) {
		if (mechanismIsPrimary(mechanism)) return msg('main.chrome.main.code.path');
		if (mechanism && mechanism.role === 'extension_point') return msg('main.mechanism_role.extension');
		if (mechanism && mechanism.role === 'operational_support') return msg('main.mechanism_role.maintenance');
		if (mechanism && mechanism.target_user_job === 'question_driven_exploration') return msg('main.mechanism_role.question');
		return msg('main.mechanism_role.other');
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

	function guideMechanisms(ids) {
		var seen = {};
		return (Array.isArray(ids) ? ids : []).map(function (id) {
			return userMechanismByID(USER_MECHANISMS, id);
		}).filter(function (mechanism) {
			if (!mechanism || seen[mechanism.artifact_id]) return false;
			seen[mechanism.artifact_id] = true;
			return true;
		});
	}

	function renderGuideReadNext(mechanism, targets) {
		if (!mechanism || !Array.isArray(targets) || !targets.length) return null;
			var section = el('section', 'rm-workspace-section rm-read-next-section');
			section.appendChild(renderViewHeading(
				msg('main.read_next.kicker'),
				msg('main.read_next.title'),
				msg('main.read_next.copy')
			));
		var grid = el('div', 'rm-read-next-grid');
		targets.slice(0, 5).forEach(function (target) {
			if (!target || !target.path) return;
			var button = el('button', 'rm-read-next-target');
			button.type = 'button';
			button.appendChild(txt('strong', '', target.label || target.symbol || target.path));
			button.appendChild(txt('code', '', target.path + (target.line ? ':' + target.line : '')));
			button.appendChild(txt('span', '', msg('main.chrome.open.in.code.path')));
			button.onclick = function () {
				openUserMechanism(mechanism.artifact_id, Number(target.step_index) || 0, true);
			};
			grid.appendChild(button);
		});
		if (!grid.children || !grid.children.length) return null;
		section.appendChild(grid);
		return section;
	}

  function mechanismPrincipalFiles(mechanism) {
    var result = [];
    var seen = {};
    (mechanism && Array.isArray(mechanism.files) ? mechanism.files : []).forEach(function (location) {
      var path = String(location && location.path || '').trim();
      if (!path || seen[path] || result.length >= 4) return;
      seen[path] = true;
      result.push(path);
    });
    return result;
  }

  function renderUserMechanismCard(mechanism, statusLabel) {
    var card = el('button', 'rm-mechanism-card');
    if (mechanismIsPrimary(mechanism)) card.className += ' is-primary';
    card.type = 'button';
    card.appendChild(txt('span', 'rm-mechanism-card__label', statusLabel || mechanismRoleLabel(mechanism)));
    card.appendChild(txt('strong', '', mechanismPresentationTitle(mechanism)));
    var titleQuestion = mechanismPresentationTitle(mechanism).replace(/[?.!]+$/, '').toLowerCase();
    var question = String(mechanism.question || '').trim();
    if (question && question.replace(/[?.!]+$/, '').toLowerCase() !== titleQuestion) {
      card.appendChild(txt('p', 'rm-mechanism-card__question', question));
    }
    var answer = mechanismShortAnswer(mechanism);
    if (answer) card.appendChild(txt('p', 'rm-mechanism-card__answer', answer));

    var phases = mechanismNarrativeItems(mechanism).slice(0, 5);
    if (phases.length) {
      var phasePreview = el('span', 'rm-mechanism-card__phases');
      phases.forEach(function (phase, index) {
        phasePreview.appendChild(txt('span', '', (index + 1) + '. ' + (phase.title || msg('main.implementation'))));
      });
      card.appendChild(phasePreview);
    }

    var principalFiles = mechanismPrincipalFiles(mechanism);
    if (principalFiles.length) {
      var files = el('span', 'rm-mechanism-card__files');
      principalFiles.forEach(function (path) { files.appendChild(txt('code', '', path)); });
      card.appendChild(files);
    }
    card.appendChild(txt('span', 'rm-mechanism-card__action', msg('main.open.code.path')));
    card.onclick = function () { openUserMechanism(mechanism.artifact_id, 0); };
    return card;
  }

	function renderTopicCard(topic) {
		var card = el('button', 'rm-mechanism-card');
		card.type = 'button';
		card.appendChild(txt('span', 'rm-mechanism-card__label', msg('main.topic.incomplete')));
		card.appendChild(txt('strong', '', topic.title || topic.question || msg('main.question.worth.exploring')));
		if (topic.question) card.appendChild(txt('p', 'rm-mechanism-card__question', topic.question));
		var symbols = Array.isArray(topic.starting_symbols) ? topic.starting_symbols.slice(0, 4) : [];
		if (symbols.length) {
			var files = el('span', 'rm-mechanism-card__files');
			symbols.forEach(function (location) {
				files.appendChild(txt('code', '', String(location.path || '') + ' · ' + String(location.symbol || '')));
			});
			card.appendChild(files);
		}
		if (topic.uncertainty) {
			card.appendChild(txt('p', 'rm-mechanism-card__answer', topic.uncertainty));
		}
		card.appendChild(txt('span', 'rm-mechanism-card__action', msg('main.inspect.starting.points')));
		card.onclick = function () {
			activeOverviewTopicID = topic.candidate_id;
			renderOverviewWorkspace();
		};
		return card;
	}

	function renderTopicDetail(root, topic) {
		var back = txt('button', 'rm-secondary-action', msg('main.all.paths'));
		back.type = 'button';
		back.onclick = function () {
			activeOverviewTopicID = '';
			renderOverviewWorkspace();
		};
		root.appendChild(back);

		var hero = el('section', 'rm-overview-hero rm-purpose-hero');
		hero.appendChild(txt('div', 'rm-view-kicker', msg('main.topic.incomplete')));
		hero.appendChild(txt('h2', '', topic.title || topic.question || msg('main.question.worth.exploring')));
		if (topic.question) hero.appendChild(txt('p', '', topic.question));
		root.appendChild(hero);

		if (topic.uncertainty) {
				var uncertainty = el('section', 'rm-workspace-section');
				uncertainty.appendChild(renderViewHeading(
					msg('main.topic.current_boundary'),
					msg('main.topic.why_not_mechanism'),
					topic.uncertainty
				));
			root.appendChild(uncertainty);
		}

		var locations = Array.isArray(topic.starting_symbols) ? topic.starting_symbols.slice(0, 4) : [];
		if (!locations.length) return;
			var section = el('section', 'rm-workspace-section');
			section.appendChild(renderViewHeading(
				msg('main.exact.places.to.start'),
				msg('main.topic.continue_repository'),
				msg('main.topic.grounded_starting_points_copy')
			));
		var grid = el('div', 'rm-read-next-grid');
		locations.forEach(function (location) {
			if (!location || !location.path || !location.symbol) return;
			if (repositoryLocationAvailable(location)) {
				var remoteTarget = staticSourceLink('', 'rm-read-next-target rm-source-target-link', location);
				if (remoteTarget) {
					remoteTarget.appendChild(txt('strong', '', location.symbol));
					remoteTarget.appendChild(txt('code', '', formatCodeLocation(location)));
					remoteTarget.appendChild(txt('span', '', msg('main.open.exact.symbol')));
					grid.appendChild(remoteTarget);
				} else {
					var button = el('button', 'rm-read-next-target');
					button.type = 'button';
					button.appendChild(txt('strong', '', location.symbol));
					button.appendChild(txt('code', '', formatCodeLocation(location)));
					button.appendChild(txt('span', '', msg('main.open.exact.symbol')));
					button.onclick = function () { openSourceLocation(location); };
					grid.appendChild(button);
				}
				return;
			}
			var exact = el('div', 'rm-read-next-target');
			exact.appendChild(txt('strong', '', location.symbol));
			exact.appendChild(txt('code', '', formatCodeLocation(location)));
			grid.appendChild(exact);
		});
		if (grid.children && grid.children.length) {
			section.appendChild(grid);
			root.appendChild(section);
		}
	}

	function appendIncompleteStudyEntry(root) {
		if (!INCOMPLETE_STUDY_DIRECTIONS.length) return;
		var section = el('section', 'rm-workspace-section rm-study-map-section');
		section.appendChild(renderViewHeading(
			msg('main.study'),
			msg('main.explore.more.grounded.questions'),
			msg('main.each.question.has.an.exact.place.to.begin.but.not.yet.a.complete.reading.path')
		));
		var open = txt(
			'button',
			'rm-secondary-action',
			msg('main.study.review_incomplete', { count: INCOMPLETE_STUDY_DIRECTIONS.length })
		);
		open.type = 'button';
		open.onclick = function () { navigateWorkspace('study_overview'); };
		section.appendChild(open);
		root.appendChild(section);
	}

	function renderMixedLearningShelf(root) {
		if (!mixedShelfAvailable()) return false;
		var activeTopic = userTopicByID(activeOverviewTopicID);
		if (activeTopic) {
			renderTopicDetail(root, activeTopic);
			return true;
		}

		var hero = el('section', 'rm-overview-hero rm-purpose-hero');
		hero.appendChild(txt('div', 'rm-view-kicker', msg('main.understand.the.repository')));
		hero.appendChild(txt('h2', '', msg('main.pick.a.path.worth.following')));
		hero.appendChild(txt(
			'p',
			'',
			msg('main.complete.mechanisms.explain.a.source.backed.path.topics.are.honest.starting.points.when.the.evidence.is.useful.but.the.path.is.not.closed.yet')
		));
		root.appendChild(hero);

		if (USER_MECHANISMS.length) {
				var mechanismSection = el('section', 'rm-workspace-section rm-primary-path-section');
				mechanismSection.appendChild(renderViewHeading(
					msg('main.mechanism.complete'),
					msg('main.mechanism.source_backed_paths'),
					msg('main.mechanism.open_complete_copy')
				));
			var mechanismGrid = el('div', 'rm-mechanism-grid');
			USER_MECHANISMS.slice(0, 4).forEach(function (mechanism) {
				var stepCount = mechanismNarrativeItems(mechanism).length;
					mechanismGrid.appendChild(renderUserMechanismCard(
						mechanism,
						msg('main.mechanism.full_status', { count: stepCount })
					));
			});
			mechanismSection.appendChild(mechanismGrid);
			root.appendChild(mechanismSection);
		}

		if (USER_TOPICS.length) {
			var topicSection = el('section', 'rm-workspace-section');
			topicSection.appendChild(renderViewHeading(
				msg('main.questions.worth.exploring'),
				msg('main.grounded.starting.points'),
				msg('main.each.topic.shows.what.is.known.where.to.start.and.why.no.complete.path.is.claimed')
			));
			var topicGrid = el('div', 'rm-mechanism-grid');
			USER_TOPICS.slice(0, 3).forEach(function (topic) {
				topicGrid.appendChild(renderTopicCard(topic));
			});
			topicSection.appendChild(topicGrid);
			root.appendChild(topicSection);
		}
		appendIncompleteStudyEntry(root);
		return true;
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

  function appendMechanismGrid(root, mechanisms) {
    if (!mechanisms.length) return;
    var grid = el('div', 'rm-mechanism-grid');
    mechanisms.forEach(function (mechanism) { grid.appendChild(renderUserMechanismCard(mechanism)); });
    root.appendChild(grid);
  }

	function openStudyDirection(directionID) {
		var direction = studyDirectionByID(directionID);
		if (!direction) {
			commitWorkspaceState(emptyWorkspaceState());
			return;
		}
		if (direction.mechanism_id && userMechanismByID(USER_MECHANISMS, direction.mechanism_id)) {
			openUserMechanism(direction.mechanism_id, 0, false);
			return;
		}
		var next = reduceWorkspaceState(workspaceState, {
			type: 'open_study', directionID: direction.id,
		}, USER_MECHANISMS);
		commitWorkspaceState(next);
	}

	function openThemeCard(ordinal) {
		var next = reduceWorkspaceState(workspaceState, {
			type: 'open_study_theme', ordinal: Number(ordinal) || 0,
		}, USER_MECHANISMS);
		commitWorkspaceState(next);
	}

	function openPavedPath(operationID) {
		if (!pavedPathByID(operationID)) {
			commitWorkspaceState(emptyWorkspaceState());
			return;
		}
		var next = reduceWorkspaceState(workspaceState, {
			type: 'open_operation', operationID: operationID,
		}, USER_MECHANISMS);
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
		var card = el('article', 'rm-study-direction-card');
		card.appendChild(txt('span', 'rm-study-direction-card__order', String(index + 1)));
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
			'button',
			'rm-study-direction-card__title',
			direction.question || msg('main.chrome.explore.this.code.area')
		);
		title.type = 'button';
		title.onclick = function () { openStudyDirection(direction.id); };
		body.appendChild(title);
		if (direction.why_it_matters) body.appendChild(txt('span', 'rm-study-direction-card__reason', direction.why_it_matters));
		if (direction.learning_outcome) body.appendChild(txt('span', 'rm-study-direction-card__outcome', direction.learning_outcome));
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
		var root = document.getElementById('rm-study-overview');
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
	// direct role, and "partial" otherwise. Scope and evidence are
	// independent axes — "Source-backed" is never contradicted by "Scope
	// partial", and a narrow-but-exact theme is not failed evidence.
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
	};
	function themeKindLabel(card) {
		var kind = String((card && card.theme_kind) || '');
		return msg(THEME_KIND_MESSAGE_IDS[kind] || 'main.study.theme.kind.learning');
	}

	// themeReadingPreviewRows collapses the reading previews of a collapsed
	// card: at most two preview rows (Decision 229 D6), with repeated public
	// symbols grouped visually ("HttpEmailProvider.Send · 2 callsites")
	// while every exact source location stays in the expanded detail.
	function themeReadingPreviewRows(card) {
		var readings = card && Array.isArray(card.readings) ? card.readings : [];
		var rows = [];
		var bySymbol = {};
		var ordered = [];
		readings.forEach(function (reading) {
			if (!reading || !reading.path || !Number.isInteger(reading.line)) return;
			var symbol = String(reading.symbol || '');
			if (!bySymbol[symbol]) {
				bySymbol[symbol] = { symbol: symbol, count: 0, first: reading };
				ordered.push(bySymbol[symbol]);
			}
			bySymbol[symbol].count += 1;
		});
		ordered.forEach(function (group) {
			if (rows.length >= 2) return;
			rows.push(group);
		});
		return rows;
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

	function renderAtlasStudyThemeShelf(root, cards) {
		root.appendChild(renderViewHeading(
			msg('main.study'),
			msg('main.study.themes.title'),
			msg('main.study.themes.copy')
		));
		// Decision 218 (A): the dedicated Study route renders every published
		// theme — no "show more" for peer theme existence. Progressive
		// disclosure may collapse card detail, reading previews, and
		// provenance, never the themes themselves.
		var shelf = el('div', 'rm-study-theme-shelf');
		cards.forEach(function (card, index) {
			shelf.appendChild(renderThemeCard(card, index));
		});
		root.appendChild(shelf);
		// Raw span/model/frontier diagnostics and the machine question
		// inventory move into a collapsed "Coverage and provenance" section
		// (Decision 217): available for the meticulous critic, out of the
		// main learning path.
		var provenance = el('details', 'rm-study-provenance');
		provenance.appendChild(txt('summary', 'rm-study-provenance__summary', msg('main.study.provenance.title')));
		var diagnostics = renderAtlasStudyDiagnostics();
		if (diagnostics) provenance.appendChild(diagnostics);
		var browse = renderAtlasStudyBrowse();
		if (browse) provenance.appendChild(browse);
		root.appendChild(provenance);
	}

	function renderThemeCard(card, index) {
		var article = el('article', 'rm-study-theme-card' + (card.badge ? ' rm-study-theme-card--' + String(card.badge).toLowerCase() : ''));
		var titleRow = el('div', 'rm-study-theme-card__title-row');
		var title = txt('button', 'rm-study-theme-card__title', card.final_title || '');
		title.type = 'button';
		title.onclick = function () { openThemeCard(Number(card.ordinal) || (index + 1)); };
		titleRow.appendChild(title);
		// Decision 229 D6: evidence and scope are independent axes. The
		// evidence badge shows source-backed vs partial coverage; the scope
		// badge shows exact vs partial thematic scope. A narrow-but-exact
		// theme is not a warning.
		titleRow.appendChild(txt('span', 'rm-study-theme-card__badge rm-study-theme-card__evidence rm-study-theme-card__evidence--' + themeCoverageState(card), themeCoverageLabel(card)));
		titleRow.appendChild(txt('span', 'rm-study-theme-card__badge rm-study-theme-card__scope rm-study-theme-card__scope--' + themeScopeState(card), themeScopeLabel(card)));
		titleRow.appendChild(txt('span', 'rm-study-theme-card__kind', themeKindLabel(card)));
		article.appendChild(titleRow);
		if (card.final_question) article.appendChild(txt('p', 'rm-study-theme-card__question', card.final_question));
		if (card.why_it_matters) article.appendChild(txt('span', 'rm-study-theme-card__reason', card.why_it_matters));
		// Decision 229 D6: collapsed by default — at most two reading
		// previews; the complete reading plan lives in the expanded detail
		// (renderThemeDetailWorkspace), never hidden, never truncated.
		var previewRows = themeReadingPreviewRows(card);
		var totalReadings = Array.isArray(card.readings) ? card.readings.length : 0;
		if (previewRows.length) {
			var previews = el('ul', 'rm-study-theme-card__previews');
			previewRows.forEach(function (group) {
				var item = txt('li', 'rm-study-theme-card__preview', '');
				var firstLocation = studyReadingLocation(group.first);
				// Decision 230 D1: each preview symbol+path is ONE
				// independent source action (never a link plus inert
				// location, never an unexplained inert value). When no
				// source action is available the preview renders as a
				// neutral non-link row with a visible reason.
				var action = renderStudySourceAction(group.first, 'rm-study-theme-card__preview-action', true);
				if (action) {
					item.appendChild(action);
				} else if (firstLocation) {
					var unavailable = el('div', 'rm-study-theme-card__preview-action rm-study-theme-card__preview-action--unavailable');
					unavailable.appendChild(txt('span', 'rm-study-theme-card__preview-symbol', bareSourceSymbol(group.symbol)));
					unavailable.appendChild(txt('code', 'rm-study-theme-card__preview-location', formatCodeLocation(firstLocation)));
					unavailable.appendChild(txt('span', 'rm-study-theme-card__preview-unavailable', msg('main.study.theme.source_unavailable')));
					item.appendChild(unavailable);
				}
				var callsiteCount = group.count > 1 ? msg('main.study.theme.callsite_count', { count: group.count }) : '';
				if (callsiteCount) item.appendChild(txt('span', 'rm-study-theme-card__preview-count', callsiteCount));
				previews.appendChild(item);
			});
			article.appendChild(previews);
		}
		if (totalReadings > previewRows.length) {
			article.appendChild(txt('p', 'rm-study-theme-card__more',
				msg('main.study.theme.more_readings', { count: totalReadings - previewRows.length })));
		}
		return article;
	}

	function renderThemeDetailWorkspace() {
		var root = document.getElementById('rm-study-detail');
		if (!root) return;
		root.replaceChildren();
		var card = themeCardByOrdinal(workspaceState.themeCardOrdinal);
		if (!card) return;
		var back = txt('button', 'rm-secondary-action rm-study-back', msg('main.all.study.directions'));
		back.type = 'button';
		back.onclick = function () { navigateWorkspace('study_overview'); };
		root.appendChild(back);
		root.appendChild(renderViewHeading(
			themeCoverageLabel(card),
			card.final_title || '',
			card.final_question || ''
		));
		// Decision 229 D6: evidence and scope are independent badges in the
		// detail header too — a source-backed theme with partial scope is
		// not failed evidence.
		var badgeRow = el('div', 'rm-study-theme-card__badge-row');
		badgeRow.appendChild(txt('span', 'rm-study-theme-card__badge rm-study-theme-card__evidence rm-study-theme-card__evidence--' + themeCoverageState(card), themeCoverageLabel(card)));
		badgeRow.appendChild(txt('span', 'rm-study-theme-card__badge rm-study-theme-card__scope rm-study-theme-card__scope--' + themeScopeState(card), themeScopeLabel(card)));
		badgeRow.appendChild(txt('span', 'rm-study-theme-card__kind', themeKindLabel(card)));
		root.appendChild(badgeRow);
		root.appendChild(txt('p', 'rm-study-theme-card__coverage-explanation', themeCoverageExplanation(card)));
		// Decision 229 D6: expected learning is its own section in the
		// expanded card (never hidden behind hover).
		if (card.expected_learning) {
			root.appendChild(txt('p', 'rm-study-theme-card__expected-learning',
				msg('main.study.theme.expected_learning', { learning: card.expected_learning })));
		}
		if (card.why_it_matters) {
			var reason = el('aside', 'rm-study-outcome');
			reason.appendChild(txt('strong', '', card.why_it_matters));
			root.appendChild(reason);
		}
		var anchors = el('div', 'rm-study-reading-list');
		(card.readings || []).forEach(function (reading, index) {
			var item = renderStudyReadingAnchor(reading, index);
			if (item) anchors.appendChild(item);
		});
		root.appendChild(anchors);
		// Decision 229 D6: limitations and provenance under a secondary
		// disclosure — present, never hover-only, out of the primary path.
		var limitation = String(card.limitation || '');
		if (limitation && limitation !== card.expected_learning) {
			var details = el('details', 'rm-study-theme-card__limitations');
			details.appendChild(txt('summary', 'rm-study-theme-card__limitations-summary', msg('main.study.theme.limitations')));
			details.appendChild(txt('p', 'rm-study-theme-card__limitations-body', limitation));
			root.appendChild(details);
		}
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
		root.appendChild(txt('p', 'rm-study-failed-banner rm-warning', msg('main.study.frontier.failed_banner')));
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

		var studyDiagnostics = renderAtlasStudyDiagnostics();
		if (studyDiagnostics) root.appendChild(studyDiagnostics);

		var frontierBrowse = renderAtlasStudyBrowse();
		if (frontierBrowse) root.appendChild(frontierBrowse);

		renderOperationsOverview(root);

		if (USER_MECHANISMS.length) {
			var deepDiveSection = el('section', 'rm-workspace-section');
			deepDiveSection.appendChild(renderViewHeading(msg('main.ready.deep.dives'), msg('main.source.backed.code.paths'), msg('main.these.directions.already.have.a.validated.step.by.step.implementation.explanation')));
			appendMechanismGrid(deepDiveSection, USER_MECHANISMS.slice(0, 6));
			root.appendChild(deepDiveSection);
		}

	}

	function renderStudyDetailWorkspace() {
		var root = document.getElementById('rm-study-detail');
		if (!root) return;
		root.replaceChildren();
		var direction = studyDirectionByID(workspaceState.directionID);
		if (!direction) return;
		var back = txt('button', 'rm-secondary-action rm-study-back', msg('main.all.study.directions'));
		back.type = 'button';
		back.onclick = function () { navigateWorkspace('study_overview'); };
		root.appendChild(back);
		root.appendChild(renderViewHeading(
				direction.incomplete ? msg('main.study.incomplete_direction') : '',
				direction.question,
				direction.why_it_matters
		));
		if (direction.learning_outcome) {
			var outcome = el('aside', 'rm-study-outcome');
			outcome.appendChild(txt('strong', '', direction.learning_outcome));
			root.appendChild(outcome);
		}
		var anchors = el('div', 'rm-study-reading-list');
		(direction.reading_anchors || []).forEach(function (reading, index) {
			var card = renderStudyReadingAnchor(reading, index);
			if (card) anchors.appendChild(card);
		});
		root.appendChild(anchors);
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
			receiver = value.slice(0, value.indexOf(')') + 1);
			last = value.slice(value.indexOf(')') + 1);
		}
		if (last) {
			var dot = last.lastIndexOf('.');
			if (dot >= 0 && dot + 1 < last.length) last = last.slice(dot + 1);
		}
		return (receiver ? receiver + '.' : '') + last;
	}

	function renderStudyReadingAnchor(reading, index) {
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
		if (reading.role === 'direct') meta.appendChild(txt('span', 'rm-study-theme-card__reading-role', msg('main.study.theme.reading.role.direct')));
		else if (reading.role === 'supporting') meta.appendChild(txt('span', 'rm-study-theme-card__reading-role', msg('main.study.theme.reading.role.supporting')));
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
		back.onclick = function () { navigateWorkspace('overview'); };
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
	// user-facing classes derived from existing fields only (Decision 217):
	// primary product entry, secondary service, tooling/CLI, library API.
	// Duplicate "HTTP server" cards with no behavioral distinction are
	// coalesced into one classified group.
	function overviewEntryClassifications(objects) {
		var groups = [];
		var byClass = {};
		var push = function (key, labelMessageID, items) {
			if (!items.length) return;
			byClass[key] = { key: key, labelMessageID: labelMessageID, items: items };
			groups.push(byClass[key]);
		};
		var primary = [], service = [], tooling = [], library = [], other = [];
		objects.forEach(function (object) {
			var kind = object.entryKind;
			var role = object.entryRole;
			if (kind === 'process_entry' && role === 'primary_application') { primary.push(object); return; }
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
		push('other', 'main.overview.entry.other', other);
		return groups;
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

	function componentEvidenceTierMessageID(tier) {
		return EVIDENCE_TIER_MESSAGE_IDS[tier] || 'main.evidence.tier.hypothesis';
	}

	function packageParticipantCount(componentID) {
		if (!ARCHITECTURE_COMPONENT_NAVIGATION || Number(ARCHITECTURE_COMPONENT_NAVIGATION.version) !== 1) return 0;
		var entry = (ARCHITECTURE_COMPONENT_NAVIGATION.components || []).filter(function (candidate) {
			return candidate && candidate.component_id === componentID;
		})[0];
		return entry && Array.isArray(entry.package_participant_ids) ? entry.package_participant_ids.length : 0;
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
		var components = (Array.isArray(canvas.components) ? canvas.components : []).filter(function (component) {
			if (!component) return false;
			if (remainderComponentID && component.id === remainderComponentID) return false;
			if (diagnosticSubsystemIDs[component.subsystem_id]) return false;
			return true;
		});
		var remainderComponents = (Array.isArray(canvas.components) ? canvas.components : []).filter(function (component) {
			if (!component) return false;
			if (remainderComponentID && component.id === remainderComponentID) return true;
			return !!diagnosticSubsystemIDs[component.subsystem_id];
		});
		var remainderPackageCount = 0;
		remainderComponents.forEach(function (component) {
			remainderPackageCount += Array.isArray(component.members) ? component.members.length : 0;
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
			remainderPackageCount: remainderPackageCount,
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
	// Priority: usable Navigator startup action with exact evidence, primary
	// production process entry, library constructor/start/use entry, a core
	// Study theme's first exact reading, then an explicit unavailable state.
	function overviewFirstAction(anatomy, atlasShelf) {
		// 1. Navigator startup action — backend-owned, already validated
		// against the exact persisted Repository Atlas. Only usable when the
		// recommended surface is a production entry, not test/helper tooling
		// (Decision 221 C: production entries rank before tests/tooling).
		if (NAVIGATOR && NAVIGATOR.state === 'selected' && NAVIGATOR.recommendation &&
			atlasShelf && atlasShelf.recommendation) {
			var nav = atlasShelf.recommendation;
			var navEntry = overviewSurfaceObjectForTriggerId(nav && nav.location);
			var navUsable = !navEntry || (navEntry.entryRole !== 'test_or_helper' &&
				navEntry.entryRole !== 'tooling');
			if (navUsable && nav && nav.snippet && nav.location) {
				return {
					label: msg('main.overview.first_action.navigator'),
					path: nav.location.path,
					line: nav.location.line,
					symbol: nav.unit && nav.unit.name || '',
					reason: msg('main.overview.first_action.reason.navigator'),
					action: function () {
						openSourceSnippet(nav.snippet, nav.location, false, { drawerFirst: true });
					},
					authority: msg('main.overview.first_action.authority.navigator'),
					kind: 'navigator',
				};
			}
		}
		// 2. Primary production process entry (not tests/tooling).
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
		// 3. Library constructor/start/use entry.
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

	// overviewSurfaceObjectForTriggerId finds the raw discovered-surface
	// trigger whose registration site matches the given path/line. Navigator
	// atlas entity ids differ from trigger ids, so identity is matched by
	// exact location; test/helper triggers are excluded from Overview cards
	// but must still be found here to judge recommendation usability.
	function overviewSurfaceObjectForTriggerId(navLocation) {
		if (!navLocation || !navLocation.path) return null;
		var triggers = DATA.discovered_surfaces && Array.isArray(DATA.discovered_surfaces.triggers)
			? DATA.discovered_surfaces.triggers : [];
		for (var index = 0; index < triggers.length; index++) {
			var trigger = triggers[index];
			var site = trigger && (trigger.registration_site || trigger.location);
			if (site && site.path === navLocation.path && site.line === navLocation.line) {
				return {
					id: trigger.id,
					entryKind: String(trigger.kind || ''),
					entryRole: String(trigger.executable_role || ''),
				};
			}
		}
		return null;
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
					groupBlock.appendChild(txt('h4', 'rm-overview-entry-group__label', msg(group.labelMessageID)));
					var grid = el('div', 'rm-overview-object-grid');
					group.items.forEach(function (object) {
						grid.appendChild(renderOverviewObjectCard(object, 'surface'));
					});
					groupBlock.appendChild(grid);
					// Decision 221 B: production entries (primary product
					// entry, service entry) stay visible; the complete
					// taxonomy (tooling, library, other) remains accessible
					// under a collapsed disclosure — never hidden.
					if (group.key === 'primary' || group.key === 'service') {
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
					DATA.architecture_synthesis && DATA.architecture_synthesis.state === 'failed' ?
						msg('main.overview.anatomy.components_copy_synthesis_failed') :
						msg('main.overview.anatomy.components_copy'),
					anatomy.components,
					'component'
				));
			}
		}

		// Decision 229 D3: unclassified exact scope (diagnostic remainder,
		// e.g. "Supporting repository evidence") is a collapsed disclosure —
		// never a principal product area. Count-reconciled, always reachable.
		var remainderCount = anatomy.components && anatomy.components.remainderPackageCount || 0;
		if (anatomy.components && anatomy.components.hasDiagnosticRemainder && remainderCount > 0) {
			var remainderSection = el('section', 'rm-workspace-section rm-overview-remainder');
			var remainderDetails = el('details', 'rm-overview-remainder__disclosure');
			remainderSection.appendChild(remainderDetails);
			remainderDetails.appendChild(txt('summary', 'rm-overview-remainder__summary',
				msg('main.overview.remainder.summary', { count: remainderCount })));
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
			root.appendChild(txt('p', 'rm-overview-zero-handlers rm-warning', copy));
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
			recommendation: repositoryAtlasNavigatorRecommendation(relations),
		};
	}

	function exactStringArraysEqual(left, right) {
		if (!Array.isArray(left) || !Array.isArray(right) || left.length !== right.length) return false;
		for (var index = 0; index < left.length; index++) {
			if (String(left[index] || '') !== String(right[index] || '')) return false;
		}
		return true;
	}

	function repositoryAtlasNavigatorRecommendation(relations) {
		if (!ATLAS_FIRST || NAVIGATOR.state !== 'selected' || !NAVIGATOR.recommendation) return null;
		var selected = NAVIGATOR.recommendation;
		if (!selected.surface || selected.surface.kind !== 'surface' ||
			!selected.application_operation || selected.application_operation.kind !== 'operation' ||
			typeof selected.relation_id !== 'string' || !selected.relation_id ||
			!Array.isArray(selected.evidence_ids) || !selected.evidence_ids.length) return null;
		var matches = relations.filter(function (relation) {
			return relation.id === selected.relation_id &&
				relation.surfaceID === selected.surface.id &&
				relation.applicationID === selected.application_operation.id &&
				exactStringArraysEqual(relation.evidenceIDs, selected.evidence_ids);
		});
		return matches.length === 1 ? matches[0] : null;
	}

	function renderRepositoryAtlasNavigatorRecommendation(shelf) {
		var section = el('section', 'rm-atlas-recommendation');
		section.appendChild(txt('div', 'rm-view-kicker', msg('main.atlas.navigator.kicker')));
		section.appendChild(txt('h3', '', msg(
			NAVIGATOR && NAVIGATOR.state === 'unavailable'
				? 'main.atlas.navigator.unavailable_title'
				: 'main.atlas.navigator.title'
		)));
		if (NAVIGATOR && NAVIGATOR.state === 'empty') {
			section.appendChild(txt('p', 'rm-empty-state', msg('main.atlas.navigator.empty')));
			return section;
		}
		if (NAVIGATOR && NAVIGATOR.state === 'unavailable') {
			section.appendChild(txt('p', 'rm-empty-state', msg(
				NAVIGATOR.unavailable_code === 'offline'
					? 'main.atlas.navigator.unavailable_offline'
					: 'main.atlas.navigator.unavailable'
			)));
			return section;
		}
		var recommendation = shelf && shelf.recommendation;
		if (!recommendation) {
			section.appendChild(txt('p', 'rm-empty-state', msg('main.atlas.navigator.source_unavailable')));
			return section;
		}
		section.appendChild(txt('p', '', msg('main.atlas.navigator.copy')));
		section.appendChild(txt('strong', 'rm-atlas-relation-unit', recommendation.unit.name));
		var roles = el('div', 'rm-atlas-relation-roles');
		roles.appendChild(txt('span', '', msg('main.atlas.workspace.process_entry')));
		roles.appendChild(txt('span', '', msg('main.atlas.workspace.application_start')));
		section.appendChild(roles);
		section.appendChild(txt('span', 'rm-atlas-evidence-count', msg(
			'main.atlas.navigator.evidence_count',
			{ count: recommendation.evidenceIDs.length }
		)));
		var sourceButton = txt(
			'button',
			'rm-primary-action rm-atlas-navigator-action',
			msg('main.atlas.navigator.inspect_source')
		);
		sourceButton.type = 'button';
		sourceButton.onclick = function () {
			openSourceSnippet(recommendation.snippet, recommendation.location, false, { drawerFirst: true });
		};
		section.appendChild(sourceButton);
		return section;
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

	function repositoryAtlasNavigatorCompactStatus(shelf) {
		if (!ATLAS_FIRST || shelf.recommendation) return null;
		if (NAVIGATOR && NAVIGATOR.state === 'empty') {
			return { code: 'empty', messageID: 'main.atlas.navigator.empty' };
		}
		if (NAVIGATOR && NAVIGATOR.state === 'unavailable') {
			return {
				code: NAVIGATOR.unavailable_code || 'unavailable',
				messageID: NAVIGATOR.unavailable_code === 'offline'
					? 'main.atlas.navigator.unavailable_offline'
					: 'main.atlas.navigator.unavailable',
			};
		}
		return { code: 'source_unavailable', messageID: 'main.atlas.navigator.source_unavailable' };
	}

	function renderRepositoryAtlasCompactDiagnostics(shelf) {
		var navigatorStatus = repositoryAtlasNavigatorCompactStatus(shelf);
		var startupUnavailable = !shelf.relations.length;
		if (!navigatorStatus && !startupUnavailable) return null;
		var diagnostics = el('section', 'rm-atlas-compact-status');
		diagnostics.appendChild(txt('strong', 'rm-atlas-compact-status__title', msg(
			'main.atlas.workspace.compact_status'
		)));
		var list = el('ul', 'rm-atlas-compact-status__list');
		if (navigatorStatus) {
			var navigatorItem = el('li', 'rm-atlas-compact-status__item');
			navigatorItem.setAttribute('data-rm-status-code', navigatorStatus.code);
			navigatorItem.appendChild(txt('span', 'rm-atlas-compact-status__label', msg(
				'main.atlas.navigator.kicker'
			)));
			navigatorItem.appendChild(txt('span', '', msg(navigatorStatus.messageID)));
			list.appendChild(navigatorItem);
		}
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

		if (!compactOverview && ATLAS_FIRST && shelf.recommendation) {
			section.appendChild(renderRepositoryAtlasNavigatorRecommendation(shelf));
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
		} else if (NAVIGATOR && NAVIGATOR.state === 'unavailable') {
			section.appendChild(txt(
				'p',
				'rm-atlas-overview-status',
				msg(NAVIGATOR.unavailable_code === 'offline'
					? 'main.atlas.navigator.unavailable_offline'
					: 'main.atlas.navigator.unavailable')
			));
		}
		root.appendChild(section);
	}

	function renderOverviewWorkspace() {
		var root = document.getElementById('rm-overview');
		if (!root) return;
		root.replaceChildren();
		renderStudyPublicationNotice(root);
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
		if (renderMixedLearningShelf(root)) return;
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

    var primary = primaryUserMechanism();
    if (primary) {
      var primarySection = el('section', 'rm-workspace-section rm-primary-path-section');
      primarySection.appendChild(renderViewHeading(msg('main.chrome.primary.path'), msg('main.chrome.start.with.the.main.behavior'), msg('main.chrome.read.one.source.backed.path.before.exploring.the.rest.of.the.repository')));
      appendMechanismGrid(primarySection, [primary]);
      root.appendChild(primarySection);
    }

		var extension = REPOSITORY_GUIDE
			? guideMechanisms(REPOSITORY_GUIDE.extension_artifact_ids)
			: USER_MECHANISMS.filter(function (mechanism) { return mechanism.role === 'extension_point'; });
		if (extension.length) {
			var extensionSection = el('section', 'rm-workspace-section');
			extensionSection.appendChild(renderViewHeading(msg('main.chrome.extension.paths'), msg('main.chrome.where.behavior.plugs.in'), msg('main.chrome.follow.an.accepted.code.path.to.a.registration.factory.adapter.or.boundary')));
			appendMechanismGrid(extensionSection, extension.slice(0, 3));
			root.appendChild(extensionSection);
		}

		var secondary = REPOSITORY_GUIDE
			? guideMechanisms(REPOSITORY_GUIDE.more_path_artifact_ids)
			: USER_MECHANISMS.filter(function (mechanism) {
				return (!primary || mechanism.artifact_id !== primary.artifact_id) && mechanism.role !== 'extension_point';
			});
		if (secondary.length) {
      var secondarySection = el('section', 'rm-workspace-section');
      secondarySection.appendChild(renderViewHeading(msg('main.chrome.other.paths'), msg('main.chrome.more.behavior.to.explore'), msg('main.chrome.continue.with.another.source.backed.explanation.when.it.matches.what.you.need')));
      appendMechanismGrid(secondarySection, secondary.slice(0, 4));
      root.appendChild(secondarySection);
    }

		var readNext = renderGuideReadNext(primary, REPOSITORY_GUIDE && REPOSITORY_GUIDE.read_next || primary && primary.read_next || []);
		if (readNext) root.appendChild(readNext);

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

	function renderStudyPublicationNotice(root) {
		if (!root || COMPLETE_STUDY_DIRECTIONS.length || !STUDY_PUBLICATION ||
			STUDY_PUBLICATION.state === 'published') return;
		var notice = el('section', 'rm-study-publication-notice');
		notice.appendChild(txt('h2', '', msg('main.study.unavailable.for.this.run')));
		notice.appendChild(txt(
			'p',
			'',
			STUDY_PUBLICATION.state === 'started'
				? msg('main.study.stage.did.not.finish.so.no.study.directions.were.published.the.overview.below.uses.independently.accepted.inputs.it.is.not.a.substitute.study.result')
				: msg('main.no.study.directions.were.published.because.the.editing.stage.did.not.pass.its.required.checks.the.overview.below.uses.independently.accepted.inputs.it.is.not.a.substitute.study.result')
		));
		root.appendChild(notice);
	}

  function renderMechanismsWorkspace() {
    var root = document.getElementById('rm-mechanisms');
    if (!root) return;
    root.replaceChildren();
    root.appendChild(renderViewHeading(
      msg('main.mechanisms'),
      msg('main.ready_explanations'),
      msg('main.choose.a.source.backed.path.and.walk.through.its.implementation.one.step.at.a.time')
    ));
    if (!USER_MECHANISMS.length) {
      var empty = el('div', 'rm-empty-state');
		empty.appendChild(txt('p', '', userArchitectureAvailable()
			? msg('main.explore.the.architecture.map.for.additional.context')
				: msg('main.no_source_backed_code_path')));
      root.appendChild(empty);
      return;
    }
    var primary = primaryUserMechanism();
    if (primary) {
      root.appendChild(txt('h3', 'rm-mechanism-group-title', msg('main.chrome.main.code.path')));
      appendMechanismGrid(root, [primary]);
    }
    var secondary = USER_MECHANISMS.filter(function (mechanism) {
      return !primary || mechanism.artifact_id !== primary.artifact_id;
    });
    if (secondary.length) {
      root.appendChild(txt('h3', 'rm-mechanism-group-title', msg('main.chrome.other.code.paths')));
      appendMechanismGrid(root, secondary);
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
    USER_MECHANISMS.forEach(function (mechanism) {
      (mechanism.steps || []).forEach(function (step) {
        (step.sources || []).forEach(function (snippet) { result.push(snippet); });
      });
      (mechanism.phases || []).forEach(function (phase) {
        (phase.sources || []).forEach(function (snippet) { result.push(snippet); });
      });
    });
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

  function sourceSnippetLabel(snippet) {
    var symbol = snippet.enclosing_symbol || snippet.path;
    return symbol + ' · ' + snippet.path + ':' + snippet.start_line + '–' + snippet.end_line;
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

  function openUserMechanism(artifactID, stepIndex, explicitStep) {
    var mechanism = userMechanismByID(USER_MECHANISMS, artifactID);
    if (!mechanism) {
      commitWorkspaceState(emptyWorkspaceState());
      return;
    }
		activeOverviewTopicID = '';
    var next = reduceWorkspaceState(workspaceState, {
      type: 'open_mechanism', artifactID: artifactID, stepIndex: stepIndex,
    }, USER_MECHANISMS);
    commitWorkspaceState(next, { mechanismRoot: !explicitStep });
  }

  function selectUserMechanismStep(stepIndex) {
    var mechanism = userMechanismByID(USER_MECHANISMS, workspaceState.artifactID);
    if (mechanism && boundedMechanismStep(mechanism, stepIndex) === workspaceState.stepIndex) return;
    var next = reduceWorkspaceState(workspaceState, {
      type: 'select_step', stepIndex: stepIndex,
    }, USER_MECHANISMS);
    commitWorkspaceState(next);
  }

  function moveUserMechanismStep(delta) {
    var next = reduceWorkspaceState(workspaceState, {
      type: 'move_step', delta: delta,
    }, USER_MECHANISMS);
    commitWorkspaceState(next);
  }

  function narrativeUnitName(mechanism) {
    return mechanismUsesPhases(mechanism) ? msg('main.phase') : msg('main.step');
  }

  function renderMechanismContext(mechanism) {
    var context = Array.isArray(mechanism && mechanism.context) ? mechanism.context : [];
    var items = [];
    context.forEach(function (item) {
      var action = repositoryAreaAction(item);
      if (!action) return;
      var remoteContext = action === 'code'
        ? staticSourceLink('', 'rm-context-item rm-source-target-link', item.code_location)
        : null;
      var button = remoteContext || el('button', 'rm-context-item');
      if (!remoteContext) button.type = 'button';
      button.appendChild(txt('strong', '', item.label || msg('main.chrome.related.code.area')));
      if (item.responsibility) button.appendChild(txt('span', '', item.responsibility));
      if (action === 'code') {
        button.appendChild(txt('code', '', formatCodeLocation(item.code_location)));
        if (!remoteContext) button.onclick = function () { openSourceLocation(item.code_location); };
      } else {
        button.appendChild(txt('span', 'rm-context-item__action', msg('main.view.on.map')));
        button.onclick = function () {
          openArchitectureTarget(item.map_target, {
            artifactID: mechanism.artifact_id,
            stepIndex: workspaceState.stepIndex,
          });
        };
      }
      items.push(button);
    });
    if (!items.length) return null;
    var strip = el('aside', 'rm-mechanism-context');
    strip.setAttribute('aria-label', msg('main.chrome.code.areas.around.this.path'));
    strip.appendChild(txt('span', 'rm-mechanism-context__label', msg('main.chrome.around.this.path')));
    var list = el('div', 'rm-mechanism-context__items');
    items.forEach(function (item) { list.appendChild(item); });
    strip.appendChild(list);
    return strip;
  }

  function renderImplementationDetails(mechanism, phase) {
    var implementationSteps = mechanismImplementationSteps(mechanism, phase);
    if (!implementationSteps.length) return null;
    var details = el('details', 'rm-implementation-details');
    details.appendChild(txt('summary', '', msg('main.mechanism.show_implementation_details', {
      count: implementationSteps.length,
    })));
    var list = el('div', 'rm-implementation-details__list');
    implementationSteps.forEach(function (step, index) {
      var item = el('article', 'rm-implementation-detail');
      item.appendChild(txt('strong', '', (index + 1) + '. ' + (step.title || msg('main.implementation'))));
      if (step.explanation) item.appendChild(txt('p', '', step.explanation));
      var sources = uniqueSourceSnippets(step.sources || [], {}, 3, false);
      if (sources.length) {
        var sourceList = el('div', 'rm-related-source-list');
        sources.forEach(function (snippet) { sourceList.appendChild(renderRelatedSourceTarget(snippet)); });
        item.appendChild(sourceList);
      }
      var exact = renderExactReferences(step.locations || []);
      if (exact) item.appendChild(exact);
      list.appendChild(item);
    });
    details.appendChild(list);
    return details;
  }

  function renderStepButtons(mechanism, step, className, includeMap) {
    var actions = el('div', className || 'rm-step-actions');
    actions.setAttribute('data-step-controls', 'true');
    var previous = txt('button', 'rm-quiet-action', msg('main.chrome.previous'));
    previous.type = 'button';
    previous.disabled = workspaceState.stepIndex === 0;
    previous.onclick = function () { moveUserMechanismStep(-1); };
    actions.appendChild(previous);
    var next = txt('button', 'rm-primary-action', msg('main.chrome.next'));
    next.type = 'button';
    next.disabled = workspaceState.stepIndex >= mechanismNarrativeItems(mechanism).length - 1;
    next.onclick = function () { moveUserMechanismStep(1); };
    actions.appendChild(next);
		if (includeMap && step.map_target && userArchitectureAvailable()) {
      var showMap = txt('button', 'rm-secondary-action', msg('main.chrome.show.on.map'));
      showMap.type = 'button';
      showMap.onclick = function () { showMechanismStepOnMap(step.map_target); };
      actions.appendChild(showMap);
    }
    return actions;
  }

  function renderMobileStepControls(mechanism) {
    var narrative = mechanismNarrativeItems(mechanism);
    var unit = narrativeUnitName(mechanism);
    var controls = el('div', 'rm-mobile-step-controls');
    controls.appendChild(txt(
      'strong', 'rm-mobile-step-progress',
      msg('main.unit_progress', {
        unit: unit,
        current: workspaceState.stepIndex + 1,
        total: narrative.length,
      })
    ));
    var previous = txt('button', 'rm-quiet-action', msg('main.chrome.previous.2'));
    previous.type = 'button';
    previous.disabled = workspaceState.stepIndex === 0;
    previous.onclick = function () { moveUserMechanismStep(-1); };
    controls.appendChild(previous);
    var picker = el('details', 'rm-mobile-step-picker');
    picker.appendChild(txt('summary', 'rm-secondary-action', msg('main.mechanism.all_units', {
      unit: unit.toLowerCase(),
    })));
    var choices = el('div', 'rm-mobile-step-picker__choices');
    narrative.forEach(function (candidate, index) {
      var choice = txt('button', index === workspaceState.stepIndex ? 'is-active' : '', (index + 1) + '. ' + candidate.title);
      choice.type = 'button';
      if (index === workspaceState.stepIndex) choice.setAttribute('aria-current', 'step');
      choice.onclick = function () {
        picker.open = false;
        selectUserMechanismStep(index);
      };
      choices.appendChild(choice);
    });
    picker.appendChild(choices);
    controls.appendChild(picker);
    var next = txt('button', 'rm-primary-action', msg('main.chrome.next.2'));
    next.type = 'button';
    next.disabled = workspaceState.stepIndex >= narrative.length - 1;
    next.onclick = function () { moveUserMechanismStep(1); };
    controls.appendChild(next);
    return controls;
  }

  function renderMechanismDetailWorkspace() {
    var root = document.getElementById('rm-mechanism-detail');
    if (!root) return;
    root.replaceChildren();
    var mechanism = userMechanismByID(USER_MECHANISMS, workspaceState.artifactID);
    var narrative = mechanismNarrativeItems(mechanism);
    if (!mechanism || !narrative.length) return;
    workspaceState.stepIndex = boundedMechanismStep(mechanism, workspaceState.stepIndex);
    var step = narrative[workspaceState.stepIndex];
    var unit = narrativeUnitName(mechanism);

    root.appendChild(renderViewHeading(
      mechanismRoleLabel(mechanism),
      mechanismPresentationTitle(mechanism),
      mechanism.question || msg('main.trace_through_implementation')
    ));
    var shortAnswer = mechanismShortAnswer(mechanism);
    if (shortAnswer) root.appendChild(txt('p', 'rm-mechanism-answer', shortAnswer));
    var context = renderMechanismContext(mechanism);
    if (context) root.appendChild(context);

    var layout = el('div', 'rm-mechanism-layout');
    var stepList = el('nav', 'rm-step-list');
    stepList.setAttribute('aria-label', msg('main.mechanism.aria_units', {
      unit: unit.toLowerCase(),
    }));
    narrative.forEach(function (candidate, index) {
      var button = txt('button', index === workspaceState.stepIndex ? 'is-active' : '', (index + 1) + '. ' + candidate.title);
      button.type = 'button';
      if (index === workspaceState.stepIndex) button.setAttribute('aria-current', 'step');
      button.onclick = function () { selectUserMechanismStep(index); };
      stepList.appendChild(button);
    });
    layout.appendChild(stepList);

    var current = el('article', 'rm-step-workspace');
    current.appendChild(renderMobileStepControls(mechanism));
    current.appendChild(txt(
      'div', 'rm-step-progress', msg('main.unit_progress', {
        unit: unit,
        current: workspaceState.stepIndex + 1,
        total: narrative.length,
      })
    ));
    current.appendChild(txt('h3', '', step.title));
    current.appendChild(txt('p', 'rm-step-explanation', step.explanation));
    current.appendChild(renderStepButtons(mechanism, step, 'rm-step-actions is-before', false));
    var sources = Array.isArray(step.sources) ? step.sources.filter(sourceSnippetAvailable) : [];
		if (sources.length) {
			var remainingLocations = (step.locations || []).slice();
			sources.forEach(function (snippet, sourceIndex) {
				current.appendChild(renderSourceSnippetCard(snippet, {
					primary: sourceIndex === 0,
						roleLabel: sourceIndex === 0
              ? msg('main.primary_implementation')
              : msg('main.supporting_implementation'),
					location: sourceSnippetLocation(snippet),
					notices: step.what_to_notice || [],
				}));
				remainingLocations = remainingExactReferences(remainingLocations, snippet);
			});
			var exactReferences = renderExactReferences(remainingLocations);
      if (exactReferences) current.appendChild(exactReferences);
    }
    var implementationDetails = renderImplementationDetails(mechanism, step);
    if (implementationDetails) current.appendChild(implementationDetails);
    current.appendChild(renderStepButtons(mechanism, step, 'rm-step-actions is-after', true));
    layout.appendChild(current);
    root.appendChild(layout);

    var allMechanismSources = [];
    var currentSourcePaths = {};
		sources.forEach(function (snippet) { currentSourcePaths[snippet.path] = true; });
    (mechanism.steps || []).forEach(function (candidate) {
      (candidate.sources || []).forEach(function (snippet) {
        allMechanismSources.push(snippet);
      });
    });
    var mechanismSources = uniqueSourceSnippets(allMechanismSources, currentSourcePaths, Number.MAX_SAFE_INTEGER, true);
    if (mechanismSources.length) {
      var involved = el('details', 'rm-workspace-section rm-mechanism-files');
      involved.appendChild(txt('summary', '', msg('main.mechanism.all_files_more', {
        count: mechanismSources.length,
      })));
      involved.appendChild(txt('p', 'rm-mechanism-files__hint', msg('main.chrome.open.another.source.excerpt.without.leaving.this.step')));
      var fileList = el('div', 'rm-related-source-list');
      mechanismSources.forEach(function (snippet) {
        fileList.appendChild(renderRelatedSourceTarget(snippet));
      });
      involved.appendChild(fileList);
      root.appendChild(involved);
    }
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
	  }
	}

  // Decision 222: source actions never open an inline code drawer in the
  // user surface — they always jump: GitHub/GitLab (static source mode) or
  // the repository server open action. When neither is available the action
  // is simply not offered (callers show the plain location text).
  function openSourceSnippet(snippet, location, expanded, options) {
    options = options || {};
    var resolved = sourceSnippetLocation(snippet, location);
    if (openStaticSource(resolved, snippet && snippet.end_line)) return;
    if (serverMode() && currentRunID() && snippet && OPENABLE_PATH_SET[snippet.path]) {
      requestOpenFile(snippet.path, Number(resolved.line) || 0, Number(resolved.column) || 0);
    }
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
    workspaceState = reduceWorkspaceState(workspaceState, { type: 'close_source' }, USER_MECHANISMS);
    writeWorkspaceHistory(workspaceHashForState(workspaceState), workspaceState, { replace: true });
    renderSourceDrawer();
  }

  function activeSourceDrawerMechanism() {
    if (workspaceState.view !== 'mechanism') return null;
    return userMechanismByID(USER_MECHANISMS, workspaceState.artifactID);
  }

	function activeSourceDrawerStudy() {
		if (workspaceState.view !== 'study') return null;
		return studyDirectionByID(workspaceState.directionID);
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
			var mechanism = activeSourceDrawerMechanism();
			var study = activeSourceDrawerStudy();
			var operation = activeSourceDrawerOperation();
			var task = workspaceState.view === 'investigate' ? TASK_INVESTIGATION : null;
			content.appendChild(txt('div', 'rm-view-kicker', mechanism ? msg('main.source.in.this.code.path') : study ? msg('main.source.in.this.reading.path') : operation ? msg('main.source.in.this.operating.path') : task ? msg('main.source.in.this.task.investigation') : msg('main.saved.source')));
    if (mechanism) {
      var step = mechanismNarrativeItems(mechanism)[boundedMechanismStep(mechanism, workspaceState.stepIndex)];
      var unit = narrativeUnitName(mechanism);
      content.appendChild(txt('h2', '', mechanismPresentationTitle(mechanism)));
      content.appendChild(txt('p', '', unit + ' ' + (workspaceState.stepIndex + 1) + ': ' + (step.title || msg('main.implementation'))));
    }
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

  function userArchitectureData() {
		if (!DATA.architecture_canvas || !userArchitectureAvailable()) return null;
    if (DEBUG_MODE) return DATA.architecture_canvas;
    var canvas = JSON.parse(JSON.stringify(DATA.architecture_canvas));
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
    });
    (canvas.subsystems || []).forEach(function (subsystem) {
      delete subsystem.diagnostics;
      delete subsystem.hash;
      delete subsystem.source_subsystem_ids;
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

			var matches = [];
			var componentStudyDirections = COMPLETE_STUDY_DIRECTIONS.length
				? COMPLETE_STUDY_DIRECTIONS
				: INCOMPLETE_STUDY_DIRECTIONS;
			componentStudyDirections.forEach(function (direction) {
				if (!direction || !direction.id) return;
				var exactTarget = (direction.areas || []).some(function (area) {
					var target = area && area.map_target;
					return target && target.kind === 'component' &&
						String(target.component_id || '') === String(component.id);
				});
				if (exactTarget) matches.push({ direction: direction });
			});

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
				(component.owned_surface_ids || []).forEach(function (surfaceID) {
					surfaceID = String(surfaceID || '');
					var surface = architectureSurfaceByID[surfaceID];
					var trigger = discoveredTriggerByID[surfaceID];
					if (!trigger) return;
					var handlerLocation = trigger.handler_location;
					var registrationLocation = trigger.registration_site || trigger.descriptor_site ||
						trigger.server_start_site;
					var entryLocation = trigger.process_entrypoint && trigger.process_entrypoint.location;
					var location = handlerLocation || registrationLocation || entryLocation;
					var filePath = String(location && location.path || '');
					var line = Number(location && location.line) || 0;
					if (!filePath || !OPENABLE_PATH_SET[filePath] || line <= 0) return;
					var surfaceName = String(surface && surface.name || trigger.identity && trigger.identity.name || '');
					var handlerName = String(trigger.handler && trigger.handler.known && trigger.handler.text || '');
					var label = surfaceName || handlerName ||
						String(trigger.process_entrypoint && trigger.process_entrypoint.name || filePath);
					if (handlerName && handlerName !== surfaceName) {
						label = surfaceName ? surfaceName + ' → ' + handlerName : handlerName;
					}
        if (!handlerLocation && registrationLocation) {
          label += ' · ' + msg('main.surface.registration_suffix');
        } else if (!handlerLocation && !registrationLocation && entryLocation) {
          label += ' · ' + msg('main.surface.process_entry_suffix');
        }
					var projectedLocation = {
						path: filePath,
						line: line,
						column: Number(location.column) || 0,
					};
					surfaceStarts.push({
						id: surfaceID,
						label: label,
						location: projectedLocation,
						actionable: sourceLocationActionAvailable(projectedLocation),
					});
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
				studies: matches.map(function (match) {
					return {
						id: match.direction.id,
						question: match.direction.question,
						why_it_matters: match.direction.why_it_matters || '',
					};
				}),
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

  function renderArchitectureReturn() {
    var root = document.getElementById('rm-architecture');
    if (!root) return;
    var existing = root.querySelector('.rm-architecture-return');
    if (existing) existing.remove();
    if (!workspaceState.mapReturn) return;
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
    var mechanism = userMechanismByID(USER_MECHANISMS, workspaceState.mapReturn.artifactID);
    if (!mechanism) return;
    var banner = el('div', 'rm-architecture-return');
    banner.appendChild(txt('span', '', msg('main.map.context', {
      title: mechanismPresentationTitle(mechanism),
    })));
    var back = txt(
      'button',
      'rm-secondary-action',
      msg('main.back_to_unit', {
        unit: narrativeUnitName(mechanism).toLowerCase(),
        index: workspaceState.mapReturn.stepIndex + 1,
      })
    );
    back.type = 'button';
    back.onclick = returnFromArchitecture;
    banner.appendChild(back);
    root.prepend(banner);
  }

  function renderArchitectureWorkspace() {
    var root = document.getElementById('rm-architecture');
    if (!root) return;
    root.replaceChildren();
    root.appendChild(renderViewHeading(
      msg('main.architecture'),
      msg('main.explore.the.repository.map'),
      msg('main.select.a.component.runtime.surface.or.code.path.to.inspect.its.implementation.context')
    ));
    // Decision 218 (E): truthful synthesis state. The visible copy maps the
    // exact closed status; "not performed" is forbidden for attempted
    // provider calls, and a rejected proposal still shows the local
    // Architecture. Closed diagnostic codes stay under provenance.
    var synthesis = DATA.architecture_synthesis || null;
    if (synthesis) {
      var synthesisNote = architectureSynthesisNote(synthesis);
      if (synthesisNote) {
        root.appendChild(txt('p', 'rm-architecture-synthesis-note' + (synthesisNote.warning ? ' rm-warning' : ''), synthesisNote.copy));
      }
    }
    // Decision 221 authority + coverage axes: one compact truth strip above
    // the relations area. Grouping authority and member evidence are never
    // conflated — the strip states who owns the grouping claim and how much
    // of the requested surface it covers.
    var truthStrip = renderArchitectureAuthorityStrip();
    if (truthStrip) root.appendChild(truthStrip);
    // Decision 222: the map/scheme is the first-class content — everything
    // else (relations inventory, component list) comes after it. The canvas
    // never reads as a runtime graph when no relation evidence exists.
    var relations = architectureRelationships();
    var relationState = architectureRelationState();
    var componentList = renderArchitectureComponentList();
    var canvasCard = null;
    if (DATA.architecture_canvas && window.RepomapArchitectureCanvas) {
      canvasCard = el('section', 'rm-card rm-architecture-canvas-card');
      if (relationState === 'no_supported_relation_evidence') {
        canvasCard.appendChild(txt('p', 'rm-architecture-canvas-conceptual rm-warning', msg('main.architecture.canvas.conceptual')));
      }
      architectureCanvasHost = el('div', 'rm-architecture-canvas-host');
      canvasCard.appendChild(architectureCanvasHost);
      root.appendChild(canvasCard);
    } else {
      var systemMap = renderUserSystemMap(DATA.high_level_map || []);
      if (systemMap) root.appendChild(systemMap);
    }
    // Relation area after the map: a closed three-state presentation derived
    // from exact evidence (Decision 218 D).
    //  - proven_component_relations: labeled relation list (and canvas);
    //  - member_relations_unprojected: exact structural-fact count and why no
    //    safe component edge was created;
    //  - no_supported_relation_evidence: the view is labeled a conceptual /
    //    package grouping and the structured list is primary.
    // Decision 230 D5: the Architecture page is a bounded workspace —
    // the map is the primary surface; relations, mechanism fragments and
    // the component list live behind explicit disclosures with compact
    // default counts. All information is preserved; nothing is deleted
    // to shorten the page.
    if (relations.length) {
      var relationDisclosure = el('details', 'rm-architecture-disclosure rm-architecture-relations-disclosure');
      relationDisclosure.open = false;
      var relationSummary = el('summary', 'rm-architecture-disclosure__summary');
      relationSummary.appendChild(txt('span', 'rm-architecture-disclosure__title', msg('main.architecture.relations.kicker')));
      relationSummary.appendChild(txt('span', 'rm-architecture-disclosure__count', msg('main.architecture.disclosure.count', { count: relations.length })));
      relationDisclosure.appendChild(relationSummary);
      relationDisclosure.appendChild(renderArchitectureRelations(relations));
      root.appendChild(relationDisclosure);
    } else if (relationState === 'member_relations_unprojected') {
      root.appendChild(renderArchitectureUnprojectedRelations());
    } else {
      root.appendChild(txt('p', 'rm-architecture-no-relation-evidence rm-warning', msg('main.architecture.no_relation_evidence')));
    }
    // Decision 226: one honest vertical fragment — entry → locally
    // supported transitions → observed boundary/resource → explicit
    // unresolved frontier. Rendered as a compact DFD-like list, never a
    // complete process proof; no invented edges.
    var fragment = DATA.mechanism_fragment;
    if (fragment && fragment.version === 1) {
      var transitionCount = Array.isArray(fragment.transitions) ? fragment.transitions.length : 0;
      var mechanismDisclosure = el('details', 'rm-architecture-disclosure rm-architecture-mechanism-disclosure');
      mechanismDisclosure.open = false;
      var mechanismSummary = el('summary', 'rm-architecture-disclosure__summary');
      mechanismSummary.appendChild(txt('span', 'rm-architecture-disclosure__title', msg('main.architecture.mechanism.title')));
      mechanismSummary.appendChild(txt('span', 'rm-architecture-disclosure__count', msg('main.architecture.disclosure.count', { count: transitionCount })));
      mechanismDisclosure.appendChild(mechanismSummary);
      mechanismDisclosure.appendChild(renderMechanismFragment(fragment));
      root.appendChild(mechanismDisclosure);
    }
    if (componentList) {
      // With no supported relation evidence the structured list is the
      // primary representation (it already follows the map in layout).
      var listDisclosure = el('details', 'rm-architecture-disclosure rm-architecture-list-disclosure');
      listDisclosure.open = false;
      var listSummary = el('summary', 'rm-architecture-disclosure__summary');
      listSummary.appendChild(txt('span', 'rm-architecture-disclosure__title', msg('main.architecture.component_list.title')));
      var componentCount = DATA.architecture_canvas && Array.isArray(DATA.architecture_canvas.components) ? DATA.architecture_canvas.components.length : 0;
      listSummary.appendChild(txt('span', 'rm-architecture-disclosure__count', msg('main.architecture.disclosure.count', { count: componentCount })));
      listDisclosure.appendChild(listSummary);
      listDisclosure.appendChild(componentList);
      root.appendChild(listDisclosure);
    }
    // Decision 217: compact unmapped-evidence disclosure preserving every
    // exact item behind an expand action.
    var unmapped = renderArchitectureUnmappedDisclosure();
    if (unmapped) root.appendChild(unmapped);
    renderArchitectureReturn();
  }

  // architectureSynthesisNote maps the exact closed Architecture synthesis
  // status to bounded user copy (Decision 218 E). It returns null when the
  // synthesis was accepted cleanly (no note needed). The mapping is:
  //  - unavailable/offline/uncalled -> not attempted;
  //  - accepted -> accepted (no note);
  //  - accepted_partial -> accepted for X/Y, local remainder shown;
  //  - validation/invalid response -> provider responded, proposal rejected,
  //    local Architecture shown;
  //  - output/response limit -> partial response unused, local shown;
  //  - cached -> accepted grouping replayed.
  function architectureSynthesisNote(synthesis) {
    if (!synthesis) return null;
    var state = String(synthesis.state || '');
    var warning = true;
    if (state === 'succeeded' || state === 'accepted') {
      if (synthesis.proposal_partial) {
        var covered = Number(synthesis.covered_conceptual_count) || 0;
        var requested = Number(synthesis.requested_conceptual_count) || 0;
        return {
          warning: false,
          copy: msg('main.architecture.synthesis_partial', { covered: covered, requested: requested }),
        };
      }
      return null;
    }
    if (state === 'cached') {
      return { warning: false, copy: msg('main.architecture.synthesis_cached') };
    }
    if (state === 'unavailable') {
      return { warning: false, copy: msg('main.architecture.synthesis_unavailable') };
    }
    if (state === 'failed') {
      if (synthesis.error_code === 'provider_output_limit') {
        return { warning: true, copy: msg('main.architecture.synthesis_output_limit') };
      }
      if (synthesis.provider_call_succeeded && synthesis.proposal_rejected) {
        return { warning: true, copy: msg('main.architecture.synthesis_rejected') };
      }
      return { warning: true, copy: msg('main.architecture.synthesis_failed') };
    }
    return null;
  }

  // architectureRelationState compiles the closed three-state relation
  // presentation (Decision 218 D) from exact local evidence: proven
  // component relations, member relations that exist but were not safely
  // projectable to component edges, or no supported relation evidence.
  function architectureRelationState() {
    var grounding = DATA.architecture_grounding || {};
    var rels = Array.isArray(grounding.relationships) ? grounding.relationships : [];
    if (rels.length) return 'proven_component_relations';
    var canvas = DATA.architecture_canvas || {};
    var facts = Array.isArray(canvas.structural_facts) ? canvas.structural_facts : [];
    var edges = Array.isArray(canvas.structural_edges) ? canvas.structural_edges : [];
    if (facts.length || edges.length) return 'member_relations_unprojected';
    return 'no_supported_relation_evidence';
  }

  function renderArchitectureUnprojectedRelations() {
    var canvas = DATA.architecture_canvas || {};
    var facts = Array.isArray(canvas.structural_facts) ? canvas.structural_facts : [];
    var edges = Array.isArray(canvas.structural_edges) ? canvas.structural_edges : [];
    var factsCount = facts.length;
    var edgesCount = edges.length;
    var section = el('section', 'rm-workspace-section rm-architecture-unprojected');
    section.appendChild(renderViewHeading(
      msg('main.architecture.relations.kicker'),
      msg('main.architecture.relations.title'),
      msg('main.architecture.unprojected.copy')
    ));
    section.appendChild(txt('p', '', msg('main.architecture.unprojected.summary', {
      facts: factsCount,
      edges: edgesCount,
    })));
    return section;
  }

  // architectureRelationships derives labeled relations from the grounding
  // evidence: exact from/to anchors with a bounded kind label.
  function architectureRelationships() {
    var grounding = DATA.architecture_grounding || {};
    var rels = Array.isArray(grounding.relationships) ? grounding.relationships : [];
    if (!rels.length) return [];
    var anchors = {};
    (DATA.architecture_canvas && Array.isArray(DATA.architecture_canvas.behavior_anchors) ? DATA.architecture_canvas.behavior_anchors : []).forEach(function (anchor) {
      if (anchor && anchor.id) anchors[String(anchor.id)] = String(anchor.label || anchor.name || anchor.id);
    });
    return rels.slice(0, 40).map(function (rel) {
      var location = rel && rel.location && rel.location.path ? rel.location : null;
      return {
        id: String(rel && rel.id || ''),
        kind: String(rel && rel.kind || ''),
        evidenceKind: String(rel && rel.evidence_kind || ''),
        from: anchors[String(rel && rel.from_anchor_id || '')] || String(rel && rel.from_anchor_id || ''),
        to: anchors[String(rel && rel.to_anchor_id || '')] || String(rel && rel.to_anchor_id || ''),
        path: location && location.path || '',
        line: location && location.line || 0,
        certainty: String(rel && rel.certainty || ''),
      };
    });
  }

  // architectureRelationKinds groups relations by kind with a human summary
  // of the proof shape before any raw inventory (Decision 221 relations
  // storytelling). Static structural support is distinguished from runtime
  // transition by the exact evidence kind.
  function architectureRelationSummary(relations) {
    if (!relations.length) return null;
    var byKind = {};
    var staticCount = 0;
    var runtimeCount = 0;
    relations.forEach(function (relation) {
      var kind = relation.kind || 'relation';
      byKind[kind] = (byKind[kind] || 0) + 1;
      var evidence = String(relation.evidenceKind || '');
      if (evidence.indexOf('runtime') >= 0 || evidence.indexOf('transition') >= 0 || evidence.indexOf('handoff') >= 0) {
        runtimeCount++;
      } else {
        staticCount++;
      }
    });
    return { byKind: byKind, staticCount: staticCount, runtimeCount: runtimeCount, total: relations.length };
  }

  // architectureRelationKindLabel maps the closed relation-kind set to
  // human copy (Decision 229 D4: relations inventory uses human group
  // labels; raw kinds stay under Evidence details).
  function architectureRelationKindLabel(kind) {
    var copy = {
      configures_security_boundary: 'main.architecture.relations.kind.configures_security_boundary',
      static_call_supporting_relation: 'main.architecture.relations.kind.static_call_supporting_relation',
    };
    return msg(copy[kind] || 'main.architecture.relations.kind.other');
  }

  function renderArchitectureRelations(relations) {
    var section = el('section', 'rm-workspace-section rm-architecture-relations');
    section.appendChild(renderViewHeading(
      msg('main.architecture.relations.kicker'),
      msg('main.architecture.relations.title'),
      msg('main.architecture.relations.copy')
    ));
    var summary = architectureRelationSummary(relations);
    if (summary) {
      // Human-facing summary before the raw inventory: how many relations,
      // how many are static structural support vs runtime transition, and
      // the kind breakdown.
      section.appendChild(txt('p', 'rm-architecture-relations__summary', msg(
        'main.architecture.relations.summary',
        { total: summary.total, staticCount: summary.staticCount, runtimeCount: summary.runtimeCount }
      )));
      var kindNames = Object.keys(summary.byKind).sort();
      if (kindNames.length) {
        var kindList = el('ul', 'rm-architecture-relation-kinds');
        kindNames.forEach(function (kind) {
          var item = txt('li', 'rm-architecture-relation-kind-row', '');
          item.appendChild(txt('span', 'rm-architecture-relation-kind-name', architectureRelationKindLabel(kind)));
          item.appendChild(txt('span', 'rm-architecture-relation-kind-count', String(summary.byKind[kind])));
          kindList.appendChild(item);
        });
        section.appendChild(kindList);
      }
    }
    // Grouped raw inventory: each relation is a labeled edge with an exact
    // source action when the evidence has a location.
    var groups = {};
    relations.forEach(function (relation) {
      var key = String(relation.kind || 'relation');
      if (!groups[key]) groups[key] = [];
      groups[key].push(relation);
    });
    var kindNames = Object.keys(groups).sort();
    kindNames.forEach(function (kind) {
      var groupSection = el('section', 'rm-architecture-relation-group');
      groupSection.appendChild(txt('h4', 'rm-architecture-relation-group__label', architectureRelationKindLabel(kind)));
      // Raw kind stays under Evidence details (Decision 229 D4) — the
      // human label is primary, the enum is the exact identifier.
      var rawKind = txt('span', 'rm-architecture-relation-group__raw-kind', kind);
      rawKind.hidden = true;
      groupSection.appendChild(rawKind);
      var list = el('ul', 'rm-architecture-relation-list');
      groups[kind].forEach(function (relation) {
        var item = txt('li', 'rm-architecture-relation-item', '');
        item.appendChild(txt('strong', '', relation.from));
        item.appendChild(txt('span', 'rm-architecture-relation-arrow', '→'));
        item.appendChild(txt('strong', '', relation.to));
        if (relation.path) {
          // Decision 221: an exact source action is shown only when the
          // persisted evidence actually contains the code for that line
          // (embedded snippet); a location-only resolution stays text-only —
          // never a dead button.
          var resolution = exactOverviewActionResolutionForLocation({ path: relation.path, line: relation.line || 0 });
          if (resolution && resolution.source && resolution.source.snippet) {
            var action = el('button', 'rm-source-action-link rm-architecture-relation-source', '');
            action.type = 'button';
            action.textContent = relation.path + ':' + relation.line;
            action.onclick = function () {
              openSourceSnippet(resolution.source.snippet, resolution.source.location, false, { drawerFirst: true });
            };
            item.appendChild(action);
          } else {
            item.appendChild(txt('span', 'rm-architecture-relation-source rm-architecture-relation-source--text', relation.path + ':' + relation.line));
          }
        }
        list.appendChild(item);
      });
      groupSection.appendChild(list);
      section.appendChild(groupSection);
    });
    return section;
  }

  // Decision 226: render the honest vertical mechanism fragment as a
  // compact DFD-like list. Every transition carries its closed contract
  // fields — claim_kind, support_mode, evidence, scenario, limitation,
  // ordering — ALL always visible, never hover-only; the unresolved
  // frontier renders with its items. This is a repository-perimeter
  // fragment, never a complete process proof.
  // Decision 229 D4/D5: raw contract enums (direct_static_call,
  // resolved_static, resolved_path_order, not_established) are never
  // primary user copy. Human phrases carry the primary meaning; the raw
  // enum stays under "Evidence details".
  var MECHANISM_CLAIM_KIND_COPY = {
   process_entry: 'main.architecture.mechanism.kind.process_entry',
   direct_static_call: 'main.architecture.mechanism.kind.direct_static_call',
   unresolved_continuation: 'main.architecture.mechanism.kind.unresolved_continuation',
   storage_boundary_callsite: 'main.architecture.mechanism.kind.storage_boundary_callsite',
   outbound_client_callsite: 'main.architecture.mechanism.kind.outbound_client_callsite',
  };
  var MECHANISM_SUPPORT_MODE_COPY = {
   resolved_static: 'main.architecture.mechanism.support.resolved_static',
   observed_local: 'main.architecture.mechanism.support.observed_local',
   unknown: 'main.architecture.mechanism.support.unknown',
  };
  var MECHANISM_ORDERING_COPY = {
   exact_local_order: 'main.architecture.mechanism.ordering.resolved_path_order',
   resolved_path_order: 'main.architecture.mechanism.ordering.resolved_path_order',
   not_established: 'main.architecture.mechanism.ordering.not_established',
  };

  function mechanismClaimKindLabel(transition) {
   var copy = MECHANISM_CLAIM_KIND_COPY[String(transition && transition.claim_kind || '')];
   return copy ? msg(copy) : String(transition && transition.claim_kind || '');
  }

  function mechanismSupportModeLabel(transition) {
   var copy = MECHANISM_SUPPORT_MODE_COPY[String(transition && transition.support_mode || '')];
   return copy ? msg(copy) : String(transition && transition.support_mode || '');
  }

  function mechanismOrderingLabel(transition) {
   var copy = MECHANISM_ORDERING_COPY[String(transition && transition.ordering || '')];
   return copy ? msg(copy) : String(transition && transition.ordering || '');
  }

  function renderMechanismFragment(fragment) {
   var section = el('section', 'rm-workspace-section rm-mechanism-fragment');
   section.appendChild(renderViewHeading(
    msg('main.architecture.mechanism.kicker'),
    msg('main.architecture.mechanism.title'),
    msg('main.architecture.mechanism.copy')
   ));
   var entry = fragment.entry || {};
   var transitions = Array.isArray(fragment.transitions) ? fragment.transitions : [];
   // Decision 230 D8: connected mechanism fragments. Array order is never
   // path order. A path exists only through exact supported joins: the
   // entry handoffs recorded by the local grounding pass. Direct static
   // handoffs whose target is a lifecycle continuation stay in the entry
   // chain; handoffs into boundary/resource targets become independent
   // fragments; boundary/resource observations without a path join become
   // side touchpoint groups (never "next steps").
   var grounding = DATA.architecture_grounding || {};
   var handoffs = Array.isArray(grounding.entry_handoffs) ? grounding.entry_handoffs : [];
   var anchorsByID = {};
   (Array.isArray(grounding.behavior_anchors) ? grounding.behavior_anchors : []).forEach(function (anchor) {
    if (anchor && anchor.id) anchorsByID[anchor.id] = anchor;
   });
   // Resolve each direct handoff's target anchor kind from the
   // relationships recorded at the same callsite.
   var relationships = Array.isArray(grounding.relationships) ? grounding.relationships : [];
   var callsiteTargetKind = {};
   relationships.forEach(function (rel) {
    if (!rel || !rel.location) return;
    var key = String(rel.location.path || '') + ':' + String(rel.location.line || 0);
    var target = anchorsByID[rel.to_anchor_id] || {};
    callsiteTargetKind[key] = String(target.kind || '');
   });

   var directCalls = transitions.filter(function (t) {
    return t && t.claim_kind === 'direct_static_call';
   });
   var touchpoints = transitions.filter(function (t) {
    return t && (t.claim_kind === 'storage_boundary_callsite' || t.claim_kind === 'outbound_client_callsite');
   });
   var frontier = fragment.frontier || {};
   var hasUnresolvedContinuation = transitions.some(function (t) {
    return t && t.claim_kind === 'unresolved_continuation';
   });

   var workspace = el('div', 'rm-mechanism-fragment__workspace');
   // --- Entry fragment: entry → lifecycle continuation → frontier ---
   var entryFragment = el('div', 'rm-mechanism-fragment__graph rm-mechanism-fragment__graph--entry');
   entryFragment.appendChild(txt('h4', 'rm-mechanism-fragment__fragment-title', msg('main.architecture.mechanism.fragment.entry')));
   var entryNode = renderMechanismLane({ transition: entry, kind: 'entry' });
   entryFragment.appendChild(entryNode);
   var chainNext = [];
   directCalls.forEach(function (t) {
    var key = String(t.path || '') + ':' + String(t.line || 0);
    if (String(callsiteTargetKind[key]).indexOf('lifecycle_') === 0) chainNext.push(t);
   });
   // Deterministic ordering by exact line, never array position.
   chainNext.sort(function (a, b) { return (Number(a.line) || 0) - (Number(b.line) || 0); });
   chainNext.forEach(function (t) {
    var arrow = el('div', 'rm-mechanism-fragment__arrow');
    arrow.appendChild(txt('span', 'rm-mechanism-fragment__arrow-line', '→'));
    arrow.appendChild(txt('span', 'rm-mechanism-fragment__arrow-mode', mechanismSupportModeLabel(t)));
    entryFragment.appendChild(arrow);
    entryFragment.appendChild(renderMechanismLane({ transition: t, kind: 'transition' }));
   });
   if (chainNext.length || hasUnresolvedContinuation) {
    var frontierArrow = el('div', 'rm-mechanism-fragment__arrow rm-mechanism-fragment__arrow--frontier');
    frontierArrow.appendChild(txt('span', 'rm-mechanism-fragment__arrow-line', '⇢'));
    frontierArrow.appendChild(txt('span', 'rm-mechanism-fragment__arrow-mode', msg('main.architecture.mechanism.fragment.unknown_continuation')));
    entryFragment.appendChild(frontierArrow);
    entryFragment.appendChild(renderMechanismFrontier(frontier, fragment));
   }
   workspace.appendChild(entryFragment);

   // --- Independent fragments: handoffs whose target is not a lifecycle
   // continuation (boundary/resource targets) ---
   var independent = directCalls.filter(function (t) {
    var key = String(t.path || '') + ':' + String(t.line || 0);
    return String(callsiteTargetKind[key]).indexOf('lifecycle_') !== 0;
   });
   independent.forEach(function (t) {
    var frag = el('div', 'rm-mechanism-fragment__graph rm-mechanism-fragment__graph--independent');
    frag.appendChild(txt('h4', 'rm-mechanism-fragment__fragment-title', msg('main.architecture.mechanism.fragment.independent')));
    var sourceNode = renderMechanismLane({ transition: t, kind: 'transition' });
    frag.appendChild(sourceNode);
    var arrow = el('div', 'rm-mechanism-fragment__arrow');
    arrow.appendChild(txt('span', 'rm-mechanism-fragment__arrow-line', '→'));
    arrow.appendChild(txt('span', 'rm-mechanism-fragment__arrow-mode', mechanismSupportModeLabel(t)));
    frag.appendChild(arrow);
    // The target symbol comes from the recorded anchor at the same callsite.
    var key = String(t.path || '') + ':' + String(t.line || 0);
    var rel = relationships.filter(function (r) {
     return r && r.location && String(r.location.path || '') + ':' + String(r.location.line || 0) === key;
    })[0];
    var targetAnchor = rel ? (anchorsByID[rel.to_anchor_id] || {}) : {};
    var targetNode = el('div', 'rm-mechanism-fragment__target');
    var targetSymbol = String(targetAnchor.label || '').replace(/^[a-z_]+ /, '');
    targetNode.appendChild(txt('strong', 'rm-mechanism-fragment__target-symbol', targetSymbol || t.symbol || ''));
    if (targetAnchor.location) {
     targetNode.appendChild(renderMechanismLocationAction(targetAnchor.location, t));
    }
    frag.appendChild(targetNode);
    workspace.appendChild(frag);
   });

   // --- Side touchpoint groups: boundary/resource observations without a
   // path join are grouped by family — never rendered as next steps. ---
   var touchpointGroups = {};
   touchpoints.forEach(function (t) {
    var family = mechanismTouchpointFamily(t);
    if (!touchpointGroups[family]) touchpointGroups[family] = [];
    touchpointGroups[family].push(t);
   });
   var familyNames = Object.keys(touchpointGroups).sort();
   if (familyNames.length) {
    var touchSection = el('div', 'rm-mechanism-fragment__touchpoints');
    touchSection.appendChild(txt('h4', 'rm-mechanism-fragment__fragment-title', msg('main.architecture.mechanism.fragment.touchpoints')));
    touchSection.appendChild(txt('p', 'rm-mechanism-fragment__touchpoints-copy', msg('main.architecture.mechanism.fragment.touchpoints_copy')));
    familyNames.forEach(function (family) {
     var group = touchpointGroups[family];
     var details = el('details', 'rm-mechanism-fragment__touchpoint-group');
     details.open = false;
     var summary = el('summary', 'rm-mechanism-fragment__touchpoint-summary');
     summary.appendChild(txt('span', 'rm-mechanism-fragment__touchpoint-family', family));
     summary.appendChild(txt('span', 'rm-mechanism-fragment__touchpoint-count', msg('main.architecture.disclosure.count', { count: group.length })));
     details.appendChild(summary);
     var list = el('ul', 'rm-mechanism-fragment__touchpoint-list');
     group.forEach(function (t) {
      var item = txt('li', 'rm-mechanism-fragment__touchpoint-item', '');
      var label = mechanismTransitionLabel(t);
      if (label) item.appendChild(txt('span', 'rm-mechanism-fragment__touchpoint-label', label));
      if (t.path && t.line) item.appendChild(renderMechanismLocationAction({ path: t.path, line: t.line }, t));
      list.appendChild(item);
     });
     details.appendChild(list);
     touchSection.appendChild(details);
    });
    workspace.appendChild(touchSection);
   }

   section.appendChild(workspace);
   if (!chainNext.length && !independent.length && !familyNames.length) {
    section.appendChild(txt('p', 'rm-arch__copy rm-arch__inspector-empty', msg('main.architecture.mechanism.no_transitions')));
   }
   return section;
  }

  function mechanismTransitionLabel(transition) {
   var rawLabel = String(transition && transition.label || '');
   var label = rawLabel.indexOf('process entry ') === 0 ? rawLabel.slice('process entry '.length) : rawLabel;
   if (label.indexOf('boundary ') === 0) {
    label = msg('main.architecture.mechanism.label.boundary') + ' · ' + label.slice('boundary '.length);
   } else if (label.indexOf('resource ') === 0) {
    label = msg('main.architecture.mechanism.label.resource') + ' · ' + label.slice('resource '.length);
   } else if (label.indexOf('operation') === 0) {
    label = msg('main.architecture.mechanism.label.operation');
   } else if (label.indexOf('surface') === 0) {
    label = msg('main.architecture.mechanism.label.surface');
   }
   return label;
  }

  function mechanismTouchpointFamily(transition) {
   var raw = String(transition && transition.label || '');
   // "boundary net", "resource github.com/casdoor/notify2" → the last token.
   var tokens = raw.split(/\s+/);
   if (tokens.length >= 2 && (tokens[0] === 'boundary' || tokens[0] === 'resource')) {
    return tokens.slice(1).join(' ') || 'unknown';
   }
   return msg('main.architecture.mechanism.fragment.touchpoint_misc');
  }

  function renderMechanismLocationAction(location, transition) {
   var callsite = location.path + (location.line ? ':' + location.line : '');
   var resolution = exactOverviewActionResolutionForLocation({ path: location.path, line: Number(location.line) || 0 });
   var snippet = resolution && resolution.source && resolution.source.snippet;
   if (snippet) {
    var action = el('button', 'rm-mechanism-fragment__location rm-source-action-link');
    action.type = 'button';
    action.textContent = callsite;
    action.onclick = function () {
     openSourceSnippet(snippet, resolution.source.location, false, { drawerFirst: true });
    };
    return action;
   }
   return txt('code', 'rm-mechanism-fragment__location rm-mechanism-fragment__location--text', callsite);
  }

  function renderMechanismFrontier(frontier, fragment) {
   var box = el('div', 'rm-mechanism-fragment__frontier');
   box.appendChild(txt('strong', null, msg('main.architecture.mechanism.frontier_title')));
   box.appendChild(txt('p', 'rm-arch__limitation', String(frontier.limitation || msg('main.architecture.mechanism.frontier_default'))));
   (Array.isArray(frontier.unresolved) ? frontier.unresolved : []).forEach(function (item) {
    if (String(item || '').trim()) box.appendChild(txt('p', 'rm-mechanism-fragment__frontier-item', String(item)));
   });
   return box;
  }

  function renderMechanismLane(lane) {
   var transition = lane.transition || {};
   var item = el('article', 'rm-mechanism-fragment__lane' + (lane.kind === 'entry' ? ' rm-mechanism-fragment__lane--entry' : ''));
   var head = el('div', 'rm-mechanism-fragment__lane-head');
   head.appendChild(txt('strong', 'rm-mechanism-fragment__kind', mechanismClaimKindLabel(transition)));
   head.appendChild(txt('span', 'rm-mechanism-fragment__mode', mechanismSupportModeLabel(transition)));
   item.appendChild(head);
   var callsite = transition.path ? transition.path + (transition.line ? ':' + transition.line : '') : '';
   if (callsite) {
    var location = { path: transition.path, line: Number(transition.line) || 0 };
    var resolution = exactOverviewActionResolutionForLocation(location);
    var snippet = resolution && resolution.source && resolution.source.snippet;
    if (snippet) {
     var locationAction = el('button', 'rm-mechanism-fragment__location rm-source-action-link');
     locationAction.type = 'button';
     locationAction.textContent = callsite;
     locationAction.onclick = function () {
      openSourceSnippet(snippet, resolution.source.location, false, { drawerFirst: true });
     };
     item.appendChild(locationAction);
    } else {
     item.appendChild(txt('code', 'rm-mechanism-fragment__location rm-mechanism-fragment__location--text', callsite));
    }
   }
   // Decision 229 D4: the entry lane's raw label starts with the claim
   // kind ("process entry …"), which the kind badge already states —
   // strip the prefix so primary copy never repeats a raw enum.
   var label = mechanismTransitionLabel(transition);
   if (transition.symbol && transition.symbol.indexOf('member-symbol') !== 0) {
    item.appendChild(txt('span', 'rm-mechanism-fragment__label', String(transition.symbol)));
   } else if (label && label !== 'handoff') {
    item.appendChild(txt('span', 'rm-mechanism-fragment__label', label));
   }
   var ordering = mechanismOrderingLabel(transition);
   if (ordering) item.appendChild(txt('span', 'rm-mechanism-fragment__ordering', ordering));
   if (transition.limitation) item.appendChild(txt('p', 'rm-arch__limitation rm-mechanism-fragment__limitation', String(transition.limitation)));
   var details = el('details', 'rm-mechanism-fragment__evidence-details');
   details.appendChild(txt('summary', 'rm-mechanism-fragment__evidence-summary', msg('main.architecture.mechanism.evidence_details')));
   var raw = [];
   if (transition.claim_kind) raw.push('claim_kind: ' + transition.claim_kind);
   if (transition.support_mode) raw.push('support_mode: ' + transition.support_mode);
   if (transition.ordering) raw.push('ordering: ' + transition.ordering);
   if (transition.evidence) raw.push('evidence: ' + transition.evidence);
   if (transition.scenario) raw.push('scenario: ' + transition.scenario);
   if (transition.path && transition.line) raw.push('exact source: ' + transition.path + ':' + transition.line);
   if (raw.length) {
    var rawList = el('ul', 'rm-mechanism-fragment__evidence-raw');
    raw.forEach(function (line) { rawList.appendChild(txt('li', '', line)); });
    details.appendChild(rawList);
    item.appendChild(details);
   }
   return item;
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
    var canvas = DATA.architecture_canvas || {};
    var remainderID = String(canvas.local_remainder_component_id || '');
    var components = (Array.isArray(canvas.components) ? canvas.components : []).filter(function (component) {
      return component && component.id !== remainderID;
    });
    if (!components.length) return null;
    var section = el('section', 'rm-workspace-section rm-architecture-list');
    section.appendChild(renderViewHeading(
      msg('main.architecture.list.kicker'),
      msg('main.architecture.list.title'),
      msg('main.architecture.list.copy')
    ));
    var list = el('ol', 'rm-architecture-list__items');
    components.forEach(function (component) {
      var tier = componentEvidenceTier(component);
      var composition = componentEvidenceComposition(component);
      var item = txt('li', 'rm-architecture-list__item rm-architecture-list__item--' + tier, '');
      var primary = el('button', 'rm-architecture-list__primary');
      primary.type = 'button';
      primary.appendChild(txt('strong', '', String(component.name || component.id)));
      primary.appendChild(txt('span', 'rm-evidence-tier rm-evidence-tier--' + tier, componentEvidenceTierLabel(tier)));
      primary.appendChild(txt('span', 'rm-evidence-composition rm-evidence-composition--' + composition, msg(
        composition === 'exact' ? 'main.architecture.evidence.exact'
          : composition === 'mixed' ? 'main.architecture.evidence.mixed'
          : 'main.architecture.evidence.package'
      )));
      primary.onclick = function () {
        if (architectureCanvasView) {
          architectureCanvasView.openComponent(String(component.id));
        } else {
          openArchitectureTarget({ kind: 'component', component_id: component.id }, null);
        }
      };
      item.appendChild(primary);
      if (component.description) item.appendChild(txt('p', 'rm-architecture-list__description', String(component.description)));
      var memberCount = Array.isArray(component.members) ? component.members.length : 0;
      if (memberCount > 0) {
        item.appendChild(txt('span', 'rm-architecture-list__count', msg('main.architecture.list.member_count', { count: memberCount })));
      }
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
      candidateDirections: DEBUG_MODE ? candidateDirections() : [],
      savedFlows: DEBUG_MODE ? (DATA.flows || []) : [],
      guidedTour: DEBUG_MODE ? (DATA.guided_tour || null) : null,
      semanticArtifacts: DEBUG_MODE ? (DATA.semantic_artifacts || []) : [],
      startHereArtifactID: DEBUG_MODE ? (DATA.start_here_artifact_id || '') : '',
      presentationText: DEBUG_MODE ? (DATA.architecture_debug_presentation || {}) : {},
      stalePaths: new Set((DATA.freshness && DATA.freshness.affected_paths) || []),
    };
		if (!DEBUG_MODE) {
		options.componentContexts = architectureComponentContexts();
		options.openStudyDirection = openStudyDirection;
		options.openSourceLocation = openSourceLocation;
		options.associations = DATA.architecture_associations || null;
		}
    if (staticSourceMode()) {
      options.openLocation = function (filePath, line) {
        return openStaticSource({ path: filePath, line: line || 0 });
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

  function showMechanismStepOnMap(target) {
		if (!target || !userArchitectureAvailable()) return;
    var next = reduceWorkspaceState(workspaceState, { type: 'show_map', target: target }, USER_MECHANISMS);
    commitWorkspaceState(next);
  }

  function openArchitectureTarget(target, returnTarget) {
		if (!userArchitectureAvailable()) {
			commitWorkspaceState(emptyWorkspaceState());
			return;
		}
    var next = reduceWorkspaceState(workspaceState, { type: 'view', view: 'architecture' }, USER_MECHANISMS);
    next.mapReturn = returnTarget || null;
    next.mapTarget = target || null;
    commitWorkspaceState(next);
  }

  function returnFromArchitecture() {
    var returnTarget = workspaceState.mapReturn;
    if (!returnTarget) return;
    var next = reduceWorkspaceState(workspaceState, { type: 'return_from_map' }, USER_MECHANISMS);
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
		var navView = view === 'mechanism'
			? 'mechanisms'
			: view === 'study'
				? ((STUDY_DIRECTIONS.length || themeCards().length) ? 'study_overview' : 'overview')
				: view === 'operate'
					? 'overview'
					: view;
    document.querySelectorAll('[data-workspace-view]').forEach(function (button) {
      var active = button.getAttribute('data-workspace-view') === navView;
      button.classList.toggle('rm-active', active);
      if (active) button.setAttribute('aria-current', 'page');
      else button.removeAttribute('aria-current');
    });
  }

  function renderWorkspaceState() {
		if (workspaceState.view === 'investigate') renderTaskInvestigationWorkspace();
    if (workspaceState.view === 'mechanism') renderMechanismDetailWorkspace();
		if (workspaceState.view === 'study_overview') renderIncompleteStudyOverview();
		if (workspaceState.view === 'study') {
			if (workspaceState.themeCardOrdinal) renderThemeDetailWorkspace();
			else renderStudyDetailWorkspace();
		}
		if (workspaceState.view === 'operate') renderOperateDetailWorkspace();
    if (workspaceState.view === 'architecture') {
      if (!architectureCanvasHost) renderArchitectureWorkspace();
      else renderArchitectureReturn();
      activateWorkspaceView('architecture');
      var ready = architectureCanvasView ? (architectureReady || Promise.resolve(architectureCanvasView)) : mountArchitectureCanvas();
      ready.then(function () { focusArchitectureTarget(workspaceState.mapTarget); });
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
			addWorkspaceTab(msg('main.overview'), 'overview');
			if (!ATLAS_FIRST && USER_MECHANISMS.length) addWorkspaceTab(msg('main.mechanisms'), 'mechanisms');
			if (STUDY_DIRECTIONS.length || themeCards().length) addWorkspaceTab(msg('main.study'), 'study_overview');
			if (userArchitectureAvailable()) addWorkspaceTab(msg('main.architecture'), 'architecture');
		}
		if (DEBUG_MODE) addWorkspaceTab(msg('main.provenance'), 'provenance');
	}

  function render() {
    DATA.flows = DATA.flows || [];
    componentSelectionViews = {};
    resumeInvestigationStarted = false;
    var repoName = document.getElementById('rm-repo-name');
    if (repoName) repoName.textContent = DATA.repo_name || msg('main.repository');
		var workspacePurpose = document.getElementById('rm-workspace-purpose');
		if (workspacePurpose && TASK_INVESTIGATION) workspacePurpose.textContent = msg('main.chrome.task.investigation');
    setupServerFeatures();
    renderRunDetails();

		renderWorkspaceTabs();

		if (TASK_INVESTIGATION) {
			renderTaskInvestigationWorkspace();
		} else {
			renderOverviewWorkspace();
			renderMechanismsWorkspace();
			renderMechanismDetailWorkspace();
			renderIncompleteStudyOverview();
			renderStudyDetailWorkspace();
			renderOperateDetailWorkspace();
		}
    renderProvenanceWorkspace();
    restoreWorkspaceFromRoute();
    if (DEBUG_MODE) resumeLatestInvestigation();
  }

  if (window.__REPOMAP_WORKSPACE_TEST__ && typeof window.__REPOMAP_WORKSPACE_TEST__ === 'object') {
    Object.assign(window.__REPOMAP_WORKSPACE_TEST__, {
      boundedMechanismStep: boundedMechanismStep,
      mechanismNarrativeItems: mechanismNarrativeItems,
      mechanismImplementationSteps: mechanismImplementationSteps,
      narrativeIndexForImplementationStep: narrativeIndexForImplementationStep,
      architectureFocusValue: architectureFocusValue,
      architectureFocusNeedsReset: architectureFocusNeedsReset,
      architectureTargetFromFocus: architectureTargetFromFocus,
		architectureComponentContexts: architectureComponentContexts,
      packageSourceTarget: packageSourceTarget,
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
      renderUserMechanismCard: renderUserMechanismCard,
      renderImplementationDetails: renderImplementationDetails,
      renderOverviewWorkspace: renderOverviewWorkspace,
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
      renderMechanismDetailWorkspace: renderMechanismDetailWorkspace,
      renderStudyDetailWorkspace: renderStudyDetailWorkspace,
      renderIncompleteStudyOverview: renderIncompleteStudyOverview,
		renderStudyDirectionCard: renderStudyDirectionCard,
		renderStudyReadingAnchor: renderStudyReadingAnchor,
		renderReadableDocument: renderReadableDocument,
		renderReadableDocumentCard: renderReadableDocumentCard,
		renderOperateDetailWorkspace: renderOperateDetailWorkspace,
      renderPavedPathCard: renderPavedPathCard,
      renderProofStaticRelation: renderProofStaticRelation,
      renderLocalProof: renderLocalProof,
      renderSemanticArtifactCard: renderSemanticArtifactCard,
      appendResearchStage: appendResearchStage,
      appendResearchRound: appendResearchRound,
      researchSelectionReasonLabel: researchSelectionReasonLabel,
      exactSymbolDetail: exactSymbolDetail,
      semanticArtifactKindLabel: semanticArtifactKindLabel,
      sourceNoticeRanges: sourceNoticeRanges,
      embeddedSourceForLocation: embeddedSourceForLocation,
      mechanismPresentationTitle: mechanismPresentationTitle,
      mechanismShortAnswer: mechanismShortAnswer,
      openUserMechanism: openUserMechanism,
		openStudyDirection: openStudyDirection,
		openPavedPath: openPavedPath,
      selectUserMechanismStep: selectUserMechanismStep,
      openSourceSnippet: openSourceSnippet,
      closeSourceDrawer: closeSourceDrawer,
      activeSourceDrawerMechanism: activeSourceDrawerMechanism,
		activeSourceDrawerOperation: activeSourceDrawerOperation,
      navigateWorkspace: navigateWorkspace,
      showMechanismStepOnMap: showMechanismStepOnMap,
      returnFromArchitecture: returnFromArchitecture,
      restoreWorkspaceFromRoute: restoreWorkspaceFromRoute,
      workspaceStateSnapshot: function () { return JSON.parse(JSON.stringify(workspaceState)); },
      userMechanismByID: userMechanismByID,
		pavedPathByID: pavedPathByID,
      viewSectionID: viewSectionID,
		openReportTarget: openReportTarget,
      reportTargetAvailable: reportTargetAvailable,
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
