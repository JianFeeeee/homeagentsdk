package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"gitcode.com/JianFeeeee/homeagent-sdk/sdk"
)

type SavedMessage struct {
	LocalID     int64  `json:"local_id"`
	MessageID   int64  `json:"message_id"`
	UserID      int64  `json:"user_id"`
	Nickname    string `json:"nickname"`
	GroupID     int64  `json:"group_id,omitempty"`
	GroupName   string `json:"group_name,omitempty"`
	MessageType string `json:"message_type"`
	Text        string `json:"text"`
	RawText     string `json:"raw_text,omitempty"`
	ReplyToID  int64  `json:"reply_to_id,omitempty"`
	ReplyToText string `json:"reply_to_text,omitempty"`
	HasImage    bool   `json:"has_image,omitempty"`
	HasFile     bool   `json:"has_file,omitempty"`
	FilePath    string `json:"file_path,omitempty"`
	Time        int64  `json:"time"`
}

const maxMessages = 2000

type ForwardRule struct {
	GroupID  int64  `json:"group_id"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Password string `json:"password"`
	Template string `json:"template"`
}

func rconSend(host string, port int, password, cmd string) error {
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("rcon dial: %w", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))

	buf := make([]byte, 4096)
	// Login
	pkt := rconPacket(1, 3, password)
	if _, err := conn.Write(pkt); err != nil {
		return fmt.Errorf("rcon login write: %w", err)
	}
	if _, err := io.ReadFull(conn, buf[:12]); err != nil {
		return fmt.Errorf("rcon login read: %w", err)
	}
	// Command
	pkt = rconPacket(2, 2, cmd)
	if _, err := conn.Write(pkt); err != nil {
		return fmt.Errorf("rcon cmd write: %w", err)
	}
	n, err := io.ReadFull(conn, buf[:12])
	if err != nil && err != io.ErrUnexpectedEOF {
		return fmt.Errorf("rcon cmd read: %w (n=%d)", err, n)
	}
	return nil
}

func rconPacket(id, typ int32, body string) []byte {
	b := []byte(body)
	b = append(b, 0) // null terminator
	b = append(b, 0) // padding
	length := 4 + 4 + len(b)
	pkt := make([]byte, 4+len(b))
	binary.LittleEndian.PutUint32(pkt, uint32(length))
	binary.LittleEndian.PutUint32(pkt[4:], uint32(id))
	binary.LittleEndian.PutUint32(pkt[8:], uint32(typ))
	copy(pkt[12:], b)
	return pkt
}

type Plugin struct {
	name           string
	sdk            *sdk.PluginSDK
	mu             sync.RWMutex
	messages       []*SavedMessage
	nextID         int64
	listenAddr     string
	napcatURL      string
	remoteDir      string
	filesDir       string
	adminID        int64
	botID          int64
	botNickname    string
	dmPolicy       string
	groupPolicy    string
	httpClient     *http.Client
	allowFrom      map[int64]struct{}
	groupAllowFrom map[int64]struct{}
	srv            *http.Server
	groupNameCache map[int64]string
	agentfsDir     string
}

func (p *Plugin) Name() string { return p.name }

func (p *Plugin) Start(s *sdk.PluginSDK) error {
	s.SetAutoRestart(true)
	p.sdk = s

	s.Settings().RegisterDef(sdk.ConfigDef{Key: "plugin.qq.listen", Default: "0.0.0.0:25580", Type: "string", DisplayName: "监听地址", Description: "Webhook HTTP 监听地址", Category: "qq"})
	s.Settings().RegisterDef(sdk.ConfigDef{Key: "plugin.qq.napcat_url", Default: "http://127.0.0.1:3000", Type: "string", DisplayName: "NapCat 地址", Description: "NapCat HTTP API 基础 URL", Category: "qq"})
	s.Settings().RegisterDef(sdk.ConfigDef{Key: "plugin.qq.admin", Default: "", Type: "string", DisplayName: "管理员 QQ", Description: "管理员 QQ 号，收到其消息时标记【重要！老大消息】", Category: "qq"})
	s.Settings().RegisterDef(sdk.ConfigDef{Key: "plugin.qq.dm_policy", Default: "open", Type: "string", DisplayName: "私聊策略", Description: "open / allowlist / disabled", Category: "qq", Options: []string{"open", "allowlist", "disabled"}})
	s.Settings().RegisterDef(sdk.ConfigDef{Key: "plugin.qq.allow_from", Default: "", Type: "string", DisplayName: "私聊白名单", Description: "允许私聊机器人的 QQ 号列表，逗号分隔", Category: "qq"})
	s.Settings().RegisterDef(sdk.ConfigDef{Key: "plugin.qq.group_policy", Default: "open", Type: "string", DisplayName: "群聊策略", Description: "open / allowlist / disabled", Category: "qq", Options: []string{"open", "allowlist", "disabled"}})
	s.Settings().RegisterDef(sdk.ConfigDef{Key: "plugin.qq.group_allow_from", Default: "", Type: "string", DisplayName: "群聊白名单", Description: "允许接入的群号列表，逗号分隔", Category: "qq"})
	s.Settings().RegisterDef(sdk.ConfigDef{Key: "plugin.qq.forward_rules", Default: "[]", Type: "string", DisplayName: "转发规则", Description: "JSON 数组，每项 {group_id,host,port,password,template}。匹配的群消息通过 RCON 转发到 Minecraft。template 支持 {nickname} {message} 占位", Category: "qq"})
	s.Settings().RegisterDef(sdk.ConfigDef{Key: "plugin.qq.files_dir", Default: "/home/newqqagent/agentfs/merged/qq_files", Type: "string", DisplayName: "文件存储目录", Description: "从QQ接收的文件保存目录（CQ file/image 自动下载到此目录）", Category: "qq"})
	s.Settings().RegisterDef(sdk.ConfigDef{Key: "plugin.qq.remote_dir", Default: "/home/program/qq-workspace/remote", Type: "string", DisplayName: "NapCat容器共享目录", Description: "与NapCat容器共享的文件目录，主机路径。发文件时文件会复制到此目录，NapCat内部映射为/app/files/", Category: "qq"})
	s.Settings().RegisterDef(sdk.ConfigDef{Key: "plugin.qq.agentfs_dir", Default: "/home/newqqagent/agentfs/merged", Type: "string", DisplayName: "AgentFS目录", Description: "文件读写的工作目录，read_document/video_download 等工具的默认工作目录", Category: "qq"})

	settings := s.Settings()

	p.listenAddr = getSetting[string](settings, "listen", "0.0.0.0:25580")
	p.napcatURL = strings.TrimRight(getSetting[string](settings, "napcat_url", "http://127.0.0.1:3000"), "/")
	p.adminID = getSetting[int64](settings, "admin", 0)
	p.dmPolicy = normalizePolicy(getSetting[string](settings, "dm_policy", "open"))
	p.groupPolicy = normalizePolicy(getSetting[string](settings, "group_policy", "open"))
	p.allowFrom = parseIDSet(getSetting[string](settings, "allow_from", ""))
	p.groupAllowFrom = parseIDSet(getSetting[string](settings, "group_allow_from", ""))
	p.filesDir = strings.TrimRight(getSetting[string](settings, "files_dir", "/home/newqqagent/agentfs/merged/qq_files"), "/")
	p.agentfsDir = strings.TrimRight(getSetting[string](settings, "agentfs_dir", "/home/newqqagent/agentfs/merged"), "/")
	p.remoteDir = strings.TrimRight(getSetting[string](settings, "remote_dir", "/home/program/qq-workspace/remote"), "/")
	os.MkdirAll(p.remoteDir, 0755)

	p.httpClient = &http.Client{Timeout: 5 * time.Second}

	// 从 NapCat 获取 Bot 身份（阻塞等待，最多 5s）
	p.fetchBotInfo()
	if p.botID == 0 {
		log.Printf("[qq] warning: 获取 Bot 身份失败，群 @ 检查将拒绝所有未提及消息")
	}

	tp := p.name + "_"

	botInfo := ""
	if p.botNickname != "" {
		botInfo = fmt.Sprintf("你的QQ昵称是%s", p.botNickname)
		if p.botID > 0 {
			botInfo += fmt.Sprintf("，QQ号是%d", p.botID)
		}
		botInfo += "。"
	}

	// ---- 注册输出通道 ----
	s.RegisterOutputChannel("qq", sdk.CapText|sdk.CapFile|sdk.CapImage|sdk.CapAudio,
		`发送QQ群聊/私聊消息，支持文字、语音、图片、文件。
meta JSON 格式：
{
  "group_id": 123456,       // 群号（与 user_id 二选一）
  "user_id": 123456,        // QQ号（与 group_id 二选一）
  "reply_to": 12345         // 可选，回复指定消息 ID
}
type 枚举: text（文字）/ voice（语音转文字后发送）/ image（图片URL）/ file（文件URL）`,
		p.handleChannelOutput)

	// ---- 消息 ----
	p.regTool(s, tp+"get_message", botInfo+"获取QQ消息正文和详细信息。可通过local_id（中断消息中的id号）或message_id（NapCat消息ID，reply_to中的id）查找。返回消息的完整信息包括发送者、引用回复（reply_to）、图片/文件标记等。若消息有引用回复，建议再用qq_get_history拉取最近消息确认上下文。", map[string]interface{}{
		"type": "object", "properties": map[string]interface{}{
			"local_id":   map[string]interface{}{"type": "integer", "description": "本地消息ID（来自中断消息中的id号），与message_id二选一"},
			"message_id": map[string]interface{}{"type": "integer", "description": "NapCat消息ID（来自reply_to.message_id），与local_id二选一"},
		},
	}, p.handleGetMessage)

	p.regTool(s, tp+"send_file", "发送文件/图片到QQ（私聊或群聊）。文件先复制到remote目录供NapCat容器访问。", map[string]interface{}{
		"type": "object", "properties": map[string]interface{}{
			"group_id": map[string]interface{}{"type": "integer", "description": "目标群号（与user_id二选一）"},
			"user_id":  map[string]interface{}{"type": "integer", "description": "目标QQ号（与group_id二选一）"},
			"file":     map[string]interface{}{"type": "string", "description": "本地文件路径"},
			"name":     map[string]interface{}{"type": "string", "description": "文件名（可选，默认取原文件名）"},
			"as_image": map[string]interface{}{"type": "boolean", "description": "作为图片发送（true）还是作为文件（false，默认）"},
		},
	}, p.handleSendFile)

	p.regTool(s, tp+"get_history", "获取QQ群聊/私聊最近历史消息。当收到引用回复消息或需要了解对话上下文时应优先调用此工具查看前后文。返回值每条格式为 [时间] 发送者: 消息内容", map[string]interface{}{
		"type": "object", "properties": map[string]interface{}{
			"group_id": map[string]interface{}{"type": "integer", "description": "群号（与user_id二选一）"},
			"user_id":  map[string]interface{}{"type": "integer", "description": "QQ号私聊历史（与group_id二选一）"},
			"count":    map[string]interface{}{"type": "integer", "description": "拉取条数，默认10"},
		}, "required": []string{},
	}, p.handleGetHistory)

	// ---- 查询 ----
	p.regTool(s, tp+"get_groups", "获取QQ群列表，可按关键词搜索群名", map[string]interface{}{
		"type": "object", "properties": map[string]interface{}{
			"keyword": map[string]interface{}{"type": "string", "description": "搜索关键词（可选）"},
		},
	}, p.handleGetGroups)

	p.regTool(s, tp+"get_friends", "获取QQ好友列表，可按昵称/备注关键词搜索", map[string]interface{}{
		"type": "object", "properties": map[string]interface{}{
			"keyword": map[string]interface{}{"type": "string", "description": "搜索关键词（可选）"},
		},
	}, p.handleGetFriends)

	p.regTool(s, tp+"resolve_name", "将QQ号或群号解析为可读的用户昵称或群名称", map[string]interface{}{
		"type": "object", "properties": map[string]interface{}{
			"user_id": map[string]interface{}{"type": "integer", "description": "QQ号（与group_id二选一）"},
			"group_id": map[string]interface{}{"type": "integer", "description": "群号（与user_id二选一）"},
		},
	}, p.handleResolveName)

	p.regTool(s, tp+"resolve_nickname", "按昵称/备注/群名片搜索QQ用户，返回匹配的QQ号和详细信息。支持搜索好友列表或指定群成员。", map[string]interface{}{
		"type": "object", "properties": map[string]interface{}{
			"keyword":  map[string]interface{}{"type": "string", "description": "搜索关键词（昵称/备注/群名片）"},
			"group_id": map[string]interface{}{"type": "integer", "description": "所在群号（可选），不传则搜索好友列表"},
		}, "required": []string{"keyword"},
	}, p.handleResolveNickname)

	p.regTool(s, tp+"get_group_member_info", "获取QQ群成员详细信息", map[string]interface{}{
		"type": "object", "properties": map[string]interface{}{
			"group_id": map[string]interface{}{"type": "integer", "description": "群号"},
			"user_id":  map[string]interface{}{"type": "integer", "description": "QQ号"},
		}, "required": []string{"group_id", "user_id"},
	}, p.handleGetGroupMemberInfo)

	// ---- 群管理 ----
	p.regTool(s, tp+"group_manage", "QQ群综合管理。通过command参数执行各种操作：leave退群, kick踢人, ban禁言, unban解禁, rename改名, mute-all全员禁言, set-card设名片, set-admin设管理, set-title设头衔, member-list成员列表, group-info群详情, member-info成员详情, at-all-remain@全体剩余, msg-history消息历史, recall撤回, pin-msg精华, list-files文件列表, pending-requests待处理请求, folder-create创建文件夹。注意：leave/kick/ban/unban/mute-all/set-admin等破坏性操作必须先请示管理员确认后再执行。", map[string]interface{}{
		"type": "object", "properties": map[string]interface{}{
			"command":  map[string]interface{}{"type": "string", "description": "操作命令"},
			"group_id": map[string]interface{}{"type": "integer", "description": "群号"},
			"user_id":  map[string]interface{}{"type": "integer", "description": "QQ号（踢人/禁言/设名片等需要）"},
			"message_id": map[string]interface{}{"type": "integer", "description": "消息ID（撤回/精华）"},
			"name":     map[string]interface{}{"type": "string", "description": "群名称（rename）或文件夹名（folder-create）"},
			"card":     map[string]interface{}{"type": "string", "description": "群名片（set-card）"},
			"title":    map[string]interface{}{"type": "string", "description": "群头衔（set-title）"},
			"enable":   map[string]interface{}{"type": "boolean", "description": "启用/禁用（set-admin/mute-all）"},
			"minutes":  map[string]interface{}{"type": "integer", "description": "禁言分钟数（ban），0=解禁"},
			"count":    map[string]interface{}{"type": "integer", "description": "消息条数（msg-history），默认10"},
			"folder_id": map[string]interface{}{"type": "string", "description": "文件夹ID（list-files）"},
			"reject_add": map[string]interface{}{"type": "boolean", "description": "踢出时拒绝加群（kick）"},
			"confirm": map[string]interface{}{"type": "boolean", "description": "高风险操作确认标记。执行 leave/kick/ban/unban/rename/mute-all/set-card/set-admin/set-title/recall/pin-msg/folder-create 时必须传 true"},
		},
	}, p.handleGroupManage)

	p.regTool(s, tp+"friend_action", "QQ好友管理：delete删除好友, block拉黑（删好友+从所有群踢出+拒绝加群）, approve-friend同意好友请求, reject-friend拒绝好友请求, list-friends列出好友。注意：涉及删除/拉黑的操作必须请示管理员确认后再执行，未经授权不可操作。", map[string]interface{}{
		"type": "object", "properties": map[string]interface{}{
			"command": map[string]interface{}{"type": "string", "description": "操作: delete|block|approve-friend|reject-friend|list-friends"},
			"user_id": map[string]interface{}{"type": "integer", "description": "目标QQ号"},
			"flag":    map[string]interface{}{"type": "string", "description": "好友请求flag（approve-friend/reject-friend需要）"},
			"remark":  map[string]interface{}{"type": "string", "description": "好友备注（approve-friend可选）"},
			"group_id": map[string]interface{}{"type": "integer", "description": "仅从指定群踢出（block配合）"},
			"confirm": map[string]interface{}{"type": "boolean", "description": "高风险操作确认标记。执行 delete/block/approve-friend/reject-friend 时必须传 true"},
		},
	}, p.handleFriendAction)

	// ---- 文件 ----
	p.regTool(s, tp+"get_group_files", "查询群文件列表、搜索文件、下载文件到本地。操作: list列出, search搜索, download下载", map[string]interface{}{
		"type": "object", "properties": map[string]interface{}{
			"group_id": map[string]interface{}{"type": "integer", "description": "群号"},
			"command":  map[string]interface{}{"type": "string", "description": "操作: list|search|download"},
			"folder_id": map[string]interface{}{"type": "string", "description": "文件夹ID（list指定文件夹）"},
			"keyword":  map[string]interface{}{"type": "string", "description": "搜索关键词（search）"},
			"file_id":  map[string]interface{}{"type": "string", "description": "文件ID（download）"},
			"filename": map[string]interface{}{"type": "string", "description": "保存文件名（download可选）"},
		},
	}, p.handleGetGroupFiles)

	p.regTool(s, tp+"upload_group_file", "上传文件到QQ群（通过base64编码发送，同时出现在群消息和群文件柜）", map[string]interface{}{
		"type": "object", "properties": map[string]interface{}{
			"group_id": map[string]interface{}{"type": "integer", "description": "目标群号"},
			"file":     map[string]interface{}{"type": "string", "description": "本地文件路径"},
			"name":     map[string]interface{}{"type": "string", "description": "文件名（可选，默认取原文件名）"},
		}, "required": []string{"group_id", "file"},
	}, p.handleUploadGroupFile)

	// ---- 文档/视频/网页工具 ----
	p.regTool(s, tp+"read_document", "读取文档内容文本。支持 PDF、DOCX、DOC、XLSX、XLS、PPTX、PPT、TXT、CSV、MD 格式。使用 libreoffice + pandoc 转换提取文本，返回前 20000 字符。适合处理用户发来的文档文件。", map[string]interface{}{
		"type": "object", "properties": map[string]interface{}{
			"path": map[string]interface{}{"type": "string", "description": "文档文件路径（已保存到本地的文件路径）"},
		}, "required": []string{"path"},
	}, p.handleReadDocument)

	p.regTool(s, tp+"video_download", "下载视频到本地。支持 B站、YouTube 等主流视频网站（通过 yt-dlp）。先调用 info_only 查看视频信息，再下载。下载后文件保存在 agentfs 目录。", map[string]interface{}{
		"type": "object", "properties": map[string]interface{}{
			"url":       map[string]interface{}{"type": "string", "description": "视频分享链接"},
			"info_only": map[string]interface{}{"type": "boolean", "description": "仅获取视频信息（标题、时长、清晰度列表），不下"},
		}, "required": []string{"url"},
	}, p.handleVideoDownload)

	// ---- 附加 ----
	p.regTool(s, tp+"send_like", "给QQ好友点赞/戳一戳", map[string]interface{}{
		"type": "object", "properties": map[string]interface{}{
			"user_id": map[string]interface{}{"type": "integer", "description": "目标QQ号"},
			"times":   map[string]interface{}{"type": "integer", "description": "点赞次数1-20，默认1"},
		}, "required": []string{"user_id"},
	}, p.handleSendLike)

	s.RegisterStage(sdk.StageBeforeToolcall, p.beforeOwnToolcall, sdk.StageScopeOwnTools)

	// ---- HTTP server for NapCat webhook ----
	mux := http.NewServeMux()
	mux.HandleFunc("/", p.handleWebhook)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok"}`))
	})
	p.srv = &http.Server{Addr: p.listenAddr, Handler: mux}
	go func() {
		log.Printf("[qq] webhook %s napcat=%s", p.listenAddr, p.napcatURL)
		if err := p.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[qq] http: %v", err)
		}
	}()

	log.Printf("[qq] plugin started: %s (%d tools)", p.name, 15)
	return nil
}

