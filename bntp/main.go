package bntp

import (
	"fmt"
	"github.com/banbox/banexg/utils"
	"github.com/sasha-s/go-deadlock"
	"math/rand"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/banbox/banexg/log"
	"github.com/beevik/ntp"
	"go.uber.org/zap"
)

// TimeSync 时间同步管理器
type TimeSync struct {
	mutex       deadlock.RWMutex
	offset      int64         // 本地时间与标准时间的偏差(毫秒)
	syncPeriod  time.Duration // 同步周期
	loopRefresh bool          // 是否开启定期刷新
	filePath    string        // 保存时间偏移的文件路径
	randomRate  float64       // 随机波动率
	langCode    string        // 语言代码
	stopChan    chan struct{} // 停止同步循环的信号通道

	// 原子访问的偏移量，用于高性能读取
	atomicOffset atomic.Int64
}

// 原子缓存，用于快速获取时间戳
var (
	// 上次缓存刷新的时间戳（毫秒）
	lastUpdateTimeMs atomic.Int64
	// 缓存的时间偏移（毫秒）
	cachedOffset atomic.Int64
	// 缓存有效期（毫秒）
	cacheValidDurationMs int64 = 1000 // 默认1秒
	// 默认时区
	LangCode = LangNone
)

const (
	LangNone   = "none"
	LangZhCN   = "zh-CN"
	LangZhHK   = "zh-HK"
	LangZhTW   = "zh-TW"
	LangJaJP   = "ja-JP"
	LangKoKr   = "ko-KR"
	LangZhSg   = "zh-SG"
	LangGlobal = "global"
)

// 按地区组织的NTP服务器
var regionNTPServers = map[string][]string{
	//https://dns.icoa.cn/ntp/#china
	"zh-CN": {
		"ntp.ntsc.ac.cn", "cn.ntp.org.cn", "ntp1.nim.ac.cn",
		"ntp.tencent.com", "ntp1.tencent.com", "ntp2.tencent.com",
		"ntp3.tencent.com", "ntp4.tencent.com", "ntp5.tencent.com",
		"ntp.aliyun.com", "ntp1.aliyun.com", "ntp2.aliyun.com",
		"ntp3.aliyun.com", "ntp4.aliyun.com", "ntp5.aliyun.com",
		"ntp6.aliyun.com", "ntp7.aliyun.com",
	},
	"zh-HK": {"hk.ntp.org.cn", "stdtime.gov.hk"},
	"zh-TW": {"tw.ntp.org.cn"},
	"ja-JP": {"jp.ntp.org.cn", "ntp.nict.jp"},
	"ko-KR": {"kr.ntp.org.cn", "time.kriss.re.kr", "time2.kriss.re.kr"},
	"zh-SG": {"sgp.ntp.org.cn"},
	"global": {
		"time1.google.com", "time2.google.com", "time3.google.com", "time4.google.com",
		"time1.apple.com", "time2.apple.com", "time3.apple.com", "time4.apple.com",
		"time5.apple.com", "time6.apple.com", "time7.apple.com",
		"time.windows.com",
		"time.facebook.com", "time1.facebook.com", "time2.facebook.com",
		"time3.facebook.com", "time4.facebook.com", "time5.facebook.com",
	},
}

// 全局单例实例和保护锁
var (
	timeSyncer     *TimeSync
	timeSyncerLock deadlock.Mutex
)

// Option 定义TimeSync的配置选项
type Option func(*TimeSync)

// WithFilePath 设置偏移文件路径
func WithFilePath(path string) Option {
	return func(ts *TimeSync) {
		ts.filePath = path
	}
}

// WithRandomRate 设置同步周期的随机波动率
func WithRandomRate(rate float64) Option {
	return func(ts *TimeSync) {
		if rate >= 0 && rate <= 1 {
			ts.randomRate = rate
		}
	}
}

// WithCountryCode 设置国家代码
func WithCountryCode(code string) Option {
	return func(ts *TimeSync) {
		if _, ok := regionNTPServers[code]; !ok {
			panic(fmt.Sprintf("invalid lang code for bntp: %s", code))
		}
		ts.langCode = code
	}
}

// WithLoopRefresh 启用定期刷新
func WithLoopRefresh(enable bool) Option {
	return func(ts *TimeSync) {
		ts.loopRefresh = enable
	}
}

// WithSyncPeriod 设置同步周期
func WithSyncPeriod(period time.Duration) Option {
	return func(ts *TimeSync) {
		if period > 0 {
			if period < time.Hour {
				log.Warn("ntp refresh period suggest to be >= 1 hour")
			}
			ts.syncPeriod = period
		}
	}
}

