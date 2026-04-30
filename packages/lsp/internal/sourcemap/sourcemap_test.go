package sourcemap

import "testing"

func TestIdentity_RoundTrip(t *testing.T) {
	t.Parallel()
	m := Identity("a.svelte", "a.gen.go", 100)
	for _, off := range []int{0, 1, 50, 99} {
		got, ok := m.SvelteToGo(off)
		if !ok || got != off {
			t.Errorf("SvelteToGo(%d) = (%d,%v), want (%d,true)", off, got, ok, off)
		}
		back, ok := m.GoToSvelte(got)
		if !ok || back != off {
			t.Errorf("GoToSvelte(%d) = (%d,%v), want (%d,true)", got, back, ok, off)
		}
	}
}

func TestIdentity_OutOfRange(t *testing.T) {
	t.Parallel()
	m := Identity("a.svelte", "a.gen.go", 10)
	if _, ok := m.SvelteToGo(10); ok {
		t.Errorf("SvelteToGo at end should be out of range")
	}
	if _, ok := m.SvelteToGo(99); ok {
		t.Errorf("SvelteToGo past end should be out of range")
	}
	if _, ok := m.GoToSvelte(11); ok {
		t.Errorf("GoToSvelte past end should be out of range")
	}
}

func TestSpans_DiscontinuousMapping(t *testing.T) {
	t.Parallel()
	m := New("p.svelte", "p.gen.go", []Span{
		// later span first to verify sort
		{SvelteStart: 50, SvelteEnd: 60, GoStart: 200, GoEnd: 210},
		{SvelteStart: 10, SvelteEnd: 20, GoStart: 100, GoEnd: 110},
	})

	cases := []struct {
		name      string
		svelteOff int
		wantGo    int
		wantOK    bool
	}{
		{"start of first span", 10, 100, true},
		{"middle of first span", 15, 105, true},
		{"end of first span (exclusive)", 20, 0, false},
		{"gap between spans", 30, 0, false},
		{"start of second span", 50, 200, true},
		{"middle of second span", 55, 205, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := m.SvelteToGo(tc.svelteOff)
			if ok != tc.wantOK {
				t.Errorf("ok=%v, want %v", ok, tc.wantOK)
			}
			if ok && got != tc.wantGo {
				t.Errorf("got go offset %d, want %d", got, tc.wantGo)
			}
		})
	}
}

func TestNew_Empty(t *testing.T) {
	t.Parallel()
	m := New("a.svelte", "a.gen.go", nil)
	if _, ok := m.SvelteToGo(0); ok {
		t.Errorf("empty map should yield no mapping")
	}
}

func TestNil_Safe(t *testing.T) {
	t.Parallel()
	var m *Map
	if _, ok := m.SvelteToGo(0); ok {
		t.Errorf("nil map should be safe and yield ok=false")
	}
	if _, ok := m.GoToSvelte(0); ok {
		t.Errorf("nil map should be safe and yield ok=false")
	}
}
