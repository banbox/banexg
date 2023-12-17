# 说明
此日志记录主要代码来自https://github.com/milvus-io/milvus/tree/master/pkg/log

# 如何使用
```go
imports github.com/anyongjin/banexg/log

// Optional
log.SetupByArgs(true, "D:/test.log")
log.Debugf("debug msg")
log.Infof("debug msg")
```