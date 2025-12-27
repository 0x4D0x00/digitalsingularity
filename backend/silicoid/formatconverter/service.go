// 格式转换器包 (formatconverter)
// 提供OpenAI与Claude格式之间的双向转换服务
//
// 文件结构:
// - openaitoclaude.go:    OpenAI请求 → Claude请求 (RequestOpenAIToClaude)
// - claudetoopenai.go:    Claude响应 → OpenAI响应 (ResponseClaudeToOpenAI)
// - servercall.go:        MCP服务器调用相关功能
// - systempormpt.go:      系统提示词处理
// - filetodata.go:        文件转文本工具
// - service.go:           核心服务和通用工具函数
//
// 使用方式:
//   converter := NewSilicoidFormatConverterService()
//   claudeRequest, err := converter.RequestOpenAIToClaude(openaiRequest)
//   openaiResponse, err := converter.ResponseClaudeToOpenAI(claudeResponse, openaiRequest)

package formatconverter

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
	"digitalsingularity/backend/common/utils/datahandle"
	"digitalsingularity/backend/aibasicplatform/database"
	"digitalsingularity/backend/common/userfiles"
)

// 全局日志器
var logger *log.Logger

// ClaudeFileUploader 定义文件上传接口，用于上传文件到 Claude Files API
type ClaudeFileUploader interface {
	UploadFile(ctx context.Context, fileBytes []byte, filename string, mimeType string, apiKey string) (string, error)
}

// 大文件处理相关常量
const (
	// maxContentSize 单个消息内容的最大长度（字符数）
	// 设置为150000字符，约等于50000 tokens，留出安全余量
	maxContentSize = 150000
	// largeFileChunkSize 大文件分块大小（字符数）
	largeFileChunkSize = 100000
)

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func init() {
	// 初始化日志器
	logger = log.New(log.Writer(), "[FormatConverter] ", log.LstdFlags)
	// 将标准库默认日志的输出重定向到同一 Writer，便于看到其他包（如 userfiles）的日志
	log.SetOutput(logger.Writer())
}

// SilicoidFormatConverterService 提供格式转换功能
type SilicoidFormatConverterService struct{
	dataService *database.AIBasicPlatformDataService
	fileService *userfiles.FileService
	claudeFileUploader ClaudeFileUploader // 用于上传文件到 Claude Files API（可选）
}

// NewSilicoidFormatConverterService 创建一个新的格式转换服务实例
func NewSilicoidFormatConverterService() *SilicoidFormatConverterService {
	// 初始化数据服务
	readWrite, err := datahandle.NewCommonReadWriteService("database")
	if err != nil {
		logger.Printf("警告: 初始化数据服务失败: %v", err)
		// 继续运行，但 dataService 为 nil
		return &SilicoidFormatConverterService{
			dataService: nil,
			fileService: userfiles.NewFileService(),
		}
	}

	// 创建 AIBasicPlatformDataService 实例
	dataService := database.NewAIBasicPlatformDataService(readWrite)

	// 创建文件服务实例
	fileService := userfiles.NewFileService()

	return &SilicoidFormatConverterService{
		dataService: dataService,
		fileService: fileService,
	}
}


