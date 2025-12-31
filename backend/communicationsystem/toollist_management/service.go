package toollist_management

import (
	"fmt"

	"digitalsingularity/backend/communicationsystem/database"
)

// GetToolList 获取工具列表
func GetToolList() ([]database.Tool, error) {
	tools, err := database.GetToolList()
	return tools, err
}

// mapToTool 将map转换为Tool结构体
func mapToTool(toolData map[string]interface{}) database.Tool {
	var tool database.Tool

	if val, ok := toolData["id"].(string); ok {
		tool.ID = val
	}
	if val, ok := toolData["name_zh"].(string); ok {
		tool.NameZh = val
	}
	if val, ok := toolData["name_en"].(string); ok {
		tool.NameEn = val
	}
	if val, ok := toolData["description_zh"].(string); ok {
		tool.DescriptionZh = val
	}
	if val, ok := toolData["description_en"].(string); ok {
		tool.DescriptionEn = val
	}
	if val, ok := toolData["details_zh"].(string); ok {
		tool.DetailsZh = val
	}
	if val, ok := toolData["details_en"].(string); ok {
		tool.DetailsEn = val
	}
	if val, ok := toolData["icon"].(string); ok {
		tool.Icon = val
	}
	if val, ok := toolData["icon_path_light"].(string); ok {
		tool.IconPathLight = val
	}
	if val, ok := toolData["icon_path_dark"].(string); ok {
		tool.IconPathDark = val
	}
	if val, ok := toolData["category_zh"].(string); ok {
		tool.CategoryZh = val
	}
	if val, ok := toolData["category_en"].(string); ok {
		tool.CategoryEn = val
	}
	if val, ok := toolData["version"].(string); ok {
		tool.Version = val
	}
	if val, ok := toolData["author"].(string); ok {
		tool.Author = val
	}

	// 处理features数组
	if featuresZh, ok := toolData["features_zh"].([]interface{}); ok {
		tool.FeaturesZh = make([]string, len(featuresZh))
		for i, f := range featuresZh {
			if str, ok := f.(string); ok {
				tool.FeaturesZh[i] = str
			}
		}
	}
	if featuresEn, ok := toolData["features_en"].([]interface{}); ok {
		tool.FeaturesEn = make([]string, len(featuresEn))
		for i, f := range featuresEn {
			if str, ok := f.(string); ok {
				tool.FeaturesEn[i] = str
			}
		}
	}

	if val, ok := toolData["usage_zh"].(string); ok {
		tool.UsageZh = val
	}
	if val, ok := toolData["usage_en"].(string); ok {
		tool.UsageEn = val
	}
	if val, ok := toolData["enabled"].(bool); ok {
		tool.Enabled = val
	} else {
		tool.Enabled = true // 默认启用
	}
	if val, ok := toolData["sort_order"].(float64); ok {
		tool.SortOrder = int(val)
	}

	return tool
}

// GetTool 获取工具（支持单个、批量或全部）
func GetTool(toolIDs interface{}) (interface{}, error) {
	switch ids := toolIDs.(type) {
	case string:
		// 单个工具
		return database.GetToolByID(ids)
	case []string:
		// 批量工具
		var tools []database.Tool
		for _, id := range ids {
			if tool, err := database.GetToolByID(id); err == nil && tool != nil {
				tools = append(tools, *tool)
			}
		}
		return tools, nil
	case nil:
		// 获取所有工具
		return database.GetToolList()
	default:
		return nil, fmt.Errorf("无效的工具ID类型")
	}
}

// CreateTool 创建工具（支持单个或批量）
func CreateTool(toolData interface{}) error {
	switch data := toolData.(type) {
	case map[string]interface{}:
		// 单个工具
		tool := mapToTool(data)
		return database.CreateTool(tool)
	case []map[string]interface{}:
		// 批量工具
		tools := make([]database.Tool, len(data))
		for i, item := range data {
			tools[i] = mapToTool(item)
		}
		return database.CreateTools(tools)
	default:
		return fmt.Errorf("无效的工具数据类型")
	}
}

// UpdateTool 更新工具（支持单个或批量）
func UpdateTool(toolData interface{}) error {
	switch data := toolData.(type) {
	case map[string]interface{}:
		// 单个工具
		tool := mapToTool(data)
		return database.UpdateTool(tool)
	case []map[string]interface{}:
		// 批量工具
		tools := make([]database.Tool, len(data))
		for i, item := range data {
			tools[i] = mapToTool(item)
		}
		return database.UpdateTools(tools)
	default:
		return fmt.Errorf("无效的工具数据类型")
	}
}

// DeleteTool 删除工具（支持单个或批量）
func DeleteTool(toolIDs interface{}) error {
	switch ids := toolIDs.(type) {
	case string:
		// 单个工具
		return database.DeleteTool(ids)
	case []string:
		// 批量工具
		return database.DeleteTools(ids)
	default:
		return fmt.Errorf("无效的工具ID类型")
	}
}

// GetToolsByCategory 根据分类获取工具
func GetToolsByCategory(categoryZh string) ([]database.Tool, error) {
	return database.GetToolsByCategory(categoryZh)
}