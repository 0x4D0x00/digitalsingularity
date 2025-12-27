package encrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"

	"digitalsingularity/backend/common/security/symmetricencryption/keyserializer"
)

// SymmetricEncryptService 对称加密数据 - 与 Python 实现兼容
//
// 参数:
//   data: 要加密的数据
//   key: 通常不使用，保留参数是为了兼容性
//   iv: 通常不使用，保留参数是为了兼容性
//
// 返回值:
//   加密后的base64字符串 和 可能的错误
func SymmetricEncryptService(data string, key, iv string) (string, error) {
	if data == "" {
		return "", errors.New("加密数据不能为空")
	}

	// 始终使用固定的密钥和IV，与Python实现保持一致
	actualKey, actualIv := keyserializer.GetKeyAndIv(key, iv)

	// 创建加密器
	block, err := aes.NewCipher(actualKey)
	if err != nil {
		return "", err
	}

	// 使用PKCS7填充
	plaintext := pkcs7Pad([]byte(data), block.BlockSize())

	// 进行加密
	ciphertext := make([]byte, len(plaintext))
	encrypter := cipher.NewCBCEncrypter(block, actualIv)
	encrypter.CryptBlocks(ciphertext, plaintext)

	// 转换为Base64编码
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// pkcs7Pad 添加PKCS7填充
func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - (len(data) % blockSize)
	padText := make([]byte, padding)
	for i := 0; i < padding; i++ {
		padText[i] = byte(padding)
	}
	return append(data, padText...)
} 