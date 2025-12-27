// 语音系统相关功能
// 处理 TTS（文本转语音）相关操作

package interceptor

import (
	"fmt"
	"log"
)

// 获取logger
var ttsLogger = log.New(log.Writer(), "silicoid_interceptor_tts: ", log.LstdFlags)

// ttsCallbackAdapter 适配器：将 silicoid/interceptor 的回调适配为 speechsystem/interceptor 的回调
type ttsCallbackAdapter struct {
	callback SynthesisResultCallback
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

// synthesizeSpeechAsync 异步进行语音合成（拦截器内部方法）
func (s *SilicoIDInterceptor) synthesizeSpeechAsync(text string, voiceGender string, requestID string, callback SynthesisResultCallback) {
	defer func() {
		if r := recover(); r != nil {
			ttsLogger.Printf("[TTS:%s] ❌ [PANIC] 语音合成发生 panic: %v", requestID, r)
			if callback != nil {
				callback.OnError(fmt.Errorf("语音合成发生 panic: %v", r))
			}
		}
	}()
	
	ttsLogger.Printf("[TTS:%s] 开始语音合成，文本长度: %d, 性别: %s", requestID, len(text), voiceGender)
	
	if s.ttsService == nil {
		err := fmt.Errorf("TTS 服务未初始化")
		ttsLogger.Printf("[TTS:%s] ❌ %v", requestID, err)
		if callback != nil {
			callback.OnError(err)
		}
		return
	}
	
	// 将 silicoid/interceptor 的回调适配为 speechsystem/interceptor 的回调
	adapter := &ttsCallbackAdapter{callback: callback}
	if err := s.ttsService.SynthesizeWithCallback(text, voiceGender, adapter); err != nil {
		ttsLogger.Printf("[TTS:%s] ❌ 语音合成失败: %v", requestID, err)
		if callback != nil {
			callback.OnError(err)
		}
		return
	}
	
	ttsLogger.Printf("[TTS:%s] ✅ 语音合成完成", requestID)
}

// SynthesizeSpeech 使用 TTS 服务合成语音
// text: 要合成的文本
// voiceGender: 语音性别（"male" 或 "female"）
// callback: 合成结果回调接口
func (s *SilicoIDInterceptor) SynthesizeSpeech(text string, voiceGender string, callback SynthesisResultCallback) error {
	if s.ttsService == nil {
		return fmt.Errorf("TTS 服务未初始化")
	}
	
	// 将 silicoid/interceptor 的回调适配为 speechsystem/interceptor 的回调
	adapter := &ttsCallbackAdapter{callback: callback}
	return s.ttsService.SynthesizeWithCallback(text, voiceGender, adapter)
}

