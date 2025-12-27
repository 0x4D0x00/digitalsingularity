package database

import (
	"fmt"
	"log"
	"time"

	"digitalsingularity/backend/common/utils/datahandle"
)

// SpeechSystemDataService 处理SpeechSystem应用相关的数据服务
type SpeechSystemDataService struct {
	readWrite *datahandle.CommonReadWriteService
	dbName    string
}

// NewSpeechSystemDataService 创建一个新的SpeechSystemDataService实例
func NewSpeechSystemDataService(readWrite *datahandle.CommonReadWriteService) *SpeechSystemDataService {
	service := &SpeechSystemDataService{
		readWrite: readWrite,
		dbName:    "speech_system", // 默认值
	}

	// 配置从配置文件中读取数据库名的逻辑已移除
	// 使用默认值 "speech_system"
	log.Printf("[SpeechSystem] 使用默认数据库名: %s", service.dbName)

	return service
}

// GetDatabaseName 返回当前使用的数据库名
func (s *SpeechSystemDataService) GetDatabaseName() string {
	return s.dbName
}

// TencentSpeechASRConfig 腾讯云语音识别(ASR)配置结构
type TencentSpeechASRConfig struct {
	AppID                 string `json:"app_id"`
	SecretID              string `json:"secret_id"`
	SecretKey             string `json:"secret_key"`
	DefaultEngineModelType string `json:"default_engine_model_type"`
	SliceSize             int    `json:"slice_size"`
	Status                int    `json:"status"`
}

// TencentSpeechTTSConfig 腾讯云语音合成(TTS)配置结构
type TencentSpeechTTSConfig struct {
	AppID             string `json:"app_id"`
	SecretID          string `json:"secret_id"`
	SecretKey         string `json:"secret_key"`
	MaleVoiceType     int    `json:"male_voice_type"`
	FemaleVoiceType   int    `json:"female_voice_type"`
	DefaultSampleRate int    `json:"default_sample_rate"`
	DefaultCodec      string `json:"default_codec"`
	Status            int    `json:"status"`
}

// GetTencentSpeechASRConfig 获取腾讯云语音识别(ASR)配置
// 先尝试从Redis读取，如果不存在则从数据库读取并写入Redis缓存
// 返回启用的配置，如果没有启用的配置，返回第一个配置
func (s *SpeechSystemDataService) GetTencentSpeechASRConfig() (*TencentSpeechASRConfig, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("获取腾讯云ASR配置异常: %v", r)
		}
	}()

	// 先尝试从Redis读取配置
	redisKey := "speech:tencent:asr:config"
	redisResult := s.readWrite.RedisRead(redisKey)
	if redisResult.IsSuccess() {
		// 尝试将Redis中的数据解析为TencentSpeechASRConfig
		if configData, ok := redisResult.Data.(map[string]interface{}); ok {
			asrConfig := &TencentSpeechASRConfig{
				AppID:                 getStringValue(configData, "app_id"),
				SecretID:              getStringValue(configData, "secret_id"),
				SecretKey:             getStringValue(configData, "secret_key"),
				DefaultEngineModelType: getStringValue(configData, "default_engine_model_type"),
			}
			
			// 处理数字类型字段
			if sliceSize, ok := configData["slice_size"].(float64); ok {
				asrConfig.SliceSize = int(sliceSize)
			} else if sliceSize, ok := configData["slice_size"].(int); ok {
				asrConfig.SliceSize = sliceSize
			} else {
				asrConfig.SliceSize = 6400
			}
			
			if status, ok := configData["status"].(float64); ok {
				asrConfig.Status = int(status)
			} else if status, ok := configData["status"].(int); ok {
				asrConfig.Status = status
			}
			
			log.Printf("[SpeechSystem] 从Redis读取腾讯ASR配置成功")
			return asrConfig, nil
		} else {
			// Redis中的数据格式不正确，将从数据库重新加载
			log.Printf("[SpeechSystem] Redis中的数据格式不正确（类型: %T），将从数据库重新加载", redisResult.Data)
		}
	} else {
		log.Printf("[SpeechSystem] 从Redis读取ASR配置失败: %v，将从数据库读取", redisResult.Error)
	}

	// 从数据库读取配置
	asrConfig, err := s.getTencentSpeechASRConfigFromDB()
	if err != nil {
		return nil, err
	}

	// 将配置写入Redis缓存，设置24小时过期时间
	if asrConfig != nil {
		// 将配置转换为map以便存储到Redis
		configMap := map[string]interface{}{
			"app_id":                  asrConfig.AppID,
			"secret_id":               asrConfig.SecretID,
			"secret_key":              asrConfig.SecretKey,
			"default_engine_model_type": asrConfig.DefaultEngineModelType,
			"slice_size":              asrConfig.SliceSize,
			"status":                  asrConfig.Status,
		}
		
		writeResult := s.readWrite.RedisWrite(redisKey, configMap, 24*time.Hour)
		if writeResult.IsSuccess() {
			log.Printf("[SpeechSystem] 成功将腾讯ASR配置写入Redis缓存")
		} else {
			log.Printf("[SpeechSystem] 写入Redis缓存失败: %v", writeResult.Error)
		}
	}

	return asrConfig, nil
}

