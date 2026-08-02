package main

// tmplPlgJSON is the plg.json template
const tmplPlgJSON = `{
  "name": "{{.Plg.Name}}",
  "name_zh": "{{.Plg.NameZh}}",
  "name_en": "{{.Plg.NameEn}}",
  "version": "{{.Plg.Version}}",
  "description": "{{.Plg.Description}}",
  "author": "{{.Plg.Author}}",
  "entry": "{{.Plg.Entry}}",
  "tags": [{{range $i, $t := .Plg.Tags}}{{if $i}}, {{end}}"{{$t}}"{{end}}],
  "targets": "{{.Plg.Targets}}"
}
`

const tmplGoMod = `module {{.ModulePath}}

go {{.GoVersion}}

require {{.SDKModule}} {{.SDKVersion}}
`

const tmplPluginGo = `package main

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
	s.RegisterStopHandler(func() { fmt.Printf("[%s] stop handler running\n", p.name) })
	s.Settings().RegisterDef(sdk.ConfigDef{
		Key: "plugin.{{.Plg.Name}}.example", Default: "hello", Type: "string",
		DisplayName: "示例配置", Description: "An example configuration key",
		Category: "{{.Plg.Name}}",
	})
	tp := p.name + "_"
	s.RegisterTool(tp+"hello", sdk.ToolDef{
		Name:        tp + "hello",
		Description: "A hello world tool",
		Parameters:  map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		NoMemory:    false, // 工具输出对 LLM 注意力有信号价值时为 false，纯操作工具为 true
		// Cleaner: func(output string) string {
		//     // 工具输出参与向量化/jieba/蒸馏前，在此过滤噪音
		//     return output
		// },
	}, p.handleHello)
	fmt.Printf("[%s] started\n", p.name)
	return nil
}

func (p *Plugin) Stop() error { fmt.Printf("[%s] stopped\n", p.name); return nil }

func (p *Plugin) handleHello(args map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"content": "Hello from {{.Plg.Name}} plugin!"}, nil
}

func NewPluginFactory(name string, config map[string]interface{}) (sdk.Plugin, error) {
	return &Plugin{name: name}, nil
}
`

const tmplSDKLua = `-- HomeAgent Lua Plugin SDK (standalone mock)
sdk = {}
function sdk.log(level, msg) print("[lua-plugin] " .. tostring(level) .. ": " .. tostring(msg)) end
function sdk.register_tool(name, def, handler) print("[lua-plugin] register_tool: " .. tostring(name)) end
function sdk.register_stage(stage, handler, scope) print("[lua-plugin] register_stage: " .. tostring(stage) .. " scope=" .. tostring(scope)) end
function sdk.register_api(name) print("[lua-plugin] register_api: " .. tostring(name)) end
function sdk.register_output_channel(name, caps, desc, def, handler) print("[lua-plugin] register_output_channel: " .. tostring(name)) end
function sdk.register_input_channel(name, def) print("[lua-plugin] register_input_channel: " .. tostring(name)) end
function sdk.get_setting(key) return nil end
function sdk.set_setting(key, value) print("[lua-plugin] set_setting: " .. tostring(key)) end
function sdk.inject_text(source, channel, text) print("[lua-plugin] inject_text: " .. tostring(source)) end
function sdk.inject_interrupt(source, channel, text) print("[lua-plugin] inject_interrupt: " .. tostring(source)) end
function sdk.inject_text_no_memory(source, channel, text) print("[lua-plugin] inject_text_no_memory: " .. tostring(source)) end
function sdk.set_auto_restart(enabled) print("[lua-plugin] set_auto_restart: " .. tostring(enabled)) end
sdk.memory = {}
function sdk.memory.recall(query, depth) return {entities={}, relations={}} end
function sdk.memory.commit(triples) return nil end
function sdk.memory.introspect() return {} end
function sdk.memory.merge(source, target) return 0 end
function sdk.memory.purge(criteria, hard) return 0 end
sdk.doc = {}
function sdk.doc.query(text, top_k) return {} end
function sdk.doc.insert(doc) return nil end
function sdk.doc.remove(id) return nil end
function sdk.doc.stats() return {} end
sdk.knowledge = {}
function sdk.knowledge.search(query, limit) return {} end
function sdk.knowledge.add(tag, content) return nil end
function sdk.knowledge.list() return {} end
sdk.text_memory = {}
function sdk.text_memory.append(evt) return nil end
sdk.llm = {}
function sdk.llm.list_sources() return {} end
function sdk.llm.set_source(name) return nil end
function sdk.llm.current_source() return nil end
sdk.social = {}
function sdk.social.get_person(name) return {} end
function sdk.social.get_network(name, depth) return {} end
function sdk.social.get_trait(name, trait) return {value=nil, found=false} end
function sdk.social.get_relations(name) return {} end
function sdk.social.list_persons() return {} end
sdk.settings = {}
function sdk.settings.get_core(key) return nil end
function sdk.settings.set_core(key, value) return nil end
function sdk.settings.list_core(prefix) return {} end
function sdk.settings.get_plugin(plugin, key) return nil end
function sdk.settings.set_plugin(plugin, key, value) return nil end
function sdk.settings.list_plugin(plugin, prefix) return {} end
function sdk.settings.list(prefix) return {} end
function sdk.settings.register_def(def) return nil end
function sdk.settings.defs(prefix) return {} end
function sdk.settings.dump() return {} end
function sdk.settings.plugins() return {} end
sdk.json = {}
function sdk.json.encode(val)
    if type(val) == "string" then return '"' .. val:gsub('"', '\\"'):gsub('\n', '\\n') .. '"'
    elseif type(val) == "number" or type(val) == "boolean" then return tostring(val)
    elseif type(val) == "table" then local parts, i = {}, 1
        for k, v in pairs(val) do parts[i] = sdk.json.encode(k) .. ":" .. sdk.json.encode(v); i = i + 1 end
        return "{" .. table.concat(parts, ",") .. "}" end
    return "null"
end
function sdk.json.decode(str) local ok, fn = pcall(load, "return " .. str); if ok then return fn() end; return nil end
sdk.http = {}
function sdk.http.get(url) print("[lua-plugin] http.get: " .. tostring(url)); return {status=200, body='{"mock":true}', headers={}} end
function sdk.http.post(url, body, ct) print("[lua-plugin] http.post: " .. tostring(url)); return {status=200, body='{"mock":true}', headers={}} end
return sdk
`

