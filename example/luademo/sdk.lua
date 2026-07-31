-- HomeAgent Lua Plugin SDK (standalone mock)
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
