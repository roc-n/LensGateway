package balancer

import (
	"sync/atomic"
)

// RoundRobin will select the server in turn from the server to proxy
type RoundRobin struct {
	BaseBalancer
	i atomic.Uint64
}

func init() {
	factories[R2Balancer] = NewRoundRobin
}

// NewRoundRobin create new RoundRobin balancer
func NewRoundRobin(name, algo string, hosts []UpstreamNode) Balancer {

	alive := make(map[string]bool)
	for _, node := range hosts {
		host := node.Url.Host
		alive[host] = true // initial mark alive
	}

	return &RoundRobin{
		i: atomic.Uint64{},
		BaseBalancer: BaseBalancer{
			hosts: hosts,
			name:  name,
			algo:  algo,
			alive: alive,
		},
	}
}

// Balance selects a suitable host according
func (r *RoundRobin) Balance(_ string) (UpstreamNode, error) {
	r.RLock()
	defer r.RUnlock()
	if len(r.hosts) == 0 {
		return UpstreamNode{}, ErrorNoHost
	}
	host := r.hosts[r.i.Add(1)%uint64(len(r.hosts))]
	return host, nil
}
