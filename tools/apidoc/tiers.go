package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Tier 是符号的可见级别。
const (
	TierPublic  = "public"  // 外部（第三方）插件可用
	TierBuiltin = "builtin" // 仅内核内置插件可用
	TierBridge  = "bridge"  // 桥接运行时注入的装配点（插件业务代码不调）
)

// tierTable 对应 tools/apidoc/tiers.json 的结构。
type tierTable struct {
	Public struct {
		Source  string   `json:"_source"`
		Methods []string `json:"methods"`
	} `json:"public"`
	Bridge struct {
		Doc     string   `json:"_doc"`
		Source  string   `json:"_source"`
		Methods []string `json:"methods"`
	} `json:"bridge"`
	TierOverrides map[string]json.RawMessage `json:"tier_overrides"`
	Interfaces    map[string]json.RawMessage `json:"interfaces"`
}

// applyTiers 把能力分层写回符号。分层表与源码一样是事实源：
// 找不到分层表就报错，不静默降级——否则文档会悄悄丢掉「仅内置」标记。
func applyTiers(p *Package) error {
	path := tierPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取 %s: %w", path, err)
	}
	var t tierTable
	if err := json.Unmarshal(data, &t); err != nil {
		return fmt.Errorf("解析 %s: %w", path, err)
	}

	pub := map[string]bool{}
	for _, m := range t.Public.Methods {
		pub[m] = true
	}
	brg := map[string]bool{}
	for _, m := range t.Bridge.Methods {
		brg[m] = true
	}

	// tier_overrides 里既有 "_doc" 这类说明键（值是数组），也有真正的裁定
	// （值是对象）。逐一解码，跳过 _ 开头的说明键。
	overrides := map[string]struct {
		Tier   string `json:"tier"`
		Reason string `json:"reason"`
	}{}
	for name, raw := range t.TierOverrides {
		if len(name) > 0 && name[0] == '_' {
			continue
		}
		var v struct {
			Tier   string `json:"tier"`
			Reason string `json:"reason"`
		}
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("tier_overrides[%s]: %w", name, err)
		}
		overrides[name] = v
	}

	// 接口级裁定先落到接口本身；其方法继承接口的 tier。
	ifaceTier := map[string]string{}
	for name, raw := range t.Interfaces {
		if len(name) > 0 && name[0] == '_' {
			continue
		}
		var v struct {
			Tier string `json:"tier"`
		}
		if err := json.Unmarshal(raw, &v); err != nil {
			return fmt.Errorf("interfaces[%s]: %w", name, err)
		}
		ifaceTier[name] = v.Tier
	}

	tierOf := func(s Symbol) (string, string) {
		// 1) 逐符号覆盖优先（它带 reason，最有信息量）。
		if o, ok := overrides[s.Name]; ok {
			return o.Tier, o.Reason
		}
		// 2) 接口方法：跟随接口的 tier。
		if s.Recv != "" {
			if tier, ok := ifaceTier[s.Recv]; ok {
				return tier, fmt.Sprintf("接口 %s 的层级裁定", s.Recv)
			}
		}
		// 3) 桥接注入点。
		if brg[s.Name] {
			return TierBridge, "桥接运行时注入点（由 hmapdev 生成的 proc_main 调用，插件业务代码不调用）"
		}
		// 4) 显式公开名单。
		if pub[s.Name] {
			return TierPublic, "桥接模板注入或 proc* 实现，外部插件可用"
		}
		// 5) 公开包里的方法默认公开；内部专有符号不会出现在本包里。
		if s.Recv == "PluginSDK" || s.Recv == "StageContext" {
			return TierPublic, "公开 SDK 的方法，未列入 bridge/内置清单"
		}
		return TierPublic, ""
	}

	for i := range p.Symbols {
		tier, reason := tierOf(p.Symbols[i])
		p.Symbols[i].Tier = tier
		p.Symbols[i].BuiltinOnly = tier == TierBuiltin
		p.Symbols[i].TierReason = reason
	}
	for i := range p.Interfaces {
		if tier, ok := ifaceTier[p.Interfaces[i].Name]; ok {
			for j := range p.Interfaces[i].Methods {
				p.Interfaces[i].Methods[j].Tier = tier
				p.Interfaces[i].Methods[j].BuiltinOnly = tier == TierBuiltin
			}
		}
	}
	return nil
}

// tierPath 找 tiers.json：先看可执行文件旁，再看源码目录，最后看工作目录。
// 这样 `go run ./tools/apidoc` 与编译后的二进制都能找到它。
func tierPath() string {
	candidates := []string{}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "tiers.json"))
	}
	candidates = append(candidates,
		filepath.Join("tools", "apidoc", "tiers.json"),
		"tiers.json",
	)
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return candidates[0]
}
