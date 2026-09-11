// Package places builds the atlas graph: every directory and file a reader
// can visit, with the deterministic facts the code knows about it, and the
// file-to-file edges of the program graph. It is pure over its inputs and
// makes no model call.
package places

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/claims"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

const (
	// docstringReach is how many lines above a declaration its docstring may
	// start; the same rule the page uses to put authors' words on cards.
	docstringReach = 12
	// generatedMarkerLines is how deep the generated-code marker is looked
	// for. kubernetes puts it after the license header.
	generatedMarkerLines = 30
	// maxReadBytes bounds what places reads from a file for its facts.
	maxReadBytes = 256 << 10
	// maxLineRunes bounds README and doc lines.
	maxLineRunes = 200
)

// TargetInput is one analyzed target: its program index and, for Go, the
// dependency catalog that carries package imports.
type TargetInput struct {
	Index programindex.Index
	// ReadIndex loads one saved index; Index then carries only its Target.
	ReadIndex    func() (programindex.Index, error)
	Dependencies *dependencies.Catalog
	// Root is the target's root directory, repository-relative.
	Root string
}

func (target TargetInput) read() (TargetInput, error) {
	if target.ReadIndex == nil {
		return target, nil
	}
	index, err := target.ReadIndex()
	if err != nil {
		return TargetInput{}, err
	}
	target.Index = index
	return target, nil
}

// Input is everything places reads.
type Input struct {
	Revision   string
	Repository *corpus.Corpus
	Targets    []TargetInput
	Claims     claims.Result
	// Facts carries the boundaries the code already knows: routes, client
	// calls, listeners, configuration reads, dynamic execution, and the
	// observed launch seeds and manifest values reused by Learn/questions.
	Facts facts.Result
}

// sdkPackages are the client libraries whose calls with a literal argument
// are integration points: a database, a queue, a cloud, a store, a service
// SDK. "Any non-platform package with a literal" was measured on kubernetes
// as 844 field paths and 120 feature-gate names; a closed list of what a
// reader would call an integration is honest and small. Matched by exact
// path or as a prefix with a slash.
var sdkPackages = []string{
	"database/sql", "net",
	"google.golang.org/grpc", "github.com/jackc/pgx", "github.com/jackc/pgconn", "github.com/lib/pq",
	"github.com/go-sql-driver/mysql", "gorm.io", "github.com/jmoiron/sqlx", "github.com/mattn/go-sqlite3",
	"modernc.org/sqlite", "go.mongodb.org/mongo-driver", "github.com/redis/go-redis", "github.com/go-redis/redis",
	"github.com/gomodule/redigo", "github.com/ClickHouse/clickhouse-go", "github.com/segmentio/kafka-go",
	"github.com/IBM/sarama", "github.com/Shopify/sarama", "github.com/confluentinc/confluent-kafka-go",
	"github.com/nats-io/nats.go", "github.com/rabbitmq/amqp091-go", "github.com/streadway/amqp",
	"github.com/apache/pulsar-client-go", "github.com/eclipse/paho.mqtt.golang",
	"github.com/aws/aws-sdk-go", "github.com/aws/aws-sdk-go-v2", "cloud.google.com/go",
	"github.com/Azure/azure-sdk-for-go", "github.com/elastic/go-elasticsearch", "github.com/olivere/elastic",
	"github.com/minio/minio-go", "github.com/hashicorp/vault", "github.com/hashicorp/consul",
	"go.etcd.io/etcd/client", "k8s.io/client-go", "github.com/docker/docker/client", "github.com/moby/moby/client",
	"github.com/go-resty/resty", "github.com/valyala/fasthttp", "github.com/gorilla/websocket", "nhooyr.io/websocket",
	"github.com/slack-go/slack", "github.com/stripe/stripe-go", "github.com/twilio", "github.com/sendgrid",
	"github.com/bwmarrin/discordgo", "gopkg.in/telebot", "github.com/go-telegram-bot-api",
	"github.com/dgraph-io/badger", "go.etcd.io/bbolt", "github.com/syndtr/goleveldb", "github.com/gocql/gocql",
	// Python and JavaScript client libraries, by import name.
	"boto3", "botocore", "pymongo", "motor", "redis", "aioredis", "sqlalchemy", "psycopg2", "psycopg", "asyncpg",
	"pymysql", "mysql", "sqlite3", "kafka", "aiokafka", "confluent_kafka", "pika", "aio_pika", "celery", "stripe",
	"twilio", "slack_sdk", "google.cloud", "azure", "elasticsearch", "cassandra", "hvac", "consul", "kubernetes",
	"docker", "paramiko", "smtplib", "ftplib", "telebot", "aiogram", "discord",
	"@aws-sdk", "aws-sdk", "mongoose", "mongodb", "pg", "mysql2", "ioredis", "kafkajs", "amqplib", "@slack",
	"firebase", "firebase-admin", "@google-cloud", "@azure", "socket.io", "socket.io-client", "ws", "nodemailer",
	"stripe", "twilio", "discord.js", "telegraf", "node-telegram-bot-api", "@elastic/elasticsearch", "cassandra-driver",
}

// sdkNeverPackages are packages whose literal arguments are never an
// integration: format strings, separators, error text, log messages, flag
// names, metric names, assertions. etcd's first atlas found 1,055 of its
// 1,791 "boundaries" in calls to zap.
var sdkNeverPackages = []string{
	"fmt", "strings", "errors", "bytes", "path", "path/filepath", "encoding/json", "io", "os",
	"time", "regexp", "flag", "html/template", "text/template", "sort", "strconv", "unicode", "log",
	"context", "sync", "math", "os/exec", "os/signal", "bufio", "crypto/sha256", "encoding/hex", "runtime",
	"go.uber.org/zap", "go.uber.org/multierr", "github.com/sirupsen/logrus", "k8s.io/klog",
	"github.com/golang/glog", "github.com/rs/zerolog", "github.com/go-logr/logr", "log/slog",
	"google.golang.org/grpc/status", "google.golang.org/grpc/codes", "google.golang.org/grpc/grpclog",
	"github.com/spf13/pflag", "github.com/spf13/cobra", "github.com/spf13/viper", "github.com/urfave/cli",
	"github.com/pkg/errors", "github.com/stretchr/testify", "github.com/onsi/ginkgo", "github.com/onsi/gomega",
	"github.com/prometheus/client_golang", "go.opentelemetry.io/otel", "github.com/google/go-cmp",
	"golang.org/x/exp", "golang.org/x/sync", "golang.org/x/text", "golang.org/x/net/context",
	"github.com/dustin/go-humanize", "github.com/olekukonko/tablewriter", "gopkg.in/yaml", "sigs.k8s.io/yaml",
	"github.com/xiang90/probing", "github.com/coreos/go-semver", "github.com/gogo/protobuf",
	"google.golang.org/protobuf", "github.com/golang/protobuf",
}

var generatedMarker = regexp.MustCompile(`(?i)code generated .* do not edit|do not edit`)

// Build derives the graph. Same inputs give byte-identical output.
func Build(input Input) (atlas.Graph, error) {
	if input.Repository == nil {
		return atlas.Graph{}, fmt.Errorf("atlas places: repository corpus is required")
	}
	if len(input.Targets) == 0 {
		return atlas.Graph{}, fmt.Errorf("atlas places: no targets")
	}
	b := &builder{
		input:             input,
		files:             make(map[string]*fileState),
		dirs:              make(map[string]*dirState),
		byID:              make(map[string]programindex.Object),
		fileOf:            make(map[string]string),
		fanIn:             make(map[string]int),
		edges:             make(map[edgeKey]*atlas.Edge),
		docs:              make(map[string][]claims.Claim),
		readmes:           make(map[string]corpus.Entry),
		entries:           make(map[string]corpus.Entry),
		seeds:             make(map[string]struct{}),
		targetOf:          make(map[string]map[string]struct{}),
		bounds:            make(map[boundaryKey]*boundaryState),
		workspace:         make(map[string]struct{}),
		symbolCallerRows:  make(map[string]map[string]atlas.SymbolCaller),
		symbolBindingRows: make(map[string]map[string]atlas.SymbolBinding),
		symbolCallRows:    make(map[string]map[string]atlas.SymbolCall),
		factSubjects:      make(map[string]string),
	}
	for _, fact := range input.Facts.Facts {
		if fact.ObjectID != "" {
			b.factSubjects[fact.ObjectID] = ""
		}
	}
	for _, target := range input.Targets {
		if target.Dependencies == nil {
			continue
		}
		for _, dependency := range target.Dependencies.Dependencies {
			if dependency.Kind == dependencies.KindWorkspace {
				b.workspace[dependency.PackagePath] = struct{}{}
			}
		}
		for _, importer := range target.Dependencies.Importers {
			b.workspace[importer.PackagePath] = struct{}{}
		}
	}
	b.indexClaims()
	b.indexCorpus()
	for _, saved := range input.Targets {
		target, err := saved.read()
		if err != nil {
			return atlas.Graph{}, err
		}
		if err := target.Index.Validate(); err != nil {
			return atlas.Graph{}, fmt.Errorf("atlas places: target %s: %w", target.Index.Target.Name, err)
		}
		b.collectObjects(target)
		b.collectEdges(target)
		b.collectImports(target)
		b.collectSeeds(target)
		b.collectSymbolCallers(b.symbolCallerRows, target)
		b.collectSymbolBindings(b.symbolBindingRows, target)
		b.collectSymbolCalls(b.symbolCallRows, target)
		b.collectExternalCallCandidates(target)
	}
	b.releaseTargetObjects()
	// A located seed may refer to a file supplied by a later target. Resolve
	// that membership after collecting the complete file inventory, without
	// loading every target again just to read its relations and seeds.
	for filePath := range b.seeds {
		if _, exists := b.files[filePath]; !exists {
			delete(b.seeds, filePath)
		}
	}
	b.claimByRoot()
	if err := b.readFiles(); err != nil {
		return atlas.Graph{}, err
	}
	b.collectDirectories()
	b.assignDepths()
	b.collectSymbols()
	b.collectBoundaries()
	b.collectExternalCalls()
	return b.graph()
}

