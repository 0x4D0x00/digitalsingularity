package decrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"

	"digitalsingularity/backend/common/security/symmetricencryption/keyserializer"
)

// SymmetricDecryptService 对称解密数据 - 与 Python 实现兼容
//
// 参数:
//   encryptedData: 要解密的base64编码数据
//   key: 通常不使用，保留参数是为了兼容性
//   iv: 通常不使用，保留参数是为了兼容性
//
// 返回值:
//   解密后的原始数据和可能的错误
func SymmetricDecryptService(encryptedData string, key, iv string) (string, error) {
	if encryptedData == "" {
		return "", errors.New("解密数据不能为空")
	}

	// 始终使用固定的密钥和IV，与Python实现保持一致
	actualKey, actualIv := keyserializer.GetKeyAndIv(key, iv)

	// 解码Base64
	ciphertext, err := base64.StdEncoding.DecodeString(encryptedData)
	if err != nil {
		return "", err
	}

	// 检查密文长度
	if len(ciphertext) < aes.BlockSize || len(ciphertext)%aes.BlockSize != 0 {
		return "", errors.New("密文长度无效")
	}

	// 创建解密器
	block, err := aes.NewCipher(actualKey)
	if err != nil {
		return "", err
	}

	// 进行解密
	plaintext := make([]byte, len(ciphertext))
	decrypter := cipher.NewCBCDecrypter(block, actualIv)
	decrypter.CryptBlocks(plaintext, ciphertext)

	// 去除填充
	unpaddedData, err := pkcs7Unpad(plaintext, block.BlockSize())
	if err != nil {
		return "", err
	}

	return string(unpaddedData), nil
}

// pkcs7Unpad 解除PKCS7填充
func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, errors.New("无效的填充数据")
	}

	// 获取填充长度
	padding := int(data[len(data)-1])
	if padding > blockSize || padding == 0 {
		return nil, errors.New("无效的填充值")
	}

	// 验证填充
	for i := len(data) - padding; i < len(data); i++ {
		if data[i] != byte(padding) {
			return nil, errors.New("填充格式不正确")
		}
	}

	// 去除填充
	return data[:len(data)-padding], nil
} 