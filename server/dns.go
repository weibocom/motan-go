package server

import "net"

type Resolver struct {
}

func (r *Resolver) LookupHost(host string) ([]net.IPAddr, error) {

	return nil, nil
}
