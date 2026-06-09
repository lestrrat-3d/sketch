package param

import (
	"fmt"
	"strconv"
)

// ParseError describes a syntax error in an expression, with the byte offset at
// which it occurred.
type ParseError struct {
	Input string // the expression being parsed
	Pos   int    // byte offset of the error
	Msg   string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("param: syntax error at position %d: %s (in %q)", e.Pos, e.Msg, e.Input)
}

type tokKind int

const (
	tEOF tokKind = iota
	tNumber
	tIdent
	tPlus
	tMinus
	tStar
	tSlash
	tPercent
	tCaret
	tLParen
	tRParen
	tComma
)

type token struct {
	kind tokKind
	text string
	val  float64 // valid when kind == tNumber
	pos  int
}

// lex tokenizes input into a slice of tokens terminated by a tEOF token.
func lex(input string) ([]token, error) {
	var toks []token
	i := 0
	n := len(input)
	for i < n {
		c := input[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
			continue
		case c == '+':
			toks = append(toks, token{kind: tPlus, text: "+", pos: i})
			i++
		case c == '-':
			toks = append(toks, token{kind: tMinus, text: "-", pos: i})
			i++
		case c == '*':
			toks = append(toks, token{kind: tStar, text: "*", pos: i})
			i++
		case c == '/':
			toks = append(toks, token{kind: tSlash, text: "/", pos: i})
			i++
		case c == '%':
			toks = append(toks, token{kind: tPercent, text: "%", pos: i})
			i++
		case c == '^':
			toks = append(toks, token{kind: tCaret, text: "^", pos: i})
			i++
		case c == '(':
			toks = append(toks, token{kind: tLParen, text: "(", pos: i})
			i++
		case c == ')':
			toks = append(toks, token{kind: tRParen, text: ")", pos: i})
			i++
		case c == ',':
			toks = append(toks, token{kind: tComma, text: ",", pos: i})
			i++
		case isDigit(c) || (c == '.' && i+1 < n && isDigit(input[i+1])):
			start := i
			i = scanNumber(input, i)
			text := input[start:i]
			v, err := strconv.ParseFloat(text, 64)
			if err != nil {
				return nil, &ParseError{Input: input, Pos: start, Msg: "invalid number " + strconv.Quote(text)}
			}
			toks = append(toks, token{kind: tNumber, text: text, val: v, pos: start})
		case isIdentStart(c):
			start := i
			i++
			for i < n && isIdentPart(input[i]) {
				i++
			}
			toks = append(toks, token{kind: tIdent, text: input[start:i], pos: start})
		default:
			return nil, &ParseError{Input: input, Pos: i, Msg: "unexpected character " + strconv.QuoteRune(rune(c))}
		}
	}
	toks = append(toks, token{kind: tEOF, pos: n})
	return toks, nil
}

// scanNumber returns the index just past a numeric literal starting at i,
// accepting an optional fractional part and a signed decimal exponent.
func scanNumber(s string, i int) int {
	n := len(s)
	for i < n && isDigit(s[i]) {
		i++
	}
	if i < n && s[i] == '.' {
		i++
		for i < n && isDigit(s[i]) {
			i++
		}
	}
	if i < n && (s[i] == 'e' || s[i] == 'E') {
		j := i + 1
		if j < n && (s[j] == '+' || s[j] == '-') {
			j++
		}
		if j < n && isDigit(s[j]) {
			i = j
			for i < n && isDigit(s[i]) {
				i++
			}
		}
	}
	return i
}

func isDigit(c byte) bool      { return c >= '0' && c <= '9' }
func isIdentStart(c byte) bool { return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
func isIdentPart(c byte) bool  { return isIdentStart(c) || isDigit(c) }

// isIdent reports whether s is a syntactically valid parameter name.
func isIdent(s string) bool {
	if s == "" || !isIdentStart(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !isIdentPart(s[i]) {
			return false
		}
	}
	return true
}