const tmplMainLua = `-- {{.Plg.Name}} plugin
local plugin = { name = "{{.Plg.Name}}" }
function plugin.start(sdk)
  sdk.log("info", "{{.Plg.Name}} starting...")
  sdk.register_tool("{{.Plg.Name}}_hello", {
    description = "A hello world tool",
    parameters = { type = "object", properties = {} }
  }, function(args) return { content = "Hello from {{.Plg.Name}} plugin!" } end)
  sdk.log("info", "{{.Plg.Name}} started")
end
function plugin.stop() sdk.log("info", "{{.Plg.Name}} stopped") end
return plugin
`

// tmplBridge — Windows DLL C ABI bridge (unchanged)
const tmplBridge = `//go:build windows && cgo

package main

/*
#include <stdlib.h>
*/
import "C"
import (
	"encoding/json"
	"sync"
	"unsafe"
	sdk "gitcode.com/JianFeeeee/homeagent-sdk/sdk"
)

var (
	mu        sync.Mutex
	handleMap = map[unsafe.Pointer]*bridgeState{}
)

type bridgeState struct {
	plugin   sdk.Plugin
	toolDefs map[string]sdk.ToolDef
	handlers map[string]sdk.ToolHandler
	stages   map[string]sdk.StageHandler
	settings map[string]interface{}
	sdk      *sdk.PluginSDK
}

func newHandle(plg sdk.Plugin) unsafe.Pointer {
	mu.Lock(); defer mu.Unlock()
	h := C.malloc(C.size_t(1))
	handleMap[h] = &bridgeState{
		plugin: plg, toolDefs: make(map[string]sdk.ToolDef),
		handlers: make(map[string]sdk.ToolHandler), stages: make(map[string]sdk.StageHandler),
		settings: make(map[string]interface{}),
	}
	return h
}
func getState(h unsafe.Pointer) *bridgeState { mu.Lock(); defer mu.Unlock(); return handleMap[h] }
func delState(h unsafe.Pointer) { mu.Lock(); defer mu.Unlock(); delete(handleMap, h); C.free(h) }

//export NewPlugin
func NewPlugin(name *C.char, configJSON *C.char) unsafe.Pointer {
	goName := C.GoString(name)
	var config map[string]interface{}
	if configJSON != nil {
		var wrapper map[string]interface{}
		if err := json.Unmarshal([]byte(C.GoString(configJSON)), &wrapper); err == nil {
			if c, ok := wrapper["config"].(map[string]interface{}); ok { config = c }
		}
	}
	plg, err := NewPluginFactory(goName, config)
	if err != nil { return nil }
	return newHandle(plg)
}

//export StartPlugin
func StartPlugin(handle unsafe.Pointer) C.int {
	bs := getState(handle)
	if bs == nil { return 1 }
	mockSett := &bridgeSettings{data: bs.settings}
	mockSDK := sdk.New(bs.plugin.Name(), mockSett,
		func(name string, def sdk.ToolDef, handler sdk.ToolHandler) error {
			bs.toolDefs[name] = def; bs.handlers[name] = handler; return nil
		},
		func(stage sdk.Stage, handler sdk.StageHandler) { bs.stages[string(stage)] = handler },
		func(name string) error { return nil },
		func(name string, caps int, desc string, def sdk.ChannelDef, handler sdk.ToolHandler) error { return nil },
	)
	mockSDK.SetInputChannelRegistrar(func(name string, def sdk.ChannelDef) error { return nil })
	bs.sdk = mockSDK
	if err := bs.plugin.Start(mockSDK); err != nil { return 1 }
	return 0
}

//export StopPlugin
func StopPlugin(handle unsafe.Pointer) C.int {
	bs := getState(handle)
	if bs == nil { return 1 }
	if bs.sdk != nil {
		bs.sdk.RunStopHandlers()
	}
	if err := bs.plugin.Stop(); err != nil { return 1 }
	return 0
}

//export DestroyPlugin
func DestroyPlugin(handle unsafe.Pointer) {
	if bs := getState(handle); bs != nil { delState(handle) }
}

//export GetToolDefsJSON
func GetToolDefsJSON(handle unsafe.Pointer) *C.char {
	bs := getState(handle)
	if bs == nil { return nil }
	defs := make([]sdk.ToolDef, 0, len(bs.toolDefs))
	for _, def := range bs.toolDefs { defs = append(defs, def) }
	b, _ := json.Marshal(defs)
	return C.CString(string(b))
}

//export InvokeToolJSON
func InvokeToolJSON(handle unsafe.Pointer, toolName *C.char, argsJSON *C.char) *C.char {
	bs := getState(handle)
	if bs == nil || toolName == nil { return nil }
	goName := C.GoString(toolName)
	handler, ok := bs.handlers[goName]
	if !ok { errMsg, _ := json.Marshal(map[string]interface{}{"error": "tool not found: " + goName}); return C.CString(string(errMsg)) }
	var args map[string]interface{}
	if argsJSON != nil { json.Unmarshal([]byte(C.GoString(argsJSON)), &args) }
	r, err := handler(args)
	if err != nil { errMsg, _ := json.Marshal(map[string]interface{}{"error": err.Error()}); return C.CString(string(errMsg)) }
	b, _ := json.Marshal(r)
	return C.CString(string(b))
}

//export GetStagesJSON
func GetStagesJSON(handle unsafe.Pointer) *C.char {
	bs := getState(handle)
	if bs == nil { return nil }
	type se struct { Stage string ` + "`" + `json:"stage"` + "`" + ` }
	var entries []se
	for s := range bs.stages { entries = append(entries, se{s}) }
	b, _ := json.Marshal(entries)
	return C.CString(string(b))
}

//export InvokeStage
func InvokeStage(handle unsafe.Pointer, stage *C.char, contextJSON *C.char) C.int {
	bs := getState(handle)
	if bs == nil || stage == nil { return 1 }
	goStage := C.GoString(stage)
	handler, ok := bs.stages[goStage]
	if !ok { return 1 }
	var ctx map[string]interface{}
	if contextJSON != nil { json.Unmarshal([]byte(C.GoString(contextJSON)), &ctx) }
	sc := &sdk.StageContext{}
	if ctx != nil {
		if v, ok := ctx["raw_message"].(string); ok { sc.RawMessage = v }
		if v, ok := ctx["user_id"].(string); ok { sc.UserID = v }
		if v, ok := ctx["phase"].(string); ok { sc.Phase = sdk.Stage(v) }
	}
	if err := handler(sc); err != nil { return 1 }
	return 0
}

//export FreeCString
func FreeCString(s *C.char) { C.free(unsafe.Pointer(s)) }

type bridgeSettings struct{ data map[string]interface{} }
func (s *bridgeSettings) Get(key string) (interface{}, error) { v, ok := s.data[key]; if !ok { return nil, nil }; return v, nil }
func (s *bridgeSettings) Set(key string, value interface{}) error { s.data[key] = value; return nil }
func (s *bridgeSettings) List(prefix string) ([]string, error) {
	var keys []string
	for k := range s.data { if len(k) >= len(prefix) && k[:len(prefix)] == prefix { keys = append(keys, k) } }
	return keys, nil
}
func (s *bridgeSettings) GetCore(key string) (interface{}, error) { return nil, nil }
func (s *bridgeSettings) SetCore(key string, value interface{}) error { return nil }
func (s *bridgeSettings) ListCore(prefix string) ([]string, error) { return nil, nil }
func (s *bridgeSettings) GetPlugin(plugin, key string) (interface{}, error) { return nil, nil }
func (s *bridgeSettings) SetPlugin(plugin, key string, value interface{}) error { return nil }
func (s *bridgeSettings) ListPlugin(plugin, prefix string) ([]string, error) { return nil, nil }
func (s *bridgeSettings) RegisterDef(def sdk.ConfigDef) {}
func (s *bridgeSettings) Defs(prefix string) []*sdk.ConfigDef { return nil }
func (s *bridgeSettings) Dump() map[string]interface{} { return s.data }
func (s *bridgeSettings) Plugins() []string { return nil }

func main() {}
`