type fileState struct {
	path      string
	decls     []atlas.Decl
	doc       string
	generated bool
	targets   map[string]struct{}
	callers   map[string]struct{}
	callees   map[string]struct{}
	depth     int
	language  string
}

type dirState struct {
	path    string
	dirs    map[string]struct{}
	files   map[string]struct{}
	count   int
	targets map[string]struct{}
	readme  string
	doc     string
	topBox  bool
}

type edgeKey struct{ from, to, kind string }

type boundaryKey struct {
	path    string
	line    int
	kind    string
	column  int
	method  string
	values  string
	subject string
}

type boundaryState struct {
	place atlas.Place
}

type builder struct {
	input             Input
	files             map[string]*fileState
	dirs              map[string]*dirState
	byID              map[string]programindex.Object
	symbolOf          map[string]string // native object -> shared, compiler-located symbol place
	factSubjects      map[string]string // only native object IDs requested by saved facts
	fileOf            map[string]string
	fanIn             map[string]int
	edges             map[edgeKey]*atlas.Edge
	docs              map[string][]claims.Claim
	readmes           map[string]corpus.Entry
	entries           map[string]corpus.Entry
	seeds             map[string]struct{}
	targetOf          map[string]map[string]struct{}
	bounds            map[boundaryKey]*boundaryState
	symbols           []atlas.Place
	symbolCallerRows  map[string]map[string]atlas.SymbolCaller
	symbolBindingRows map[string]map[string]atlas.SymbolBinding
	symbolCallRows    map[string]map[string]atlas.SymbolCall
	externalCalls     []externalCallCandidate
	memberOwners      map[string]string // retained declaration -> native owner's symbol place
	typeFields        map[string]typeField
	// workspace lists the package paths of the repository's own modules, from
	// the dependency catalogs: a call into one of them is not an integration.
	workspace map[string]struct{}
}

type typeField struct {
	owner  string
	member atlas.TypeMember
}

func (b *builder) indexClaims() {
	for _, claim := range b.input.Claims.Claims {
		if claim.Source == claims.SourceDocstring && claim.Path != "" {
			b.docs[claim.Path] = append(b.docs[claim.Path], claim)
		}
	}
	for filePath := range b.docs {
		sort.Slice(b.docs[filePath], func(i, j int) bool { return b.docs[filePath][i].Line < b.docs[filePath][j].Line })
	}
}

func (b *builder) indexCorpus() {
	for _, entry := range b.input.Repository.Entries() {
		b.entries[entry.Path] = entry
		base := strings.ToLower(path.Base(entry.Path))
		if base == "readme.md" || base == "readme" || base == "readme.rst" || base == "readme.txt" {
			dir := parentDir(entry.Path)
			if _, taken := b.readmes[dir]; !taken || base == "readme.md" {
				b.readmes[dir] = entry
			}
		}
	}
}

// declarationKinds says which objects are declarations a reader sees.
func declaration(object programindex.Object, byID map[string]programindex.Object) bool {
	switch object.Kind {
	case programindex.ObjectFunction, programindex.ObjectMethod, programindex.ObjectType:
		// Closures are named after their function with a "$n" suffix; they
		// are code, not declarations a reader looks up.
		if object.Name == "" || object.Name == "call result" || strings.Contains(object.Name, "$") {
			return false
		}
		return object.Location != nil
	case programindex.ObjectVariable:
		// Module-level variables of Python and TypeScript are declarations;
		// ContainerID is lexical containment; OwnerID identifies a callable/type
		// owner and is empty on module-level variables. Locals are not shown.
		if object.Location == nil || object.ContainerID == "" {
			return false
		}
		container, ok := byID[object.ContainerID]
		if !ok || container.Kind != programindex.ObjectModule {
			return false
		}
		return object.Name != "" && object.Name != "self" && !strings.HasPrefix(object.Name, "_")
	default:
		return false
	}
}

// Only the current target needs native-object lookups. Cross-target consumers
// keep the source-located declarations and observations they actually publish.
func (b *builder) useTargetObjects(index programindex.Index) {
	b.byID = make(map[string]programindex.Object, len(index.Objects))
	b.fileOf = make(map[string]string)
	b.symbolOf = make(map[string]string)
	callbacks := make(map[string]bool)
	for _, relation := range index.Relations {
		// A source-located closure can own a real call even when returned
		// from a factory rather than registered as a callback. Its caller
		// identity must survive for argument provenance and question evidence.
		if relation.Kind == programindex.RelationCalls || relation.Kind == programindex.RelationInvokesExternal || relation.Kind == programindex.RelationExecutes {
			callbacks[relation.FromID] = true
		}
		if relation.Kind == programindex.RelationPassesCallback {
			for _, id := range relation.ToIDs {
				callbacks[id] = true
			}
		}
		for _, pattern := range relation.Patterns {
			for _, argument := range pattern.Arguments {
				for _, id := range argument.ObjectIDs {
					callbacks[id] = true
				}
			}
		}
	}
	for _, object := range index.Objects {
		b.byID[object.ID] = object
	}
	for _, object := range index.Objects {
		if object.Location == nil || object.Kind == programindex.ObjectExternalSymbol {
			continue
		}
		filePath := atlasPath(object.Location.Path)
		b.fileOf[object.ID] = filePath
		if !declaration(object, b.byID) && !(object.Kind == programindex.ObjectFunction && callbacks[object.ID]) {
			continue
		}
		name := object.Name
		if object.Kind == programindex.ObjectMethod && !strings.Contains(name, ".") {
			if owner, ok := b.byID[object.OwnerID]; ok && owner.Kind == programindex.ObjectType {
				name = owner.Name + "." + name
			}
		}
		// A file two targets index carries each declaration in both indexes.
		b.symbolOf[object.ID] = atlas.SymbolID(filePath, object.Location.Line, name)
	}
}

func (b *builder) releaseTargetObjects() {
	b.byID, b.fileOf, b.symbolOf = nil, nil, nil
}

func (b *builder) collectObjects(target TargetInput) {
	index := target.Index
	targetID := index.Target.ID
	b.useTargetObjects(index)
	if b.memberOwners == nil {
		b.memberOwners = make(map[string]string)
		b.typeFields = make(map[string]typeField)
	}
	for _, object := range index.Objects {
		filePath, located := b.fileOf[object.ID]
		if !located {
			continue
		}
		state := b.file(filePath)
		state.targets[targetID] = struct{}{}
		if state.language == "" {
			state.language = index.Target.Language
		}
		if b.symbolOf[object.ID] == "" {
			continue
		}
		// Keep the fact's exact target-local identity before declarations from
		// overlapping targets merge and the current native lookups are released.
		if _, needed := b.factSubjects[object.ID]; needed {
			b.factSubjects[object.ID] = b.symbolOf[object.ID]
		}
		name := object.Name
		if object.Kind == programindex.ObjectMethod && !strings.Contains(name, ".") {
			if owner, ok := b.byID[object.OwnerID]; ok && owner.Kind == programindex.ObjectType {
				name = owner.Name + "." + name
			}
		}
		if state.hasDecl(object.Location.Line, name) {
			continue
		}
		state.decls = append(state.decls, atlas.Decl{
			Name:      name,
			Kind:      string(object.Kind),
			Signature: languageSignature(object.Signature, state.language),
			LineNo:    object.Location.Line,
			Column:    object.Location.Column,
			Exported:  object.Visibility == programindex.VisibilityPublic,
			ObjectID:  object.ID,
		})
		if owner := b.byID[object.OwnerID]; owner.Kind == programindex.ObjectType && owner.Location != nil {
			b.memberOwners[object.ID] = b.symbolOf[owner.ID]
		}
	}
	for _, object := range index.Objects {
		owner := b.byID[object.OwnerID]
		if object.Kind != programindex.ObjectVariable || object.Location == nil || owner.Kind != programindex.ObjectType || object.ContainerID != owner.ID {
			continue
		}
		id := b.symbolOf[owner.ID]
		filePath := atlasPath(object.Location.Path)
		file := b.files[filePath]
		if id == "" || file == nil {
			continue
		}
		key := fmt.Sprintf("%s\x00%s\x00%d\x00%d\x00%s", id, filePath, object.Location.Line, object.Location.Column, object.Name)
		// The previous all-object pass sorted native IDs before deduplicating
		// fields. Preserve that representative independently of target order.
		if previous, exists := b.typeFields[key]; exists && previous.member.Decl.ObjectID <= object.ID {
			continue
		}
		b.typeFields[key] = typeField{owner: id, member: atlas.TypeMember{Path: filePath, Decl: atlas.Decl{
			ObjectID: object.ID, Name: object.Name, Kind: string(object.Kind), Signature: languageSignature(object.Signature, file.language),
			LineNo: object.Location.Line, Column: object.Location.Column, Exported: object.Visibility == programindex.VisibilityPublic,
		}}}
	}
	if root := atlasPath(target.Root); root != "" {
		b.targetOf[targetID] = map[string]struct{}{root: {}}
	}
}

