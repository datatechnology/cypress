package cypress

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	uuid "github.com/satori/go.uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// LogLevel logging level
type LogLevel int32

// ContextKey key type for context values
type ContextKey struct{}

// TraceActivityIDKey context key for trace activity id
var TraceActivityIDKey = ContextKey{}

const (
	// LogLevelDebug debug logging level
	LogLevelDebug LogLevel = 1 + iota

	// LogLevelInfo info logging level
	LogLevelInfo

	// LogLevelWarn warnning level
	LogLevelWarn

	// LogLevelError error level
	LogLevelError
)

const (
	// CorrelationIDHeader http header name for correlation id header
	CorrelationIDHeader = "x-correlation-id"
)

// NewRollingLogWriter returns a new file based rolling log writer
// maxSizeInMegaBytes specifies the maximum size of each log file, while maxRotationFiles
// tells the maximum files to keep
func NewRollingLogWriter(fileName string, maxSizeInMegaBytes, maxRotationFiles int) io.Writer {
	return &lumberjack.Logger{
		Filename:   fileName,
		MaxSize:    maxSizeInMegaBytes,
		MaxBackups: maxRotationFiles,
		Compress:   false,
	}
}

// SetupLogger setup the global logger with specified log writer and log level
// this must be called before the logging can actually work
func SetupLogger(level LogLevel, writer io.Writer) {
	logLevel := zap.DebugLevel
	switch level {
	case LogLevelInfo:
		logLevel = zap.InfoLevel
		break
	case LogLevelWarn:
		logLevel = zap.WarnLevel
		break
	case LogLevelError:
		logLevel = zap.ErrorLevel
	}

	jsonEncoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	writeSyncer := zapcore.AddSync(writer)
	zapCore := zapcore.NewCore(jsonEncoder, writeSyncer, logLevel)
	logger := zap.New(zapCore)
	zap.ReplaceGlobals(logger)
}

// LoggingHandler http incoming logging handler
func LoggingHandler(handler http.Handler) http.Handler {
	handlerFunction := func(writer http.ResponseWriter, request *http.Request) {
		var correlationID string
		var activityID string
		timeNow := time.Now()
		headerValues, ok := request.Header[CorrelationIDHeader]
		if ok && len(headerValues) > 0 {
			correlationID = headerValues[0]
		} else {
			uuid, err := uuid.NewV4()
			if err != nil {
				correlationID = "no-correlation-id"
			} else {
				correlationID = uuid.String()
			}
		}

		uuid, err := uuid.NewV4()
		if err != nil {
			activityID = uuid.String()
		} else {
			activityID = "no-activity-id"
		}

		request.WithContext(context.WithValue(request.Context(), TraceActivityIDKey, activityID))
		handler.ServeHTTP(writer, request)

		elapsed := time.Since(timeNow)
		zap.L().Info(fmt.Sprintf("request %s served", request.URL),
			zap.String("correlationId", correlationID),
			zap.String("activityId", activityID),
			zap.String("requestUri", request.URL.String()),
			zap.String("requestMethod", request.Method),
			zap.Int("responseStatus", request.Response.StatusCode),
			zap.Int("latency", int(elapsed.Nanoseconds()/1000)))
	}

	return http.HandlerFunc(handlerFunction)
}
