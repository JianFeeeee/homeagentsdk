package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writePluginJSON 是「白名单 map 重建」，新增字段若忘了同步进 map 会被静默丢弃。
// 本测试钉住这件事：声明了 proxies 的插件，产物 plugin.json 里必须还有 proxies。
func TestWritePluginJSONPreservesProxies(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	plg := &PlgConfig{
		Name:    "demo",
		NameZh:  "示例",
		NameEn:  "Demo",
		Version: "0.1.0",
		Author:  "test",
		Entry:   "plugin.bin",
		Tags:    []string{"demo"},
		Proxies: []ProxyConfig{
			{Name: "ui", Host: "demo", Target: "127.0.0.1:12100"},
			{Name: "gw", Host: "demo-gw", Target: "127.0.0.1:9890", WebSocket: true, Auth: "none"},
		},
	}
	writePluginJSON(plg, []string{"linux"}, "plugin.bin")

	raw, err := os.ReadFile(filepath.Join(dir, "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	proxies, ok := got["proxies"].([]interface{})
	if !ok {
		t.Fatalf("产物 plugin.json 丢了 proxies 字段，内容：%s", raw)
	}
	if len(proxies) != 2 {
		t.Fatalf("proxies 条数 = %d，期望 2", len(proxies))
	}
	first, _ := proxies[0].(map[string]interface{})
	if first["target"] != "127.0.0.1:12100" || first["host"] != "demo" {
		t.Errorf("第 1 条声明内容不对: %v", first)
	}
	second, _ := proxies[1].(map[string]interface{})
	if second["websocket"] != true || second["auth"] != "none" {
		t.Errorf("第 2 条声明丢了 websocket/auth: %v", second)
	}
}

// 没声明 proxies 时不应凭空冒出该字段（保持旧产物的字段集合不变）。
func TestWritePluginJSONOmitsEmptyProxies(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)

	writePluginJSON(&PlgConfig{Name: "nop", Entry: "plugin.bin"}, nil, "plugin.bin")
	raw, _ := os.ReadFile(filepath.Join(dir, "plugin.json"))
	if strings.Contains(string(raw), "proxies") {
		t.Errorf("未声明 proxies 却出现在产物里：%s", raw)
	}
}

// 字段漂移守卫：ProxyConfig 的 JSON 键必须与 proxySchemaKeys 完全一致。
// 加字段时若只改结构体不改这个清单，测试会红，提醒同步内核侧解析与文档。
func TestProxyConfigSchemaKeys(t *testing.T) {
	raw, err := json.Marshal(ProxyConfig{})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	// 空结构体 + omitempty 会全部省略，所以改用非零值探测。
	// 每个字段都必须出现在探测体里，否则「新增字段忘了同步」检测不出来。
	raw, _ = json.Marshal(ProxyConfig{
		Name: "n", Host: "h", Path: "/p", StripPath: true,
		Target: "t", WebSocket: true, Auth: "a",
	})
	m = nil
	_ = json.Unmarshal(raw, &m)
	if len(m) != len(proxySchemaKeys) {
		t.Fatalf("ProxyConfig 有 %d 个 JSON 键，proxySchemaKeys 列了 %d 个；请同步",
			len(m), len(proxySchemaKeys))
	}
	for _, k := range proxySchemaKeys {
		if _, ok := m[k]; !ok {
			t.Errorf("proxySchemaKeys 列了 %q，但 ProxyConfig 序列化后没有该键", k)
		}
	}
}

// validateProxies 必须让插件作者在**打包时**就发现写错的声明。
func TestValidateProxies(t *testing.T) {
	ok := [][]ProxyConfig{
		{{Target: "127.0.0.1:12100"}},
		{{Target: "http://127.0.0.1:12100", Host: "aaa"}},
		{{Target: "127.0.0.1:9890", Host: "devices", WebSocket: true, Auth: "none"}},
		{{Target: "https://example.com"}}, // 远程允许
	}
	for i, list := range ok {
		if err := validateProxies(list); err != nil {
			t.Errorf("合法集合 #%d 被拒: %v", i, err)
		}
	}
	bad := [][]ProxyConfig{
		{{Target: ""}},
		{{Target: "   "}},
		{{Target: "127.0.0.1:1", Auth: "nope"}},
		{{Target: "127.0.0.1:1", Host: "a_b"}},
		{{Target: "127.0.0.1:1", Host: "-x"}},
		{{Target: "127.0.0.1:1", Host: "X"}},
		{{Target: "127.0.0.1:notaport"}},
		{{Target: "127.0.0.1:99999"}},
		{{Target: "127.0.0.1:1", Host: "dup"}, {Target: "127.0.0.1:2", Host: "dup"}}, // 同包内重复
	}
	for i, list := range bad {
		if err := validateProxies(list); err == nil {
			t.Errorf("非法集合 #%d 应被拒: %+v", i, list)
		}
	}
}