// claimByRoot gives a file under another target's root to that target
// alone: a program reaches the files of the libraries it uses, but they are
// the library's, and a call into them is the seam between the two. The
// deepest indexed root wins. Targets with the same root keep their shared
// files; their order cannot erase the library beside an executable.
func (b *builder) claimByRoot() {
	type root struct {
		targetID string
		path     string
	}
	var roots []root
	for _, target := range b.input.Targets {
		path := atlasPath(target.Root)
		if path == "" {
			continue
		}
		roots = append(roots, root{target.Index.Target.ID, path})
	}
	for filePath, state := range b.files {
		owners, depth := make(map[string]struct{}), -1
		for _, r := range roots {
			if _, indexed := state.targets[r.targetID]; !indexed {
				continue
			}
			if r.path != "." && filePath != r.path && !strings.HasPrefix(filePath, r.path+"/") {
				continue
			}
			d := strings.Count(r.path, "/") + 1
			if r.path == "." {
				d = 0
			}
			if d > depth {
				owners, depth = make(map[string]struct{}), d
			}
			if d == depth {
				owners[r.targetID] = struct{}{}
			}
		}
		if len(owners) > 0 {
			state.targets = owners
		}
	}
}

func (state *fileState) hasDecl(line int, name string) bool {
	for _, decl := range state.decls {
		if decl.LineNo == line && decl.Name == name {
			return true
		}
	}
	return false
}

func (b *builder) file(filePath string) *fileState {
	state, ok := b.files[filePath]
	if !ok {
		state = &fileState{
			path: filePath, targets: make(map[string]struct{}),
			callers: make(map[string]struct{}), callees: make(map[string]struct{}),
		}
		b.files[filePath] = state
	}
	return state
}

func (b *builder) collectEdges(target TargetInput) {
	for _, relation := range target.Index.Relations {
		kind := edgeKind(relation.Kind)
		if kind == "" {
			continue
		}
		from, ok := b.fileOf[relation.FromID]
		if !ok {
			continue
		}
		caller := b.byID[relation.FromID]
		for _, toID := range relation.ToIDs {
			b.fanIn[toID]++
			to, ok := b.fileOf[toID]
			if !ok || to == from {
				continue
			}
			callee := b.byID[toID]
			b.addEdge(atlas.FileID(from), atlas.FileID(to), kind, atlas.Witness{
				Caller: displayName(caller, b.byID), Callee: displayName(callee, b.byID),
				Path: from, LineNo: relationLine(relation),
			})
			b.file(from).callees[to] = struct{}{}
			b.file(to).callers[from] = struct{}{}
		}
	}
}

func edgeKind(kind programindex.RelationKind) string {
	switch kind {
	case programindex.RelationCalls:
		return "calls"
	case programindex.RelationImports:
		return "imports"
	case programindex.RelationPassesCallback:
		return "passes_callback"
	case programindex.RelationDecorates:
		return "decorates"
	case programindex.RelationExecutes:
		return "executes"
	default:
		return ""
	}
}

func relationLine(relation programindex.Relation) int {
	if relation.Location != nil {
		return relation.Location.Line
	}
	for _, witness := range relation.Witnesses {
		if witness.Location != nil {
			return witness.Location.Line
		}
	}
	return 0
}

func displayName(object programindex.Object, byID map[string]programindex.Object) string {
	name := object.Name
	if object.Kind == programindex.ObjectMethod && !strings.Contains(name, ".") {
		if owner, ok := byID[object.OwnerID]; ok && owner.Kind == programindex.ObjectType {
			name = owner.Name + "." + name
		}
	}
	if name == "" {
		return string(object.Kind)
	}
	return name
}

func (b *builder) addEdge(from, to, kind string, witness atlas.Witness) {
	key := edgeKey{from, to, kind}
	edge, ok := b.edges[key]
	if !ok {
		edge = &atlas.Edge{From: from, To: to, Kind: kind}
		b.edges[key] = edge
	}
	edge.Count++
	if witness.Caller != "" && len(edge.Witnesses) < 64 {
		edge.Witnesses = append(edge.Witnesses, witness)
	}
}

// collectImports adds directory-to-directory edges for Go package imports of
// workspace packages, which the object graph does not carry.
func (b *builder) collectImports(target TargetInput) {
	catalog := target.Dependencies
	if catalog == nil {
		return
	}
	importers := make(map[string]dependencies.Importer, len(catalog.Importers))
	for _, importer := range catalog.Importers {
		importers[importer.Ref] = importer
	}
	for _, dependency := range catalog.Dependencies {
		if dependency.Kind != dependencies.KindWorkspace || dependency.RepositoryPath == "" {
			continue
		}
		to := atlasPath(dependency.RepositoryPath)
		for _, ref := range dependency.ImporterRefs {
			importer, ok := importers[ref]
			if !ok || importer.RepositoryPath == "" {
				continue
			}
			from := atlasPath(importer.RepositoryPath)
			if from == to {
				continue
			}
			// An import has no call site to witness; the edge counts alone.
			b.addEdge(atlas.DirectoryID(from), atlas.DirectoryID(to), "imports", atlas.Witness{})
		}
	}
}

func (b *builder) collectSeeds(target TargetInput) {
	for _, seed := range target.Index.Target.Seeds {
		if filePath, ok := b.fileOf[seed.ObjectID]; ok {
			b.seeds[filePath] = struct{}{}
			continue
		}
		if seed.Location != nil {
			b.seeds[atlasPath(seed.Location.Path)] = struct{}{}
		}
	}
}

// readFiles attaches docstrings to declarations, finds module docs and marks
// generated files.
func (b *builder) readFiles() error {
	for filePath, state := range b.files {
		sort.Slice(state.decls, func(i, j int) bool {
			if state.decls[i].LineNo != state.decls[j].LineNo {
				return state.decls[i].LineNo < state.decls[j].LineNo
			}
			return state.decls[i].Name < state.decls[j].Name
		})
		for i := range state.decls {
			state.decls[i].FanIn = b.fanIn[state.decls[i].ObjectID]
			state.decls[i].Doc = b.docstringFor(filePath, state.decls[i].LineNo, state.decls)
		}
		state.doc = b.moduleDoc(filePath, state)
		entry, ok := b.entries[filePath]
		if !ok {
			continue
		}
		state.generated = generatedByName(filePath)
		if state.generated {
			continue
		}
		content, err := b.input.Repository.ReadFile(entry.ID, 8<<10)
		if err != nil {
			return fmt.Errorf("atlas places: read %s: %w", filePath, err)
		}
		state.generated = generatedByMarker(content.Bytes)
	}
	return nil
}

func generatedByName(filePath string) bool {
	base := path.Base(filePath)
	return strings.HasPrefix(base, "zz_generated") || strings.HasSuffix(base, ".pb.go") ||
		strings.HasSuffix(base, ".pb.gw.go") || strings.HasSuffix(base, "_generated.go") ||
		strings.HasSuffix(base, ".gen.go") || strings.HasSuffix(base, ".d.ts") ||
		strings.HasSuffix(base, ".min.js")
}

func generatedByMarker(content []byte) bool {
	lines := bytes.Split(content, []byte("\n"))
	if len(lines) > generatedMarkerLines {
		lines = lines[:generatedMarkerLines]
	}
	for _, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if !bytes.HasPrefix(trimmed, []byte("//")) && !bytes.HasPrefix(trimmed, []byte("#")) &&
			!bytes.HasPrefix(trimmed, []byte("/*")) && !bytes.HasPrefix(trimmed, []byte("*")) {
			continue
		}
		if generatedMarker.Match(trimmed) && bytes.Contains(bytes.ToLower(trimmed), []byte("generated")) {
			return true
		}
	}
	return false
}

// docstringFor finds the docstring that belongs to the declaration at line:
// the nearest docstring above it within reach, with no other declaration
// between them.
func (b *builder) docstringFor(filePath string, line int, decls []atlas.Decl) string {
	return firstSentence(b.quotedDocstringFor(filePath, line, decls))
}

// quotedDocstringFor retains the existing bounded author quote. Type contracts
// need later sentences as well: these often state effects or lifecycle rules.
func (b *builder) quotedDocstringFor(filePath string, line int, decls []atlas.Decl) string {
	docs := b.docs[filePath]
	best := ""
	for _, doc := range docs {
		if doc.Line > line || line-doc.Line > docstringReach {
			continue
		}
		between := false
		for _, decl := range decls {
			if decl.LineNo > doc.Line && decl.LineNo < line {
				between = true
				break
			}
		}
		if between {
			continue
		}
		best = doc.Text
	}
	return best
}

// moduleDoc is the file's own documentation: a Python module docstring, a
// leading JSDoc, or a Go package comment when this file carries it.
func (b *builder) moduleDoc(filePath string, state *fileState) string {
	docs := b.docs[filePath]
	if len(docs) == 0 {
		return ""
	}
	first := docs[0]
	firstDecl := 0
	if len(state.decls) > 0 {
		firstDecl = state.decls[0].LineNo
	}
	switch strings.ToLower(path.Ext(filePath)) {
	case ".py":
		if firstDecl == 0 || firstDecl-first.Line > docstringReach || first.Line <= 2 {
			return firstSentence(first.Text)
		}
	case ".go":
		if strings.HasPrefix(first.Text, "Package ") {
			return firstSentence(first.Text)
		}
	default:
		if firstDecl == 0 || firstDecl-first.Line > docstringReach {
			return firstSentence(first.Text)
		}
	}
	return ""
}

