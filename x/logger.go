package x

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"golang.org/x/net/context"
)

var logging LoggerImpl

type ILogger interface {
	Audit(ctx context.Context, args ...interface{})
	Sync()
}

func initLogger() error {
	core := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(os.Stdout), zap.DebugLevel)

	logger := zap.New(core)
	sugarLogger := logger.Sugar()

	logging = LoggerImpl{
		sugarLogger: sugarLogger,
	}
	return nil
}

type LoggerImpl struct {
	sugarLogger *zap.SugaredLogger
}

func Audit(args ...interface{}) {
	logging.sugarLogger.Info(args...)
}

func (l *LoggerImpl) Sync() {
	_ = l.sugarLogger.Sync()
}
