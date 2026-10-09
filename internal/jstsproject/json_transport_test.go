package jstsproject

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gitfiles"
)

func nativeTransportCommand(t *testing.T, script string) *exec.Cmd {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("real Node is required")
	}
	return exec.CommandContext(t.Context(), node, "--input-type=module", "--eval", nodeJSONTransport+"\n"+script)
}

type transportDigest struct {
	hash.Hash
	bytes int64
}

func (digest *transportDigest) Write(value []byte) (int, error) {
	digest.bytes += int64(len(value))
	return digest.Hash.Write(value)
}

func TestNativeJSONTransportExceedsActualV8AggregateStringRepresentation(t *testing.T) {
	command := nativeTransportCommand(t, `
import { constants } from "node:buffer";
const text = "x".repeat(64 * 1024);
const rows = Math.ceil(constants.MAX_STRING_LENGTH / text.length) + 1;
const value = {schema:"native", rows:Array.from({length:rows},(_,id)=>({id,text}))};
let refused = false;
try { JSON.stringify(value); } catch (error) { if (!(error instanceof RangeError) || !error.message.includes("Invalid string length")) throw error; refused = true; }
if (!refused) throw new Error("fixture did not cross the actual V8 representation boundary");
let writes = 0, backpressure = 0, inFlight = 0, maxInFlight = 0, maxChunk = 0;
const nativeWrite = process.stdout.write.bind(process.stdout);
process.stdout.write = (chunk, callback) => {
  writes++; inFlight++; maxInFlight = Math.max(maxInFlight, inFlight); maxChunk = Math.max(maxChunk, chunk.length);
  const accepted = nativeWrite(chunk, error => { inFlight--; callback(error); });
  if (!accepted) backpressure++;
  return accepted;
};
await writeNativeJSON(value,process.stdout);
process.stderr.write(JSON.stringify({rows,textBytes:text.length,stringLimit:constants.MAX_STRING_LENGTH,writes,backpressure,maxInFlight,maxChunk,framing:process.stdout.writableHighWaterMark}));
`)
	actual := &transportDigest{Hash: sha256.New()}
	var diagnostic bytes.Buffer
	command.Stdout, command.Stderr = actual, &diagnostic
	if err := command.Run(); err != nil {
		t.Fatalf("real native transport: %v: %s", err, &diagnostic)
	}
	var observed struct{ Rows, TextBytes, StringLimit, Writes, Backpressure, MaxInFlight, MaxChunk, Framing int }
	if err := json.Unmarshal(diagnostic.Bytes(), &observed); err != nil {
		t.Fatal(err)
	}
	if observed.Rows*observed.TextBytes <= observed.StringLimit || observed.Writes < 2 || observed.Backpressure < 1 || observed.MaxInFlight != 1 || observed.MaxChunk > observed.Framing {
		t.Fatalf("actual representation/backpressure not exercised: %+v", observed)
	}
	want := &transportDigest{Hash: sha256.New()}
	io.WriteString(want, `{"schema":"native","rows":[`)
	text := strings.Repeat("x", observed.TextBytes)
	for id := 0; id < observed.Rows; id++ {
		if id > 0 {
			io.WriteString(want, ",")
		}
		io.WriteString(want, `{"id":`+strconv.Itoa(id)+`,"text":"`)
		io.WriteString(want, text)
		io.WriteString(want, `"}`)
	}
	io.WriteString(want, "]}")
	if actual.bytes != want.bytes || !bytes.Equal(actual.Sum(nil), want.Sum(nil)) {
		t.Fatalf("complete ordered row bytes changed: actual %d/%x want %d/%x", actual.bytes, actual.Sum(nil), want.bytes, want.Sum(nil))
	}
	t.Logf("real V8 limit=%d; complete rows=%d, output=%dB sha256=%s; acknowledged writes=%d, backpressure=%d", observed.StringLimit, observed.Rows, actual.bytes, hex.EncodeToString(actual.Sum(nil)), observed.Writes, observed.Backpressure)
}

func TestNativeJSONTransportPreservesStringifyEncodingOrderAndUnicodeBoundaries(t *testing.T) {
	command := nativeTransportCommand(t, `
import { createHash } from "node:crypto";
const shared = {same:"original"};
// Put the high surrogate at the very last unit of the native stdout frame.
const boundary = "x".repeat(process.stdout.writableHighWaterMark-'{"boundary":"'.length-1)+"😀end";
const value = {boundary,schema:1,omitted:undefined,keys:{10:"ten",2:"two",a:"first",b:"second"},strings:["\r\n\t\"\\\ud800😀"],arrays:[undefined,()=>{},Symbol("unknown"),null,NaN,Infinity,-0],boxed:[new Number(3),new String("text"),new Boolean(false)],date:new Date("2026-10-06T00:00:00Z"),left:shared,right:shared,toJSON:{toJSON(key){return {key}}}};
const expected=JSON.stringify(value);
await writeNativeJSON(value,process.stdout);
process.stderr.write(JSON.stringify({bytes:Buffer.byteLength(expected),sha:createHash("sha256").update(expected).digest("hex")}));
`)
	var actual, diagnostic bytes.Buffer
	command.Stdout, command.Stderr = &actual, &diagnostic
	if err := command.Run(); err != nil {
		t.Fatalf("transport: %v: %s", err, &diagnostic)
	}
	var expected struct {
		Bytes int
		SHA   string
	}
	if err := json.Unmarshal(diagnostic.Bytes(), &expected); err != nil {
		t.Fatal(err)
	}
	sha := sha256.Sum256(actual.Bytes())
	if actual.Len() != expected.Bytes || hex.EncodeToString(sha[:]) != expected.SHA {
		t.Fatalf("native JSON.stringify bytes differ: %d/%x vs %+v", actual.Len(), sha, expected)
	}
	if !json.Valid(actual.Bytes()) {
		t.Fatal("stream did not emit one complete JSON value")
	}
}

