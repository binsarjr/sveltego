package mcp

import (
	"fmt"
	"path"
	"strings"
)

const (
	kindPage   = "page"
	kindLayout = "layout"
	kindServer = "server"
	kindError  = "error"
)

// scaffold returns boilerplate for a route file at routePath of kind.
// Honors ADR 0003: server-side .go files have no `+` prefix and start
// with `//go:build sveltego`.
func scaffold(routePath, kind string) (string, error) {
	clean := path.Clean("/" + strings.Trim(routePath, "/"))
	rel := strings.TrimPrefix(clean, "/")
	if rel == "." {
		rel = ""
	}
	dir := path.Join("src/routes", rel)
	switch kind {
	case kindPage:
		return scaffoldPage(dir, rel), nil
	case kindLayout:
		return scaffoldLayout(dir, rel), nil
	case kindServer:
		return scaffoldServer(dir, rel), nil
	case kindError:
		return scaffoldError(dir, rel), nil
	default:
		return "", fmt.Errorf("unknown kind %q (want page|layout|server|error)", kind)
	}
}

func scaffoldPage(dir, rel string) string {
	title := titleFor(rel, "Page")
	svelte := fmt.Sprintf(`<!-- %s/+page.svelte -->
<script lang="go">
type PageData = struct {
	Title string
}
</script>

<h1>{Data.Title}</h1>
`, dir)
	server := fmt.Sprintf(`//go:build sveltego

package routes

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

func Load(ctx *kit.LoadCtx) (any, error) {
	return struct {
		Title string
	}{
		Title: %q,
	}, nil
}
`, title)
	return joinFiles(
		fileBlock(path.Join(dir, "+page.svelte"), svelte),
		fileBlock(path.Join(dir, "page.server.go"), server),
	)
}

func scaffoldLayout(dir, rel string) string {
	title := titleFor(rel, "Layout")
	svelte := fmt.Sprintf(`<!-- %s/+layout.svelte -->
<script lang="go">
type LayoutData = struct {
	Title string
}
</script>

<header>
	<h1>{Data.Title}</h1>
</header>

<main>
	<slot />
</main>
`, dir)
	server := fmt.Sprintf(`//go:build sveltego

package routes

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

func Load(ctx *kit.LoadCtx) (any, error) {
	return struct {
		Title string
	}{
		Title: %q,
	}, nil
}
`, title)
	return joinFiles(
		fileBlock(path.Join(dir, "+layout.svelte"), svelte),
		fileBlock(path.Join(dir, "layout.server.go"), server),
	)
}

func scaffoldServer(dir, _ string) string {
	body := `//go:build sveltego

package routes

import (
	"net/http"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
)

func GET(ev *kit.RequestEvent) (*kit.Response, error) {
	return kit.JSON(http.StatusOK, map[string]any{
		"ok": true,
	}), nil
}
`
	return fileBlock(path.Join(dir, "server.go"), body)
}

func scaffoldError(dir, _ string) string {
	body := `<!-- +error.svelte -->
<script lang="go">
type PageData = struct {
	Status  int
	Message string
}
</script>

<h1>{Data.Status} — {Data.Message}</h1>
`
	return fileBlock(path.Join(dir, "+error.svelte"), body)
}

func titleFor(rel, fallback string) string {
	if rel == "" {
		return fallback
	}
	last := path.Base(rel)
	if last == "." || last == "/" {
		return fallback
	}
	if strings.HasPrefix(last, "[") && strings.HasSuffix(last, "]") {
		return fallback
	}
	return strings.ToUpper(last[:1]) + last[1:]
}

func fileBlock(filename, body string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// %s\n", filename)
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}

func joinFiles(blocks ...string) string {
	return strings.Join(blocks, "\n")
}
