package main

import (
	//	"io"
	"fmt"
	"strings"
	//"unicode"
	//	"unicode/utf8"
)

// lexer technique based on:
// https://talks.golang.org/2011/lex.slide

type itemType int

const (
	tEOF itemType = iota
	tERROR
	tLITERAL
	tNAME // hex or name
	tCARET
	tPLUS
	tCOLON
	tEQUAL
	tLANGLE
	tRANGLE
	tLPAREN
	tRPAREN
	tLSQUARE
	tRSQUARE
	tLBRACE
	tRBRACE
	tGROUPSTART
	tGROUPCOMMIT
	tGROUPABORT
)

func (t itemType) String() string {

	itemTypes := map[itemType]string{
		tEOF:         "tEOF",
		tERROR:       "tERROR",
		tLITERAL:     "tLITERAL",
		tNAME:        "tNAME",
		tCARET:       "tCARET",
		tPLUS:        "tPLUS",
		tCOLON:       "tCOLON",
		tEQUAL:       "tEQUAL",
		tLANGLE:      "tLANGLE",
		tRANGLE:      "tRANGLE",
		tLPAREN:      "tLPAREN",
		tRPAREN:      "tRPAREN",
		tLSQUARE:     "tLSQUARE",
		tRSQUARE:     "tRSQUARE",
		tLBRACE:      "tLBRACE",
		tRBRACE:      "tRBRACE",
		tGROUPSTART:  "tGROUPSTART",
		tGROUPCOMMIT: "tGROUPCOMMIT",
		tGROUPABORT:  "tGROUPABORT",
	}
	return itemTypes[t]
}

// some single-rune items
var singles = map[rune]itemType{
	'(': tLPAREN,
	')': tRPAREN,
	'[': tLSQUARE,
	']': tRSQUARE,
	'{': tLBRACE,
	'}': tRBRACE,
	'<': tLANGLE,
	'>': tRANGLE,
	':': tCOLON,
	'+': tPLUS,
}

type item struct {
	typ itemType
	val string
	pos position
}

func (it item) String() string {
	return fmt.Sprintf("%d:%d %s %q", it.pos.line, it.pos.col, it.typ, it.val)
}

type stateFn func(*lexer) stateFn

type position struct {
	offset int
	line   int
	col    int
}

const (
	EOF = -1
)

type lexer struct {
	input []byte
	curr  position // current pos
	start position // start of current item
	state stateFn
	items chan item
}

func newLexer(input []byte) *lexer {
	l := &lexer{
		input: input,
		curr:  position{offset: 0, line: 1, col: 0},
		start: position{offset: 0, line: 1, col: 0},
		state: lexDefault,
		items: make(chan item, 2),
	}
	return l
}

// nextItem returns the next item from the input.
func (l *lexer) nextItem() item {
	for {
		select {
		case item := <-l.items:
			return item
		default:
			l.state = l.state(l)
		}
	}
}

// Return the next rune.
func (l *lexer) next() rune {
	if l.curr.offset >= len(l.input) {
		return EOF
	}

	// Straight ASCII.
	r := rune(l.input[l.curr.offset])
	// Use LF to track line numbers.
	if r == '\n' {
		l.curr.line++
		l.curr.col = 0
	} else {
		l.curr.col++
	}
	l.curr.offset++
	return r
}

// Return the next rune without consuming it.
func (l *lexer) peek() rune {
	if l.curr.offset >= len(l.input) {
		return EOF
	}
	// Straight ASCII.
	return rune(l.input[l.curr.offset])
}

// emit outputs an item
func (l *lexer) emit(t itemType) {
	l.items <- item{
		typ: t,
		val: string(l.input[l.start.offset:l.curr.offset]),
		pos: l.start,
	}
	l.start = l.curr
}

// emitErrorf emits an error
func (l *lexer) emitErrorf(format string, args ...interface{}) {
	l.items <- item{
		typ: tERROR,
		val: fmt.Sprintf(format, args...),
		pos: l.start,
	}
}

