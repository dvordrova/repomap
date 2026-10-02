package report

import (
	"cmp"
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/groupindex"
)

// Same-named inputs told apart (reviewer, 2026-10-02: etcd's server read
// fourteen identical "POST" under Election and lock APIs, and a reader
// looking for Campaign or Lock had to open them one by one). An input keeps
// its name as registered, verbatim; beside it stand the words that tell it
// apart from the inputs of its kind its program names alike, chosen here
// when the page is assembled from saved facts, and the list, the canvas
// tile and the reading's heading show them as saved.

// pageApartWord is one word beside an input's name: Of says what it is
// and At where it is written, for its hover.
type pageApartWord struct {
	Word string `json:"word"`
	// Of is one of apartOf*.
	Of string `json:"of"`
	At string `json:"at,omitempty"`
}

// What a word beside a name is, in the order they are tried.
const (
	// apartOptions: the subcommands it is an option of (freqtrade's two
	// "--erase", of download-data and of install-ui).
	apartOptions = "options"
	// apartDeclared: the declaration declaring a catalogue's input
	// (dataformat_ohlcv in SCHEMA_TRADE_REQUIRED).
	apartDeclared = "declared"
	// apartKey: its registration's own word beyond its name (GroupsIndex
	// Operation.Key: version_main beside -V --version).
	apartKey = "key"
	// apartHandler: a word its handler's code declares, an entry of its
	// kind nothing but that handler declares (GroupsIndex
	// Reach.SubArguments): etcd's Campaign POST passes
	// "/v3electionpb.Election/Campaign" in its handler.
	apartHandler = "handler"
	// apartRegistered: the function its registration is written in
	// (RegisterElectionHandlerServer, RegisterElectionHandlerClient).
	apartRegistered = "registered"
)

// pageApartFacts are an input's facts a word beside its name may come
// from, gathered with the input.
type pageApartFacts struct {
	// options are the node IDs of its own options (Reach.Options).
	options []string
	// handler are the words its handler's code declares, in source order.
	handler []pageApartWord
	// registered is the function its registration is written in.
	registered pageApartWord
}

// apartFacts gathers an input's facts for telling it apart.
func (builder *pageBuilder) apartFacts(index *groupindex.Index, operation groupindex.Operation, reach groupindex.Reach, inputNode func(string) string) pageApartFacts {
	var facts pageApartFacts
	for _, id := range reach.Options {
		facts.options = append(facts.options, inputNode(id))
	}
	// Only what the handler itself declares, of the input's own kind: a
	// value another entry reads (Reach.SubArguments' ValueOf) or a setting
	// read in the handler's body names no request.
	var words []groupindex.Operation
	for _, id := range reach.SubArguments {
		for _, other := range index.Operations {
			if other.ID == id && operation.SubjectID != "" && other.DeclaredBy == operation.SubjectID && other.Kind == operation.Kind {
				words = append(words, other)
			}
		}
	}
	slices.SortStableFunc(words, func(a, b groupindex.Operation) int {
		return cmp.Or(strings.Compare(a.Location.Path, b.Location.Path), cmp.Compare(a.Location.Line, b.Location.Line), cmp.Compare(a.Location.Column, b.Location.Column))
	})
	for _, word := range words {
		if word.Name != "" && !strings.ContainsAny(word.Name, "\n\r") {
			facts.handler = append(facts.handler, pageApartWord{Word: word.Name, Of: apartHandler, At: placeText(word.Location.Path, word.Location.Line)})
		}
	}
	if ref, known := builder.subject(index.Target.ID, operation.DeclaredBy); known && operation.DeclaredBy != "" {
		if name, anchor := builder.subjectDisplay(ref.subject); name != "" {
			facts.registered = pageApartWord{Word: builder.withType(index.Target.ID, ref.subject, name), Of: apartRegistered}
			if anchor != nil {
				facts.registered.At = anchor.Text
			}
		}
	}
	return facts
}

func placeText(path string, line int) string {
	if line > 0 {
		return path + ":" + strconv.Itoa(line)
	}
	return path
}