// getTencentSpeechASRConfigFromDB 从数据库读取腾讯云ASR配置
func (s *SpeechSystemDataService) getTencentSpeechASRConfigFromDB() (*TencentSpeechASRConfig, error) {
	// 先尝试获取启用的配置
	query := fmt.Sprintf(`
		SELECT app_id, secret_id, secret_key, 
		       default_engine_model_type, slice_size, status
		FROM %s.tencent_speech_asr 
		WHERE status = 1
		ORDER BY updated_at DESC
		LIMIT 1
	`, s.dbName)
	
	opResult := s.readWrite.QueryDb(query)
	if !opResult.IsSuccess() {
		log.Printf("获取腾讯云ASR配置错误: %v", opResult.Error)
		// 如果查询失败，尝试获取任意一个配置
		query = fmt.Sprintf(`
			SELECT app_id, secret_id, secret_key, 
			       default_engine_model_type, slice_size, status
			FROM %s.tencent_speech_asr 
			ORDER BY updated_at DESC
			LIMIT 1
		`, s.dbName)
		opResult = s.readWrite.QueryDb(query)
		if !opResult.IsSuccess() {
			return nil, fmt.Errorf("无法获取腾讯云ASR配置: %v", opResult.Error)
		}
	}
	
	result, ok := opResult.Data.([]map[string]interface{})
	if !ok || len(result) == 0 {
		return nil, fmt.Errorf("未找到腾讯云ASR配置")
	}
	
	config := result[0]
	
	asrConfig := &TencentSpeechASRConfig{
		AppID:                 getStringValue(config, "app_id"),
		SecretID:              getStringValue(config, "secret_id"),
		SecretKey:             getStringValue(config, "secret_key"),
		DefaultEngineModelType: getStringValue(config, "default_engine_model_type"),
	}
	
	// 处理切片大小
	if sliceSize, ok := config["slice_size"].(int64); ok {
		asrConfig.SliceSize = int(sliceSize)
	} else if sliceSize, ok := config["slice_size"].(int); ok {
		asrConfig.SliceSize = sliceSize
	} else {
		// 默认值
		asrConfig.SliceSize = 6400
	}
	
	// 处理状态
	if status, ok := config["status"].(int64); ok {
		asrConfig.Status = int(status)
	} else if status, ok := config["status"].(int); ok {
		asrConfig.Status = status
	}
	
	log.Printf("[SpeechSystem] 从数据库读取腾讯ASR配置成功")
	return asrConfig, nil
}

// GetTencentSpeechTTSConfig 获取腾讯云语音合成(TTS)配置
// 先尝试从Redis读取，如果不存在则从数据库读取并写入Redis缓存
// 返回启用的配置，如果没有启用的配置，返回第一个配置
func (s *SpeechSystemDataService) GetTencentSpeechTTSConfig() (*TencentSpeechTTSConfig, error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("获取腾讯云TTS配置异常: %v", r)
		}
	}()

	// 先尝试从Redis读取配置
	redisKey := "speech:tencent:tts:config"
	redisResult := s.readWrite.RedisRead(redisKey)
	if redisResult.IsSuccess() {
		// 尝试将Redis中的数据解析为TencentSpeechTTSConfig
		if configData, ok := redisResult.Data.(map[string]interface{}); ok {
			ttsConfig := &TencentSpeechTTSConfig{
				AppID:        getStringValue(configData, "app_id"),
				SecretID:     getStringValue(configData, "secret_id"),
				SecretKey:    getStringValue(configData, "secret_key"),
				DefaultCodec: getStringValue(configData, "default_codec"),
			}
			
			// 处理数字类型字段
			if maleVoiceType, ok := configData["male_voice_type"].(float64); ok {
				ttsConfig.MaleVoiceType = int(maleVoiceType)
			} else if maleVoiceType, ok := configData["male_voice_type"].(int); ok {
				ttsConfig.MaleVoiceType = maleVoiceType
			} else {
				ttsConfig.MaleVoiceType = 301034
			}
			
			if femaleVoiceType, ok := configData["female_voice_type"].(float64); ok {
				ttsConfig.FemaleVoiceType = int(femaleVoiceType)
			} else if femaleVoiceType, ok := configData["female_voice_type"].(int); ok {
				ttsConfig.FemaleVoiceType = femaleVoiceType
			} else {
				ttsConfig.FemaleVoiceType = 601000
			}
			
			if sampleRate, ok := configData["default_sample_rate"].(float64); ok {
				ttsConfig.DefaultSampleRate = int(sampleRate)
			} else if sampleRate, ok := configData["default_sample_rate"].(int); ok {
				ttsConfig.DefaultSampleRate = sampleRate
			}
			
			if status, ok := configData["status"].(float64); ok {
				ttsConfig.Status = int(status)
			} else if status, ok := configData["status"].(int); ok {
				ttsConfig.Status = status
			}
			
			log.Printf("[SpeechSystem] 从Redis读取腾讯TTS配置成功")
			return ttsConfig, nil
		} else {
			// Redis中的数据格式不正确，将从数据库重新加载
			log.Printf("[SpeechSystem] Redis中的数据格式不正确（类型: %T），将从数据库重新加载", redisResult.Data)
		}
	} else {
		log.Printf("[SpeechSystem] 从Redis读取TTS配置失败: %v，将从数据库读取", redisResult.Error)
	}

	// 从数据库读取配置
	ttsConfig, err := s.getTencentSpeechTTSConfigFromDB()
	if err != nil {
		return nil, err
	}

	// 将配置写入Redis缓存，设置24小时过期时间
	if ttsConfig != nil {
		// 将配置转换为map以便存储到Redis
		configMap := map[string]interface{}{
			"app_id":              ttsConfig.AppID,
			"secret_id":           ttsConfig.SecretID,
			"secret_key":          ttsConfig.SecretKey,
			"male_voice_type":     ttsConfig.MaleVoiceType,
			"female_voice_type":   ttsConfig.FemaleVoiceType,
			"default_sample_rate": ttsConfig.DefaultSampleRate,
			"default_codec":       ttsConfig.DefaultCodec,
			"status":              ttsConfig.Status,
		}
		
		writeResult := s.readWrite.RedisWrite(redisKey, configMap, 24*time.Hour)
		if writeResult.IsSuccess() {
			log.Printf("[SpeechSystem] 成功将腾讯TTS配置写入Redis缓存")
		} else {
			log.Printf("[SpeechSystem] 写入Redis缓存失败: %v", writeResult.Error)
		}
	}

	return ttsConfig, nil
}

