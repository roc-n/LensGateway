package logging

import (
	"time"

	"github.com/rs/zerolog"
)

// Core information of a single structured request log entry.
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

// MarshalZerologObject allows zerolog to encode Entry as flat JSON fields.
func (e *Entry) MarshalZerologObject(enc *zerolog.Event) {
	enc.Time("timestamp", e.Timestamp)
	if e.Level != "" {
		enc.Str("level", e.Level)
	}
	if e.ClientIP != "" {
		enc.Str("client_ip", e.ClientIP)
	}
	if e.Method != "" {
		enc.Str("http_method", e.Method)
	}
	if e.Path != "" {
		enc.Str("http_path", e.Path)
	}
	if e.Status != 0 {
		enc.Int("http_status", e.Status)
	}
	if e.LatencyMs != 0 {
		enc.Int64("latency_ms", e.LatencyMs)
	}
	if e.UserAgent != "" {
		enc.Str("user_agent", e.UserAgent)
	}
	if e.RequestID != "" {
		enc.Str("request_id", e.RequestID)
	}
	// Gateway
	if e.Gateway.RoutePrefix != "" {
		enc.Str("route_prefix", e.Gateway.RoutePrefix)
	}
	if e.Gateway.UpstreamName != "" {
		enc.Str("upstream_name", e.Gateway.UpstreamName)
	}
	if e.Gateway.UpstreamNode != "" {
		enc.Str("upstream_node", e.Gateway.UpstreamNode)
	}
	// Auth
	if e.Auth.UserID != "" {
		enc.Str("user_id", e.Auth.UserID)
	}
	if e.Auth.Status != "" {
		enc.Str("auth_status", e.Auth.Status)
	}
	if e.Error != "" {
		enc.Str("error", e.Error)
	}
}
