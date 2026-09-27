package cproject

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// clang's JSON dumper writes a location's file and line only when they differ
// from the previous location it wrote anywhere in the document. The decoder
// therefore reads the document strictly in order, one value at a time, and
// replays that state through every location, including the ones inside
// declarations it does not keep (system headers). Within a location the
// spelling comes before the expansion; presumedFile, presumedLine and
// includedFrom never change the state.
//
// Only top-level declarations whose expansion location is a corpus file are
// materialized; everything else is scanned for its locations and dropped, so
// memory follows the repository's own code rather than the dump's size.

// fileNames resolves clang's file names ("x.c", "./x.h", absolute paths) to
// corpus paths, or to cleaned absolute paths for files outside the corpus.
type fileNames struct {
	cwd    string   // absolute directory clang ran in
	roots  []string // the repository root as given and with symlinks resolved
	corpus map[string]bool
	cache  map[string]fileName
}

type fileName struct{ raw, path string }

func (names *fileNames) resolve(raw []byte) fileName {
	if name, ok := names.cache[string(raw)]; ok {
		return name
	}
	text := string(raw)
	name := fileName{raw: text, path: names.normalize(text)}
	names.cache[text] = name
	return name
}

func (names *fileNames) normalize(raw string) string {
	if raw == "" || strings.HasPrefix(raw, "<") {
		return raw
	}
	path := raw
	if !filepath.IsAbs(path) {
		path = filepath.Join(names.cwd, path)
	}
	path = filepath.Clean(path)
	for _, root := range names.roots {
		if rel, ok := under(root, path); ok {
			rel = filepath.ToSlash(rel)
			if names.corpus[rel] {
				return rel
			}
			break
		}
	}
	return path
}

// under reports path relative to dir when path is dir or below it.
func under(dir, path string) (string, bool) {
	if dir == "" {
		return "", false
	}
	if path == dir {
		return ".", true
	}
	prefix := dir
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	if strings.HasPrefix(path, prefix) {
		return path[len(prefix):], true
	}
	return "", false
}

type dumpMode int

const (
	// modeUnit keeps corpus top-level declarations of one complete dump.
	modeUnit dumpMode = iota
	// modeFilter reads clang's -ast-dump-filter output: a sequence of
	// independent dumps, of which only FunctionDecl main is kept.
	modeFilter
)

type decoder struct {
	in    io.Reader
	buf   []byte
	pos   int
	end   int
	read  int64
	err   error
	key   []byte
	text  []byte
	names *fileNames
	atoms map[string]string

	// The location state clang's dumper elides against.
	file fileName
	line int

	decls    []*Node
	external []ExternalDecl
}

func newDecoder(in io.Reader, names *fileNames) *decoder {
	return &decoder{in: in, buf: make([]byte, 256<<10), names: names, atoms: map[string]string{}}
}

// decodeUnit reads one complete clang JSON AST dump.
func decodeUnit(in io.Reader, names *fileNames) (decls []*Node, external []ExternalDecl, bytes int64, err error) {
	d := newDecoder(in, names)
	if err := d.root(); err != nil {
		return nil, nil, d.offset(), err
	}
	if c := d.peek(); c != 0 {
		return nil, nil, d.offset(), d.fail("trailing data after the dump")
	}
	if d.err != nil && !errors.Is(d.err, io.EOF) {
		return nil, nil, d.offset(), d.err
	}
	return d.decls, d.external, d.offset(), nil
}

// decodeFiltered reads the dumps -ast-dump-filter prints and returns the
// FunctionDecl nodes named main with their children.
func decodeFiltered(in io.Reader, names *fileNames) ([]*Node, error) {
	d := newDecoder(in, names)
	for d.peek() != 0 {
		// Every filtered declaration is dumped by a fresh dumper.
		d.file, d.line = fileName{}, 0
		node := &Node{}
		if err := d.node(node, modeFilter, 0); err != nil {
			return nil, err
		}
		if node.Kind == "FunctionDecl" && node.Name == "main" {
			d.decls = append(d.decls, node)
		}
	}
	if d.err != nil && !errors.Is(d.err, io.EOF) {
		return nil, d.err
	}
	return d.decls, nil
}

func (d *decoder) offset() int64 { return d.read + int64(d.pos) }

func (d *decoder) fail(format string, args ...any) error {
	return fmt.Errorf("clang JSON at byte %d: %s", d.offset(), fmt.Sprintf(format, args...))
}

