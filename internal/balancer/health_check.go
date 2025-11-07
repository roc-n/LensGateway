package balancer

import (
	"fmt"
	"log"
	"sync"
	"time"

	"LensGateway.com/util"
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
	// Supervisor: maintain a set of workers per-balancer by name.
	// Avoids orphan goroutines when the routing table / balancers are replaced.
	var (
		mu      sync.Mutex
		workers = make(map[string]chan struct{})
	)

	// Reconcile desired balancers once (callers may call this periodically on update).
	mu.Lock()
	defer mu.Unlock()

	desired := make(map[string]Balancer)
	for _, b := range balancers {
		desired[b.Name()] = b
	}

	// stop workers for balancers that no longer exist
	for name, stopCh := range workers {
		if _, ok := desired[name]; !ok {
			close(stopCh)
			delete(workers, name)
		}
	}

	// start workers for new balancers
	for name, b := range desired {
		if _, ok := workers[name]; ok {
			continue
		}
		stopCh := make(chan struct{})
		workers[name] = stopCh
		go func(bb Balancer, stop <-chan struct{}) {
			ticker := time.NewTicker(time.Duration(interval) * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					nodes := bb.Hosts()
					var wg sync.WaitGroup
					fmt.Println("Length of nodes:", len(nodes))
					for _, node := range nodes {
						fmt.Println(node.Url.Host)
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
