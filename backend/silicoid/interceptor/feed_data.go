package interceptor

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ContentChunk 表示内容分块
type ContentChunk struct {
	Index    int    `json:"index"`
	Total    int    `json:"total"`
	Content  string `json:"content"`
	IsLast   bool   `json:"is_last"`
	ToolName string `json:"tool_name"`
}

// chunkContent 将大内容分块处理
func chunkContent(content string, toolName string, maxChunkSize int) []ContentChunk {
	if len(content) <= maxChunkSize {
		// 内容不大，直接返回
		return []ContentChunk{
			{
				Index:    1,
				Total:    1,
				Content:  content,
				IsLast:   true,
				ToolName: toolName,
			},
		}
	}
	
	// 计算分块数量
	totalChunks := (len(content) + maxChunkSize - 1) / maxChunkSize
	chunks := make([]ContentChunk, 0, totalChunks)
	
	for i := 0; i < totalChunks; i++ {
		start := i * maxChunkSize
		end := start + maxChunkSize
		if end > len(content) {
			end = len(content)
		}
		
		chunk := ContentChunk{
			Index:    i + 1,
			Total:    totalChunks,
			Content:  content[start:end],
			IsLast:   i == totalChunks-1,
			ToolName: toolName,
		}
		chunks = append(chunks, chunk)
	}
	
	return chunks
}

// isLargeContent 判断内容是否过大需要分批处理
func isLargeContent(content string, threshold int) bool {
	return len(content) > threshold
}

// BatchFeedResult 分批投喂结果
type BatchFeedResult struct {
	Success     bool
	ChunkIndex  int
	Error       error
	RetryCount  int
}

// feedBatchWithRetry 带重试机制的分批投喂
func (s *SilicoIDInterceptor) feedBatchWithRetry(ctx context.Context, chunk ContentChunk, requestID string, data map[string]interface{}, maxRetries int) *BatchFeedResult {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		logger.Printf("[%s] 📤 尝试投喂第 %d/%d 批次 (尝试 %d/%d)", requestID, chunk.Index, chunk.Total, attempt, maxRetries)
		
		err := s.feedSingleBatch(ctx, chunk, requestID, data)
		if err == nil {
			logger.Printf("[%s] ✅ 第 %d 批次投喂成功", requestID, chunk.Index)
			return &BatchFeedResult{
				Success:    true,
				ChunkIndex: chunk.Index,
				RetryCount: attempt - 1,
			}
		}
		
		logger.Printf("[%s] ❌ 第 %d 批次投喂失败 (尝试 %d/%d): %v", requestID, chunk.Index, attempt, maxRetries, err)
		
		if attempt < maxRetries {
			// 指数退避重试
			backoffTime := time.Duration(attempt) * time.Second
			logger.Printf("[%s] ⏳ 等待 %v 后重试", requestID, backoffTime)
			time.Sleep(backoffTime)
		}
	}
	
	return &BatchFeedResult{
		Success:    false,
		ChunkIndex: chunk.Index,
		Error:      fmt.Errorf("第 %d 批次投喂失败，已重试 %d 次", chunk.Index, maxRetries),
		RetryCount: maxRetries,
	}
}

