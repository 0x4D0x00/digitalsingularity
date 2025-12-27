package tts

import (
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"digitalsingularity/backend/common/utils/datahandle"
	"digitalsingularity/backend/speechsystem/database"
	"github.com/tencentcloud/tencentcloud-speech-sdk-go/common"
	"github.com/tencentcloud/tencentcloud-speech-sdk-go/tts"
)

// 默认配置
const (
	DefaultCodec      = "pcm"  // 默认音频格式：pcm/mp3
	DefaultSampleRate = 16000  // 默认音频采样率：8000/16000
	DefaultVoiceType  = 601000 // 默认女声音色
)

var logger = log.New(log.Writer(), "[TencentTTS] ", log.LstdFlags)

// TencentSynthesisListener 腾讯语音合成监听器，处理合成回调
type TencentSynthesisListener struct {
	RequestID          string
	Codec              string
	SampleRate         int
	AudioData          []byte
	Subtitles          []interface{}
	SynthesisComplete  bool
	mu                 sync.Mutex
	AudioQueue         chan []byte
}

// OnMessage 音频数据回调
func (l *TencentSynthesisListener) OnMessage(response *tts.SpeechSynthesisResponse) {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	// 累积完整的音频数据
	l.AudioData = append(l.AudioData, response.Data...)
	
	// 将音频块添加到队列中，用于流式处理
	select {
	case l.AudioQueue <- response.Data:
	default:
		// 队列已满，跳过
	}
	
	logger.Printf("[%s] 收到音频数据，长度: %d字节", l.RequestID, len(response.Data))
}

// OnComplete 合成完成回调
func (l *TencentSynthesisListener) OnComplete(response *tts.SpeechSynthesisResponse) {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	logger.Printf("[%s] 语音合成完成，音频大小: %d字节", l.RequestID, len(l.AudioData))
	l.SynthesisComplete = true
	
	// 标记队列结束
	close(l.AudioQueue)
}

// OnCancel 合成取消回调
func (l *TencentSynthesisListener) OnCancel(response *tts.SpeechSynthesisResponse) {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	logger.Printf("[%s] 语音合成取消", l.RequestID)
	l.SynthesisComplete = true
	
	// 标记队列结束
	close(l.AudioQueue)
}

// OnFail 合成失败回调
func (l *TencentSynthesisListener) OnFail(response *tts.SpeechSynthesisResponse, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	if err != nil {
		logger.Printf("[%s] 语音合成失败: %v", l.RequestID, err)
	} else {
		logger.Printf("[%s] 语音合成失败", l.RequestID)
	}
	
	l.SynthesisComplete = true
	
	// 标记队列结束
	close(l.AudioQueue)
}

// TencentSynthesisService 腾讯语音合成服务，提供简单的接口用于文本到语音转换
type TencentSynthesisService struct {
	appID        string
	secretID     string
	secretKey    string
	voiceType    int
	codec        string
	sampleRate   int
	dataService  *database.SpeechSystemDataService
}

// NewTencentSynthesisService 创建新的腾讯语音合成服务实例
// 从Redis获取配置（如果Redis中没有，则从数据库读取并缓存到Redis）
func NewTencentSynthesisService() (*TencentSynthesisService, error) {
	// 创建数据服务，使用 speech_system 数据库配置
	readWrite, err := datahandle.NewCommonReadWriteService("speech_system")
	if err != nil {
		return nil, fmt.Errorf("创建数据服务失败: %v", err)
	}
	
	dataService := database.NewSpeechSystemDataService(readWrite)
	
	// 从Redis获取TTS配置
	config, err := dataService.GetTencentSpeechTTSConfig()
	if err != nil {
		return nil, fmt.Errorf("获取腾讯TTS配置失败: %v", err)
	}
	
	// 解析AppID
	appID := config.AppID
	if appID == "" {
		return nil, fmt.Errorf("AppID不能为空")
	}
	
	// 使用配置中的参数
	voiceType := config.FemaleVoiceType
	if voiceType == 0 {
		voiceType = DefaultVoiceType
	}
	
	codec := config.DefaultCodec
	if codec == "" {
		codec = DefaultCodec
	}
	
	sampleRate := config.DefaultSampleRate
	if sampleRate == 0 {
		sampleRate = DefaultSampleRate
	}
	
	service := &TencentSynthesisService{
		appID:       appID,
		secretID:    config.SecretID,
		secretKey:   config.SecretKey,
		voiceType:   voiceType,
		codec:       codec,
		sampleRate:  sampleRate,
		dataService: dataService,
	}
	
	logger.Printf("腾讯语音合成服务初始化完成，AppID: %s, VoiceType: %d, Codec: %s, SampleRate: %d", 
		appID, voiceType, codec, sampleRate)
	return service, nil
}

