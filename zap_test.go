package cypress

import (
	"os"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestZapLogger(t *testing.T) {
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{
		"stdout",
	}

	logger, err := config.Build()
	if err != nil {
		t.Error(err)
		return
	}

	defer logger.Sync()
	logger.Info("test message", zap.String("field", "value"), zap.Stack("callstack"))

	jsonEncoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	writeSyncer := zapcore.AddSync(os.Stdout)
	zapCore := zapcore.NewCore(jsonEncoder, writeSyncer, zapcore.DebugLevel)
	l := zap.New(zapCore)
	defer l.Sync()
	l.Info("test message1", zap.String("field", "value2"))
}