// feedSingleBatch 投喂单个数据块
func (s *SilicoIDInterceptor) feedSingleBatch(ctx context.Context, chunk ContentChunk, requestID string, data map[string]interface{}) error {
	// 构造分批消息
	var batchMessage string
	if chunk.Total == 1 {
		// 只有一个批次，正常处理
		batchMessage = fmt.Sprintf("工具调用 '%s' 的执行结果：\n\n```json\n%s\n```\n\n请基于以上结果继续回答用户的问题。", 
			chunk.ToolName, chunk.Content)
	} else {
		// 多个批次，添加批次信息
		if chunk.IsLast {
			batchMessage = fmt.Sprintf("工具调用 '%s' 的执行结果 (第 %d/%d 批次，最后一批)：\n\n```json\n%s\n```\n\n所有数据已投喂完毕，请基于以上所有结果继续回答用户的问题。", 
				chunk.ToolName, chunk.Index, chunk.Total, chunk.Content)
		} else {
			batchMessage = fmt.Sprintf("工具调用 '%s' 的执行结果 (第 %d/%d 批次)：\n\n```json\n%s\n```\n\n这是第 %d 批数据，请等待所有数据投喂完毕后再回答。", 
				chunk.ToolName, chunk.Index, chunk.Total, chunk.Content, chunk.Index)
		}
	}
	
	// 获取当前messages
	messages, _ := data["messages"].([]interface{})
	
	// 添加工具结果
	messages = append(messages, map[string]interface{}{
		"id":      uuid.New().String(),
		"role":    "user",
		"content": batchMessage,
	})
	
	// 更新请求数据
	data["messages"] = messages
	
	// 如果不是最后一批，让AI确认接收
	if !chunk.IsLast {
		logger.Printf("[%s] ⏳ 第 %d 批次已投喂，等待AI确认接收", requestID, chunk.Index)
		
		// 调用AI确认接收
		confirmResponse := s.openaiService.CreateChatCompletionNonStream(ctx, data)
		
		// 检查确认响应是否有错误
		if errObj, exists := confirmResponse["error"]; exists {
			return fmt.Errorf("AI确认调用返回错误: %v", errObj)
		}
		
		// 提取确认响应
		if confirmChoices, ok := confirmResponse["choices"].([]interface{}); ok && len(confirmChoices) > 0 {
			if confirmChoice, ok := confirmChoices[0].(map[string]interface{}); ok {
				if confirmMessage, ok := confirmChoice["message"].(map[string]interface{}); ok {
					if confirmContent, ok := confirmMessage["content"].(string); ok {
						logger.Printf("[%s] ✅ 第 %d 批次确认响应: %s", requestID, chunk.Index, truncateString(confirmContent, 100))
						
						// 添加AI的确认响应
						messages = append(messages, map[string]interface{}{
							"id":      uuid.New().String(),
							"role":    "assistant",
							"content": confirmContent,
						})
						
						// 更新messages
						data["messages"] = messages
					} else {
						return fmt.Errorf("无法提取AI确认响应内容")
					}
				} else {
					return fmt.Errorf("无法提取AI确认响应消息")
				}
			} else {
				return fmt.Errorf("无法提取AI确认响应选择")
			}
		} else {
			return fmt.Errorf("AI确认响应格式无效")
		}
	}
	
	return nil
}

// validateAllBatchesFed 验证所有数据块是否都已投喂
func (s *SilicoIDInterceptor) validateAllBatchesFed(feedResults []*BatchFeedResult, requestID string) error {
	successCount := 0
	failedBatches := []int{}
	
	for _, result := range feedResults {
		if result.Success {
			successCount++
		} else {
			failedBatches = append(failedBatches, result.ChunkIndex)
		}
	}
	
	totalBatches := len(feedResults)
	
	if successCount != totalBatches {
		logger.Printf("[%s] ❌ 数据块投喂不完整：成功 %d/%d，失败批次: %v", requestID, successCount, totalBatches, failedBatches)
		return fmt.Errorf("数据块投喂不完整：成功 %d/%d，失败批次: %v", successCount, totalBatches, failedBatches)
	}
	
	logger.Printf("[%s] ✅ 所有 %d 个数据块已成功投喂", requestID, totalBatches)
	return nil
}