// truncateString 截断字符串到指定长度
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// getFileFromUserfiles 向 userfiles 服务请求文件并读取内容
// expectedHash: 可选的 MD5 哈希值，用于验证文件是否正确（如果提供则进行验证）
func (s *SilicoidFormatConverterService) getFileFromUserfiles(fileId string, userId string, expectedHash string) ([]byte, string, error) {
	if s.fileService == nil {
		return nil, "", fmt.Errorf("文件服务未初始化")
	}

	if expectedHash != "" {
		logger.Printf("准备读取并验证文件 (file_id: %s, user_id: %s, md5: %s)", fileId, userId, expectedHash)
	} else {
		logger.Printf("准备读取文件 (file_id: %s, user_id: %s)", fileId, userId)
	}

	// 请求文件元信息与本地路径
	req := &userfiles.DownloadFileRequest{
		FileId: fileId,
		AppId:  "", // 文件所有者访问，不需要 appId
	}

	result, err := s.fileService.DownloadFile(userId, req)
	if err != nil {
		logger.Printf("读取文件失败 (file_id: %s, user_id: %s): %v", fileId, userId, err)
		return nil, "", fmt.Errorf("读取文件失败: %v", err)
	}

	if !result.Success {
		logger.Printf("读取文件未成功 (file_id: %s, user_id: %s): %s", fileId, userId, result.Message)
		return nil, "", fmt.Errorf("读取文件失败: %s", result.Message)
	}

	// 读取文件内容
	fileBytes, err := os.ReadFile(result.FilePath)
	if err != nil {
		logger.Printf("读取文件失败 (file_id: %s, path: %s): %v", fileId, result.FilePath, err)
		return nil, "", fmt.Errorf("读取文件失败: %v", err)
	}
	logger.Printf("文件读取成功 (file_id: %s, size: %d bytes, mime: %s, path: %s)", fileId, len(fileBytes), result.MimeType, result.FilePath)

	// 如果提供了 MD5，验证文件哈希值
	if expectedHash != "" {
		hash := md5.Sum(fileBytes)
		actualHash := fmt.Sprintf("%x", hash)
		// 转换为小写进行比较（MD5 通常不区分大小写）
		if strings.ToLower(actualHash) != strings.ToLower(expectedHash) {
			return nil, "", fmt.Errorf("文件 MD5 验证失败: 期望 %s，实际 %s", expectedHash, actualHash)
		}
		logger.Printf("文件 MD5 验证成功 (file_id: %s, md5: %s)", fileId, actualHash)
	}

	return fileBytes, result.MimeType, nil
}

// SetClaudeFileUploader 设置 Claude 文件上传器（用于支持 Claude Files API）
func (s *SilicoidFormatConverterService) SetClaudeFileUploader(uploader ClaudeFileUploader) {
	s.claudeFileUploader = uploader
	logger.Printf("已设置 Claude 文件上传器")
}