// 偏移记录结构
type OffsetRecord struct {
	Timestamp int64 `json:"timestamp"` // 记录时间
	Offset    int64 `json:"offset"`    // 时间偏移(毫秒)
}

// 获取按国家代码排序的NTP服务器列表
func getNTPServersByCode(langCode string) []string {
	var servers []string

	// 首先添加指定国家的服务器
	if countryServers, ok := regionNTPServers[langCode]; ok {
		servers = append(servers, countryServers...)
	}
	rand.Shuffle(len(servers), func(i, j int) { servers[i], servers[j] = servers[j], servers[i] })

	// 然后添加全球服务器
	if langCode != "global" {
		globals := append([]string{}, regionNTPServers["global"]...)
		rand.Shuffle(len(globals), func(i, j int) { globals[i], globals[j] = globals[j], globals[i] })
		servers = append(servers, globals...)
	}

	return servers
}

func ClearTimeSync() {
	timeSyncerLock.Lock()
	if timeSyncer != nil {
		timeSyncer.Close()
		timeSyncer = nil
	}
	timeSyncerLock.Unlock()
}

// SetTimeSync init timeSyncer, call `ClearTimeSync` if reset is need
func SetTimeSync(options ...Option) (*TimeSync, error) {
	timeSyncerLock.Lock()
	defer timeSyncerLock.Unlock()

	if timeSyncer != nil {
		return timeSyncer, nil
	}

	// 创建新实例或重用现有实例
	cacheDir, err := utils.GetCacheDir()
	if err != nil {
		log.Warn("get cache dir fail, use default", zap.Error(err))
		cacheDir = os.TempDir()
	}
	curLang := LangCode
	if curLang == LangNone {
		curLang = LangGlobal
	}
	timeSyncer = &TimeSync{
		syncPeriod:  24 * time.Hour,
		filePath:    filepath.Join(cacheDir, "ban_ntp.json"),
		randomRate:  0.1,
		langCode:    curLang,
		loopRefresh: false, // 默认不启用定期刷新
	}

	err = timeSyncer.setOptions(options...)

	return timeSyncer, err
}

// GetTimeSync 获取时间同步器实例
// 如果实例不存在，则创建一个默认配置的实例
func GetTimeSync() *TimeSync {
	if timeSyncer == nil {
		ts, err := SetTimeSync()
		if err != nil {
			log.Warn("ntp initialization failed, using local time", zap.Error(err))
		}
		return ts
	}
	return timeSyncer
}

func (ts *TimeSync) SetOptions(options ...Option) error {
	ts.mutex.Lock()
	defer ts.mutex.Unlock()
	return ts.setOptions(options...)
}

func (ts *TimeSync) setOptions(options ...Option) error {
	if ts.stopChan != nil {
		close(ts.stopChan)
	}

	// restart stopChan
	ts.stopChan = make(chan struct{})

	// apply options
	for _, option := range options {
		option(ts)
	}

	// ensure cache dir
	dir := filepath.Dir(ts.filePath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		_ = os.MkdirAll(dir, 0755)
	}

	// 先尝试从文件加载
	loaded, err := ts.loadOffsetFromFile()
	if err != nil {
		log.Warn("failed to load time offset", zap.Error(err))
	}

	// 如果没有加载到有效数据，执行同步
	if !loaded {
		if err = ts.refresh(); err != nil {
			return fmt.Errorf("failed to sync time: %w", err)
		}
	}

	// 并启动循环同步（如果需要）
	if ts.loopRefresh {
		go ts.loopSync()
	}

	return nil
}

// saveOffsetToFile 将时间偏移保存到本地文件
func (ts *TimeSync) saveOffsetToFile() error {
	record := OffsetRecord{
		Timestamp: time.Now().Unix(),
		Offset:    ts.offset,
	}

	data, err := utils.Marshal(record)
	if err != nil {
		return err
	}

	return os.WriteFile(ts.filePath, data, 0644)
}

// loadOffsetFromFile 从本地文件加载时间偏移
func (ts *TimeSync) loadOffsetFromFile() (bool, error) {
	data, err := os.ReadFile(ts.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil // 文件不存在，不是错误
		}
		return false, err
	}

	var record OffsetRecord
	if err = utils.Unmarshal(data, &record, utils.JsonNumAuto); err != nil {
		return false, err
	}

	// 检查偏移值是否太旧
	if time.Now().Unix()-record.Timestamp > int64(ts.syncPeriod.Seconds()) {
		return false, nil
	}

	// 更新偏移值（同时更新原子变量和缓存）
	ts.offset = record.Offset

	// 原子更新
	ts.atomicOffset.Store(record.Offset)
	cachedOffset.Store(record.Offset)
	lastUpdateTimeMs.Store(time.Now().UnixMilli())

	log.Info("restored ntp", zap.Int64("offset_ms", record.Offset),
		zap.String("from", time.Unix(record.Timestamp, 0).Format(time.RFC3339)))
	return true, nil
}

