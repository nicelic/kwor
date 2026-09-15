package logger

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/alireza0/s-ui/config"
	"github.com/op/go-logging"
)

var (
	logger *logging.Logger
)

const (
	logBufferMaxEntries = 500
	logBufferMaxBytes   = 512 * 1024
	logEntryMaxBytes    = 16 * 1024

	diskLogMaxTotalBytes = 50 * 1024 * 1024 // 50MB 磁盘总限额
	diskLogMaxFileBytes  = 10 * 1024 * 1024 // 10MB 单文件滚动上限
)

type logBufferEntry struct {
	time  string
	level logging.Level
	log   string
}

// fixedLogRingBuffer 实现真正的定长环形队列，彻底杜绝切片泄漏与反复内存重新分配
type fixedLogRingBuffer struct {
	mu       sync.RWMutex
	entries  [logBufferMaxEntries]logBufferEntry
	head     int // 最老条目索引
	tail     int // 下一个写入索引
	count    int // 有效条目数
	curBytes int // 当前所有条目 log 字符串占用的总字节数
}

var globalRingBuffer fixedLogRingBuffer

// Compatibility variables for tests
var (
	logBufferMu    = &globalRingBuffer.mu
	logBufferBytes int
)

// diskLogManager 负责 Promanager_data/log 目录的日志落盘与严格 50MB 限额滚动管理
type diskLogManager struct {
	mu          sync.Mutex
	dir         string
	curFile     *os.File
	writer      *bufio.Writer
	curBytes    int64
	totalBytes  int64
	initialized bool
}

var globalDiskLogger diskLogManager

func (d *diskLogManager) initLocked() error {
	if d.initialized {
		return nil
	}
	d.dir = filepath.Join(config.GetDataDir(), "log")
	if err := os.MkdirAll(d.dir, 0750); err != nil {
		return err
	}

	// 启动时仅扫描一次目录，计算历史日志总大小并清理超限文件
	d.recalculateAndPruneLocked()

	activePath := filepath.Join(d.dir, "kwor.log")
	f, err := os.OpenFile(activePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0640)
	if err != nil {
		return err
	}
	fi, err := f.Stat()
	if err == nil {
		d.curBytes = fi.Size()
	}
	d.curFile = f
	d.writer = bufio.NewWriterSize(f, 32*1024) // 32KB 写入缓冲，极大降低高频磁盘 I/O
	d.initialized = true
	return nil
}

func (d *diskLogManager) recalculateAndPruneLocked() {
	entries, err := os.ReadDir(d.dir)
	if err != nil {
		return
	}
	type fileMeta struct {
		path    string
		size    int64
		modTime time.Time
	}
	var logFiles []fileMeta
	var total int64

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		p := filepath.Join(d.dir, entry.Name())
		sz := info.Size()
		total += sz
		if entry.Name() != "kwor.log" {
			logFiles = append(logFiles, fileMeta{
				path:    p,
				size:    sz,
				modTime: info.ModTime(),
			})
		}
	}

	// 按修改时间升序排列（最旧的在前面）
	sort.Slice(logFiles, func(i, j int) bool {
		return logFiles[i].modTime.Before(logFiles[j].modTime)
	})

	// 超过 50MB 限额时，淘汰最旧的历史日志文件直到总大小 <= 40MB
	for len(logFiles) > 0 && total > diskLogMaxTotalBytes {
		oldest := logFiles[0]
		logFiles = logFiles[1:]
		if err := os.Remove(oldest.path); err == nil {
			total -= oldest.size
		}
	}
	d.totalBytes = total
}

func (d *diskLogManager) rotateLocked() {
	if d.writer != nil {
		_ = d.writer.Flush()
	}
	if d.curFile != nil {
		_ = d.curFile.Close()
		d.curFile = nil
	}

	timestamp := time.Now().Format("20060102-150405")
	activePath := filepath.Join(d.dir, "kwor.log")
	rotatedPath := filepath.Join(d.dir, fmt.Sprintf("kwor-%s.log", timestamp))

	_ = os.Rename(activePath, rotatedPath)

	// 轮转后重新计算总大小并清理超标日志
	d.recalculateAndPruneLocked()

	f, err := os.OpenFile(activePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640)
	if err == nil {
		d.curFile = f
		d.curBytes = 0
		d.writer = bufio.NewWriterSize(f, 32*1024)
	}
}

func (d *diskLogManager) WriteLine(timeStr, levelStr, message string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.initialized {
		if err := d.initLocked(); err != nil {
			return
		}
	}

	line := fmt.Sprintf("%s [%s] %s\n", timeStr, levelStr, message)
	lineBytes := len(line)

	if d.curBytes+int64(lineBytes) >= diskLogMaxFileBytes {
		d.rotateLocked()
	}

	if d.writer != nil {
		n, _ := d.writer.WriteString(line)
		d.curBytes += int64(n)
		d.totalBytes += int64(n)
		// 如果是 ERROR 级别日志，立即刷新缓冲区确保落地
		if levelStr == "ERROR" {
			_ = d.writer.Flush()
		}
	}
}

func (d *diskLogManager) Flush() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.writer != nil {
		_ = d.writer.Flush()
	}
}