// processContentArray 处理数组格式的 content，转换为 Claude 需要的格式
// 如果启用了 Claude Files API，会先上传文件获取 file_id，然后使用 file_id 引用
func (s *SilicoidFormatConverterService) processContentArray(contentArray []interface{}, userId string) ([]interface{}, error) {
	var claudeContent []interface{}

	for _, part := range contentArray {
		partMap, ok := part.(map[string]interface{})
		if !ok {
			continue
		}

		partType, _ := partMap["type"].(string)

		switch partType {
		case "text":
			// 文本类型，直接添加
			if text, ok := partMap["text"].(string); ok {
				claudeContent = append(claudeContent, map[string]interface{}{
					"type": "text",
					"text": text,
				})
			}
		case "file_read":
			// 文件读取类型，从 userfiles 读取文件并转换为 Claude 格式
			if fileId, ok := partMap["file_id"].(string); ok && userId != "" {
				// 提取 MD5 哈希值（支持 file_hash 或 md5 字段名）
				var fileHash string
				if hash, ok := partMap["file_hash"].(string); ok && hash != "" {
					fileHash = hash
				} else if hash, ok := partMap["md5"].(string); ok && hash != "" {
					fileHash = hash
				}
				
				fileBytes, mimeType, err := s.getFileFromUserfiles(fileId, userId, fileHash)
				if err != nil {
					logger.Printf("读取文件失败 (file_id: %s): %v", fileId, err)
					// 降级为文本提示
					claudeContent = append(claudeContent, map[string]interface{}{
						"type": "text",
						"text": fmt.Sprintf("[文件读取失败: %s]", fileId),
					})
					continue
				}

				// 检查是否支持 Claude Files API 且文件类型符合要求
				useFilesAPI := s.claudeFileUploader != nil && 
					(strings.HasPrefix(mimeType, "image/") || mimeType == "application/pdf" || mimeType == "text/plain")
				
				if useFilesAPI {
					// 使用 Claude Files API：上传文件并获取 file_id
					// 从 fileId 中提取文件名（如果没有，使用默认名称）
					filename := fileId
					if ext := getFileExtension(mimeType); ext != "" {
						filename = fileId + ext
					}
					
					// 获取 API Key（从请求参数中，这里暂时使用空字符串，由 UploadFile 内部处理）
					claudeFileID, err := s.claudeFileUploader.UploadFile(context.Background(), fileBytes, filename, mimeType, "")
					if err != nil {
						logger.Printf("上传文件到 Claude Files API 失败 (file_id: %s): %v，降级为 base64", fileId, err)
						// 降级为 base64 方式
						useFilesAPI = false
					} else {
						logger.Printf("✅ 文件已上传到 Claude Files API (file_id: %s -> claude_file_id: %s)", fileId, claudeFileID)
						
						// 根据文件类型使用 file_id 引用
						if strings.HasPrefix(mimeType, "image/") {
							// 图片类型：使用 image block with file_id
							claudeContent = append(claudeContent, map[string]interface{}{
								"type": "image",
								"source": map[string]interface{}{
									"type":    "file",
									"file_id": claudeFileID,
								},
							})
						} else if mimeType == "application/pdf" || mimeType == "text/plain" {
							// PDF 或文本类型：使用 document block with file_id
							claudeContent = append(claudeContent, map[string]interface{}{
								"type": "document",
								"source": map[string]interface{}{
									"type":    "file",
									"file_id": claudeFileID,
								},
							})
						}
						continue
					}
				}
				
				// 降级方案：使用 base64 嵌入（兼容旧方式）
				if !useFilesAPI {
					// 转换为 base64
					base64Data := base64.StdEncoding.EncodeToString(fileBytes)

					// 根据文件类型转换为 Claude 格式
					if strings.HasPrefix(mimeType, "image/") {
						// 图片类型
						claudeContent = append(claudeContent, map[string]interface{}{
							"type": "image",
							"source": map[string]interface{}{
								"type":      "base64",
								"media_type": mimeType,
								"data":       base64Data,
							},
						})
					} else if mimeType == "application/pdf" {
						// PDF 文档类型
						claudeContent = append(claudeContent, map[string]interface{}{
							"type": "document",
							"source": map[string]interface{}{
								"type":      "base64",
								"media_type": mimeType,
								"data":       base64Data,
							},
						})
					} else {
						// 其他类型，降级为文本提示
						claudeContent = append(claudeContent, map[string]interface{}{
							"type": "text",
							"text": fmt.Sprintf("[文件: %s, 类型: %s]", fileId, mimeType),
						})
					}
				}
			} else {
				logger.Printf("警告: file_read 类型缺少 file_id 或 userId，已忽略")
			}
		case "image", "document":
			// 图片和文档类型，直接使用接口提供的数据（不需要 file_id）
			// 支持两种格式：
			// 1. 直接包含 source 字段（Claude 格式）
			// 2. 包含 image_url 字段（OpenAI 格式）
			if source, ok := partMap["source"].(map[string]interface{}); ok {
				// Claude 格式：直接使用 source 字段
				claudeContent = append(claudeContent, map[string]interface{}{
					"type":   partType,
					"source": source,
				})
			} else if imageUrl, ok := partMap["image_url"].(map[string]interface{}); ok {
				// OpenAI 格式：支持 image_url（直接 base64 数据）
				if url, ok := imageUrl["url"].(string); ok {
					// 检查是否是 base64 格式
					if strings.HasPrefix(url, "data:") {
						// 提取 base64 数据
						parts := strings.SplitN(url, ",", 2)
						if len(parts) == 2 {
							mediaType := "image/png"
							if strings.HasPrefix(parts[0], "data:image/") {
								mediaTypeParts := strings.Split(parts[0], ";")
								if len(mediaTypeParts) > 0 {
									mediaType = strings.TrimPrefix(mediaTypeParts[0], "data:")
								}
							}
							claudeContent = append(claudeContent, map[string]interface{}{
								"type": "image",
								"source": map[string]interface{}{
									"type":       "base64",
									"media_type": mediaType,
									"data":       parts[1],
								},
							})
						}
					} else {
						logger.Printf("警告: image 类型包含非 base64 的 URL，不支持，已忽略")
					}
				}
			} else {
				// 如果都没有，直接传递整个 partMap（可能包含其他格式的数据）
				claudeContent = append(claudeContent, partMap)
				logger.Printf("警告: %s 类型缺少 source 或 image_url，直接传递原始数据", partType)
			}
		default:
			// 其他类型，忽略或转换为文本
			logger.Printf("未知的 content 类型: %s", partType)
		}
	}

	return claudeContent, nil
}

