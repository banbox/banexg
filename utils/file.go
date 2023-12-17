package utils

import (
	"github.com/bytedance/sonic"
	"os"
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
	bytes, err := sonic.Marshal(data)
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
	return sonic.Unmarshal(data, obj)
}
