(function (root) {
  'use strict';

  var INDEX_ELEMENT_ID = 'rm-bundle-index';
  var INDEX_VERSION = 4;
  var REPOSITORY_PAYLOAD_VERSION = 1;
  var TARGET_PAYLOAD_VERSION = 1;
  var MAX_ERROR_MESSAGE_CHARS = 160;
  var MAX_TRANSPORT_BYTES = 1 << 30;
  // A target browser projection is derived from one canonical report.json,
  // whose production envelope is 64 MiB. The aggregate bundle keeps its
  // independent 1 GiB ceiling; this bound protects one browser decode.
  var MAX_TARGET_RAW_BYTES = 64 << 20;
  var MAX_TARGET_COMPRESSED_BYTES = MAX_TARGET_RAW_BYTES + (1 << 20);
  var MAX_BASE64_CHARACTERS = Math.floor((MAX_TARGET_COMPRESSED_BYTES + 2) / 3) * 4;

  function fail(message) {
    var bounded = String(message || 'transport failure');
    if (bounded.length > MAX_ERROR_MESSAGE_CHARS) {
      bounded = bounded.slice(0, MAX_ERROR_MESSAGE_CHARS);
    }
    throw new Error('Report loader: ' + bounded);
  }

  function record(value, label) {
    if (!value || typeof value !== 'object' || Array.isArray(value)) {
      fail(label + ' must be an object.');
    }
    return value;
  }

  function exactFields(value, required, optional, label) {
    value = record(value, label);
    var allowed = Object.create(null);
    required.concat(optional || []).forEach(function (field) { allowed[field] = true; });
    Object.keys(value).forEach(function (field) {
      if (!allowed[field]) fail(label + ' contains an unknown field.');
    });
    required.forEach(function (field) {
      if (!Object.prototype.hasOwnProperty.call(value, field)) {
        fail(label + ' is missing a required field.');
      }
    });
    return value;
  }

  function exactText(value, label) {
    if (typeof value !== 'string' || !value || value.trim() !== value) {
      fail(label + ' must be exact non-empty text.');
    }
    return value;
  }

  function nonNegativeInteger(value, label) {
    if (!Number.isSafeInteger(value) || value < 0) {
      fail(label + ' must be a non-negative integer.');
    }
    return value;
  }

  function boundedByteLength(value, label) {
    value = nonNegativeInteger(value, label);
    if (value > MAX_TRANSPORT_BYTES) fail(label + ' exceeds the transport limit.');
    return value;
  }

  function positiveBoundedByteLength(value, limit, label) {
    value = boundedByteLength(value, label);
    if (value === 0 || value > limit) fail(label + ' exceeds its payload limit.');
    return value;
  }

  function digest(value, label) {
    value = exactText(value, label);
    if (!/^[0-9a-f]{64}$/.test(value)) fail(label + ' must be a lowercase SHA-256 digest.');
    return value;
  }

  function parseJSON(raw, label) {
    var value;
    try {
      value = JSON.parse(raw);
    } catch (_error) {
      fail(label + ' is not valid JSON.');
    }
    return record(value, label);
  }

  function elementText(documentObject, elementID, label) {
    var matches = documentObject.querySelectorAll('[id]');
    var element = documentObject.getElementById(elementID);
    var matchCount = 0;
    for (var index = 0; index < matches.length; index += 1) {
      if (matches[index].id !== elementID) continue;
      matchCount += 1;
    }
    if (matchCount !== 1 || !element || typeof element.textContent !== 'string') {
      fail(label + ' element must occur exactly once.');
    }
    return element.textContent;
  }

  function byteLength(value, label) {
    if (typeof root.TextEncoder !== 'function') fail('UTF-8 decoding is unavailable.');
    try {
      return new root.TextEncoder().encode(value);
    } catch (_error) {
      fail(label + ' could not be encoded as UTF-8.');
    }
  }

  function hex(bytes) {
    return Array.prototype.map.call(new Uint8Array(bytes), function (value) {
      return value.toString(16).padStart(2, '0');
    }).join('');
  }

  async function verifyRawBytes(bytes, reference, label) {
    if (bytes.byteLength !== reference.rawByteLength) fail(label + ' raw byte length does not match its index.');
    if (!root.crypto || !root.crypto.subtle || typeof root.crypto.subtle.digest !== 'function') {
      fail('SHA-256 verification is unavailable.');
    }
    var actual;
    try {
      actual = hex(await root.crypto.subtle.digest('SHA-256', bytes));
    } catch (_error) {
      fail('SHA-256 verification failed.');
    }
    if (actual !== reference.sha256) fail(label + ' SHA-256 does not match its index.');
  }

  function identityReference(value, label) {
    value = exactFields(value, [
      'element_id', 'encoding', 'raw_byte_length', 'sha256'
    ], [], label);
    if (value.encoding !== 'identity-json') fail(label + ' encoding is not supported.');
    var reference = Object.freeze({
      elementID: exactText(value.element_id, label + '.element_id'),
      rawByteLength: positiveBoundedByteLength(
        value.raw_byte_length, MAX_TRANSPORT_BYTES, label + '.raw_byte_length'
      ),
      sha256: digest(value.sha256, label + '.sha256')
    });
    if (reference.elementID !== 'rm-repository-payload') {
      fail(label + ' element identity is invalid.');
    }
    return reference;
  }

  function chunkReference(value, label) {
    value = exactFields(value, [
      'element_id', 'encoding', 'raw_byte_length', 'compressed_byte_length', 'sha256'
    ], [], label);
    if (value.encoding !== 'gzip+base64') fail(label + ' encoding is not supported.');
    if (!Object.prototype.hasOwnProperty.call(value, 'compressed_byte_length')) {
      fail(label + '.compressed_byte_length is required.');
    }
    return Object.freeze({
      elementID: exactText(value.element_id, label + '.element_id'),
      rawByteLength: positiveBoundedByteLength(
        value.raw_byte_length, MAX_TARGET_RAW_BYTES, label + '.raw_byte_length'
      ),
      compressedByteLength: positiveBoundedByteLength(
        value.compressed_byte_length,
        MAX_TARGET_COMPRESSED_BYTES,
        label + '.compressed_byte_length'
      ),
      sha256: digest(value.sha256, label + '.sha256')
    });
  }

  function strictBase64(value, expectedByteLength, label) {
    if (typeof value !== 'string' || !value || value.length > MAX_BASE64_CHARACTERS ||
        value.length !== Math.ceil(expectedByteLength / 3) * 4 ||
        value.length % 4 !== 0 ||
        !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(value)) {
      fail(label + ' is not canonical base64.');
    }
    if (typeof root.atob !== 'function' || typeof root.btoa !== 'function') {
      fail('Base64 decoding is unavailable.');
    }
    var binary;
    try {
      binary = root.atob(value);
      if (root.btoa(binary) !== value) fail(label + ' is not canonical base64.');
    } catch (_error) {
      fail(label + ' is not canonical base64.');
    }
    var result = new Uint8Array(binary.length);
    for (var index = 0; index < binary.length; index += 1) {
      result[index] = binary.charCodeAt(index);
    }
    return result;
  }

  async function gunzip(bytes, expectedRawByteLength) {
    if (typeof root.DecompressionStream !== 'function' || typeof root.Blob !== 'function') {
      fail('Gzip decoding is unavailable.');
    }
    var reader;
    try {
      var stream = new root.Blob([bytes]).stream().pipeThrough(new root.DecompressionStream('gzip'));
      reader = stream.getReader();
    } catch (_error) {
      fail('Target chunk is not valid gzip.');
    }
    var raw;
    try {
      raw = new Uint8Array(expectedRawByteLength);
    } catch (_error) {
      fail('Target JSON buffer is unavailable.');
    }
    var total = 0;
    var overflow = false;
    try {
      while (true) {
        var result = await reader.read();
        if (result.done) break;
        var chunk = result.value;
        if (!chunk || !Number.isSafeInteger(chunk.byteLength) || chunk.byteLength < 0) {
          throw new Error('invalid decompression chunk');
        }
        if (chunk.byteLength > expectedRawByteLength - total) {
          overflow = true;
          try { await reader.cancel(); } catch (_cancelError) { /* keep the bounded length error */ }
          break;
        }
        raw.set(chunk, total);
        total += chunk.byteLength;
      }
    } catch (_error) {
      fail('Target chunk is not valid gzip.');
    }
    if (overflow || total !== expectedRawByteLength) {
      fail('Target JSON raw byte length does not match its index.');
    }
    return raw;
  }

  function decodeUTF8(bytes, label) {
    if (typeof root.TextDecoder !== 'function') fail('UTF-8 decoding is unavailable.');
    try {
      return new root.TextDecoder('utf-8', { fatal: true }).decode(bytes);
    } catch (_error) {
      fail(label + ' is not valid UTF-8.');
    }
  }

  function repositoryTransportMinimum(value, logicalDefaultTargetID) {
    value = exactFields(value, [
      'version', 'repository', 'source', 'logical_default_selected_target_id',
      'targets', 'openable_paths'
    ], ['runtime', 'warnings'], 'repository JSON');
    if (value.version !== REPOSITORY_PAYLOAD_VERSION) fail('Repository JSON version is not supported.');
    if (value.logical_default_selected_target_id !== logicalDefaultTargetID) {
      fail('Repository JSON default target does not match its index.');
    }
    return value;
  }

  function targetTransportMinimum(value, target) {
    value = exactFields(value, [
      'version', 'target', 'openable_paths', 'features'
    ], [], 'target JSON');
    if (value.version !== TARGET_PAYLOAD_VERSION) fail('Target JSON version is not supported.');
    var identity = exactFields(value.target, [
      'id', 'language', 'kind', 'name', 'selector'
    ], [], 'target JSON target');
    if (identity.id !== target.programTargetID) {
      fail('Target JSON identity does not match its index.');
    }
    return value;
  }

  function targetRows(values) {
    if (!Array.isArray(values) || !values.length) fail('index.targets must be a non-empty array.');
    var byID = Object.create(null);
    var byProgramTargetID = Object.create(null);
    var rows = values.map(function (raw, index) {
      var label = 'index.targets[' + String(index) + ']';
      raw = record(raw, label);
      var targetID = exactText(raw.target_id, label + '.target_id');
      if (byID[targetID]) fail('index target identities are not unique.');
      var state = exactText(raw.state, label + '.state');
      if (state === 'analyzed') {
        raw = exactFields(raw, [
          'target_id', 'program_target_id', 'state', 'chunk'
        ], [], label);
      } else if (state === 'not_analyzed') {
        raw = exactFields(raw, ['target_id', 'state'], [], label);
      } else {
        fail('target state is not supported.');
      }
      var programTargetID = raw.program_target_id == null ? null :
        exactText(raw.program_target_id, label + '.program_target_id');
      var chunk = raw.chunk == null ? null : chunkReference(raw.chunk, label + '.chunk');
      if (state === 'analyzed') {
        if (!programTargetID || !chunk) fail('analyzed target transport is incomplete.');
        if (byProgramTargetID[programTargetID]) fail('index ProgramTarget identities are not unique.');
        byProgramTargetID[programTargetID] = true;
        if (chunk.elementID !== 'rm-target-chunk-' + String(index)) {
          fail('target chunk element identity is invalid.');
        }
      } else if (state === 'not_analyzed') {
        if (programTargetID || chunk) fail('not-analyzed target unexpectedly has a chunk.');
      }
      var row = Object.freeze({
        targetID: targetID,
        programTargetID: programTargetID,
        state: state,
        chunk: chunk
      });
      byID[targetID] = row;
      return row;
    });
    return { rows: Object.freeze(rows), byID: byID };
  }

  function create(documentObject, locationObject) {
    if (!documentObject || typeof documentObject.getElementById !== 'function' ||
        typeof documentObject.querySelectorAll !== 'function') {
      fail('document is unavailable.');
    }
    if (!locationObject || typeof locationObject !== 'object') fail('location is unavailable.');

    var indexRaw = elementText(documentObject, INDEX_ELEMENT_ID, 'bundle index');
    var index = exactFields(parseJSON(indexRaw, 'bundle index'), [
      'version', 'repository', 'logical_default_target_id', 'targets'
    ], [], 'bundle index');
    if (index.version !== INDEX_VERSION) fail('bundle index version is not supported.');
    var repository = identityReference(index.repository, 'index.repository');
    var targets = targetRows(index.targets);
    var logicalDefaultTargetID = exactText(
      index.logical_default_target_id, 'index.logical_default_target_id'
    );
    var aggregateRawBytes = repository.rawByteLength;
    targets.rows.forEach(function (target) {
      if (!target.chunk) return;
      if (aggregateRawBytes > MAX_TRANSPORT_BYTES - target.chunk.rawByteLength) {
        fail('bundle raw payloads exceed the aggregate transport limit.');
      }
      aggregateRawBytes += target.chunk.rawByteLength;
    });
    var repositoryPromise = null;
    var targetPromises = Object.create(null);

    function loadRepository() {
      if (repositoryPromise) return repositoryPromise;
      repositoryPromise = Promise.resolve().then(async function () {
        var raw = elementText(documentObject, repository.elementID, 'repository JSON');
        var bytes = byteLength(raw, 'repository JSON');
        await verifyRawBytes(bytes, repository, 'Repository JSON');
        return repositoryTransportMinimum(parseJSON(raw, 'repository JSON'), logicalDefaultTargetID);
      });
      return repositoryPromise;
    }

    function loadTarget(targetID) {
      if (typeof targetID !== 'string' || !targetID || targetID.trim() !== targetID) {
        return Promise.reject(new Error('Report loader: target identity is invalid.'));
      }
      var target = targets.byID[targetID];
      if (!target) return Promise.reject(new Error('Report loader: target is absent from the index.'));
      if (targetPromises[targetID]) return targetPromises[targetID];
      targetPromises[targetID] = Promise.resolve().then(async function () {
        if (target.state !== 'analyzed' || !target.chunk) fail('target was not analyzed.');
        var encoded = elementText(documentObject, target.chunk.elementID, 'target chunk');
        var compressed = strictBase64(
          encoded, target.chunk.compressedByteLength, 'target chunk'
        );
        if (compressed.byteLength !== target.chunk.compressedByteLength) {
          fail('Target chunk compressed byte length does not match its index.');
        }
        var rawBytes = await gunzip(compressed, target.chunk.rawByteLength);
        await verifyRawBytes(rawBytes, target.chunk, 'Target JSON');
        var raw = decodeUTF8(rawBytes, 'Target JSON');
        return targetTransportMinimum(parseJSON(raw, 'target JSON'), target);
      });
      return targetPromises[targetID];
    }

    return Object.freeze({
      loadRepository: loadRepository,
      loadTarget: loadTarget
    });
  }

  var api = Object.freeze({ create: create });
  if (root.RepomapReportLoader) fail('asset namespace is already installed.');
  Object.defineProperty(root, 'RepomapReportLoader', {
    value: api,
    enumerable: false,
    configurable: false,
    writable: false
  });
})(globalThis);
