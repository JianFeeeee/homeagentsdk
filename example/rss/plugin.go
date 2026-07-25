package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	sdk "gitcode.com/JianFeeeee/homeagent-sdk/sdk"
	"github.com/mmcdole/gofeed"
)

type FeedSub struct {
	URL      string `json:"url"`
	Title    string `json:"title"`
	AddedAt  string `json:"added_at"`
	Interval int    `json:"interval"`
}

type Plugin struct {
	name       string
	sdk        *sdk.PluginSDK
	client     *http.Client
	fp         *gofeed.Parser
	dataDir    string
	mu         sync.RWMutex
	feeds      []FeedSub
	seenGUIDs  map[string]bool
	stopCh     chan struct{}
	wg         sync.WaitGroup
	pollTicker *time.Ticker
}

func NewPluginFactory(name string, config map[string]interface{}) (sdk.Plugin, error) {
	return &Plugin{name: name}, nil
}

func (p *Plugin) Name() string { return p.name }

func getSetting[T string | int64 | float64](s sdk.SettingsAPI, key string, fallback T) T {
	v, err := s.Get(key)
	if err != nil || v == nil {
		return fallback
	}
	switch any(fallback).(type) {
	case string:
		if sv, ok := v.(string); ok {
			return any(sv).(T)
		}
	case int64:
		switch val := v.(type) {
		case float64:
			return any(int64(val)).(T)
		case string:
			if n, err := strconv.ParseInt(val, 10, 64); err == nil {
				return any(n).(T)
			}
		}
	case float64:
		switch val := v.(type) {
		case float64:
			return any(val).(T)
		case string:
			if n, err := strconv.ParseFloat(val, 64); err == nil {
				return any(n).(T)
			}
		}
	}
	return fallback
}

func readArg(args map[string]interface{}, key string) string {
	if v, ok := args[key]; ok && v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func readArgInt(args map[string]interface{}, key string, fallback int) int {
	if v, ok := args[key]; ok && v != nil {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int64:
			return int(n)
		}
	}
	return fallback
}

func (p *Plugin) Start(s *sdk.PluginSDK) error {
	s.SetAutoRestart(true)
	p.sdk = s
	p.client = &http.Client{Timeout: 30 * time.Second}
	p.fp = gofeed.NewParser()
	p.stopCh = make(chan struct{})
	p.seenGUIDs = make(map[string]bool)
	p.feeds = []FeedSub{}

	dataHome := os.Getenv("HOME")
	if dataHome == "" {
		dataHome = "/tmp"
	}
	p.dataDir = filepath.Join(dataHome, ".homeagent", "rss")
	os.MkdirAll(p.dataDir, 0755)
	p.loadData()

	s.Settings().RegisterDef(sdk.ConfigDef{
		Key: "poll_interval", Default: "30", Type: "string",
		DisplayName: "Poll Interval", Description: "Default polling interval in minutes (default: 30)",
		Category: "rss",
	})

	tp := p.name + "_"
	s.RegisterTool(tp+"subscribe", sdk.ToolDef{
		Name: tp + "subscribe", Description: "Subscribe to an RSS/Atom feed URL",
		NoMemory: true,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url":      map[string]interface{}{"type": "string", "description": "Feed URL"},
				"interval": map[string]interface{}{"type": "integer", "description": "Poll interval in minutes (default: 30, minimum: 5)"},
			},
			"required": []string{"url"},
		},
	}, p.handleSubscribe)

	s.RegisterTool(tp+"unsubscribe", sdk.ToolDef{
		Name: tp + "unsubscribe", Description: "Unsubscribe from a feed",
		NoMemory: true,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url": map[string]interface{}{"type": "string", "description": "Feed URL to unsubscribe"},
			},
			"required": []string{"url"},
		},
	}, p.handleUnsubscribe)

	s.RegisterTool(tp+"list", sdk.ToolDef{
		Name: tp + "list", Description: "List all subscribed feeds",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{},
		},
	}, p.handleList)

	s.RegisterTool(tp+"check_now", sdk.ToolDef{
		Name: tp + "check_now", Description: "Manually check all feeds for new articles now",
		NoMemory: true,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{},
		},
	}, p.handleCheckNow)

	pollMin := int(getSetting(s.Settings(), "poll_interval", int64(30)))
	if pollMin < 5 {
		pollMin = 5
	}
	p.pollTicker = time.NewTicker(time.Duration(pollMin) * time.Minute)

	p.wg.Add(1)
	go p.pollLoop()

	fmt.Printf("[%s] started (%d feeds, poll every %dm)\n", p.name, len(p.feeds), pollMin)
	return nil
}