func InitLogger(level logging.Level) {
	newLogger := logging.MustGetLogger("kwor")
	var err error
	var backend logging.Backend
	var format logging.Formatter

	backend, err = logging.NewSyslogBackend("")
	if err != nil {
		backend = logging.NewLogBackend(os.Stderr, "", 0)
	}
	if err != nil {
		format = logging.MustStringFormatter(`%{time:2006/01/02 15:04:05} %{level} - %{message}`)
	} else {
		format = logging.MustStringFormatter(`%{level} - %{message}`)
	}

	backendFormatter := logging.NewBackendFormatter(backend, format)
	backendLeveled := logging.AddModuleLevel(backendFormatter)
	backendLeveled.SetLevel(level, "kwor")
	newLogger.SetBackend(backendLeveled)

	logger = newLogger

	// 初始化磁盘日志
	globalDiskLogger.mu.Lock()
	_ = globalDiskLogger.initLocked()
	globalDiskLogger.mu.Unlock()
}

func GetLogger() *logging.Logger {
	return logger
}

func FlushDiskLog() {
	globalDiskLogger.Flush()
}

func Debug(args ...interface{}) {
	if config.IsDebug() {
		if logger != nil {
			logger.Debug(args...)
		}
		addToBuffer("DEBUG", fmt.Sprint(args...))
	}
}

func Debugf(format string, args ...interface{}) {
	if config.IsDebug() {
		if logger != nil {
			logger.Debugf(format, args...)
		}
		addToBuffer("DEBUG", fmt.Sprintf(format, args...))
	}
}

func Info(args ...interface{}) {
	if logger != nil {
		logger.Info(args...)
	}
	addToBuffer("INFO", fmt.Sprint(args...))
}

func Infof(format string, args ...interface{}) {
	if logger != nil {
		logger.Infof(format, args...)
	}
	addToBuffer("INFO", fmt.Sprintf(format, args...))
}

func Warning(args ...interface{}) {
	if logger != nil {
		logger.Warning(args...)
	}
	addToBuffer("WARNING", fmt.Sprint(args...))
}

func Warningf(format string, args ...interface{}) {
	if logger != nil {
		logger.Warningf(format, args...)
	}
	addToBuffer("WARNING", fmt.Sprintf(format, args...))
}

func Error(args ...interface{}) {
	if logger != nil {
		logger.Error(args...)
	}
	addToBuffer("ERROR", fmt.Sprint(args...))
}

func Errorf(format string, args ...interface{}) {
	if logger != nil {
		logger.Errorf(format, args...)
	}
	addToBuffer("ERROR", fmt.Sprintf(format, args...))
}

func addToBuffer(level string, newLog string) {
	newLog = truncateLogEntry(newLog, logEntryMaxBytes)
	now := time.Now()
	timeStr := now.Format("2006/01/02 15:04:05")
	logLevel, _ := logging.LogLevel(level)

	entry := logBufferEntry{
		time:  timeStr,
		level: logLevel,
		log:   newLog,
	}

	// 1. 写入内存定长环形队列
	globalRingBuffer.push(entry)

	// 2. 写入 Promanager_data/log 磁盘管理
	globalDiskLogger.WriteLine(timeStr, level, newLog)
}

func (r *fixedLogRingBuffer) push(entry logBufferEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()

	entryLen := len(entry.log)

	// 当队列已满，或者加入新条目会导致总内存超出限制时，淘汰最老的数据
	for r.count >= logBufferMaxEntries || (r.curBytes+entryLen > logBufferMaxBytes && r.count > 0) {
		oldLen := len(r.entries[r.head].log)
		r.entries[r.head].log = "" // 显式置空引用，让 GC 立即回收旧字符串
		r.head = (r.head + 1) % logBufferMaxEntries
		r.count--
		r.curBytes -= oldLen
	}

	r.entries[r.tail] = entry
	r.tail = (r.tail + 1) % logBufferMaxEntries
	r.count++
	r.curBytes += entryLen

	logBufferBytes = r.curBytes
}

func truncateLogEntry(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	suffix := "...[truncated]"
	if maxBytes <= len(suffix) {
		return suffix[:maxBytes]
	}
	cut := maxBytes - len(suffix)
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return value[:cut] + suffix
}

func GetLogs(c int, level string) []string {
	if c <= 0 {
		return []string{}
	}
	if c > logBufferMaxEntries {
		c = logBufferMaxEntries
	}
	logLevel, _ := logging.LogLevel(level)

	globalRingBuffer.mu.RLock()
	defer globalRingBuffer.mu.RUnlock()

	if rCount := globalRingBuffer.count; c > rCount {
		c = rCount
	}
	if c == 0 {
		return []string{}
	}

	output := make([]string, 0, c)
	// 从最新到最老遍历
	idx := (globalRingBuffer.tail - 1 + logBufferMaxEntries) % logBufferMaxEntries
	for i := 0; i < globalRingBuffer.count && len(output) < c; i++ {
		entry := globalRingBuffer.entries[idx]
		if entry.level <= logLevel && entry.log != "" {
			output = append(output, fmt.Sprintf("%s %s - %s", entry.time, entry.level, entry.log))
		}
		idx = (idx - 1 + logBufferMaxEntries) % logBufferMaxEntries
	}
	return output
}
