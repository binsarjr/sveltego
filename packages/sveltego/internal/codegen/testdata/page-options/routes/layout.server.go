//go:build sveltego

package routes

import "github.com/binsarjr/sveltego/exports/kit"

const TrailingSlash = kit.TrailingSlashAlways

type LayoutData struct{ Name string }

func Load(ctx *kit.LoadCtx) (LayoutData, error) {
	_ = ctx
	return LayoutData{Name: "root"}, nil
}