// getRandomizedSyncPeriod 获取带随机波动的同步周期
func (ts *TimeSync) getRandomizedSyncPeriod() time.Duration {
	if ts.randomRate <= 0 {
		return ts.syncPeriod
	}

	// 生成[-randomRate, +randomRate]范围内的随机浮点数
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	randomFactor := (2*r.Float64() - 1) * ts.randomRate

	// 计算随机化后的周期（基础周期 +/- 随机波动）
	randomizedSeconds := float64(ts.syncPeriod.Seconds()) * (1 + randomFactor)
	return time.Duration(randomizedSeconds) * time.Second
}

func (ts *TimeSync) Close() {
	ts.mutex.Lock()
	defer ts.mutex.Unlock()
	if ts.stopChan != nil {
		close(ts.stopChan)
		ts.stopChan = nil
	}
}

// SetCacheValidDuration 设置缓存有效期
func SetCacheValidDuration(duration time.Duration) {
	atomic.StoreInt64(&cacheValidDurationMs, duration.Milliseconds())
}

// Refresh 刷新时间偏移并保存
func (ts *TimeSync) Refresh() error {
	ts.mutex.Lock()
	defer ts.mutex.Unlock()
	return ts.refresh()
}

func (ts *TimeSync) refresh() error {
	err := ts.syncTime()
	if err != nil {
		return err
	}

	// 保存同步结果
	if err = ts.saveOffsetToFile(); err != nil {
		log.Warn("failed to save time offset", zap.Error(err))
	}
	return nil
}

// loopSync 定期同步时间
func (ts *TimeSync) loopSync() {
	stopChan := ts.stopChan // 本地副本避免竞态条件

	for {
		syncPeriod := ts.getRandomizedSyncPeriod()
		select {
		case <-stopChan:
			return
		case <-time.After(syncPeriod):
			if err := ts.Refresh(); err != nil {
				log.Warn("time sync refresh failed", zap.Error(err))
			}
		}
	}
}

// SyncTime 同步时间，计算本地时间与NTP服务器时间的偏差
func (ts *TimeSync) syncTime() error {
	var lastErr error

	// 尝试所有NTP服务器，直到成功
	servers := getNTPServersByCode(ts.langCode)
	for _, server := range servers {
		ntpTime, err := ntp.Time(server)
		if err != nil {
			lastErr = err
			log.Warn("ntp query failed", zap.String("server", server), zap.Error(err))
			continue // 尝试下一个服务器
		}

		// 计算偏差(毫秒)
		localTime := time.Now()
		offsetMs := ntpTime.UnixMilli() - localTime.UnixMilli()

		// 更新偏差值（同时更新原子变量和缓存）
		ts.offset = offsetMs

		// 原子更新
		ts.atomicOffset.Store(offsetMs)
		cachedOffset.Store(offsetMs)
		lastUpdateTimeMs.Store(time.Now().UnixMilli())

		log.Info("ntp sync successful",
			zap.String("server", server),
			zap.Int64("offset_ms", offsetMs))
		return nil // 同步成功
	}

	// 所有服务器都失败了
	return fmt.Errorf("all ntp servers failed: %w", lastErr)
}

// UTCStamp 获取校正后的UTC时间戳(毫秒)
func UTCStamp() int64 {
	nowMs := time.Now().UnixMilli()

	if LangCode == LangNone {
		return nowMs
	}

	// 快速路径: 检查缓存是否有效
	lastUpdate := lastUpdateTimeMs.Load()
	cacheDuration := atomic.LoadInt64(&cacheValidDurationMs)

	// 如果缓存有效，直接返回结果
	if lastUpdate > 0 && nowMs-lastUpdate < cacheDuration {
		return nowMs + cachedOffset.Load()
	}

	// 缓存无效，尝试使用同步器的值
	ts := GetTimeSync()

	// 使用原子操作获取偏移量
	offset := ts.atomicOffset.Load()

	// 更新全局缓存
	cachedOffset.Store(offset)
	lastUpdateTimeMs.Store(nowMs)

	return nowMs + offset
}

func Now() time.Time {
	correctedTime := UTCStamp()
	return time.Unix(0, correctedTime*int64(time.Millisecond))
}

// GetTimeOffset get time offset in milliseconds(>0 means local < standard)
func GetTimeOffset() int64 {
	if LangCode == LangNone {
		return 0
	}
	ts := GetTimeSync()

	return ts.atomicOffset.Load()
}
