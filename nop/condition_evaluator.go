// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package nop

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// ConditionError is returned when a condition expression cannot be parsed or evaluated.
type ConditionError struct {
	Message    string
	Expression string
}

func (e *ConditionError) Error() string {
	return fmt.Sprintf("%s  Expression: «%s»", e.Message, e.Expression)
}

// EvaluateCondition evaluates a CEL-subset condition against completed node results
// (NPS-5 §3.1.5). Returns true when the node should execute, false to skip.
//
// Supported: comparisons (==, !=, <, >, <=, >=), boolean logic (&&, ||, !),
// grouping (), literals (numbers, "strings", true, false, null) and $.node.field
// JSONPath references.
func EvaluateCondition(condition string, context map[string]json.RawMessage) (result bool, err error) {
	if strings.TrimSpace(condition) == "" {
		return true, nil
	}

	defer func() {
		if r := recover(); r != nil {
			if ce, ok := r.(*ConditionError); ok {
				err = ce
				return
			}
			err = &ConditionError{Message: fmt.Sprintf("Condition evaluation error: %v", r), Expression: condition}
		}
	}()

	tokens := tokenizeCondition(strings.TrimSpace(condition))
	p := &conditionParser{tokens: tokens, context: context}
	return p.parseOrExpr(), nil
}

// ── Tokenizer ─────────────────────────────────────────────────────────────────

type tokenKind int

const (
	tkDollarPath tokenKind = iota
	tkNumber
	tkString
	tkTrue
	tkFalse
	tkNull
	tkGt
	tkGte
	tkLt
	tkLte
	tkEq
	tkNeq
	tkAnd
	tkOr
	tkNot
	tkLParen
	tkRParen
	tkEof
)

type condToken struct {
	kind tokenKind
	raw  string
}

