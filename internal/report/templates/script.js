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
	var REPOSITORY_GUIDE = DATA.repository_guide || null;
	var STUDY_MAP = DATA.study_map || null;
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
    if (state.view === 'study_overview' && STUDY_DIRECTIONS.length) return '#/study';
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
			state.view = STUDY_DIRECTIONS.length ? 'study_overview' : 'overview';
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
    var button = txt('button', cls || '', label);
    button.type = 'button';
    button.onclick = action;
    return button;
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
        if (DATA.architecture_synthesis.proposal_normalized) {
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
			actions.appendChild(source);
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
		var card = el('button', 'rm-study-direction-card');
		card.type = 'button';
		card.appendChild(txt('span', 'rm-study-direction-card__order', String(index + 1)));
		var body = el('span', 'rm-study-direction-card__body');
			if (direction.incomplete) {
				body.appendChild(txt(
					'span',
					'rm-study-reading-anchor__label',
					msg('main.study.stage_incomplete', {
						stage: studyStageLabel(direction.learning_stage),
					})
				));
		}
		body.appendChild(txt('strong', '', direction.question || msg('main.chrome.explore.this.code.area')));
		if (direction.why_it_matters) body.appendChild(txt('span', 'rm-study-direction-card__reason', direction.why_it_matters));
		if (direction.learning_outcome) body.appendChild(txt('span', 'rm-study-direction-card__outcome', direction.learning_outcome));
		var anchors = el('span', 'rm-study-direction-card__anchors');
		(direction.principal_anchors || []).slice(0, 4).forEach(function (anchor) {
			anchors.appendChild(txt('code', '', anchor.symbol || anchor.path));
		});
		if (anchors.childNodes.length) body.appendChild(anchors);
		body.appendChild(txt(
			'span',
				'rm-study-direction-card__action',
				direction.mechanism_id
					? msg('main.study.open_ready_deep_dive')
					: direction.incomplete
						? msg('main.study.inspect_starting_point')
						: msg('main.explore.this.direction')
		));
		card.appendChild(body);
		card.onclick = function () { openStudyDirection(direction.id); };
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

	var STUDY_READING_LABEL_MESSAGE_IDS = {
		'Start here': 'main.study.reading_label.start_here',
		'Then inspect': 'main.study.reading_label.then_inspect',
		'Related implementation': 'main.study.reading_label.related_implementation',
		'Public boundary': 'main.study.reading_label.public_boundary',
		'Core data type': 'main.study.reading_label.core_data_type',
	};

	function studyReadingLabel(label) {
		return msg(
			STUDY_READING_LABEL_MESSAGE_IDS[String(label || '')] ||
			'main.study.reading_label.unknown'
		);
	}

	function renderIncompleteStudyOverview() {
		var root = document.getElementById('rm-study-overview');
		if (!root) return;
		root.replaceChildren();
		var canonical = COMPLETE_STUDY_DIRECTIONS.length > 0;
		var directions = canonical ? COMPLETE_STUDY_DIRECTIONS : INCOMPLETE_STUDY_DIRECTIONS;
		root.appendChild(canonical
			? renderViewHeading(
				msg('main.study'),
				msg('main.a.useful.path.through.the.repository'),
				msg('main.choose.a.question.and.begin.with.concrete.source.anchors.the.order.is.for.learning.not.an.execution.trace')
			)
			: renderViewHeading(
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

	function renderStudyMapOverview(root) {
// repomap-source-episode:start
		var episode = renderSourceEpisode(SOURCE_EPISODE);
		if (episode) root.appendChild(episode);
// repomap-source-episode:end

		var brief = STUDY_MAP && STUDY_MAP.brief || {};
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
		root.appendChild(hero);

		var areaCards = (Array.isArray(STUDY_MAP.shape) ? STUDY_MAP.shape : []).map(renderRepositoryArea).filter(Boolean);
		if (areaCards.length) {
			var shapeSection = el('section', 'rm-workspace-section');
			shapeSection.appendChild(renderViewHeading(msg('main.repository.shape'), msg('main.code.areas.to.know'), msg('main.a.compact.map.of.the.production.areas.worth.recognizing.first')));
			var areaGrid = el('div', 'rm-repository-area-grid');
			areaCards.forEach(function (card) { areaGrid.appendChild(card); });
			shapeSection.appendChild(areaGrid);
			root.appendChild(shapeSection);
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

		if (USER_MECHANISMS.length) {
			var deepDiveSection = el('section', 'rm-workspace-section');
			deepDiveSection.appendChild(renderViewHeading(msg('main.ready.deep.dives'), msg('main.source.backed.code.paths'), msg('main.these.directions.already.have.a.validated.step.by.step.implementation.explanation')));
			appendMechanismGrid(deepDiveSection, USER_MECHANISMS.slice(0, 6));
			root.appendChild(deepDiveSection);
		}

		var hasArchitecture = userArchitectureAvailable();
		if (hasArchitecture) {
			var exploreSection = el('section', 'rm-workspace-section rm-overview-explore');
			exploreSection.appendChild(renderViewHeading(msg('main.explore'), msg('main.open.the.full.architecture'), msg('main.use.the.study.map.as.a.starting.point.then.inspect.the.wider.repository.map')));
			var actions = el('div', 'rm-overview-actions');
			if (hasArchitecture) {
				var architecture = txt('button', 'rm-secondary-action', msg('main.full.architecture'));
				architecture.type = 'button';
				architecture.onclick = function () { openArchitectureTarget(null, null); };
				actions.appendChild(architecture);
			}
			exploreSection.appendChild(actions);
			root.appendChild(exploreSection);
		}
	}

	function renderStudyDetailWorkspace() {
		var root = document.getElementById('rm-study-detail');
		if (!root) return;
		root.replaceChildren();
		var direction = studyDirectionByID(workspaceState.directionID);
		if (!direction) return;
		var incomplete = !!direction.incomplete;
		var back = txt('button', 'rm-secondary-action rm-study-back', msg('main.all.study.directions'));
		back.type = 'button';
		back.onclick = function () { navigateWorkspace('study_overview'); };
		root.appendChild(back);
			root.appendChild(renderViewHeading(
				incomplete ? msg('main.study.incomplete_direction') : msg('main.reading.path'),
				direction.question,
				direction.why_it_matters
		));
		if (direction.learning_outcome) {
			var outcome = el('aside', 'rm-study-outcome');
			outcome.appendChild(txt('span', '', incomplete ? msg('main.what.you.can.learn') : msg('main.after.this.reading.path')));
			outcome.appendChild(txt('strong', '', direction.learning_outcome));
			root.appendChild(outcome);
		}
		if (incomplete) {
			var incompleteAnchorCount = Array.isArray(direction.reading_anchors) ? direction.reading_anchors.length : 0;
				root.appendChild(txt(
					'p',
					'rm-study-order-note',
					msg('main.study.incomplete_places_copy', { count: incompleteAnchorCount })
				));
		} else {
			root.appendChild(txt('p', 'rm-study-order-note', msg('main.these.anchors.are.ordered.for.learning.they.do.not.claim.runtime.execution.order')));
		}
		var start = studyStartReference(direction);
		if (start) {
			var startActions = el('div', 'rm-study-start-actions');
			var startButton = sourceActionElement(
				msg('main.action.start_with', { target: start.label }),
				'rm-primary-action rm-source-action-link',
				start.location,
				0,
				function () { openSourceLocation(start.location); }
			);
			startActions.appendChild(startButton);
			root.appendChild(startActions);
		}
		root.appendChild(renderViewHeading(
				'',
				incomplete
					? ((direction.reading_anchors || []).length === 1
						? msg('main.study.exact_place_to_start')
						: msg('main.exact.places.to.start'))
					: msg('main.you.will.inspect'),
			msg('main.open.an.anchor.at.its.exact.repository.location')
		));
		var anchors = el('div', 'rm-study-reading-list');
		(direction.reading_anchors || []).forEach(function (reading, index) {
			var card = renderStudyReadingAnchor(reading, index);
			if (card) anchors.appendChild(card);
		});
		root.appendChild(anchors);

		var related = el('section', 'rm-workspace-section rm-study-related');
		related.appendChild(renderViewHeading(msg('main.continue.exploring'), msg('main.related.repository.context'), msg('main.open.a.referenced.document.or.inspect.the.full.architecture')));
		var actions = el('div', 'rm-overview-actions');
		(direction.documents || []).forEach(function (document) {
			var documentLocation = document && (document.location || sourceSnippetLocation(document.source));
			if (!document || !repositoryLocationAvailable(documentLocation)) return;
			var button = sourceActionElement(
				studyDocumentActionLabel(document),
				'rm-secondary-action rm-source-action-link',
				documentLocation,
				0,
				function () { openSourceLocation(documentLocation); }
			);
			actions.appendChild(button);
		});
		if (userArchitectureAvailable()) {
			var areaTarget = null;
			(direction.areas || []).some(function (area) {
				if (area && area.map_target) { areaTarget = area.map_target; return true; }
				return false;
			});
			var architecture = txt('button', 'rm-secondary-action', msg('main.architecture'));
			architecture.type = 'button';
			architecture.onclick = function () {
				openArchitectureTarget(areaTarget, { directionID: direction.id });
			};
			actions.appendChild(architecture);
		}
		related.appendChild(actions);
		root.appendChild(related);
	}

	function firstSourcedStudyDocument(direction) {
		var documents = direction && Array.isArray(direction.documents) ? direction.documents : [];
		for (var index = 0; index < documents.length; index++) {
			if (documents[index] && sourceSnippetAvailable(documents[index].source)) return documents[index];
		}
		return null;
	}

	function studyDocumentActionLabel(document) {
		if (!document) return msg('main.open.documentation');
		var location = document.location || sourceSnippetLocation(document.source);
		var path = location && location.path || '';
		var label = document.label || msg('main.open.documentation');
		return path ? label + ' · ' + path : label;
	}

	function firstSourcedStudyAnchor(direction) {
		var anchors = direction && Array.isArray(direction.reading_anchors) ? direction.reading_anchors : [];
		for (var index = 0; index < anchors.length; index++) {
			if (anchors[index] && sourceSnippetAvailable(anchors[index].source)) return anchors[index];
		}
		return null;
	}

	function studyStartReference(direction) {
		if (!direction) return null;
		var documentFirst = direction.target_user_job === 'first_contact' ||
			direction.target_user_job === 'use_or_operate';
		var documentReference = documentFirst ? firstSourcedStudyDocument(direction) : null;
		if (documentReference) {
			return {
				kind: 'document',
				label: studyDocumentActionLabel(documentReference),
				location: documentReference.location || sourceSnippetLocation(documentReference.source),
				source: documentReference.source,
			};
		}
		var reading = firstSourcedStudyAnchor(direction);
		if (!reading) return null;
		return {
			kind: 'anchor',
			label: reading.source.enclosing_symbol ||
				(reading.label ? studyReadingLabel(reading.label) : reading.location.path),
			location: reading.location || sourceSnippetLocation(reading.source),
			source: reading.source,
		};
	}

	function renderStudyReadingAnchor(reading, index) {
		if (!reading || !sourceSnippetAvailable(reading.source)) return null;
		var location = reading.location || sourceSnippetLocation(reading.source);
		var card = el('article', 'rm-study-reading-anchor');
		card.appendChild(txt('span', 'rm-study-reading-anchor__order', String(index + 1)));
		var copy = el('div', 'rm-study-reading-anchor__copy');
		if (reading.label) {
			copy.appendChild(txt(
				'span',
				'rm-study-reading-anchor__label',
				studyReadingLabel(reading.label)
			));
		}
		copy.appendChild(txt('strong', '', reading.source.enclosing_symbol || location.path));
		copy.appendChild(renderFileReference(
			location.path,
			'rm-study-reading-anchor__location',
			location.line || 0,
			formatCodeLocation(location)
		));
		if (reading.what_to_look_for) copy.appendChild(txt('p', '', reading.what_to_look_for));
		card.appendChild(copy);
		var open = sourceActionElement(
			staticSourceMode() ? staticSourceOpenLabel() : msg('main.open.exact.source'),
			'rm-secondary-action rm-study-reading-anchor__open rm-source-action-link',
			location,
			Number(reading.source.end_line) || 0,
			function () { openSourceLocation(location); }
		);
		card.appendChild(open);
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
			card.appendChild(source);
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
				heading.appendChild(showSource);
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

  function renderOverviewWorkspace() {
    var root = document.getElementById('rm-overview');
    if (!root) return;
    root.replaceChildren();
		renderStudyPublicationNotice(root);
		if (STUDY_MAP && COMPLETE_STUDY_DIRECTIONS.length) {
			renderStudyMapOverview(root);
			return;
		}
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
    hero.appendChild(txt('p', '', overviewPurpose || msg('main.explore.how.this.repository.is.organized.and.implemented')));
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
		if (!root || !STUDY_PUBLICATION || STUDY_PUBLICATION.state === 'published') return;
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
    card.appendChild(show);
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
			return;
		}
		var snippet = embeddedSourceForLocation(location);
		if (snippet) {
			openSourceSnippet(snippet, location);
		}
	}

  function openSourceSnippet(snippet, location, expanded) {
    var resolved = sourceSnippetLocation(snippet, location);
    if (openStaticSource(resolved, snippet && snippet.end_line)) return;
    if (!sourceSnippetHasCode(snippet) || !OPENABLE_PATH_SET[snippet.path]) return;
    var next = reduceWorkspaceState(workspaceState, {
      type: 'open_source',
      selection: {
        path: snippet.path,
        line: resolved.line,
        column: resolved.column,
        snippet: snippet,
        expanded: !!expanded,
      },
    }, USER_MECHANISMS);
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
    if (staticSourceMode()) {
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
			componentStudyDirections.forEach(function (direction, directionIndex) {
				if (!direction || !direction.id) return;
				var matchedAnchors = (direction.reading_anchors || []).filter(function (anchor) {
					var location = anchor && (anchor.location || (anchor.source && {
						path: anchor.source.path,
						line: anchor.source.start_line,
					}));
					return !!(location && componentFiles[String(location.path || '')]);
				});
				if (!matchedAnchors.length) return;
				matches.push({ direction: direction, directionIndex: directionIndex, anchors: matchedAnchors });
			});
			matches.sort(function (left, right) {
				return right.anchors.length - left.anchors.length ||
					left.directionIndex - right.directionIndex ||
					String(left.direction.id).localeCompare(String(right.direction.id));
			});

				var sourceSeen = {};
				var sources = [];
				var hasPreciseMemberSource = false;
				(component.members || []).forEach(function (member) {
					(member && member.facts || []).forEach(function (fact) {
						var location = fact && fact.location;
						var filePath = String(location && location.path || '');
						if (!filePath || !OPENABLE_PATH_SET[filePath]) return;
						var line = Number(location.line) || 0;
						if (line <= 0) return;
						var key = filePath + '\u0000' + String(line);
						if (sourceSeen[key]) return;
						sourceSeen[key] = true;
						hasPreciseMemberSource = true;
						sources.push({
							label: member.name || filePath,
							detail: member.name || filePath,
							location: {
								path: filePath,
								line: line,
								column: Number(location.column) || 0,
							},
							priority: line > 0 ? 0 : 3,
						});
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
				(component.anchor_ids || []).forEach(function (anchorID) {
					var anchor = behaviorAnchorByID[String(anchorID || '')];
					var location = anchor && anchor.location;
					var filePath = String(location && location.path || '');
					if (!filePath || !OPENABLE_PATH_SET[filePath]) return;
					if (hasPreciseMemberSource) return;
					if (hasExactComponentFiles && !componentFiles[filePath]) return;
					var line = Number(location.line) || 0;
					var key = filePath + '\u0000' + String(line);
					if (sourceSeen[key]) return;
					sourceSeen[key] = true;
					sources.push({
						label: anchor.label || filePath,
						detail: anchor.label || filePath,
						location: {
							path: filePath,
							line: line,
							column: Number(location.column) || 0,
						},
						priority: 1,
					});
				});
			sources.sort(function (left, right) {
				var leftLine = Number(left && left.location && left.location.line) || 0;
				var rightLine = Number(right && right.location && right.location.line) || 0;
				return Number(left && left.priority) - Number(right && right.priority) ||
					String(left && left.location && left.location.path || '').localeCompare(
						String(right && right.location && right.location.path || '')
					) ||
					leftLine - rightLine;
			});
			sources.forEach(function (source) {
				delete source.priority;
			});

			var packageTargets = packagePaths.map(function (packagePath) {
				var packageFileSet = {};
				var packageFiles = (packageByPath[packagePath].files || []).map(String).filter(function (filePath) {
					if (!filePath || !OPENABLE_PATH_SET[filePath]) return false;
					packageFileSet[filePath] = true;
					return true;
				}).sort();
				var source = sources.find(function (candidate) {
					var filePath = String(candidate && candidate.location && candidate.location.path || '');
					return !!packageFileSet[filePath];
				});
				var location = source && source.location ? source.location : (
					packageFiles.length ? { path: packageFiles[0], line: 0 } : null
				);
				return {
					path: packagePath,
					file_count: packageFiles.length,
					location: location,
					actionable: sourceLocationActionAvailable(location),
				};
			});

			if (!packagePaths.length && !sources.length && !surfaceStarts.length && !matches.length) return;
			contexts[String(component.id)] = {
				package_paths: packagePaths,
				package_targets: packageTargets,
				file_count: Object.keys(componentFiles).length,
				sources: sources,
				surface_starts: surfaceStarts,
				studies: matches.map(function (match) {
					return {
						id: match.direction.id,
						question: match.direction.question,
						why_it_matters: match.direction.why_it_matters || '',
					};
				}),
			};
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
    if (DATA.architecture_canvas && window.RepomapArchitectureCanvas) {
      var card = el('section', 'rm-card rm-architecture-canvas-card');
      architectureCanvasHost = el('div', 'rm-architecture-canvas-host');
      card.appendChild(architectureCanvasHost);
      root.appendChild(card);
    } else {
      var systemMap = renderUserSystemMap(DATA.high_level_map || []);
      if (systemMap) root.appendChild(systemMap);
      else root.appendChild(txt('p', 'rm-empty-state', msg('main.chrome.open.one.of.the.suggested.source.files')));
    }
    renderArchitectureReturn();
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
				? (STUDY_DIRECTIONS.length ? 'study_overview' : 'overview')
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
		if (workspaceState.view === 'study') renderStudyDetailWorkspace();
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
    details.hidden = !DEBUG_MODE;
    if (!DEBUG_MODE) return;
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
        state: String(DATA.freshness.state || 'unavailable'),
      });
    }
    var submodules = DATA.repository_submodules || [];
    if (submodules.length) {
      document.getElementById('rm-submodule-detail').textContent = msg('main.run.excluded_submodules', {
        count: submodules.length,
      });
    }
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

    var tabs = document.getElementById('rm-tabs');
    if (tabs) {
      tabs.replaceChildren();
			if (TASK_INVESTIGATION) {
				addWorkspaceTab(msg('main.task'), 'investigate');
			} else {
				addWorkspaceTab(msg('main.overview'), 'overview');
				if (USER_MECHANISMS.length) addWorkspaceTab(msg('main.mechanisms'), 'mechanisms');
				if (STUDY_DIRECTIONS.length) addWorkspaceTab(msg('main.study'), 'study_overview');
				if (userArchitectureAvailable()) addWorkspaceTab(msg('main.architecture'), 'architecture');
			}
      if (DEBUG_MODE) addWorkspaceTab(msg('main.provenance'), 'provenance');
    }

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
		studyStartReference: studyStartReference,
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
