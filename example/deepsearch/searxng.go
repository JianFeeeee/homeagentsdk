package main

// SearXNG 生命周期托管：插件启动时拉起搜索后端，插件停止时关闭它。
//
// 契约依据（内核侧 internal/plugin/proc/*，已逐行核对）：
//   - 内核停止插件：发 `plugin.stop` → 插件先跑 RunStopHandlers（LIFO、幂等）→ 再 Stop() → exit(0)
//   - 若插件未在 stopGracePeriod（**5 秒**）内退出，内核直接 SIGKILL
//   - stdin 关闭（内核消失）同样会跑 handlers + Stop()
//
// 因此这里的关闭动作必须**有界**：searxShutdownBudget 取 4s，留 1s 余量。
//
// 归属规则（谁拉起谁关）：**只有本插件真正执行了 `docker compose up -d` 的实例才算「我们起的」**。
// 探活发现已在运行的实例只「接管」——不认领关闭责任。否则同一台机器上的第二个实例
// （E2E 测试拉起的插件、另一个 daemon）退出时会把生产后端一起带走：实测就是这条把
// 线上搜索服务反复关停的（测试实例用默认配置，测试结束就 `docker compose stop`）。
// 若插件是被 kill -9 / OOM 带走的，关闭动作不会执行 —— SearXNG 会留在运行态；
// 下次 Start 探测到它在跑就直接接管，这是更安全的失败方向。

import (
	"context"
	"log"
	"net/http"
	"os/exec"
	"time"
)

const (
	cfgManageSearx = "manage_searxng"
	cfgSearxDir    = "searxng_dir"
	cfgStopOnExit  = "stop_searxng_on_exit"

	defaultSearxDir = "/root/searxng-agent"

	searxProbeTimeout   = 1500 * time.Millisecond // 单次 healthz 探测
	searxUpBudget       = 20 * time.Second        // docker compose up -d 的上限（正常 1s 内返回）
	searxReadyBudget    = 6 * time.Second         // up 之后等 healthz 就绪的上限
	searxShutdownBudget = 4 * time.Second         // 必须 < 内核 5s 宽限期
)

// searxBudget 把四个时间预算收拢，便于单测注入短值（否则测试要真等就绪窗口）。
type searxBudget struct {
	probe    time.Duration
	up       time.Duration
	ready    time.Duration
	shutdown time.Duration
}

func (p *Plugin) budget() searxBudget {
	b := p.bud
	if b.probe == 0 {
		b.probe = searxProbeTimeout
	}
	if b.up == 0 {
		b.up = searxUpBudget
	}
	if b.ready == 0 {
		b.ready = searxReadyBudget
	}
	if b.shutdown == 0 {
		b.shutdown = searxShutdownBudget
	}
	return b
}

// cmdRunner 抽出来是为了让生命周期逻辑可单测：注入假执行器，不起真容器。
type cmdRunner func(ctx context.Context, dir, name string, args ...string) (string, error)

func defaultRunner(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// searxReachable 探测搜索后端是否可用（只看 healthz，不发检索请求）。
func (p *Plugin) searxReachable(timeout time.Duration) bool {
	if p.searxURL == "" {
		return false
	}
	base := p.http
	if base == nil {
		base = &http.Client{}
	}
	cl := *base // 复制一份，避免改到共享 client 的超时
	cl.Timeout = timeout
	req, err := http.NewRequest(http.MethodGet, p.searxURL+"/healthz", nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", p.userAgent)
	resp, err := cl.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 400
}

// ensureSearxng 在插件启动时确保搜索后端在跑；已在跑则直接接管，不重启。
func (p *Plugin) ensureSearxng() {
	b := p.budget()
	if !p.manageSearx {
		log.Printf("[%s] 未启用 SearXNG 托管（manage_searxng=false），假定 %s 由外部维护", p.name, p.searxURL)
		return
	}
	if p.searxReachable(b.probe) {
		// 只接管，不认领：不是我们拉起来的，就不能由我们关掉
		log.Printf("[%s] SearXNG 已在运行（%s），直接接管（不认领关闭责任）", p.name, p.searxURL)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), b.up)
	out, err := p.run(ctx, p.searxDir, "docker", "compose", "up", "-d")
	cancel()
	if err != nil {
		log.Printf("[%s] 拉起 SearXNG 失败（dir=%s，请检查 manage_searxng/searxng_dir 配置）: %v；输出: %s",
			p.name, p.searxDir, err, oneLine(out, 300))
		return
	}
	log.Printf("[%s] 已执行 docker compose up -d（%s）：%s", p.name, p.searxDir, oneLine(out, 200))

	deadline := time.Now().Add(b.ready)
	for time.Now().Before(deadline) {
		if p.searxReachable(800 * time.Millisecond) {
			log.Printf("[%s] SearXNG 就绪", p.name)
			p.markSearxOwned()
			return
		}
		time.Sleep(600 * time.Millisecond)
	}
	log.Printf("[%s] SearXNG 已启动但 %s 内未就绪；首次检索会自动等待", p.name, b.ready)
	p.markSearxOwned()
}

func (p *Plugin) markSearxOwned() {
	p.searxMu.Lock()
	p.searxOwned = true
	p.searxMu.Unlock()
}

// shutdownSearxng 关闭搜索后端。幂等，且有界（内核宽限期 5s，这里最多 4s）。
func (p *Plugin) shutdownSearxng() {
	b := p.budget()
	p.searxMu.Lock()
	owned := p.searxOwned
	p.searxOwned = false
	p.searxMu.Unlock()

	if !owned {
		return // 不是我们拉起来的 / 已经关过
	}
	if !p.manageSearx || !p.stopOnExit {
		log.Printf("[%s] 保留 SearXNG 运行（stop_searxng_on_exit=false）", p.name)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), b.shutdown)
	defer cancel()
	out, err := p.run(ctx, p.searxDir, "docker", "compose", "stop", "-t", "2")
	if err != nil {
		// 故意只记日志：这里再重试就会拖过内核宽限期，被 SIGKILL 更糟
		log.Printf("[%s] 关闭 SearXNG 失败（忽略）: %v；输出: %s", p.name, err, oneLine(out, 200))
		return
	}
	log.Printf("[%s] 已关闭 SearXNG", p.name)
}
