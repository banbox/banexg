package log

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

// OutCapture capture stdout/stderr and write to file
type OutCapture struct {
	orgStdout *os.File
	orgStderr *os.File
	outFile   *os.File
	errFile   *os.File
	outReader *os.File
	outWriter *os.File
	errReader *os.File
	errWriter *os.File
	wg        sync.WaitGroup
}

// NewOutCaptureToFile outPath, errPath can be same or different
func NewOutCaptureToFile(outPath, errPath string) (*OutCapture, error) {
	// 创建输出文件
	outFile, err := os.Create(outPath)
	if err != nil {
		return nil, err
	}

	var errFile *os.File
	if errPath == "" || errPath == outPath {
		errFile = outFile
	} else {
		// 创建错误输出文件
		errFile, err = os.Create(errPath)
		if err != nil {
			outFile.Close()
			return nil, err
		}
	}
	var sta *OutCapture
	sta, err = NewOutCapture(outFile, errFile)
	if err != nil {
		outFile.Close()
		if outFile != errFile {
			errFile.Close()
		}
		return nil, err
	}
	return sta, nil
}

func NewOutCapture(outFile, errFile *os.File) (*OutCapture, error) {
	if outFile == nil || errFile == nil {
		return nil, errors.New("outFile and errFile are required")
	}
	// 保存原始的stdout和stderr
	orgStdout := os.Stdout
	orgStderr := os.Stderr

	// 为stdout创建pipe
	outReader, outWriter, err := os.Pipe()
	if err != nil {
		return nil, err
	}

	// 为stderr创建pipe
	errReader, errWriter, err := os.Pipe()
	if err != nil {
		outReader.Close()
		outWriter.Close()
		return nil, err
	}

	return &OutCapture{
		orgStdout: orgStdout,
		orgStderr: orgStderr,
		outFile:   outFile,
		errFile:   errFile,
		outReader: outReader,
		outWriter: outWriter,
		errReader: errReader,
		errWriter: errWriter,
	}, nil
}

// Start start capture stdout/stderr
func (c *OutCapture) Start() {
	// 将标准输出重定向到管道
	os.Stdout = c.outWriter
	os.Stderr = c.errWriter

	// 启动goroutine监听stdout并写入文件
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		io.Copy(io.MultiWriter(c.outFile, c.orgStdout), c.outReader)
	}()

	// 启动goroutine监听stderr并写入文件
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		io.Copy(io.MultiWriter(c.errFile, c.orgStderr), c.errReader)
	}()
}

// Stop stop capture stdout/stderr
func (c *OutCapture) Stop() {
	// 恢复原始的stdout和stderr
	os.Stdout = c.orgStdout
	os.Stderr = c.orgStderr

	// 关闭写入端以触发读取goroutine的退出
	err := c.outWriter.Close()
	if err != nil {
		fmt.Printf("close outWriter fail %v\n", err)
	}
	err = c.errWriter.Close()
	if err != nil {
		fmt.Printf("close errWriter fail %v\n", err)
	}

	// 等待所有goroutine完成
	c.wg.Wait()

	// 关闭所有文件
	err = c.outReader.Close()
	if err != nil {
		fmt.Printf("close outReader fail %v\n", err)
	}
	err = c.errReader.Close()
	if err != nil {
		fmt.Printf("close outReader fail %v\n", err)
	}
	err = c.outFile.Close()
	if err != nil {
		fmt.Printf("close outFile fail %v\n", err)
	}
	if c.errFile != c.outFile {
		err = c.errFile.Close()
		if err != nil {
			fmt.Printf("close errFile fail %v\n", err)
		}
	}
}
