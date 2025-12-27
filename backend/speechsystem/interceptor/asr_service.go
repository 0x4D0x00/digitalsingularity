package interceptor

import (
	"errors"
	"log"

	"digitalsingularity/backend/speechsystem/formatconverter"
	"digitalsingularity/backend/speechsystem/tencent/asr"
)

// RecognitionResultCallback 识别结果回调接口
// 拦截器通过此接口通知外部识别结果，外部（如WebSocket层）负责将结果发送给客户端
type RecognitionResultCallback interface {
	// OnSentenceBegin 句子开始回调
	OnSentenceBegin()
	// OnPartialResult 部分识别结果回调（识别过程中实时更新）
	// text: 当前识别的文本
	OnPartialResult(text string)
	// OnSentenceEnd 句子结束回调（一句话识别完成）
	// text: 最终识别结果
	OnSentenceEnd(text string)
	// OnError 识别错误回调
	// err: 错误信息
	OnError(err error)
}

// SpeechInterceptor 语音拦截器接口
// 拦截器职责：
// 1. 拦截音频数据
// 2. 转换音频格式
// 3. 将音频数据转发给ASR服务
// 4. 接收ASR服务的识别结果
// 5. 通过回调通知外部识别结果
type SpeechInterceptor interface {
	// Connect 连接到语音服务
	Connect() error
	// Write 写入音频数据（会自动进行格式转换）
	Write(data []byte) error
	// Stop 停止识别
	Stop() error
	// GetProvider 获取提供商
	GetProvider() formatconverter.SpeechProvider
	// IsConnected 检查是否已连接
	IsConnected() bool
	// SetCallback 设置识别结果回调
	// 拦截器通过此回调通知外部识别结果，外部负责将结果发送给客户端
	SetCallback(callback RecognitionResultCallback)
}

// InterceptorService 拦截器服务
type InterceptorService struct {
	alibabaInterceptor SpeechInterceptor
	tencentInterceptor SpeechInterceptor
	currentInterceptor SpeechInterceptor
	logger             *log.Logger
}

// NewInterceptorService 创建新的拦截器服务
func NewInterceptorService() *InterceptorService {
	return &InterceptorService{
		logger: log.New(log.Writer(), "[SpeechInterceptor] ", log.LstdFlags),
	}
}

// CreateInterceptor 创建并返回可用的拦截器
// 暂时跳过阿里服务（暂时不可用），直接使用腾讯服务
func (s *InterceptorService) CreateInterceptor() (SpeechInterceptor, error) {
	// 暂时跳过阿里服务（暂时不可用）
	// 代码结构保留，待后续实现阿里服务时可启用
	if s.alibabaInterceptor == nil {
		s.alibabaInterceptor = NewAlibabaInterceptor()
	}
	
	if err := s.alibabaInterceptor.Connect(); err == nil {
		s.logger.Printf("成功连接到阿里语音服务")
		s.currentInterceptor = s.alibabaInterceptor
		return s.alibabaInterceptor, nil
	} else {
		s.logger.Printf("阿里语音服务暂时不可用: %v，使用腾讯服务", err)
	}
	
	// 使用腾讯服务
	if s.tencentInterceptor == nil {
		s.tencentInterceptor = NewTencentInterceptor()
	}
	
	if err := s.tencentInterceptor.Connect(); err == nil {
		s.logger.Printf("成功连接到腾讯语音服务")
		s.currentInterceptor = s.tencentInterceptor
		return s.tencentInterceptor, nil
	} else {
		s.logger.Printf("连接腾讯语音服务失败: %v", err)
		return nil, err
	}
}

// GetCurrentInterceptor 获取当前使用的拦截器
func (s *InterceptorService) GetCurrentInterceptor() SpeechInterceptor {
	return s.currentInterceptor
}

// AlibabaInterceptor 阿里语音拦截器
type AlibabaInterceptor struct {
	connected bool
	provider  formatconverter.SpeechProvider
	converter formatconverter.FormatConverter
	callback  RecognitionResultCallback
}

// NewAlibabaInterceptor 创建阿里拦截器
func NewAlibabaInterceptor() *AlibabaInterceptor {
	converterService := formatconverter.NewFormatConverterService()
	return &AlibabaInterceptor{
		provider:  formatconverter.ProviderAlibaba,
		converter: converterService.GetConverter(formatconverter.ProviderAlibaba),
	}
}

// Connect 连接到阿里语音服务
func (i *AlibabaInterceptor) Connect() error {
	// TODO: 实现阿里ASR连接逻辑
	// 暂时返回错误，表示阿里服务暂时不可用
	// 后续实现真正的连接逻辑时，需要检查连接是否真的成功
	return errors.New("阿里语音服务暂时不可用，请使用腾讯语音服务")
}

