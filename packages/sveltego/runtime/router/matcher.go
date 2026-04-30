package router

// ParamMatcher decides whether a captured parameter value is acceptable
// for a route segment carrying `[name=matcher]` syntax.
type ParamMatcher interface {
	Match(value string) bool
}

// Matchers maps matcher names to their ParamMatcher implementations.
type Matchers map[string]ParamMatcher

// MatcherFunc adapts a plain function to the ParamMatcher interface.
type MatcherFunc func(string) bool

// Match calls f.
func (f MatcherFunc) Match(value string) bool { return f(value) }