func TestNativeJSONTransportClosedPipeFailsAndRunsCleanup(t *testing.T) {
	command := nativeTransportCommand(t, `
try {
  const text="x".repeat(64*1024);
  await writeNativeJSON({rows:Array.from({length:1000},()=>({text}))},process.stdout);
} catch(error) {
  process.stderr.write("transport_failed:"+error.code+"\n"); process.exitCode=1;
} finally { process.stderr.write("cleanup_completed\n"); }
`)
	pipe, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var diagnostic bytes.Buffer
	command.Stderr = &diagnostic
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4096)
	if _, err := io.ReadFull(pipe, buf); err != nil {
		t.Fatal(err)
	}
	if err := pipe.Close(); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err == nil || !strings.Contains(diagnostic.String(), "transport_failed:EPIPE") || !strings.Contains(diagnostic.String(), "cleanup_completed") {
		t.Fatalf("native pipe failure skipped cleanup: %v: %s", err, &diagnostic)
	}
}

func TestNativeJSONTransportInvalidValueRefusesExplicitly(t *testing.T) {
	for _, expression := range []string{"undefined", "1n", "(()=>{const v={};v.self=v;return v})()"} {
		t.Run(expression, func(t *testing.T) {
			command := nativeTransportCommand(t, fmt.Sprintf(`try { await writeNativeJSON(%s,process.stdout); } catch(error) { process.stderr.write(error.message); process.exitCode=1; }`, expression))
			var diagnostic bytes.Buffer
			command.Stderr = &diagnostic
			if err := command.Run(); err == nil || !strings.Contains(diagnostic.String(), "native JSON transport:") {
				t.Fatalf("unknown native representation accepted: %v: %s", err, &diagnostic)
			}
		})
	}
}

func TestCumulativeJSTSStreamPreservesTheCompleteOriginalNativeGraph(t *testing.T) {
	root := preparedCompilerProject(t)
	_, file, _, _ := runtime.Caller(0)
	testdata := filepath.Join(filepath.Dir(file), "../../testdata")
	encoded, err := os.ReadFile(filepath.Join(testdata, "contracts/jsts.files.json"))
	if err != nil {
		t.Fatal(err)
	}
	var inventory struct {
		Entries []struct {
			Path string `json:"path"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(encoded, &inventory); err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, entry := range inventory.Entries {
		source, err := os.ReadFile(filepath.Join(testdata, "repositories/jsts", entry.Path))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, root, entry.Path, string(source))
		paths = append(paths, entry.Path)
	}
	materializeCumulativeJSTSDependencyTypes(t, root)
	repository, err := corpus.New(t.Context(), root, gitfiles.Listing{Paths: paths, RegularPaths: paths})
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	streamed, err := DiscoverSelected(t.Context(), repository, root, "jsts:package.json")
	if err != nil {
		t.Fatal(err)
	}
	// This small complete fixture fits the original serializer. Compare the
	// actual Compiler API producer twice; only its output transport differs.
	original := nodeHelper
	const call = "await writeNativeJSON(result, process.stdout)"
	if strings.Count(original, call) != 1 {
		t.Fatal("owning helper transport call changed")
	}
	t.Cleanup(func() { nodeHelper = original })
	nodeHelper = strings.Replace(original, call, "process.stdout.write(JSON.stringify(result))", 1)
	aggregate, err := DiscoverSelected(t.Context(), repository, root, "jsts:package.json")
	if err != nil {
		t.Fatal(err)
	}
	actual, err := json.Marshal(streamed)
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(aggregate)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, want) {
		t.Fatal("streaming changed complete native records, fields, order or sealed identities")
	}
	actualIndex, actualCatalog, err := BuildFromResult(streamed)
	if err != nil {
		t.Fatal(err)
	}
	wantIndex, wantCatalog, err := BuildFromResult(aggregate)
	if err != nil {
		t.Fatal(err)
	}
	if actualIndex.SHA256 != wantIndex.SHA256 || !reflect.DeepEqual(actualCatalog, wantCatalog) {
		t.Fatal("native transport changed the ordinary sealed adapter graph")
	}
	t.Logf("all %d cumulative source files; identical native output %dB, index %s", len(paths), len(actual), actualIndex.SHA256)
}
