//go:build sveltego

package longlist

import (
	"strconv"

	"github.com/binsarjr/sveltego/packages/sveltego/exports/kit"
)

const Templates = "svelte"

type PageData struct {
	Title string   `json:"title"`
	Items []string `json:"items"`
}

func Load(ctx *kit.LoadCtx) (PageData, error) {
	_ = ctx
	items := make([]string, 100)
	for i := range items {
		items[i] = "item " + strconv.Itoa(i)
	}
	return PageData{Title: "longlist", Items: items}, nil
}
