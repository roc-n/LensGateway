package balancer

import (
	"log"
	"sync"
	"time"

	"LensGateway.com/util"
)

// package-level supervisor state so HealthCheckAll can be called
// multiple times without creating duplicate workers.
var (
	healthMu      sync.Mutex
	healthWorkers = make(map[string]chan struct{})
)

// ReadAlive reads the alive status of the site
func (b *BaseBalancer) ReadAlive(host string) bool {
	return b.alive[host]
}

// SetAlive sets the alive status to the site
func (b *BaseBalancer) SetAlive(host string, alive bool) {
	b.alive[host] = alive
}

func HealthCheckAll(balancers []Balancer, interval uint) {
	// Simplified policy: stop all existing health-check workers and recreate
	// them from the latest balancers slice. This avoids complex diffing when
	// balancer internals (like hosts) change.
	healthMu.Lock()
	defer healthMu.Unlock()

	desired := make(map[string]Balancer)
	for _, b := range balancers {
		desired[b.Name()] = b
	}

	// stop all existing workers unconditionally and clear the map. This keeps
	// the implementation simple: callers that rebuild the routing table can
	// call HealthCheckAll and expect health-check workers to reflect the
	// latest configuration.
	for name, stopCh := range healthWorkers {
		close(stopCh)
		delete(healthWorkers, name)
	}

	// start workers for desired balancers
	for name, b := range desired {
		stopCh := make(chan struct{})
		healthWorkers[name] = stopCh
		go func(bb Balancer, stop <-chan struct{}) {
			ticker := time.NewTicker(time.Duration(interval) * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					nodes := bb.Hosts()
					var wg sync.WaitGroup
					for _, node := range nodes {
						// probe each node concurrently
						wg.Add(1)
						go func(n UpstreamNode) {
							defer wg.Done()
							host := n.Url.Host
							alive := util.IsBackendAlive(host)
							// try to use ReadAlive/SetAlive if balancer exposes them (usually BaseBalancer)
							if as, ok := bb.(interface {
								ReadAlive(string) bool
								SetAlive(string, bool)
							}); ok {
								prev := as.ReadAlive(host)
								if !alive && prev {
									log.Printf("Site unreachable, remove %s from load balancer.", host)
									as.SetAlive(host, false)
									bb.Remove(n)
								} else if alive && !prev {
									log.Printf("Site reachable, add %s to load balancer.", host)
									as.SetAlive(host, true)
									bb.Add(n)
								}
							} else {
								// fallback: if host not alive, try remove; if alive, try add
								if !alive {
									bb.Remove(n)
								} else {
									bb.Add(n)
								}
							}
						}(node)
					}
					wg.Wait()
				case <-stop:
					return
				}
			}
		}(b, stopCh)
	}
}
