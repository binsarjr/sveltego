package server

import "errors"

// ErrProxyDisabled is returned by ProxyClient stubs while the gopls proxy is
// not yet wired. Callers fall back to scaffold handlers.
var ErrProxyDisabled = errors.New("gopls proxy not yet implemented")

// ProxyClient is the contract for the future gopls proxy: forward an LSP
// request to a gopls subprocess and return its response. The scaffold ships a
// disabled implementation so handlers can call it unconditionally.
type ProxyClient interface {
	Forward(method string, params any) (any, error)
}

// DisabledProxy is a ProxyClient that always returns ErrProxyDisabled.
type DisabledProxy struct{}

// Forward implements ProxyClient by reporting the proxy is disabled.
func (DisabledProxy) Forward(string, any) (any, error) {
	return nil, ErrProxyDisabled
}
