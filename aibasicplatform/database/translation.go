package database

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"digitalsingularity/backend/common/utils/datahandle"
)

// TranslationRecord 通用翻译记录
// 用于从 aibasicplatform.aibasicplatform_translation 表读取多语言配置
type TranslationRecord struct {
	App           string `json:"app"`
	Category      string `json:"category"`
	ItemKey       string `json:"item_key"`
	Locale        string `json:"locale"`
	TranslatedTxt string `json:"translated_text"`
}

// ensureAIBasicPlatformServiceWithRW 确保有可用的 AIBasicPlatformDataService（如果当前实例没有 readWrite，则创建一个新的）
func ensureAIBasicPlatformServiceWithRW(s *AIBasicPlatformDataService) *AIBasicPlatformDataService {
	if s != nil && s.readWrite != nil {
		return s
	}

	rw, err := datahandle.NewCommonReadWriteService("database")
	if err != nil {
		log.Printf("[Translation] 创建数据服务失败: %v", err)
		return nil
	}

	return &AIBasicPlatformDataService{
		readWrite: rw,
		dbName:    "aibasicplatform",
	}
}

// getDefaultTranslationService 返回一个可用的翻译服务实例，禁止在其他文件中直接操作数据库
func getDefaultTranslationService() (*AIBasicPlatformDataService, error) {
	service := ensureAIBasicPlatformServiceWithRW(nil)
	if service == nil || service.readWrite == nil {
		return nil, fmt.Errorf("翻译服务未初始化")
	}
	return service, nil
}

// FetchTranslationText 对外暴露的便捷方法，供 HTTP 层调用，不直接操作数据库
func FetchTranslationText(app, category, itemKey, locale string) (string, error) {
	service, err := getDefaultTranslationService()
	if err != nil {
		return "", err
	}
	return service.GetTranslation(app, category, itemKey, locale)
}

// FetchTranslationsDict 获取指定 app/category/locale 下的翻译字典
// 优先从 Redis 中读取，若不存在则回源数据库
func FetchTranslationsDict(app, category, locale string) (map[string]string, error) {
	service, err := getDefaultTranslationService()
	if err != nil {
		return nil, err
	}

	translations, err := service.GetTranslationsFromRedis(app, category, locale)
	if err != nil || len(translations) == 0 {
		return service.GetTranslations(app, category, locale)
	}

	return translations, nil
}

// GetTranslation 获取单条翻译
// app: 应用模块，例如 "storagebox"
// category: 翻译类别，例如 "role_name", "role_type"
// itemKey: 翻译键，例如 "storagebox_product_expert"
// locale: 语言区域，例如 "zh-CN"、"en-US"
func (s *AIBasicPlatformDataService) GetTranslation(app, category, itemKey, locale string) (string, error) {
	service := ensureAIBasicPlatformServiceWithRW(s)
	if service == nil || service.readWrite == nil {
		return "", fmt.Errorf("翻译服务未初始化")
	}

	if app == "" || category == "" || itemKey == "" {
		return "", fmt.Errorf("app, category, itemKey 不能为空")
	}

	if locale == "" {
		locale = "zh-CN"
	}

	query := `
		SELECT translated_text
		FROM aibasicplatform.aibasicplatform_translation
		WHERE app = ? AND category = ? AND item_key = ? AND locale = ? AND enabled = 1
		LIMIT 1
	`

	opResult := service.readWrite.QueryDb(query, app, category, itemKey, locale)
	if !opResult.IsSuccess() {
		log.Printf("[Translation] 查询翻译失败 app=%s category=%s item_key=%s locale=%s: %v",
			app, category, itemKey, locale, opResult.Error)
		return "", opResult.Error
	}

	rows, ok := opResult.Data.([]map[string]interface{})
	if !ok || len(rows) == 0 {
		return "", fmt.Errorf("未找到翻译 app=%s category=%s item_key=%s locale=%s",
			app, category, itemKey, locale)
	}

	if text, ok := rows[0]["translated_text"].(string); ok {
		return text, nil
	}

	return "", fmt.Errorf("翻译记录格式错误 app=%s category=%s item_key=%s locale=%s",
		app, category, itemKey, locale)
}