func (p *Plugin) Stop() error {
	if p.srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		p.srv.Shutdown(ctx)
	}
	return nil
}

func (p *Plugin) regTool(s *sdk.PluginSDK, name, desc string, params map[string]interface{}, handler sdk.ToolHandler) {
	s.RegisterTool(name, sdk.ToolDef{Name: name, Description: desc, Parameters: params}, handler)
}

// ======== Bot Identity ========

func (p *Plugin) fetchBotInfo() {
	resp, err := p.rawNapcat("get_login_info", nil)
	if err != nil {
		log.Printf("[qq] fetch login info: %v", err)
		return
	}
	var info struct {
		Status   string `json:"status"`
		Data     *struct {
			UserID   int64  `json:"user_id"`
			Nickname string `json:"nickname"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(resp), &info); err != nil {
		log.Printf("[qq] parse login info: %v", err)
		return
	}
	if info.Data != nil {
		p.botID = info.Data.UserID
		p.botNickname = info.Data.Nickname
		log.Printf("[qq] bot identity: %s (%d)", p.botNickname, p.botID)
	}
}

// rawNapcat sends a request to NapCat and returns raw JSON string.
func (p *Plugin) rawNapcat(action string, params map[string]interface{}) (string, error) {
	data, _ := json.Marshal(params)
	url := fmt.Sprintf("%s/%s", p.napcatURL, action)
	resp, err := p.httpClient.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("napcat %s: %w", action, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(body), nil
}

// getSetting reads a setting from the SDK; returns fallback if unset or wrong type.
func getSetting[T string | int64 | float64](s sdk.SettingsAPI, key string, fallback T) T {
	v, err := s.Get(key)
	if err != nil || v == nil {
		return fallback
	}
	switch any(fallback).(type) {
	case string:
		if str, ok := v.(string); ok {
			return any(str).(T)
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

func normalizePolicy(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "allowlist":
		return "allowlist"
	case "disabled":
		return "disabled"
	default:
		return "open"
	}
}

func parseIDSet(raw string) map[int64]struct{} {
	out := make(map[int64]struct{})
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if n, err := strconv.ParseInt(part, 10, 64); err == nil {
			out[n] = struct{}{}
		}
	}
	return out
}

// isAtBot checks if the message contains an @-mention of the bot.
func (p *Plugin) isAtBot(msg interface{}) bool {
	segments, ok := msg.([]interface{})
	if !ok {
		return false
	}
	botIDStr := strconv.FormatInt(p.botID, 10)
	for _, seg := range segments {
		s, ok := seg.(map[string]interface{})
		if !ok {
			continue
		}
		if s["type"] == "at" {
			if data, ok := s["data"].(map[string]interface{}); ok {
				if qq, ok := data["qq"]; ok {
					switch v := qq.(type) {
					case string:
						if v == botIDStr || v == "all" {
							return true
						}
					case float64:
						if int64(v) == p.botID {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

// ======== Webhook ========

func (p *Plugin) isDMAllowed(userID int64) bool {
	switch p.dmPolicy {
	case "disabled":
		return false
	case "allowlist":
		_, ok := p.allowFrom[userID]
		return ok
	default:
		return true
	}
}

func (p *Plugin) isGroupAllowed(groupID int64) bool {
	switch p.groupPolicy {
	case "disabled":
		return false
	case "allowlist":
		_, ok := p.groupAllowFrom[groupID]
		return ok
		default:
		return true
	}
}

func (p *Plugin) beforeOwnToolcall(ctx *sdk.StageContext) error {
	ctx.Lock()
	defer ctx.Unlock()
	if len(ctx.ToolCalls) == 0 {
		return nil
	}
	tc := &ctx.ToolCalls[0]
	if tc.Name == p.name+"_send_file" || tc.Name == p.name+"_upload_group_file" {
		if file, ok := tc.Arguments["file"].(string); ok {
			tc.Arguments["file"] = p.sensitiveFilter(file)
		}
	}
	if tc.Name == p.name+"_group_manage" {
		cmd, _ := tc.Arguments["command"].(string)
		if requiresConfirmGroupCommand(cmd) {
			if ok, _ := tc.Arguments["confirm"].(bool); !ok {
				msg := fmt.Sprintf("QQ群管理命令 %s 属于高风险操作，必须显式传入 confirm=true 后才能执行", cmd)
				ctx.Response = &msg
				return nil
			}
		}
	}
	if tc.Name == p.name+"_friend_action" {
		cmd, _ := tc.Arguments["command"].(string)
		if requiresConfirmFriendCommand(cmd) {
			if ok, _ := tc.Arguments["confirm"].(bool); !ok {
				msg := fmt.Sprintf("QQ好友管理命令 %s 属于高风险操作，必须显式传入 confirm=true 后才能执行", cmd)
				ctx.Response = &msg
				return nil
			}
		}
	}
	return nil
}

func requiresConfirmGroupCommand(cmd string) bool {
	switch cmd {
	case "leave", "kick", "ban", "unban", "rename", "mute-all", "set-card", "set-admin", "set-title", "recall", "pin-msg", "folder-create":
		return true
	default:
		return false
	}
}

func requiresConfirmFriendCommand(cmd string) bool {
	switch cmd {
	case "delete", "block", "approve-friend", "reject-friend":
		return true
	default:
		return false
	}
}

func (p *Plugin) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "", http.StatusMethodNotAllowed)
		return
	}
	body, _ := io.ReadAll(r.Body)
	var evt struct {
		PostType    string      `json:"post_type"`
		MessageType string      `json:"message_type,omitempty"`
		MessageID   int64       `json:"message_id"`
		UserID      int64       `json:"user_id,omitempty"`
		GroupID     int64       `json:"group_id,omitempty"`
		RawMessage  string      `json:"raw_message,omitempty"`
		Message     interface{} `json:"message,omitempty"`
		Time        int64       `json:"time"`
		Sender      *struct {
			Nickname string `json:"nickname"`
			Card     string `json:"card,omitempty"`
		} `json:"sender,omitempty"`
	}
	if json.Unmarshal(body, &evt) != nil || evt.PostType != "message" {
		w.WriteHeader(http.StatusOK)
		return
	}

	rawCQ := evt.RawMessage
	text := rawCQ
	if text == "" {
		if s, ok := evt.Message.(string); ok {
			text = s
		}
	}
	if text == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// 提取引用回复和附件信息
	var replyToID int64
	var hasImage, hasFile bool
	var filePath string

	// 如果有结构化消息段，解析并下载文件/图片，生成可读文本
	if segments, ok := evt.Message.([]interface{}); ok && len(segments) > 0 {
		for _, seg := range segments {
			s, ok := seg.(map[string]interface{})
			if !ok {
				continue
			}
			typ, _ := s["type"].(string)
			data, _ := s["data"].(map[string]interface{})
			if data == nil {
				continue
			}
			switch typ {
			case "reply":
				if idStr, ok := data["id"].(string); ok {
					replyToID, _ = strconv.ParseInt(idStr, 10, 64)
				} else if id, ok := data["id"].(float64); ok {
					replyToID = int64(id)
				}
			case "image":
				hasImage = true
			case "file":
				hasFile = true
				if name, ok := data["name"].(string); ok && name != "" {
					filePath = name
				}
			}
		}
		parsedText := p.processMessageSegments(segments)
		if parsedText != "" {
			text = parsedText
		}
	}

	// 查找被引用的消息正文
	replyToText := ""
	if replyToID > 0 {
		for _, sm := range p.messages {
			if sm.MessageID == replyToID {
				replyToText = sm.Text
				break
			}
		}
	}

	if evt.MessageType == "private" {
		if !p.isDMAllowed(evt.UserID) {
			w.WriteHeader(http.StatusOK)
			return
		}
	}
	if evt.MessageType == "group" {
		if !p.isGroupAllowed(evt.GroupID) {
			w.WriteHeader(http.StatusOK)
			return
		}
		// 群消息必须 @ 机器人才响应
		if p.botID == 0 {
			log.Printf("[qq] bot ID unknown, rejecting group message from %d", evt.GroupID)
			w.WriteHeader(http.StatusOK)
			return
		}
		if !p.isAtBot(evt.Message) {
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	nickname := ""
	if evt.Sender != nil {
		nickname = evt.Sender.Nickname
		if evt.Sender.Card != "" {
			nickname = evt.Sender.Card
		}
	}

	p.mu.Lock()
	localID := p.nextID
	p.nextID++

	msg := &SavedMessage{
		LocalID: localID, MessageID: evt.MessageID, UserID: evt.UserID, Nickname: nickname,
		GroupID: evt.GroupID, MessageType: evt.MessageType, Text: text, RawText: rawCQ,
		ReplyToID: replyToID, ReplyToText: replyToText,
		HasImage: hasImage, HasFile: hasFile, FilePath: filePath,
		Time: evt.Time,
	}
	groupName := ""
	if evt.MessageType == "group" {
		if n, ok := p.groupNameCache[evt.GroupID]; ok {
			groupName = n
		} else {
			groupName = "群聊"
			go func(gid int64) {
				resp, err := p.napcat("get_group_info", map[string]interface{}{"group_id": gid})
				if err != nil {
					return
				}
				raw, _ := resp.(string)
				var gi struct {
					Data *struct {
						GroupName string `json:"group_name"`
					} `json:"data"`
				}
				if json.Unmarshal([]byte(raw), &gi) == nil && gi.Data != nil && gi.Data.GroupName != "" {
					p.mu.Lock()
					p.groupNameCache[gid] = gi.Data.GroupName
					p.mu.Unlock()
				}
			}(evt.GroupID)
		}
		msg.GroupName = groupName
	}
	p.messages = append(p.messages, msg)
	if len(p.messages) > maxMessages {
		p.messages = p.messages[1:]
	}

	tp := p.name + "_"
	outputTool := "output_send__" + p.name
	var interrupt string
	if evt.MessageType == "group" {
		interrupt = fmt.Sprintf("来自%s的（%s）群聊消息，通过id%d使用%sget_message工具获取消息正文。如果消息包含引用回复，使用%sget_history（group_id=%d）拉取最近消息以确认引用上下文。必须使用%s工具回复该群聊（用%s_help查看JSON格式要求），系统不会自动发送回复", nickname, groupName, localID, tp, tp, evt.GroupID, outputTool, outputTool)
	} else {
		interrupt = fmt.Sprintf("来自%s的私聊消息，通过id%d使用%sget_message工具获取消息正文。如果消息包含引用回复，使用%sget_history（user_id=%d）拉取最近消息以确认引用上下文。必须使用%s工具回复对方（用%s_help查看JSON格式要求），系统不会自动发送回复", nickname, localID, tp, tp, evt.UserID, outputTool, outputTool)
	}
	if p.adminID > 0 && evt.UserID == p.adminID {
		interrupt = "【重要！老大消息】" + interrupt
	}
	p.mu.Unlock()

	if evt.MessageType == "group" && p.sdk != nil {
		rulesRaw := getSetting[string](p.sdk.Settings(), "forward_rules", "[]")
		var rules []ForwardRule
		if json.Unmarshal([]byte(rulesRaw), &rules) == nil {
			for _, rule := range rules {
				if evt.GroupID == rule.GroupID {
					mcMsg := fmt.Sprintf("%s 说 %s", nickname, text)
					go func(r ForwardRule, msg string) {
						if err := rconSend(r.Host, r.Port, r.Password, "say "+msg); err != nil {
							log.Printf("[qq] rcon forward to %s:%d: %v", r.Host, r.Port, err)
						}
					}(rule, mcMsg)
				}
			}
		}
	}

	if p.sdk != nil {
		p.sdk.InjectInterruptText(p.name, p.name, interrupt)
	}
	w.WriteHeader(http.StatusOK)
}

// ======== Tool Handlers ========

func (p *Plugin) handleGetMessage(args map[string]interface{}) (interface{}, error) {
	id, err := convInt64(args["local_id"])
	msgID, msgIDErr := convInt64(args["message_id"])

	if err != nil && msgIDErr != nil {
		return map[string]interface{}{
			"content":   "需要提供 local_id 或 message_id 参数",
			"not_found": true,
		}, nil
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	var found *SavedMessage
	for _, m := range p.messages {
		if err == nil && m.LocalID == id {
			found = m
			break
		}
		if msgIDErr == nil && m.MessageID == msgID {
			found = m
			break
		}
	}

	if found == nil {
		key := id
		if err != nil {
			key = msgID
		}
		return map[string]interface{}{
			"content":   fmt.Sprintf("消息 %d 未找到。消息可能已被处理过期，或插件重启后本地缓存已清空。请使用 qq_get_history 从 NapCat 拉取历史消息。", key),
			"local_id":  id,
			"message_id": msgID,
			"not_found": true,
		}, nil
	}

	from := found.Nickname
	if found.GroupName != "" && found.GroupName != fmt.Sprintf("%d", found.GroupID) {
		from = fmt.Sprintf("%s（%s）", found.Nickname, found.GroupName)
	}
	loc := "私聊"
	if found.MessageType == "group" {
		loc = "群聊"
	}
	result := map[string]interface{}{
		"content":      found.Text,
		"local_id":     found.LocalID,
		"message_id":   found.MessageID,
		"from":         from,
		"type":         loc,
		"user_id":      found.UserID,
		"group_id":     found.GroupID,
		"nickname":     found.Nickname,
		"message_type": found.MessageType,
		"time":         time.Unix(found.Time, 0).Format("15:04:05"),
	}
	if found.RawText != "" {
		result["raw_text"] = found.RawText
	}
	if found.ReplyToID > 0 {
		replyInfo := map[string]interface{}{
			"message_id": found.ReplyToID,
		}
		if found.ReplyToText != "" {
			replyInfo["content"] = found.ReplyToText
		}
		// 在本机缓存中查找被引用的消息 local_id
		for _, sm := range p.messages {
			if sm.MessageID == found.ReplyToID {
				replyInfo["local_id"] = sm.LocalID
				break
			}
		}
		result["reply_to"] = replyInfo
	}
	if found.HasImage {
		result["has_image"] = true
	}
	if found.HasFile {
		result["has_file"] = true
	}
	if found.FilePath != "" {
		result["file_name"] = found.FilePath
	}

	return result, nil
}

// handleChannelOutput — output_send(channel="qq") 的处理器
// args 包含 payload, type, (可选 meta)
func (p *Plugin) handleChannelOutput(args map[string]interface{}) (interface{}, error) {
	payload, _ := args["payload"].(string)
	rawType, _ := args["type"].(string)
	meta, _ := args["meta"].(string)
	log.Printf("[qq] handleChannelOutput payload=%q type=%s meta=%s", payload, rawType, meta)
	if payload == "" || rawType == "" {
		return nil, fmt.Errorf("payload 和 type 参数不能为空")
	}

	// 解析 meta
	var groupID, userID int64
	var replyTo int64
	if meta != "" {
		var m struct {
			GroupID int64 `json:"group_id"`
			UserID  int64 `json:"user_id"`
			ReplyTo int64 `json:"reply_to"`
		}
		if err := json.Unmarshal([]byte(meta), &m); err == nil {
			groupID = m.GroupID
			userID = m.UserID
			replyTo = m.ReplyTo
		}
	}
	if groupID == 0 && userID == 0 {
		return nil, fmt.Errorf("meta 中需要 group_id 或 user_id 字段。用 output_send__qq_help 查看格式说明")
	}

	switch rawType {
	case "text":
		text := p.sensitiveFilter(payload)
		msg := map[string]interface{}{"message": text}
		if groupID != 0 {
			msg["group_id"] = groupID
		} else {
			msg["user_id"] = userID
		}
		if replyTo > 0 {
			msg["reply_to"] = replyTo
		}
		if groupID != 0 {
			return p.napcat("send_group_msg", msg)
		}
		return p.napcat("send_private_msg", msg)

	case "voice", "audio":
		text := p.sensitiveFilter(payload)
		audioFile, err := p.ttsToFile(text)
		if err != nil {
			return nil, fmt.Errorf("语音生成失败: %w", err)
		}
		os.MkdirAll(p.remoteDir, 0755)
		dest := filepath.Join(p.remoteDir, filepath.Base(audioFile))
		data, err := os.ReadFile(audioFile)
		if err != nil {
			return nil, fmt.Errorf("读取音频文件失败: %w", err)
		}
		if err := os.WriteFile(dest, data, 0644); err != nil {
			return nil, fmt.Errorf("写入共享目录失败: %w", err)
		}
		os.Remove(audioFile)
		uri := fmt.Sprintf("file:///app/files/%s", filepath.Base(dest))
		cqMsg := fmt.Sprintf("[CQ:record,file=%s]", uri)
		msg := map[string]interface{}{"message": cqMsg}
		if groupID != 0 {
			msg["group_id"] = groupID
		} else {
			msg["user_id"] = userID
		}
		if groupID != 0 {
			return p.napcat("send_group_msg", msg)
		}
		return p.napcat("send_private_msg", msg)

	case "image":
		msg := map[string]interface{}{"message": fmt.Sprintf("[CQ:image,file=%s]", payload)}
		if groupID != 0 {
			msg["group_id"] = groupID
		} else {
			msg["user_id"] = userID
		}
		if groupID != 0 {
			return p.napcat("send_group_msg", msg)
		}
		return p.napcat("send_private_msg", msg)

	case "file":
		msg := map[string]interface{}{"message": fmt.Sprintf("[CQ:file,file=%s]", payload)}
		if groupID != 0 {
			msg["group_id"] = groupID
		} else {
			msg["user_id"] = userID
		}
		if groupID != 0 {
			return p.napcat("send_group_msg", msg)
		}
		return p.napcat("send_private_msg", msg)

	default:
		return nil, fmt.Errorf("不支持的 type: %s（枚举值: text/voice/image/file）", rawType)
	}
}

func (p *Plugin) buildOutputHelp() string {
	return `【参数】
payload — 消息载荷。type=text时直接填文字，type=voice时填文字（自动转语音），type=image/file时填URL
meta    — JSON 元数据，必含 group_id（群聊）或 user_id（私聊），可选 reply_to
type    — text / voice / image / file

【示例】
群聊文字：output_send__qq(payload="你好", meta="{\"group_id\":123456789}", type="text")
私聊语音：output_send__qq(payload="你好", meta="{\"user_id\":123456789}", type="voice")
发送图片：output_send__qq(payload="https://example.com/img.jpg", meta="{\"group_id\":123456789}", type="image")

【注意】
- type=text 时 payload 直接是文字，无需 JSON 包裹
- type=voice 时 payload 是文字内容，自动转语音发送
- type=image/file 时 payload 是 URL 或路径`
}

// ttsToFile 用 edge-tts 将文本转为音频文件，返回临时文件路径
func (p *Plugin) ttsToFile(text string) (string, error) {
	// 清理文本中的特殊字符
	clean := strings.Map(func(r rune) rune {
		if r == '"' || r == '\n' || r == '\r' {
			return ' '
		}
		return r
	}, text)
	clean = strings.TrimSpace(clean)
	if clean == "" {
		clean = " "
	}

	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("qq_tts_%d.mp3", time.Now().UnixNano()))
	cmd := exec.Command("edge-tts",
		"--voice", "zh-CN-XiaoxiaoNeural",
		"--text", clean,
		"--write-media", tmpFile,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("edge-tts: %w\nstderr: %s", err, stderr.String())
	}
	if _, err := os.Stat(tmpFile); os.IsNotExist(err) {
		return "", fmt.Errorf("edge-tts 未生成输出文件")
	}
	return tmpFile, nil
}

func (p *Plugin) handleSendFile(args map[string]interface{}) (interface{}, error) {
	gid, gerr := convInt64(args["group_id"])
	uid, uerr := convInt64(args["user_id"])
	if gerr != nil && uerr != nil {
		return nil, fmt.Errorf("need group_id or user_id")
	}
	filePath, _ := args["file"].(string)
	if filePath == "" {
		return nil, fmt.Errorf("need file path")
	}
	name, _ := args["name"].(string)
	if name == "" {
		name = filepath.Base(filePath)
	}
	name = p.sensitiveFilter(name)
	asImage, _ := args["as_image"].(bool)

	// copy to remote dir for NapCat container access
	dest := filepath.Join(p.remoteDir, name)
	srcData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	if err := os.WriteFile(dest, srcData, 0644); err != nil {
		return nil, fmt.Errorf("write remote: %w", err)
	}

	uri := fmt.Sprintf("file:///app/files/%s", name)
	var cqMsg string
	if asImage {
		cqMsg = fmt.Sprintf("[CQ:image,file=%s]", uri)
	} else {
		cqMsg = fmt.Sprintf("[CQ:file,file=%s,title=%s]", uri, name)
	}

	params := map[string]interface{}{"message": cqMsg}
	if gerr == nil {
		params["group_id"] = gid
		return p.napcat("send_group_msg", params)
	}
	params["user_id"] = uid
	return p.napcat("send_private_msg", params)
}

func (p *Plugin) handleGetHistory(args map[string]interface{}) (interface{}, error) {
	gid, gerr := convInt64(args["group_id"])
	uid, uerr := convInt64(args["user_id"])
	count := 10
	if c, err := convInt64(args["count"]); err == nil && c > 0 {
		count = int(c)
	}

	var endpoint string
	var params map[string]interface{}
	if gerr == nil {
		endpoint = "get_group_msg_history"
		params = map[string]interface{}{"group_id": gid, "count": count}
	} else if uerr == nil {
		endpoint = "get_friend_msg_history"
		params = map[string]interface{}{"user_id": uid, "count": count}
	} else {
		return nil, fmt.Errorf("need group_id or user_id")
	}

	rawResp, err := p.napcat(endpoint, params)
	if err != nil {
		return nil, err
	}
	rawStr, _ := rawResp.(string)
	if rawStr == "" {
		return map[string]interface{}{"messages": []interface{}{}, "note": "未获取到历史消息"}, nil
	}

	// 解析 NapCat 响应，提取消息列表
	var resp struct {
		Data *struct {
			Messages []interface{} `json:"messages"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(rawStr), &resp); err != nil || resp.Data == nil {
		return map[string]interface{}{"raw_response": rawStr, "note": "解析 NapCat 响应失败"}, nil
	}

	// 格式化消息为可读文本
	var lines []string
	for _, m := range resp.Data.Messages {
		msg, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		sender := ""
		if s, ok := msg["sender"].(map[string]interface{}); ok {
			if nick, _ := s["nickname"].(string); nick != "" {
				sender = nick
			}
			if card, _ := s["card"].(string); card != "" {
				sender = card
			}
		}
		msgText, _ := msg["raw_message"].(string)
		if msgText == "" {
			msgText, _ = msg["message"].(string)
		}
		if msgText == "" {
			continue
		}
		ts := ""
		if t, ok := msg["time"].(float64); ok {
			ts = time.Unix(int64(t), 0).Format("15:04")
		}
		line := msgText
		if sender != "" {
			line = sender + ": " + msgText
		}
		if ts != "" {
			line = "[" + ts + "] " + line
		}
		lines = append(lines, line)
	}

	if len(lines) == 0 {
		return map[string]interface{}{
			"messages": []interface{}{},
			"note":     "未找到历史消息，可能群内暂无消息记录",
		}, nil
	}

	return map[string]interface{}{
		"messages": lines,
		"count":    len(lines),
	}, nil
}

func (p *Plugin) handleGetGroups(args map[string]interface{}) (interface{}, error) {
	return p.napcat("get_group_list", map[string]interface{}{})
}

func (p *Plugin) handleGetFriends(args map[string]interface{}) (interface{}, error) {
	return p.napcat("get_friend_list", map[string]interface{}{})
}

func (p *Plugin) handleResolveName(args map[string]interface{}) (interface{}, error) {
	if uid, err := convInt64(args["user_id"]); err == nil {
		return p.napcat("get_stranger_info", map[string]interface{}{"user_id": uid, "no_cache": true})
	}
	if gid, err := convInt64(args["group_id"]); err == nil {
		return p.napcat("get_group_info", map[string]interface{}{"group_id": gid, "no_cache": true})
	}
	return nil, fmt.Errorf("need user_id or group_id")
}

func (p *Plugin) handleResolveNickname(args map[string]interface{}) (interface{}, error) {
	keyword, _ := args["keyword"].(string)
	if keyword == "" {
		return nil, fmt.Errorf("keyword is required")
	}
	keyword = strings.ToLower(keyword)

	gid, 	groupErr := convInt64(args["group_id"])
	if groupErr == nil {
		v, err := p.napcat("get_group_member_list", map[string]interface{}{"group_id": gid})
		if err != nil {
			return nil, err
		}
		raw, _ := v.(string)
		return filterMemberList(raw, keyword)
	}

	v, err := p.napcat("get_friend_list", map[string]interface{}{})
	if err != nil {
		return nil, err
	}
	raw, _ := v.(string)
	return filterFriendList(raw, keyword)
}

func filterFriendList(raw, keyword string) (interface{}, error) {
	var resp struct {
		Data []struct {
			UserID   int64  `json:"user_id"`
			Nickname string `json:"nickname"`
			Remark   string `json:"remark"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return raw, nil
	}
	var matches []map[string]interface{}
	for _, f := range resp.Data {
		if strings.Contains(strings.ToLower(f.Nickname), keyword) ||
			strings.Contains(strings.ToLower(f.Remark), keyword) {
			matches = append(matches, map[string]interface{}{
				"user_id":  f.UserID,
				"nickname": f.Nickname,
				"remark":   f.Remark,
			})
		}
	}
	if len(matches) == 0 {
		return fmt.Sprintf("未找到昵称/备注包含 %q 的好友", keyword), nil
	}
	return matches, nil
}

func filterMemberList(raw, keyword string) (interface{}, error) {
	var resp struct {
		Data []struct {
			UserID   int64  `json:"user_id"`
			Nickname string `json:"nickname"`
			Card     string `json:"card"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return raw, nil
	}
	var matches []map[string]interface{}
	for _, m := range resp.Data {
		if strings.Contains(strings.ToLower(m.Nickname), keyword) ||
			strings.Contains(strings.ToLower(m.Card), keyword) {
			matches = append(matches, map[string]interface{}{
				"user_id":  m.UserID,
				"nickname": m.Nickname,
				"card":     m.Card,
			})
		}
	}
	if len(matches) == 0 {
		return fmt.Sprintf("未找到昵称/名片包含 %q 的群成员", keyword), nil
	}
	return matches, nil
}

func (p *Plugin) handleGetGroupMemberInfo(args map[string]interface{}) (interface{}, error) {
	gid, _ := convInt64(args["group_id"])
	uid, _ := convInt64(args["user_id"])
	return p.napcat("get_group_member_info", map[string]interface{}{"group_id": gid, "user_id": uid})
}

func (p *Plugin) handleGroupManage(args map[string]interface{}) (interface{}, error) {
	cmd, _ := args["command"].(string)
	if cmd == "" {
		return nil, fmt.Errorf("need command")
	}
	if requiresConfirmGroupCommand(cmd) {
		if ok, _ := args["confirm"].(bool); !ok {
			return map[string]interface{}{"isError": true, "content": fmt.Sprintf("高风险操作 %s 需要 confirm=true", cmd)}, nil
		}
	}

	switch cmd {
	case "group-list":
		return p.napcat("get_group_list", map[string]interface{}{})
	case "group-info", "member-list", "member-info", "at-all-remain", "msg-history":
		gid, _ := convInt64(args["group_id"])
		if cmd == "msg-history" {
			count := 10
			if c, err := convInt64(args["count"]); err == nil && c > 0 {
				count = int(c)
			}
			return p.napcat("get_group_msg_history", map[string]interface{}{"group_id": gid, "count": count})
		}
		if cmd == "member-info" {
			uid, _ := convInt64(args["user_id"])
			return p.napcat("get_group_member_info", map[string]interface{}{"group_id": gid, "user_id": uid})
		}
		if cmd == "at-all-remain" {
			return p.napcat("get_group_at_all_remain", map[string]interface{}{"group_id": gid})
		}
		if cmd == "group-info" {
			return p.napcat("get_group_info", map[string]interface{}{"group_id": gid})
		}
		return p.napcat("get_group_member_list", map[string]interface{}{"group_id": gid})

	case "list-files":
		gid, _ := convInt64(args["group_id"])
		folderID, _ := args["folder_id"].(string)
		if folderID != "" {
			return p.napcat("get_group_files_by_folder", map[string]interface{}{"group_id": gid, "folder_id": folderID})
		}
		return p.napcat("get_group_root_files", map[string]interface{}{"group_id": gid})

	case "pending-requests":
		return p.napcat("get_group_system_msg", map[string]interface{}{})

	case "leave":
		gid, _ := convInt64(args["group_id"])
		return p.napcat("set_group_leave", map[string]interface{}{"group_id": gid})

	case "kick":
		gid, _ := convInt64(args["group_id"])
		uid, _ := convInt64(args["user_id"])
		reject, _ := args["reject_add"].(bool)
		return p.napcat("set_group_kick", map[string]interface{}{"group_id": gid, "user_id": uid, "reject_add_request": reject})

	case "ban":
		gid, _ := convInt64(args["group_id"])
		uid, _ := convInt64(args["user_id"])
		minutes := 10
		if m, err := convInt64(args["minutes"]); err == nil {
			minutes = int(m)
		}
		return p.napcat("set_group_ban", map[string]interface{}{"group_id": gid, "user_id": uid, "duration": minutes * 60})

	case "unban":
		gid, _ := convInt64(args["group_id"])
		uid, _ := convInt64(args["user_id"])
		return p.napcat("set_group_ban", map[string]interface{}{"group_id": gid, "user_id": uid, "duration": 0})

	case "rename":
		gid, _ := convInt64(args["group_id"])
		name, _ := args["name"].(string)
		return p.napcat("set_group_name", map[string]interface{}{"group_id": gid, "group_name": name})

	case "mute-all":
		gid, _ := convInt64(args["group_id"])
		enable, _ := args["enable"].(bool)
		return p.napcat("set_group_whole_ban", map[string]interface{}{"group_id": gid, "enable": enable})

	case "set-card":
		gid, _ := convInt64(args["group_id"])
		uid, _ := convInt64(args["user_id"])
		card, _ := args["card"].(string)
		return p.napcat("set_group_card", map[string]interface{}{"group_id": gid, "user_id": uid, "card": card})

	case "set-admin":
		gid, _ := convInt64(args["group_id"])
		uid, _ := convInt64(args["user_id"])
		enable, _ := args["enable"].(bool)
		return p.napcat("set_group_admin", map[string]interface{}{"group_id": gid, "user_id": uid, "enable": enable})

	case "set-title":
		gid, _ := convInt64(args["group_id"])
		uid, _ := convInt64(args["user_id"])
		title, _ := args["title"].(string)
		return p.napcat("set_group_special_title", map[string]interface{}{"group_id": gid, "user_id": uid, "special_title": title})

	case "recall":
		mid, _ := convInt64(args["message_id"])
		return p.napcat("delete_msg", map[string]interface{}{"message_id": mid})

	case "pin-msg":
		mid, _ := convInt64(args["message_id"])
		return p.napcat("set_essence_msg", map[string]interface{}{"message_id": mid})

	case "folder-create":
		gid, _ := convInt64(args["group_id"])
		name, _ := args["name"].(string)
		return p.napcat("create_group_file_folder", map[string]interface{}{"group_id": gid, "name": name})

	default:
		return nil, fmt.Errorf("unknown group_manage command: %s", cmd)
	}
}

func (p *Plugin) handleFriendAction(args map[string]interface{}) (interface{}, error) {
	cmd, _ := args["command"].(string)
	if requiresConfirmFriendCommand(cmd) {
		if ok, _ := args["confirm"].(bool); !ok {
			return map[string]interface{}{"isError": true, "content": fmt.Sprintf("高风险操作 %s 需要 confirm=true", cmd)}, nil
		}
	}
	switch cmd {
	case "list-friends":
		return p.napcat("get_friend_list", map[string]interface{}{})
	case "delete":
		uid, _ := convInt64(args["user_id"])
		return p.napcat("delete_friend", map[string]interface{}{"user_id": uid})
	case "block":
		uid, _ := convInt64(args["user_id"])
		// delete friend
		p.napcat("delete_friend", map[string]interface{}{"user_id": uid})
		// kick from groups
		if gid, err := convInt64(args["group_id"]); err == nil {
			p.napcat("set_group_kick", map[string]interface{}{"group_id": gid, "user_id": uid, "reject_add_request": true})
		} else {
			grps, _ := p.napcat("get_group_list", map[string]interface{}{})
			if list, ok := grps.([]interface{}); ok {
				for _, g := range list {
					if m, ok := g.(map[string]interface{}); ok {
						if gid, ok := m["group_id"].(float64); ok {
							p.napcat("set_group_kick", map[string]interface{}{"group_id": int64(gid), "user_id": uid, "reject_add_request": true})
						}
					}
				}
			}
		}
		return `{"status":"ok","message":"blocked"}`, nil
	case "approve-friend":
		flag, _ := args["flag"].(string)
		remark, _ := args["remark"].(string)
		return p.napcat("set_friend_add_request", map[string]interface{}{"flag": flag, "approve": true, "remark": remark})
	case "reject-friend":
		flag, _ := args["flag"].(string)
		return p.napcat("set_friend_add_request", map[string]interface{}{"flag": flag, "approve": false})
	default:
		return nil, fmt.Errorf("unknown friend_action command: %s", cmd)
	}
}

func (p *Plugin) handleGetGroupFiles(args map[string]interface{}) (interface{}, error) {
	gid, _ := convInt64(args["group_id"])
	cmd, _ := args["command"].(string)

	switch cmd {
	case "list":
		folderID, _ := args["folder_id"].(string)
		if folderID != "" {
			return p.napcat("get_group_files_by_folder", map[string]interface{}{"group_id": gid, "folder_id": folderID})
		}
		return p.napcat("get_group_root_files", map[string]interface{}{"group_id": gid})

	case "search":
		return p.napcat("get_group_root_files", map[string]interface{}{"group_id": gid})

	case "download":
		fileID, _ := args["file_id"].(string)
		filename, _ := args["filename"].(string)
		if filename == "" {
			filename = fmt.Sprintf("group_file_%s", fileID)
		}
		// get download URL
		resp, err := p.napcat("get_group_file_url", map[string]interface{}{"group_id": gid, "file_id": fileID})
		if err != nil {
			return nil, err
		}
		respStr, ok := resp.(string)
		if !ok {
			return resp, nil
		}
		// parse URL from response
		var parsed struct {
			Data struct {
				URL string `json:"url"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(respStr), &parsed); err != nil || parsed.Data.URL == "" {
			return resp, nil
		}
		dlURL := parsed.Data.URL
		httpResp, err := http.Get(dlURL)
		if err != nil {
			return nil, fmt.Errorf("download: %w", err)
		}
		defer httpResp.Body.Close()
		content, err := io.ReadAll(httpResp.Body)
		if err != nil {
			return nil, fmt.Errorf("read download: %w", err)
		}
		os.MkdirAll(p.filesDir, 0755)
		savePath := filepath.Join(p.filesDir, filename)
		if err := os.WriteFile(savePath, content, 0644); err != nil {
			return nil, fmt.Errorf("save: %w", err)
		}
		return map[string]interface{}{
			"status": "ok", "path": savePath, "filename": filename, "size": len(content),
		}, nil

	default:
		return nil, fmt.Errorf("unknown get_group_files command: %s", cmd)
	}
}

func (p *Plugin) handleUploadGroupFile(args map[string]interface{}) (interface{}, error) {
	gid, _ := convInt64(args["group_id"])
	filePath, _ := args["file"].(string)
	name, _ := args["name"].(string)
	if name == "" {
		name = filepath.Base(filePath)
	}
	name = p.sensitiveFilter(name)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	b64 := fmt.Sprintf("base64://%s", base64.StdEncoding.EncodeToString(data))

	resp, err := p.napcat("send_group_msg", map[string]interface{}{
		"group_id": gid,
		"message": []map[string]interface{}{
			{"type": "file", "data": map[string]interface{}{"file": b64, "name": name}},
		},
	})
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"status": "ok", "file": name, "napcat": resp}, nil
}

func (p *Plugin) handleSendLike(args map[string]interface{}) (interface{}, error) {
	uid, _ := convInt64(args["user_id"])
	times := 1
	if t, err := convInt64(args["times"]); err == nil && t > 0 && t <= 20 {
		times = int(t)
	}
	return p.napcat("send_like", map[string]interface{}{"user_id": uid, "times": times})
}

// ======== CQ Code / Message Segment Processing ========

func (p *Plugin) processMessageSegments(segments []interface{}) string {
	if len(segments) == 0 {
		return ""
	}
	os.MkdirAll(p.filesDir, 0755)
	botIDStr := strconv.FormatInt(p.botID, 10)
	var parts []string
	type dlItem struct{ fileID, name, url string }
	var dlQueue []dlItem

	for _, seg := range segments {
		s, ok := seg.(map[string]interface{})
		if !ok {
			continue
		}
		typ, _ := s["type"].(string)
		data, _ := s["data"].(map[string]interface{})
		if data == nil {
			continue
		}

		switch typ {
		case "text":
			if t, _ := data["text"].(string); t != "" {
				parts = append(parts, t)
			}
		case "at":
			qq, _ := data["qq"].(string)
			if qq == "all" {
				parts = append(parts, "@所有人")
			} else if qq == botIDStr {
				continue
			} else {
				parts = append(parts, "@"+qq)
			}
		case "face", "sface":
			if id, _ := data["id"].(string); id != "" {
				parts = append(parts, "[表情]")
			}
		case "file":
			fid, _ := data["file"].(string)
			name, _ := data["name"].(string)
			size, _ := data["size"].(string)
			sizeDesc := ""
			if s, err := strconv.ParseInt(size, 10, 64); err == nil && s > 0 {
				sizeDesc = fmt.Sprintf(" (%.1f MB)", float64(s)/1048576)
			}
			if fid != "" {
				dlQueue = append(dlQueue, dlItem{fileID: fid, name: name})
			}
			if name != "" {
				parts = append(parts, fmt.Sprintf("[文件:%s%s]", name, sizeDesc))
			} else {
				parts = append(parts, "[文件]")
			}
		case "image":
			fid, _ := data["file"].(string)
			summary, _ := data["summary"].(string)
			imgURL, _ := data["url"].(string)
			if fid != "" {
				dlQueue = append(dlQueue, dlItem{fileID: fid, name: "image_" + fid + ".jpg", url: imgURL})
			} else if imgURL != "" {
				dlQueue = append(dlQueue, dlItem{url: imgURL, name: "image_" + filepath.Base(imgURL)})
			}
			label := "图片"
			if summary != "" {
				label = summary
			}
			parts = append(parts, fmt.Sprintf("[%s]", label))
		case "video":
			fid, _ := data["file"].(string)
			videoURL, _ := data["url"].(string)
			if fid != "" {
				dlQueue = append(dlQueue, dlItem{fileID: fid, name: "video_" + fid + ".mp4", url: videoURL})
			} else if videoURL != "" {
				dlQueue = append(dlQueue, dlItem{url: videoURL, name: "video_" + filepath.Base(videoURL)})
			}
			parts = append(parts, "[视频]")
		case "reply":
			if id, ok := data["id"].(float64); ok {
				parts = append(parts, fmt.Sprintf("[回复消息id=%.0f]", id))
			}
		case "music":
			if title, _ := data["title"].(string); title != "" {
				parts = append(parts, fmt.Sprintf("[音乐:%s]", title))
			} else {
				parts = append(parts, "[音乐]")
			}
		case "share":
			title, _ := data["title"].(string)
			urlStr, _ := data["url"].(string)
			if title != "" && urlStr != "" {
				parts = append(parts, fmt.Sprintf("[分享:%s %s]", title, urlStr))
			} else if urlStr != "" {
				parts = append(parts, fmt.Sprintf("[分享:%s]", urlStr))
			}
		default:
			if typ != "" {
				parts = append(parts, "["+typ+"]")
			}
		}
	}

	// 异步下载文件（不影响消息处理）
	if len(dlQueue) > 0 {
		go func(items []dlItem) {
			for _, item := range items {
				p.downloadFile(item.fileID, item.name, item.url)
			}
		}(dlQueue)
	}

	return strings.TrimSpace(strings.Join(parts, " "))
}

func (p *Plugin) downloadFile(fileID, filename, fileURL string) string {
	if p.filesDir == "" {
		return ""
	}
	os.MkdirAll(p.filesDir, 0755)

	// 优先使用 URL 直下（NapCat 消息 data 中的 url 字段）
	if fileURL != "" {
		if filename == "" {
			filename = "file_" + filepath.Base(fileURL)
		}
		filename = sanitizeFilename(filename)
		localPath := filepath.Join(p.filesDir, filename)
		dlResp, err := p.httpClient.Get(fileURL)
		if err == nil {
			defer dlResp.Body.Close()
			data, err := io.ReadAll(dlResp.Body)
			if err == nil && len(data) > 0 {
				os.WriteFile(localPath, data, 0644)
				return localPath
			}
		}
	}

	if fileID == "" {
		return ""
	}

	// 处理 base64:// 前缀的内嵌文件
	if strings.HasPrefix(fileID, "base64://") {
		data, err := base64.StdEncoding.DecodeString(fileID[9:])
		if err != nil {
			return ""
		}
		if filename == "" {
			filename = "file.bin"
		}
		filename = sanitizeFilename(filename)
		localPath := filepath.Join(p.filesDir, filename)
		os.WriteFile(localPath, data, 0644)
		return localPath
	}

	// 处理 file:// 路径
	if strings.HasPrefix(fileID, "file://") {
		localFile := strings.TrimPrefix(fileID, "file://")
		if _, err := os.Stat(localFile); err == nil {
			return localFile
		}
	}

	// 通过 NapCat get_file API 获取文件信息
	raw, err := p.napcat("get_file", map[string]interface{}{"file_id": fileID})
	if err != nil {
		log.Printf("[qq] get_file %s: %v", fileID, err)
		return ""
	}
	rawStr, _ := raw.(string)
	var resp struct {
		Data *struct {
			File     string `json:"file"`
			FileName string `json:"file_name"`
			FileSize int64  `json:"file_size"`
			Base64   string `json:"base64"`
			URL      string `json:"url"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(rawStr), &resp) != nil || resp.Data == nil {
		log.Printf("[qq] get_file %s: bad response (NapCat returned no data, fileID=%q url=%q)", fileID, fileID, fileURL)
		return ""
	}
	info := resp.Data

	if filename == "" {
		filename = info.FileName
	}
	if filename == "" {
		filename = "file_" + fileID
	}
	filename = sanitizeFilename(filename)
	localPath := filepath.Join(p.filesDir, filename)

	// 优先 base64
	if info.Base64 != "" {
		data, err := base64.StdEncoding.DecodeString(info.Base64)
		if err == nil {
			os.WriteFile(localPath, data, 0644)
			return localPath
		}
	}

	// 其次 URL 下载
	if info.URL != "" {
		dlResp, err := p.httpClient.Get(info.URL)
		if err == nil {
			defer dlResp.Body.Close()
			data, err := io.ReadAll(dlResp.Body)
			if err == nil {
				os.WriteFile(localPath, data, 0644)
				return localPath
			}
		}
	}

	// 尝试直接读取 file 路径
	if info.File != "" {
		src, err := os.ReadFile(info.File)
		if err == nil {
			os.WriteFile(localPath, src, 0644)
			return localPath
		}
	}

	return ""
}

func sanitizeFilename(name string) string {
	name = filepath.Base(name)
	name = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			return '_'
		}
		return r
	}, name)
	return name
}

// ======== Tool Handlers: Document / Video / Web ========

func (p *Plugin) handleReadDocument(args map[string]interface{}) (interface{}, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return map[string]interface{}{
			"content": fmt.Sprintf("文件不存在: %s", path),
		}, nil
	}

	ext := strings.ToLower(filepath.Ext(path))
	textContent := ""

	switch ext {
	case ".txt", ".md", ".csv":
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read file: %w", err)
		}
		textContent = string(data)
	case ".docx", ".doc", ".epub", ".html", ".htm":
		textContent = p.readWithPandoc(path)
	default:
		// Try pandoc first, fallback to libreoffice
		textContent = p.readWithPandoc(path)
		if textContent == "" {
			textContent = p.readWithLibreoffice(path)
		}
		if textContent == "" {
			// last resort: read as plain text
			data, err := os.ReadFile(path)
			if err == nil {
				textContent = string(data)
			}
		}
	}

	if textContent == "" {
		return map[string]interface{}{
			"content": fmt.Sprintf("无法提取文件内容: %s（不支持的文件格式或文件损坏）", path),
		}, nil
	}

	// 截断到 20000 字符
	origLen := len(textContent)
	truncated := origLen > 20000
	if truncated {
		textContent = textContent[:20000]
	}

	result := textContent
	if truncated {
		result += fmt.Sprintf("\n\n...（内容过长，仅显示前 20000 字符，共 %d 字符）", origLen)
	}
	return map[string]interface{}{
		"content": result,
		"file":    path,
		"truncated": truncated,
	}, nil
}