func (d *decoder) fill() bool {
	if d.err != nil {
		return false
	}
	if d.pos > 0 {
		copy(d.buf, d.buf[d.pos:d.end])
		d.read += int64(d.pos)
		d.end -= d.pos
		d.pos = 0
	}
	if d.end == len(d.buf) {
		d.buf = append(d.buf, make([]byte, len(d.buf))...)
	}
	n, err := d.in.Read(d.buf[d.end:])
	d.end += n
	if err != nil {
		d.err = err
	}
	return n > 0 || err == nil
}

// peek skips white space and returns the next byte, or 0 at the end.
func (d *decoder) peek() byte {
	for {
		for d.pos < d.end {
			switch c := d.buf[d.pos]; c {
			case ' ', '\n', '\r', '\t':
				d.pos++
			default:
				return c
			}
		}
		if !d.fill() {
			return 0
		}
	}
}

func (d *decoder) expect(c byte) error {
	if got := d.peek(); got != c {
		if got == 0 {
			return d.fail("unexpected end, want %q", c)
		}
		return d.fail("got %q, want %q", got, c)
	}
	d.pos++
	return nil
}

// more consumes the separator after a member and reports whether another
// member follows before close.
func (d *decoder) more(close byte) (bool, error) {
	switch c := d.peek(); c {
	case ',':
		d.pos++
		return true, nil
	case close:
		d.pos++
		return false, nil
	case 0:
		return false, d.fail("unexpected end, want %q", close)
	default:
		return false, d.fail("got %q, want ',' or %q", c, close)
	}
}

// open consumes '{' or '[' and reports whether the container is empty (and
// then consumed completely).
func (d *decoder) open(open, close byte) (bool, error) {
	if err := d.expect(open); err != nil {
		return false, err
	}
	if d.peek() == close {
		d.pos++
		return true, nil
	}
	return false, nil
}

// readKey reads an object key and its colon into d.key.
func (d *decoder) readKey() error {
	var err error
	if d.key, err = d.readString(d.key[:0]); err != nil {
		return err
	}
	return d.expect(':')
}

// readString appends the decoded JSON string at the input to dst.
func (d *decoder) readString(dst []byte) ([]byte, error) {
	if err := d.expect('"'); err != nil {
		return dst, err
	}
	for {
		start := d.pos
		for d.pos < d.end {
			c := d.buf[d.pos]
			if c == '"' || c == '\\' {
				break
			}
			d.pos++
		}
		dst = append(dst, d.buf[start:d.pos]...)
		if d.pos == d.end {
			if !d.fill() {
				return dst, d.fail("unterminated string")
			}
			continue
		}
		c := d.buf[d.pos]
		d.pos++
		if c == '"' {
			return dst, nil
		}
		escape, err := d.readByte()
		if err != nil {
			return dst, err
		}
		switch escape {
		case '"', '\\', '/':
			dst = append(dst, escape)
		case 'b':
			dst = append(dst, '\b')
		case 'f':
			dst = append(dst, '\f')
		case 'n':
			dst = append(dst, '\n')
		case 'r':
			dst = append(dst, '\r')
		case 't':
			dst = append(dst, '\t')
		case 'u':
			r, err := d.readHex()
			if err != nil {
				return dst, err
			}
			if utf16.IsSurrogate(r) {
				if next, _ := d.readByte(); next == '\\' {
					if u, _ := d.readByte(); u == 'u' {
						low, err := d.readHex()
						if err != nil {
							return dst, err
						}
						r = utf16.DecodeRune(r, low)
					}
				}
			}
			dst = utf8.AppendRune(dst, r)
		default:
			return dst, d.fail("invalid escape %q", escape)
		}
	}
}

// skipString skips a JSON string without decoding it.
func (d *decoder) skipString() error {
	if err := d.expect('"'); err != nil {
		return err
	}
	for {
		for d.pos < d.end {
			switch d.buf[d.pos] {
			case '"':
				d.pos++
				return nil
			case '\\':
				if d.pos+1 == d.end {
					// Keep the escape and its byte in one window.
					goto refill
				}
				d.pos += 2
			default:
				d.pos++
			}
		}
	refill:
		if !d.fill() || d.pos == d.end {
			return d.fail("unterminated string")
		}
	}
}

func (d *decoder) readByte() (byte, error) {
	if d.pos == d.end && !d.fill() {
		return 0, d.fail("unexpected end")
	}
	if d.pos == d.end {
		return 0, d.fail("unexpected end")
	}
	c := d.buf[d.pos]
	d.pos++
	return c, nil
}

