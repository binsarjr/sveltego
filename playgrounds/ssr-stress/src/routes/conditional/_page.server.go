//go:build sveltego

package conditional

import "github.com/binsarjr/sveltego/packages/sveltego/exports/kit"

const Templates = "svelte"

type PageData struct {
	LoggedIn bool   `json:"loggedIn"`
	Username string `json:"username"`
}

func Load(ctx *kit.LoadCtx) (PageData, error) {
	_ = ctx
	return PageData{LoggedIn: true, Username: "alice"}, nil
}
