package asr

import (
	"fmt"
	"log"
	"sync"

	"digitalsingularity/backend/common/utils/datahandle"
	"digitalsingularity/backend/speechsystem/database"
	"github.com/tencentcloud/tencentcloud-speech-sdk-go/asr"
	"github.com/tencentcloud/tencentcloud-speech-sdk-go/common"
)

// StreamRecognitionCallback 流式识别结果回调接口
type StreamRecognitionCallback interface {
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

var logger = log.New(log.Writer(), "[TencentASR] ", log.LstdFlags)

// TencentRecognitionService 腾讯语音识别服务，提供流式语音识别功能
type TencentRecognitionService struct {
	appID        string
	secretID     string
	secretKey    string
	engineModel  string
	dataService  *database.SpeechSystemDataService
}

// NewTencentRecognitionService 创建新的腾讯语音识别服务实例
// 从Redis获取配置（如果Redis中没有，则从数据库读取并缓存到Redis）
func NewTencentRecognitionService() (*TencentRecognitionService, error) {
	// 创建数据服务，使用 speech_system 数据库配置
	readWrite, err := datahandle.NewCommonReadWriteService("speech_system")
	if err != nil {
		return nil, fmt.Errorf("创建数据服务失败: %v", err)
	}
	
	dataService := database.NewSpeechSystemDataService(readWrite)
	
	// 从Redis获取ASR配置
	config, err := dataService.GetTencentSpeechASRConfig()
	if err != nil {
		return nil, fmt.Errorf("获取腾讯ASR配置失败: %v", err)
	}
	
	// 从数据库配置中解析AppID
	appID := config.AppID
	if appID == "" {
		return nil, fmt.Errorf("AppID不能为空")
	}
	
	// 从数据库配置中获取引擎模型类型
	engineModel := config.DefaultEngineModelType
	if engineModel == "" {
		return nil, fmt.Errorf("DefaultEngineModelType不能为空")
	}
	
	service := &TencentRecognitionService{
		appID:       appID,
		secretID:    config.SecretID,
		secretKey:   config.SecretKey,
		engineModel: engineModel,
		dataService: dataService,
	}
	
	logger.Printf("腾讯语音识别服务初始化完成，AppID: %s, EngineModel: %s", appID, engineModel)
	return service, nil
}

// TencentStreamRecognitionListener 腾讯流式识别监听器
type TencentStreamRecognitionListener struct {
	callback StreamRecognitionCallback
	logger   *log.Logger
}

// OnRecognitionStart 识别开始回调
func (l *TencentStreamRecognitionListener) OnRecognitionStart(response *asr.SpeechRecognitionResponse) {
	l.logger.Printf("识别开始，语音ID: %s", response.VoiceID)
}

// OnSentenceBegin 句子开始回调
func (l *TencentStreamRecognitionListener) OnSentenceBegin(response *asr.SpeechRecognitionResponse) {
	l.logger.Printf("句子开始")
	if l.callback != nil {
		l.callback.OnSentenceBegin()
	}
}

// OnRecognitionResultChange 识别结果变化回调（部分结果）
func (l *TencentStreamRecognitionListener) OnRecognitionResultChange(response *asr.SpeechRecognitionResponse) {
	if l.callback != nil && response.Result.VoiceTextStr != "" {
		// 部分识别结果
		l.callback.OnPartialResult(response.Result.VoiceTextStr)
	}
}

// OnSentenceEnd 句子结束回调（最终结果）
func (l *TencentStreamRecognitionListener) OnSentenceEnd(response *asr.SpeechRecognitionResponse) {
	l.logger.Printf("句子结束")
	if l.callback != nil && response.Result.VoiceTextStr != "" {
		// 最终识别结果
		l.callback.OnSentenceEnd(response.Result.VoiceTextStr)
	}
}

// OnRecognitionComplete 识别完成回调
func (l *TencentStreamRecognitionListener) OnRecognitionComplete(response *asr.SpeechRecognitionResponse) {
	l.logger.Printf("识别完成，语音ID: %s", response.VoiceID)
}

// OnFail 识别失败回调
func (l *TencentStreamRecognitionListener) OnFail(response *asr.SpeechRecognitionResponse, err error) {
	l.logger.Printf("识别失败: %v", err)
	if l.callback != nil {
		l.callback.OnError(err)
	}
}

// StreamRecognizer 流式识别器
type StreamRecognizer struct {
	recognizer *asr.SpeechRecognizer
	listener   *TencentStreamRecognitionListener
	callback   StreamRecognitionCallback
	mu         sync.RWMutex
	started    bool
	logger     *log.Logger
}

// NewStreamRecognizer 创建流式识别器
func (s *TencentRecognitionService) NewStreamRecognizer(callback StreamRecognitionCallback) (*StreamRecognizer, error) {
	// 创建监听器
	listener := &TencentStreamRecognitionListener{
		callback: callback,
		logger:   logger,
	}

	// 创建认证对象
	credential := common.NewCredential(s.secretID, s.secretKey)

	// 创建识别器
	recognizer := asr.NewSpeechRecognizer(s.appID, credential, s.engineModel, listener)

	// 设置识别参数
	recognizer.FilterModal = 1    // 过滤语气词
	recognizer.FilterPunc = 1     // 过滤标点
	recognizer.FilterDirty = 1    // 过滤脏话
	recognizer.NeedVad = 1        // 启用VAD
	recognizer.VoiceFormat = asr.AudioFormatPCM // PCM格式
	recognizer.WordInfo = 1       // 获取词级别时间戳
	recognizer.ConvertNumMode = 1 // 阿拉伯数字转换

	return &StreamRecognizer{
		recognizer: recognizer,
		listener:   listener,
		callback:   callback,
		logger:     logger,
		started:    false,
	}, nil
}

// Start 启动识别器
func (sr *StreamRecognizer) Start() error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	if sr.started {
		return nil
	}

	err := sr.recognizer.Start()
	if err != nil {
		return fmt.Errorf("启动识别器失败: %v", err)
	}

	sr.started = true
	sr.logger.Printf("流式识别器已启动")
	return nil
}

// Write 写入音频数据
func (sr *StreamRecognizer) Write(data []byte) error {
	sr.mu.RLock()
	needsStart := !sr.started
	sr.mu.RUnlock()

	if needsStart {
		// 首次写入时自动启动
		if err := sr.Start(); err != nil {
			return err
		}
	}

	sr.mu.RLock()
	recognizer := sr.recognizer
	sr.mu.RUnlock()

	if err := recognizer.Write(data); err != nil {
		return fmt.Errorf("写入音频数据失败: %v", err)
	}

	return nil
}

// Stop 停止识别器
func (sr *StreamRecognizer) Stop() error {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	if !sr.started {
		return nil
	}

	sr.recognizer.Stop()
	sr.started = false
	sr.logger.Printf("流式识别器已停止")
	return nil
}

// SetCallback 设置识别结果回调
func (sr *StreamRecognizer) SetCallback(callback StreamRecognitionCallback) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	sr.callback = callback
	if sr.listener != nil {
		sr.listener.callback = callback
	}
}