func tokenizeCondition(input string) []condToken {
	var tokens []condToken
	i := 0
	n := len(input)

	for i < n {
		c := input[i]
		if unicode.IsSpace(rune(c)) {
			i++
			continue
		}

		// Dollar path
		if c == '$' && i+1 < n && input[i+1] == '.' {
			start := i
			for i < n && (isAlnum(input[i]) || input[i] == '_' || input[i] == '.' || input[i] == '$') {
				i++
			}
			tokens = append(tokens, condToken{tkDollarPath, input[start:i]})
			continue
		}

		// String literal
		if c == '"' {
			start := i
			i++
			for i < n && input[i] != '"' {
				i++
			}
			i++ // closing quote
			tokens = append(tokens, condToken{tkString, input[start+1 : i-1]})
			continue
		}

		// Number
		if isDigit(c) || (c == '-' && i+1 < n && isDigit(input[i+1])) {
			start := i
			if c == '-' {
				i++
			}
			for i < n && (isDigit(input[i]) || input[i] == '.') {
				i++
			}
			tokens = append(tokens, condToken{tkNumber, input[start:i]})
			continue
		}

		// Operators
		if c == '>' && i+1 < n && input[i+1] == '=' {
			tokens = append(tokens, condToken{tkGte, ">="})
			i += 2
			continue
		}
		if c == '<' && i+1 < n && input[i+1] == '=' {
			tokens = append(tokens, condToken{tkLte, "<="})
			i += 2
			continue
		}
		if c == '=' && i+1 < n && input[i+1] == '=' {
			tokens = append(tokens, condToken{tkEq, "=="})
			i += 2
			continue
		}
		if c == '!' && i+1 < n && input[i+1] == '=' {
			tokens = append(tokens, condToken{tkNeq, "!="})
			i += 2
			continue
		}
		if c == '&' && i+1 < n && input[i+1] == '&' {
			tokens = append(tokens, condToken{tkAnd, "&&"})
			i += 2
			continue
		}
		if c == '|' && i+1 < n && input[i+1] == '|' {
			tokens = append(tokens, condToken{tkOr, "||"})
			i += 2
			continue
		}
		if c == '>' {
			tokens = append(tokens, condToken{tkGt, ">"})
			i++
			continue
		}
		if c == '<' {
			tokens = append(tokens, condToken{tkLt, "<"})
			i++
			continue
		}
		if c == '!' {
			tokens = append(tokens, condToken{tkNot, "!"})
			i++
			continue
		}
		if c == '(' {
			tokens = append(tokens, condToken{tkLParen, "("})
			i++
			continue
		}
		if c == ')' {
			tokens = append(tokens, condToken{tkRParen, ")"})
			i++
			continue
		}

		// Keywords
		if unicode.IsLetter(rune(c)) {
			start := i
			for i < n && isAlnum(input[i]) {
				i++
			}
			kw := input[start:i]
			switch kw {
			case "true":
				tokens = append(tokens, condToken{tkTrue, "true"})
			case "false":
				tokens = append(tokens, condToken{tkFalse, "false"})
			case "null":
				tokens = append(tokens, condToken{tkNull, "null"})
			default:
				panic(&ConditionError{Message: fmt.Sprintf("Unknown token '%s'.", kw), Expression: input})
			}
			continue
		}

		panic(&ConditionError{Message: fmt.Sprintf("Unexpected character '%c' at position %d.", c, i), Expression: input})
	}

	tokens = append(tokens, condToken{tkEof, ""})
	return tokens
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
func isAlnum(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// ── Recursive-descent parser ──────────────────────────────────────────────────

type condValue struct {
	kind tokenKind
	// value is one of: float64, string, bool, or nil.
	value any
}

type conditionParser struct {
	tokens  []condToken
	pos     int
	context map[string]json.RawMessage
}

func (p *conditionParser) current() condToken { return p.tokens[p.pos] }
func (p *conditionParser) consume() condToken {
	t := p.tokens[p.pos]
	p.pos++
	return t
}

// or_expr := and_expr ('||' and_expr)*
func (p *conditionParser) parseOrExpr() bool {
	left := p.parseAndExpr()
	for p.current().kind == tkOr {
		p.consume()
		right := p.parseAndExpr()
		left = left || right
	}
	return left
}

// and_expr := not_expr ('&&' not_expr)*
func (p *conditionParser) parseAndExpr() bool {
	left := p.parseNotExpr()
	for p.current().kind == tkAnd {
		p.consume()
		right := p.parseNotExpr()
		left = left && right
	}
	return left
}

// not_expr := '!' not_expr | comparison
func (p *conditionParser) parseNotExpr() bool {
	if p.current().kind == tkNot {
		p.consume()
		return !p.parseNotExpr()
	}
	return p.parseComparison()
}

// comparison := '(' or_expr ')' | true | false | value (op value)?
func (p *conditionParser) parseComparison() bool {
	if p.current().kind == tkLParen {
		p.consume()
		inner := p.parseOrExpr()
		if p.current().kind != tkRParen {
			panic(&ConditionError{Message: "Expected ')'.", Expression: ""})
		}
		p.consume()
		return inner
	}

	if p.current().kind == tkTrue {
		p.consume()
		return true
	}
	if p.current().kind == tkFalse {
		p.consume()
		return false
	}

	lhs := p.parseValue()
	opKind := p.current().kind
	if !isComparisonOp(opKind) {
		return asTruthy(lhs)
	}
	p.consume()
	rhs := p.parseValue()
	return compareValues(lhs, opKind, rhs)
}

func isComparisonOp(k tokenKind) bool {
	switch k {
	case tkGt, tkGte, tkLt, tkLte, tkEq, tkNeq:
		return true
	}
	return false
}

// value := dollar_path | number | string | null | true | false
func (p *conditionParser) parseValue() condValue {
	tok := p.consume()
	switch tok.kind {
	case tkDollarPath:
		return condValue{tkDollarPath, p.resolvePath(tok.raw)}
	case tkNumber:
		f, err := strconv.ParseFloat(tok.raw, 64)
		if err != nil {
			panic(&ConditionError{Message: fmt.Sprintf("Invalid number '%s'.", tok.raw), Expression: ""})
		}
		return condValue{tkNumber, f}
	case tkString:
		return condValue{tkString, tok.raw}
	case tkTrue:
		return condValue{tkTrue, true}
	case tkFalse:
		return condValue{tkFalse, false}
	case tkNull:
		return condValue{tkNull, nil}
	default:
		panic(&ConditionError{Message: fmt.Sprintf("Expected a value, got '%s'.", tok.raw), Expression: ""})
	}
}

func (p *conditionParser) resolvePath(path string) any {
	raw, err := ResolvePath(path, p.context)
	if err != nil {
		panic(&ConditionError{Message: err.Error(), Expression: path})
	}
	if raw == nil {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	switch t := v.(type) {
	case nil:
		return nil
	case bool:
		return t
	case float64:
		return t
	case string:
		return t
	default:
		// object/array → raw text (mirrors .NET GetRawText fallback)
		return string(raw)
	}
}

func asTruthy(v condValue) bool {
	switch t := v.value.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	case string:
		return t != ""
	case nil:
		return false
	default:
		return true
	}
}

func compareValues(lhs condValue, op tokenKind, rhs condValue) bool {
	if op == tkEq {
		return valuesEqual(lhs.value, rhs.value)
	}
	if op == tkNeq {
		return !valuesEqual(lhs.value, rhs.value)
	}
	if lhs.value == nil || rhs.value == nil {
		return false
	}

	if ld, ok := lhs.value.(float64); ok {
		if rd, ok := rhs.value.(float64); ok {
			switch op {
			case tkGt:
				return ld > rd
			case tkGte:
				return ld >= rd
			case tkLt:
				return ld < rd
			case tkLte:
				return ld <= rd
			}
			return false
		}
	}

	if ls, ok := lhs.value.(string); ok {
		if rs, ok := rhs.value.(string); ok {
			cmp := strings.Compare(ls, rs)
			switch op {
			case tkGt:
				return cmp > 0
			case tkGte:
				return cmp >= 0
			case tkLt:
				return cmp < 0
			case tkLte:
				return cmp <= 0
			}
			return false
		}
	}

	return false
}

func valuesEqual(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a == b
}
