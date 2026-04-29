package ast

import "strings"

// ParseError is a single problem found while parsing a Svelte template.
// Hint is optional and may be empty.
type ParseError struct {
	Pos     Pos
	Message string
	Hint    string
}

// Error formats the error as "line:col: message" with an optional
// " (hint: ...)" suffix when Hint is set.
func (e ParseError) Error() string {
	var b strings.Builder
	b.WriteString(e.Pos.String())
	b.WriteString(": ")
	b.WriteString(e.Message)
	if e.Hint != "" {
		b.WriteString(" (hint: ")
		b.WriteString(e.Hint)
		b.WriteString(")")
	}
	return b.String()
}

// Errors is a list of ParseError values aggregated by the parser. The zero
// value is an empty list.
type Errors []ParseError

// Error joins each contained error on its own line. The format mirrors
// hashicorp/multierror so existing tooling renders it sensibly.
func (es Errors) Error() string {
	switch len(es) {
	case 0:
		return ""
	case 1:
		return es[0].Error()
	}
	var b strings.Builder
	for i, e := range es {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(e.Error())
	}
	return b.String()
}

// ErrorOrNil returns nil when there are no errors and es otherwise. Lets
// callers write `return ast.Errors(errs).ErrorOrNil()` without wrapping.
func (es Errors) ErrorOrNil() error {
	if len(es) == 0 {
		return nil
	}
	return es
}