// tmplCABIHeader — shared C ABI type definitions for both core and plugin
// 此模板中的常量应与 core/internal/meta/meta.go 保持一致（ABI 版本、dispatch method IDs）。
const tmplCABIHeader = `
#ifndef HOMEAGENT_CABI_H
#define HOMEAGENT_CABI_H
// HOMEAGENT_ABI_VERSION 与 sdk/meta/meta.go ABIVersion 同步
#define HOMEAGENT_ABI_VERSION 1
#ifdef __cplusplus
extern "C" {
#endif

// PluginAPI — implemented by the plugin, called by the core
typedef struct {
    int version; int version_min;
    int (*init_plugin)(char*, char*, char**);
    int (*start_plugin)(void*, int, char**);
    int (*stop_plugin)(char**);
    int (*invoke_tool)(char*, char*, char**, char**);
    int (*invoke_stage)(char*, char*, char**);
    int (*invoke_output)(char*, char*, char*, char**);
    void (*free_string)(char*);
} PluginAPI;

// CoreAPI — implemented by the core, passed to plugin via start_plugin
// Uses single dispatch function to avoid function pointer ABI issues
typedef struct {
    int version; int version_min;
    int (*dispatch)(int method_id, void* ctx, char* s1, char* s2, char* s3, int i1, int i2, char** result, char** error);
    void* ctx;
} CoreAPI;

// Dispatch method IDs (plugin→core SDK calls)
enum {
    CORE_REGISTER_TOOL        = 1,
    CORE_REGISTER_STAGE       = 2,
    CORE_REGISTER_OUTPUT_CH   = 3,
    CORE_REGISTER_PLUGIN_API  = 4,
    CORE_INJECT_TEXT          = 5,
    CORE_INJECT_INTERRUPT_TEXT = 6,
    CORE_INJECT_TEXT_NO_MEMORY = 7,
    CORE_SET_AUTO_RESTART     = 8,
    CORE_MEMORY_RECALL        = 9,
    CORE_MEMORY_COMMIT        = 10,
    CORE_MEMORY_INTROSPECT    = 11,
    CORE_MEMORY_MERGE         = 12,
    CORE_MEMORY_PURGE         = 13,
    CORE_DOC_QUERY            = 14,
    CORE_KNOWLEDGE_SEARCH     = 15,
    CORE_SETTINGS_GET         = 16,
    CORE_SETTINGS_SET         = 17,
    CORE_SETTINGS_REGISTER_DEF = 18,
    CORE_LLM_LIST_SOURCES     = 19,
    CORE_LLM_SET_SOURCE       = 20,
    CORE_SOCIAL_GET_PERSON    = 21,
    CORE_SOCIAL_GET_NETWORK   = 22,
    CORE_SUBSCRIBE            = 23,
    CORE_UNSUBSCRIBE          = 24,
    CORE_FREE_STRING          = 25,
    CORE_SETTINGS_GET_CORE     = 26,
    CORE_SETTINGS_SET_CORE     = 27,
    CORE_SETTINGS_LIST_CORE    = 28,
    CORE_SETTINGS_GET_PLUGIN   = 29,
    CORE_SETTINGS_SET_PLUGIN   = 30,
    CORE_SETTINGS_LIST_PLUGIN  = 31,
    CORE_DOC_INSERT            = 32,
    CORE_DOC_REMOVE            = 33,
    CORE_DOC_STATS             = 34,
    CORE_KNOWLEDGE_ADD         = 35,
    CORE_KNOWLEDGE_LIST        = 36,
    CORE_LLM_CURRENT_SOURCE    = 37,
    CORE_SOCIAL_GET_TRAIT      = 38,
    CORE_SOCIAL_GET_RELATIONS  = 39,
    CORE_SOCIAL_LIST_PERSONS   = 40,
    CORE_TEXT_MEMORY_APPEND    = 41,
    CORE_SETTINGS_LIST         = 42,
    CORE_SETTINGS_DEFS         = 43,
    CORE_SETTINGS_DUMP         = 44,
    CORE_SETTINGS_PLUGINS      = 45,
    CORE_REGISTER_INPUT_CH     = 46,
};

#ifdef __cplusplus
}
#endif
#endif
`

