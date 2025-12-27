package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MCPServerConfig MCP服务器配置
type MCPServerConfig struct {
	Type               string `json:"type"`               // "url" 或 "sse"
	URL                string `json:"url"`                // 服务器URL
	Name               string `json:"name"`               // 服务器名称
	AuthorizationToken string `json:"authorization_token,omitempty"` // 认证令牌
	Description        string `json:"description,omitempty"`          // 描述
}

// MCPTool MCP工具定义
type MCPTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// MCPClient MCP客户端
type MCPClient struct {
	config     *MCPServerConfig
	httpClient *http.Client
	logger     *log.Logger
}

// NewMCPClient 创建MCP客户端
func NewMCPClient(config *MCPServerConfig) *MCPClient {
	return &MCPClient{
		config: config,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: log.New(log.Writer(), "[MCP-Client] ", log.LstdFlags),
	}
}

// MCPRequest MCP请求结构
type MCPRequest struct {
	ID      string      `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// MCPResponse MCP响应结构
type MCPResponse struct {
	ID     string      `json:"id"`
	Result interface{} `json:"result,omitempty"`
	Error  *MCPError   `json:"error,omitempty"`
}

// MCPError MCP错误结构
type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCPToolCall MCP工具调用
type MCPToolCall struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ConnectSSE 连接到SSE服务器并监听消息
func (c *MCPClient) ConnectSSE(ctx context.Context) (<-chan *MCPResponse, <-chan error, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.config.URL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("创建SSE请求失败: %v", err)
	}

	// 设置认证头
	if c.config.AuthorizationToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.AuthorizationToken)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("连接SSE失败: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, nil, fmt.Errorf("SSE连接失败, 状态码: %d", resp.StatusCode)
	}

	responseChan := make(chan *MCPResponse, 10)
	errorChan := make(chan error, 10)

	go func() {
		defer resp.Body.Close()
		defer close(responseChan)
		defer close(errorChan)

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
				line := strings.TrimSpace(scanner.Text())
				if strings.HasPrefix(line, "data: ") {
					data := strings.TrimPrefix(line, "data: ")
					if data == "" {
						continue
					}

					var response MCPResponse
					if err := json.Unmarshal([]byte(data), &response); err != nil {
						errorChan <- fmt.Errorf("解析SSE数据失败: %v, 数据: %s", err, data)
						continue
					}

					select {
					case responseChan <- &response:
					case <-ctx.Done():
						return
					}
				}
			}
		}

		if err := scanner.Err(); err != nil {
			errorChan <- fmt.Errorf("SSE扫描错误: %v", err)
		}
	}()

	return responseChan, errorChan, nil
}

// CallTool 调用MCP工具
func (c *MCPClient) CallTool(ctx context.Context, toolCall *MCPToolCall) (interface{}, error) {
	request := MCPRequest{
		ID:     uuid.New().String(),
		Method: "tools/call",
		Params: map[string]interface{}{
			"name":      toolCall.Name,
			"arguments": toolCall.Arguments,
		},
	}

	return c.sendRequest(ctx, &request)
}

// ListTools 获取可用工具列表
func (c *MCPClient) ListTools(ctx context.Context) ([]*MCPTool, error) {
	request := MCPRequest{
		ID:     uuid.New().String(),
		Method: "tools/list",
	}

	result, err := c.sendRequest(ctx, &request)
	if err != nil {
		return nil, err
	}

	// 解析工具列表
	toolsData, ok := result.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("无效的工具列表响应")
	}

	tools, ok := toolsData["tools"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("响应中没有tools字段")
	}

	var mcpTools []*MCPTool
	for _, tool := range tools {
		toolMap, ok := tool.(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := toolMap["name"].(string)
		description, _ := toolMap["description"].(string)

		mcpTool := &MCPTool{
			Name:        name,
			Description: description,
		}

		if inputSchema, ok := toolMap["inputSchema"].(map[string]interface{}); ok {
			mcpTool.Parameters = inputSchema
		}

		mcpTools = append(mcpTools, mcpTool)
	}

	return mcpTools, nil
}

// sendRequest 发送HTTP请求到MCP服务器
func (c *MCPClient) sendRequest(ctx context.Context, request *MCPRequest) (interface{}, error) {
	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.URL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("创建HTTP请求失败: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.config.AuthorizationToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.AuthorizationToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("请求失败, 状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	var response MCPResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v, 响应: %s", err, string(body))
	}

	if response.Error != nil {
		return nil, fmt.Errorf("MCP错误: %s (代码: %d)", response.Error.Message, response.Error.Code)
	}

	return response.Result, nil
}

// MCPClientManager MCP客户端管理器
type MCPClientManager struct {
	clients map[string]*MCPClient
	mutex   sync.RWMutex
	logger  *log.Logger
}

// NewMCPClientManager 创建MCP客户端管理器
func NewMCPClientManager() *MCPClientManager {
	return &MCPClientManager{
		clients: make(map[string]*MCPClient),
		logger:  log.New(log.Writer(), "[MCP-Manager] ", log.LstdFlags),
	}
}

// GetClient 获取或创建MCP客户端
func (m *MCPClientManager) GetClient(serverName string, config *MCPServerConfig) *MCPClient {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if client, exists := m.clients[serverName]; exists {
		return client
	}

	client := NewMCPClient(config)
	m.clients[serverName] = client
	m.logger.Printf("创建MCP客户端: %s -> %s", serverName, config.URL)

	return client
}

// RemoveClient 移除客户端
func (m *MCPClientManager) RemoveClient(serverName string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, exists := m.clients[serverName]; exists {
		delete(m.clients, serverName)
		m.logger.Printf("移除MCP客户端: %s", serverName)
	}
}

// ListClients 列出所有客户端
func (m *MCPClientManager) ListClients() []string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	names := make([]string, 0, len(m.clients))
	for name := range m.clients {
		names = append(names, name)
	}

	return names
}

// GlobalMCPClientManager 全局MCP客户端管理器实例
var GlobalMCPClientManager = NewMCPClientManager()
