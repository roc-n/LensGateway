package balancer

import (
	"sync"
)

type BaseBalancer struct {
	sync.RWMutex
	hosts []UpstreamNode
	name  string
	algo  string
}

// Add new host to the balancer
func (b *BaseBalancer) Add(host UpstreamNode) {
	b.Lock()
	defer b.Unlock()
	for _, h := range b.hosts {
		if h == host {
			return
		}
	}
	b.hosts = append(b.hosts, host)
}

// Remove new host from the balancer
func (b *BaseBalancer) Remove(host UpstreamNode) {
	b.Lock()
	defer b.Unlock()
	for i, h := range b.hosts {
		if h == host {
			b.hosts = append(b.hosts[:i], b.hosts[i+1:]...)
			return
		}
	}
}

// Balance selects a suitable host according
func (b *BaseBalancer) Balance(key string) (string, error) {
	return "", nil
}

// Inc .
func (b *BaseBalancer) Inc(_ string) {}

// Done .
func (b *BaseBalancer) Done(_ string) {}

// Done .
func (b *BaseBalancer) RequestCtx() func(string) {
	return func(_ string) {}
}

func (b *BaseBalancer) Name() string {
	return b.name
}
