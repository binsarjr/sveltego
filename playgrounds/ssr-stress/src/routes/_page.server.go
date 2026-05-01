//go:build sveltego

package routes

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

const Templates = "svelte"

type PageData struct {
	Greeting string `json:"greeting"`
}

func Load(ctx *kit.LoadCtx) (PageData, error) {
	_ = ctx
	return PageData{Greeting: "ssr-stress index"}, nil
}
