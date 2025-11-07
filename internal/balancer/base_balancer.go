package balancer

import (
	"sync"
)

type BaseBalancer struct {
	sync.RWMutex
	nodes []UpstreamNode
	name  string
	algo  string
	alive map[string]bool
}

// Add new host to the balancer
func (b *BaseBalancer) Add(node UpstreamNode) {
	b.Lock()
	defer b.Unlock()
	for _, n := range b.nodes {
		if n == node {
			return
		}
	}
	b.nodes = append(b.nodes, node)
}

// Remove new host from the balancer
func (b *BaseBalancer) Remove(host UpstreamNode) {
	b.Lock()
	defer b.Unlock()
	for i, h := range b.nodes {
		if h == host {
			b.nodes = append(b.nodes[:i], b.nodes[i+1:]...)
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

func (b *BaseBalancer) Algo() string {
	return b.algo
}

func (b *BaseBalancer) Hosts() []UpstreamNode {
	return b.nodes
}
