package network

import "net"

type AutoHttpListener struct {
	net.Listener
}

func NewAutoHttpListener(listener net.Listener) net.Listener {
	return &AutoHttpListener{
		Listener: listener,
	}
}

func (l *AutoHttpListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return NewAutoHttpConn(conn), nil
}
