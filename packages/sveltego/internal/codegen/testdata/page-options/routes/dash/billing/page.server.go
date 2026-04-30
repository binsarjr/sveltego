//go:build sveltego

package billing

import "github.com/binsarjr/sveltego/exports/kit"

const (
	Prerender     = true
	SSROnly       = true
	TrailingSlash = kit.TrailingSlashIgnore
)

type PageData struct{ Total int }

func Load(ctx *kit.LoadCtx) (PageData, error) {
	_ = ctx
	return PageData{Total: 42}, nil
}
