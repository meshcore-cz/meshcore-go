package meshpkt

import (
	"crypto/aes"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
)

const (
	cipherKeySize = 16 // AES-128 key length (CIPHER_KEY_SIZE)
	cipherMACSize = 2  // truncated HMAC length (CIPHER_MAC_SIZE)
	cipherKeyFull = 32 // full key buffer size used for HMAC (PUB_KEY_SIZE)
)

// sealMAC encrypts plaintext with AES-128-ECB and computes a 2-byte
// HMAC-SHA256 tag, matching the firmware's Utils::encryptThenMAC.
// The HMAC key is the 32-byte buffer [secret16 ‖ zero16].
func sealMAC(secret16, plaintext []byte) (mac, ciphertext []byte, err error) {
	ciphertext, err = aes128ECBEncrypt(secret16, plaintext)
	if err != nil {
		return
	}
	hmacKey := make([]byte, cipherKeyFull)
	copy(hmacKey, secret16)
	mac = hmacSHA256Truncated(hmacKey, ciphertext, cipherMACSize)
	return
}

// openMAC verifies the 2-byte HMAC-SHA256 tag then decrypts ciphertext.
// Returns ok=false (and no error) when the MAC doesn't match — caller should
// treat this as a wrong key or corrupt packet.
func openMAC(secret16, mac, ciphertext []byte) (plaintext []byte, ok bool, err error) {
	hmacKey := make([]byte, cipherKeyFull)
	copy(hmacKey, secret16)
	want := hmacSHA256Truncated(hmacKey, ciphertext, cipherMACSize)
	if !hmac.Equal(want, mac) {
		return nil, false, nil
	}
	plaintext, err = aes128ECBDecrypt(secret16, ciphertext)
	if err != nil {
		return nil, false, err
	}
	return plaintext, true, nil
}

// aes128ECBEncrypt encrypts src with AES-128-ECB (no IV, zero-padded to block
// size), matching the firmware's Utils::encrypt implementation.
func aes128ECBEncrypt(key, src []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("meshpkt: AES init: %w", err)
	}
	bs := block.BlockSize()
	padded := make([]byte, (len(src)+bs-1)/bs*bs)
	copy(padded, src)
	dst := make([]byte, len(padded))
	for i := 0; i < len(padded); i += bs {
		block.Encrypt(dst[i:i+bs], padded[i:i+bs])
	}
	return dst, nil
}

// aes128ECBDecrypt decrypts src with AES-128-ECB. src must be a multiple of
// the block size. Zero padding added by the encoder is left in place; callers
// should trim trailing null bytes from the plaintext as appropriate.
func aes128ECBDecrypt(key, src []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("meshpkt: AES init: %w", err)
	}
	bs := block.BlockSize()
	if len(src)%bs != 0 {
		return nil, fmt.Errorf("meshpkt: ciphertext length %d is not a multiple of block size %d", len(src), bs)
	}
	dst := make([]byte, len(src))
	for i := 0; i < len(src); i += bs {
		block.Decrypt(dst[i:i+bs], src[i:i+bs])
	}
	return dst, nil
}

// hmacSHA256Truncated returns the first n bytes of HMAC-SHA256(key, data).
func hmacSHA256Truncated(key, data []byte, n int) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)[:n]
}
