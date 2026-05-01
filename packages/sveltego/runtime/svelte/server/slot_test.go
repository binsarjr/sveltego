package server

import "testing"

func TestSlotProvided(t *testing.T) {
	called := false
	slotFn := SlotFn(func(p *Payload, sp map[string]any) {
		called = true
		p.Push("slot:" + Stringify(sp["x"]))
	})
	props := map[string]any{
		"$$slots": map[string]any{"default": slotFn},
	}
	var p Payload
	Slot(&p, props, "default", map[string]any{"x": 1}, func(p *Payload) {
		p.Push("FALLBACK")
	})
	if !called {
		t.Fatal("slot fn not called")
	}
	if got := p.Body(); got != "slot:1" {
		t.Errorf("body = %q", got)
	}
}

func TestSlotFallback(t *testing.T) {
	var p Payload
	Slot(&p, nil, "default", nil, func(p *Payload) {
		p.Push("fallback")
	})
	if got := p.Body(); got != "fallback" {
		t.Errorf("body = %q", got)
	}
}

func TestSlotNoFallbackNoSlot(t *testing.T) {
	var p Payload
	Slot(&p, map[string]any{}, "default", nil, nil)
	if got := p.Body(); got != "" {
		t.Errorf("body = %q", got)
	}
}

func TestSlotChildrenAlias(t *testing.T) {
	called := false
	props := map[string]any{
		"children": SlotFn(func(p *Payload, _ map[string]any) {
			called = true
			p.Push("kid")
		}),
	}
	var p Payload
	Slot(&p, props, "default", nil, nil)
	if !called {
		t.Fatal("children alias not honored")
	}
	if got := p.Body(); got != "kid" {
		t.Errorf("body = %q", got)
	}
}
