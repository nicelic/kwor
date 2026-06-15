package network

import "net"

type ManagedTLSListener struct {
	net.Listener
}

func NewManagedTLSListener(listener net.Listener) net.Listener {
	return &ManagedTLSListener{Listener: listener}
}

func (l *ManagedTLSListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return NewManagedTLSConn(conn), nil
}