func (l *lexer) syntaxError() stateFn {
	l.emitErrorf("Syntax error")
	return nil
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\n' || r == '\r' || r == '\t'
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

func isAlpha(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isHex(r rune) bool {
	return (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F' || (r >= '0' && r <= '9'))
}

// lexDefault is the default scanning state
func lexDefault(l *lexer) stateFn {
	for {
		r := l.peek()
		if r == EOF {
			l.emit(tEOF)
			return nil
		}

		// skip whitespace
		if isSpace(r) {
			l.next()
			l.start = l.curr // don't capture whitespace
			continue
		}

		// single-rune token?
		if typ, got := singles[r]; got {
			l.next()
			l.emit(typ)
			return lexDefault
		}

		// reference? ^ID
		if r == '^' {
			l.next()
			l.emit(tCARET)
			id := l.gatherID()
			if len(id) == 0 {
				return l.syntaxError()
			}
			l.emit(tNAME)
			return lexDefault
		}
		if r == '/' {
			return lexComment
		}

		if r == '=' {
			return lexLiteral
		}

		if isAlpha(r) || isDigit(r) || r == '_' {
			return lexName
		}

		//
		if r == '@' {
			return lexGroup
		}

		return l.syntaxError()
	}
}

// lexLiteral scans from '=' to end of literal.
func lexLiteral(l *lexer) stateFn {
	r := l.next()
	if r != '=' {
		l.emitErrorf("Missing '='")
		return nil
	}

	// spit out '=' as a separate token
	l.emit(tEQUAL)

	// don't do escaping here, but need to track the escape char to know when literal ends.
	esc := false
	for {
		r := l.peek()
		if r == EOF {
			break
		}
		if !esc && r == ')' {
			break // done!
		} else if !esc && r == '\\' {
			esc = true
		} else {
			esc = false
		}
		l.next()
	}

	l.emit(tLITERAL)
	return lexDefault
}

func lexComment(l *lexer) stateFn {
	r := l.next()
	if r != '/' {
		l.emitErrorf("expected '/'")
		return nil
	}
	r = l.next()
	if r != '/' {
		l.emitErrorf("expected '/'")
		return nil
	}
	// Scan to end of line.
	for {
		r = l.peek()
		//Should handle LF, CR, CRLF, LFCR... but hey.
		if r == EOF || r == '\n' {
			break
		}
		l.next()
	}
	// Discard.
	l.start = l.curr
	return lexDefault
}

// name or hex id
func lexName(l *lexer) stateFn {
	nonLeading := "-!?+" // ':'?
	first := true
	for {
		r := l.peek()
		if !(isAlpha(r) || isDigit(r) || r == '_' || (!first && strings.ContainsRune(nonLeading, r))) {
			break
		}
		l.next()
		first = false
	}
	l.emit(tNAME)
	return lexDefault
}

func (l *lexer) gatherID() string {
	var id string
	for {
		r := l.peek()
		if !isHex(r) {
			break
		}
		id = id + string(r)
		l.next()
	}
	return id
}

func (l *lexer) expect(s string) bool {
	for _, expected := range s {
		r := l.next()
		if r != expected {
			l.emitErrorf("Expected \"%s\"", s)
			return false
		}
	}
	return true
}

func lexGroup(l *lexer) stateFn {
	if !l.expect("@$$") {
		return nil
	}
	switch l.next() {
	case '{':
		// start group "@$${ID{@"
		id := l.gatherID()
		if len(id) == 0 {
			l.emitErrorf("Bad ID")
			return nil
		}
		if !l.expect("{@") {
			return nil
		}
		l.emit(tGROUPSTART)
		return lexDefault
	case '}':
		// end of group.
		r := l.peek()
		if r == '~' {
			// Abort. Documented as:
			//   @$$}~abort~ID}@
			// but coded as:
			//   @$$}~~}@
			if !l.expect("~~}@") {
				return nil
			}
			l.emit(tGROUPABORT)
			return lexDefault
		}
		// commit group
		//  	@$$}ID}@
		id := l.gatherID()
		if len(id) == 0 {
			l.emitErrorf("Bad ID")
			return nil
		}
		if !l.expect("}@") {
			return nil
		}
		l.emit(tGROUPCOMMIT)
		return lexDefault
	}

	return l.syntaxError()
}
