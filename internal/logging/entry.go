package logging

import (
	"time"
)

// The core information of a single structured log entry.
// omitempty ensures that empty fields do not appear in the final JSON output, keeping logs concise.
type Entry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	ClientIP  string    `json:"client_ip"`
	Method    string    `json:"http_method"`
	Path      string    `json:"http_path"`
	Status    int       `json:"http_status"`
	LatencyMs int64     `json:"latency_ms"`
	UserAgent string    `json:"user_agent"`
	RequestID string    `json:"request_id"`

	Gateway struct {
		RoutePrefix  string `json:"route_prefix,omitempty"`
		UpstreamName string `json:"upstream_name,omitempty"`
		UpstreamNode string `json:"upstream_node,omitempty"`
	} `json:"gateway"`

	Auth struct {
		UserID string `json:"user_id,omitempty"`
		Status string `json:"status,omitempty"`
	} `json:"auth"`

	Error string `json:"error,omitempty"`
}