// Write 写入音频数据（会自动进行格式转换）
func (i *AlibabaInterceptor) Write(data []byte) error {
	if !i.connected {
		return nil // TODO: 返回连接错误
	}
	
	// 使用转换器转换音频数据格式
	_, err := i.converter.ConvertAudioData(data)
	if err != nil {
		return err
	}
	
	// TODO: 将转换后的数据写入阿里ASR服务，并在识别结果回调中触发callback
	// convertedData 暂时未使用，待实现阿里ASR服务时使用
	return nil
}

// Stop 停止识别
func (i *AlibabaInterceptor) Stop() error {
	i.connected = false
	// TODO: 实现停止逻辑
	return nil
}

// GetProvider 获取提供商
func (i *AlibabaInterceptor) GetProvider() formatconverter.SpeechProvider {
	return i.provider
}

// IsConnected 检查是否已连接
func (i *AlibabaInterceptor) IsConnected() bool {
	return i.connected
}

// SetCallback 设置识别结果回调
func (i *AlibabaInterceptor) SetCallback(callback RecognitionResultCallback) {
	i.callback = callback
}

// TencentInterceptor 腾讯语音拦截器
type TencentInterceptor struct {
	connected    bool
	provider     formatconverter.SpeechProvider
	converter    formatconverter.FormatConverter
	callback     RecognitionResultCallback
	recognizer   *asr.StreamRecognizer
	asrService   *asr.TencentRecognitionService
	logger       *log.Logger
}

// asrCallbackAdapter 适配拦截器回调和ASR服务回调
type asrCallbackAdapter struct {
	callback RecognitionResultCallback
}

func (a *asrCallbackAdapter) OnSentenceBegin() {
	if a.callback != nil {
		a.callback.OnSentenceBegin()
	}
}

func (a *asrCallbackAdapter) OnPartialResult(text string) {
	if a.callback != nil {
		a.callback.OnPartialResult(text)
	}
}

func (a *asrCallbackAdapter) OnSentenceEnd(text string) {
	if a.callback != nil {
		a.callback.OnSentenceEnd(text)
	}
}

func (a *asrCallbackAdapter) OnError(err error) {
	if a.callback != nil {
		a.callback.OnError(err)
	}
}

// NewTencentInterceptor 创建腾讯拦截器
func NewTencentInterceptor() *TencentInterceptor {
	converterService := formatconverter.NewFormatConverterService()
	return &TencentInterceptor{
		provider: formatconverter.ProviderTencent,
		converter: converterService.GetConverter(formatconverter.ProviderTencent),
		logger: log.New(log.Writer(), "[TencentInterceptor] ", log.LstdFlags),
	}
}

// Connect 连接到腾讯语音服务
func (i *TencentInterceptor) Connect() error {
	// 创建腾讯ASR服务
	asrService, err := asr.NewTencentRecognitionService()
	if err != nil {
		i.logger.Printf("创建腾讯ASR服务失败: %v", err)
		return err
	}
	
	i.asrService = asrService
	
	// 创建回调适配器
	adapter := &asrCallbackAdapter{callback: i.callback}
	
	// 创建流式识别器
	recognizer, err := asrService.NewStreamRecognizer(adapter)
	if err != nil {
		i.logger.Printf("创建流式识别器失败: %v", err)
		return err
	}
	
	i.recognizer = recognizer
	i.connected = true
	i.logger.Printf("成功连接到腾讯语音服务")
	return nil
}

// Write 写入音频数据（会自动进行格式转换）
func (i *TencentInterceptor) Write(data []byte) error {
	if !i.connected || i.recognizer == nil {
		return nil // TODO: 返回连接错误
	}
	
	// 使用转换器转换音频数据格式
	convertedData, err := i.converter.ConvertAudioData(data)
	if err != nil {
		if i.callback != nil {
			i.callback.OnError(err)
		}
		return err
	}
	
	// 将转换后的数据写入腾讯ASR服务
	if err := i.recognizer.Write(convertedData); err != nil {
		if i.callback != nil {
			i.callback.OnError(err)
		}
		return err
	}
	
	return nil
}

// Stop 停止识别
func (i *TencentInterceptor) Stop() error {
	if i.recognizer != nil {
		i.recognizer.Stop()
		i.recognizer = nil
	}
	i.connected = false
	i.logger.Printf("已停止腾讯语音识别")
	return nil
}

// GetProvider 获取提供商
func (i *TencentInterceptor) GetProvider() formatconverter.SpeechProvider {
	return i.provider
}

// IsConnected 检查是否已连接
func (i *TencentInterceptor) IsConnected() bool {
	return i.connected
}

// SetCallback 设置识别结果回调
func (i *TencentInterceptor) SetCallback(callback RecognitionResultCallback) {
	i.callback = callback
	// 如果识别器已创建，更新其回调
	if i.recognizer != nil {
		adapter := &asrCallbackAdapter{callback: callback}
		i.recognizer.SetCallback(adapter)
	}
}