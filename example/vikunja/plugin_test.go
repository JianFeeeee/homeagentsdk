package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestPlugin 构造一个不依赖 sdk 的插件实例，指向 httptest 服务。
// ensure() 在 sdk==nil 时会保留已设置的字段，因此可以这样直接测处理器。
func newTestPlugin(t *testing.T, h http.HandlerFunc) (*Plugin, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	p := &Plugin{
		name:     "vikunja",
		baseURL:  srv.URL,
		token:    "tk_test",
		apiVer:   "v2",
		maxItems: 5,
		compact:  true,
		http:     srv.Client(),
	}
	return p, srv
}

func mustJSON(t *testing.T, v interface{}) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// 1) 列表：v2 用 q= 搜索，且 filter 会带上默认 done 条件
func TestTasksListV2(t *testing.T) {
	var gotQuery string
	p, _ := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		if r.Header.Get("Authorization") != "Bearer tk_test" {
			t.Errorf("缺少 Bearer 头: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"title":"写周报","done":false,"project_id":3,"due_date":"2026-09-13T10:00:00Z","labels":[{"title":"工作"}],"assignees":[{"username":"jianf"}]}]`))
	})
	res, err := p.handleTasksList(map[string]interface{}{"search": "周报", "limit": float64(5)})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(gotQuery, "q=%E5%91%A8%E6%8A%A5") {
		t.Errorf("v2 应使用 q= 搜索，实际 query=%s", gotQuery)
	}
	if strings.Contains(gotQuery, "s=") {
		t.Errorf("v2 不应使用 s=，实际 query=%s", gotQuery)
	}
	if !strings.Contains(gotQuery, "per_page=5") {
		t.Errorf("per_page 未生效: %s", gotQuery)
	}
	if !strings.Contains(gotQuery, "filter=done+%3D+false") && !strings.Contains(gotQuery, "filter=done%20%3D%20false") {
		t.Errorf("默认应过滤未完成，实际 filter 片段: %s", gotQuery)
	}
	m, ok := res.(map[string]interface{})
	if !ok {
		t.Fatalf("结果应为 map，实际 %T", res)
	}
	if m["count"].(int) != 1 {
		t.Errorf("count 应为 1，实际 %v", m["count"])
	}
	tasks := m["tasks"].([]interface{})
	tk := tasks[0].(map[string]interface{})
	if _, ok := tk["labels"].([]string); !ok {
		t.Errorf("标签应被投影成名称数组，实际 %T", tk["labels"])
	}
	if _, ok := tk["description"]; ok {
		t.Errorf("精简输出不应出现 description")
	}
}

// 2) 列表：v1 用 s= 搜索
func TestTasksListV1SearchParam(t *testing.T) {
	var gotQuery string
	p, _ := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`[]`))
	})
	p.apiVer = "v1"
	if _, err := p.handleTasksList(map[string]interface{}{"search": "abc"}); err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(gotQuery, "s=abc") {
		t.Errorf("v1 应使用 s= 搜索，实际 %s", gotQuery)
	}
	if strings.Contains(gotQuery, "q=") {
		t.Errorf("v1 不应出现 q=，实际 %s", gotQuery)
	}
}

// 3) 建任务：v1=PUT、v2=POST（同路径，方法不同）
func TestTaskCreateMethodByVersion(t *testing.T) {
	for _, tc := range []struct {
		ver    string
		method string
	}{
		{"v1", http.MethodPut},
		{"v2", http.MethodPost},
	} {
		var gotMethod, gotPath string
		p, _ := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath = r.Method, r.URL.Path
			_, _ = w.Write([]byte(`{"id":42,"title":"买菜"}`))
		})
		p.apiVer = tc.ver
		if _, err := p.handleTaskCreate(map[string]interface{}{"project_id": "3", "title": "买菜"}); err != nil {
			t.Fatalf("[%s] err: %v", tc.ver, err)
		}
		if gotMethod != tc.method {
			t.Errorf("[%s] 期望 %s，实际 %s", tc.ver, tc.method, gotMethod)
		}
		if gotPath != "/api/"+tc.ver+"/projects/3/tasks" {
			t.Errorf("[%s] 路径错误: %s", tc.ver, gotPath)
		}
	}
}

// 4) 改任务（v2）：走 merge-patch，只发变更字段
func TestTaskUpdateV2MergePatch(t *testing.T) {
	var method, ctype string
	var body map[string]interface{}
	p, _ := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		ctype = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = w.Write([]byte(`{"id":7,"done":true}`))
	})
	if _, err := p.handleTaskUpdate(map[string]interface{}{"id": "7", "done": true}); err != nil {
		t.Fatalf("err: %v", err)
	}
	if method != http.MethodPatch {
		t.Errorf("v2 应用 PATCH，实际 %s", method)
	}
	if !strings.Contains(ctype, "merge-patch") {
		t.Errorf("应使用 merge-patch 内容类型，实际 %s", ctype)
	}
	if len(body) != 1 || body["done"] != true {
		t.Errorf("只应发送变更字段，实际 %v", body)
	}
}