func (b *builder) collectDirectories() {
	for filePath, state := range b.files {
		dir := parentDir(filePath)
		d := b.dir(dir)
		d.files[path.Base(filePath)] = struct{}{}
		for targetID := range state.targets {
			d.targets[targetID] = struct{}{}
		}
		child := dir
		for {
			b.dir(child).count++
			if child == "." {
				break
			}
			parent := parentDir(child)
			p := b.dir(parent)
			p.dirs[path.Base(child)] = struct{}{}
			for targetID := range state.targets {
				p.targets[targetID] = struct{}{}
			}
			child = parent
		}
	}
	for dir, state := range b.dirs {
		state.readme = b.readmeLine(dir)
		state.doc = b.packageDoc(dir)
		state.topBox = len(state.files) > 0 && !b.hasFilesAbove(dir)
	}
}

func (b *builder) hasFilesAbove(dir string) bool {
	for dir != "." {
		dir = parentDir(dir)
		if state, ok := b.dirs[dir]; ok && len(state.files) > 0 {
			return true
		}
	}
	return false
}

func (b *builder) dir(dir string) *dirState {
	state, ok := b.dirs[dir]
	if !ok {
		state = &dirState{
			path: dir, dirs: make(map[string]struct{}), files: make(map[string]struct{}),
			targets: make(map[string]struct{}),
		}
		b.dirs[dir] = state
	}
	return state
}

func (b *builder) readmeLine(dir string) string {
	entry, ok := b.readmes[dir]
	if !ok {
		return ""
	}
	content, err := b.input.Repository.ReadFile(entry.ID, maxReadBytes)
	if err != nil || !utf8.Valid(content.Bytes) {
		return ""
	}
	// A README's first readable line is often its title alone; the first
	// line that says something is the first with a few words in it.
	first := ""
	for _, line := range strings.Split(string(content.Bytes), "\n") {
		text := readableLine(line)
		if text == "" {
			continue
		}
		if first == "" {
			first = text
		}
		if len(strings.Fields(text)) >= 4 {
			return truncateRunes(text, maxLineRunes)
		}
	}
	return truncateRunes(first, maxLineRunes)
}

// readableLine strips markdown decoration and keeps prose only.
func readableLine(line string) string {
	text := strings.TrimSpace(line)
	if text == "" || strings.HasPrefix(text, "<") || strings.HasPrefix(text, "[![") ||
		strings.HasPrefix(text, "```") || strings.HasPrefix(text, "|") || strings.HasPrefix(text, "---") ||
		strings.HasPrefix(text, "===") {
		return ""
	}
	text = strings.TrimLeft(text, "#> ")
	text = strings.NewReplacer("**", "", "__", "", "`", "").Replace(text)
	// [label](url) -> label
	for {
		open := strings.Index(text, "](")
		if open < 0 {
			break
		}
		start := strings.LastIndex(text[:open], "[")
		end := strings.Index(text[open:], ")")
		if start < 0 || end < 0 {
			break
		}
		text = text[:start] + text[start+1:open] + text[open+end+1:]
	}
	text = strings.TrimSpace(text)
	letters := 0
	for _, r := range text {
		if unicode.IsLetter(r) {
			letters++
		}
	}
	if letters < 3 {
		return ""
	}
	return text
}

// packageDoc is the directory's package documentation: the Go package
// comment of any file here, a Python __init__ docstring, or the description
// of a package.json.
func (b *builder) packageDoc(dir string) string {
	if init := path.Join(dir, "__init__.py"); b.hasFile(init) {
		if state, ok := b.files[init]; ok && state.doc != "" {
			return state.doc
		}
		if docs := b.docs[init]; len(docs) > 0 {
			return firstSentence(docs[0].Text)
		}
	}
	if entry, ok := b.entries[path.Join(dir, "package.json")]; ok {
		if content, err := b.input.Repository.ReadFile(entry.ID, maxReadBytes); err == nil {
			var manifest struct {
				Description string `json:"description"`
			}
			if json.Unmarshal(content.Bytes, &manifest) == nil && manifest.Description != "" {
				return truncateRunes(strings.TrimSpace(manifest.Description), maxLineRunes)
			}
		}
	}
	state, ok := b.dirs[dir]
	if !ok {
		return ""
	}
	names := make([]string, 0, len(state.files))
	for name := range state.files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		if file, ok := b.files[path.Join(dir, name)]; ok && strings.HasPrefix(file.doc, "Package ") {
			return file.doc
		}
	}
	// Go package comments live above the package clause, which claims does
	// not quote; read the first lines of each Go file for it.
	for _, name := range names {
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		entry, ok := b.entries[path.Join(dir, name)]
		if !ok {
			continue
		}
		content, err := b.input.Repository.ReadFile(entry.ID, 16<<10)
		if err != nil {
			continue
		}
		if doc := goPackageComment(string(content.Bytes)); doc != "" {
			return doc
		}
	}
	return ""
}

func goPackageComment(source string) string {
	lines := strings.Split(source, "\n")
	var block []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "package "):
			if len(block) > 0 {
				text := strings.Join(block, " ")
				if strings.HasPrefix(text, "Package ") {
					return firstSentence(text)
				}
			}
			return ""
		case strings.HasPrefix(trimmed, "//"):
			if strings.HasPrefix(trimmed, "//go:") {
				continue
			}
			block = append(block, strings.TrimSpace(strings.TrimPrefix(trimmed, "//")))
		case trimmed == "":
			block = nil
		default:
			if strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
				block = append(block, strings.TrimSpace(strings.Trim(trimmed, "/* ")))
				continue
			}
			return ""
		}
	}
	return ""
}

// shortSignature drops module paths from a Go signature: a reader says
// corpus.Entry, not github.com/owner/repo/internal/corpus.Entry.
func languageSignature(signature, language string) string {
	if language == "go" {
		return shortSignature(signature)
	}
	return signature
}

func shortSignature(signature string) string {
	if !strings.Contains(signature, "/") {
		return signature
	}
	var out strings.Builder
	token := strings.Builder{}
	flush := func() {
		text := token.String()
		if slash := strings.LastIndex(text, "/"); slash >= 0 && strings.Contains(text[slash:], ".") {
			text = text[slash+1:]
		}
		out.WriteString(text)
		token.Reset()
	}
	for _, r := range signature {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '/' || r == '.' || r == '_' || r == '-' {
			token.WriteRune(r)
			continue
		}
		flush()
		out.WriteRune(r)
	}
	flush()
	return out.String()
}

func (b *builder) hasFile(filePath string) bool {
	_, ok := b.entries[filePath]
	return ok
}

// assignDepths runs the BFS over file edges from the union of seeds. Files no
// round reaches come last.
func (b *builder) assignDepths() {
	depth := make(map[string]int, len(b.files))
	queue := make([]string, 0, len(b.seeds))
	for seed := range b.seeds {
		queue = append(queue, seed)
		depth[seed] = 0
	}
	sort.Strings(queue)
	last := 0
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		next := make([]string, 0)
		for callee := range b.files[current].callees {
			if _, seen := depth[callee]; seen {
				continue
			}
			depth[callee] = depth[current] + 1
			if depth[callee] > last {
				last = depth[callee]
			}
			next = append(next, callee)
		}
		sort.Strings(next)
		queue = append(queue, next...)
	}
	for filePath, state := range b.files {
		if d, ok := depth[filePath]; ok {
			state.depth = d
		} else {
			state.depth = last + 1
		}
	}
}

// MaxSymbolCandidates is how many declarations of one file may become
// symbol places: exported and documented first, then by callers.
const MaxSymbolCandidates = 10

// collectSymbols lifts each file's most telling declarations to places.
func (b *builder) collectSymbols() {
	calls := b.symbolCalls()
	bindings := b.symbolBindings()
	callers := b.symbolCallers()
	members := b.typeMembers()
	for filePath, state := range b.files {
		ranked := append([]atlas.Decl(nil), state.decls...)
		sort.SliceStable(ranked, func(i, j int) bool {
			a, c := ranked[i], ranked[j]
			if (a.Exported && a.Doc != "") != (c.Exported && c.Doc != "") {
				return a.Exported && a.Doc != ""
			}
			if a.Exported != c.Exported {
				return a.Exported
			}
			if (a.Doc != "") != (c.Doc != "") {
				return a.Doc != ""
			}
			if a.FanIn != c.FanIn {
				return a.FanIn > c.FanIn
			}
			return a.LineNo < c.LineNo
		})
		for rank, decl := range ranked {
			if state.generated && decl.Kind != "function" && decl.Kind != "method" && decl.Kind != "type" {
				continue
			}
			if rank >= MaxSymbolCandidates && decl.Kind != "function" && decl.Kind != "method" && decl.Kind != "type" {
				continue
			}
			given := decl.Doc
			if given == "" {
				given = decl.Signature
			}
			if given == "" {
				given = decl.Kind + " " + decl.Name
			}
			id := atlas.SymbolID(filePath, decl.LineNo, decl.Name)
			if decl.Kind == string(programindex.ObjectType) {
				decl.Doc = b.quotedDocstringFor(filePath, decl.LineNo, state.decls)
			}
			b.symbols = append(b.symbols, atlas.Place{
				ID: id, Kind: atlas.PlaceSymbol, Path: filePath,
				LineNo: decl.LineNo, Column: decl.Column, Depth: state.depth, TargetIDs: sortedKeys(state.targets),
				Parent: atlas.FileID(filePath), Given: truncateRunes(given, maxLineRunes),
				Symbol: &atlas.SymbolFacts{Decl: decl, Members: members[id], Calls: calls[id], Bindings: bindings[id], CalledBy: callers[id], Candidate: !state.generated && (rank < MaxSymbolCandidates || decl.Kind == "function" || decl.Kind == "method"), Rank: rank + 1},
			})
		}
	}
	// Some declarations are intentionally not lifted (for example incidental
	// closures and lower-ranked variables). Their names remain observations, but
	// they cannot become dangling context links.
	known := make(map[string]bool, len(b.symbols))
	for _, place := range b.symbols {
		known[place.ID] = true
	}
	for objectID, subjectID := range b.factSubjects {
		if !known[subjectID] {
			delete(b.factSubjects, objectID)
		}
	}
	for i := range b.symbols {
		symbol := b.symbols[i].Symbol
		for j := range symbol.Calls {
			var ids []string
			for _, id := range symbol.Calls[j].CalleeIDs {
				if known[id] {
					ids = append(ids, id)
				}
			}
			symbol.Calls[j].CalleeIDs = ids
		}
		for j := range symbol.CalledBy {
			if !known[symbol.CalledBy[j].PlaceID] {
				symbol.CalledBy[j].PlaceID = ""
			}
		}
	}
}

