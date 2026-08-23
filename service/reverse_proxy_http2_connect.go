package service

import _ "unsafe"

// x/net/http2 keeps RFC 8441 Extended CONNECT disabled by default for
// compatibility with legacy websocket handlers. The reverse proxy handles
// the stream itself, so advertise and accept the protocol on its H2 server.
// The module version is pinned in go.mod; this link avoids changing the
// dependency's global default for unrelated HTTP/2 clients in the process.
//
//go:linkname reverseProxyHTTP2DisableExtendedConnectProtocol golang.org/x/net/http2.disableExtendedConnectProtocol
var reverseProxyHTTP2DisableExtendedConnectProtocol bool

func enableReverseProxyHTTP2ExtendedConnect() {
	reverseProxyHTTP2DisableExtendedConnectProtocol = false
}
