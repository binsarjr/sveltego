//go:build sveltego

package level3

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

const Templates = "svelte"

type PageData struct {
	Title string `json:"title"`
}

func Load(ctx *kit.LoadCtx) (PageData, error) {
	_ = ctx
	return PageData{Title: "deep layout leaf"}, nil
}