// typeMembers follows native ownership, including declarations in other files.
// A matching prefix, file, or method name never establishes membership.
func (b *builder) typeMembers() map[string][]atlas.TypeMember {
	result := make(map[string][]atlas.TypeMember)
	for path, file := range b.files {
		for _, decl := range file.decls {
			id := b.memberOwners[decl.ObjectID]
			if id != "" {
				decl.Doc = b.quotedDocstringFor(path, decl.LineNo, file.decls)
				result[id] = append(result[id], atlas.TypeMember{Path: path, Decl: decl})
			}
		}
	}
	// Class attributes are native declarations owned by the type, without
	// becoming unrelated top-level symbol candidates. Local variables and
	// assignments to an arbitrary instance never acquire type ownership here.
	for _, field := range b.typeFields {
		result[field.owner] = append(result[field.owner], field.member)
	}
	for id := range result {
		sort.Slice(result[id], func(i, j int) bool {
			a, c := result[id][i], result[id][j]
			if a.Path != c.Path {
				return a.Path < c.Path
			}
			if a.Decl.LineNo != c.Decl.LineNo {
				return a.Decl.LineNo < c.Decl.LineNo
			}
			if a.Decl.Column != c.Decl.Column {
				return a.Decl.Column < c.Decl.Column
			}
			return a.Decl.Name < c.Decl.Name
		})
	}
	return result
}

func (b *builder) collectSymbolCallers(rows map[string]map[string]atlas.SymbolCaller, target TargetInput) {
	for _, relation := range target.Index.Relations {
		if relation.Kind != programindex.RelationCalls && relation.Kind != programindex.RelationExecutes {
			continue
		}
		from, ok := b.byID[relation.FromID]
		if !ok || from.Location == nil {
			continue
		}
		row := atlas.SymbolCaller{ObjectID: from.ID, PlaceID: b.symbolOf[from.ID], Name: displayName(from, b.byID), Signature: languageSignature(from.Signature, target.Index.Target.Language), Path: from.Location.Path, Line: relationLine(relation), Kind: string(relation.Kind), Invocation: relation.Invocation, Resolution: string(relation.Resolution)}
		key := row
		if key.PlaceID != "" {
			key.ObjectID = ""
		}
		raw, _ := json.Marshal(key)
		for _, nativeID := range relation.ToIDs {
			id := b.symbolOf[nativeID]
			if id == "" {
				continue
			}
			if rows[id] == nil {
				rows[id] = make(map[string]atlas.SymbolCaller)
			}
			rows[id][string(raw)] = row
		}
	}
}

func (b *builder) symbolCallers() map[string][]atlas.SymbolCaller {
	rows := b.symbolCallerRows
	if rows == nil {
		rows = make(map[string]map[string]atlas.SymbolCaller)
		for _, target := range b.input.Targets {
			b.useTargetObjects(target.Index)
			b.collectSymbolCallers(rows, target)
		}
	}
	result := make(map[string][]atlas.SymbolCaller)
	for id, values := range rows {
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			result[id] = append(result[id], values[key])
		}
	}
	return result
}

func (b *builder) collectSymbolBindings(rows map[string]map[string]atlas.SymbolBinding, target TargetInput) {
	// Callback provenance already identifies its exact argument at one call
	// site. Retain that registration's neighbouring literal arguments so a
	// handler can see its path/topic without reading unrelated factory calls.
	registrations := make(map[string][]atlas.RegistrationArgument)
	registrationEvidence := make(map[string][]atlas.EdgeEvidence)
	valueUses := make(map[string][]atlas.EdgeEvidence)
	producers := make(map[string][]programindex.RelationPattern)
	for _, relation := range target.Index.Relations {
		for _, pattern := range relation.Patterns {
			if pattern.ResultID != "" {
				producers[pattern.ResultID] = append(producers[pattern.ResultID], pattern)
			}
			if pattern.ReceiverID == "" || pattern.Location == nil {
				continue
			}
			valueUses[pattern.ReceiverID] = append(valueUses[pattern.ReceiverID], atlas.EdgeEvidence{
				Extractor: "registration_result_use", Label: "call on the registration result: " + pattern.Selector,
				Path: pattern.Location.Path, LineNo: pattern.Location.Line,
			})
		}
	}
	for _, relation := range target.Index.Relations {
		if relation.Kind == programindex.RelationPassesCallback && relation.SourceArgumentID != "" {
			registrations[relation.SourceArgumentID] = nil
		}
	}
	for _, relation := range target.Index.Relations {
		for _, pattern := range relation.Patterns {
			if pattern.Location == nil {
				continue
			}
			var selected []string
			for _, argument := range pattern.Arguments {
				if _, needed := registrations[argument.ID]; needed {
					selected = append(selected, argument.ID)
				}
			}
			if len(selected) == 0 {
				continue
			}
			var arguments []atlas.RegistrationArgument
			for _, argument := range pattern.Arguments {
				if value, ok := literalArgument(argument); ok {
					arguments = append(arguments, atlas.RegistrationArgument{Position: argument.Position, Keyword: argument.Keyword,
						Kind: string(argument.Kind), Value: value, Path: pattern.Location.Path, Line: pattern.Location.Line})
				}
			}
			for _, id := range selected {
				registrations[id] = arguments
				label := "receiving call: " + pattern.Selector
				if len(relation.ToIDs) == 1 {
					if recipient, ok := b.byID[relation.ToIDs[0]]; ok {
						label = "receiving call: " + displayName(recipient, b.byID)
					}
				}
				registrationEvidence[id] = append(registrationEvidence[id], atlas.EdgeEvidence{
					Extractor: "callback_registration", Label: label,
					Path: pattern.Location.Path, LineNo: pattern.Location.Line,
				})
				if pattern.ResultID != "" {
					registrationEvidence[id] = append(registrationEvidence[id], valueUses[pattern.ResultID]...)
				}
				// Fluent registration arguments belong to the exact receiver
				// producer, e.g. job.at("00:07").do(callback). Keep that syntax
				// beside the callback without interpreting a schedule locally.
				seen := map[string]bool{}
				var receiverEvidence func(string)
				receiverEvidence = func(receiverID string) {
					if receiverID == "" || seen[receiverID] {
						return
					}
					seen[receiverID] = true
					for _, producer := range producers[receiverID] {
						if producer.Location == nil {
							continue
						}
						var literals []string
						for _, argument := range producer.Arguments {
							if value, ok := literalArgument(argument); ok {
								literals = append(literals, strconv.Quote(value))
							}
						}
						registrationEvidence[id] = append(registrationEvidence[id], atlas.EdgeEvidence{
							Extractor: "registration_receiver_call", Label: producer.Selector + "(" + strings.Join(literals, ", ") + ")",
							Path: producer.Location.Path, LineNo: producer.Location.Line,
						})
						receiverEvidence(producer.ReceiverID)
					}
				}
				receiverEvidence(pattern.ReceiverID)
			}
		}
	}
	for _, relation := range target.Index.Relations {
		if relation.Kind != programindex.RelationPassesCallback {
			continue
		}
		from, known := b.byID[relation.FromID]
		if !known {
			continue
		}
		var evidence []atlas.EdgeEvidence
		for _, witness := range relation.Witnesses {
			if (witness.Kind == "callable_receiver_field" || witness.Kind == "interface_field_assignment") && witness.Location != nil {
				evidence = append(evidence, atlas.EdgeEvidence{Extractor: witness.Kind, Label: witness.Detail, Path: witness.Location.Path, LineNo: witness.Location.Line})
			}
		}
		for _, id := range relation.ToIDs {
			to, known := b.byID[id]
			if !known {
				continue
			}
			for _, witness := range relation.Witnesses {
				if witness.Kind == "callable_receiver_field" || witness.Kind == "interface_field_assignment" {
					continue
				}
				row := atlas.SymbolBinding{From: displayName(from, b.byID), To: displayName(to, b.byID), Detail: witness.Detail, Invocation: relation.Invocation, Resolution: string(relation.Resolution)}
				row.Evidence = append(append([]atlas.EdgeEvidence{}, evidence...), registrationEvidence[relation.SourceArgumentID]...)
				row.Evidence = canonicalBindingEvidence(row.Evidence)
				row.Arguments = registrations[relation.SourceArgumentID]
				if witness.Location != nil {
					row.Path, row.Line = witness.Location.Path, witness.Location.Line
				}
				raw, _ := json.Marshal(row)
				for _, owner := range []string{b.symbolOf[relation.FromID], b.symbolOf[id]} {
					if owner == "" {
						continue
					}
					if rows[owner] == nil {
						rows[owner] = make(map[string]atlas.SymbolBinding)
					}
					rows[owner][string(raw)] = row
				}
			}
		}
	}
}

