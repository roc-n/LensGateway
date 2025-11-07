package balancer

import (
	"errors"
	"net/url"
)

var (
	ErrorNoHost                = errors.New("no host")
	ErrorAlgorithmNotSupported = errors.New("algorithm not supported")
)

type UpstreamNode struct {
	Url *url.URL
}

// Balancer interface is the load balancer for the reverse proxy.
type Balancer interface {
	Add(UpstreamNode)
	Remove(UpstreamNode)
	Balance(string) (UpstreamNode, error)
	Inc(string)
	Done(string)
	// RequestCtx() func(string)

	Name() string
	Algo() string
	Hosts() []UpstreamNode
}

// Factory is the factory that generates Balancer,
// and the factory design pattern is used here.
type Factory func(string, string, []UpstreamNode) Balancer

var factories = make(map[string]Factory)

// Generate corresponding Balancer according to the algorithm.
func Build(name, algo string, hosts []UpstreamNode) (Balancer, error) {
	factory, ok := factories[algo]
	if !ok {
		return nil, ErrorAlgorithmNotSupported
	}
	return factory(name, algo, hosts), nil
}
