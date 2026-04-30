//go:build sveltego

package _id_

import (
	"errors"

	"github.com/binsarjr/sveltego/exports/kit"
)

func Load(ctx *kit.LoadCtx) (struct {
	Title string
	Body  string
}, error,
) {
	id := ctx.Params["id"]
	if id == "" {
		return struct {
			Title string
			Body  string
		}{}, errors.New("missing id param")
	}
	return struct {
		Title string
		Body  string
	}{
		Title: "Post " + id,
		Body:  "This is the body of post " + id + ".",
	}, nil
}