func (p *Plugin) readWithPandoc(path string) string {
	var out bytes.Buffer
	cmd := exec.Command("pandoc", path, "-t", "plain", "--wrap=none")
	cmd.Stdout = &out
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return ""
	}
	return strings.TrimSpace(out.String())
}

func (p *Plugin) readWithLibreoffice(path string) string {
	tmpDir, err := os.MkdirTemp("", "lo-doc-*")
	if err != nil {
		return ""
	}
	defer os.RemoveAll(tmpDir)

	cmd := exec.Command("libreoffice", "--headless", "--convert-to", "txt:Text", "--outdir", tmpDir, path)
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return ""
	}

	// 找生成的 txt 文件
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".txt") {
			data, err := os.ReadFile(filepath.Join(tmpDir, e.Name()))
			if err == nil {
				return strings.TrimSpace(string(data))
			}
		}
	}
	return ""
}

func (p *Plugin) handleVideoDownload(args map[string]interface{}) (interface{}, error) {
	url, _ := args["url"].(string)
	if url == "" {
		return nil, fmt.Errorf("url is required")
	}
	infoOnly, _ := args["info_only"].(bool)

	outputDir := filepath.Join(p.agentfsDir, "videos")
	os.MkdirAll(outputDir, 0755)

	if infoOnly {
		var out bytes.Buffer
		cmd := exec.Command("yt-dlp", "--dump-json", url)
		cmd.Stdout = &out
		cmd.Stderr = nil
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("yt-dlp info: %w", err)
		}
		var info struct {
			Title    string `json:"title"`
			Duration int    `json:"duration"`
			Webpage  string `json:"webpage_url"`
			Formats  []struct {
				FormatID string `json:"format_id"`
				Ext      string `json:"ext"`
				Width    int    `json:"width"`
				Height   int    `json:"height"`
				Filesize int64  `json:"filesize"`
				Format   string `json:"format"`
			} `json:"formats"`
		}
		if err := json.Unmarshal(out.Bytes(), &info); err != nil {
			return string(out.String()), nil
		}
		dur := ""
		if info.Duration > 0 {
			dur = fmt.Sprintf("%d分%d秒", info.Duration/60, info.Duration%60)
		}
		lines := []string{fmt.Sprintf("🎬 %s", info.Title)}
		if dur != "" {
			lines = append(lines, fmt.Sprintf("   时长: %s", dur))
		}
		lines = append(lines, fmt.Sprintf("   链接: %s", info.Webpage))
		lines = append(lines, "")
		for _, f := range info.Formats {
			fs := ""
			if f.Filesize > 0 {
				fs = fmt.Sprintf(" (%.1f MB)", float64(f.Filesize)/1048576)
			}
			res := ""
			if f.Width > 0 && f.Height > 0 {
				res = fmt.Sprintf(" %dx%d", f.Width, f.Height)
			}
			lines = append(lines, fmt.Sprintf("  [%s] %s%s%s", f.FormatID, f.Format, res, fs))
		}
		return map[string]interface{}{"content": strings.Join(lines, "\n")}, nil
	}

	// 下载
	outputTmpl := filepath.Join(outputDir, "%(title)s.%(ext)s")
	var out bytes.Buffer
	cmd := exec.Command("yt-dlp", "-o", outputTmpl, "--no-playlist", "--print", "after_move:filepath", url)
	cmd.Stdout = &out
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("yt-dlp download: %w", err)
	}

	// 解析 yt-dlp 输出的文件路径
	dlPath := strings.TrimSpace(out.String())
	if dlPath == "" {
		return map[string]interface{}{
			"content": "下载完成，但无法获取文件路径",
		}, nil
	}
	dlFilename := filepath.Base(dlPath)
	var fileSize int64 = 0
	if fi, err := os.Stat(dlPath); err == nil {
		fileSize = fi.Size()
	}
	return map[string]interface{}{
		"content":  fmt.Sprintf("✅ 下载完成: %s\n  大小: %.1f MB\n  路径: %s", dlFilename, float64(fileSize)/1048576, dlPath),
		"file":     dlPath,
		"filename": dlFilename,
	}, nil
}