// tmplLinuxBridge — auto-generated Go bridge for Linux c-shared builds.
// Called by plugin's Start() with a PluginSDK that wraps CoreAPI dispatch.
// PluginSDK calls go through C ABI → CoreAPI dispatch → core's Go PluginSDK.
const tmplLinuxBridge = `package main

/*
#include <stdlib.h>
int ha_dispatch(int method_id, void* core_api, char* s1, char* s2, char* s3, int i1, int i2, char** result, char** error);
*/
import "C"
import (
	"encoding/json"
	"fmt"
	"sync"
	"unsafe"
	sdk "gitcode.com/JianFeeeee/homeagent-sdk/sdk"
)

// ---- global state ----

var (
	mu         sync.Mutex
	currentPlg sdk.Plugin
	currentSDK *sdk.PluginSDK
	coreAPI    unsafe.Pointer

	handlerMu   sync.RWMutex
	coreAPIMu   sync.RWMutex
	toolHandlers  = map[string]sdk.ToolHandler{}
	stageHandlers = map[string]sdk.StageHandler{}
	outputHandlers = map[string]sdk.ToolHandler{}
)

// ---- CoreAPI dispatch helpers ----

func callVoid(methodID int, s1, s2, s3 string, i1, i2 int) error {
	coreAPIMu.RLock()
	api := coreAPI
	coreAPIMu.RUnlock()
	var c1, c2, c3 *C.char
	if s1 != "" { c1 = C.CString(s1); defer C.free(unsafe.Pointer(c1)) }
	if s2 != "" { c2 = C.CString(s2); defer C.free(unsafe.Pointer(c2)) }
	if s3 != "" { c3 = C.CString(s3); defer C.free(unsafe.Pointer(c3)) }
	var cErr *C.char
	if C.ha_dispatch(C.int(methodID), api, c1, c2, c3, C.int(i1), C.int(i2), nil, &cErr) != 0 && cErr != nil {
		return fmt.Errorf("%s", C.GoString(cErr))
	}
	return nil
}

func callString(methodID int, s1, s2, s3 string, i1, i2 int) (string, error) {
	coreAPIMu.RLock()
	api := coreAPI
	coreAPIMu.RUnlock()
	var c1, c2, c3 *C.char
	if s1 != "" { c1 = C.CString(s1); defer C.free(unsafe.Pointer(c1)) }
	if s2 != "" { c2 = C.CString(s2); defer C.free(unsafe.Pointer(c2)) }
	if s3 != "" { c3 = C.CString(s3); defer C.free(unsafe.Pointer(c3)) }
	var strResult, cErr *C.char
	if C.ha_dispatch(C.int(methodID), api, c1, c2, c3, C.int(i1), C.int(i2), &strResult, &cErr) != 0 && cErr != nil {
		return "", fmt.Errorf("%s", C.GoString(cErr))
	}
	if strResult != nil {
		result := C.GoString(strResult)
		C.ha_dispatch(C.int(25), api, strResult, nil, nil, 0, 0, nil, nil)
		return result, nil
	}
	return "", nil
}

// ---- buildPluginSDK: PluginSDK backed by CoreAPI dispatch ----
//   - ALL SDK methods route through C ABI → CoreAPI → core's PluginSDK
//   - Handlers for tools/stages/output are stored locally AND registered via dispatch

func buildPluginSDK(name string) *sdk.PluginSDK {
	sett := &dispatchSettings{}
	base := sdk.New(name, sett,
		func(toolName string, def sdk.ToolDef, handler sdk.ToolHandler) error {
			handlerMu.Lock()
			toolHandlers[toolName] = handler
			handlerMu.Unlock()
			b, _ := json.Marshal(def)
			return callVoid(1, toolName, string(b), "", 0, 0)
		},
		func(stage sdk.Stage, handler sdk.StageHandler) {
			handlerMu.Lock()
			stageHandlers[string(stage)] = handler
			handlerMu.Unlock()
			callVoid(2, string(stage), "", "", 0, 0)
		},
		func(name string) error { return callVoid(4, name, "", "", 0, 0) },
		func(name string, caps int, desc string, def sdk.ChannelDef, handler sdk.ToolHandler) error {
			handlerMu.Lock()
			outputHandlers[name] = handler
			handlerMu.Unlock()
			defJSON, _ := json.Marshal(def)
			return callVoid(3, name, desc, string(defJSON), caps, 0)
		},
	)
	base.SetIOInjector(dispatchIO{})
	base.SetMemoryAPI(dispatchMemory{})
	base.SetDocMemoryAPI(dispatchDocMemory{})
	base.SetKnowledgeAPI(dispatchKnowledge{})
	base.SetLLMAPI(dispatchLLM{})
	base.SetSocialAPI(dispatchSocial{})
	base.SetTextMemoryAPI(dispatchTextMemory{})
	base.SetInputChannelRegistrar(
		func(name string, def sdk.ChannelDef) error {
			defJSON, _ := json.Marshal(def)
			return callVoid(46, name, string(defJSON), "", 0, 0)
		},
	)
	return base
}

// ---- dispatch IO (inline definitions) ----

type dispatchIO struct{}
func (dispatchIO) InjectInterruptText(s, c, t string) { callVoid(6, s, c, t, 0, 0) }
func (dispatchIO) InjectText(s, c, t string)           { callVoid(5, s, c, t, 0, 0) }
func (dispatchIO) InjectTextNoMemory(s, c, t string)   { callVoid(7, s, c, t, 0, 0) }

type dispatchMemory struct{}
func (dispatchMemory) Recall(q []string, d int) ([]sdk.Entity, []sdk.Relation, error) {
	b, _ := json.Marshal(q); r, e := callString(9, string(b), "", "", d, 0)
	if e != nil || r == "" { return nil, nil, e }
	var v struct{ Entities []sdk.Entity; Relations []sdk.Relation }
	if e = json.Unmarshal([]byte(r), &v); e != nil { return nil, nil, e }
	if v.Entities == nil { v.Entities = []sdk.Entity{} }
	if v.Relations == nil { v.Relations = []sdk.Relation{} }
	return v.Entities, v.Relations, nil
}
func (dispatchMemory) Commit(t []sdk.Triple) error { b, _ := json.Marshal(t); return callVoid(10, string(b), "", "", 0, 0) }
func (dispatchMemory) Introspect() (map[string]interface{}, error) { r, e := callString(11, "", "", "", 0, 0); if e != nil || r == "" { return nil, e }; var m map[string]interface{}; return m, json.Unmarshal([]byte(r), &m) }
func (dispatchMemory) MergeEntities(s, t string) (int, error) { return 1, callVoid(12, s, t, "", 0, 0) }
func (dispatchMemory) Purge(c map[string]string, m string) (int, error) { b, _ := json.Marshal(c); i := 0; if m == "hard" { i = 1 }; return 1, callVoid(13, string(b), "", "", i, 0) }

type dispatchDocMemory struct{}
func (dispatchDocMemory) Query(t string, k int) []*sdk.Doc { r, e := callString(14, t, "", "", k, 0); if e != nil || r == "" { return nil }; var d []*sdk.Doc; json.Unmarshal([]byte(r), &d); return d }
func (dispatchDocMemory) Insert(doc *sdk.Doc) error { b, _ := json.Marshal(doc); return callVoid(32, string(b), "", "", 0, 0) }
func (dispatchDocMemory) Remove(id string) { callVoid(33, id, "", "", 0, 0) }
func (dispatchDocMemory) Stats() map[string]interface{} { r, e := callString(34, "", "", "", 0, 0); if e != nil || r == "" { return nil }; var m map[string]interface{}; json.Unmarshal([]byte(r), &m); return m }

type dispatchKnowledge struct{}
func (dispatchKnowledge) Search(q string, k int) ([]*sdk.Knowledge, error) { r, e := callString(15, q, "", "", k, 0); if e != nil || r == "" { return nil, e }; var v []*sdk.Knowledge; return v, json.Unmarshal([]byte(r), &v) }
func (dispatchKnowledge) Add(n, c string) error { return callVoid(35, n, c, "", 0, 0) }
func (dispatchKnowledge) List() ([]string, error) { r, e := callString(36, "", "", "", 0, 0); if e != nil || r == "" { return nil, e }; var v []string; return v, json.Unmarshal([]byte(r), &v) }

type dispatchLLM struct{}
func (dispatchLLM) ListSources() []string { r, e := callString(19, "", "", "", 0, 0); if e != nil || r == "" { return nil }; var v []string; json.Unmarshal([]byte(r), &v); return v }
func (dispatchLLM) SetSource(n string) error { return callVoid(20, n, "", "", 0, 0) }
func (dispatchLLM) CurrentSource() string { r, e := callString(37, "", "", "", 0, 0); if e != nil || r == "" { return "" }; return r }

type dispatchSocial struct{}
func (dispatchSocial) GetPerson(n string) (*sdk.PersonProfile, error) { r, e := callString(21, n, "", "", 0, 0); if e != nil || r == "" { return nil, e }; var v sdk.PersonProfile; return &v, json.Unmarshal([]byte(r), &v) }
func (dispatchSocial) GetTrait(n, t string) (string, bool) { r, e := callString(38, n, t, "", 0, 0); if e != nil || r == "" { return "", false }; var m map[string]interface{}; json.Unmarshal([]byte(r), &m); v, _ := m["value"].(string); ok, _ := m["found"].(bool); return v, ok }
func (dispatchSocial) GetRelations(name string) ([]sdk.SocialRelation, error) { r, e := callString(39, name, "", "", 0, 0); if e != nil || r == "" { return nil, e }; var v []sdk.SocialRelation; return v, json.Unmarshal([]byte(r), &v) }
func (dispatchSocial) GetNetwork(n string, d int) ([]*sdk.PersonProfile, error) { r, e := callString(22, n, "", "", d, 0); if e != nil || r == "" { return nil, e }; var v []*sdk.PersonProfile; return v, json.Unmarshal([]byte(r), &v) }
func (dispatchSocial) ListPersons() ([]string, error) { r, e := callString(40, "", "", "", 0, 0); if e != nil || r == "" { return nil, e }; var v []string; return v, json.Unmarshal([]byte(r), &v) }

type dispatchTextMemory struct{}
func (dispatchTextMemory) Append(evt sdk.TextEvent) error { b, _ := json.Marshal(evt); return callVoid(41, string(b), "", "", 0, 0) }

// ---- dispatchSettings (inline) ----

type dispatchSettings struct{}
func (d *dispatchSettings) Get(key string) (interface{}, error) {
	r, e := callString(16, key, "", "", 0, 0); if e != nil || r == "" { return nil, e }; var v interface{}; return v, json.Unmarshal([]byte(r), &v)
}
func (d *dispatchSettings) Set(key string, value interface{}) error {
	b, _ := json.Marshal(value); return callVoid(17, key, string(b), "", 0, 0)
}
func (d *dispatchSettings) RegisterDef(def sdk.ConfigDef) { b, _ := json.Marshal(def); callVoid(18, string(b), "", "", 0, 0) }
func (d *dispatchSettings) List(prefix string) ([]string, error) {
	r, e := callString(42, prefix, "", "", 0, 0); if e != nil || r == "" { return nil, e }; var v []string; return v, json.Unmarshal([]byte(r), &v)
}
func (d *dispatchSettings) GetCore(key string) (interface{}, error) {
	r, e := callString(26, key, "", "", 0, 0); if e != nil || r == "" { return nil, e }; var v interface{}; return v, json.Unmarshal([]byte(r), &v)
}
func (d *dispatchSettings) SetCore(key string, value interface{}) error {
	b, _ := json.Marshal(value); return callVoid(27, key, string(b), "", 0, 0)
}
func (d *dispatchSettings) ListCore(prefix string) ([]string, error) {
	r, e := callString(28, prefix, "", "", 0, 0); if e != nil || r == "" { return nil, e }; var v []string; return v, json.Unmarshal([]byte(r), &v)
}
func (d *dispatchSettings) GetPlugin(plugin, key string) (interface{}, error) {
	r, e := callString(29, plugin, key, "", 0, 0); if e != nil || r == "" { return nil, e }; var v interface{}; return v, json.Unmarshal([]byte(r), &v)
}
func (d *dispatchSettings) SetPlugin(plugin, key string, value interface{}) error {
	b, _ := json.Marshal(value); return callVoid(30, plugin, key, string(b), 0, 0)
}
func (d *dispatchSettings) ListPlugin(plugin, prefix string) ([]string, error) {
	r, e := callString(31, plugin, prefix, "", 0, 0); if e != nil || r == "" { return nil, e }; var v []string; return v, json.Unmarshal([]byte(r), &v)
}
func (d *dispatchSettings) Defs(prefix string) []*sdk.ConfigDef {
	r, e := callString(43, prefix, "", "", 0, 0); if e != nil || r == "" { return nil }; var v []*sdk.ConfigDef; json.Unmarshal([]byte(r), &v); return v
}
func (d *dispatchSettings) Dump() map[string]interface{} {
	r, e := callString(44, "", "", "", 0, 0); if e != nil || r == "" { return nil }; var m map[string]interface{}; json.Unmarshal([]byte(r), &m); return m
}
func (d *dispatchSettings) Plugins() []string {
	r, e := callString(45, "", "", "", 0, 0); if e != nil || r == "" { return nil }; var v []string; json.Unmarshal([]byte(r), &v); return v
}

// ---- Go callbacks (called from z_entry.c via C) ----

//export go_init_plugin
func go_init_plugin(name *C.char, configJSON *C.char, errorOut **C.char) C.int {
	plg, err := NewPluginFactory(C.GoString(name), nil)
	if err != nil || plg == nil {
		if err != nil { *errorOut = C.CString(err.Error()) } else { *errorOut = C.CString("NewPluginFactory returned nil") }
		return 1
	}
	mu.Lock(); currentPlg = plg; mu.Unlock()
	_ = configJSON
	return 0
}

//export go_start_plugin
func go_start_plugin(coreAPIptr unsafe.Pointer, coreVersion C.int, errorOut **C.char) C.int {
	mu.Lock()
	plg := currentPlg
	coreAPIMu.Lock()
	coreAPI = coreAPIptr
	coreAPIMu.Unlock()
	mu.Unlock()
	_ = coreVersion
	if plg == nil { *errorOut = C.CString("not initialized"); return 1 }
	sdk := buildPluginSDK(plg.Name())
	mu.Lock(); currentSDK = sdk; mu.Unlock()
	if err := plg.Start(sdk); err != nil { *errorOut = C.CString(err.Error()); return 1 }
	return 0
}

//export go_stop_plugin
func go_stop_plugin(errorOut **C.char) C.int {
	mu.Lock()
	plg := currentPlg
	sdk := currentSDK
	currentPlg = nil
	currentSDK = nil
	coreAPIMu.Lock()
	coreAPI = nil
	coreAPIMu.Unlock()
	mu.Unlock()
	if sdk != nil {
		sdk.RunStopHandlers()
	}
	if plg != nil {
		if err := plg.Stop(); err != nil { *errorOut = C.CString(err.Error()); return 1 }
	}
	return 0
}

//export go_invoke_tool
func go_invoke_tool(name *C.char, argsJSON *C.char, resultOut **C.char, errorOut **C.char) C.int {
	goName := C.GoString(name)
	handlerMu.RLock()
	h, ok := toolHandlers[goName]
	handlerMu.RUnlock()
	if !ok { *errorOut = C.CString("tool not found"); return 1 }
	var args map[string]interface{}
	if argsJSON != nil { json.Unmarshal([]byte(C.GoString(argsJSON)), &args) }
	r, err := h(args)
	if err != nil { *errorOut = C.CString(err.Error()); return 1 }
	b, _ := json.Marshal(r)
	*resultOut = C.CString(string(b))
	return 0
}

//export go_invoke_stage
func go_invoke_stage(stage *C.char, ctxJSON *C.char, errorOut **C.char) C.int {
	goStage := C.GoString(stage)
	handlerMu.RLock()
	h, ok := stageHandlers[goStage]
	handlerMu.RUnlock()
	if !ok { return 0 }
	sc := &sdk.StageContext{}
	if ctxJSON != nil {
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(C.GoString(ctxJSON)), &m); err == nil {
			if v, _ := m["raw_message"].(string); v != "" { sc.RawMessage = v }
			if v, _ := m["user_id"].(string); v != "" { sc.UserID = v }
			if v, _ := m["group_id"].(string); v != "" { sc.GroupID = v }
			if v, _ := m["phase"].(string); v != "" { sc.Phase = sdk.Stage(v) }
			if v, _ := m["llm_text"].(string); v != "" { sc.LLMText = v }
			if v, _ := m["final_text"].(string); v != "" { sc.FinalText = v }
			if v, _ := m["no_memory"].(bool); v { sc.NoMemory = true }
			if v, _ := m["response"].(string); v != "" { sc.Response = &v }
			if v, _ := m["tool_calls"].([]interface{}); len(v) > 0 {
				b, _ := json.Marshal(v); json.Unmarshal(b, &sc.ToolCalls)
			}
			if v, _ := m["tool_results"].([]interface{}); len(v) > 0 {
				b, _ := json.Marshal(v); json.Unmarshal(b, &sc.ToolResults)
			}
		}
	}
	if err := h(sc); err != nil { *errorOut = C.CString(err.Error()); return 1 }
	return 0
}

//export go_invoke_output
func go_invoke_output(channel *C.char, msgType *C.char, payloadJSON *C.char, errorOut **C.char) C.int {
	goChan := C.GoString(channel)
	handlerMu.RLock()
	h, ok := outputHandlers[goChan]
	handlerMu.RUnlock()
	if !ok { return 0 }
	// payloadJSON contains the full args JSON from output_send (e.g. {"content":"...","user_id":123})
	var args map[string]interface{}
	if payloadJSON != nil {
		json.Unmarshal([]byte(C.GoString(payloadJSON)), &args)
	}
	if _, err := h(args); err != nil { *errorOut = C.CString(err.Error()); return 1 }
	return 0
}

//export go_free_string
func go_free_string(ptr *C.char) { C.free(unsafe.Pointer(ptr)) }

func main() {}
`

