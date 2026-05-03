package fallback

import "testing"

func TestStripFragmentMarkers_basicWrap(t *testing.T) {
	t.Parallel()
	got := StripFragmentMarkers("<!--[--><h1>x</h1><!--]-->")
	want := "<h1>x</h1>"
	if got != want {
		t.Errorf("strip = %q, want %q", got, want)
	}
}

func TestStripFragmentMarkers_preservesPadding(t *testing.T) {
	t.Parallel()
	// Leading + trailing whitespace must survive so concatenation
	// against the surrounding chain doesn't collapse spacing.
	got := StripFragmentMarkers("\n<!--[--><p>x</p><!--]-->\n")
	want := "\n<p>x</p>\n"
	if got != want {
		t.Errorf("strip = %q, want %q", got, want)
	}
}

func TestStripFragmentMarkers_preservesInnerContent(t *testing.T) {
	t.Parallel()
	in := "<!--[--><h1>Hello</h1> <p>world <!---->trailing<!--]-->"
	got := StripFragmentMarkers(in)
	want := "<h1>Hello</h1> <p>world <!---->trailing"
	if got != want {
		t.Errorf("strip = %q, want %q", got, want)
	}
}

func TestStripFragmentMarkers_noMarkersIsNoop(t *testing.T) {
	t.Parallel()
	in := "<h1>plain</h1>"
	if got := StripFragmentMarkers(in); got != in {
		t.Errorf("expected no-op, got %q", got)
	}
}

func TestStripFragmentMarkers_unmatchedOpenLeftAlone(t *testing.T) {
	t.Parallel()
	// Open without close: the sidecar produced something unexpected;
	// leave the body untouched so the mismatch surfaces upstream.
	in := "<!--[--><h1>x</h1>"
	if got := StripFragmentMarkers(in); got != in {
		t.Errorf("expected no-op on unmatched open, got %q", got)
	}
}

func TestStripFragmentMarkers_emptyBody(t *testing.T) {
	t.Parallel()
	if got := StripFragmentMarkers(""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}