// NormalizeOpenAIRequest 规范化 OpenAI 格式的请求（用于 OpenAI 兼容的 API，如 DeepSeek）
// 主要功能：
// 1. 处理 system prompt 的注入和拼接
// 2. 将数组格式的 content 转换为字符串（OpenAI 兼容 API 只支持字符串）
func (s *SilicoidFormatConverterService) NormalizeOpenAIRequest(openaiRequest map[string]interface{}) (map[string]interface{}, error) {
	logger.Println("开始规范化 OpenAI 请求格式")

	// 添加客户端执行器工具（如果角色支持）
	s.AddExecutorTools(openaiRequest)

	// 提取请求中的关键信息
	messages, _ := openaiRequest["messages"].([]interface{})
	
	// 获取 model_code，用于判断是否是 Kimi 模型
	modelCode, _ := openaiRequest["model_code"].(string)
	isKimiModel := strings.ToLower(modelCode) == "kimi" || strings.Contains(strings.ToLower(modelCode), "kimi")
	if isKimiModel {
		logger.Printf("✅ 检测到 Kimi 模型，将按照 Kimi API 格式处理文件内容")
	}
	
	// 处理 system_prompt：从 Redis 获取并注入到请求中
	systemMessage := s.processSystemPrompt(openaiRequest)
	
	// 获取 userId（如果存在）
	userId, _ := openaiRequest["_user_id"].(string)
	if userId == "" {
		// 兼容调用方仅传递 user_id 的情况（如 WebSocket 路径）
		if uid, ok := openaiRequest["user_id"].(string); ok && uid != "" {
			userId = uid
		}
	}
	if userId == "" {
		// 再次兜底：少数路径可能使用 user 字段
		if uid, ok := openaiRequest["user"].(string); ok && uid != "" {
			userId = uid
		}
	}
	if userId != "" {
		logger.Printf("已解析到用户ID用于文件读取: %s", userId)
	} else {
		logger.Printf("未解析到用户ID，将无法执行 file_read 从用户文件读取")
	}
	
	// 处理消息列表
	var normalizedMessages []map[string]interface{}
	
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]interface{})
		if !ok {
			continue
		}
		
		role, _ := msgMap["role"].(string)
		
		// 提取 content，可能是字符串或数组
		var contentStr string
		
		switch content := msgMap["content"].(type) {
		case string:
			// 如果已经是字符串，直接使用
			contentStr = content
			
		case []interface{}:
			// 如果是数组格式，需要转换为字符串
			logger.Printf("检测到数组格式的 content，正在转换为字符串")
			var textParts []string
			
			for i, part := range content {
				partMap, ok := part.(map[string]interface{})
				if !ok {
					logger.Printf("警告: 数组元素 %d 不是 map 格式，跳过", i)
					continue
				}
				
				partType, _ := partMap["type"].(string)
				logger.Printf("处理数组元素 %d，类型: %s", i, partType)
				
				if partType == "text" {
					// 提取文本内容
					if text, ok := partMap["text"].(string); ok {
						textParts = append(textParts, text)
						logger.Printf("提取到文本内容: %s", text[:min(len(text), 100)])
					} else {
						logger.Printf("警告: 无法提取文本内容，partMap: %v", partMap)
					}
				} else if partType == "image_url" {
					// image_url 类型应该通过 file_read 从后端读取，不支持直接传入
					logger.Printf("警告: 检测到 image_url 类型，应使用 file_read 类型从后端读取文件，已忽略")
				} else if partType == "image" || partType == "document" {
					// 图片和文档类型，直接使用接口提供的数据（不需要 file_id）
					// OpenAI 兼容 API 只支持文本，所以转换为文本提示
					if _, ok := partMap["source"].(map[string]interface{}); ok {
						// Claude 格式的 source 字段
						textParts = append(textParts, fmt.Sprintf("[%s: Claude格式数据]", partType))
					} else if imageUrl, ok := partMap["image_url"].(map[string]interface{}); ok {
						// OpenAI 格式的 image_url
						if url, ok := imageUrl["url"].(string); ok {
							if strings.HasPrefix(url, "data:") {
								// base64 图片，OpenAI 兼容 API 可能不支持，添加提示
								textParts = append(textParts, fmt.Sprintf("[%s: base64数据]", partType))
							} else {
								logger.Printf("警告: %s 类型包含非 base64 的 URL，不支持，已忽略", partType)
							}
						}
					} else {
						// 其他格式，直接忽略或添加提示
						logger.Printf("警告: %s 类型在 OpenAI 兼容 API 中不支持，已忽略", partType)
						textParts = append(textParts, fmt.Sprintf("[%s: 不支持的类型]", partType))
					}
				} else if partType == "file_read" {
					// 处理文件读取：从文件服务器读取文件
					if fileId, ok := partMap["file_id"].(string); ok {
						logger.Printf("检测到文件ID引用: %s，正在从文件服务器读取", fileId)
						if userId != "" {
							// 提取 MD5 哈希值（支持 file_hash 或 md5 字段名）
							var fileHash string
							if hash, ok := partMap["file_hash"].(string); ok && hash != "" {
								fileHash = hash
							} else if hash, ok := partMap["md5"].(string); ok && hash != "" {
								fileHash = hash
							}
							
							fileBytes, mimeType, err := s.getFileFromUserfiles(fileId, userId, fileHash)
							if err != nil {
								logger.Printf("读取文件失败 (file_id: %s): %v", fileId, err)
								textParts = append(textParts, fmt.Sprintf("[文件读取失败: %s]", fileId))
							} else {
								// 使用统一的文件转文本工具处理文件内容
								// 注意：Kimi API 要求将文件内容直接放在 content 中，而不是文件 ID
								result := ConvertFileSmart(fileBytes, mimeType, fileId, "kimi")
								
								if result.Success {
									// 检查文件内容是否过大，需要分批处理
									fileContent := result.Text
									contentLength := len(fileContent)
									
									if contentLength > maxContentSize {
										// 文件内容太大，需要分批处理
										logger.Printf("⚠️  检测到大文件 (file_id: %s, 类型: %s, 长度: %d 字符，超过阈值: %d)，将分批处理", 
											fileId, mimeType, contentLength, maxContentSize)
										
										// 只取第一块内容
										firstChunk := fileContent
										if len(firstChunk) > largeFileChunkSize {
											firstChunk = firstChunk[:largeFileChunkSize]
										}
										textParts = append(textParts, firstChunk)
										
										// 将剩余内容分块并存储到请求数据中，供后续分批投喂使用
										remainingContent := fileContent[len(firstChunk):]
										if len(remainingContent) > 0 {
											// 初始化大文件分块列表
											var largeFileChunks []map[string]interface{}
											if existingChunks, ok := openaiRequest["_large_file_chunks"].([]interface{}); ok {
												for _, chunk := range existingChunks {
													if chunkMap, ok := chunk.(map[string]interface{}); ok {
														largeFileChunks = append(largeFileChunks, chunkMap)
													}
												}
											}
											
											// 将剩余内容分块
											chunkIndex := 1
											for i := 0; i < len(remainingContent); i += largeFileChunkSize {
												end := i + largeFileChunkSize
												if end > len(remainingContent) {
													end = len(remainingContent)
												}
												
												chunk := remainingContent[i:end]
												chunkIndex++
												
												largeFileChunks = append(largeFileChunks, map[string]interface{}{
													"file_id": fileId,
													"index":   chunkIndex,
													"content": chunk,
													"is_last": end >= len(remainingContent),
												})
											}
											
											openaiRequest["_large_file_chunks"] = largeFileChunks
											logger.Printf("✅ 已将大文件分块 (file_id: %s, 总块数: %d, 第一块长度: %d, 剩余块数: %d)", 
												fileId, len(largeFileChunks)+1, len(firstChunk), len(largeFileChunks))
										}
										
										if isKimiModel {
											logger.Printf("✅ [Kimi] 已提取文件内容（第一块）并放入 content (file_id: %s, 类型: %s, 第一块长度: %d, 总长度: %d)", 
												fileId, mimeType, len(firstChunk), contentLength)
										} else {
											logger.Printf("✅ 已提取文件内容（第一块）(file_id: %s, 类型: %s, 第一块长度: %d, 总长度: %d)", 
												fileId, mimeType, len(firstChunk), contentLength)
										}
									} else {
										// 文件内容不大，直接使用
										textParts = append(textParts, fileContent)
										if isKimiModel {
											logger.Printf("✅ [Kimi] 已提取文件内容并直接放入 content (file_id: %s, 类型: %s, 长度: %d)", 
												fileId, mimeType, contentLength)
										} else {
											logger.Printf("✅ 已提取文件内容 (file_id: %s, 类型: %s, 长度: %d)", 
												fileId, mimeType, contentLength)
										}
									}
									
									// 如果是二进制 PDF，对于 Kimi 模型仍然传递内容（Kimi 可能支持）
									if result.IsBinary && mimeType == "application/pdf" && isKimiModel {
										logger.Printf("⚠️  [Kimi] PDF 文件是二进制格式，但已尝试将内容放入 content (file_id: %s, 大小: %d bytes)", 
											fileId, len(fileBytes))
									}
								} else {
									// 提取失败，根据情况处理
									if result.IsBinary && mimeType == "application/pdf" && isKimiModel {
										// 对于 Kimi，即使是二进制 PDF，也尝试传递内容（Kimi 可能支持）
										// 根据 Kimi 文档，应该传递文件内容
										textParts = append(textParts, result.Text)
										logger.Printf("⚠️  [Kimi] PDF 文件是二进制格式，但已尝试将内容放入 content (file_id: %s, 大小: %d bytes, 错误: %s)", 
											fileId, len(fileBytes), result.ErrorMsg)
									} else {
										// 其他情况，使用错误信息或提示文本
										if result.Text != "" {
											textParts = append(textParts, result.Text)
										} else {
											textParts = append(textParts, fmt.Sprintf("[文件处理失败: %s, 错误: %s]", fileId, result.ErrorMsg))
										}
										logger.Printf("⚠️  文件处理失败 (file_id: %s, 类型: %s, 错误: %s)", 
											fileId, mimeType, result.ErrorMsg)
									}
								}
							}
						} else {
							// 没有 userId，无法读取文件
							logger.Printf("未提供 userId，跳过从用户文件读取，使用占位符记录: %s", fileId)
							textParts = append(textParts, fmt.Sprintf("[文件: %s]", fileId))
						}
					}
				} else {
					logger.Printf("警告: 未知的数组元素类型: %s", partType)
				}
			}
			
			// 拼接所有文本部分
			contentStr = strings.Join(textParts, "\n")
			logger.Printf("数组转换完成，提取到 %d 个文本部分，最终内容长度: %d", len(textParts), len(contentStr))
			
		default:
			// 其他类型，尝试转换为字符串
			contentStr = fmt.Sprintf("%v", content)
		}
		
		// 确保内容非空
		if contentStr == "" && role != "system" {
			contentStr = "请继续"
		}
		
		// 处理不同角色的消息
		logger.Printf("处理消息，角色: %s, 内容长度: %d", role, len(contentStr))
		if role == "system" {
			// 如果已经处理了 system_prompt，忽略 messages 中的 system 消息
			// 避免重复添加 system_prompt
			if systemMessage != "" {
				logger.Println("⚠️  已处理 system_prompt，忽略 messages 中的 system 消息，避免重复")
			} else {
				// 只有在没有处理 system_prompt 时才使用 messages 中的 system 消息
				systemMessage = contentStr
				logger.Println("使用 messages 中的 system 消息")
			}
			// 不将 system 消息添加到 normalizedMessages（OpenAI API 支持顶级 system 参数）
			
		} else {
			// user, assistant, tool 等角色的消息
			normalizedMsg := map[string]interface{}{
				"role":    role,
				"content": contentStr,
			}
			
			// 保留其他字段（如 name, tool_calls 等）
			for key, value := range msgMap {
				if key != "role" && key != "content" {
					normalizedMsg[key] = value
				}
			}
			
			normalizedMessages = append(normalizedMessages, normalizedMsg)
			logger.Printf("添加 %s 消息到规范化列表，内容: %s", role, contentStr[:min(len(contentStr), 100)])
		}
	}
	
	// 如果消息列表为空，添加一个默认用户消息
	if len(normalizedMessages) == 0 {
		normalizedMessages = append(normalizedMessages, map[string]interface{}{
			"role":    "user",
			"content": "Hello",
		})
	}
	
	// 构建规范化后的请求
	normalizedRequest := make(map[string]interface{})
	
	// 需要保留的内部标记（需要传递给后续处理）
	preservedInternalKeys := map[string]bool{
		"_system_prompt":    true,
		"_base_url":         true,
		"_endpoint":         true,
		"_use_user_key":     true,
		"_user_openai_key":  true,
		"_user_claude_key":  true,
		"_large_file_chunks": true, // 保留大文件分块信息，供后续分批投喂使用
		"model_code":        true,
		"role_name":         true, // 保留 role_name，因为流式请求可能需要再次使用
	}
	
	// 复制原始请求的所有参数
	for key, value := range openaiRequest {
		// 保留需要传递的内部标记、非内部标记（除了 messages）
		if preservedInternalKeys[key] || (!strings.HasPrefix(key, "_") && key != "messages") {
			normalizedRequest[key] = value
		}
	}
	
	// 如果有 system message，将其作为第一条消息添加到消息列表开头
	if systemMessage != "" {
		// 创建 system 消息并插入到消息列表开头
		systemMsg := map[string]interface{}{
			"role":    "system",
			"content": systemMessage,
		}
		// 将 system 消息插入到消息列表开头
		normalizedMessages = append([]map[string]interface{}{systemMsg}, normalizedMessages...)
		logger.Printf("✅ 已将 system prompt 作为第一条 system 消息 (长度: %d，前100字符: %s)", len(systemMessage), truncateString(systemMessage, 100))
	} else {
		logger.Println("⚠️  没有 system message 需要添加")
	}
	
	// 设置规范化后的 messages
	normalizedRequest["messages"] = normalizedMessages

	// 如果有工具，设置 tool_choice 为 auto 来启用工具调用
	if tools, ok := normalizedRequest["tools"].([]interface{}); ok && len(tools) > 0 {
		normalizedRequest["tool_choice"] = "auto"
		logger.Printf("✅ 设置 tool_choice=auto 以启用工具调用 (%d 个工具)", len(tools))
	}

	logger.Printf("规范化完成: 处理了 %d 条消息", len(normalizedMessages))
	return normalizedRequest, nil
}

