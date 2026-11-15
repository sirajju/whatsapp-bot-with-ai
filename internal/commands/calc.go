package commands

// Tiny recursive-descent arithmetic evaluator:
//   expr    := term (('+' | '-') term)*
//   term    := factor (('*' | '/' | '%') factor)*
//   factor  := unary ('^' factor)?
//   unary   := ('+' | '-') unary | primary
//   primary := number | '(' expr ')'
//
// No external deps.

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

type calcParser struct {
	src string
	pos int
}

func evalExpr(s string) (float64, error) {
	p := &calcParser{src: strings.TrimSpace(s)}
	v, err := p.expr()
	if err != nil {
		return 0, err
	}
	p.skipWS()
	if p.pos != len(p.src) {
		return 0, fmt.Errorf("unexpected %q at %d", p.src[p.pos:], p.pos)
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("non-finite result")
	}
	return v, nil
}

func (p *calcParser) skipWS() {
	for p.pos < len(p.src) && unicode.IsSpace(rune(p.src[p.pos])) {
		p.pos++
	}
}

func (p *calcParser) peek() byte {
	p.skipWS()
	if p.pos >= len(p.src) {
		return 0
	}
	return p.src[p.pos]
}

func (p *calcParser) accept(c byte) bool {
	if p.peek() == c {
		p.pos++
		return true
	}
	return false
}

func (p *calcParser) expr() (float64, error) {
	v, err := p.term()
	if err != nil {
		return 0, err
	}
	for {
		switch p.peek() {
		case '+':
			p.pos++
			rhs, err := p.term()
			if err != nil {
				return 0, err
			}
			v += rhs
		case '-':
			p.pos++
			rhs, err := p.term()
			if err != nil {
				return 0, err
			}
			v -= rhs
		default:
			return v, nil
		}
	}
}

func (p *calcParser) term() (float64, error) {
	v, err := p.factor()
	if err != nil {
		return 0, err
	}
	for {
		switch p.peek() {
		case '*':
			p.pos++
			rhs, err := p.factor()
			if err != nil {
				return 0, err
			}
			v *= rhs
		case '/':
			p.pos++
			rhs, err := p.factor()
			if err != nil {
				return 0, err
			}
			if rhs == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			v /= rhs
		case '%':
			p.pos++
			rhs, err := p.factor()
			if err != nil {
				return 0, err
			}
			if rhs == 0 {
				return 0, fmt.Errorf("modulo by zero")
			}
			v = math.Mod(v, rhs)
		default:
			return v, nil
		}
	}
}

func (p *calcParser) factor() (float64, error) {
	v, err := p.unary()
	if err != nil {
		return 0, err
	}
	if p.accept('^') {
		rhs, err := p.factor()
		if err != nil {
			return 0, err
		}
		v = math.Pow(v, rhs)
	}
	return v, nil
}

func (p *calcParser) unary() (float64, error) {
	if p.accept('+') {
		return p.unary()
	}
	if p.accept('-') {
		v, err := p.unary()
		return -v, err
	}
	return p.primary()
}

func (p *calcParser) primary() (float64, error) {
	if p.accept('(') {
		v, err := p.expr()
		if err != nil {
			return 0, err
		}
		if !p.accept(')') {
			return 0, fmt.Errorf("missing )")
		}
		return v, nil
	}
	p.skipWS()
	start := p.pos
	for p.pos < len(p.src) && (p.src[p.pos] == '.' ||
		(p.src[p.pos] >= '0' && p.src[p.pos] <= '9')) {
		p.pos++
	}
	if start == p.pos {
		return 0, fmt.Errorf("number expected at %d", start)
	}
	return strconv.ParseFloat(p.src[start:p.pos], 64)
}
