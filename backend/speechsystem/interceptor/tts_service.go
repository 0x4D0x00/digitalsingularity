package interceptor

import (
	"errors"
	"fmt"
	"log"

	"digitalsingularity/backend/speechsystem/formatconverter"
	tencenttts "digitalsingularity/backend/speechsystem/tencent/tts"
)

// SynthesisResultCallback TTS 合成结果回调接口
// 拦截器通过此接口通知外部合成结果，外部（如WebSocket层）负责将结果发送给客户端
type SynthesisResultCallback interface {
	// OnAudioChunk 流式音频数据块回调（识别过程中实时返回）
	OnAudioChunk(audioData []byte)
	// OnSynthesisComplete 合成完成回调（合成完成时返回完整音频）
	OnSynthesisComplete(audioData []byte)
	// OnError 合成错误回调
	OnError(err error)
}

// TTSSynthesizer TTS 合成器接口
// 合成器职责：
// 1. 接收文本和语音性别参数
// 2. 调用对应的 TTS 服务（阿里/腾讯）
// 3. 通过回调返回合成结果
type TTSSynthesizer interface {
	// Synthesize 同步合成（通过回调返回结果）
	Synthesize(text string, voiceGender string, callback SynthesisResultCallback) error
	// SynthesizeStream 流式合成（通过回调返回结果）
	SynthesizeStream(text string, voiceGender string, callback SynthesisResultCallback) error
	// GetProvider 获取提供商
	GetProvider() formatconverter.SpeechProvider
	// SetCallback 设置回调（类似 ASR 的 SetCallback）
	SetCallback(callback SynthesisResultCallback)
}

// TTSService TTS 拦截器服务
type TTSService struct {
	alibabaSynthesizer TTSSynthesizer
	tencentSynthesizer TTSSynthesizer
	currentSynthesizer TTSSynthesizer
	formatConverterService *formatconverter.TTSFormatConverterService
	logger             *log.Logger
}

// NewTTSService 创建新的 TTS 服务
func NewTTSService() *TTSService {
	return &TTSService{
		formatConverterService: formatconverter.NewTTSFormatConverterService(),
		logger: log.New(log.Writer(), "[TTSService] ", log.LstdFlags),
	}
}

// CreateSynthesizer 创建并返回可用的合成器
// 暂时跳过阿里服务（暂时不可用），直接使用腾讯服务
func (s *TTSService) CreateSynthesizer() (TTSSynthesizer, error) {
	// 暂时跳过阿里服务（暂时不可用）
	// 代码结构保留，待后续实现阿里服务时可启用
	if s.alibabaSynthesizer == nil {
		s.alibabaSynthesizer = NewAlibabaSynthesizer()
	}

	if err := s.alibabaSynthesizer.Synthesize("test", "female", nil); err == nil {
		s.logger.Printf("成功连接到阿里TTS服务")
		s.currentSynthesizer = s.alibabaSynthesizer
		return s.alibabaSynthesizer, nil
	} else {
		s.logger.Printf("阿里TTS服务暂时不可用: %v，使用腾讯服务", err)
	}

	// 使用腾讯服务
	if s.tencentSynthesizer == nil {
		tencentSynth, err := NewTencentSynthesizer()
		if err != nil {
			s.logger.Printf("创建腾讯TTS合成器失败: %v", err)
			return nil, err
		}
		s.tencentSynthesizer = tencentSynth
	}

	s.currentSynthesizer = s.tencentSynthesizer
	s.logger.Printf("成功连接到腾讯TTS服务")
	return s.tencentSynthesizer, nil
}

// GetCurrentSynthesizer 获取当前使用的合成器
func (s *TTSService) GetCurrentSynthesizer() TTSSynthesizer {
	return s.currentSynthesizer
}

// SynthesizeWithCallback 通过回调机制进行语音合成
func (s *TTSService) SynthesizeWithCallback(text string, voiceGender string, callback SynthesisResultCallback) error {
	if s.currentSynthesizer == nil {
		// 如果还没有创建合成器，先创建一个
		_, err := s.CreateSynthesizer()
		if err != nil {
			return err
		}
	}

	if s.currentSynthesizer == nil {
		return errors.New("没有可用的TTS合成器")
	}

	// 设置回调
	s.currentSynthesizer.SetCallback(callback)

	// 触发合成（使用流式合成）
	return s.currentSynthesizer.SynthesizeStream(text, voiceGender, callback)
}

