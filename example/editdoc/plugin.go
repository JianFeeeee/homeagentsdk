package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/JianFeeeee/homeagentsdk/sdk"
)

type Plugin struct {
	name       string
	sdk        *sdk.PluginSDK
	scriptPath string
	venvPython string
}

func (p *Plugin) Name() string { return p.name }

func (p *Plugin) Start(s *sdk.PluginSDK) error {
	s.SetAutoRestart(true)
	p.sdk = s

	s.Settings().RegisterDef(sdk.ConfigDef{
		Key: "script_path", Default: "", Type: "string",
		DisplayName: "编辑脚本路径",
		Description: "edit_doc.py 的绝对路径；留空时使用插件可执行文件同目录下的 edit_doc.py",
		Category:    p.name,
	})
	s.Settings().RegisterDef(sdk.ConfigDef{
		Key: "venv_python", Default: "", Type: "string",
		DisplayName: "venv Python 解释器",
		Description: "执行 edit_doc.py 使用的 Python 解释器（建议用 venv 内的 python）；必须配置，留空将报错",
		Category:    p.name,
	})

	if v, err := s.Settings().Get("script_path"); err == nil {
		if str, ok := v.(string); ok {
			p.scriptPath = str
		}
	}
	if v, err := s.Settings().Get("venv_python"); err == nil {
		if str, ok := v.(string); ok {
			p.venvPython = str
		}
	}
	s.RegisterTool("edit_document", sdk.ToolDef{
		Name:        "edit_document",
		Description: "编辑 Office 文档内容。支持替换文本、修改单元格等操作。编辑后原文件被覆盖。操作前建议先用 read_document 查看内容。支持 .docx / .xlsx / .pptx。",
		NoMemory:    true,
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"file":        map[string]interface{}{"type": "string", "description": "文档文件路径"},
				"operation":   map[string]interface{}{"type": "string", "description": "操作: replace_text（查找替换）, set_cell（设置单元格）, insert_row（插入行）"},
				"target":      map[string]interface{}{"type": "string", "description": "要查找的文本（replace_text）"},
				"replacement": map[string]interface{}{"type": "string", "description": "替换为的文本（replace_text）"},
				"sheet":       map[string]interface{}{"type": "string", "description": "工作表名称（xlsx，可选）"},
				"row":         map[string]interface{}{"type": "integer", "description": "行号（set_cell/insert_row）"},
				"col":         map[string]interface{}{"type": "integer", "description": "列号（set_cell）"},
				"value":       map[string]interface{}{"type": "string", "description": "单元格值（set_cell）"},
			},
			"required": []string{"file", "operation"},
		},
	}, p.handleEditDocument)
	return nil
}

func (p *Plugin) Stop() error { return nil }

func (p *Plugin) handleEditDocument(args map[string]interface{}) (interface{}, error) {
	file, _ := args["file"].(string)
	if file == "" {
		return nil, fmt.Errorf("file is required")
	}
	operation, _ := args["operation"].(string)
	if operation == "" {
		return nil, fmt.Errorf("operation is required")
	}
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return map[string]interface{}{
			"content": fmt.Sprintf("文件不存在: %s", file),
		}, nil
	}

	pyArgs := map[string]interface{}{}
	if v, ok := args["target"]; ok {
		pyArgs["target"] = v
	}
	if v, ok := args["replacement"]; ok {
		pyArgs["replacement"] = v
	}
	if v, ok := args["sheet"]; ok {
		pyArgs["sheet"] = v
	}
	if v, ok := args["row"]; ok {
		pyArgs["row"] = v
	}
	if v, ok := args["col"]; ok {
		pyArgs["col"] = v
	}
	if v, ok := args["value"]; ok {
		pyArgs["value"] = v
	}
	pyArgsJSON, _ := json.Marshal(pyArgs)

	scriptPath := p.scriptPath
	if scriptPath == "" {
		scriptPath = filepath.Join(filepath.Dir(os.Args[0]), "edit_doc.py")
		log.Printf("[%s] script_path 未配置，使用默认脚本路径: %s", p.name, scriptPath)
	}
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("edit_doc.py not found at %s（请在插件配置 script_path 中指定脚本路径）", scriptPath)
	}

	if p.venvPython == "" {
		return nil, fmt.Errorf("venv_python 未配置，无法执行脚本；请在插件配置中设置 venv_python（venv 内 python 的绝对路径）")
	}
	if _, err := os.Stat(p.venvPython); err != nil {
		return nil, fmt.Errorf("venv python 不存在: %s（请检查 venv_python 配置）", p.venvPython)
	}

	var out bytes.Buffer
	cmd := exec.Command(p.venvPython, scriptPath, file, operation, string(pyArgsJSON))
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("edit document: %w", err)
	}

	var result struct {
		Ok    bool   `json:"ok"`
		Error string `json:"error,omitempty"`
		Count int    `json:"count,omitempty"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		return map[string]interface{}{
			"content": fmt.Sprintf("编辑完成，输出: %s", out.String()),
			"file":    file,
		}, nil
	}
	if !result.Ok {
		return map[string]interface{}{
			"content": fmt.Sprintf("编辑失败: %s", result.Error),
		}, nil
	}
	msg := fmt.Sprintf("编辑完成，已保存到原文件: %s", file)
	if result.Count > 0 {
		msg += fmt.Sprintf("\n共处理 %d 处", result.Count)
	}
	return map[string]interface{}{
		"content": msg,
		"file":    file,
	}, nil
}


func NewPluginFactory(name string, config map[string]interface{}) (sdk.Plugin, error) {
	return &Plugin{name: name}, nil
}
