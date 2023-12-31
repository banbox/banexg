package utils

import (
	"bytes"
	"crypto"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/anyongjin/banexg/errs"
	"github.com/anyongjin/banexg/log"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
	"hash"
	"io"
)

/*
Signature
method: rsa, eddsa, hmac

	hashName:
		rsa: sha256/sha384/sha512
		eddsa: 不需要
		hmac:sha256/sha384/sha512
	digest: hmac: base64/hex
*/
func Signature(data string, secret string, method string, hashName string, digest string) (string, *errs.Error) {
	secretBytes, err := EncodeToLatin1(secret)
	if err != nil {
		return "", errs.New(errs.CodeSignFail, err)
	}
	dataBytes, err := EncodeToLatin1(data)
	if err != nil {
		return "", errs.New(errs.CodeSignFail, err)
	}
	algoMap := map[string]crypto.Hash{
		"sha256": crypto.SHA256,
		"sha384": crypto.SHA384,
		"sha512": crypto.SHA512,
	}
	hashType, ok := algoMap[hashName]
	var sign string
	if method == "rsa" {
		if !ok {
			return "", errs.NewMsg(errs.CodeSignFail, "unsupport hash type:"+hashName)
		}
		secretKey, err := loadPrivateKey(secretBytes)
		if err != nil {
			return "", errs.New(errs.CodeSignFail, err)
		}
		sign, err = rsaSign(dataBytes, secretKey, hashType)
	} else if method == "eddsa" {
		sign, err = Eddsa(dataBytes, secretBytes)
	} else if method == "hmac" {
		if !ok {
			return "", errs.NewMsg(errs.CodeSignFail, "unsupport hash type:"+hashName)
		}
		sign = HMAC(dataBytes, secretBytes, hashType.New, digest)
		return sign, nil
	} else {
		msgText := "unsupport sign method: " + method
		log.Panic(msgText)
		return "", errs.NewMsg(errs.CodeSignFail, msgText)
	}
	if err != nil {
		return "", errs.New(errs.CodeSignFail, err)
	}
	sign = EncodeURIComponent(sign, UriEncodeSafe)
	return sign, nil
}

// RSA签名
func rsaSign(data []byte, privateKey *rsa.PrivateKey, hashType crypto.Hash) (string, error) {
	var hashed []byte
	var h hash.Hash

	switch hashType {
	case crypto.SHA256:
		h = sha256.New()
	case crypto.SHA384:
		h = sha512.New384()
	case crypto.SHA512:
		h = sha512.New()
	default:
		return "", errors.New("unsupported hash type")
	}

	h.Write(data)
	hashed = h.Sum(nil)

	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, hashType, hashed)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(signature), nil
}

// 加载PEM格式的私钥
func loadPrivateKey(pemEncoded []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemEncoded)
	if block == nil {
		return nil, errors.New("failed to parse PEM block containing the key")
	}

	privKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	return privKey, nil
}

func Eddsa(request []byte, secret []byte) (string, error) {
	block, _ := pem.Decode(secret)
	if block == nil {
		return "", fmt.Errorf("failed to parse PEM block containing the key")
	}

	privateKey := ed25519.NewKeyFromSeed(block.Bytes)
	sig := ed25519.Sign(privateKey, request)
	return base64.StdEncoding.EncodeToString(sig), nil
}

func HMAC(request []byte, secret []byte, algorithm func() hash.Hash, digest string) string {
	h := hmac.New(algorithm, secret)
	h.Write(request)
	binary := h.Sum(nil)

	switch digest {
	case "hex":
		return hex.EncodeToString(binary)
	case "base64":
		return base64.StdEncoding.EncodeToString(binary)
	default:
		return string(binary)
	}
}

func EncodeToLatin1(input string) ([]byte, error) {
	encoder := charmap.ISO8859_1.NewEncoder()
	reader := transform.NewReader(bytes.NewReader([]byte(input)), encoder)
	return io.ReadAll(reader)
}
