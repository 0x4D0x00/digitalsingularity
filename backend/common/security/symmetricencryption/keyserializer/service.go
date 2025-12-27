package keyserializer

import (
	"crypto/sha256"
)

// 与 Python 代码一致的常量
const (
	DefaultEncryptKey = "PotAGI_Phone_Encryption_Key_2024"
	DefaultEncryptIv  = "PotAGI_Fixed_IV_"
)

// GetKeyAndIv 获取加密所需的密钥和IV
//
// 参数:
//   customKey: 可选的自定义密钥，通常不使用
//   customIv: 可选的自定义初始化向量，通常不使用
//
// 返回值:
//   (key, iv) 包含处理后的密钥和IV
func GetKeyAndIv(customKey, customIv string) ([]byte, []byte) {
	// 为了与 Python 代码兼容，我们忽略自定义密钥和IV
	// 始终使用固定的密钥和IV
	encryptKey := DefaultEncryptKey
	encryptIv := DefaultEncryptIv

	// 使用SHA-256生成固定长度的密钥，与Python实现保持一致
	hasher := sha256.New()
	hasher.Write([]byte(encryptKey))
	key := hasher.Sum(nil)

	// 确保IV长度正确，与Python实现保持一致
	iv := []byte(encryptIv)
	blockSize := 16 // AES block size

	if len(iv) < blockSize {
		// 填充IV到正确的长度
		padding := make([]byte, blockSize-len(iv))
		for i := range padding {
			padding[i] = 0
		}
		iv = append(iv, padding...)
	}

	// 截取到正确的长度
	iv = iv[:blockSize]

	return key, iv
} 