// tellInputsApart sets, for every input of one program sharing its name
// and kind with another, the words beside its name. Each input takes the
// first word that tells it apart, tried in order: the subcommands it is an
// option of, its catalogue's declaring declaration, its key, a word its
// handler declares, the function registering it; a kind of word every one
// of them has alike tells none apart, and an input lacking one tries the
// next (etcd's Observe of RegisterElectionHandlerServer passes no word in
// its handler and reads by its registering function). Inputs still alike
// add the next word that differs among them (Campaign registered by
// RegisterElectionHandlerServer and by RegisterElectionHandlerClient). No
// word is made up: each is written in the code, and two inputs nothing
// tells apart keep their names alike.
func tellInputsApart(nodes []*pageMapNode) {
	byName := map[string][]*pageMapNode{}
	var order []string
	for _, node := range nodes {
		key := node.Activation + "\x00" + node.FullTitle
		if _, seen := byName[key]; !seen {
			order = append(order, key)
		}
		byName[key] = append(byName[key], node)
	}
	// The subcommands each input is an option of, by name.
	holders := map[string][]string{}
	for _, node := range nodes {
		for _, option := range node.apart.options {
			if !slices.Contains(holders[option], node.FullTitle) {
				holders[option] = append(holders[option], node.FullTitle)
			}
		}
	}
	for _, key := range order {
		members := byName[key]
		if len(members) < 2 {
			continue
		}
		levels := make([][]pageApartWord, len(members))
		for position, node := range members {
			var handler pageApartWord
			if len(node.apart.handler) > 0 {
				handler = node.apart.handler[0]
			}
			var declared pageApartWord
			if node.DeclaredBy != "" {
				declared = pageApartWord{Word: node.DeclaredBy, Of: apartDeclared}
			}
			var key pageApartWord
			if node.Key != "" {
				key = pageApartWord{Word: node.Key, Of: apartKey}
			}
			var options pageApartWord
			if held := holders[node.ID]; len(held) > 0 {
				options = pageApartWord{Word: strings.Join(held, ", "), Of: apartOptions}
			}
			levels[position] = []pageApartWord{options, declared, key, handler, node.apart.registered}
		}
		chosen := chooseApart(levels)
		for position, node := range members {
			if len(chosen[position]) == 0 {
				continue
			}
			node.apartWords = chosen[position]
			raw, err := json.Marshal(chosen[position])
			if err == nil {
				node.Apart = string(raw)
			}
		}
	}
}

// chooseApart picks each member's words from its levels (see
// tellInputsApart).
func chooseApart(levels [][]pageApartWord) [][]pageApartWord {
	if len(levels) == 0 {
		return nil
	}
	count := len(levels[0])
	// A level tells something apart when some member has a word there and
	// not every member has the same one.
	tells := func(members []int, level int) bool {
		seen := map[string]bool{}
		has := 0
		for _, member := range members {
			if word := levels[member][level].Word; word != "" {
				seen[word] = true
				has++
			}
		}
		return has > 0 && (len(seen) > 1 || has < len(members))
	}
	all := make([]int, len(levels))
	for position := range all {
		all[position] = position
	}
	chosen := make([][]pageApartWord, len(levels))
	last := make([]int, len(levels))
	for member := range levels {
		last[member] = count
		for level := 0; level < count; level++ {
			if tells(all, level) && levels[member][level].Word != "" {
				chosen[member], last[member] = []pageApartWord{levels[member][level]}, level
				break
			}
		}
	}
	// Members still alike add the next word that differs among them.
	for changed := true; changed; {
		changed = false
		alike := map[string][]int{}
		var keys []string
		for member, words := range chosen {
			var said []string
			for _, word := range words {
				said = append(said, word.Word)
			}
			key := strings.Join(said, "\x00")
			if _, seen := alike[key]; !seen {
				keys = append(keys, key)
			}
			alike[key] = append(alike[key], member)
		}
		for _, key := range keys {
			members := alike[key]
			if len(members) < 2 {
				continue
			}
			from := count
			for _, member := range members {
				from = min(from, last[member]+1)
			}
			for level := from; level < count; level++ {
				if !tells(members, level) {
					continue
				}
				for _, member := range members {
					if word := levels[member][level]; word.Word != "" {
						chosen[member] = append(chosen[member], word)
					}
					last[member] = level
				}
				changed = true
				break
			}
		}
	}
	return chosen
}
