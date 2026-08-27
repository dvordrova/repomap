package report

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportLoaderKeepsBrowserPayloadVersionsIndependent(t *testing.T) {
	if !strings.Contains(reportLoaderJS, "var REPOSITORY_PAYLOAD_VERSION = 1;") ||
		!strings.Contains(reportLoaderJS, "var TARGET_PAYLOAD_VERSION = 1;") ||
		strings.Contains(reportLoaderJS, "var PAYLOAD_VERSION =") {
		t.Fatal("report loader collapsed independent repository and target payload versions")
	}
}

func TestReportLoaderTransportContract(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}

	loaderPath, err := filepath.Abs(filepath.Join("templates", "report_loader.js"))
	if err != nil {
		t.Fatal(err)
	}
	runnerPath := filepath.Join(t.TempDir(), "report_loader_contract.js")
	if err := os.WriteFile(runnerPath, []byte(reportLoaderContractRunner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, loaderPath).CombinedOutput()
	if err != nil {
		t.Fatalf("run report loader contract: %v\n%s", err, output)
	}
}

const reportLoaderContractRunner = `
const fs = require('fs');
const vm = require('vm');
const zlib = require('zlib');
const nodeCrypto = require('crypto');

const failures = [];
function check(condition, message) {
  if (!condition) failures.push(message);
}

function sha256(bytes) {
  return nodeCrypto.createHash('sha256').update(bytes).digest('hex');
}

function targetJSON(programTargetID, marker) {
  return JSON.stringify({
    version: 1,
    target: {
      id: programTargetID, language: 'go', kind: 'library',
      name: programTargetID, selector: 'go:' + programTargetID
    },
    openable_paths: [],
    features: { marker }
  });
}

function targetChunk(elementID, raw) {
  const rawBytes = Buffer.from(raw, 'utf8');
  const compressed = zlib.gzipSync(rawBytes, { level: 9, mtime: 0 });
  return {
    reference: {
      element_id: elementID,
      encoding: 'gzip+base64',
      raw_byte_length: rawBytes.length,
      compressed_byte_length: compressed.length,
      sha256: sha256(rawBytes)
    },
    encoded: compressed.toString('base64')
  };
}

function fixture() {
  const repositoryRaw = JSON.stringify({
    version: 1,
    repository: {
      name: 'etcd λ', captured_revision: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
    },
    source: { kind: 'none' },
    logical_default_selected_target_id: 'selected-a',
    targets: [], openable_paths: [], warnings: []
  });
  const repositoryBytes = Buffer.from(repositoryRaw, 'utf8');
  const firstRaw = targetJSON('program-a', 'FIRST');
  const secondRaw = targetJSON('program-b', 'SECOND');
  const first = targetChunk('rm-target-chunk-0', firstRaw);
  const second = targetChunk('rm-target-chunk-1', secondRaw);
  const index = {
    version: 4,
    repository: {
      element_id: 'rm-repository-payload',
      encoding: 'identity-json',
      raw_byte_length: repositoryBytes.length,
      sha256: sha256(repositoryBytes)
    },
    logical_default_target_id: 'selected-a',
    targets: [
      {
        target_id: 'selected-a',
        program_target_id: 'program-a',
        state: 'analyzed',
        chunk: first.reference
      },
      {
        target_id: 'selected-b',
        program_target_id: 'program-b',
        state: 'analyzed',
        chunk: second.reference
      },
      { target_id: 'selected-failed', state: 'not_analyzed' }
    ]
  };
  return {
    index,
    repositoryRaw,
    firstRaw,
    secondRaw,
    elements: {
      'rm-repository-payload': repositoryRaw,
      'rm-target-chunk-0': first.encoded,
      'rm-target-chunk-1': second.encoded
    }
  };
}

function replaceTargetRaw(spec, targetIndex, raw) {
  const row = spec.index.targets[targetIndex];
  const replacement = targetChunk(row.chunk.element_id, raw);
  row.chunk = replacement.reference;
  spec.elements[row.chunk.element_id] = replacement.encoded;
  if (targetIndex === 0) spec.firstRaw = raw;
  if (targetIndex === 1) spec.secondRaw = raw;
}

const parseEvents = [];
let decompressions = 0;
class CountingDecompressionStream {
  constructor(format) {
    decompressions += 1;
    return new DecompressionStream(format);
  }
}
const instrumentedJSON = {
  parse(value) {
    parseEvents.push(value);
    return JSON.parse(value);
  }
};
const context = {
  JSON: instrumentedJSON,
  TextEncoder,
  TextDecoder,
  Blob,
  Response,
  DecompressionStream: CountingDecompressionStream,
  crypto: nodeCrypto.webcrypto,
  atob(value) { return Buffer.from(value, 'base64').toString('binary'); },
  btoa(value) { return Buffer.from(value, 'binary').toString('base64'); }
};
context.globalThis = context;
vm.runInNewContext(fs.readFileSync(process.argv[2], 'utf8'), context, {
  filename: process.argv[2]
});
const loader = context.RepomapReportLoader;

function createScenario(mutate) {
  const spec = fixture();
  if (mutate) mutate(spec);
  const indexRaw = spec.indexRawOverride == null ? JSON.stringify(spec.index) : spec.indexRawOverride;
  const elements = Object.assign({ 'rm-bundle-index': indexRaw }, spec.elements);
  const reads = Object.create(null);
  const nodes = Object.keys(elements).map((id) => ({ id, textContent: elements[id] }));
  (spec.duplicateIDs || []).forEach((id) => nodes.unshift({ id, textContent: '{}' }));
  const document = {
    getElementById(id) {
      reads[id] = (reads[id] || 0) + 1;
      return nodes.find((node) => node.id === id) || null;
    },
    querySelectorAll(selector) { return selector === '[id]' ? nodes : []; }
  };
  return {
    spec,
    indexRaw,
    reads,
    create() { return loader.create(document, { search: '', hash: '#/repository' }); }
  };
}

function parseCount(raw) {
  return parseEvents.filter((value) => value === raw).length;
}

async function expectReject(promise, label) {
  try {
    await promise;
    failures.push(label + ' did not reject');
  } catch (error) {
    const message = error && error.message ? error.message : String(error);
    check(message.startsWith('Report loader: '), label + ' exposed an unbounded native error');
    check(message.length <= 180, label + ' error was not bounded');
  }
}

function expectCreateReject(mutate, label) {
  try {
    createScenario(mutate).create();
    failures.push(label + ' did not reject');
  } catch (error) {
    const message = error && error.message ? error.message : String(error);
    check(message.startsWith('Report loader: '), label + ' exposed an unbounded native error');
    check(message.length <= 180, label + ' error was not bounded');
  }
}

(async function run() {
  const primary = createScenario();
  const source = primary.create();
  check(primary.reads['rm-bundle-index'] === 1, 'create must read the bundle index exactly once');
  check(!primary.reads['rm-repository-payload'], 'create must not read repository JSON');
  check(!primary.reads['rm-target-chunk-0'] && !primary.reads['rm-target-chunk-1'],
    'create must not read target chunks');
  check(parseCount(primary.indexRaw) === 1, 'bundle index must be parsed exactly once');

  const repository = await source.loadRepository();
  check(repository.repository.name === 'etcd λ', 'repository JSON was not restored');
  check(primary.reads['rm-repository-payload'] === 1, 'repository element must be read once');
  check(!primary.reads['rm-target-chunk-0'] && !primary.reads['rm-target-chunk-1'],
    'repository load must perform zero target chunk reads');
  check(decompressions === 0, 'repository load must perform zero target decompressions');
  check(parseCount(primary.spec.repositoryRaw) === 1, 'repository JSON must be parsed once');
  const repositoryAgain = await source.loadRepository();
  check(repositoryAgain === repository, 'repository result must be cached by identity');
  check(primary.reads['rm-repository-payload'] === 1 && parseCount(primary.spec.repositoryRaw) === 1,
    'repository cache must avoid a second read and parse');

  const ordinarySibling = createScenario((spec) => {
    spec.index.targets = [spec.index.targets[1]];
    spec.index.targets[0].chunk.element_id = 'rm-target-chunk-0';
    spec.elements['rm-target-chunk-0'] = spec.elements['rm-target-chunk-1'];
  });
  const ordinarySiblingRepository = await ordinarySibling.create().loadRepository();
  check(ordinarySiblingRepository.logical_default_selected_target_id === 'selected-a',
    'one-target ordinary transport must retain a logical default outside its local chunk index');

  const [first, firstConcurrent] = await Promise.all([
    source.loadTarget('selected-a'), source.loadTarget('selected-a')
  ]);
  check(first.features.marker === 'FIRST' && firstConcurrent === first,
    'selected target must decode once and share the cached result');
  check(primary.reads['rm-target-chunk-0'] === 1, 'selected target chunk must be read exactly once');
  check(!primary.reads['rm-target-chunk-1'], 'neighbor target chunk must not be read');
  check(decompressions === 1, 'selected target must be decompressed exactly once');
  check(parseCount(primary.spec.firstRaw) === 1, 'selected target JSON must be parsed exactly once');
  const firstAgain = await source.loadTarget('selected-a');
  check(firstAgain === first && primary.reads['rm-target-chunk-0'] === 1 &&
    parseCount(primary.spec.firstRaw) === 1, 'target cache must avoid a second read and parse');
  check(decompressions === 1, 'target cache must avoid a second decompression');

  await expectReject(source.loadTarget('selected-failed'), 'not-analyzed target');
  check(!primary.reads['rm-target-chunk-1'], 'failed target must not disturb a neighboring chunk');
  check(await source.loadRepository() === repository,
    'failed target must not disturb the cached repository result');
  check((await source.loadTarget('selected-a')) === first,
    'failed target must not disturb a successful target cache');
  check(decompressions === 1, 'failed target must not trigger or disturb decompression');
  await expectReject(source.loadTarget('unknown-target'), 'unknown target');

  await expectReject(createScenario((spec) => {
    spec.index.repository.raw_byte_length += 1;
  }).create().loadRepository(), 'repository raw length corruption');
  await expectReject(createScenario((spec) => {
    spec.index.repository.sha256 = '0'.repeat(64);
  }).create().loadRepository(), 'repository digest corruption');
  await expectReject(createScenario((spec) => {
    const raw = JSON.stringify({
      version: 2,
      repository: {
        name: 'etcd λ', captured_revision: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
      },
      source: { kind: 'none' },
      logical_default_selected_target_id: 'selected-a',
      targets: [], openable_paths: [], warnings: []
    });
    spec.repositoryRaw = raw;
    spec.elements['rm-repository-payload'] = raw;
    spec.index.repository.raw_byte_length = Buffer.byteLength(raw);
    spec.index.repository.sha256 = sha256(Buffer.from(raw));
  }).create().loadRepository(), 'repository version mismatch');
  await expectReject(createScenario((spec) => {
    const raw = JSON.stringify({
      version: 1,
      repository: {
        name: 'etcd λ', captured_revision: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
      },
      source: { kind: 'none' },
      logical_default_selected_target_id: 'selected-b',
      targets: [], openable_paths: [], warnings: []
    });
    spec.repositoryRaw = raw;
    spec.elements['rm-repository-payload'] = raw;
    spec.index.repository.raw_byte_length = Buffer.byteLength(raw);
    spec.index.repository.sha256 = sha256(Buffer.from(raw));
  }).create().loadRepository(), 'repository default target mismatch');
  await expectReject(createScenario((spec) => {
    spec.index.targets[0].chunk.raw_byte_length += 1;
  }).create().loadTarget('selected-a'), 'target raw length corruption');
  await expectReject(createScenario((spec) => {
    spec.index.targets[0].chunk.raw_byte_length = 1;
  }).create().loadTarget('selected-a'), 'declared-small gzip expansion');
  await expectReject(createScenario((spec) => {
    spec.index.targets[0].chunk.sha256 = '0'.repeat(64);
  }).create().loadTarget('selected-a'), 'target digest corruption');
  await expectReject(createScenario((spec) => {
    spec.index.targets[0].chunk.compressed_byte_length += 1;
  }).create().loadTarget('selected-a'), 'target compressed length corruption');
  expectCreateReject((spec) => {
    delete spec.index.targets[0].chunk.compressed_byte_length;
  }, 'missing target compressed length');
  expectCreateReject((spec) => {
    spec.index.targets[0].chunk.compressed_byte_length = 0;
  }, 'zero target compressed length');
  await expectReject(createScenario((spec) => {
    spec.elements['rm-target-chunk-0'] = Buffer.from('not gzip', 'utf8').toString('base64');
    spec.index.targets[0].chunk.compressed_byte_length = Buffer.byteLength('not gzip');
  }).create().loadTarget('selected-a'), 'target gzip corruption');
  await expectReject(createScenario((spec) => {
    spec.elements['rm-target-chunk-0'] += '\n';
  }).create().loadTarget('selected-a'), 'non-canonical base64');
  await expectReject(Promise.resolve().then(() => createScenario((spec) => {
    spec.duplicateIDs = ['rm-target-chunk-0'];
  }).create().loadTarget('selected-a')), 'duplicate DOM target element');
  await expectReject(createScenario((spec) => {
    replaceTargetRaw(spec, 0, targetJSON('wrong-program-target', 'WRONG'));
  }).create().loadTarget('selected-a'), 'target identity mismatch');
  await expectReject(createScenario((spec) => {
    const raw = JSON.stringify({
      version: 2,
      target: {
        id: 'program-a', language: 'go', kind: 'library',
        name: 'program-a', selector: 'go:program-a'
      },
      openable_paths: [], features: {}
    });
    replaceTargetRaw(spec, 0, raw);
  }).create().loadTarget('selected-a'), 'target version mismatch');

  expectCreateReject((spec) => { spec.indexRawOverride = '{'; }, 'malformed bundle index JSON');
  expectCreateReject((spec) => { spec.index.unexpected = true; }, 'unknown bundle index field');
  expectCreateReject((spec) => { spec.index.repository.unexpected = true; },
    'unknown repository reference field');
  expectCreateReject((spec) => { spec.index.targets[0].unexpected = true; },
    'unknown target row field');
  expectCreateReject((spec) => { spec.index.targets[0].chunk.unexpected = true; },
    'unknown target chunk reference field');
  expectCreateReject((spec) => { spec.index.targets[2].unexpected = true; },
    'unknown not-analyzed target row field');
  await expectReject(createScenario((spec) => {
    const raw = '{';
    spec.repositoryRaw = raw;
    spec.elements['rm-repository-payload'] = raw;
    spec.index.repository.raw_byte_length = Buffer.byteLength(raw);
    spec.index.repository.sha256 = sha256(Buffer.from(raw));
  }).create().loadRepository(), 'malformed repository JSON');
  await expectReject(createScenario((spec) => {
    const value = JSON.parse(spec.repositoryRaw);
    value.unexpected = true;
    const raw = JSON.stringify(value);
    spec.repositoryRaw = raw;
    spec.elements['rm-repository-payload'] = raw;
    spec.index.repository.raw_byte_length = Buffer.byteLength(raw);
    spec.index.repository.sha256 = sha256(Buffer.from(raw));
  }).create().loadRepository(), 'unknown repository payload field');
  await expectReject(createScenario((spec) => {
    replaceTargetRaw(spec, 0, '{');
  }).create().loadTarget('selected-a'), 'malformed target JSON');
  await expectReject(createScenario((spec) => {
    const value = JSON.parse(spec.firstRaw);
    value.unexpected = true;
    replaceTargetRaw(spec, 0, JSON.stringify(value));
  }).create().loadTarget('selected-a'), 'unknown target payload field');

  expectCreateReject((spec) => { spec.index.version = 5; }, 'unsupported index version');
  expectCreateReject((spec) => {
    spec.index.repository.element_id = 'rm-repository-other';
  }, 'wrong repository element identity');
  expectCreateReject((spec) => {
    spec.index.targets[0].chunk.element_id = 'rm-target-chunk-9';
  }, 'wrong target chunk element identity');
  expectCreateReject((spec) => {
    spec.index.targets[1].program_target_id = spec.index.targets[0].program_target_id;
  }, 'duplicate ProgramTarget identity');
  expectCreateReject((spec) => {
    spec.index.repository.raw_byte_length = (1 << 30) + 1;
  }, 'repository transport byte limit');
  expectCreateReject((spec) => {
    spec.index.repository.raw_byte_length = (1 << 30);
  }, 'aggregate transport byte limit');
  expectCreateReject((spec) => {
    spec.index.targets[0].chunk.compressed_byte_length = (1 << 30) + 1;
  }, 'target compressed transport byte limit');
  expectCreateReject((spec) => {
    spec.index.targets[0].chunk.raw_byte_length = (64 << 20) + 1;
  }, 'target canonical raw byte limit');
  expectCreateReject((spec) => {
    spec.index.repository.encoding = 'gzip+base64';
  }, 'unsupported repository encoding');
  expectCreateReject((spec) => {
    spec.index.targets[0].chunk.encoding = 'identity-json';
  }, 'unsupported target encoding');

  if (failures.length) {
    throw new Error(failures.join('\n'));
  }
})().catch((error) => {
  process.stderr.write((error && error.stack ? error.stack : String(error)) + '\n');
  process.exitCode = 1;
});
`
