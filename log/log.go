package log

import (
	"errors"
	"fmt"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
)

var _globalL, _globalP, _globalS atomic.Value

var (
	_globalLevelLogger sync.Map
	_namedRateLimiters sync.Map
)

func init() {
	l, p := newStdLogger()

	replaceLeveledLoggers(l)
	_globalL.Store(l)
	_globalP.Store(p)

	s := _globalL.Load().(*zap.Logger).Sugar()
	_globalS.Store(s)

}

// InitLogger initializes a zap logger.
func InitLogger(cfg *Config, opts ...zap.Option) (*zap.Logger, *ZapProperties, error) {
	var outputs []zapcore.WriteSyncer
	if cfg.File != nil && len(cfg.File.LogPath) > 0 {
		lg, err := initFileLog(cfg.File)
		if err != nil {
			return nil, nil, err
		}
		outputs = append(outputs, zapcore.AddSync(lg))
	}
	if cfg.Stdout {
		stdOut, _, err := zap.Open([]string{"stdout"}...)
		if err != nil {
			return nil, nil, err
		}
		outputs = append(outputs, stdOut)
	}
	debugCfg := *cfg
	debugCfg.Level = "debug"
	outputsWriter := zap.CombineWriteSyncers(outputs...)
	debugL, r, err := InitLoggerWithWriteSyncer(&debugCfg, outputsWriter, cfg.Handlers, opts...)
	if err != nil {
		return nil, nil, err
	}
	replaceLeveledLoggers(debugL)
	level := zapcore.DebugLevel
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		return nil, nil, err
	}
	r.Level.SetLevel(level)
	return debugL.WithOptions(zap.AddCallerSkip(1)), r, nil
}

// InitLoggerWithWriteSyncer initializes a zap logger with specified  write syncer.
func InitLoggerWithWriteSyncer(cfg *Config, output zapcore.WriteSyncer, handlers []zapcore.Core, opts ...zap.Option) (*zap.Logger, *ZapProperties, error) {
	level := zap.NewAtomicLevel()
	err := level.UnmarshalText([]byte(cfg.Level))
	if err != nil {
		return nil, nil, fmt.Errorf("initLoggerWithWriteSyncer UnmarshalText cfg.Level errs:%w", err)
	}
	core := NewTextCore(newZapTextEncoder(cfg), output, level)
	if len(handlers) > 0 {
		handlers = append([]zapcore.Core{core}, handlers...)
		core = zapcore.NewTee(handlers...)
	}
	opts = append(cfg.buildOptions(output), opts...)
	lg := zap.New(core, opts...)
	r := &ZapProperties{
		Core:   core,
		Syncer: output,
		Level:  level,
	}
	return lg, r, nil
}

// initFileLog initializes file based logging options.
func initFileLog(cfg *FileLogConfig) (*lumberjack.Logger, error) {
	if st, err := os.Stat(cfg.LogPath); err == nil {
		if st.IsDir() {
			return nil, errors.New("can't use directory as log file name")
		}
	}
	if cfg.MaxSize == 0 {
		cfg.MaxSize = defaultLogMaxSize
	}

	// use lumberjack to logrotate
	return &lumberjack.Logger{
		Filename:   cfg.LogPath,
		MaxSize:    cfg.MaxSize,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxDays,
		LocalTime:  true,
	}, nil
}

func newStdLogger() (*zap.Logger, *ZapProperties) {
	conf := &Config{Level: "debug", Stdout: true, DisableErrorVerbose: true}
	lg, r, _ := InitLogger(conf, zap.OnFatal(zapcore.WriteThenPanic))
	return lg, r
}

// L returns the global Logger, which can be reconfigured with ReplaceGlobals.
// It's safe for concurrent use.
func L() *zap.Logger {
	return _globalL.Load().(*zap.Logger)
}

// S returns the global SugaredLogger, which can be reconfigured with
// ReplaceGlobals. It's safe for concurrent use.
func S() *zap.SugaredLogger {
	return _globalS.Load().(*zap.SugaredLogger)
}

func ctxL() *zap.Logger {
	level := _globalP.Load().(*ZapProperties).Level.Level()
	l, ok := _globalLevelLogger.Load(level)
	if !ok {
		return L()
	}
	return l.(*zap.Logger)
}

func debugL() *zap.Logger {
	v, _ := _globalLevelLogger.Load(zapcore.DebugLevel)
	return v.(*zap.Logger)
}

func infoL() *zap.Logger {
	v, _ := _globalLevelLogger.Load(zapcore.InfoLevel)
	return v.(*zap.Logger)
}

func warnL() *zap.Logger {
	v, _ := _globalLevelLogger.Load(zapcore.WarnLevel)
	return v.(*zap.Logger)
}

func errorL() *zap.Logger {
	v, _ := _globalLevelLogger.Load(zapcore.ErrorLevel)
	return v.(*zap.Logger)
}

func fatalL() *zap.Logger {
	v, _ := _globalLevelLogger.Load(zapcore.FatalLevel)
	return v.(*zap.Logger)
}

// ReplaceGlobals replaces the global Logger and SugaredLogger.
// It's safe for concurrent use.
func ReplaceGlobals(logger *zap.Logger, props *ZapProperties) {
	_globalL.Store(logger)
	_globalS.Store(logger.Sugar())
	_globalP.Store(props)
}

func replaceLeveledLoggers(debugLogger *zap.Logger) {
	levels := []zapcore.Level{
		zapcore.DebugLevel, zapcore.InfoLevel, zapcore.WarnLevel, zapcore.ErrorLevel,
		zapcore.DPanicLevel, zapcore.PanicLevel, zapcore.FatalLevel,
	}
	for _, level := range levels {
		levelL := debugLogger.WithOptions(zap.IncreaseLevel(level))
		_globalLevelLogger.Store(level, levelL)
	}
}

// Sync flushes any buffered log entries.
func Sync() error {
	if err := L().Sync(); err != nil {
		return err
	}
	if err := S().Sync(); err != nil {
		return err
	}
	var reterr error
	_globalLevelLogger.Range(func(key, val interface{}) bool {
		l := val.(*zap.Logger)
		if err := l.Sync(); err != nil {
			reterr = err
			return false
		}
		return true
	})
	return reterr
}

func Level() zap.AtomicLevel {
	return _globalP.Load().(*ZapProperties).Level
}

// SetupLogger is used to initialize the log with config.
func SetupLogger(cfg *Config) {
	// Initialize logger.
	logger, p, err := InitLogger(cfg, zap.AddStacktrace(zap.ErrorLevel))
	if err == nil {
		ReplaceGlobals(logger, p)
	} else {
		Fatal("initialize logger error", zap.Error(err))
	}
	if cfg.File != nil && len(cfg.File.LogPath) > 0 {
		Info("Log To", zap.String("path", cfg.File.LogPath))
	}
}

func Setup(debug bool, logFile string, handlers ...zapcore.Core) {
	if debug && len(logFile) == 0 {
		return
	}
	var level = "info"
	if debug {
		level = "debug"
	}
	var file *FileLogConfig
	if len(logFile) > 0 {
		file = &FileLogConfig{
			LogPath: logFile,
		}
	}
	SetupLogger(&Config{
		Stdout:   true,
		Format:   "text",
		Level:    level,
		File:     file,
		Handlers: handlers,
	})
}

func Type(key string, v interface{}) zap.Field {
	return zap.Stringer(fmt.Sprintf("type_of_%s", key), reflect.TypeOf(v))
}
