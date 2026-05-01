package server

import (
	"sort"
	"strings"
)

// ToClass mirrors svelte/internal/shared/attributes.to_class. Combines a
// base class value, an optional CSS hash, and a directives map (class:foo).
// Returns "" (instead of JS null) when nothing remains — the caller in
// compiled output checks for "" before emitting class="".
func ToClass(value any, hash string, directives map[string]bool) string {
	classname := ""
	if value != nil {
		classname = Stringify(value)
	}
	if hash != "" {
		if classname != "" {
			classname += " " + hash
		} else {
			classname = hash
		}
	}
	if len(directives) == 0 {
		return classname
	}
	keys := make([]string, 0, len(directives))
	for k := range directives {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if directives[key] {
			if classname != "" {
				classname += " " + key
			} else {
				classname = key
			}
		} else if classname != "" {
			classname = removeClassToken(classname, key)
		}
	}
	return classname
}

// removeClassToken removes the whitespace-bounded occurrences of token from s.
// Mirrors Svelte's whitespace-bounded indexOf loop in to_class.
func removeClassToken(s, token string) string {
	tlen := len(token)
	if tlen == 0 {
		return s
	}
	a := 0
	for {
		idx := strings.Index(s[a:], token)
		if idx < 0 {
			return s
		}
		idx += a
		b := idx + tlen
		leftOK := idx == 0 || isClassWhitespace(s[idx-1])
		rightOK := b == len(s) || isClassWhitespace(s[b])
		if leftOK && rightOK {
			cut := b
			if cut < len(s) {
				cut = b + 1
			}
			s = s[:idx] + s[cut:]
			s = strings.TrimRight(s, " ")
		} else {
			a = b
		}
		if a >= len(s) {
			return s
		}
	}
}

func isClassWhitespace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '\f':
		return true
	}
	return false
}

// ToStyle mirrors svelte/internal/shared/attributes.to_style. When styles
// is empty, returns the trimmed value (or "" for nil). When styles is set,
// merges literal value with the styles map, dropping reserved keys.
//
// styles is a single map keyed by CSS property name. The optional important
// flag is encoded by passing the same map twice — first call regular, second
// !important — kept simple for v1.
func ToStyle(value any, styles map[string]string) string {
	if len(styles) == 0 {
		if value == nil {
			return ""
		}
		return strings.TrimSpace(Stringify(value))
	}

	reserved := make(map[string]struct{}, len(styles))
	for k := range styles {
		reserved[toCSSName(k)] = struct{}{}
	}

	var out strings.Builder
	if value != nil {
		raw := Stringify(value)
		raw = stripCSSComments(raw)
		raw = strings.TrimSpace(raw)
		for _, decl := range splitDecls(raw) {
			name, _, ok := splitDecl(decl)
			if !ok {
				continue
			}
			if _, skip := reserved[toCSSName(strings.TrimSpace(name))]; skip {
				continue
			}
			if out.Len() > 0 {
				out.WriteByte(' ')
			}
			out.WriteString(strings.TrimSpace(decl))
			out.WriteByte(';')
		}
	}

	keys := make([]string, 0, len(styles))
	for k := range styles {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := strings.TrimSpace(styles[k])
		if v == "" {
			continue
		}
		if out.Len() > 0 {
			out.WriteByte(' ')
		}
		out.WriteString(toCSSName(k))
		out.WriteString(": ")
		out.WriteString(v)
		out.WriteByte(';')
	}
	return strings.TrimSpace(out.String())
}

func toCSSName(name string) string {
	if strings.HasPrefix(name, "--") {
		return name
	}
	return strings.ToLower(name)
}

func stripCSSComments(s string) string {
	for {
		i := strings.Index(s, "/*")
		if i < 0 {
			return s
		}
		j := strings.Index(s[i+2:], "*/")
		if j < 0 {
			return s[:i]
		}
		s = s[:i] + s[i+2+j+2:]
	}
}

func splitDecls(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	depth := 0
	inStr := byte(0)
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inStr != 0:
			if c == inStr {
				inStr = 0
			}
		case c == '\'' || c == '"':
			inStr = c
		case c == '(':
			depth++
		case c == ')':
			if depth > 0 {
				depth--
			}
		case c == ';' && depth == 0:
			seg := strings.TrimSpace(s[start:i])
			if seg != "" {
				out = append(out, seg)
			}
			start = i + 1
		}
	}
	if start < len(s) {
		seg := strings.TrimSpace(s[start:])
		if seg != "" {
			out = append(out, seg)
		}
	}
	return out
}

func splitDecl(decl string) (name, value string, ok bool) {
	idx := strings.IndexByte(decl, ':')
	if idx < 0 {
		return "", "", false
	}
	return decl[:idx], strings.TrimSpace(decl[idx+1:]), true
}