// AlibabaSynthesizer 阿里 TTS 合成器
type AlibabaSynthesizer struct {
	provider formatconverter.SpeechProvider
	callback SynthesisResultCallback
}

// NewAlibabaSynthesizer 创建阿里合成器
func NewAlibabaSynthesizer() *AlibabaSynthesizer {
	return &AlibabaSynthesizer{
		provider: formatconverter.ProviderAlibaba,
	}
}

// Synthesize 同步合成
func (s *AlibabaSynthesizer) Synthesize(text string, voiceGender string, callback SynthesisResultCallback) error {
	// TODO: 实现阿里TTS同步合成
	// 当实现后，需要在这里调用格式转换器：
	// converter := formatconverter.NewTTSFormatConverterService().GetConverter(s.provider)
	// convertedAudioData, err := converter.ConvertAudioData(audioData)
	return errors.New("阿里TTS服务暂时不可用，请使用腾讯TTS服务")
}

// SynthesizeStream 流式合成
func (s *AlibabaSynthesizer) SynthesizeStream(text string, voiceGender string, callback SynthesisResultCallback) error {
	// TODO: 实现阿里TTS流式合成
	// 当实现后，需要在处理音频流时调用格式转换器：
	// converter := formatconverter.NewTTSFormatConverterService().GetConverter(s.provider)
	// convertedChunk, err := converter.ConvertAudioData(audioChunk)
	if callback != nil {
		callback.OnError(errors.New("阿里TTS服务暂时不可用，请使用腾讯TTS服务"))
	}
	return errors.New("阿里TTS服务暂时不可用，请使用腾讯TTS服务")
}

// GetProvider 获取提供商
func (s *AlibabaSynthesizer) GetProvider() formatconverter.SpeechProvider {
	return s.provider
}

// SetCallback 设置回调
func (s *AlibabaSynthesizer) SetCallback(callback SynthesisResultCallback) {
	s.callback = callback
}

// TencentSynthesizer 腾讯 TTS 合成器
type TencentSynthesizer struct {
	provider   formatconverter.SpeechProvider
	callback   SynthesisResultCallback
	ttsService *tencenttts.TencentSynthesisService
	logger     *log.Logger
}

// NewTencentSynthesizer 创建腾讯合成器
func NewTencentSynthesizer() (*TencentSynthesizer, error) {
	ttsService, err := tencenttts.NewTencentSynthesisService()
	if err != nil {
		return nil, err
	}

	return &TencentSynthesizer{
		provider:   formatconverter.ProviderTencent,
		ttsService: ttsService,
		logger:     log.New(log.Writer(), "[TencentSynthesizer] ", log.LstdFlags),
	}, nil
}

// ttsCallbackAdapter 适配拦截器回调和腾讯TTS服务的回调
type ttsCallbackAdapter struct {
	callback SynthesisResultCallback
	logger   *log.Logger
}

func (a *ttsCallbackAdapter) OnAudioChunk(audioData []byte) {
	if a.callback != nil {
		a.callback.OnAudioChunk(audioData)
	}
}

func (a *ttsCallbackAdapter) OnSynthesisComplete(audioData []byte) {
	if a.callback != nil {
		a.callback.OnSynthesisComplete(audioData)
	}
}

func (a *ttsCallbackAdapter) OnError(err error) {
	if a.callback != nil {
		a.callback.OnError(err)
	}
}

