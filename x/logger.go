package x

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type ILogger interface {
	AuditI(string, ...interface{})
	Sync()
}

func InitLogger(dir string, filename string) (ILogger, error) {
	getWriterSyncer := func() zapcore.WriteSyncer {
		return zapcore.AddSync(&lumberjack.Logger{
			Filename: dir + "/" + filename,
			MaxSize:  100,
			MaxAge:   30,
		})
	}
	core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		getWriterSyncer(), zap.DebugLevel)

	return &LoggerImpl{
		logger: zap.New(core),
	}, nil
}

type LoggerImpl struct {
	logger *zap.Logger
}

func (l *LoggerImpl) AuditI(msg string, args ...interface{}) {
	flds := make([]zap.Field, len(args)/2)
	for i := 0; i < len(args); i = i + 2 {
		flds[i/2] = zap.Any(args[i].(string), args[i+1])
	}
	l.logger.Info(msg, flds...)
}

func (l *LoggerImpl) AuditE(msg string, args ...interface{}) {
	flds := make([]zap.Field, len(args)/2)
	for i := range args {
		flds[i/2] = zap.Any(args[i].(string), args[i+1])
	}
	l.logger.Error(msg, flds...)
}

func (l *LoggerImpl) Sync() {
	if l == nil {
		return
	}
	_ = l.logger.Sync()
}
