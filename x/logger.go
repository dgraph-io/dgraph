/*
 * SPDX-FileCopyrightText: © 2017-2025 Istari Digital, Inc.
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type LoggerConf struct {
	Compress      bool
	Output        string
	EncryptionKey Sensitive
	Size          int64
	Days          int64
	MessageKey    string
}

func InitLogger(conf *LoggerConf, filename string) (*Logger, error) {
	config := zap.NewProductionEncoderConfig()
	config.MessageKey = conf.MessageKey
	config.LevelKey = zapcore.OmitKey
	config.EncodeTime = zapcore.ISO8601TimeEncoder
	// if stdout, then init the logger and return
	if conf.Output == "stdout" {
		return &Logger{
			logger: zap.New(zapcore.NewCore(zapcore.NewJSONEncoder(config),
				zapcore.AddSync(os.Stdout), zapcore.DebugLevel)),
			writer: nil,
		}, nil
	}

	if err := os.MkdirAll(conf.Output, 0700); err != nil {
		return nil, err
	}
	if conf.EncryptionKey != nil {
		filename = filename + ".enc"
	}

	path, err := filepath.Abs(filepath.Join(conf.Output, filename))
	if err != nil {
		return nil, err
	}
	w := &LogWriter{
		FilePath:      path,
		MaxSize:       conf.Size,
		MaxAge:        conf.Days,
		EncryptionKey: conf.EncryptionKey,
		Compress:      conf.Compress,
	}
	if w, err = w.Init(); err != nil {
		return nil, err
	}

	return &Logger{
		logger: zap.New(zapcore.NewCore(zapcore.NewJSONEncoder(config),
			zapcore.AddSync(w), zap.DebugLevel)),
		writer: w,
	}, nil
}

type Logger struct {
	logger *zap.Logger
	writer *LogWriter
}

// AuditI logs audit message as info. args are key value pairs with key as string value
func (l *Logger) AuditI(msg string, args ...interface{}) {
	if l == nil {
		return
	}
	flds := make([]zap.Field, 0, len(args))
	for i := 0; i < len(args); i = i + 2 {
		flds = append(flds, zap.Any(args[i].(string), args[i+1]))
	}
	l.logger.Info(msg, flds...)
}

func (l *Logger) AuditE(msg string, args ...interface{}) {
	if l == nil {
		return
	}
	flds := make([]zap.Field, 0, len(args))
	for i := 0; i < len(args); i = i + 2 {
		flds = append(flds, zap.Any(args[i].(string), args[i+1]))
	}
	l.logger.Error(msg, flds...)
}

func (l *Logger) Sync() {
	if l == nil {
		return
	}
	_ = l.logger.Sync()
	_ = l.writer.Close()
}

var slowOperationLogger *zap.Logger

func init() {
	initSlowOperationLogger()
}

func initSlowOperationLogger() {
	config := zap.NewProductionEncoderConfig()
	config.EncodeTime = zapcore.ISO8601TimeEncoder
	config.TimeKey = "timestamp"
	config.MessageKey = "message"

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(config),
		zapcore.AddSync(os.Stderr),
		zapcore.WarnLevel,
	)
	slowOperationLogger = zap.New(core)
}

// SlowOperationLatency holds timing information for slow operation logging.
type SlowOperationLatency struct {
	Start      time.Time
	Parsing    time.Duration
	Processing time.Duration
	Encoding   time.Duration
}

// Total returns the total duration since Start.
func (l *SlowOperationLatency) Total() time.Duration {
	return time.Since(l.Start)
}

// LogSlowOperation logs a slow operation with structured fields including trace ID.
// It only logs if the operation duration exceeds the configured threshold.
// Parameters:
//   - operation: the type of operation (e.g., "query", "mutation", "backup")
//   - opType: specific type within the operation (e.g., "dql", "graphql")
//   - payload: the operation payload (e.g., query text) - will be truncated if too long
func LogSlowOperation(ctx context.Context, operation, opType, payload string, latency *SlowOperationLatency) {
	threshold := WorkerConfig.SlowQueryLogThreshold
	if threshold <= 0 {
		return
	}

	total := latency.Total()
	if total < threshold {
		return
	}

	fields := []zap.Field{
		zap.String("operation", operation),
		zap.String("type", opType),
		zap.Duration("total", total),
		zap.Duration("parsing", latency.Parsing),
		zap.Duration("processing", latency.Processing),
		zap.Duration("encoding", latency.Encoding),
	}

	// Extract trace ID from context if available
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		fields = append(fields,
			zap.String("trace_id", span.SpanContext().TraceID().String()),
			zap.String("span_id", span.SpanContext().SpanID().String()),
		)
	}

	// Truncate payload for logging (avoid huge log entries)
	if len(payload) > 1000 {
		payload = payload[:1000] + "...[truncated]"
	}
	fields = append(fields, zap.String("payload", payload))

	slowOperationLogger.Warn("slow operation", fields...)
}