// Binding evidence is a set of native observations. Target-local relation
// order must not create different binding identities for the same set.
func canonicalBindingEvidence(evidence []atlas.EdgeEvidence) []atlas.EdgeEvidence {
	sort.Slice(evidence, func(i, j int) bool {
		a, b := evidence[i], evidence[j]
		if a.Extractor != b.Extractor {
			return a.Extractor < b.Extractor
		}
		if a.Label != b.Label {
			return a.Label < b.Label
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.LineNo < b.LineNo
	})
	n := 0
	for _, observation := range evidence {
		if n == 0 || evidence[n-1] != observation {
			evidence[n] = observation
			n++
		}
	}
	return evidence[:n]
}

func (b *builder) symbolBindings() map[string][]atlas.SymbolBinding {
	rows := b.symbolBindingRows
	if rows == nil {
		rows = make(map[string]map[string]atlas.SymbolBinding)
		for _, target := range b.input.Targets {
			b.useTargetObjects(target.Index)
			b.collectSymbolBindings(rows, target)
		}
	}
	result := make(map[string][]atlas.SymbolBinding)
	for id, values := range rows {
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			result[id] = append(result[id], values[key])
		}
	}
	return result
}

// symbolCalls preserves call-site evidence without a framework vocabulary.
// Multiple target indexes may contain the same declaration and witness.
func symbolCallKey(call atlas.SymbolCall) string {
	// Keep the existing evidence order independent of a local source
	// column. The column distinguishes otherwise identical source sites,
	// but must not reorder provider facts and invalidate unrelated answers.
	column := call.Column
	call.Column = 0
	raw, _ := json.Marshal(call)
	return fmt.Sprintf("%s:%d", raw, column)
}

func (b *builder) collectSymbolCalls(byObject map[string]map[string]atlas.SymbolCall, target TargetInput) {
	var observed []nativeSymbolCall
	var source *nativeDispatchSource
	add := func(id string, call atlas.SymbolCall) {
		if b.symbolOf[id] == "" {
			return
		}
		observed = append(observed, nativeSymbolCall{owner: id, path: b.fileOf[id], call: call, source: source})
	}
	for _, relation := range target.Index.Relations {
		if relation.Kind == programindex.RelationImports || relation.Kind == programindex.RelationContains {
			continue
		}
		source = dispatchSource(relation)
		// Compiler dispatch and direct-call witnesses need not have a value
		// pattern. Dropping them removed the call into an implementation from
		// handler evidence, especially for interface dispatch.
		if len(relation.Patterns) == 0 && (relation.Kind == programindex.RelationCalls || relation.Kind == programindex.RelationExecutes || relation.Kind == programindex.RelationInvokesExternal) {
			var evidence []atlas.EdgeEvidence
			for _, witness := range relation.Witnesses {
				if witness.Kind == "interface_field_assignment" && witness.Location != nil {
					evidence = append(evidence, atlas.EdgeEvidence{Extractor: witness.Kind, Label: witness.Detail, Path: witness.Location.Path, LineNo: witness.Location.Line})
				}
			}
			for _, witness := range relation.Witnesses {
				// A receiver assignment supports this call; it is not another
				// call at the constructor's source line.
				if witness.Kind == "interface_field_assignment" {
					continue
				}
				call := atlas.SymbolCall{Kind: string(relation.Kind), Invocation: relation.Invocation, Resolution: string(relation.Resolution), Detail: witness.Detail, Evidence: evidence}
				if witness.Location != nil {
					call.Line = witness.Location.Line
					call.Column = witness.Location.Column
				}
				for _, id := range relation.ToIDs {
					if object, ok := b.byID[id]; ok {
						call.Name = displayName(object, b.byID)
						call.CalleeIDs = nil
						if symbolID := b.symbolOf[id]; symbolID != "" {
							call.CalleeIDs = []string{symbolID}
						}
						add(relation.FromID, call)
					}
				}
				if len(relation.ToIDs) == 0 && call.Detail != "" {
					add(relation.FromID, call)
				}
			}
		}
		for _, pattern := range relation.Patterns {
			call := atlas.SymbolCall{Kind: string(relation.Kind), Name: pattern.Selector, Invocation: relation.Invocation, Resolution: string(relation.Resolution), ReceiverValue: sourcevalue.Clone(pattern.ReceiverValue), ResultValue: sourcevalue.Clone(pattern.ResultValue)}
			for _, witness := range pattern.Context {
				if witness.Location != nil {
					call.Evidence = append(call.Evidence, atlas.EdgeEvidence{Extractor: witness.Kind, Label: witness.Detail, Path: witness.Location.Path, LineNo: witness.Location.Line})
				}
			}
			for _, id := range relation.ToIDs {
				if symbolID := b.symbolOf[id]; symbolID != "" {
					call.CalleeIDs = appendUnique(call.CalleeIDs, symbolID)
				}
			}
			sort.Strings(call.CalleeIDs)
			// The selector alone loses the receiver/package: context.Background
			// and a remote client's Background would become the same evidence.
			if len(relation.ToIDs) == 1 {
				if object, ok := b.byID[relation.ToIDs[0]]; ok && object.External != nil {
					call.Name = externalName(*object.External)
					if object.External.RepositoryPath == "" {
						call.API = &atlas.CallAPI{Package: object.External.PackagePath, Receiver: object.External.Receiver, Name: object.External.Name}
					}
				}
			}
			if pattern.Location != nil {
				call.Line = pattern.Location.Line
				call.Column = pattern.Location.Column
			}
			for _, argument := range pattern.Arguments {
				if value, ok := literalArgument(argument); ok {
					call.Values = appendUnique(call.Values, value)
				}
				if argument.Origin != nil {
					call.SourceArguments = append(call.SourceArguments, atlas.SourceArgument{Position: argument.Position, Keyword: argument.Keyword, Origin: sourcevalue.Clone(argument.Origin)})
				}
				for _, id := range argument.ObjectIDs {
					if object, ok := b.byID[id]; ok {
						call.Arguments = appendUnique(call.Arguments, displayName(object, b.byID))
					}
				}
			}
			if call.Name == "" {
				continue
			}
			add(relation.FromID, call)
		}
	}
	// Reconcile native views while their exact target and object scope is
	// still available. Cross-target observations are not counterpart evidence.
	for _, row := range mergeDeclaredDispatchPairs(observed) {
		id := b.symbolOf[row.owner]
		if byObject[id] == nil {
			byObject[id] = make(map[string]atlas.SymbolCall)
		}
		byObject[id][symbolCallKey(row.call)] = row.call
	}
}

func (b *builder) symbolCalls() map[string][]atlas.SymbolCall {
	byObject := b.symbolCallRows
	if byObject == nil {
		byObject = make(map[string]map[string]atlas.SymbolCall)
		for _, target := range b.input.Targets {
			b.useTargetObjects(target.Index)
			b.collectSymbolCalls(byObject, target)
		}
	}
	result := make(map[string][]atlas.SymbolCall)
	for id, rows := range byObject {
		// A narrower target view can leave the same dispatch unresolved while
		// another view observes possible receivers. Keep every candidate and
		// its open resolution, but do not turn the empty view into another call.
		// Matching all other fields (including the compiler column) preserves
		// distinct call sites, dispatch details and independent evidence. An
		// exact call does not subsume an uncertain observation from another view.
		for _, call := range rows {
			if call.Resolution != string(programindex.ResolutionAlternatives) || len(call.CalleeIDs) == 0 || call.Line < 1 || call.Column < 1 {
				continue
			}
			call.Name, call.Resolution = "", string(programindex.ResolutionUnresolved)
			call.CalleeIDs, call.Evidence = nil, nil
			delete(rows, symbolCallKey(call))
		}
		keys := make([]string, 0, len(rows))
		for key := range rows {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			result[id] = append(result[id], rows[key])
		}
	}
	return result
}

// collectBoundaries lifts the facts the code already knows as integration
// points into boundary places. Only the same anchored observation is shared
// across targets; different methods, paths and registration columns stay separate.
func (b *builder) collectBoundaries() {
	// Facts name their own target rows; the atlas speaks in program target
	// IDs, so a fact's target is translated before it names a place.
	programTarget := make(map[string]string, len(b.input.Facts.Targets))
	for _, target := range b.input.Facts.Targets {
		programTarget[target.ID] = target.ProgramTargetID
	}
	for _, fact := range b.input.Facts.Facts {
		if fact.Anchor == nil {
			continue
		}
		targetID, ok := programTarget[fact.TargetID]
		if !ok || targetID == "" {
			continue
		}
		var direction, kind, method string
		var values []string
		switch fact.Kind {
		case facts.KindHTTPRoute:
			direction, kind, method, values = atlas.DirectionIn, atlas.BoundaryHTTPServer, fact.Method, []string{fact.Path}
		case facts.KindHTTPCall:
			direction, kind, method, values = atlas.DirectionOut, atlas.BoundaryHTTPClient, fact.Method, []string{fact.Path}
		case facts.KindListenAddress:
			direction, kind, values = atlas.DirectionIn, atlas.BoundaryListenAddress, []string{fact.Value}
		case facts.KindConfigRead:
			direction, kind, values = atlas.DirectionOut, atlas.BoundaryConfig, []string{fact.Key}
			if fact.Value != "" {
				values = append(values, "default "+fact.Value)
			}
		default:
			continue
		}
		filePath := atlasPath(fact.Anchor.Path)
		file, ok := b.files[filePath]
		if !ok {
			continue
		}
		encodedValues, _ := json.Marshal(values)
		key := boundaryKey{path: filePath, line: fact.Anchor.Line, column: fact.Anchor.Column, kind: kind,
			method: method, values: string(encodedValues), subject: b.factSubjects[fact.ObjectID]}
		origin := atlas.BoundaryOrigin{TargetID: targetID, FactID: fact.ID, ObjectID: fact.ObjectID}
		if state, exists := b.bounds[key]; exists {
			state.place.Boundary.Origins = append(state.place.Boundary.Origins, origin)
			state.place.TargetIDs = appendUnique(state.place.TargetIDs, targetID)
			continue
		}
		caller, callerDoc := b.callerOf(file, fact.ObjectID, fact.Symbol, fact.Anchor.Line)
		b.bounds[key] = &boundaryState{place: atlas.Place{
			ID: nativeBoundaryID(key), Kind: atlas.PlaceBoundary, Path: filePath,
			LineNo: fact.Anchor.Line, Column: fact.Anchor.Column, Depth: file.depth, TargetIDs: []string{targetID},
			Parent: atlas.FileID(filePath),
			Boundary: &atlas.BoundaryFacts{
				Source: "fact", Origins: []atlas.BoundaryOrigin{origin}, ObjectID: fact.ObjectID, SubjectID: b.factSubjects[fact.ObjectID],
				Caller: caller, CallerDoc: callerDoc, Method: method, Values: values,
				Direction: direction, GivenKind: kind,
			},
		}}
	}
}

