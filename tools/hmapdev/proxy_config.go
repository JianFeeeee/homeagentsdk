package main

// ProxyConfig 是 plugin.json 里的反代声明项（工具链侧镜像，对应 sdk.ProxyDef）。
//
// 为什么不直接 import SDK 的 sdk.ProxyDef：工具链是**独立可执行**，
// 打包机可能只装了工具链而没有 SDK 源码树（旧版本常见）。镜像一份最小结构
// 能让 hmapdev 单机可用；两者的字段名必须保持一致，由
// tools/hmapdev/proxy_config_test.go 的对照测试钉住，防止单边漂移。
type ProxyConfig struct {
	Name      string `json:"name,omitempty"`
	Host      string `json:"host,omitempty"`
	Path      string `json:"path,omitempty"`
	StripPath bool   `json:"strip_path,omitempty"`
	Target    string `json:"target"`
	WebSocket bool   `json:"websocket,omitempty"`
	Auth      string `json:"auth,omitempty"`
}

// proxySchemaKeys 是 Proxies 序列化后允许出现的 JSON 键集合。
// writePluginJSON 用的是 map 重建，这里列清楚是为了让
// 「新增字段忘了同步」这件事在测试里立刻暴露。
var proxySchemaKeys = []string{"name", "host", "path", "strip_path", "target", "websocket", "auth"}
