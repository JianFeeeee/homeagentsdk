package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"gitcode.com/JianFeeeee/homeagent-sdk/sdk"
)

type Plugin struct {
	name string
	sdk  *sdk.PluginSDK
}

func (p *Plugin) Name() string { return p.name }

func (p *Plugin) Start(s *sdk.PluginSDK) error {
	s.SetAutoRestart(true)
	p.sdk = s
	s.RegisterTool("edit_document", sdk.ToolDef{
		Name:        "edit_document",
		Description: "编辑 Office 文档内容。支持替换文本、修改单元格等操作。编辑后原文件被覆盖。操作前建议先用 read_document 查看内容。支持 .docx / .xlsx / .pptx。",
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

	scriptPath := "/home/newqqagent/plugins/editdoc/edit_doc.py"
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("edit_doc.py not found at %s", scriptPath)
	}

	venvPython := "/home/program/qq-workspace/self-workplace/.venv/bin/python3"
	pythonBin := "python3"
	if _, err := os.Stat(venvPython); err == nil {
		pythonBin = venvPython
	}

	var out bytes.Buffer
	cmd := exec.Command(pythonBin, scriptPath, file, operation, string(pyArgsJSON))
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


func NewPlugin(name string, config map[string]interface{}) (sdk.Plugin, error) {
	return &Plugin{name: name}, nil
}