// collectExternalCalls lifts calls into non-platform packages that carry a
// literal argument: an SDK client method called with a topic, a table, a
// bucket. The model says what kind of integration it is.
type externalCallCandidate struct {
	path, objectID, caller, external, targetID string
	line                                       int
	values                                     []string
}

// Retain only the boundary observation while this target's index is loaded.
// File docstrings, depths and native boundaries become available after the
// complete file inventory; none of them requires retaining the target index.
func (b *builder) collectExternalCallCandidates(target TargetInput) {
	for _, relation := range target.Index.Relations {
		if relation.Kind != programindex.RelationInvokesExternal {
			continue
		}
		from, ok := b.fileOf[relation.FromID]
		if !ok {
			continue
		}
		var external *programindex.ExternalSymbol
		for _, toID := range relation.ToIDs {
			if object, ok := b.byID[toID]; ok && object.External != nil {
				external = object.External
				break
			}
		}
		if external == nil || external.RepositoryPath != "" || !sdkCandidate(*external) {
			continue
		}
		// JSTS carries compiler-resolved repository origins on each object.
		// A different workspace package with the same npm name is not its origin.
		if language := target.Index.Target.Language; language != "javascript" && language != "typescript" {
			if _, own := b.workspace[external.PackagePath]; own {
				continue
			}
		}
		var values []string
		line := relationLine(relation)
		for _, pattern := range relation.Patterns {
			if pattern.Location != nil && line == 0 {
				line = pattern.Location.Line
			}
			for _, argument := range pattern.Arguments {
				if value, ok := literalArgument(argument); ok {
					values = appendUnique(values, value)
				}
			}
		}
		if len(values) == 0 || line == 0 {
			continue
		}
		if len(values) > 8 {
			values = values[:8]
		}
		b.externalCalls = append(b.externalCalls, externalCallCandidate{
			path: from, line: line, objectID: relation.FromID,
			caller:   displayName(b.byID[relation.FromID], b.byID),
			external: externalName(*external), values: values, targetID: target.Index.Target.ID,
		})
	}
}

func (b *builder) collectExternalCalls() {
	for _, call := range b.externalCalls {
		from, line, values := call.path, call.line, call.values
		claimed := false
		for key := range b.bounds {
			if key.path == from && key.line == line {
				claimed = true
				break
			}
		}
		if claimed {
			continue
		}
		key := boundaryKey{path: from, line: line, kind: "sdk"}
		if state, exists := b.bounds[key]; exists {
			state.place.Boundary.Values = appendUnique(state.place.Boundary.Values, values...)
			state.place.TargetIDs = appendUnique(state.place.TargetIDs, call.targetID)
			continue
		}
		callerName, callerDoc := b.callerOf(b.files[from], call.objectID, call.caller, line)
		b.bounds[key] = &boundaryState{place: atlas.Place{
			ID: boundaryID(from, line, "sdk"), Kind: atlas.PlaceBoundary, Path: from,
			LineNo: line, Depth: b.files[from].depth, TargetIDs: []string{call.targetID},
			Parent: atlas.FileID(from),
			Boundary: &atlas.BoundaryFacts{
				Source: "external_call", ObjectID: call.objectID,
				Caller: callerName, CallerDoc: callerDoc,
				External: call.external, Values: values,
				Direction: atlas.DirectionOut,
			},
		}}
	}
	b.externalCalls = nil
}

func sdkCandidate(external programindex.ExternalSymbol) bool {
	if _, never := packageMatches(external.PackagePath, sdkNeverPackages...); never {
		return false
	}
	for _, known := range sdkPackages {
		if known == "net" {
			// net itself (Dial, Listen), not net/http, whose client calls
			// with a literal URL are already facts.
			if external.PackagePath == "net" {
				return true
			}
			continue
		}
		if external.PackagePath == known || strings.HasPrefix(external.PackagePath, known+"/") ||
			strings.HasPrefix(external.PackagePath, known+".") {
			return true
		}
	}
	return false
}

func externalName(external programindex.ExternalSymbol) string {
	pkg := external.PackagePath
	if slash := strings.LastIndex(pkg, "/"); slash >= 0 {
		pkg = pkg[slash+1:]
	}
	name := external.Name
	if strings.HasPrefix(name, pkg+".") {
		name = strings.TrimPrefix(name, pkg+".")
	}
	if external.Receiver != "" && !strings.HasPrefix(name, external.Receiver+".") {
		name = strings.TrimPrefix(external.Receiver, "*") + "." + name
	}
	return pkg + "." + name
}

func literalArgument(argument programindex.PatternArgument) (string, bool) {
	switch argument.Kind {
	case programindex.PatternLiteralString:
		return argument.Value, argument.Value != ""
	case programindex.PatternStringTemplate:
		var text strings.Builder
		for _, part := range argument.Parts {
			if part.Kind == programindex.PatternPartHole {
				text.WriteString("{param}")
				continue
			}
			text.WriteString(part.Text)
		}
		return text.String(), text.Len() > 0
	default:
		return "", false
	}
}

// packageMatches reports whether a package path is one of the candidates or
// beneath one, tolerating a Go major-version suffix.
func packageMatches(packagePath string, candidates ...string) (string, bool) {
	if slash := strings.LastIndex(packagePath, "/"); slash >= 0 {
		suffix := packagePath[slash+1:]
		if len(suffix) >= 2 && suffix[0] == 'v' && strings.Trim(suffix[1:], "0123456789") == "" {
			packagePath = packagePath[:slash]
		}
	}
	for _, candidate := range candidates {
		if packagePath == candidate || strings.HasPrefix(packagePath, candidate+"/") {
			return candidate, true
		}
	}
	return "", false
}

// callerOf names the declaration a boundary sits in and its docstring.
func (b *builder) callerOf(file *fileState, objectID, symbol string, line int) (string, string) {
	if file == nil {
		return symbol, ""
	}
	if objectID != "" {
		for _, decl := range file.decls {
			if decl.ObjectID == objectID {
				return decl.Name, decl.Doc
			}
		}
	}
	if symbol != "" {
		for _, decl := range file.decls {
			if decl.Name == symbol || strings.HasSuffix(decl.Name, "."+symbol) {
				return decl.Name, decl.Doc
			}
		}
	}
	best := atlas.Decl{}
	for _, decl := range file.decls {
		if decl.LineNo <= line && decl.LineNo >= best.LineNo {
			best = decl
		}
	}
	if best.Name != "" {
		return best.Name, best.Doc
	}
	return symbol, ""
}

func boundaryID(filePath string, line int, kind string) string {
	return fmt.Sprintf("bnd:%s:%d:%s", filePath, line, kind)
}

func nativeBoundaryID(key boundaryKey) string {
	encoded, _ := json.Marshal([]any{key.path, key.line, key.column, key.kind, key.method, key.values, key.subject})
	return fmt.Sprintf("%s:%x", boundaryID(key.path, key.line, key.kind), sha256.Sum256(encoded))
}

func appendUnique(values []string, more ...string) []string {
	for _, value := range more {
		if value == "" {
			continue
		}
		found := false
		for _, existing := range values {
			if existing == value {
				found = true
				break
			}
		}
		if !found {
			values = append(values, value)
		}
	}
	return values
}

// boundaryGiven is the fallback line of a boundary: what it touches, in
// which declaration.
func boundaryGiven(place atlas.Place) string {
	facts := place.Boundary
	subject := facts.External
	if subject == "" {
		subject = facts.GivenKind
	}
	detail := strings.Join(facts.Values, ", ")
	if facts.Method != "" {
		detail = facts.Method + " " + detail
	}
	text := subject
	if detail != "" {
		text += " " + detail
	}
	if facts.Caller != "" {
		text += " in " + facts.Caller
	}
	return truncateRunes(strings.TrimSpace(text), maxLineRunes)
}

