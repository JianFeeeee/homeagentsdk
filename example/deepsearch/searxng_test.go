package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeCall struct {
	dir  string
	name string
	args []string
}

func (c fakeCall) String() string { return c.name + " " + strings.Join(c.args, " ") }

// newFakeRunner 记录调用并返回预设结果
func newFakeRunner(calls *[]fakeCall, out string, err error) cmdRunner {
	var mu sync.Mutex
	return func(ctx context.Context, dir, name string, args ...string) (string, error) {
		mu.Lock()
		*calls = append(*calls, fakeCall{dir: dir, name: name, args: args})
		mu.Unlock()
		return out, err
	}
}

// fastBudget 把就绪窗口压到毫秒级，避免单测真等
func fastBudget() searxBudget {
	return searxBudget{
		probe:    50 * time.Millisecond,
		up:       time.Second,
		ready:    200 * time.Millisecond,
		shutdown: time.Second,
	}
}

// 1) 后端没跑 → 应执行 docker compose up -d，并认领关闭责任
func TestEnsureSearxngStartsWhenUnreachable(t *testing.T) {
	var calls []fakeCall
	p := &Plugin{
		name: "deepsearch", searxURL: "http://127.0.0.1:1", searxDir: "/tmp/fake-searx",
		manageSearx: true, stopOnExit: true, userAgent: "test",
		bud: fastBudget(), run: newFakeRunner(&calls, "Container searxng-agent Started", nil),
	}
	p.ensureSearxng()

	if len(calls) != 1 {
		t.Fatalf("应恰好拉起一次，实际 %d 次：%v", len(calls), calls)
	}
	got := calls[0]
	if got.name != "docker" || strings.Join(got.args, " ") != "compose up -d" {
		t.Errorf("命令不对：%s", got)
	}
	if got.dir != "/tmp/fake-searx" {
		t.Errorf("工作目录应为配置的 compose 目录，实际 %q", got.dir)
	}
	if !p.searxOwned {
		t.Error("既然是我们拉起的，就应认领关闭责任")
	}
}

// 2) 后端已在跑 → 不重启，直接接管
func TestEnsureSearxngAdoptsRunningBackend(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			_, _ = w.Write([]byte("OK"))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	var calls []fakeCall
	p := &Plugin{
		name: "deepsearch", searxURL: srv.URL, searxDir: "/tmp/fake-searx",
		manageSearx: true, stopOnExit: true, userAgent: "test",
		bud: fastBudget(), run: newFakeRunner(&calls, "", nil),
	}
	p.ensureSearxng()

	if len(calls) != 0 {
		t.Errorf("已在跑就不该重启它，实际执行了：%v", calls)
	}
	if !p.searxOwned {
		t.Error("接管后也应负责停止（与「不重启」不冲突）")
	}
}

// 3) 关掉托管 → 完全不碰 docker
func TestEnsureSearxngDisabled(t *testing.T) {
	var calls []fakeCall
	p := &Plugin{
		name: "deepsearch", searxURL: "http://127.0.0.1:1", searxDir: "/tmp/fake-searx",
		manageSearx: false, stopOnExit: true, userAgent: "test",
		bud: fastBudget(), run: newFakeRunner(&calls, "", nil),
	}
	p.ensureSearxng()
	if len(calls) != 0 || p.searxOwned {
		t.Errorf("manage_searxng=false 时不该有任何动作：calls=%v owned=%v", calls, p.searxOwned)
	}
}

// 4) 拉起失败不能让插件起不来（记日志即可）
func TestEnsureSearxngFailureNonFatal(t *testing.T) {
	var calls []fakeCall
	p := &Plugin{
		name: "deepsearch", searxURL: "http://127.0.0.1:1", searxDir: "/tmp/fake-searx",
		manageSearx: true, stopOnExit: true, userAgent: "test",
		bud: fastBudget(), run: newFakeRunner(&calls, "Cannot connect to the Docker daemon", errors.New("exit status 1")),
	}
	p.ensureSearxng() // 不应 panic
	if p.searxOwned {
		t.Error("没拉起来就不该认领关闭责任（否则停止时会去关一个不是我们起的服务）")
	}
}

// 5) 停止：关掉我们拉起的后端，且幂等
func TestShutdownStopsOwnedBackend(t *testing.T) {
	var calls []fakeCall
	runner := newFakeRunner(&calls, "ok", nil)
	p := &Plugin{
		name: "deepsearch", searxURL: "http://127.0.0.1:1", searxDir: "/tmp/fake-searx",
		manageSearx: true, stopOnExit: true, userAgent: "test",
		bud: fastBudget(), run: runner,
	}
	p.ensureSearxng()
	calls = nil

	p.shutdownSearxng()
	if len(calls) != 1 {
		t.Fatalf("应执行一次 compose stop，实际 %v", calls)
	}
	if got := strings.Join(calls[0].args, " "); !strings.HasPrefix(got, "compose stop") {
		t.Errorf("停止命令不对：%s", got)
	}
	if p.searxOwned {
		t.Error("停止后应清掉认领标记")
	}

	p.shutdownSearxng() // 幂等：不应再调一次
	if len(calls) != 1 {
		t.Errorf("重复停止应无副作用，实际 %v", calls)
	}
}

// 6) 不是我们拉起的 → 停止时不许动它
func TestShutdownSkippedWhenNotOwned(t *testing.T) {
	var calls []fakeCall
	p := &Plugin{
		name: "deepsearch", searxDir: "/tmp/fake-searx", manageSearx: true, stopOnExit: true,
		bud: fastBudget(), run: newFakeRunner(&calls, "", nil),
	}
	p.shutdownSearxng()
	if len(calls) != 0 {
		t.Errorf("不该去停一个我们没起的服务：%v", calls)
	}
}

// 7) 配了「停止时保留」→ 认领过也不关
func TestShutdownKeepsBackendWhenConfigured(t *testing.T) {
	var calls []fakeCall
	p := &Plugin{
		name: "deepsearch", searxURL: "http://127.0.0.1:1", searxDir: "/tmp/fake-searx",
		manageSearx: true, stopOnExit: false, userAgent: "test",
		bud: fastBudget(), run: newFakeRunner(&calls, "", nil),
	}
	p.ensureSearxng()
	calls = nil
	p.shutdownSearxng()
	if len(calls) != 0 {
		t.Errorf("stop_searxng_on_exit=false 时不应关闭：%v", calls)
	}
}

// 8) Stop() 自身也要收尾（内核 stdin 关闭路径不会走 stop handler 的注册顺序之外）
func TestStopTriggersShutdown(t *testing.T) {
	var calls []fakeCall
	p := &Plugin{
		name: "deepsearch", searxURL: "http://127.0.0.1:1", searxDir: "/tmp/fake-searx",
		manageSearx: true, stopOnExit: true, userAgent: "test",
		bud: fastBudget(), run: newFakeRunner(&calls, "", nil),
	}
	p.ensureSearxng()
	calls = nil
	if err := p.Stop(); err != nil {
		t.Fatalf("Stop 返回错误: %v", err)
	}
	if len(calls) != 1 {
		t.Errorf("Stop 应触发一次关闭，实际 %v", calls)
	}
}