// ======== NapCat HTTP Client ========

func (p *Plugin) napcat(action string, params map[string]interface{}) (interface{}, error) {
	data, _ := json.Marshal(params)
	url := fmt.Sprintf("%s/%s", p.napcatURL, action)

	resp, err := p.httpClient.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("napcat %s: %w", action, err)
	}
	defer resp.Body.Close()

	var raw json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("napcat decode %s: %w", action, err)
	}
	return string(raw), nil
}

// ======== Helpers ========


var reAPIKey = regexp.MustCompile(`(?i)(api[_-]?key|token|secret|password)\s*[=:]\s*\S+`)
var reSKKey = regexp.MustCompile(`sk-[a-zA-Z0-9]{20,}`)
var reInternalIP = regexp.MustCompile(`\b(127\.\d{1,3}\.\d{1,3}\.\d{1,3}|10\.\d{1,3}\.\d{1,3}\.\d{1,3}|172\.(1[6-9]|2\d|3[01])\.\d{1,3}\.\d{1,3}|192\.168\.\d{1,3}\.\d{1,3})\b`)

func (p *Plugin) sensitiveFilter(text string) string {
	if p.remoteDir != "" {
		text = strings.ReplaceAll(text, p.remoteDir, "[remote]")
	}
	if p.filesDir != "" {
		text = strings.ReplaceAll(text, p.filesDir, "[files]")
	}

	text = reAPIKey.ReplaceAllString(text, "$1=***")
	text = reSKKey.ReplaceAllString(text, "sk-***")
	text = reInternalIP.ReplaceAllString(text, "[IP]")
	return text
}

func convInt64(v interface{}) (int64, error) {
	switch n := v.(type) {
	case int64:
		return n, nil
	case float64:
		return int64(n), nil
	case int:
		return int64(n), nil
	case json.Number:
		return n.Int64()
	case string:
		return strconv.ParseInt(n, 10, 64)
	}
	return 0, fmt.Errorf("cannot convert %T to int64", v)
}

func NewPlugin(name string, config map[string]interface{}) (sdk.Plugin, error) {
	return &Plugin{
		name:           name,
		nextID:         1,
		messages:       make([]*SavedMessage, 0, maxMessages),
		groupNameCache: make(map[int64]string),
		allowFrom:      make(map[int64]struct{}),
		groupAllowFrom: make(map[int64]struct{}),
		dmPolicy:       "open",
		groupPolicy:    "open",
	}, nil
}






