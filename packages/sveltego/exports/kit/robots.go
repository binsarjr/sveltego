package kit

import "strings"

// robotsGroup is one User-agent block in a robots.txt file.
type robotsGroup struct {
	agent string
	rules []robotsRule
}

type robotsRule struct {
	directive string
	value     string
}

// RobotsBuilder accumulates User-agent groups and global Sitemap lines
// for a robots.txt file. The chainable API mirrors the on-disk format:
// each call to UserAgent starts a new group; subsequent Allow / Disallow
// calls append to that group; Sitemap calls record global directives
// emitted after every group.
//
// Not safe for concurrent use; build per-request and discard.
type RobotsBuilder struct {
	groups   []robotsGroup
	sitemaps []string
}

// NewRobots returns an empty builder.
func NewRobots() *RobotsBuilder {
	return &RobotsBuilder{}
}

// UserAgent starts a new group keyed by name. Subsequent Allow and
// Disallow calls attach to this group until UserAgent is called again.
// Use "*" to match every crawler.
func (r *RobotsBuilder) UserAgent(name string) *RobotsBuilder {
	r.groups = append(r.groups, robotsGroup{agent: name})
	return r
}

// Allow records an Allow directive on the current group. If no
// UserAgent group has been opened yet, Allow opens an implicit "*" group
// so the rule has a host. Returns r for chaining.
func (r *RobotsBuilder) Allow(path string) *RobotsBuilder {
	r.appendRule("Allow", path)
	return r
}

// Disallow records a Disallow directive on the current group. Same
// implicit "*" rule as Allow when no group has been opened.
func (r *RobotsBuilder) Disallow(path string) *RobotsBuilder {
	r.appendRule("Disallow", path)
	return r
}

// Sitemap records a global Sitemap directive emitted after every
// User-agent group. Multiple Sitemap calls produce multiple lines, in
// insertion order.
func (r *RobotsBuilder) Sitemap(url string) *RobotsBuilder {
	r.sitemaps = append(r.sitemaps, url)
	return r
}

// String renders the accumulated robots.txt body. Lines end with `\n`.
func (r *RobotsBuilder) String() string {
	var b strings.Builder
	for i, g := range r.groups {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("User-agent: ")
		b.WriteString(g.agent)
		b.WriteByte('\n')
		for _, rule := range g.rules {
			b.WriteString(rule.directive)
			b.WriteString(": ")
			b.WriteString(rule.value)
			b.WriteByte('\n')
		}
	}
	if len(r.sitemaps) > 0 {
		if len(r.groups) > 0 {
			b.WriteByte('\n')
		}
		for _, s := range r.sitemaps {
			b.WriteString("Sitemap: ")
			b.WriteString(s)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (r *RobotsBuilder) appendRule(directive, value string) {
	if len(r.groups) == 0 {
		r.groups = append(r.groups, robotsGroup{agent: "*"})
	}
	idx := len(r.groups) - 1
	r.groups[idx].rules = append(r.groups[idx].rules, robotsRule{directive: directive, value: value})
}
