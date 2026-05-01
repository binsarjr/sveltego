package server

import "testing"

func TestElement(t *testing.T) {
	var p Payload
	Element(&p, "div", func(p *Payload) {
		p.Push(Attr("id", "main", false))
	}, func(p *Payload) {
		p.Push("hi")
	})
	want := `<!----><div id="main">hi<!----></div><!---->`
	if got := p.Body(); got != want {
		t.Errorf("Element div = %q, want %q", got, want)
	}
}

func TestElementVoid(t *testing.T) {
	var p Payload
	Element(&p, "br", nil, nil)
	want := `<!----><br><!---->`
	if got := p.Body(); got != want {
		t.Errorf("Element br = %q, want %q", got, want)
	}
}

func TestElementRawText(t *testing.T) {
	var p Payload
	Element(&p, "script", nil, func(p *Payload) {
		p.Push("var x = 1;")
	})
	want := `<!----><script>var x = 1;</script><!---->`
	if got := p.Body(); got != want {
		t.Errorf("Element script = %q, want %q", got, want)
	}
}

func TestElementEmptyTag(t *testing.T) {
	var p Payload
	Element(&p, "", nil, nil)
	want := `<!----><!---->`
	if got := p.Body(); got != want {
		t.Errorf("Element empty = %q, want %q", got, want)
	}
}

func TestHead(t *testing.T) {
	var p Payload
	Head(&p, "abc123", func(p *Payload) {
		p.Push("<title>hi</title>")
	})
	want := `<!--abc123--><title>hi</title><!---->`
	if got := p.HeadHTML(); got != want {
		t.Errorf("Head = %q, want %q", got, want)
	}
}

func TestHydrationConstants(t *testing.T) {
	if EmptyComment != "<!---->" {
		t.Errorf("EmptyComment = %q", EmptyComment)
	}
	if BlockOpen != "<!--[-->" {
		t.Errorf("BlockOpen = %q", BlockOpen)
	}
	if BlockClose != "<!--]-->" {
		t.Errorf("BlockClose = %q", BlockClose)
	}
	if BlockOpenElse != "<!--[!-->" {
		t.Errorf("BlockOpenElse = %q", BlockOpenElse)
	}
}

func TestVoidAndRawTables(t *testing.T) {
	if !IsVoidElement("br") || !IsVoidElement("img") {
		t.Errorf("br/img must be void")
	}
	if IsVoidElement("div") {
		t.Errorf("div must not be void")
	}
	if !IsRawTextElement("script") || !IsRawTextElement("style") {
		t.Errorf("script/style must be raw-text")
	}
	if IsRawTextElement("div") {
		t.Errorf("div must not be raw-text")
	}
}
