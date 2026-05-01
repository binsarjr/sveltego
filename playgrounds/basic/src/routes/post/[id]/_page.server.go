//go:build sveltego

package _id_

import (
	"errors"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
)

const Templates = "svelte"

type PageData struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

func Load(ctx *kit.LoadCtx) (PageData, error) {
	id := ctx.Params["id"]
	if id == "" {
		return PageData{}, errors.New("missing id param")
	}
	return PageData{
		Title: "Post " + id,
		Body:  "This is the body of post " + id + ".",
	}, nil
}
