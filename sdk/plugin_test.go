package sdk

import (
	"testing"
)

func TestRegisterStageGlobalDefault(t *testing.T) {
	called := false
	regStage := func(stage Stage, handler StageHandler) {
		called = true
	}
	s := &PluginSDK{regStage: regStage, name: "test"}
	s.RegisterStage(StageBeforeToolcall, func(ctx *StageContext) error { return nil })

	if !called {
		t.Error("global scope: handler not registered")
	}
}

func TestRegisterStageGlobalExplicit(t *testing.T) {
	called := false
	regStage := func(stage Stage, handler StageHandler) {
		called = true
	}
	s := &PluginSDK{regStage: regStage, name: "test"}
	s.RegisterStage(StageBeforeToolcall, func(ctx *StageContext) error { return nil }, StageScopeGlobal)

	if !called {
		t.Error("global scope: handler not registered")
	}
}

func TestRegisterStageOwnToolsMatch(t *testing.T) {
	var registered StageHandler
	regStage := func(stage Stage, handler StageHandler) {
		registered = handler
	}
	s := &PluginSDK{regStage: regStage, name: "myplugin"}
	s.RegisterStage(StageBeforeToolcall, func(ctx *StageContext) error { return nil }, StageScopeOwnTools)

	if registered == nil {
		t.Fatal("handler not registered")
	}

	ctx := &StageContext{}
	ctx.ToolCalls = []ToolCall{{Plugin: "myplugin", Name: "my_tool"}}
	ctx.ToolResults = nil

	err := registered(ctx)
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestRegisterStageOwnToolsSkipOtherPlugin(t *testing.T) {
	var registered StageHandler
	regStage := func(stage Stage, handler StageHandler) {
		registered = handler
	}
	s := &PluginSDK{regStage: regStage, name: "myplugin"}

	callCount := 0
	s.RegisterStage(StageBeforeToolcall, func(ctx *StageContext) error {
		callCount++
		return nil
	}, StageScopeOwnTools)

	if registered == nil {
		t.Fatal("handler not registered")
	}

	ctx := &StageContext{}
	ctx.ToolCalls = []ToolCall{{Plugin: "other", Name: "other_tool"}}

	err := registered(ctx)
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
	if callCount != 0 {
		t.Error("handler should not be called for other plugin's tool")
	}
}

func TestRegisterStageOwnToolsNonToolcallDegrades(t *testing.T) {
	regStage := func(stage Stage, handler StageHandler) {
		if stage != StagePreAction {
			t.Errorf("expected StagePreAction, got %s", stage)
		}
	}
	s := &PluginSDK{regStage: regStage, name: "test"}
	s.RegisterStage(StagePreAction, func(ctx *StageContext) error { return nil }, StageScopeOwnTools)
}

func TestRegisterStageOwnToolsStageBeforeToolcallNoToolCalls(t *testing.T) {
	var registered StageHandler
	regStage := func(stage Stage, handler StageHandler) {
		registered = handler
	}
	s := &PluginSDK{regStage: regStage, name: "myplugin"}

	callCount := 0
	s.RegisterStage(StageBeforeToolcall, func(ctx *StageContext) error {
		callCount++
		return nil
	}, StageScopeOwnTools)

	if registered == nil {
		t.Fatal("handler not registered")
	}

	ctx := &StageContext{}

	err := registered(ctx)
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
	if callCount != 0 {
		t.Error("handler should not be called when ToolCalls is empty")
	}
}

func TestRegisterStageOwnToolsStageAfterToolcallMatch(t *testing.T) {
	var registered StageHandler
	regStage := func(stage Stage, handler StageHandler) {
		registered = handler
	}
	s := &PluginSDK{regStage: regStage, name: "myplugin"}

	callCount := 0
	s.RegisterStage(StageAfterToolcall, func(ctx *StageContext) error {
		callCount++
		return nil
	}, StageScopeOwnTools)

	if registered == nil {
		t.Fatal("handler not registered")
	}

	ctx := &StageContext{}
	ctx.ToolResults = []ToolResult{{Plugin: "myplugin", Name: "my_tool"}}

	err := registered(ctx)
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
	if callCount != 1 {
		t.Error("handler should be called for own plugin's tool result")
	}
}

func TestRegisterStageOwnToolsStageAfterToolcallSkip(t *testing.T) {
	var registered StageHandler
	regStage := func(stage Stage, handler StageHandler) {
		registered = handler
	}
	s := &PluginSDK{regStage: regStage, name: "myplugin"}

	callCount := 0
	s.RegisterStage(StageAfterToolcall, func(ctx *StageContext) error {
		callCount++
		return nil
	}, StageScopeOwnTools)

	ctx := &StageContext{}
	ctx.ToolResults = []ToolResult{{Plugin: "other", Name: "other_tool"}}

	err := registered(ctx)
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
	if callCount != 0 {
		t.Error("handler should not be called for other plugin's tool result")
	}
}

func TestRegisterStageOwnToolsNilRegStage(t *testing.T) {
	s := &PluginSDK{name: "test"}
	s.RegisterStage(StageBeforeToolcall, func(ctx *StageContext) error { return nil }, StageScopeOwnTools)
}
