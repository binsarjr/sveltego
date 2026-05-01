//go:build sveltego

package routes

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

const Templates = "svelte"

type Post struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type PageData struct {
	Greeting string `json:"greeting"`
	Posts    []Post `json:"posts"`
}

func Load(ctx *kit.LoadCtx) (PageData, error) {
	_ = ctx
	return PageData{
		Greeting: "Hello, sveltego!",
		Posts: []Post{
			{ID: "1", Title: "First post"},
			{ID: "2", Title: "Second post"},
		},
	}, nil
}
