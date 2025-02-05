/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package x

import (
	"os"
	"path/filepath"

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
