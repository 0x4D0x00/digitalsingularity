package formatconverter

import (
	"encoding/json"
	"log"
	"strings"
)

// TTSFormatConverter TTS 格式转换器接口
type TTSFormatConverter interface {
	// ConvertAudioData 转换音频数据格式
	// 将提供商特定的音频格式转换为标准格式（如 PCM）
	ConvertAudioData(data []byte) ([]byte, error)
	// PreprocessTextForTTS 预处理文本（如从 SSE 格式中提取纯文本）
	// 将可能包含 SSE 格式的文本转换为纯文本，用于 TTS 合成
	PreprocessTextForTTS(text string) (string, error)
}

// TTSFormatConverterService TTS 格式转换器服务
type TTSFormatConverterService struct {
	logger *log.Logger
}

// NewTTSFormatConverterService 创建新的 TTS 格式转换器服务
func NewTTSFormatConverterService() *TTSFormatConverterService {
	return &TTSFormatConverterService{
		logger: log.New(log.Writer(), "[TTSFormatConverter] ", log.LstdFlags),
	}
}

// GetConverter 根据provider获取对应的 TTS 格式转换器
func (s *TTSFormatConverterService) GetConverter(provider SpeechProvider) TTSFormatConverter {
	switch provider {
	case ProviderAlibaba:
		return NewAlibabaTTSConverter()
	case ProviderTencent:
		return NewTencentTTSConverter()
	default:
		s.logger.Printf("未知的provider: %s，使用腾讯转换器作为默认", provider)
		return NewTencentTTSConverter()
	}
}

// AlibabaTTSConverter 阿里 TTS 格式转换器
type AlibabaTTSConverter struct {
	logger *log.Logger
}

// NewAlibabaTTSConverter 创建阿里 TTS 转换器
func NewAlibabaTTSConverter() *AlibabaTTSConverter {
	return &AlibabaTTSConverter{
		logger: log.New(log.Writer(), "[AlibabaTTSConverter] ", log.LstdFlags),
	}
}

// ConvertAudioData 转换阿里 TTS 音频数据为标准格式
func (c *AlibabaTTSConverter) ConvertAudioData(data []byte) ([]byte, error) {
	// TODO: 实现阿里 TTS 音频格式转换
	// 例如：可能需要将阿里返回的音频格式转换为标准 PCM 格式
	// 或者调整采样率、声道数等参数
	c.logger.Printf("转换阿里 TTS 音频数据，长度: %d", len(data))
	
	// 暂时直接返回原始数据，待后续实现具体转换逻辑
	return data, nil
}

// PreprocessTextForTTS 预处理文本（从 SSE 格式中提取纯文本）
func (c *AlibabaTTSConverter) PreprocessTextForTTS(text string) (string, error) {
	return preprocessTextFromSSE(text, c.logger)
}

// TencentTTSConverter 腾讯 TTS 格式转换器
type TencentTTSConverter struct {
	logger *log.Logger
}

// NewTencentTTSConverter 创建腾讯 TTS 转换器
func NewTencentTTSConverter() *TencentTTSConverter {
	return &TencentTTSConverter{
		logger: log.New(log.Writer(), "[TencentTTSConverter] ", log.LstdFlags),
	}
}

// ConvertAudioData 转换腾讯 TTS 音频数据为标准格式
func (c *TencentTTSConverter) ConvertAudioData(data []byte) ([]byte, error) {
	// TODO: 实现腾讯 TTS 音频格式转换
	// 腾讯 TTS 默认返回 PCM 格式（16kHz, 16bit, 单声道）
	// 如果客户端需要其他格式（如 MP3、不同的采样率等），可以在这里进行转换
	c.logger.Printf("转换腾讯 TTS 音频数据，长度: %d", len(data))
	
	// 暂时直接返回原始数据，待后续实现具体转换逻辑
	// 腾讯 TTS 默认返回的 PCM 格式已经是标准格式，可以直接使用
	return data, nil
}

// PreprocessTextForTTS 预处理文本（从 SSE 格式中提取纯文本）
func (c *TencentTTSConverter) PreprocessTextForTTS(text string) (string, error) {
	return preprocessTextFromSSE(text, c.logger)
}

// preprocessTextFromSSE 从 SSE 格式中提取纯文本
// 支持两种格式：
// 1. SSE 格式：data: {"id":"...","choices":[{"delta":{"content":"text"}}]}
// 2. 纯文本格式：直接返回
func preprocessTextFromSSE(text string, logger *log.Logger) (string, error) {
	// 如果文本为空，直接返回
	if strings.TrimSpace(text) == "" {
		return "", nil
	}
	
	// 检查是否是 SSE 格式（包含 "data: " 前缀）
	lines := strings.Split(text, "\n")
	var extractedText strings.Builder
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		// 检查是否是 SSE 数据行
		if strings.HasPrefix(line, "data: ") {
			dataStr := strings.TrimPrefix(line, "data: ")
			
			// 检查是否是结束标记
			if dataStr == "[DONE]" {
				continue
			}
			
			// 尝试解析 JSON
			var jsonData map[string]interface{}
			if err := json.Unmarshal([]byte(dataStr), &jsonData); err != nil {
				// 如果不是 JSON，可能是纯文本，直接使用
				if strings.TrimSpace(dataStr) != "" {
					extractedText.WriteString(dataStr)
				}
				continue
			}
			
			// 提取 choices[0].delta.content 或 choices[0].message.content
			if choices, ok := jsonData["choices"].([]interface{}); ok && len(choices) > 0 {
				if choice, ok := choices[0].(map[string]interface{}); ok {
					// 尝试获取 delta.content（流式响应）
					if delta, ok := choice["delta"].(map[string]interface{}); ok {
						if content, ok := delta["content"].(string); ok && content != "" {
							extractedText.WriteString(content)
							continue
						}
					}
					
					// 尝试获取 message.content（完整响应）
					if message, ok := choice["message"].(map[string]interface{}); ok {
						if content, ok := message["content"].(string); ok && content != "" {
							extractedText.WriteString(content)
							continue
						}
					}
				}
			}
		} else {
			// 不是 SSE 格式，直接作为纯文本处理
			// 但需要检查是否包含 JSON 结构
			if strings.HasPrefix(line, "{") || strings.HasPrefix(line, "[") {
				// 可能是 JSON 格式，尝试解析
				var jsonData map[string]interface{}
				if err := json.Unmarshal([]byte(line), &jsonData); err == nil {
					// 成功解析为 JSON，尝试提取内容
					if choices, ok := jsonData["choices"].([]interface{}); ok && len(choices) > 0 {
						if choice, ok := choices[0].(map[string]interface{}); ok {
							if delta, ok := choice["delta"].(map[string]interface{}); ok {
								if content, ok := delta["content"].(string); ok && content != "" {
									extractedText.WriteString(content)
									continue
								}
							}
							if message, ok := choice["message"].(map[string]interface{}); ok {
								if content, ok := message["content"].(string); ok && content != "" {
									extractedText.WriteString(content)
									continue
								}
							}
						}
					}
				}
			}
			
			// 纯文本或无法解析，直接添加
			extractedText.WriteString(line)
		}
	}
	
	result := extractedText.String()
	if logger != nil && result != text {
		logger.Printf("文本预处理完成：原始长度 %d -> 提取后长度 %d", len(text), len(result))
	}
	
	return result, nil
}