// GetTranslations 获取指定 app + category 在某个 locale 下的所有翻译
// 返回 map[item_key]translated_text
func (s *AIBasicPlatformDataService) GetTranslations(app, category, locale string) (map[string]string, error) {
	service := ensureAIBasicPlatformServiceWithRW(s)
	if service == nil || service.readWrite == nil {
		return nil, fmt.Errorf("翻译服务未初始化")
	}

	if app == "" || category == "" {
		return nil, fmt.Errorf("app, category 不能为空")
	}

	if locale == "" {
		locale = "zh-CN"
	}

	query := `
		SELECT item_key, translated_text
		FROM aibasicplatform.aibasicplatform_translation
		WHERE app = ? AND category = ? AND locale = ? AND enabled = 1
		ORDER BY item_key ASC
	`

	opResult := service.readWrite.QueryDb(query, app, category, locale)
	if !opResult.IsSuccess() {
		log.Printf("[Translation] 查询翻译列表失败 app=%s category=%s locale=%s: %v",
			app, category, locale, opResult.Error)
		return nil, opResult.Error
	}

	rows, ok := opResult.Data.([]map[string]interface{})
	if !ok {
		return map[string]string{}, nil
	}

	result := make(map[string]string, len(rows))
	for _, row := range rows {
		key, _ := row["item_key"].(string)
		text, _ := row["translated_text"].(string)
		if key != "" && text != "" {
			result[key] = text
		}
	}

	return result, nil
}

// LoadTranslationsToRedis 在服务启动时，将指定 app+category+locale 的翻译列表加载到 Redis
// Redis Key 约定为： translation:{app}:{category}:{locale}
// Value 为 JSON 序列化的 map[item_key]translated_text
func (s *AIBasicPlatformDataService) LoadTranslationsToRedis(app, category, locale string) error {
	service := ensureAIBasicPlatformServiceWithRW(s)
	if service == nil || service.readWrite == nil {
		return fmt.Errorf("翻译服务未初始化")
	}

	translations, err := service.GetTranslations(app, category, locale)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("translation:%s:%s:%s", app, category, locale)

	data, err := json.Marshal(translations)
	if err != nil {
		return fmt.Errorf("序列化翻译数据失败: %w", err)
	}

	// 默认不过期（0 表示不过期）
	opResult := service.readWrite.RedisWrite(key, string(data), 0)
	if !opResult.IsSuccess() {
		return fmt.Errorf("写入 Redis 失败: %w", opResult.Error)
	}

	log.Printf("[Translation] 已加载翻译到 Redis key=%s, 条数=%d", key, len(translations))
	return nil
}

// GetTranslationsFromRedis 从 Redis 获取翻译字典，如果不存在则回源数据库并写回 Redis
func (s *AIBasicPlatformDataService) GetTranslationsFromRedis(app, category, locale string) (map[string]string, error) {
	service := ensureAIBasicPlatformServiceWithRW(s)
	if service == nil || service.readWrite == nil {
		return nil, fmt.Errorf("翻译服务未初始化")
	}

	if locale == "" {
		locale = "zh-CN"
	}

	key := fmt.Sprintf("translation:%s:%s:%s", app, category, locale)

	opResult := service.readWrite.GetRedis(key)
	if opResult.IsSuccess() {
		if raw, ok := opResult.Data.(string); ok && raw != "" {
			var translations map[string]string
			if err := json.Unmarshal([]byte(raw), &translations); err == nil {
				return translations, nil
			}
		}
	}

	// Redis 中没有或解析失败，回源数据库并写回 Redis，设置一个合理过期时间（例如 24 小时）
	translations, err := service.GetTranslations(app, category, locale)
	if err != nil {
		return nil, err
	}

	data, err := json.Marshal(translations)
	if err != nil {
		return translations, nil
	}

	writeResult := service.readWrite.RedisWrite(key, string(data), 24*time.Hour)
	if !writeResult.IsSuccess() {
		log.Printf("[Translation] 回源后写入 Redis 失败 key=%s: %v", key, writeResult.Error)
	}

	return translations, nil
}

