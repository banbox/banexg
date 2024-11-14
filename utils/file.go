package utils

import (
	"fmt"
	"github.com/banbox/banexg/errs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func WriteFile(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// 将字符串写入文件
	_, err = file.Write(data)
	if err != nil {
		return err
	}

	// 将文件的缓冲区内容刷新到磁盘
	err = file.Sync()
	if err != nil {
		return err
	}
	return nil
}

func WriteJsonFile(path string, data interface{}) error {
	bytes, err := Marshal(data)
	if err != nil {
		return err
	}
	return WriteFile(path, bytes)
}

func ReadFile(path string) ([]byte, error) {
	_, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// 读取文件内容
	stat, _ := file.Stat()
	fileSize := stat.Size()
	content := make([]byte, fileSize)
	_, err = file.Read(content)
	if err != nil {
		return nil, err
	}
	return content, nil
}

func ReadJsonFile(path string, obj interface{}) error {
	data, err := ReadFile(path)
	if err != nil {
		return err
	}
	return Unmarshal(data, obj)
}

func WriteCacheFile(key, content string, expSecs int) *errs.Error {
	path := filepath.Join(os.TempDir(), "banexg_"+key)
	file, err := os.Create(path)
	if err != nil {
		return errs.New(errs.CodeIOWriteFail, err)
	}
	expireAt := int64(0)
	if expSecs > 0 {
		expireAt = time.Now().UnixMilli() + int64(expSecs)*1000
	}
	_, err = file.WriteString(fmt.Sprintf("%v\n", expireAt))
	if err != nil {
		return errs.New(errs.CodeIOWriteFail, err)
	}
	_, err = file.WriteString(content)
	if err != nil {
		return errs.New(errs.CodeIOWriteFail, err)
	}
	err = file.Close()
	if err != nil {
		return errs.New(errs.CodeIOWriteFail, err)
	}
	return nil
}

func ReadCacheFile(key string) (string, *errs.Error) {
	path := filepath.Join(os.TempDir(), "banexg_"+key)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", errs.New(errs.CodeIOReadFail, err)
	}
	fileText := string(data)
	sepIdx := strings.Index(fileText, "\n")
	expireMS, err := strconv.ParseInt(fileText[:sepIdx], 10, 64)
	if err != nil {
		return "", errs.New(errs.CodeInvalidData, err)
	}
	if expireMS > 0 && expireMS < time.Now().UnixMilli() {
		stamp := time.UnixMilli(expireMS)
		expDate := stamp.Format("2006-01-02 15:04:05")
		return "", errs.NewMsg(errs.CodeExpired, "expired at: %v", expDate)
	}
	return fileText[sepIdx+1:], nil
}
