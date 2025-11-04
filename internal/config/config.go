package config

import (
	"github.com/spf13/viper"
)

// Global 服务全局配置
type GlobalConfig struct {
	ListenAddr     string   `mapstructure:"listen_addr"`
	ReadTimeout    string   `mapstructure:"read_timeout"`
	WriteTimeout   string   `mapstructure:"write_timeout"`
	TrustedProxies []string `mapstructure:"trusted_proxies"`
}

// MiddlewareConfig 通用中间件配置（占位，后续扩展使用）
type MiddlewareConfig struct {
	Enabled bool           `mapstructure:"enabled"`
	Order   int            `mapstructure:"order"`
	Config  map[string]any `mapstructure:"config"`
}

// RouteConfig 单条路由规则
type RouteConfig struct {
	// 目前仅支持前缀匹配：如 "/api/users/**" 表示匹配以 /api/users/ 开头的所有路径
	Path    string   `mapstructure:"path"`
	Methods []string `mapstructure:"methods"`
	// 可选：将匹配到的前缀重写为该值（如将 /api/users/ 重写为 /users/）
	Rewrite string `mapstructure:"rewrite"`
	// Middlewares defines a list of middleware configurations for this specific route.
	Middlewares []map[string]any `mapstructure:"middlewares"`
}

// UpstreamConfig 上游服务配置
type UpstreamConfig struct {
	Name          string        `mapstructure:"name"`
	Scheme        string        `mapstructure:"scheme"`         // http 或 https，默认 http
	Hosts         []string      `mapstructure:"hosts"`          // 形如 ["localhost:8081", "localhost:8082"] 或带 scheme 的完整地址
	LoadBalancing string        `mapstructure:"load_balancing"` // round_robin/ip_hash（仅预留，当前实现 round_robin）
	HealthCheck   string        `mapstructure:"health_check"`   // 预留
	Routes        []RouteConfig `mapstructure:"routes"`
}

// ConfigSource 配置来源描述
type ConfigSource struct {
	Type     string `mapstructure:"type"`      // file|etcd|consul（当前实现 file）
	FilePath string `mapstructure:"file_path"` // 当 type=file 时生效
	Etcd     struct {
		Endpoints []string `mapstructure:"endpoints"`
		Key       string   `mapstructure:"key"`
		Watch     bool     `mapstructure:"watch"`
	} `mapstructure:"etcd"`
}

// GatewayConfig 网关完整配置
type GatewayConfig struct {
	Global       GlobalConfig                `mapstructure:"global"`
	Middlewares  map[string]MiddlewareConfig `mapstructure:"middlewares"`
	Upstreams    []UpstreamConfig            `mapstructure:"upstreams"`
	ConfigSource ConfigSource                `mapstructure:"config_source"`
}

// LoadConfig 读取并解析配置
func LoadConfig(path string) (*GatewayConfig, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var conf GatewayConfig
	if err := v.Unmarshal(&conf); err != nil {
		return nil, err
	}
	return &conf, nil
}

// 如需 etcd 动态配置，可在此补充 watch 能力
// func WatchEtcdConfig(conf *ConfigSource, onChange func(newConf *GatewayConfig)) {}