// feedBatchWithRetryClaude 带重试机制的分批投喂（Claude版本）
func (s *SilicoIDInterceptor) feedBatchWithRetryClaude(ctx context.Context, chunk ContentChunk, requestID string, data map[string]interface{}, maxRetries int) *BatchFeedResult {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		logger.Printf("[%s] 📤 尝试投喂第 %d/%d 批次 (尝试 %d/%d) [Claude]", requestID, chunk.Index, chunk.Total, attempt, maxRetries)
		
		err := s.feedSingleBatchClaude(ctx, chunk, requestID, data)
		if err == nil {
			logger.Printf("[%s] ✅ 第 %d 批次投喂成功 [Claude]", requestID, chunk.Index)
			return &BatchFeedResult{
				Success:    true,
				ChunkIndex: chunk.Index,
				RetryCount: attempt - 1,
			}
		}
		
		logger.Printf("[%s] ❌ 第 %d 批次投喂失败 (尝试 %d/%d): %v [Claude]", requestID, chunk.Index, attempt, maxRetries, err)
		
		if attempt < maxRetries {
			// 指数退避重试
			backoffTime := time.Duration(attempt) * time.Second
			logger.Printf("[%s] ⏳ 等待 %v 后重试 [Claude]", requestID, backoffTime)
			time.Sleep(backoffTime)
		}
	}
	
	return &BatchFeedResult{
		Success:    false,
		ChunkIndex: chunk.Index,
		Error:      fmt.Errorf("第 %d 批次投喂失败，已重试 %d 次 [Claude]", chunk.Index, maxRetries),
		RetryCount: maxRetries,
	}
}

// feedSingleBatchClaude 投喂单个数据块（Claude版本）
func (s *SilicoIDInterceptor) feedSingleBatchClaude(ctx context.Context, chunk ContentChunk, requestID string, data map[string]interface{}) error {
	// 构造分批消息
	var batchMessage string
	if chunk.Total == 1 {
		// 只有一个批次，正常处理
		batchMessage = fmt.Sprintf("工具调用 '%s' 的执行结果：\n\n```json\n%s\n```\n\n请基于以上结果继续回答用户的问题。", 
			chunk.ToolName, chunk.Content)
	} else {
		// 多个批次，添加批次信息
		if chunk.IsLast {
			batchMessage = fmt.Sprintf("工具调用 '%s' 的执行结果 (第 %d/%d 批次，最后一批)：\n\n```json\n%s\n```\n\n所有数据已投喂完毕，请基于以上所有结果继续回答用户的问题。", 
				chunk.ToolName, chunk.Index, chunk.Total, chunk.Content)
		} else {
			batchMessage = fmt.Sprintf("工具调用 '%s' 的执行结果 (第 %d/%d 批次)：\n\n```json\n%s\n```\n\n这是第 %d 批数据，请等待所有数据投喂完毕后再回答。", 
				chunk.ToolName, chunk.Index, chunk.Total, chunk.Content, chunk.Index)
		}
	}
	
	// 获取当前messages
	messages, _ := data["messages"].([]interface{})
	
	// 添加工具结果
	messages = append(messages, map[string]interface{}{
		"id":      uuid.New().String(),
		"role":    "user",
		"content": batchMessage,
	})
	
	// 更新请求数据
	data["messages"] = messages
	
	// 如果不是最后一批，让AI确认接收
	if !chunk.IsLast {
		logger.Printf("[%s] ⏳ 第 %d 批次已投喂，等待AI确认接收 [Claude]", requestID, chunk.Index)
		
		// 转换为Claude格式
		claudeData, err := s.formatConverter.RequestOpenAIToClaude(data)
		if err != nil {
			return fmt.Errorf("批次确认格式转换失败: %v", err)
		}
		
		// 调用Claude API确认接收
		confirmClaudeResponse := s.claudeService.CreateChatCompletionNonStream(ctx, claudeData)
		
		// 将Claude响应转换为OpenAI格式
		confirmResponse, err := s.formatConverter.ResponseClaudeToOpenAI(confirmClaudeResponse, data)
		if err != nil {
			return fmt.Errorf("批次确认响应转换失败: %v", err)
		}
		
		// 检查确认响应是否有错误
		if errObj, exists := confirmResponse["error"]; exists {
			return fmt.Errorf("AI确认调用返回错误: %v", errObj)
		}
		
		// 提取确认响应
		if confirmChoices, ok := confirmResponse["choices"].([]interface{}); ok && len(confirmChoices) > 0 {
			if confirmChoice, ok := confirmChoices[0].(map[string]interface{}); ok {
				if confirmMessage, ok := confirmChoice["message"].(map[string]interface{}); ok {
					if confirmContent, ok := confirmMessage["content"].(string); ok {
						logger.Printf("[%s] ✅ 第 %d 批次确认响应: %s [Claude]", requestID, chunk.Index, truncateString(confirmContent, 100))
						
						// 添加AI的确认响应
						messages = append(messages, map[string]interface{}{
							"id":      uuid.New().String(),
							"role":    "assistant",
							"content": confirmContent,
						})
						
						// 更新messages
						data["messages"] = messages
					} else {
						return fmt.Errorf("无法提取AI确认响应内容")
					}
				} else {
					return fmt.Errorf("无法提取AI确认响应消息")
				}
			} else {
				return fmt.Errorf("无法提取AI确认响应选择")
			}
		} else {
			return fmt.Errorf("AI确认响应格式无效")
		}
	}
	
	return nil
}

