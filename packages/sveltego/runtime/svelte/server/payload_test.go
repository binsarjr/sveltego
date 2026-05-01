package server

import "testing"

func TestPayload(t *testing.T) {
	var p Payload
	p.Push("hello ")
	p.Push("world")
	if got := p.Body(); got != "hello world" {
		t.Errorf("Body() = %q", got)
	}
	p.PushHead("<title>x</title>")
	if got := p.HeadHTML(); got != "<title>x</title>" {
		t.Errorf("HeadHTML() = %q", got)
	}
	p.Title = "foo"
	p.Reset()
	if p.Body() != "" || p.HeadHTML() != "" || p.Title != "" {
		t.Errorf("Reset didn't clear: body=%q head=%q title=%q", p.Body(), p.HeadHTML(), p.Title)
	}
}