// Synthesize 将文本转换为语音
// 参数:
//   text: 要合成的文本
// 返回:
//   合成的音频数据和错误
func (s *TencentSynthesisService) Synthesize(text string) ([]byte, error) {
	requestID := fmt.Sprintf("tts-%d", time.Now().Unix())
	logger.Printf("[%s] 开始语音合成，文本: %s", requestID, text)
	
	// 创建监听器
	audioQueue := make(chan []byte, 100)
	listener := &TencentSynthesisListener{
		RequestID:         requestID,
		Codec:             s.codec,
		SampleRate:        s.sampleRate,
		AudioData:         []byte{},
		Subtitles:         []interface{}{},
		SynthesisComplete: false,
		AudioQueue:        audioQueue,
	}
	
	// 创建认证对象
	credential := common.NewCredential(s.secretID, s.secretKey)
	
	// 解析AppID为int64
	appIDInt, err := strconv.ParseInt(s.appID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("AppID格式错误: %v", err)
	}
	
	// 创建合成器
	synthesizer := tts.NewSpeechSynthesizer(appIDInt, credential, listener)
	synthesizer.VoiceType = int64(s.voiceType)
	synthesizer.SampleRate = int64(s.sampleRate)
	synthesizer.Codec = s.codec
	
	// 启动合成
	synthesizer.Synthesis(text)
	
	// 等待合成完成
	synthesizer.Wait()
	
	// 等待监听器完成
	maxWait := 30 * time.Second // 最多等待30秒
	waitInterval := 100 * time.Millisecond
	waited := 0 * time.Millisecond
	
	for !listener.SynthesisComplete && waited < maxWait {
		time.Sleep(waitInterval)
		waited += waitInterval
	}
	
	if !listener.SynthesisComplete {
		logger.Printf("[%s] 等待合成完成超时", requestID)
		return nil, fmt.Errorf("等待合成完成超时")
	}
	
	logger.Printf("[%s] 语音合成完成，音频长度: %d字节", requestID, len(listener.AudioData))
	
	return listener.AudioData, nil
}

// SynthesizeStream 流式合成文本为语音
// 参数:
//   text: 要合成的文本
// 返回:
//   音频数据块的通道
func (s *TencentSynthesisService) SynthesizeStream(text string) (<-chan []byte, error) {
	requestID := fmt.Sprintf("tts-stream-%d", time.Now().Unix())
	logger.Printf("[%s] 开始流式语音合成，文本: %s", requestID, text)
	
	// 创建监听器
	audioQueue := make(chan []byte, 100)
	listener := &TencentSynthesisListener{
		RequestID:         requestID,
		Codec:             s.codec,
		SampleRate:        s.sampleRate,
		AudioData:         []byte{},
		Subtitles:         []interface{}{},
		SynthesisComplete: false,
		AudioQueue:        audioQueue,
	}
	
	// 创建认证对象
	credential := common.NewCredential(s.secretID, s.secretKey)
	
	// 解析AppID为int64
	appIDInt, err := strconv.ParseInt(s.appID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("AppID格式错误: %v", err)
	}
	
	// 创建合成器
	synthesizer := tts.NewSpeechSynthesizer(appIDInt, credential, listener)
	synthesizer.VoiceType = int64(s.voiceType)
	synthesizer.SampleRate = int64(s.sampleRate)
	synthesizer.Codec = s.codec
	
	// 在单独的goroutine中启动合成
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Printf("[%s] 流式合成goroutine发生panic: %v", requestID, r)
				close(audioQueue)
			}
		}()
		
		synthesizer.Synthesis(text)
		synthesizer.Wait()
	}()
	
	logger.Printf("[%s] 流式语音合成已启动", requestID)
	
	return audioQueue, nil
}

// GetAppID 获取AppID（用于测试）
func (s *TencentSynthesisService) GetAppID() string {
	return s.appID
}

// GetVoiceType 获取音色类型（用于测试）
func (s *TencentSynthesisService) GetVoiceType() int {
	return s.voiceType
}

// GetCodec 获取编码格式（用于测试）
func (s *TencentSynthesisService) GetCodec() string {
	return s.codec
}

// GetSampleRate 获取采样率（用于测试）
func (s *TencentSynthesisService) GetSampleRate() int {
	return s.sampleRate
}