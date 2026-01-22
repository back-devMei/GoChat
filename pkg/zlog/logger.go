package zlog

import (
	"fmt"
	"os"
	"path"
	"runtime"

	"gochat/internal/config" //	项目内部配置包，用于获取日志路径

	"github.com/natefinch/lumberjack" // 提供日志文件轮转功能
	"go.uber.org/zap"                 // Uber 开源的高性能日志库
	"go.uber.org/zap/zapcore"         // 日志记录的核心组件
)

var logger *zap.Logger
var sugarLogger *zap.SugaredLogger
var logPath string

// 自动调用
func init() {
	// 初始化日志记录器
	encoderConfig := zap.NewProductionEncoderConfig()
	// 设置日志记录中时间格式
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	// 日志encoder：还是JSONEncoder，把日志行格式化成JSON格式
	encoder := zapcore.NewJSONEncoder(encoderConfig)

	// 从配置中获取日志路径
	conf := config.GetConfig()
	logPath = conf.LogPath

	// 确保日志目录存在
	ensureLogDirExists(logPath)

	// 获取日志级别
	logLevel := getLogLevelFromConfig(conf.LogLevel)

	// 使用文件日志写入器（带轮转功能）
	fileWriteSyncer := getFileLogWriter()

	// 创建日志记录核心，将日志同时输出到控制台和文件
	core := zapcore.NewTee(
		zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), logLevel),
		zapcore.NewCore(encoder, fileWriteSyncer, logLevel),
	)

	// 创建日志记录器
	logger = zap.New(core)
	sugarLogger = logger.Sugar()
}

// ensureLogDirExists 确保日志目录存在
func ensureLogDirExists(logPath string) {
	logDir := path.Dir(logPath)
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		os.MkdirAll(logDir, 0755)
	}
}

// getLogLevelFromConfig 从配置字符串获取日志级别
func getLogLevelFromConfig(levelStr string) zapcore.Level {
	switch levelStr {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	case "fatal":
		return zapcore.FatalLevel
	default:
		return zapcore.InfoLevel // 默认信息级别
	}
}

func getFileLogWriter() (writeSyncer zapcore.WriteSyncer) {
	// 使用 lumberjack 实现 log rotate
	lumberJackLogger := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    100,   // 单个文件最大100M
		MaxBackups: 60,    // 多于60个日志文件后，清理较旧的日志
		MaxAge:     1,     // 一天一切割
		Compress:   false, // 是否压缩旧的日志文件
	}

	return zapcore.AddSync(lumberJackLogger)
}

// getCallerInfoForLog 获得调用方的日志信息，包括函数名，文件名，行号
func getCallerInfoForLog() (callerFields []zap.Field) {
	pc, file, line, ok := runtime.Caller(2) // 回溯两层，拿到写日志的调用方的函数信息
	if !ok {
		return
	}
	funcName := runtime.FuncForPC(pc).Name()
	funcName = path.Base(funcName) // Base函数返回路径的最后一个元素，只保留函数名

	callerFields = append(callerFields, zap.String("func", funcName), zap.String("file", file), zap.Int("line", line))
	return
}

func Info(message string, fields ...zap.Field) {
	callerFields := getCallerInfoForLog()
	fields = append(fields, callerFields...)
	logger.Info(message, fields...)
}

func Warn(message string, fields ...zap.Field) {
	callerFields := getCallerInfoForLog()
	fields = append(fields, callerFields...)
	logger.Warn(message, fields...)
}

func Error(message string, fields ...zap.Field) {
	callerFields := getCallerInfoForLog()
	fields = append(fields, callerFields...)
	logger.Error(message, fields...)
}

func Fatal(message string, fields ...zap.Field) {
	callerFields := getCallerInfoForLog()
	fields = append(fields, callerFields...)
	logger.Fatal(message, fields...)
}

func Debug(message string, fields ...zap.Field) {
	callerFields := getCallerInfoForLog()
	fields = append(fields, callerFields...)
	logger.Debug(message, fields...)
}

// With 返回一个带有额外字段的新日志记录器
func With(fields ...zap.Field) *zap.Logger {
	return logger.With(fields...)
}

// Sugar 返回一个 SugaredLogger，提供更简洁的 API
func Sugar() *zap.SugaredLogger {
	return sugarLogger
}

// Infof 使用格式化字符串记录信息级日志
func Infof(template string, args ...interface{}) {
	callerFields := getCallerInfoForLog()
	sugarLogger.Infow(template, append(callerFields, toFields(args)...))
}

// Errorf 使用格式化字符串记录错误级日志
func Errorf(template string, args ...interface{}) {
	callerFields := getCallerInfoForLog()
	sugarLogger.Errorw(template, append(callerFields, toFields(args)...))
}

// Warnf 使用格式化字符串记录警告级日志
func Warnf(template string, args ...interface{}) {
	callerFields := getCallerInfoForLog()
	sugarLogger.Warnw(template, append(callerFields, toFields(args)...))
}

// Debugf 使用格式化字符串记录调试级日志
func Debugf(template string, args ...interface{}) {
	callerFields := getCallerInfoForLog()
	sugarLogger.Debugw(template, append(callerFields, toFields(args)...))
}

// Fatalf 使用格式化字符串记录致命级日志
func Fatalf(template string, args ...interface{}) {
	callerFields := getCallerInfoForLog()
	sugarLogger.Fatalw(template, append(callerFields, toFields(args)...))
}

// toFields 将可变参数转换为 zap.Field
func toFields(args []interface{}) []zap.Field {
	fields := make([]zap.Field, 0, len(args))
	for i, arg := range args {
		fields = append(fields, zap.Any(fmt.Sprintf("arg%d", i), arg))
	}
	return fields
}
