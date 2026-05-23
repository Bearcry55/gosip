package main

import (
	"crypto/aes"    // The actual AES cipher algorithm
	"crypto/cipher" // For GCM mode (adds the safe wrappers around AES)
	"crypto/rand"   // To generate the secure, random unique nonces
	"crypto/sha256" // To stretch your 6-character password into a solid 32-byte key
	"encoding/hex"
	 // To convert raw encrypted bytes into safe text for ntfy.sh
)
func EncryptMessage(message string, password string) string {
	// 1. Stretch the 6-character password to a 32-byte key using SHA-256
	hasher := sha256.New()
	hasher.Write([]byte(password))
	key := hasher.Sum(nil)

	// 2. Set up the AES cipher block and GCM engine
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)

	// 3. Create a unique, random Nonce (number used once) for this message
	nonce := make([]byte, gcm.NonceSize())
	_, _ = rand.Read(nonce) // Using blank identifiers to keep the code short and clean

	// 4. Scramble the message and attach the nonce right to the front of it
	cipherText := gcm.Seal(nonce, nonce, []byte(message), nil)

	// 5. Convert the raw scrambled bytes to readable hex text
	return hex.EncodeToString(cipherText)
}
func DecryptMessage(hexCipherText string, password string) string {
    hasher := sha256.New()
    hasher.Write([]byte(password))
    key := hasher.Sum(nil)

    cipherText, err := hex.DecodeString(hexCipherText)
    if err != nil {
        return ""
    }

    block, _ := aes.NewCipher(key)
    gcm, _   := cipher.NewGCM(block)

    // ADD THIS CHECK
    if len(cipherText) < gcm.NonceSize() {
        return ""
    }

    nonceSize := gcm.NonceSize()
    nonce, actualSecretData := cipherText[:nonceSize], cipherText[nonceSize:]
    plainTextBytes, err := gcm.Open(nil, nonce, actualSecretData, nil)
    if err != nil {
        return ""
    }
    return string(plainTextBytes)
}