// Synthesize 同步合成
func (s *TencentSynthesizer) Synthesize(text string, voiceGender string, callback SynthesisResultCallback) error {
	// 根据语音性别设置音色
	err := s.setVoiceType(voiceGender)
	if err != nil {
		if callback != nil {
			callback.OnError(err)
		}
		return err
	}

	// 使用格式转换器预处理文本（从 SSE 格式中提取纯文本）
	converter := formatconverter.NewTTSFormatConverterService().GetConverter(s.provider)
	preprocessedText, err := converter.PreprocessTextForTTS(text)
	if err != nil {
		s.logger.Printf("文本预处理失败: %v，使用原始文本", err)
		preprocessedText = text // 预处理失败时使用原始文本
	} else if preprocessedText != text {
		s.logger.Printf("文本预处理完成：原始长度 %d -> 预处理后长度 %d", len(text), len(preprocessedText))
	}

	// 调用腾讯TTS服务进行同步合成
	audioData, err := s.ttsService.Synthesize(preprocessedText)
	if err != nil {
		if callback != nil {
			callback.OnError(err)
		}
		return err
	}

	// 通过格式转换器转换音频数据（复用已有的 converter）
	convertedAudioData, err := converter.ConvertAudioData(audioData)
	if err != nil {
		s.logger.Printf("音频格式转换失败: %v，使用原始数据", err)
		convertedAudioData = audioData // 转换失败时使用原始数据
	}

	// 通过回调返回结果
	if callback != nil {
		callback.OnSynthesisComplete(convertedAudioData)
	}

	return nil
}

// SynthesizeStream 流式合成
func (s *TencentSynthesizer) SynthesizeStream(text string, voiceGender string, callback SynthesisResultCallback) error {
	// 根据语音性别设置音色
	err := s.setVoiceType(voiceGender)
	if err != nil {
		if callback != nil {
			callback.OnError(err)
		}
		return err
	}

	// 使用格式转换器预处理文本（从 SSE 格式中提取纯文本）
	converter := formatconverter.NewTTSFormatConverterService().GetConverter(s.provider)
	preprocessedText, err := converter.PreprocessTextForTTS(text)
	if err != nil {
		s.logger.Printf("文本预处理失败: %v，使用原始文本", err)
		preprocessedText = text // 预处理失败时使用原始文本
	} else if preprocessedText != text {
		s.logger.Printf("文本预处理完成：原始长度 %d -> 预处理后长度 %d", len(text), len(preprocessedText))
	}

	// 调用腾讯TTS服务进行流式合成
	audioQueue, err := s.ttsService.SynthesizeStream(preprocessedText)
	if err != nil {
		if callback != nil {
			callback.OnError(err)
		}
		return err
	}

	// 在单独的 goroutine 中处理音频流
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Printf("处理音频流时发生panic: %v", r)
				if callback != nil {
					callback.OnError(fmt.Errorf("处理音频流时发生panic: %v", r))
				}
			}
		}()

		// 复用外部的格式转换器（用于音频格式转换）
		for audioChunk := range audioQueue {
			// 对流式音频块进行格式转换
			convertedChunk, err := converter.ConvertAudioData(audioChunk)
			if err != nil {
				s.logger.Printf("音频块格式转换失败: %v，使用原始数据", err)
				convertedChunk = audioChunk // 转换失败时使用原始数据
			}
			
			// 通过流式回调发送音频块（前端实时播放）
			if callback != nil {
				callback.OnAudioChunk(convertedChunk)
			}
		}

		// 流式完成，发送完成信号（不包含音频数据，避免重复播放）
		// 流式音频块已经通过 OnAudioChunk 发送并播放，这里只发送完成信号
		if callback != nil {
			callback.OnSynthesisComplete(nil)
		}
	}()

	return nil
}

// setVoiceType 根据语音性别设置音色类型
func (s *TencentSynthesizer) setVoiceType(voiceGender string) error {
	// 需要从数据库读取配置来获取音色类型
	// 这里暂时使用默认值，实际实现需要从配置中读取
	// TODO: 从数据库配置中读取 MaleVoiceType 和 FemaleVoiceType
	// 目前 TencentSynthesisService 在初始化时已经读取了配置，但我们需要根据 voiceGender 动态设置
	
	// 注意：当前 TencentSynthesisService 的实现不支持动态设置音色
	// 需要修改 TencentSynthesisService 以支持在每次合成时设置音色
	// 暂时先使用默认配置，后续可以优化
	
	return nil
}

// GetProvider 获取提供商
func (s *TencentSynthesizer) GetProvider() formatconverter.SpeechProvider {
	return s.provider
}

// SetCallback 设置回调
func (s *TencentSynthesizer) SetCallback(callback SynthesisResultCallback) {
	s.callback = callback
}
