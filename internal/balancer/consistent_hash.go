package balancer

import (
	"net/url"
	"slices"

	"github.com/lafikl/consistent"
)

func init() {
	factories[ConsistentHashBalancer] = NewConsistent
}

// Consistent refers to consistent hash.
type Consistent struct {
	BaseBalancer
	ch *consistent.Consistent
}

// NewRoundRobin create new RoundRobin balancer
func NewConsistent(name, algo string, nodes []UpstreamNode) Balancer {
	alive := make(map[string]bool)
	for _, node := range nodes {
		host := node.Url.Host
		alive[host] = true // initial mark alive
	}

	c := &Consistent{
		ch: consistent.New(),
		BaseBalancer: BaseBalancer{
			name:  name,
			algo:  algo,
			alive: alive,
			nodes: nodes,
		},
	}

	for _, h := range nodes {
		c.ch.Add(h.Url.Host)
	}
	return c
}

// Add new host to the balancer
func (c *Consistent) Add(node UpstreamNode) {
	c.Lock()
	defer c.Unlock()

	c.ch.Add(node.Url.Host)
	c.nodes = append(c.nodes, node)
}

// Remove new host from the balancer
func (c *Consistent) Remove(node UpstreamNode) {
	c.Lock()
	defer c.Unlock()

	c.ch.Remove(node.Url.Host)
	for i, h := range c.nodes {
		if h == node {
			c.nodes = slices.Delete(c.nodes, i, i+1)
			return
		}
	}
}

// Balance selects a suitable host according to the key value
func (c *Consistent) Balance(key string) (UpstreamNode, error) {
	if len(c.ch.Hosts()) == 0 {
		return UpstreamNode{}, ErrorNoHost
	}

	host, _ := c.ch.Get(key)
	url := url.URL{Host: host}
	node := UpstreamNode{Url: &url}
	return node, nil
}
