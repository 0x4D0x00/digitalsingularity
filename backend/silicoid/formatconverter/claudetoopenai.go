package formatconverter

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ResponseClaudeToOpenAI 将Claude Messages API格式的响应转换为OpenAI格式
func (s *SilicoidFormatConverterService) ResponseClaudeToOpenAI(claudeResponse map[string]interface{}, openaiRequest map[string]interface{}) (map[string]interface{}, error) {
	logger.Println("开始转换Claude Messages API响应为OpenAI格式")

	// 获取Claude Messages API响应中的内容
	content := ""
	// 从content列表中获取文本内容
	if contentList, ok := claudeResponse["content"].([]interface{}); ok {
		for _, item := range contentList {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if itemMap["type"] == "text" {
				content, _ = itemMap["text"].(string)
				break
			}
		}
	}

	// 获取模型和stop_reason
	stopReason, _ := claudeResponse["stop_reason"].(string)
	model, _ := claudeResponse["model"].(string)

	// 构建OpenAI格式的响应
	currentTime := int(time.Now().Unix())
	responseID := fmt.Sprintf("chatcmpl-%s", strings.ReplaceAll(uuid.New().String(), "-", ""))

	// 检查工具调用
	choiceMessage := map[string]interface{}{
		"role":    "assistant",
		"content": content,
	}

	// 如果存在工具调用，添加到消息中
	if toolCalls, ok := claudeResponse["tool_calls"].([]interface{}); ok && len(toolCalls) > 0 {
		var openaiToolCalls []map[string]interface{}

		for _, toolCall := range toolCalls {
			toolCallMap, ok := toolCall.(map[string]interface{})
			if !ok {
				continue
			}

			id, _ := toolCallMap["id"].(string)
			if id == "" {
				id = fmt.Sprintf("call_%s", uuid.New().String()[:8])
			}

			name, _ := toolCallMap["name"].(string)
			args, _ := toolCallMap["args"].(map[string]interface{})

			argsJSON, err := json.Marshal(args)
			if err != nil {
				logger.Printf("序列化工具参数失败: %v", err)
				continue
			}

			openaiToolCall := map[string]interface{}{
				"id":   id,
				"type": "function",
				"function": map[string]interface{}{
					"name":      name,
					"arguments": string(argsJSON),
				},
			}

			openaiToolCalls = append(openaiToolCalls, openaiToolCall)
		}

		choiceMessage["tool_calls"] = openaiToolCalls
		logger.Printf("转换了 %d 个工具调用", len(openaiToolCalls))
	} else if stopReason == "tool_use" || claudeResponse["stop_reason"] == "tool_use" {
		// 如果 stop_reason 是 tool_use 但没有 tool_calls 字段，记录警告
		// 通常这种情况不应该发生，因为 tool_use 应该伴随 tool_calls
		logger.Printf("⚠️  检测到 tool_use stop_reason 但没有 tool_calls 字段，跳过工具调用处理")
	}

	// 构建完整的OpenAI响应
	finishReason := "stop"
	if _, ok := choiceMessage["tool_calls"]; ok {
		finishReason = "tool_calls"
	} else if stopReason == "stop_sequence" {
		finishReason = "stop"
	} else if stopReason != "" {
		finishReason = stopReason
	}

	// 处理使用量统计
	usage := map[string]interface{}{
		"prompt_tokens":     0,
		"completion_tokens": 0,
		"total_tokens":      0,
	}

	if usageData, ok := claudeResponse["usage"].(map[string]interface{}); ok {
		inputTokens, _ := usageData["input_tokens"].(float64)
		outputTokens, _ := usageData["output_tokens"].(float64)
		usage = map[string]interface{}{
			"prompt_tokens":     int(inputTokens),
			"completion_tokens": int(outputTokens),
			"total_tokens":      int(inputTokens + outputTokens),
		}
	}

	// 最终构建OpenAI响应
	openaiResponse := map[string]interface{}{
		"id":      responseID,
		"object":  "chat.completion",
		"created": currentTime,
		"model":   model,
		"choices": []map[string]interface{}{
			{
				"index":        0,
				"message":      choiceMessage,
				"finish_reason": finishReason,
			},
		},
		"usage": usage,
	}

	// 如果原始请求中有模型，优先使用原始请求的模型
	if openaiRequest != nil {
		if requestModel, ok := openaiRequest["model"].(string); ok && requestModel != "" {
			openaiResponse["model"] = requestModel
		}
	}

	logger.Println("转换完成: Claude Messages API -> OpenAI")
	return openaiResponse, nil
}

// HandleResponseClaudeStream 处理Claude Messages API流式响应并转换为OpenAI流式格式
func (s *SilicoidFormatConverterService) HandleResponseClaudeStream(claudeStream interface{}) (chan string, error) {
	responseID := fmt.Sprintf("chatcmpl-%s", strings.ReplaceAll(uuid.New().String(), "-", ""))
	currentTime := int(time.Now().Unix())
	stream := make(chan string)

	// Go协程处理流式数据
	go func() {
		defer close(stream)

		// 实际实现需要根据Claude的流式接口进行适配
		// 这里只是一个基本框架
		// 以下是将Claude的事件转换为OpenAI格式的处理逻辑

		// 示例：当收到Claude事件时
		/*
		   处理每个事件，将其转换为OpenAI格式
		   例如：
		   当接收到Claude的文本数据时:
		   openaiChunk := map[string]interface{}{
		       "id": responseID,
		       "object": "chat.completion.chunk",
		       "created": currentTime,
		       "model": "claude-3-7-sonnet-20250219",
		       "choices": []map[string]interface{}{
		           {
		               "index": 0,
		               "delta": map[string]interface{}{
		                   "content": deltaText,
		               },
		               "finish_reason": nil,
		           },
		       },
		   }
		   jsonBytes, _ := json.Marshal(openaiChunk)
		   stream <- fmt.Sprintf("data: %s\n\n", string(jsonBytes))

		   当流结束时:
		   finalChunk := map[string]interface{}{...}
		   jsonBytes, _ := json.Marshal(finalChunk)
		   stream <- fmt.Sprintf("data: %s\n\n", string(jsonBytes))
		   stream <- "data: [DONE]\n\n"
		*/

		// 发送一个示例事件（完整实现需要根据实际的Claude流事件进行处理）
		message := map[string]interface{}{
			"id":      responseID,
			"object":  "chat.completion.chunk",
			"created": currentTime,
			"model":   "claude-3-7-sonnet-20250219",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"delta": map[string]interface{}{
						"content": "这是一个示例响应",
					},
					"finish_reason": nil,
				},
			},
		}
		
		jsonBytes, _ := json.Marshal(message)
		stream <- fmt.Sprintf("data: %s\n\n", string(jsonBytes))

		// 发送结束标记
		stream <- "data: [DONE]\n\n"
	}()

	return stream, nil
}