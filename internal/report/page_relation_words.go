package report

import (
	"encoding/json"
	"strings"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// relationPhrases is the one closed vocabulary a relation row is said in:
// its kind chooses the sentence and the code names fill it. A stored label
// had printed the kind as it was written ("initServer passes_callback
// acceptHandler"), and a joint between two programs as "integrates with"
// with neither function named. Each phrase is a UI message, translated with
// the rest of the report's own words; the names stay verbatim.
var relationPhrases = map[string]string{
	string(programindex.RelationCalls):               "{0} calls {1}",
	string(programindex.RelationInvokesExternal):     "{0} calls {1}",
	string(programindex.RelationPassesCallback):      "{0} passes {1} as a callback",
	string(programindex.RelationBindsImplementation): "{0} supplies {1} as an implementation",
	string(programindex.RelationDecorates):           "{0} is decorated by {1}",
	string(programindex.RelationExecutes):            "{0} runs {1}",
	string(programindex.RelationImports):             "{0} imports {1}",
	relationIncludes:                                 "{0} includes {1}",
	string(programindex.RelationImplements):          "{0} implements {1}",
	string(programindex.RelationReads):               "{0} reads {1}",
	string(programindex.RelationWrites):              "{0} writes {1}",
	string(programindex.RelationSources):             "{0} sources {1}",
	relationIntegration:                              "{0} connects to {1}",
}

const (
	// relationIncludes is a C program's import: an #include is said as one.
	relationIncludes = "includes"
	// relationIntegration is a matched joint between two programs, from the
	// declaration that sends to the one that takes it.
	relationIntegration = "integration"
)

// relationWord is the vocabulary kind of one native relation in a program
// of the given language, or "" when the vocabulary has no phrase for it.
func relationWord(kind, language string) string {
	if kind == string(programindex.RelationImports) && strings.EqualFold(language, "c") {
		kind = relationIncludes
	}
	if _, known := relationPhrases[kind]; !known {
		return ""
	}
	return kind
}

// Phrase is the vocabulary message this row is said in, or "" when the row
// is a sentence of its own (the model's) and not one relation between two
// named declarations.
func (row pageConnection) Phrase() string {
	if row.FromName == "" || row.ToName == "" {
		return ""
	}
	return relationPhrases[row.Kind]
}

// FromKey and ToKey identify the declarations at the row's ends the way the
// page's script keys a declaration (its link, its editor action, or its
// place when it has neither), so a declaration's reading finds the rows
// that call it and that it calls without matching names.
func (row pageConnection) FromKey() string { return declarationKey(row.FromDecl) }
func (row pageConnection) ToKey() string   { return declarationKey(row.ToDecl) }

func declarationKey(anchor *pageAnchor) string {
	switch {
	case anchor == nil:
		return ""
	case anchor.Href != "":
		return anchor.Href
	case anchor.Open != "":
		return anchor.Open
	case !anchor.NoSource:
		return ""
	case anchor.Path != "":
		raw, err := json.Marshal([]any{anchor.Path, anchor.Line})
		if err != nil {
			return ""
		}
		return string(raw)
	default:
		return anchor.Text
	}
}

// targetLanguage is the language of the program a target's section reads.
func (builder *pageBuilder) targetLanguage(targetID string) string {
	if section := builder.byProgram[targetID]; section != nil {
		return section.Language
	}
	return ""
}

// nameConnectionEnds says a stored connection as one relation between two
// declarations when the connection is one: a native relation, whose stored
// label carries its kind as written, or a joint between two programs,
// whose ends are the declarations its boundaries name (redis-cli's
// anetTcpConnect and redis-server's anetAccept, not "integrates with").
// A connection the model described in its own words keeps them.
func (builder *pageBuilder) nameConnectionEnds(row *pageConnection, connection groupindex.Connection) {
	kind := ""
	switch {
	case strings.HasPrefix(connection.SourceKind, "native_"):
		kind = relationWord(strings.TrimPrefix(connection.SourceKind, "native_"), builder.targetLanguage(connection.From.TargetID))
	case connection.SourceKind == "integration":
		kind = relationIntegration
	}
	if kind == "" || connection.FromSubjectID == "" || connection.ToSubjectID == "" {
		return
	}
	from, fromKnown := builder.subject(connection.From.TargetID, connection.FromSubjectID)
	to, toKnown := builder.subject(connection.To.TargetID, connection.ToSubjectID)
	if !fromKnown || !toKnown {
		return
	}
	fromName, fromDecl := builder.subjectDisplay(from.subject)
	toName, toDecl := builder.subjectDisplay(to.subject)
	if fromName == "" || toName == "" {
		return
	}
	row.Kind, row.FromName, row.ToName, row.FromDecl, row.ToDecl = kind, fromName, toName, fromDecl, toDecl
}

// connectionCall is a stored connection as the call its arrow's card lists,
// or nil when the connection is a sentence of its own. The call carries its
// relation and the names and keys of its two declarations as fields of
// their own, never one label the card would split again: a name may hold
// spaces ("StartLdapServer (inline, 3)", GroupsIndex's accepted name of a
// callable written inline), and casdoor's LDAP connection had counted a
// call its card could not read and so never listed (external review,
// 2026-10-03). The relation is the vocabulary's words when its phrase
// stands between the two names ("connects to", which had been "integrates
// with" and named no function), and the relation's own kind when the
// phrase wraps the callee ("passes {1} as a callback" is "passes callback"
// on the card), with underscores for its spaces. The arrow's calls are not
// translated.
func (builder *pageBuilder) connectionCall(connection groupindex.Connection) *pageEdgeCall {
	var said pageConnection
	builder.nameConnectionEnds(&said, connection)
	phrase := said.Phrase()
	if phrase == "" {
		return nil
	}
	relation, between := strings.CutPrefix(phrase, "{0} ")
	relation, ends := strings.CutSuffix(relation, " {1}")
	if !between || !ends || strings.Contains(relation, "{") {
		relation = said.Kind
	}
	call := &pageEdgeCall{Kind: strings.ReplaceAll(relation, " ", "_"), CallerName: said.FromName, CalleeName: said.ToName, Caller: said.FromKey(), Callee: said.ToKey()}
	if location := connection.FromLocation; location != nil {
		from := builder.links.anchor(location.Path, location.Line, location.Column)
		call.From, call.At = from.Href, from.Text
	}
	if location := connection.ToLocation; location != nil {
		call.To = builder.links.anchor(location.Path, location.Line, location.Column).Href
	}
	// A joint between two programs names each program's side of it.
	if connection.SourceKind == "integration" && connection.From.TargetID != connection.To.TargetID {
		from, to := builder.callSide(connection.From.TargetID, connection.FromSubjectID), builder.callSide(connection.To.TargetID, connection.ToSubjectID)
		if from != nil && to != nil {
			call.Sides = []pageCallSide{*from, *to}
		}
	}
	builder.foldCall(call, connection)
	return call
}