func (d *decoder) readHex() (rune, error) {
	var r rune
	for range 4 {
		c, err := d.readByte()
		if err != nil {
			return 0, err
		}
		switch {
		case c >= '0' && c <= '9':
			r = r<<4 | rune(c-'0')
		case c >= 'a' && c <= 'f':
			r = r<<4 | rune(c-'a'+10)
		case c >= 'A' && c <= 'F':
			r = r<<4 | rune(c-'A'+10)
		default:
			return 0, d.fail("invalid \\u escape")
		}
	}
	return r, nil
}

// readNumber reads a JSON number's text into d.text.
func (d *decoder) readNumber() ([]byte, error) {
	d.text = d.text[:0]
	d.peek()
	for {
		for d.pos < d.end {
			c := d.buf[d.pos]
			if !(c >= '0' && c <= '9' || c == '-' || c == '+' || c == '.' || c == 'e' || c == 'E') {
				if len(d.text) == 0 {
					return nil, d.fail("invalid value %q", c)
				}
				return d.text, nil
			}
			d.text = append(d.text, c)
			d.pos++
		}
		if !d.fill() {
			if len(d.text) == 0 {
				return nil, d.fail("unexpected end")
			}
			return d.text, nil
		}
	}
}

func (d *decoder) readInt() (int, error) {
	text, err := d.readNumber()
	if err != nil {
		return 0, err
	}
	value, err := strconv.Atoi(string(text))
	if err != nil {
		return 0, d.fail("invalid integer %q", text)
	}
	return value, nil
}

func (d *decoder) readBool() (bool, error) {
	switch d.peek() {
	case 't':
		return true, d.literal("true")
	case 'f':
		return false, d.literal("false")
	default:
		return false, d.fail("want a boolean")
	}
}

func (d *decoder) literal(word string) error {
	for i := range len(word) {
		c, err := d.readByte()
		if err != nil {
			return err
		}
		if c != word[i] {
			return d.fail("invalid literal")
		}
	}
	return nil
}

// atom reads a string value shared by many nodes (kinds, types, names).
func (d *decoder) atom() (string, error) {
	var err error
	if d.text, err = d.readString(d.text[:0]); err != nil {
		return "", err
	}
	if value, ok := d.atoms[string(d.text)]; ok {
		return value, nil
	}
	value := string(d.text)
	d.atoms[value] = value
	return value, nil
}

func (d *decoder) str() (string, error) {
	var err error
	d.text, err = d.readString(d.text[:0])
	return string(d.text), err
}

// scalarText reads a string or a number as text (clang prints a character
// literal's value as a number and other literal values as strings).
func (d *decoder) scalarText() (string, error) {
	switch c := d.peek(); {
	case c == '"':
		return d.str()
	case c == '-' || c >= '0' && c <= '9':
		text, err := d.readNumber()
		return string(text), err
	default:
		return "", d.skim()
	}
}

// skim skips any value while replaying every location inside it: an object
// whose first key is offset is a bare location.
func (d *decoder) skim() error {
	switch c := d.peek(); {
	case c == '{':
		empty, err := d.open('{', '}')
		if err != nil || empty {
			return err
		}
		if err := d.readKey(); err != nil {
			return err
		}
		if string(d.key) == "offset" {
			var position Position
			return d.bareRest(&position, nil)
		}
		for {
			if err := d.skim(); err != nil {
				return err
			}
			more, err := d.more('}')
			if err != nil || !more {
				return err
			}
			if err := d.readKey(); err != nil {
				return err
			}
		}
	case c == '[':
		empty, err := d.open('[', ']')
		if err != nil || empty {
			return err
		}
		for {
			if err := d.skim(); err != nil {
				return err
			}
			more, err := d.more(']')
			if err != nil || !more {
				return err
			}
		}
	case c == '"':
		return d.skipString()
	case c == 't':
		return d.literal("true")
	case c == 'f':
		return d.literal("false")
	case c == 'n':
		return d.literal("null")
	case c == 0:
		return d.fail("unexpected end")
	default:
		_, err := d.readNumber()
		return err
	}
}

// bareRest reads the rest of a bare location whose "offset" key was just read
// and applies it to the elided state. macroArg receives isMacroArgExpansion.
func (d *decoder) bareRest(position *Position, macroArg *bool) error {
	var fileSeen, lineSeen bool
	var file fileName
	for first := true; ; first = false {
		if !first {
			more, err := d.more('}')
			if err != nil {
				return err
			}
			if !more {
				break
			}
			if err := d.readKey(); err != nil {
				return err
			}
		}
		var err error
		switch string(d.key) {
		case "offset":
			position.Offset, err = d.readInt()
		case "file":
			if d.text, err = d.readString(d.text[:0]); err == nil {
				file, fileSeen = d.names.resolve(d.text), true
			}
		case "line":
			position.Line, err = d.readInt()
			lineSeen = true
		case "col":
			position.Col, err = d.readInt()
		case "tokLen":
			position.TokLen, err = d.readInt()
		case "isMacroArgExpansion":
			var value bool
			if value, err = d.readBool(); err == nil && macroArg != nil {
				*macroArg = value
			}
		default: // includedFrom, presumedFile, presumedLine
			err = d.skim()
		}
		if err != nil {
			return err
		}
	}
	if fileSeen {
		d.file = file
	}
	if lineSeen {
		d.line = position.Line
	}
	position.File, position.Line = d.file.path, d.line
	return nil
}

