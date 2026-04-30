//go:build sveltego

package routes

import "github.com/binsarjr/sveltego/exports/kit"

func Load(ctx *kit.LoadCtx) (struct {
	Greeting string
	Posts    []struct {
		ID    string
		Title string
	}
}, error,
) {
	_ = ctx
	return struct {
		Greeting string
		Posts    []struct {
			ID    string
			Title string
		}
	}{
		Greeting: "Hello, sveltego!",
		Posts: []struct {
			ID    string
			Title string
		}{
			{ID: "1", Title: "First post"},
			{ID: "2", Title: "Second post"},
		},
	}, nil
}