// tmplPluginInitC — C entry point for the plugin .so file.
// Contains PluginAPI, CoreAPI (single dispatch), and ha_dispatch bridge.
const tmplPluginInitC = `#include <stdlib.h>
#include <string.h>

#define HOMEAGENT_ABI_VERSION 1

typedef struct {
    int version; int version_min;
    int (*init_plugin)(char*, char*, char**);
    int (*start_plugin)(void*, int, char**);
    int (*stop_plugin)(char**);
    int (*invoke_tool)(char*, char*, char**, char**);
    int (*invoke_stage)(char*, char*, char**);
    int (*invoke_output)(char*, char*, char*, char**);
    void (*free_string)(char*);
} PluginAPI;

typedef struct {
    int version; int version_min;
    int (*dispatch)(int, void*, char*, char*, char*, int, int, char**, char**);
    void* ctx;
} CoreAPI;

extern int go_init_plugin(char*, char*, char**);
extern int go_start_plugin(void*, int, char**);
extern int go_stop_plugin(char**);
extern int go_invoke_tool(char*, char*, char**, char**);
extern int go_invoke_stage(char*, char*, char**);
extern int go_invoke_output(char*, char*, char*, char**);
extern void go_free_string(char*);

int c_init_plugin(char* n, char* c, char** e) { return go_init_plugin(n, c, e); }
int c_start_plugin(void* a, int v, char** e) { return go_start_plugin(a, v, e); }
int c_stop_plugin(char** e) { return go_stop_plugin(e); }
int c_invoke_tool(char* n, char* a, char** r, char** e) { return go_invoke_tool(n, a, r, e); }
int c_invoke_stage(char* s, char* c, char** e) { return go_invoke_stage(s, c, e); }
int c_invoke_output(char* c, char* m, char* p, char** e) { return go_invoke_output(c, m, p, e); }
void c_free_string(char* p) { go_free_string(p); }

// ha_dispatch — called by Go bridge, passes through to CoreAPI dispatch
int ha_dispatch(int id, void* api, char* s1, char* s2, char* s3, int i1, int i2, char** r, char** e) {
    CoreAPI* a = (CoreAPI*)api;
    if (!a || !a->dispatch) return 1;
    return a->dispatch(id, a->ctx, s1, s2, s3, i1, i2, r, e);
}

PluginAPI* plugin_init(void) {
    static PluginAPI api;
    memset(&api, 0, sizeof(api));
    api.version = HOMEAGENT_ABI_VERSION; api.version_min = HOMEAGENT_ABI_VERSION;
    api.init_plugin = c_init_plugin; api.start_plugin = c_start_plugin; api.stop_plugin = c_stop_plugin;
    api.invoke_tool = c_invoke_tool; api.invoke_stage = c_invoke_stage; api.invoke_output = c_invoke_output;
    api.free_string = c_free_string;
    return &api;
}
`

const tmplReadme = `# {{.Plg.Name}}

{{.Plg.Description}}

## Build

` + "```bash" + `
plugindev build
` + "```" + `

## Install

Upload the .hmap file through the Plugin Manager API.
`