func (p *Plugin) Stop() error {
	close(p.stopCh)
	p.pollTicker.Stop()
	p.wg.Wait()
	p.saveData()
	fmt.Printf("[%s] stopped\n", p.name)
	return nil
}

func (p *Plugin) pollLoop() {
	defer p.wg.Done()

	p.checkAllFeeds()

	for {
		select {
		case <-p.pollTicker.C:
			p.checkAllFeeds()
		case <-p.stopCh:
			return
		}
	}
}

func (p *Plugin) checkAllFeeds() {
	p.mu.RLock()
	feeds := make([]FeedSub, len(p.feeds))
	copy(feeds, p.feeds)
	p.mu.RUnlock()

	for _, feed := range feeds {
		select {
		case <-p.stopCh:
			return
		default:
		}
		p.checkFeed(feed)
	}
}

func (p *Plugin) checkFeed(sub FeedSub) {
	parsed, err := p.fp.ParseURL(sub.URL)
	if err != nil {
		return
	}

	title := parsed.Title
	if title == "" {
		title = sub.URL
	}

	var newArticles []*gofeed.Item
	for _, item := range parsed.Items {
		guid := item.GUID
		if guid == "" {
			guid = item.Link
		}
		if guid == "" {
			continue
		}
		guid = sub.URL + "|" + guid
		p.mu.RLock()
		seen := p.seenGUIDs[guid]
		p.mu.RUnlock()
		if !seen {
			newArticles = append(newArticles, item)
		}
	}

	if len(newArticles) == 0 {
		return
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("📡 %s (%s) — %d 篇新文章:", title, sub.URL, len(newArticles)))
	for _, item := range newArticles {
		pubDate := ""
		if item.PublishedParsed != nil {
			pubDate = item.PublishedParsed.Format("01-02 15:04")
		}
		line := fmt.Sprintf("  • %s", item.Title)
		if pubDate != "" {
			line += fmt.Sprintf(" [%s]", pubDate)
		}
		if item.Link != "" {
			line += "\n    " + item.Link
		}
		lines = append(lines, line)
	}

	p.sdk.InjectInterruptText("rss", "rss", strings.Join(lines, "\n"))

	p.mu.Lock()
	for _, item := range newArticles {
		guid := item.GUID
		if guid == "" {
			guid = item.Link
		}
		if guid == "" {
			continue
		}
		p.seenGUIDs[sub.URL+"|"+guid] = true
	}
	p.mu.Unlock()
	p.saveData()
}