// getFileExtension 根据 MIME 类型获取文件扩展名
func getFileExtension(mimeType string) string {
	switch mimeType {
	case "application/pdf":
		return ".pdf"
	case "text/plain":
		return ".txt"
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		if strings.HasPrefix(mimeType, "image/") {
			return ".img"
		}
		return ""
	}
}






// contains 检查字符串切片是否包含特定元素
func contains(slice []string, str string) bool {
	for _, item := range slice {
		if item == str {
			return true
		}
	}
	return false
}



// NormalizeModelsResponse 统一不同 API 返回的模型列表格式为 OpenAI 兼容格式
// 输入: 任意格式的模型列表响应
// 输出: 统一的 OpenAI 格式 {"object": "list", "data": [...]}
func (s *SilicoidFormatConverterService) NormalizeModelsResponse(provider string, rawResponse map[string]interface{}) (map[string]interface{}, error) {
	logger.Printf("开始规范化 %s 模型列表响应格式", provider)
	
	// 检查是否已经是 OpenAI 格式
	if object, ok := rawResponse["object"].(string); ok && object == "list" {
		if data, ok := rawResponse["data"].([]interface{}); ok {
			logger.Printf("响应已经是 OpenAI 格式，包含 %d 个模型", len(data))
			return rawResponse, nil
		}
	}
	
	// 尝试从不同格式中提取模型列表
	var models []interface{}
	
	// 方式1: 检查是否有 "data" 字段
	if data, ok := rawResponse["data"].([]interface{}); ok {
		models = data
		logger.Printf("从 'data' 字段提取到 %d 个模型", len(models))
	} else if data, ok := rawResponse["data"].(map[string]interface{}); ok {
		// 如果 data 是 map，尝试提取其中的列表
		if list, ok := data["models"].([]interface{}); ok {
			models = list
			logger.Printf("从 'data.models' 字段提取到 %d 个模型", len(models))
		}
	}
	
	// 方式2: 检查是否有 "models" 字段
	if len(models) == 0 {
		if modelList, ok := rawResponse["models"].([]interface{}); ok {
			models = modelList
			logger.Printf("从 'models' 字段提取到 %d 个模型", len(models))
		}
	}
	
	// 方式3: 如果 rawResponse 本身就是数组格式（某些 API 可能直接返回数组）
	// 注意：由于 rawResponse 是 map[string]interface{}，这种情况较少见
	// 但我们可以检查是否有其他可能的字段名
	if len(models) == 0 {
		// 尝试查找其他可能的字段名
		for key, value := range rawResponse {
			if key != "object" && key != "data" && key != "models" {
				if modelArray, ok := value.([]interface{}); ok {
					models = modelArray
					logger.Printf("从字段 '%s' 提取到 %d 个模型", key, len(models))
					break
				}
			}
		}
	}
	
	// 如果仍然无法提取模型列表，记录警告并返回空列表
	if len(models) == 0 {
		logger.Printf("⚠️  警告: 无法从 %s API 响应中提取模型列表，返回空列表", provider)
	}
	
	// 规范化每个模型对象的格式
	var normalizedModels []interface{}
	for i, model := range models {
		modelMap, ok := model.(map[string]interface{})
		if !ok {
			logger.Printf("警告: 模型 %d 不是 map 格式，跳过", i)
			continue
		}
		
		// 确保模型对象有必要的字段
		normalizedModel := make(map[string]interface{})
		
		// 复制所有现有字段
		for key, value := range modelMap {
			normalizedModel[key] = value
		}
		
		// 确保有 "id" 字段
		if _, hasID := normalizedModel["id"]; !hasID {
			// 尝试从其他字段获取 ID
			if name, ok := normalizedModel["name"].(string); ok {
				normalizedModel["id"] = name
			} else if modelID, ok := normalizedModel["model_id"].(string); ok {
				normalizedModel["id"] = modelID
			} else {
				normalizedModel["id"] = fmt.Sprintf("model-%d", i)
			}
		}
		
		// 确保有 "object" 字段
		if _, hasObject := normalizedModel["object"]; !hasObject {
			normalizedModel["object"] = "model"
		}
		
		// 确保有 "created" 字段
		if _, hasCreated := normalizedModel["created"]; !hasCreated {
			normalizedModel["created"] = int(time.Now().Unix())
		}
		
		// 确保有 "owned_by" 字段
		if _, hasOwnedBy := normalizedModel["owned_by"]; !hasOwnedBy {
			normalizedModel["owned_by"] = strings.ToLower(provider)
		}
		
		normalizedModels = append(normalizedModels, normalizedModel)
	}
	
	// 构建统一的响应格式
	normalizedResponse := map[string]interface{}{
		"object": "list",
		"data":   normalizedModels,
	}
	
	logger.Printf("✅ 规范化完成: 共 %d 个模型", len(normalizedModels))
	return normalizedResponse, nil
}