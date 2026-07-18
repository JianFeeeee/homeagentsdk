package main

import (
	"fmt"

	"gitcode.com/JianFeeeee/homeagent-sdk/sdk"
)

type Plugin struct {
	name string
	sdk  *sdk.PluginSDK
}

func (p *Plugin) Name() string { return p.name }

func (p *Plugin) Start(s *sdk.PluginSDK) error {
	p.sdk = s

	s.Settings().RegisterDef(sdk.ConfigDef{
		Key:         "plugin.testplugin.example",
		Default:     "hello",
		Type:        "string",
		DisplayName: "示例配置",
		Description: "An example configuration key",
		Category:    "testplugin",
	})

	tp := p.name + "_"
	s.RegisterTool(tp+"hello", sdk.ToolDef{
		Name:        tp + "hello",
		Description: "A hello world tool",
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}, p.handleHello)

	fmt.Printf("[%s] started\n", p.name)
	return nil
}

func (p *Plugin) Stop() error {
	fmt.Printf("[%s] stopped\n", p.name)
	return nil
}

func (p *Plugin) handleHello(args map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{
		"content": "Hello from testplugin plugin!",
	}, nil
}

// NewPluginFactory creates a Plugin instance. Called by both Linux entry (main.go)
// and Windows bridge (z_bridge_gen.go) to avoid naming conflict with C export.
func NewPluginFactory(name string, config map[string]interface{}) (sdk.Plugin, error) {
	return &Plugin{name: name}, nil
}
