package formatconverter

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// loadMCPServerConfigs 加载MCP服务器配置
func (s *SilicoidFormatConverterService) loadMCPServerConfigs() ([]interface{}, error) {
	// 读取MCP配置文件
	file, err := os.Open("backend/silicoid/mcp.json")
	if err != nil {
		return nil, fmt.Errorf("无法打开MCP配置文件: %v", err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("读取MCP配置文件失败: %v", err)
	}

	// 替换环境变量
	configStr := string(data)
	configStr = s.replaceEnvironmentVariables(configStr)

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(configStr), &config); err != nil {
		return nil, fmt.Errorf("解析MCP配置文件失败: %v", err)
	}

	// 提取mcpServers数组
	mcpServers, ok := config["mcpServers"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("MCP配置文件中没有mcpServers字段")
	}

	// 为每个服务器配置设置授权令牌（如果未设置）
	for _, server := range mcpServers {
		if serverMap, ok := server.(map[string]interface{}); ok {
			serverName, _ := serverMap["name"].(string)

			// 检查是否已有authorization_token
			if token, exists := serverMap["authorization_token"]; !exists || token == "" || strings.HasPrefix(token.(string), "${") {
				// 尝试从数据库或环境变量获取令牌
				token := s.getAuthorizationToken(serverName)
				if token != "" {
					serverMap["authorization_token"] = token
					logger.Printf("✅ 为MCP服务器 %s 设置了授权令牌", serverName)
				} else {
					logger.Printf("⚠️ MCP服务器 %s 没有设置授权令牌", serverName)
				}
			}
		}
	}

	logger.Printf("✅ 加载了 %d 个MCP服务器配置", len(mcpServers))
	return mcpServers, nil
}
// replaceEnvironmentVariables 替换字符串中的环境变量
func (s *SilicoidFormatConverterService) replaceEnvironmentVariables(input string) string {
	// 简单的环境变量替换逻辑
	// 查找 ${VAR_NAME} 格式的变量并替换为环境变量值
	result := input

	// 使用简单的字符串替换来处理环境变量
	// 这里可以扩展为更复杂的逻辑
	if strings.Contains(result, "${MCP_CURRENT_TIME_TOKEN}") {
		token := os.Getenv("MCP_CURRENT_TIME_TOKEN")
		if token == "" {
			token = "default_current_time_token" // 默认令牌
		}
		result = strings.ReplaceAll(result, "${MCP_CURRENT_TIME_TOKEN}", token)
	}

	if strings.Contains(result, "${MCP_CURRENT_WEATHER_TOKEN}") {
		token := os.Getenv("MCP_CURRENT_WEATHER_TOKEN")
		if token == "" {
			token = "default_weather_token" // 默认令牌
		}
		result = strings.ReplaceAll(result, "${MCP_CURRENT_WEATHER_TOKEN}", token)
	}

	if strings.Contains(result, "${MCP_STORAGEBOX_DATA_TOKEN}") {
		token := os.Getenv("MCP_STORAGEBOX_DATA_TOKEN")
		if token == "" {
			token = "default_storagebox_token" // 默认令牌
		}
		result = strings.ReplaceAll(result, "${MCP_STORAGEBOX_DATA_TOKEN}", token)
	}

	return result
}

