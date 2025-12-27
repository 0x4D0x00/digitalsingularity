package database

import (
	"encoding/json"
	"fmt"
)

// Tool 客户端执行器工具
type Tool struct {
	ID                 int                    `json:"id"`
	ToolName           string                 `json:"tool_name"`
	ToolDescription    string                 `json:"tool_description"`
	InputSchema        map[string]interface{} `json:"input_schema"`
	Category           string                 `json:"category"`
	ExecutionType      string                 `json:"execution_type"`
	Enabled            bool                   `json:"enabled"`
	AvailableForRoles  []string               `json:"available_for_roles"`
	Priority           int                    `json:"priority"`
	CreatedAt          string                 `json:"created_at"`
	UpdatedAt          string                 `json:"updated_at"`
}

// GetToolsForRole 根据角色获取可用的客户端执行器工具
func (s *AIBasicPlatformDataService) GetToolsForRole(roleName string) ([]Tool, error) {
	// 查询启用的工具，按优先级排序
	query := fmt.Sprintf(`
	SELECT 
			id, tool_name, tool_description, input_schema, 
			category, execution_type, enabled, available_for_roles, priority,
			created_at, updated_at
		FROM %s.aibasicplatform_tools
		WHERE enabled = 1
		ORDER BY priority DESC, id ASC
	`, s.dbName)
	
	opResult := s.readWrite.QueryDb(query)
	if !opResult.IsSuccess() {
		return nil, fmt.Errorf("查询客户端执行器工具失败: %v", opResult.Error)
	}

	rows, ok := opResult.Data.([]map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("数据格式错误")
	}
	
	var tools []Tool
	for _, row := range rows {
		tool := Tool{
			ID:              getIntValue(row["id"]),
			ToolName:        row["tool_name"].(string),
			ToolDescription: row["tool_description"].(string),
			Category:        getStringValue(row, "category"),
			ExecutionType:   getStringValue(row, "execution_type"),
			Enabled:         getIntValue(row["enabled"]) == 1,
			Priority:        getIntValue(row["priority"]),
			CreatedAt:       getStringValue(row, "created_at"),
			UpdatedAt:       getStringValue(row, "updated_at"),
		}
		
		// 解析 input_schema JSON
		if schemaStr, ok := row["input_schema"].(string); ok {
			var schema map[string]interface{}
			if err := json.Unmarshal([]byte(schemaStr), &schema); err == nil {
				tool.InputSchema = schema
			}
		} else if schemaBytes, ok := row["input_schema"].([]byte); ok {
			var schema map[string]interface{}
			if err := json.Unmarshal(schemaBytes, &schema); err == nil {
				tool.InputSchema = schema
			}
		}
		
		// 解析 available_for_roles JSON
		if rolesStr, ok := row["available_for_roles"].(string); ok && rolesStr != "" {
			var roles []string
			if err := json.Unmarshal([]byte(rolesStr), &roles); err == nil {
				tool.AvailableForRoles = roles
			}
		} else if rolesBytes, ok := row["available_for_roles"].([]byte); ok && len(rolesBytes) > 0 {
			var roles []string
			if err := json.Unmarshal(rolesBytes, &roles); err == nil {
				tool.AvailableForRoles = roles
			}
		}
		
		// 检查工具是否对当前角色可用
		if len(tool.AvailableForRoles) == 0 {
			// 如果 available_for_roles 为空或null，表示对所有角色可用
			tools = append(tools, tool)
		} else {
			// 检查当前角色是否在可用列表中
			for _, role := range tool.AvailableForRoles {
				if role == roleName {
					tools = append(tools, tool)
					break
				}
			}
		}
	}
	
	return tools, nil
}

// GetAllTools 获取所有启用的客户端执行器工具（不过滤角色）
func (s *AIBasicPlatformDataService) GetAllTools() ([]Tool, error) {
	query := fmt.Sprintf(`
		SELECT
			id, tool_name, tool_description, input_schema,
			category, execution_type, enabled, available_for_roles, priority,
			created_at, updated_at
		FROM %s.aibasicplatform_tools
		WHERE enabled = 1
		ORDER BY priority DESC, id ASC
	`, s.dbName)
	
	opResult := s.readWrite.QueryDb(query)
	if !opResult.IsSuccess() {
		return nil, fmt.Errorf("查询客户端执行器工具失败: %v", opResult.Error)
	}

	rows, ok := opResult.Data.([]map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("数据格式错误")
	}
	
	var tools []Tool
	for _, row := range rows {
		tool := Tool{
			ID:              getIntValue(row["id"]),
			ToolName:        row["tool_name"].(string),
			ToolDescription: row["tool_description"].(string),
			Category:        getStringValue(row, "category"),
			ExecutionType:   getStringValue(row, "execution_type"),
			Enabled:         getIntValue(row["enabled"]) == 1,
			Priority:        getIntValue(row["priority"]),
			CreatedAt:       getStringValue(row, "created_at"),
			UpdatedAt:       getStringValue(row, "updated_at"),
		}
		
		// 解析 input_schema JSON
		if schemaStr, ok := row["input_schema"].(string); ok {
			var schema map[string]interface{}
			if err := json.Unmarshal([]byte(schemaStr), &schema); err == nil {
				tool.InputSchema = schema
			}
		} else if schemaBytes, ok := row["input_schema"].([]byte); ok {
			var schema map[string]interface{}
			if err := json.Unmarshal(schemaBytes, &schema); err == nil {
				tool.InputSchema = schema
			}
		}
		
		// 解析 available_for_roles JSON
		if rolesStr, ok := row["available_for_roles"].(string); ok && rolesStr != "" {
			var roles []string
			if err := json.Unmarshal([]byte(rolesStr), &roles); err == nil {
				tool.AvailableForRoles = roles
			}
		} else if rolesBytes, ok := row["available_for_roles"].([]byte); ok && len(rolesBytes) > 0 {
			var roles []string
			if err := json.Unmarshal(rolesBytes, &roles); err == nil {
				tool.AvailableForRoles = roles
			}
		}
		
		tools = append(tools, tool)
	}
	
	return tools, nil
}

// ConvertToolsToOpenAIFormat 将客户端执行器工具转换为OpenAI tools格式
func ConvertToolsToOpenAIFormat(tools []Tool) []map[string]interface{} {
	var openaiTools []map[string]interface{}
	
	for _, tool := range tools {
		openaiTool := map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        tool.ToolName,
				"description": tool.ToolDescription,
				"parameters":  tool.InputSchema,
			},
		}
		openaiTools = append(openaiTools, openaiTool)
	}
	
	return openaiTools
}

// ConvertToolsToClaudeFormat 将客户端执行器工具转换为Claude tools格式
func ConvertToolsToClaudeFormat(tools []Tool) []map[string]interface{} {
	var claudeTools []map[string]interface{}
	
	for _, tool := range tools {
		claudeTool := map[string]interface{}{
			"name":         tool.ToolName,
			"description":  tool.ToolDescription,
			"input_schema": tool.InputSchema,
		}
		claudeTools = append(claudeTools, claudeTool)
	}
	
	return claudeTools
}

// getStringValue 安全获取字符串值
func getStringValue(row map[string]interface{}, key string) string {
	if val, ok := row[key]; ok && val != nil {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// note: integer helpers moved to helpers.go