// getTencentSpeechTTSConfigFromDB 从数据库读取腾讯云TTS配置
func (s *SpeechSystemDataService) getTencentSpeechTTSConfigFromDB() (*TencentSpeechTTSConfig, error) {
	// 先尝试获取启用的配置
	query := fmt.Sprintf(`
		SELECT app_id, secret_id, secret_key, 
		       male_voice_type, female_voice_type,
		       default_sample_rate, default_codec, status
		FROM %s.tencent_speech_tts 
		WHERE status = 1
		ORDER BY updated_at DESC
		LIMIT 1
	`, s.dbName)
	
	opResult := s.readWrite.QueryDb(query)
	if !opResult.IsSuccess() {
		log.Printf("获取腾讯云TTS配置错误: %v", opResult.Error)
		// 如果查询失败，尝试获取任意一个配置
		query = fmt.Sprintf(`
			SELECT app_id, secret_id, secret_key, 
			       male_voice_type, female_voice_type,
			       default_sample_rate, default_codec, status
			FROM %s.tencent_speech_tts 
			ORDER BY updated_at DESC
			LIMIT 1
		`, s.dbName)
		opResult = s.readWrite.QueryDb(query)
		if !opResult.IsSuccess() {
			return nil, fmt.Errorf("无法获取腾讯云TTS配置: %v", opResult.Error)
		}
	}
	
	result, ok := opResult.Data.([]map[string]interface{})
	if !ok || len(result) == 0 {
		return nil, fmt.Errorf("未找到腾讯云TTS配置")
	}
	
	config := result[0]
	
	ttsConfig := &TencentSpeechTTSConfig{
		AppID:        getStringValue(config, "app_id"),
		SecretID:     getStringValue(config, "secret_id"),
		SecretKey:    getStringValue(config, "secret_key"),
		DefaultCodec: getStringValue(config, "default_codec"),
	}
	
	// 处理男声音色类型
	if maleVoiceType, ok := config["male_voice_type"].(int64); ok {
		ttsConfig.MaleVoiceType = int(maleVoiceType)
	} else if maleVoiceType, ok := config["male_voice_type"].(int); ok {
		ttsConfig.MaleVoiceType = maleVoiceType
	} else {
		// 默认值
		ttsConfig.MaleVoiceType = 301034
	}
	
	// 处理女声音色类型
	if femaleVoiceType, ok := config["female_voice_type"].(int64); ok {
		ttsConfig.FemaleVoiceType = int(femaleVoiceType)
	} else if femaleVoiceType, ok := config["female_voice_type"].(int); ok {
		ttsConfig.FemaleVoiceType = femaleVoiceType
	} else {
		// 默认值
		ttsConfig.FemaleVoiceType = 601000
	}
	
	if sampleRate, ok := config["default_sample_rate"].(int64); ok {
		ttsConfig.DefaultSampleRate = int(sampleRate)
	} else if sampleRate, ok := config["default_sample_rate"].(int); ok {
		ttsConfig.DefaultSampleRate = sampleRate
	}
	
	if status, ok := config["status"].(int64); ok {
		ttsConfig.Status = int(status)
	} else if status, ok := config["status"].(int); ok {
		ttsConfig.Status = status
	}
	
	log.Printf("[SpeechSystem] 从数据库读取腾讯TTS配置成功")
	return ttsConfig, nil
}

// getStringValue 从map中安全获取字符串值
func getStringValue(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
		// 尝试转换为字符串
		return fmt.Sprintf("%v", val)
	}
	return ""
} 