func (b *builder) graph() (atlas.Graph, error) {
	graph := atlas.Graph{Version: atlas.GraphVersion, Revision: b.input.Revision}
	for dir, state := range b.dirs {
		place := atlas.Place{
			ID: atlas.DirectoryID(dir), Kind: atlas.PlaceDirectory, Path: dir,
			Depth: treeDepth(dir), TargetIDs: sortedKeys(state.targets),
			Directory: &atlas.DirectoryFacts{
				Readme: state.readme, Doc: state.doc,
				Dirs: sortedKeys(state.dirs), Files: sortedKeys(state.files),
				FileCount: state.count, TopBox: state.topBox,
			},
		}
		if dir != "." {
			place.Parent = atlas.DirectoryID(parentDir(dir))
		}
		place.Given = directoryGiven(place)
		graph.Places = append(graph.Places, place)
	}
	for filePath, state := range b.files {
		place := atlas.Place{
			ID: atlas.FileID(filePath), Kind: atlas.PlaceFile, Path: filePath,
			Depth: state.depth, TargetIDs: sortedKeys(state.targets),
			Parent: atlas.DirectoryID(parentDir(filePath)),
			File: &atlas.FileFacts{
				Doc: state.doc, Decls: state.decls,
				Callers: fileIDs(state.callers), Callees: fileIDs(state.callees),
				Generated: state.generated,
			},
		}
		if place.File.Decls == nil {
			place.File.Decls = []atlas.Decl{}
		}
		place.Given = fileGiven(place)
		graph.Places = append(graph.Places, place)
	}
	graph.Places = append(graph.Places, b.symbols...)
	for _, state := range b.bounds {
		place := state.place
		// Native observations retain only their original target scopes. An
		// external candidate follows its file's ownership as before.
		if place.Boundary.Source != "fact" {
			if file, ok := b.files[place.Path]; ok {
				place.TargetIDs = sortedKeys(file.targets)
			}
		}
		place.Boundary.Origins = atlas.CanonicalBoundaryOrigins(place.Boundary.Origins)
		// This is only the shared row's representative. Target projection uses
		// Origins; semantic context uses the compiler-located SubjectID.
		if len(place.Boundary.Origins) > 0 {
			place.Boundary.ObjectID = place.Boundary.Origins[0].ObjectID
		}
		sort.Strings(place.TargetIDs)
		if place.Boundary.Values == nil {
			place.Boundary.Values = []string{}
		}
		place.Given = boundaryGiven(place)
		graph.Places = append(graph.Places, place)
	}
	for position := range graph.Places {
		sanitizePlace(&graph.Places[position])
	}
	b.addExtractions(&graph)
	b.addSourceFacts(&graph)
	if err := b.addDocuments(&graph); err != nil {
		return atlas.Graph{}, err
	}
	atlas.SortPlaces(graph.Places)
	known := make(map[string]struct{}, len(graph.Places))
	for _, place := range graph.Places {
		known[place.ID] = struct{}{}
	}
	for _, edge := range b.edges {
		// An import of a package whose declarations the index did not reach
		// names a directory with no place; the edge has nowhere to land.
		if _, ok := known[edge.From]; !ok {
			continue
		}
		if _, ok := known[edge.To]; !ok {
			continue
		}
		sort.SliceStable(edge.Witnesses, func(i, j int) bool {
			if edge.Witnesses[i].LineNo != edge.Witnesses[j].LineNo {
				return edge.Witnesses[i].LineNo < edge.Witnesses[j].LineNo
			}
			return edge.Witnesses[i].Caller+edge.Witnesses[i].Callee < edge.Witnesses[j].Caller+edge.Witnesses[j].Callee
		})
		if edge.Witnesses == nil {
			edge.Witnesses = []atlas.Witness{}
		}
		graph.Edges = append(graph.Edges, *edge)
	}
	sort.Slice(graph.Edges, func(i, j int) bool {
		a, c := graph.Edges[i], graph.Edges[j]
		if a.From != c.From {
			return a.From < c.From
		}
		if a.To != c.To {
			return a.To < c.To
		}
		if a.Kind != c.Kind {
			return a.Kind < c.Kind
		}
		if a.Evidence != nil && c.Evidence != nil {
			if a.Evidence.Label != c.Evidence.Label {
				return a.Evidence.Label < c.Evidence.Label
			}
			if a.Evidence.Path != c.Evidence.Path {
				return a.Evidence.Path < c.Evidence.Path
			}
			return a.Evidence.LineNo < c.Evidence.LineNo
		}
		return false
	})
	if graph.Edges == nil {
		graph.Edges = []atlas.Edge{}
	}
	graph.Seeds = make([]string, 0, len(b.seeds))
	for seed := range b.seeds {
		graph.Seeds = append(graph.Seeds, atlas.FileID(seed))
	}
	sort.Strings(graph.Seeds)
	return graph, nil
}

// directoryGiven is the fallback line of a directory: its README's first
// line, its package doc, or what it holds.
func directoryGiven(place atlas.Place) string {
	facts := place.Directory
	if facts.Readme != "" {
		return facts.Readme
	}
	if facts.Doc != "" {
		return facts.Doc
	}
	names := append(append([]string{}, facts.Dirs...), facts.Files...)
	if len(names) > 3 {
		names = names[:3]
	}
	unit := "files"
	if facts.FileCount == 1 {
		unit = "file"
	}
	if len(names) == 0 {
		return fmt.Sprintf("%d %s", facts.FileCount, unit)
	}
	return fmt.Sprintf("%d %s: %s", facts.FileCount, unit, strings.Join(names, ", "))
}

// fileGiven is the fallback line of a file: its module doc, its first
// docstring, or its declarations.
func fileGiven(place atlas.Place) string {
	facts := place.File
	if facts.Doc != "" {
		return facts.Doc
	}
	for _, decl := range facts.Decls {
		if decl.Doc != "" {
			return decl.Doc
		}
	}
	names := make([]string, 0, 3)
	for _, decl := range facts.Decls {
		names = append(names, decl.Name)
		if len(names) == 3 {
			break
		}
	}
	unit := "declarations"
	if len(facts.Decls) == 1 {
		unit = "declaration"
	}
	if len(names) == 0 {
		return "no declarations"
	}
	return fmt.Sprintf("%d %s: %s", len(facts.Decls), unit, strings.Join(names, ", "))
}

// sanitizePlace keeps every text of a place printable: a literal argument
// with a newline or a docstring with a tab would otherwise be refused by the
// atlas, and a request must never carry a control character.
func sanitizePlace(place *atlas.Place) {
	place.Given = cleanText(place.Given)
	if place.Directory != nil {
		place.Directory.Readme = cleanText(place.Directory.Readme)
		place.Directory.Doc = cleanText(place.Directory.Doc)
	}
	if place.File != nil {
		place.File.Doc = cleanText(place.File.Doc)
		for i := range place.File.Decls {
			place.File.Decls[i].Doc = cleanText(place.File.Decls[i].Doc)
			place.File.Decls[i].Signature = cleanText(place.File.Decls[i].Signature)
		}
	}
	if place.Symbol != nil {
		place.Symbol.Decl.Doc = cleanText(place.Symbol.Decl.Doc)
		place.Symbol.Decl.Signature = cleanText(place.Symbol.Decl.Signature)
		for i := range place.Symbol.Members {
			place.Symbol.Members[i].Decl.Doc = cleanText(place.Symbol.Members[i].Decl.Doc)
			place.Symbol.Members[i].Decl.Signature = cleanText(place.Symbol.Members[i].Decl.Signature)
		}
	}
	if place.Boundary != nil {
		place.Boundary.CallerDoc = cleanText(place.Boundary.CallerDoc)
		for i := range place.Boundary.Values {
			place.Boundary.Values[i] = cleanText(place.Boundary.Values[i])
		}
	}
}

// cleanText replaces control characters with spaces and collapses runs.
func cleanText(text string) string {
	if text == "" {
		return ""
	}
	dirty := false
	for _, r := range text {
		if r < 0x20 || r == 0x7f {
			dirty = true
			break
		}
	}
	if !dirty {
		return text
	}
	fields := strings.FieldsFunc(text, func(r rune) bool { return r < 0x20 || r == 0x7f || unicode.IsSpace(r) })
	return strings.Join(fields, " ")
}

func fileIDs(set map[string]struct{}) []string {
	result := make([]string, 0, len(set))
	for filePath := range set {
		result = append(result, atlas.FileID(filePath))
	}
	sort.Strings(result)
	return result
}

func sortedKeys(set map[string]struct{}) []string {
	result := make([]string, 0, len(set))
	for key := range set {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func parentDir(filePath string) string {
	dir := path.Dir(filePath)
	if dir == "" || dir == "/" {
		return "."
	}
	return dir
}

func treeDepth(dir string) int {
	if dir == "." {
		return 0
	}
	return strings.Count(dir, "/") + 1
}

func atlasPath(value string) string {
	value = strings.TrimPrefix(strings.ReplaceAll(value, "\\", "/"), "./")
	value = path.Clean(value)
	if value == "" || value == "/" {
		return "."
	}
	return strings.TrimPrefix(value, "/")
}

// firstSentence keeps the first sentence of a docstring, bounded.
func firstSentence(text string) string {
	text = strings.TrimSpace(strings.Join(strings.Fields(text), " "))
	if text == "" {
		return ""
	}
	for i, r := range text {
		if (r == '.' || r == '!' || r == '?') && (i+1 == len(text) || text[i+1] == ' ') {
			// Do not cut "e.g." or a version like "v1.2".
			if i >= 2 && (unicode.IsDigit(rune(text[i-1])) && i+1 < len(text) && unicode.IsDigit(rune(text[i+1]))) {
				continue
			}
			return truncateRunes(text[:i+1], maxLineRunes)
		}
	}
	return truncateRunes(text, maxLineRunes)
}

func truncateRunes(text string, limit int) string {
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	runes := []rune(text)
	cut := limit - 1
	for cut > limit/2 && !unicode.IsSpace(runes[cut]) {
		cut--
	}
	return strings.TrimSpace(string(runes[:cut])) + "…"
}