// FeedLargeFileChunks 处理大文件分块的分批投喂
// 检测请求数据中的 _large_file_chunks，如果有则分批投喂给AI
func (s *SilicoIDInterceptor) FeedLargeFileChunks(ctx context.Context, requestID string, data map[string]interface{}) error {
	// 检查是否有大文件分块
	largeFileChunks, ok := data["_large_file_chunks"].([]interface{})
	if !ok || len(largeFileChunks) == 0 {
		return nil // 没有大文件分块，直接返回
	}
	
	logger.Printf("[%s] 📦 检测到大文件分块，共 %d 块，开始分批投喂", requestID, len(largeFileChunks))
	
	// 判断是否是 Claude 模型
	modelName, _ := data["model"].(string)
	isClaudeModel := s.isClaudeModel(modelName)
	
	// 将分块转换为 ContentChunk 格式
	chunks := make([]ContentChunk, 0, len(largeFileChunks))
	for _, chunkInterface := range largeFileChunks {
		chunkMap, ok := chunkInterface.(map[string]interface{})
		if !ok {
			continue
		}
		
		fileId, _ := chunkMap["file_id"].(string)
		index, _ := chunkMap["index"].(float64)
		content, _ := chunkMap["content"].(string)
		isLast, _ := chunkMap["is_last"].(bool)
		
		// 计算总块数
		total := len(largeFileChunks)
		
		chunks = append(chunks, ContentChunk{
			Index:    int(index),
			Total:    total,
			Content:  content,
			IsLast:   isLast,
			ToolName: fmt.Sprintf("文件内容_%s", fileId),
		})
	}
	
	// 按索引排序
	for i := 0; i < len(chunks)-1; i++ {
		for j := i + 1; j < len(chunks); j++ {
			if chunks[i].Index > chunks[j].Index {
				chunks[i], chunks[j] = chunks[j], chunks[i]
			}
		}
	}
	
	// 分批投喂
	const maxRetries = 3
	feedResults := make([]*BatchFeedResult, 0, len(chunks))
	
	for _, chunk := range chunks {
		var result *BatchFeedResult
		if isClaudeModel {
			result = s.feedBatchWithRetryClaude(ctx, chunk, requestID, data, maxRetries)
		} else {
			result = s.feedBatchWithRetry(ctx, chunk, requestID, data, maxRetries)
		}
		feedResults = append(feedResults, result)
	}
	
	// 验证所有批次是否都成功
	if err := s.validateAllBatchesFed(feedResults, requestID); err != nil {
		return fmt.Errorf("大文件分批投喂失败: %v", err)
	}
	
	logger.Printf("[%s] ✅ 大文件分批投喂完成，共 %d 块", requestID, len(chunks))
	
	// 清理标记
	delete(data, "_large_file_chunks")
	
	return nil
}
