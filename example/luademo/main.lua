-- luademo plugin — 展示 v0.8.0 Lua SDK 全部能力
-- 运行环境：内核注入真实实现；lua main.lua 可用 sdk.lua mock 独立测试
local plugin = { name = "luademo" }

function plugin.start(sdk)
  sdk.log("info", "luademo starting...")

  -- 注册配置项（WebUI 可展示）
  sdk.settings.register_def({
    key = "plugin.luademo.greeting",
    default = "Hello",
    type = "string",
    display_name = "Greeting",
    description = "Greeting prefix for the hello tool",
    category = "luademo",
  })

  -- 注册工具：no_memory（输出跳过记忆计算）+ cleaner（计算层过滤函数）
  sdk.register_tool("luademo_hello", {
    description = "A hello world tool with no_memory and cleaner",
    parameters = { type = "object", properties = {} },
    no_memory = true,
    cleaner = function(text) return "CLEANED:" .. text end,
  }, function(args)
    local prefix, err = sdk.settings.get_core("plugin.luademo.greeting")
    if err ~= nil then prefix = "Hello" end
    return { content = (prefix or "Hello") .. " from luademo plugin!" }
  end)

  -- 注册工具：数据类 API 巡检（memory/doc/knowledge/text_memory/llm/settings/social）
  sdk.register_tool("luademo_probe", {
    description = "Exercise every aligned data API and return combined results",
    parameters = { type = "object", properties = {} },
    no_memory = true,
  }, function(args)
    local res = {}

    local ok, err = sdk.memory.commit({ { subject = "demo", relation = "uses", object = "lua" } })
    res.memory_commit = { ok = ok, err = err }
    local recalled, rerr = sdk.memory.recall("demo", 1)
    res.memory_recall = { result = recalled, err = rerr }

    ok, err = sdk.doc.insert({ id = "demo-1", title = "lua demo doc", content = "hello lua world" })
    res.doc_insert = { ok = ok, err = err }
    local docs, derr = sdk.doc.query("lua", 2)
    res.doc_query = { result = docs, err = derr }

    ok, err = sdk.knowledge.add("luademo", "lua knowledge entry")
    res.knowledge_add = { ok = ok, err = err }
    local entries, kerr = sdk.knowledge.search("luademo", 2)
    res.knowledge_search = { result = entries, err = kerr }

    ok, err = sdk.text_memory.append({ role = "tool", content = "luademo probe ran", channel = "luademo" })
    res.text_memory = { ok = ok, err = err }

    local sources, serr = sdk.llm.list_sources()
    res.llm_sources = { result = sources, err = serr }

    local v, verr = sdk.settings.get_core("agent.name")
    res.settings_get_core = { result = v, err = verr }
    local defs, defserr = sdk.settings.defs("plugin.luademo")
    res.settings_defs = { result = defs, err = defserr }

    local persons, perr = sdk.social.list_persons()
    res.social_persons = { result = persons, err = perr }

    return { content = res }
  end)

  -- 阶段钩子：own_tools 作用域（仅本插件工具被调用时触发）
  sdk.register_stage("before_toolcall", function(ctx)
    local calls = ctx.tool_calls or {}
    if calls[1] then
      sdk.log("info", "luademo stage before_toolcall: tool=" .. tostring(calls[1].name))
    end
    return nil
  end, "own_tools")

  -- 阶段钩子：全局作用域
  sdk.register_stage("pre_action", function(ctx)
    sdk.log("info", "luademo stage pre_action: user=" .. tostring(ctx.user_id))
    return nil
  end)

  -- 输出通道：路由输出到外部渠道（def 支持 no_memory/cleaner）
  sdk.register_output_channel("luademo_out", 0, "luademo push channel",
    { no_memory = true, cleaner = function(t) return "OCLEANED:" .. t end },
    function(args) return { content = "out-channel ack" } end)

  -- 输入通道
  sdk.register_input_channel("luademo_in", { no_memory = true })

  -- 其他 API
  sdk.register_api("luademo.ping")
  sdk.set_auto_restart(true)

  sdk.log("info", "luademo started")
end

function plugin.stop() sdk.log("info", "luademo stopped") end
return plugin