// 5) 改任务（v2）回退：merge-patch 被拒 → 取回-合并-PUT
func TestTaskUpdateV2FallbackToMergePut(t *testing.T) {
	var calls []string
	var putBody map[string]interface{}
	p, _ := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodPatch:
			w.WriteHeader(http.StatusUnsupportedMediaType)
			_, _ = w.Write([]byte(`{"code":9,"message":"unsupported media type"}`))
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"id":7,"title":"旧标题","done":false,"priority":1}`))
		case r.Method == http.MethodPut:
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &putBody)
			_, _ = w.Write([]byte(`{"id":7,"title":"新标题","done":false,"priority":1}`))
		default:
			t.Errorf("意外请求: %s %s", r.Method, r.URL.Path)
		}
	})
	if _, err := p.handleTaskUpdate(map[string]interface{}{"id": "7", "title": "新标题"}); err != nil {
		t.Fatalf("err: %v", err)
	}
	want := []string{"PATCH /api/v2/tasks/7", "GET /api/v2/tasks/7", "PUT /api/v2/tasks/7"}
	if len(calls) != len(want) {
		t.Fatalf("调用序列不符: %v", calls)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Errorf("第 %d 步期望 %s，实际 %s", i+1, want[i], calls[i])
		}
	}
	if putBody["title"] != "新标题" {
		t.Errorf("合并后的 body 应含新标题，实际 %v", putBody)
	}
	if putBody["priority"] != float64(1) {
		t.Errorf("合并必须保留原有字段（priority），实际 %v", putBody)
	}
}

// 6) 改任务（v1）：没有 merge-patch，必须取回-合并-POST
func TestTaskUpdateV1FetchMergePost(t *testing.T) {
	var calls []string
	p, _ := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"id":9,"title":"旧","priority":2}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":9,"title":"新","priority":2}`))
	})
	p.apiVer = "v1"
	if _, err := p.handleTaskUpdate(map[string]interface{}{"id": "9", "title": "新"}); err != nil {
		t.Fatalf("err: %v", err)
	}
	want := []string{"GET /api/v1/tasks/9", "POST /api/v1/tasks/9"}
	if len(calls) != 2 || calls[0] != want[0] || calls[1] != want[1] {
		t.Fatalf("v1 应为 GET→POST，实际 %v", calls)
	}
}

// 7) 错误映射：401 提示检查 token
func TestErrorHint401(t *testing.T) {
	p, _ := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":11,"message":"invalid token"}`))
	})
	_, err := p.handleTasksList(map[string]interface{}{})
	if err == nil {
		t.Fatal("应返回错误")
	}
	msg := err.Error()
	if !strings.Contains(msg, "401") || !strings.Contains(msg, "code=11") {
		t.Errorf("错误信息应含状态码与 Vikunja code，实际 %s", msg)
	}
	if !strings.Contains(msg, "token") {
		t.Errorf("401 应给出 token 提示，实际 %s", msg)
	}
}

// 8) 未配置 token 时应给出可操作提示，而不是发出无凭据请求
func TestMissingToken(t *testing.T) {
	p, _ := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("未配置 token 时不应发请求")
	})
	p.token = ""
	_, err := p.handleTasksList(map[string]interface{}{})
	if err == nil || !strings.Contains(err.Error(), "vikunja.token") {
		t.Fatalf("应提示配置项名，实际 %v", err)
	}
}

// 9) 导入：Todoist 必须走 v1（即使插件默认是 v2）
func TestMigrateUsesV1ForTodoist(t *testing.T) {
	var path string
	p, _ := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	if _, err := p.handleMigrate(map[string]interface{}{"action": "start", "source": "todoist", "code": "abc"}); err != nil {
		t.Fatalf("err: %v", err)
	}
	if path != "/api/v1/migration/todoist/migrate" {
		t.Errorf("Todoist 导入必须走 v1，实际 %s", path)
	}
}

// 10) 导入：WeKan 走 v2
func TestMigrateUsesV2ForWekan(t *testing.T) {
	var path, method string
	p, _ := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		path, method = r.URL.Path, r.Method
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	if _, err := p.handleMigrate(map[string]interface{}{"action": "start", "source": "wekan"}); err != nil {
		t.Fatalf("err: %v", err)
	}
	if path != "/api/v2/migration/wekan/migrate" || method != http.MethodPost {
		t.Errorf("WeKan 应走 v2 POST，实际 %s %s", method, path)
	}
}