// bare reads a bare location object; an empty object is an invalid location
// and leaves the state alone.
func (d *decoder) bare(position *Position, macroArg *bool) error {
	empty, err := d.open('{', '}')
	if err != nil || empty {
		return err
	}
	if err := d.readKey(); err != nil {
		return err
	}
	if string(d.key) != "offset" {
		// Not a location clang would print; replay whatever it holds.
		for {
			if err := d.skim(); err != nil {
				return err
			}
			more, err := d.more('}')
			if err != nil || !more {
				return err
			}
			if err := d.readKey(); err != nil {
				return err
			}
		}
	}
	return d.bareRest(position, macroArg)
}

// location reads a source location: a bare location, or a spelling and an
// expansion location for a token a macro produced.
func (d *decoder) location(loc *Loc) error {
	empty, err := d.open('{', '}')
	if err != nil || empty {
		return err
	}
	if err := d.readKey(); err != nil {
		return err
	}
	if string(d.key) == "offset" {
		if err := d.bareRest(&loc.Spelling, nil); err != nil {
			return err
		}
		loc.Expansion = loc.Spelling
		return nil
	}
	for {
		var err error
		switch string(d.key) {
		case "spellingLoc":
			err = d.bare(&loc.Spelling, nil)
		case "expansionLoc":
			err = d.bare(&loc.Expansion, &loc.MacroArg)
		default:
			err = d.skim()
		}
		if err != nil {
			return err
		}
		more, err := d.more('}')
		if err != nil || !more {
			return err
		}
		if err := d.readKey(); err != nil {
			return err
		}
	}
}

func (d *decoder) sourceRange(begin, end *Loc) error {
	empty, err := d.open('{', '}')
	if err != nil || empty {
		return err
	}
	for {
		if err := d.readKey(); err != nil {
			return err
		}
		switch string(d.key) {
		case "begin":
			err = d.location(begin)
		case "end":
			err = d.location(end)
		default:
			err = d.skim()
		}
		if err != nil {
			return err
		}
		more, err := d.more('}')
		if err != nil || !more {
			return err
		}
	}
}

func (d *decoder) typ(value *Type) error {
	empty, err := d.open('{', '}')
	if err != nil || empty {
		return err
	}
	for {
		if err := d.readKey(); err != nil {
			return err
		}
		switch string(d.key) {
		case "qualType":
			value.QualType, err = d.atom()
		case "desugaredQualType":
			value.Desugared, err = d.atom()
		default:
			err = d.skim()
		}
		if err != nil {
			return err
		}
		more, err := d.more('}')
		if err != nil || !more {
			return err
		}
	}
}

func (d *decoder) declRef() (*DeclRef, error) {
	ref := &DeclRef{}
	empty, err := d.open('{', '}')
	if err != nil || empty {
		return ref, err
	}
	for {
		if err := d.readKey(); err != nil {
			return nil, err
		}
		switch string(d.key) {
		case "id":
			ref.ID, err = d.str()
		case "kind":
			ref.Kind, err = d.atom()
		case "name":
			ref.Name, err = d.atom()
		case "type":
			err = d.typ(&ref.Type)
		default:
			err = d.skim()
		}
		if err != nil {
			return nil, err
		}
		more, err := d.more('}')
		if err != nil || !more {
			return ref, err
		}
	}
}

// keepTop decides, before a top-level node's children, whether it is the
// repository's own declaration.
func (d *decoder) keepTop(node *Node) bool {
	return !node.IsImplicit && d.names.corpus[node.Loc.Expansion.File]
}

