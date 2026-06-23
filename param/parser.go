package param

import "fmt"

// Parse compiles an expression string into an [Expr] tree. Identifiers are not
// resolved at parse time, so expressions may reference parameters, constants
// and functions that are defined later.
//
// Grammar (lowest to highest precedence):
//
//	expr    := term  (("+" | "-") term)*
//	term    := unary (("*" | "/" | "%") unary)*
//	unary   := ("+" | "-") unary | power
//	power   := primary ("^" unary)?            // right associative
//	primary := NUMBER | IDENT | IDENT "(" args ")" | "(" expr ")"
//	args    := ε | expr ("," expr)*
func Parse(input string) (Expr, error) {
	toks, err := lex(input)
	if err != nil {
		return nil, err
	}
	p := &parser{input: input, toks: toks}
	e, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tEOF {
		return nil, p.errf(p.peek().pos, "unexpected %q", p.peek().text)
	}
	return e, nil
}

type parser struct {
	input string
	toks  []token
	pos   int
}

// opRune returns the operator's first byte as a rune, used to tag expression
// nodes with their operator.
func (t token) opRune() rune { return rune(t.text[0]) }

func (p *parser) peek() token { return p.toks[p.pos] }
func (p *parser) next() token { t := p.toks[p.pos]; p.pos++; return t }

func (p *parser) errf(pos int, format string, args ...any) error {
	return &ParseError{Input: p.input, Pos: pos, Msg: fmt.Sprintf(format, args...)}
}

func (p *parser) parseExpr() (Expr, error) {
	left, err := p.parseTerm()
	if err != nil {
		return nil, err
	}
	for {
		switch p.peek().kind {
		case tPlus, tMinus:
			op := p.next()
			right, err := p.parseTerm()
			if err != nil {
				return nil, err
			}
			left = &binaryExpr{op: op.opRune(), x: left, y: right}
		default:
			return left, nil
		}
	}
}

func (p *parser) parseTerm() (Expr, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		switch p.peek().kind {
		case tStar, tSlash, tPercent:
			op := p.next()
			right, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			left = &binaryExpr{op: op.opRune(), x: left, y: right}
		default:
			return left, nil
		}
	}
}

func (p *parser) parseUnary() (Expr, error) {
	if k := p.peek().kind; k == tPlus || k == tMinus {
		op := p.next()
		x, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &unaryExpr{op: op.opRune(), x: x}, nil
	}
	return p.parsePower()
}

func (p *parser) parsePower() (Expr, error) {
	base, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	if p.peek().kind == tCaret {
		p.next()
		// Exponent is a unary so that 2^-3 and 2^3^2 (right associative) work.
		exp, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &binaryExpr{op: '^', x: base, y: exp}, nil
	}
	return base, nil
}

func (p *parser) parsePrimary() (Expr, error) {
	t := p.peek()
	switch t.kind {
	case tNumber:
		p.next()
		return &numberExpr{v: t.val}, nil
	case tIdent:
		p.next()
		if p.peek().kind == tLParen {
			return p.parseCall(t.text)
		}
		return &identExpr{name: t.text}, nil
	case tLParen:
		p.next()
		e, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if p.peek().kind != tRParen {
			return nil, p.errf(p.peek().pos, "expected ')'")
		}
		p.next()
		return e, nil
	case tEOF:
		return nil, p.errf(t.pos, "unexpected end of expression")
	default:
		return nil, p.errf(t.pos, "unexpected %q", t.text)
	}
}

func (p *parser) parseCall(name string) (Expr, error) {
	p.next() // consume '('
	var args []Expr
	if p.peek().kind != tRParen {
		for {
			a, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			args = append(args, a)
			if p.peek().kind == tComma {
				p.next()
				continue
			}
			break
		}
	}
	if p.peek().kind != tRParen {
		return nil, p.errf(p.peek().pos, "expected ')' to close call to %q", name)
	}
	p.next()
	return &callExpr{name: name, args: args}, nil
}
