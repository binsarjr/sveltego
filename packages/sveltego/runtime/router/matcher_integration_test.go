package router_test

import (
	"testing"

	"github.com/binsarjr/sveltego/exports/kit/params"
	"github.com/binsarjr/sveltego/runtime/router"
)

func TestMatcherIntegration_IntAccepts(t *testing.T) {
	tree := mustBuild(t, []router.Route{mkRoute(t, "/post/[id=int]")})
	tree, err := tree.WithMatchers(params.DefaultMatchers())
	if err != nil {
		t.Fatalf("WithMatchers: %v", err)
	}
	r, ps, ok := tree.Match("/post/42")
	if !ok {
		t.Fatalf("expected match for /post/42")
	}
	if r.Pattern != "/post/[id=int]" {
		t.Errorf("Pattern = %q", r.Pattern)
	}
	if got := ps["id"]; got != "42" {
		t.Errorf("id = %q, want 42", got)
	}
}

func TestMatcherIntegration_IntRejects(t *testing.T) {
	tree := mustBuild(t, []router.Route{mkRoute(t, "/post/[id=int]")})
	tree, err := tree.WithMatchers(params.DefaultMatchers())
	if err != nil {
		t.Fatalf("WithMatchers: %v", err)
	}
	if r, _, ok := tree.Match("/post/abc"); ok {
		t.Errorf("expected no match for /post/abc, got %q", r.Pattern)
	}
}

func TestMatcherIntegration_SlugAccepts(t *testing.T) {
	tree := mustBuild(t, []router.Route{mkRoute(t, "/blog/[slug=slug]")})
	tree, err := tree.WithMatchers(params.DefaultMatchers())
	if err != nil {
		t.Fatalf("WithMatchers: %v", err)
	}
	r, ps, ok := tree.Match("/blog/my-post")
	if !ok {
		t.Fatalf("expected match for /blog/my-post")
	}
	if r.Pattern != "/blog/[slug=slug]" {
		t.Errorf("Pattern = %q", r.Pattern)
	}
	if got := ps["slug"]; got != "my-post" {
		t.Errorf("slug = %q, want my-post", got)
	}
}

func TestMatcherIntegration_SlugRejects(t *testing.T) {
	tree := mustBuild(t, []router.Route{mkRoute(t, "/blog/[slug=slug]")})
	tree, err := tree.WithMatchers(params.DefaultMatchers())
	if err != nil {
		t.Fatalf("WithMatchers: %v", err)
	}
	if r, _, ok := tree.Match("/blog/My_Post"); ok {
		t.Errorf("expected no match for /blog/My_Post, got %q", r.Pattern)
	}
}

func TestMatcherIntegration_UUIDAccepts(t *testing.T) {
	tree := mustBuild(t, []router.Route{mkRoute(t, "/u/[id=uuid]")})
	tree, err := tree.WithMatchers(params.DefaultMatchers())
	if err != nil {
		t.Fatalf("WithMatchers: %v", err)
	}
	r, ps, ok := tree.Match("/u/00000000-0000-0000-0000-000000000000")
	if !ok {
		t.Fatalf("expected match")
	}
	if r.Pattern != "/u/[id=uuid]" {
		t.Errorf("Pattern = %q", r.Pattern)
	}
	if got := ps["id"]; got != "00000000-0000-0000-0000-000000000000" {
		t.Errorf("id = %q", got)
	}
}

func TestMatcherIntegration_DefaultMatchers_EndToEnd(t *testing.T) {
	tree := mustBuild(t, []router.Route{
		mkRoute(t, "/u/[id=int]"),
		mkRoute(t, "/u/[name=slug]"),
	})
	tree, err := tree.WithMatchers(params.DefaultMatchers())
	if err != nil {
		t.Fatalf("WithMatchers: %v", err)
	}

	// numeric → int branch
	r, ps, ok := tree.Match("/u/42")
	if !ok {
		t.Fatalf("no match for /u/42")
	}
	if r.Pattern != "/u/[id=int]" {
		t.Errorf("Pattern = %q, want /u/[id=int]", r.Pattern)
	}
	if got := ps["id"]; got != "42" {
		t.Errorf("id = %q", got)
	}

	// slug → slug branch
	r, ps, ok = tree.Match("/u/jane-doe")
	if !ok {
		t.Fatalf("no match for /u/jane-doe")
	}
	if r.Pattern != "/u/[name=slug]" {
		t.Errorf("Pattern = %q, want /u/[name=slug]", r.Pattern)
	}
	if got := ps["name"]; got != "jane-doe" {
		t.Errorf("name = %q", got)
	}
}