// node reads one AST node. depth is 0 for a top-level node of a unit dump.
func (d *decoder) node(n *Node, mode dumpMode, depth int) error {
	empty, err := d.open('{', '}')
	if err != nil || empty {
		return err
	}
	keep := true
	decided := false
	for {
		if err := d.readKey(); err != nil {
			return err
		}
		var err error
		switch string(d.key) {
		case "id":
			n.ID, err = d.str()
		case "kind":
			n.Kind, err = d.atom()
		case "loc":
			err = d.location(&n.Loc)
		case "range":
			err = d.sourceRange(&n.Begin, &n.End)
		case "name":
			n.Name, err = d.atom()
		case "type":
			err = d.typ(&n.Type)
		case "storageClass":
			n.StorageClass, err = d.atom()
		case "inline":
			n.Inline, err = d.readBool()
		case "variadic":
			n.Variadic, err = d.readBool()
		case "isImplicit":
			n.IsImplicit, err = d.readBool()
		case "isUsed":
			n.IsUsed, err = d.readBool()
		case "isReferenced":
			n.IsReferenced, err = d.readBool()
		case "previousDecl":
			n.PreviousDecl, err = d.str()
		case "init":
			n.Init, err = d.atom()
		case "tagUsed":
			n.TagUsed, err = d.atom()
		case "completeDefinition":
			n.CompleteDefinition, err = d.readBool()
		case "isBitfield":
			n.IsBitfield, err = d.readBool()
		case "castKind":
			n.CastKind, err = d.atom()
		case "opcode":
			n.Opcode, err = d.atom()
		case "isPostfix":
			n.IsPostfix, err = d.readBool()
		case "isArrow":
			n.IsArrow, err = d.readBool()
		case "value":
			n.Value, err = d.scalarText()
		case "referencedDecl":
			n.ReferencedDecl, err = d.declRef()
		case "referencedMemberDecl":
			n.ReferencedMemberDecl, err = d.str()
		case "decl":
			n.Decl, err = d.declRef()
		case "ownedTagDecl":
			n.OwnedTagDecl, err = d.declRef()
		case "cleanup_function":
			n.CleanupFunction, err = d.declRef()
		case "hasElse":
			n.HasElse, err = d.readBool()
		case "targetLabelDeclId":
			n.TargetLabelDeclID, err = d.str()
		case "inner", "array_filler":
			// Children follow every attribute, so the keep decision is
			// made here, once.
			if !decided {
				decided = true
				switch {
				case mode == modeUnit && depth == 0:
					keep = d.keepTop(n)
				case mode == modeFilter && depth == 0:
					keep = n.Kind == "FunctionDecl" && n.Name == "main"
				}
			}
			err = d.children(n, string(d.key) == "array_filler", keep, mode, depth)
		default:
			err = d.skim()
		}
		if err != nil {
			return err
		}
		more, err := d.more('}')
		if err != nil {
			return err
		}
		if !more {
			return nil
		}
	}
}

// children reads a child array. clang labels an InitListExpr's first child
// array_filler and writes the initializers after it in the same array.
func (d *decoder) children(n *Node, filler, keep bool, mode dumpMode, depth int) error {
	empty, err := d.open('[', ']')
	if err != nil || empty {
		return err
	}
	for first := true; ; first = false {
		if keep {
			child := &Node{}
			if err := d.node(child, mode, depth+1); err != nil {
				return err
			}
			if filler && first {
				n.ArrayFiller = child
			} else {
				n.Inner = append(n.Inner, child)
			}
		} else if err := d.skim(); err != nil {
			return err
		}
		more, err := d.more(']')
		if err != nil || !more {
			return err
		}
	}
}

var externalKinds = map[string]bool{"FunctionDecl": true, "VarDecl": true, "RecordDecl": true, "TypedefDecl": true, "EnumDecl": true}

// root reads the TranslationUnitDecl and its top-level declarations.
func (d *decoder) root() error {
	empty, err := d.open('{', '}')
	if err != nil || empty {
		return err
	}
	for {
		if err := d.readKey(); err != nil {
			return err
		}
		if string(d.key) == "inner" {
			err = d.topLevel()
		} else {
			err = d.skim()
		}
		if err != nil {
			return err
		}
		more, err := d.more('}')
		if err != nil || !more {
			return err
		}
	}
}

func (d *decoder) topLevel() error {
	empty, err := d.open('[', ']')
	if err != nil || empty {
		return err
	}
	for {
		node := &Node{}
		if err := d.node(node, modeUnit, 0); err != nil {
			return err
		}
		switch {
		case d.keepTop(node):
			d.decls = append(d.decls, node)
		case externalKinds[node.Kind] && node.Loc.Expansion.Valid():
			d.external = append(d.external, ExternalDecl{ID: node.ID, Kind: node.Kind, Name: node.Name, TagUsed: node.TagUsed, Type: node.Type.QualType, Position: node.Loc.Expansion})
		}
		more, err := d.more(']')
		if err != nil || !more {
			return err
		}
	}
}
