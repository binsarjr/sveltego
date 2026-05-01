package kit_test

import (
	"testing"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
)

func TestActionDataResult(t *testing.T) {
	t.Parallel()
	got := kit.ActionDataResult(200, map[string]string{"ok": "yes"})
	d, ok := got.(kit.ActionData)
	if !ok {
		t.Fatalf("type = %T, want kit.ActionData", got)
	}
	if d.Code != 200 {
		t.Errorf("Code = %d, want 200", d.Code)
	}
	if m, ok := d.Data.(map[string]string); !ok || m["ok"] != "yes" {
		t.Errorf("Data = %#v, want map[ok:yes]", d.Data)
	}
}

func TestActionFail(t *testing.T) {
	t.Parallel()
	got := kit.ActionFail(400, "bad")
	d, ok := got.(kit.ActionFailData)
	if !ok {
		t.Fatalf("type = %T, want kit.ActionFailData", got)
	}
	if d.Code != 400 {
		t.Errorf("Code = %d, want 400", d.Code)
	}
	if d.Data != "bad" {
		t.Errorf("Data = %v, want bad", d.Data)
	}
}

func TestActionRedirect(t *testing.T) {
	t.Parallel()
	got := kit.ActionRedirect(0, "/dash")
	r, ok := got.(kit.ActionRedirectResult)
	if !ok {
		t.Fatalf("type = %T, want kit.ActionRedirectResult", got)
	}
	if r.Code != 303 {
		t.Errorf("Code = %d, want default 303", r.Code)
	}
	if r.Location != "/dash" {
		t.Errorf("Location = %q, want /dash", r.Location)
	}

	r2 := kit.ActionRedirect(307, "/keep").(kit.ActionRedirectResult)
	if r2.Code != 307 {
		t.Errorf("override Code = %d, want 307", r2.Code)
	}
}

func TestActionMap_DispatchShape(t *testing.T) {
	t.Parallel()
	calls := 0
	m := kit.ActionMap{
		"default": func(_ *kit.RequestEvent) kit.ActionResult {
			calls++
			return kit.ActionDataResult(200, "hi")
		},
	}
	res := m["default"](nil)
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
	if _, ok := res.(kit.ActionData); !ok {
		t.Fatalf("result type = %T", res)
	}
}
