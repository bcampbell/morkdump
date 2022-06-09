package main

import (
	"fmt"
)

type Row map[string]string

type Table struct {
	meta map[string]string `json:"meta,omitempty"`
	rows map[string]Row    `json:"rows,omitempty"`
}

type parser struct {
	filename string
	lex      *lexer
	peeked   *item // if we've peeked a token, keep it here
	// err holds the last error. To save error checking, the expect*() fns are
	// all basically no-ops if this is set. So we don't need to check errors
	// after every step.
	err error

	// a map of dicts, keyed by namespace
	dicts map[string]map[string]string
}

func NewParser(filename string, lex *lexer) *parser {
	return &parser{filename: filename, lex: lex,
		dicts: make(map[string]map[string]string),
	}
}

// dict returns the dictionary for the given namespace, creating it if required.
func (p *parser) dict(namespace string) map[string]string {
	var dict map[string]string
	var ok bool
	if dict, ok = p.dicts[namespace]; !ok {
		dict = make(map[string]string)
		p.dicts[namespace] = dict
	}
	return dict
}

// resolve returns the string currently stored for the id:namespace pair.
// If not found, returns "" and sets p.err
func (p *parser) resolve(id, namespace string) string {
	if p.err != nil {
		return ""
	}
	dict := p.dict(namespace)
	full, ok := dict[id]
	if !ok {
		p.err = fmt.Errorf("%s: bad alias %s:%s", p.filename, id, namespace)
		return ""
	}
	return full
}

// peekTok returns the next token in the input, without consuming it.
func (p *parser) peekTok() item {
	if p.peeked == nil {
		tok := p.lex.nextItem()
		p.peeked = &tok
	}
	return *p.peeked
}

// nextTok reads and consumes the next token in the input.
func (p *parser) nextTok() item {
	var tok item
	if p.peeked != nil {
		tok = *p.peeked
		p.peeked = nil
	} else {
		tok = p.lex.nextItem()
	}
	//fmt.Printf("nextTok() => %q\n", tok)
	return tok
}

func (p *parser) Parse() (map[string]Table, error) {
	tabs := make(map[string]Table)
	for {
		tok := p.peekTok()
		switch tok.typ {
		case tEOF:
			return tabs, p.err // Done!
		case tLANGLE:
			p.expectDict()
		case tLSQUARE:
			// TODO: can this happen?
			_, _ = p.expectRow("c")
			//rowID, rowData := p.expectRow("c")
			//fmt.Printf("row: %q %q\n", rowID, rowData)
		case tLBRACE:
			toid, tab := p.expectTable()
			tabs[toid] = tab
		case tGROUPSTART:
			_ = p.expectGroup()
		default:
			p.err = fmt.Errorf("%s: Unexpected %s", p.filename, tok)
		}

		if p.err != nil {
			return tabs, p.err
		}
	}
}

// expect returns the next item if it matches expected (and if p.err is unset).
// Otherwise an empty item will be returned, and p.err will be set.
func (p *parser) expect(expected itemType) item {
	// do nothing if we're already in error state
	if p.err != nil {
		return item{}
	}
	tok := p.nextTok()
	if tok.typ != expected {
		p.err = fmt.Errorf("%s: Unexpected - %s", p.filename, &tok)
		panic("poop")
		return item{}
	}
	return tok
}

//    dict ::= < (metadict | alias)* >
//    metadict ::= < (cell)* >
//    alias ::= ( id value )
//    value ::= ^oid | =literal
//    oid ::= id | id:scope
func (p *parser) expectDict() {
	p.expect(tLANGLE)
	scope := "a"

	// metadict is optional
	tok := p.peekTok()
	if tok.typ == tLANGLE {
		meta := p.expectMetadict()
		// only one entry we want to check for...
		if a, ok := meta["a"]; ok {
			scope = a
		}
	}

	cells := p.expectCells()
	p.expect(tRANGLE)

	if p.err == nil {
		//fmt.Printf("Update dict '%s': %q\n", scope, cells)
		targ := p.dict(scope)
		// update the values in the appropriate dict
		for k, v := range cells {
			targ[k] = v
		}
	}
}

func (p *parser) expectMetadict() map[string]string {
	p.expect(tLANGLE)
	cells := p.expectCells()
	p.expect(tRANGLE)
	return cells
}

func (p *parser) expectName() string {
	tok := p.expect(tNAME)
	return tok.val
}

//  cell ::= ( col slot )
//  col ::= ref /*default scope is c*/ | name
//  slot ::= ref /*default scope is a*/ | =literal
//  ref ::= ^id | ^id:scope
func (p *parser) expectCell() (string, string) {
	p.expect(tLPAREN)

	var name, value string
	// column name
	tok := p.peekTok()
	if tok.typ == tCARET {
		name = p.expectRef("c")
	} else {
		name = p.expectName()
	}

	// value
	tok = p.peekTok()
	if tok.typ == tEQUAL {
		p.nextTok()
		value = p.expect(tLITERAL).val
		// TODO: ESCAPING!!!
	} else {
		value = p.expectRef("a")
	}
	p.expect(tRPAREN)
	return name, value
}