func (p *Plugin) handleSubscribe(args map[string]interface{}) (interface{}, error) {
	url := readArg(args, "url")
	if url == "" {
		return map[string]interface{}{"isError": true, "content": "URL is required"}, nil
	}

	p.mu.RLock()
	for _, f := range p.feeds {
		if f.URL == url {
			p.mu.RUnlock()
			return map[string]interface{}{"isError": true, "content": "Already subscribed to: " + url}, nil
		}
	}
	p.mu.RUnlock()

	interval := readArgInt(args, "interval", 30)
	if interval < 5 {
		interval = 5
	}

	parsed, err := p.fp.ParseURL(url)
	if err != nil {
		return map[string]interface{}{"isError": true, "content": "Failed to parse feed: " + err.Error()}, nil
	}

	feedTitle := parsed.Title
	if feedTitle == "" {
		feedTitle = url
	}

	sub := FeedSub{
		URL:      url,
		Title:    feedTitle,
		AddedAt:  time.Now().Format("2006-01-02 15:04"),
		Interval: interval,
	}

	guidCount := 0
	for _, item := range parsed.Items {
		guid := item.GUID
		if guid == "" {
			guid = item.Link
		}
		if guid == "" {
			continue
		}
		p.seenGUIDs[url+"|"+guid] = true
		guidCount++
	}

	p.mu.Lock()
	p.feeds = append(p.feeds, sub)
	p.mu.Unlock()
	p.saveData()

	return map[string]interface{}{
		"content": fmt.Sprintf("Subscribed to: %s\nTitle: %s\nArticles found: %d\nPoll interval: %d min", url, feedTitle, guidCount, interval),
	}, nil
}

func (p *Plugin) handleUnsubscribe(args map[string]interface{}) (interface{}, error) {
	url := readArg(args, "url")
	if url == "" {
		return map[string]interface{}{"isError": true, "content": "URL is required"}, nil
	}

	p.mu.Lock()
	found := false
	for i, f := range p.feeds {
		if f.URL == url {
			p.feeds = append(p.feeds[:i], p.feeds[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		p.mu.Unlock()
		return map[string]interface{}{"isError": true, "content": "Not subscribed to: " + url}, nil
	}

	for guid := range p.seenGUIDs {
		if strings.HasPrefix(guid, url+"|") {
			delete(p.seenGUIDs, guid)
		}
	}
	p.mu.Unlock()
	p.saveData()

	return map[string]interface{}{"content": "Unsubscribed: " + url}, nil
}

func (p *Plugin) handleList(args map[string]interface{}) (interface{}, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if len(p.feeds) == 0 {
		return map[string]interface{}{"content": "No subscriptions. Use rss_subscribe to add one."}, nil
	}

	sort.Slice(p.feeds, func(i, j int) bool {
		return p.feeds[i].Title < p.feeds[j].Title
	})

	var lines []string
	lines = append(lines, fmt.Sprintf("📡 Subscriptions (%d):", len(p.feeds)))
	for _, f := range p.feeds {
		lines = append(lines, fmt.Sprintf("  • %s\n    %s (every %dm, added %s)", f.Title, f.URL, f.Interval, f.AddedAt))
	}

	return map[string]interface{}{"content": strings.Join(lines, "\n")}, nil
}

func (p *Plugin) handleCheckNow(args map[string]interface{}) (interface{}, error) {
	go p.checkAllFeeds()
	return map[string]interface{}{"content": "Checking all feeds for updates..."}, nil
}

func (p *Plugin) dataFile() string {
	return filepath.Join(p.dataDir, "feeds.json")
}

func (p *Plugin) loadData() {
	b, err := os.ReadFile(p.dataFile())
	if err != nil {
		return
	}
	var data struct {
		Feeds     []FeedSub      `json:"feeds"`
		SeenGUIDs map[string]bool `json:"seen"`
	}
	if json.Unmarshal(b, &data) != nil {
		return
	}
	if data.Feeds != nil {
		p.feeds = data.Feeds
	}
	if data.SeenGUIDs != nil {
		p.seenGUIDs = data.SeenGUIDs
	}
}

func (p *Plugin) saveData() {
	p.mu.RLock()
	defer p.mu.RUnlock()
	data := struct {
		Feeds     []FeedSub      `json:"feeds"`
		SeenGUIDs map[string]bool `json:"seen"`
	}{
		Feeds:     p.feeds,
		SeenGUIDs: p.seenGUIDs,
	}
	b, _ := json.MarshalIndent(data, "", "  ")
	os.WriteFile(p.dataFile(), b, 0644)
}


