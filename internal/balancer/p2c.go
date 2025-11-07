package balancer

import (
	"hash/crc32"
	"math/rand"
	"slices"
	"time"
)

const Salt = "%#!?$"

func init() {
	factories[P2CBalancer] = NewP2C
}

type hostInfo struct {
	name string
	load uint64
}

// P2C refer to the power of 2 random choice
type P2C struct {
	BaseBalancer
	rnd     *rand.Rand
	loadMap map[string]*hostInfo
}

// NewP2C create new P2C balancer
func NewP2C(name, algo string, nodes []UpstreamNode) Balancer {
	alive := make(map[string]bool)
	for _, node := range nodes {
		host := node.Url.Host
		alive[host] = true // initial mark alive
	}

	p := &P2C{
		loadMap: make(map[string]*hostInfo),
		rnd:     rand.New(rand.NewSource(time.Now().UnixNano())),
		BaseBalancer: BaseBalancer{
			name:  name,
			algo:  algo,
			alive: alive,
			nodes: nodes,
		},
	}

	for _, node := range nodes {
		host := node.Url.Host
		h := &hostInfo{name: host, load: 0}
		p.loadMap[host] = h
	}

	return p
}

// Add new host to the balancer
func (p *P2C) Add(node UpstreamNode) {
	p.Lock()
	defer p.Unlock()

	hostName := node.Url.Host
	if _, ok := p.loadMap[hostName]; ok {
		return
	}

	h := &hostInfo{name: hostName, load: 0}
	p.nodes = append(p.nodes, node)
	p.loadMap[hostName] = h
}

// Remove new host from the balancer
func (p *P2C) Remove(node UpstreamNode) {
	p.Lock()
	defer p.Unlock()

	host := node.Url.Host
	if _, ok := p.loadMap[host]; !ok {
		return
	}

	delete(p.loadMap, host)

	for i, h := range p.nodes {
		if h == node {
			p.nodes = slices.Delete(p.nodes, i, i+1)
			return
		}
	}
}

// Balance selects a suitable host according to the key value
func (p *P2C) Balance(key string) (UpstreamNode, error) {
	p.RLock()
	defer p.RUnlock()

	if len(p.nodes) == 0 {
		return UpstreamNode{}, ErrorNoHost
	}

	n1, n2 := p.hash(key)
	host := n2
	if p.loadMap[n1.Url.Host].load <= p.loadMap[n2.Url.Host].load {
		host = n1
	}
	return host, nil
}

func (p *P2C) hash(key string) (UpstreamNode, UpstreamNode) {
	var n1, n2 UpstreamNode
	if len(key) > 0 {
		saltKey := key + Salt
		n1 = p.nodes[crc32.ChecksumIEEE([]byte(key))%uint32(len(p.nodes))]
		n2 = p.nodes[crc32.ChecksumIEEE([]byte(saltKey))%uint32(len(p.nodes))]
		return n1, n2
	}
	n1 = p.nodes[p.rnd.Intn(len(p.nodes))]
	n2 = p.nodes[p.rnd.Intn(len(p.nodes))]
	return n1, n2
}

// Inc refers to the number of connections to the server `+1`
func (p *P2C) Inc(host string) {
	p.Lock()
	defer p.Unlock()

	h, ok := p.loadMap[host]

	if !ok {
		return
	}
	h.load++
}

// Done refers to the number of connections to the server `-1`
func (p *P2C) Done(host string) {
	p.Lock()
	defer p.Unlock()

	h, ok := p.loadMap[host]

	if !ok {
		return
	}

	if h.load > 0 {
		h.load--
	}
}