// expectCells parses 0 or more cells
func (p *parser) expectCells() map[string]string {
	out := map[string]string{}
	for {
		tok := p.peekTok()
		if tok.typ != tLPAREN {
			break
		}
		name, val := p.expectCell()
		if p.err != nil {
			return nil
		}
		out[name] = val
	}
	return out
}

//  row ::= [ roid (metarow | cell)* ]
//  roid ::= oid /*default scope is rowScope from table*/
//  metarow ::= [ (cell)* ]

// expectRow parses a row, returning the row ID (roid) and the cells.
func (p *parser) expectRow(rowScope string) (string, map[string]string) {
	p.expect(tLSQUARE)
	roid, _ := p.expectOID(rowScope)
	// TODO: handle optional metarow???
	cells := p.expectCells()
	p.expect(tRSQUARE)
	return roid, cells
}

// expectID fetches the next token as a hex ID
func (p *parser) expectID() string {
	tok := p.expect(tNAME)
	if p.err != nil {
		return ""
	}
	// Fail if it's not all hex.
	for _, r := range tok.val {
		if !isHex(r) {
			p.err = fmt.Errorf("%s: Not hex - %s", p.filename, &tok)
			return ""
		}
	}
	return tok.val
}

// ^id:ns
func (p *parser) expectRef(defaultNamespace string) string {
	p.expect(tCARET)
	id := p.expectID()
	tok := p.peekTok()
	ns := defaultNamespace
	if tok.typ == tCOLON {
		p.nextTok()
		tok = p.expect(tNAME)
		ns = tok.val
	}
	return p.resolve(id, ns)
}

// Oids/Mids
// A Morkoid is an objectid , and includes both the hexid and the namespace
// scope. When not explicitly stated, the scope is implicitly understood as
// some default for each context.
//    oid ::= id | id:scope
//    scope ::= name | ^id | ^oid

func (p *parser) expectOID(defaultNamespace string) (string, string) {
	name := p.expectID()
	scope := ""
	// is there a scope?
	colon := p.peekTok()
	if colon.typ == tCOLON {
		p.nextTok()
		tok := p.peekTok()
		if tok.typ == tCARET {
			scope = p.expectRef(defaultNamespace)
		} else {
			// name
			scope = p.expect(tNAME).val
		}
	}
	return name, scope
}

// expectTable returns a tableID (toid) and the table.
//  table ::= { toid (metatable | row | roid )* }
//  toid ::= oid /*default scope is c*/
//  metatable ::= { (cell)* }
func (p *parser) expectTable() (string, Table) {
	tab := Table{
		meta: make(map[string]string),
		rows: make(map[string]Row),
	}
	p.expect(tLBRACE)
	toid, tscope := p.expectOID("c")

	// For now, coallese the id and scope
	toid = fmt.Sprintf("%s:%s", toid, tscope)

	// check for metatable
	tok := p.peekTok()
	if tok.typ == tLBRACE {
		p.nextTok()
		// there's a metatable
		tab.meta = p.expectCells()
		// TODO: handle metarow... ugh
		// for now, just skip past to end of metatable
		for {
			if p.err != nil {
				return toid, Table{}
			}
			tok = p.nextTok()
			if tok.typ == tRBRACE {
				break
			}
		}
	}
	// rows
	for {
		tok := p.peekTok()
		switch tok.typ {
		case tRBRACE: // end of table
			p.nextTok()
			return toid, tab
		case tLSQUARE:
			// row data
			rowID, rowData := p.expectRow(tscope)
			tab.rows[rowID] = rowData
		case tNAME: // numeric
			p.nextTok()
			//TODO!!!
			fmt.Printf("IGNORING row id\n")
		}
	}
}

// Expect a group.
// TODO: rethink this. we want to return a list of changes...
func (p *parser) expectGroup() map[string]Table {
	// TODO: need to avoid directly changing parser-global dicts here
	// - a group abort won't roll back those changes!
	p.expect(tGROUPSTART)
	tabs := make(map[string]Table)
	for {
		tok := p.peekTok()
		switch tok.typ {
		case tEOF:
			// treat as group abort.
			return map[string]Table{}
		case tLANGLE:
			p.expectDict()
		case tLSQUARE:
			// TODO: can this happen?
			_, _ = p.expectRow("c")
			//rowID, rowData := p.expectRow("c")
			//fmt.Printf("row: %q %q\n", rowID, rowData)
		case tLBRACE:
			toid, tab := p.expectTable()
			tabs[toid] = tab
		case tGROUPCOMMIT:
			p.expect(tGROUPCOMMIT)
			// TODO: check group ID matches tGROUPSTART one
			return tabs
		case tGROUPABORT:
			p.expect(tGROUPABORT)
			return map[string]Table{}
		default:
			p.err = fmt.Errorf("%s: Unexpected %s", p.filename, tok)
		}

		if p.err != nil {
			return tabs
		}
	}
}
