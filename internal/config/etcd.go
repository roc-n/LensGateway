package config

import (
	context "context"
	"encoding/json"
	"errors"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// EtcdClient wraps etcd client for fetching and watching gateway upstream config.
type EtcdClient struct {
	cli *clientv3.Client
}

func NewEtcdClient(endpoints []string) (*EtcdClient, error) {
	cfg := clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: 5 * time.Second,
	}
	cli, err := clientv3.New(cfg)
	if err != nil {
		return nil, err
	}
	return &EtcdClient{cli: cli}, nil
}

// FetchUpstreams reads upstreams JSON from the given key and unmarshals to []UpstreamConfig.
// Expected JSON shape: {"upstreams": [ ... UpstreamConfig ... ]}
func (e *EtcdClient) FetchUpstreams(ctx context.Context, key string) ([]UpstreamConfig, error) {
	resp, err := e.cli.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if len(resp.Kvs) == 0 {
		return nil, errors.New("etcd: key not found")
	}
	var payload struct {
		Upstreams []UpstreamConfig `json:"upstreams"`
	}
	if err := json.Unmarshal(resp.Kvs[0].Value, &payload); err != nil {
		return nil, err
	}
	return payload.Upstreams, nil
}

// WatchUpstreams watches the key and calls onUpdate whenever upstreams change.
func (e *EtcdClient) WatchUpstreams(ctx context.Context, key string, onUpdate func([]UpstreamConfig)) error {
	w := e.cli.Watch(ctx, key)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-w:
			if !ok {
				return errors.New("etcd watch closed")
			}
			for _, evi := range ev.Events {
				if evi.Kv == nil {
					continue
				}
				var payload struct {
					Upstreams []UpstreamConfig `json:"upstreams"`
				}
				if err := json.Unmarshal(evi.Kv.Value, &payload); err == nil && payload.Upstreams != nil {
					onUpdate(payload.Upstreams)
				}
			}
		}
	}
}