// 11) 时间跟踪：秒数换算成 end_time；计时开始则不带 end_time
func TestTimeEntrySecondsBecomesEndTime(t *testing.T) {
	var body map[string]interface{}
	p, _ := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = w.Write([]byte(`{"id":1}`))
	})
	start := "2026-09-12T10:00:00+08:00"
	if _, err := p.handleTimeEntries(map[string]interface{}{
		"action": "create", "task_id": "5", "seconds": float64(600),
		"start_time": start,
	}); err != nil {
		t.Fatalf("err: %v", err)
	}
	// 判据不写死字符串：按时区无关的方式比较两个时间点
	sStart, err := time.Parse(time.RFC3339, start)
	if err != nil {
		t.Fatalf("case 自身时间写错: %v", err)
	}
	gotEnd, ok := body["end_time"].(string)
	if !ok {
		t.Fatalf("应有 end_time，实际 %v", body["end_time"])
	}
	tEnd, err := time.Parse(time.RFC3339, gotEnd)
	if err != nil {
		t.Fatalf("end_time 不是 RFC3339: %q", gotEnd)
	}
	if diff := tEnd.Sub(sStart); diff != 10*time.Minute {
		t.Errorf("end_time 应由 start_time+600s 推出，实际差值 %v", diff)
	}
	if _, ok := body["seconds"]; ok {
		t.Errorf("TimeEntry 没有 seconds 字段，不应发送：%v", body)
	}

	body = nil
	if _, err := p.handleTimeEntries(map[string]interface{}{"action": "timer_start", "task_id": "5"}); err != nil {
		t.Fatalf("err: %v", err)
	}
	v, present := body["end_time"]
	if !present || v != nil {
		t.Errorf("计时开始应显式 end_time=null（live timer），实际 %v", body)
	}
}

// 12) 时间跟踪在 v1 下应给出明确不可用提示
func TestTimeEntryUnavailableOnV1(t *testing.T) {
	p, _ := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {})
	p.apiVer = "v1"
	_, err := p.handleTimeEntries(map[string]interface{}{"action": "list"})
	if err == nil || !strings.Contains(err.Error(), "v2") {
		t.Fatalf("v1 下应提示改用 v2，实际 %v", err)
	}
}

// 13) 标签：v1 收 Label 对象、v2 收 label_id
func TestLabelBodyByVersion(t *testing.T) {
	var body map[string]interface{}
	p, _ := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = w.Write([]byte(`{}`))
	})
	if _, err := p.handleTaskLabels(map[string]interface{}{"action": "add", "id": "1", "label_id": "5"}); err != nil {
		t.Fatalf("err: %v", err)
	}
	if body["label_id"] != float64(5) {
		t.Errorf("v2 应发送 label_id，实际 %v", body)
	}

	body = nil
	p.apiVer = "v1"
	if _, err := p.handleTaskLabels(map[string]interface{}{"action": "add", "id": "1", "label_id": "5"}); err != nil {
		t.Fatalf("err: %v", err)
	}
	if body["id"] != float64(5) {
		t.Errorf("v1 应发送 Label 对象(id)，实际 %v", body)
	}
}

// 14) 时间字符串容忍：today / +3d / ISO
func TestNormalizeTime(t *testing.T) {
	for _, in := range []string{"today", "tomorrow", "+3d", "2026-09-12 18:00", "2026-09-12T18:00:00+08:00"} {
		got := normalizeTime(in)
		s, ok := got.(string)
		if !ok {
			t.Fatalf("%s: 期望字符串，实际 %T", in, got)
		}
		if _, err := time.Parse(time.RFC3339, s); err != nil {
			t.Errorf("%s → %s 不是 RFC3339: %v", in, s, err)
		}
	}
}

// 15) 通用直通：可指定 api_version，method 大小写不敏感
func TestRawAPI(t *testing.T) {
	var method, path string
	p, _ := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		_, _ = w.Write([]byte(`[]`))
	})
	if _, err := p.handleRawAPI(map[string]interface{}{"method": "get", "path": "projects", "api_version": "v1"}); err != nil {
		t.Fatalf("err: %v", err)
	}
	if method != http.MethodGet || path != "/api/v1/projects" {
		t.Errorf("直通参数未生效: %s %s", method, path)
	}
}

// 16) 精简输出可关闭（关闭时返回原样）
func TestCompactToggle(t *testing.T) {
	p, _ := newTestPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"id":1,"title":"t","description":"很长的描述","done":false}]`))
	})
	p.compact = false
	res, err := p.handleTasksList(map[string]interface{}{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	arr, ok := res.([]interface{})
	if !ok {
		t.Fatalf("关闭精简后应原样返回数组，实际 %T", res)
	}
	if _, ok := arr[0].(map[string]interface{})["description"]; !ok {
		t.Errorf("关闭精简后应保留 description")
	}
}
