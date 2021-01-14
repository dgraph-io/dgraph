package x

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

func InitLogger(dir string, filename string) (*Logger, error) {
	getWriterSyncer := func() zapcore.WriteSyncer {
		return zapcore.AddSync(&lumberjack.Logger{
			Filename: dir + "/" + filename,
			MaxSize:  100,
			MaxAge:   30,
		})
	}
	core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		getWriterSyncer(), zap.DebugLevel)

	return &Logger{
		logger: zap.New(core),
	}, nil
}

type Logger struct {
	logger *zap.Logger
}

func (l *Logger) AuditI(msg string, args ...interface{}) {
	if l == nil {
		return
	}
	flds := make([]zap.Field, len(args)/2)
	for i := 0; i < len(args); i = i + 2 {
		flds[i/2] = zap.Any(args[i].(string), args[i+1])
	}
	l.logger.Info(msg, flds...)
}

func (l *Logger) AuditE(msg string, args ...interface{}) {
	if l == nil {
		return
	}
	flds := make([]zap.Field, len(args)/2)
	for i := range args {
		flds[i/2] = zap.Any(args[i].(string), args[i+1])
	}
	l.logger.Error(msg, flds...)
}

func (l *Logger) Sync() {
	if l == nil {
		return
	}
	_ = l.logger.Sync()
}
