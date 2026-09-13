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

  -- 工具：1.1/1.2/1.3 新增能力巡检（媒体块 / 注入标志位 / 事件 / 动态通道注销）
  -- 注意：故意不在这里调用 sdk.inject_input_sync——工具handler 运行在 LLM 回合内，
  -- 同步注入会等本轮回复，等于自己等自己（死锁）。同步注入只适合事件回调等外部入口。
  sdk.register_tool("luademo_probe_v2", {
    description = "Exercise media blocks, inject opts, events and channel unregister",
    parameters = { type = "object", properties = {} },
    no_memory = true,
    context_policy = "prune",
  }, function(args)
    local res = {}

    -- 多模态：设置下一轮 tool message 携带的内容块
    sdk.set_tool_blocks({
      { type = "text", text = "luademo media block" },
      { type = "image_url", image_url = { url = "https://example.com/x.png", detail = "low" } },
    })
    res.set_tool_blocks = "ok"

    -- 注入标志位（零值 opts 与旧三参数等价）
    sdk.inject_text_opts("luademo", "luademo_in", "opts inject", {
      no_memory = true, context_policy = "prune",
    })
    res.inject_text_opts = "ok"

    -- 带媒体的中断注入
    sdk.inject_interrupt_media("luademo", "luademo_in", "media inject", {
      { type = "audio_url", audio_url = { url = "https://example.com/a.mp3" } },
    })
    res.inject_interrupt_media = "ok"

    -- 媒体入记忆：三元组带原句，文档带附件
    local _, merr = sdk.memory.commit({{
      subject = "luademo", relation = "shows", object = "image",
      sentence_text = "luademo shows an image", media_digests = {},
    }})
    res.memory_commit_with_sentence = { err = merr }
    local _, derr = sdk.doc.insert_with_media(
      { id = "luademo-media", title = "media", content = "with attachment" },
      { { mime = "image/png", name = "x.png", data = "aGVsbG8=" } })
    res.doc_insert_with_media = { err = derr }

    -- 事件订阅（返回取消订阅函数）
    local unsub = sdk.events.subscribe("agent_output", function(evt)
      sdk.log("info", "luademo event: " .. tostring(evt.type))
    end)
    res.events_subscribe = type(unsub)
    if unsub then unsub() end

    -- 插件管理（只读查询）
    res.plugin_mgr_loaded = type(sdk.plugin_mgr.list_loaded())

    -- 动态输出通道注销
    sdk.register_output_channel("luademo_dyn", 0, "dynamic", {}, function(a) return { ok = true } end)
    local _, uerr = sdk.unregister_output_channel("luademo_dyn")
    res.unregister = { err = uerr }

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

  -- 阶段钩子：全局作用域（修改 ctx 字段会写回内核，见 applyLuaStageResult）
  sdk.register_stage("pre_action", function(ctx)
    sdk.log("info", "luademo stage pre_action: user=" .. tostring(ctx.user_id))
    -- 演示 stage 写回：给 llm_text 追加标记（内核会同步回 StageContext）
    if ctx.llm_text then
      ctx.llm_text = ctx.llm_text .. "[luademo]"
    end
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