// getAuthorizationToken 获取指定MCP服务器的授权令牌
func (s *SilicoidFormatConverterService) getAuthorizationToken(serverName string) string {
	// 首先尝试从环境变量获取
	envVar := "MCP_" + strings.ToUpper(strings.ReplaceAll(serverName, "-", "_")) + "_TOKEN"
	if token := os.Getenv(envVar); token != "" {
		return token
	}

	// 如果环境变量不存在，可以从数据库或其他配置源获取
	// 这里提供一个默认令牌生成机制
	switch serverName {
	case "current-time":
		return "time_service_token_2024"
	case "current-weather":
		return "weather_service_token_2024"
	case "storagebox-data":
		return "storagebox_service_token_2024"
	default:
		return "default_mcp_token_" + serverName
	}
} 
// AddExecutorTools 为支持的角色添加客户端执行器工具和MCP工具集
func (s *SilicoidFormatConverterService) AddExecutorTools(requestData map[string]interface{}) error {
	// 检查消息链中是否已经包含了工具调用上下文（assistant + tool消息对）
	messages, _ := requestData["messages"].([]interface{})
	hasToolCallContext := false

	// 检查是否有assistant消息包含tool_calls，并且有对应的tool消息
	hasAssistantWithToolCalls := false
	hasToolMessages := false

	for _, msg := range messages {
		if msgMap, ok := msg.(map[string]interface{}); ok {
			role, _ := msgMap["role"].(string)
			if role == "assistant" {
				if _, hasToolCalls := msgMap["tool_calls"]; hasToolCalls {
					hasAssistantWithToolCalls = true
				}
			} else if role == "tool" {
				hasToolMessages = true
			}
		}
	}

	// 如果既有assistant消息包含tool_calls，又有tool消息，说明工具已经执行过了
	if hasAssistantWithToolCalls && hasToolMessages {
		hasToolCallContext = true
	}

	if hasToolCallContext {
		logger.Printf("⏭️ 检测到消息链中已包含完整的工具调用上下文，跳过添加工具定义")
		return nil
	}

	// 检查是否已经有 tools 参数（避免覆盖）
	existingTools, _ := requestData["tools"].([]interface{})
	var mcpServers []interface{} // MCP服务器配置列表

	// 添加数据库中的工具（客户端执行器工具和MCP工具集）
	if s.dataService != nil {
		// 获取角色名称
		roleName, _ := requestData["role_name"].(string)
		if roleName != "" {
			// 获取该角色可用的所有工具（包括客户端执行器和MCP工具集）
			allTools, err := s.dataService.GetToolsForRole(roleName)
			if err != nil {
				logger.Printf("⚠️ 获取工具失败 (role: %s): %v", roleName, err)
			} else if len(allTools) > 0 {
				clientToolCount := 0
				mcpToolsetCount := 0

				logger.Printf("✅ 为角色 %s 加载了 %d 个工具", roleName, len(allTools))

				// 加载MCP服务器配置
				mcpServerConfigs, err := s.loadMCPServerConfigs()
				if err != nil {
					logger.Printf("⚠️ 加载MCP服务器配置失败: %v", err)
				} else {
					// 将所有MCP服务器配置添加到列表
					mcpServers = append(mcpServers, mcpServerConfigs...)
				}

				// 转换为 OpenAI 格式（通用格式）
				for _, tool := range allTools {
					if tool.ExecutionType == "client_executor" || tool.ExecutionType == "server_executor" {
						// 客户端执行器工具和服务器执行器工具（包括MCP工具）
						openaiTool := map[string]interface{}{
							"type": "function",
							"function": map[string]interface{}{
								"name":        tool.ToolName,
								"description": tool.ToolDescription,
								"parameters":  tool.InputSchema,
							},
						}
						existingTools = append(existingTools, openaiTool)

						if tool.ExecutionType == "client_executor" {
							clientToolCount++
						} else {
							mcpToolsetCount++
						}
					}
				}

				if clientToolCount > 0 {
					logger.Printf("✅ 客户端执行器工具: %d 个", clientToolCount)
				}
				if mcpToolsetCount > 0 {
					logger.Printf("✅ 服务器执行器工具: %d 个", mcpToolsetCount)
				}
			}
		}
	} else {
		logger.Printf("⚠️ DataService 未初始化，跳过工具添加")
	}

	// 更新tools参数
	if len(existingTools) > 0 {
		requestData["tools"] = existingTools
		logger.Printf("📋 总共添加了 %d 个工具到请求", len(existingTools))
		logger.Printf("✅ 工具已添加到tools参数，依赖模型原生工具调用支持")
	}

	// 添加MCP服务器配置
	if len(mcpServers) > 0 {
		requestData["mcp_servers"] = mcpServers
		logger.Printf("📡 添加了 %d 个MCP服务器配置", len(mcpServers))
	}

	return nil
}