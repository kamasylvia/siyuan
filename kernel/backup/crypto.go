// SiYuan - https://github.com/siyuan-note/siyuan
// Copyright (c) 2020-present, b3log.org
// SPDX-License-Identifier: AGPL-3.0-or-later

package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"

	"golang.org/x/crypto/scrypt"
)

const (
	// scrypt parameters are stored in every manifest so the KDF can evolve
	// without making older snapshots unreadable.
	scryptN = 1 << 15
	scryptR = 8
	scryptP = 1
)

type ManifestEncryption struct {
	Cipher string `json:"cipher"`
	KDF    string `json:"kdf"`
	Salt   string `json:"salt"`
	N      int    `json:"n"`
	R      int    `json:"r"`
	P      int    `json:"p"`
}

func newEncryptionInfo(passphrase string) (*ManifestEncryption, []byte, error) {
	if passphrase == "" {
		return nil, nil, errors.New("backup encryption passphrase is empty")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, nil, err
	}
	key, err := scrypt.Key([]byte(passphrase), salt, scryptN, scryptR, scryptP, 32)
	if err != nil {
		return nil, nil, err
	}
	return &ManifestEncryption{
		Cipher: "aes-256-gcm", KDF: "scrypt",
		Salt: base64.StdEncoding.EncodeToString(salt), N: scryptN, R: scryptR, P: scryptP,
	}, key, nil
}

func deriveEncryptionKey(passphrase string, info *ManifestEncryption) ([]byte, error) {
	if info == nil {
		return nil, nil
	}
	if info.Cipher != "aes-256-gcm" || info.KDF != "scrypt" {
		return nil, fmt.Errorf("unsupported backup encryption [%s/%s]", info.KDF, info.Cipher)
	}
	salt, err := base64.StdEncoding.DecodeString(info.Salt)
	if err != nil {
		return nil, err
	}
	n, r, p := info.N, info.R, info.P
	if n <= 0 || r <= 0 || p <= 0 {
		n, r, p = scryptN, scryptR, scryptP
	}
	return scrypt.Key([]byte(passphrase), salt, n, r, p, 32)
}

func sealBlob(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func openBlob(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return nil, errors.New("encrypted backup blob is truncated")
	}
	nonce := ciphertext[:gcm.NonceSize()]
	return gcm.Open(nil, nonce, ciphertext[gcm.NonceSize():], nil)
}

func blobHash(data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:])
}
