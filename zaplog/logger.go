package zaplog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/uptrace/opentelemetry-go-extra/otelzap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type loggerCtxKey struct{}

func init() {
	if err := zap.RegisterEncoder("console2", func(encoderConfig zapcore.EncoderConfig) (zapcore.Encoder, error) {
		encoderConfig.EncodeName = func(loggerName string, encoder zapcore.PrimitiveArrayEncoder) {
			encoder.AppendString("[" + loggerName + "]")
		}
		return zapcore.NewConsoleEncoder(encoderConfig), nil
	}); err != nil {
		panic(err)
	}
}

type Logger otelzap.Logger

func L() *Logger {
	return (*Logger)(otelzap.L())
}

func ReplaceGlobals(logger *Logger) {
	zap.ReplaceGlobals(logger.Logger)
	otelzap.ReplaceGlobals((*otelzap.Logger)(logger))
}

func Configure() *Logger {
	cfg := zap.NewDevelopmentConfig()
	cfg.Encoding = "console2"
	zapLogger, err := cfg.Build(
		zap.WithCaller(false),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
	if err != nil {
		panic(err)
	}
	logger := (*Logger)(otelzap.New(zapLogger))
	ReplaceGlobals(logger)
	return logger
}

func (l *Logger) Named(s string) *Logger {
	return l.cloneWith(l.Logger.Named(s))
}

func (l *Logger) Ctx(ctx context.Context) otelzap.LoggerWithCtx {
	return (*otelzap.Logger)(l).Ctx(ctx)
}

func (l *Logger) With(fields ...zapcore.Field) *Logger {
	return l.cloneWith(l.Logger.With(fields...))
}

func (l *Logger) cloneWith(logger *zap.Logger) *Logger {
	c := *l
	c.Logger = logger
	return &c
}

func (l *Logger) LogCallTime(name string, fields ...zapcore.Field) func(fp ...func() []zapcore.Field) {
	st := time.Now()
	return func(fp ...func() []zapcore.Field) {
		f := make([]zapcore.Field, 0, len(fields)+1)
		f = append(f, zap.Duration("time", time.Since(st)))
		f = append(f, fields...)
		for _, p := range fp {
			f = append(f, p()...)
		}
		l.Info("call "+name, f...)
	}
}

func (l *Logger) Measure(message string, fields ...zapcore.Field) func() {
	l.Info(message+"...", fields...)
	st := time.Now()
	return func() {
		l.Info(fmt.Sprintf("%s - completed in %v", message, time.Since(st)), fields...)
	}
}

func (l *Logger) MeasureBlock(message string, fn func() error, fields ...zapcore.Field) error {
	defer l.Measure(message, fields...)()
	return fn()
}

func FromContext(ctx context.Context) *Logger {
	val := ctx.Value(loggerCtxKey{})
	if val == nil {
		return L()
	}
	return val.(*Logger)
}

func WithContext(ctx context.Context, logger *Logger) context.Context {
	return context.WithValue(ctx, loggerCtxKey{}, logger)
}

func Flush() {
	err := L().Sync()
	if err == nil {
		return
	}
	var perr *os.PathError
	if errors.As(err, &perr) && perr.Op == "sync" && perr.Path == "/dev/stderr" {
		return
	}
	_, _ = fmt.Fprintf(os.Stderr, "failed to flush logger: %v", err)
}

func Recover() {
	defer Flush()
	if r := recover(); r != nil {
		L().Panic("panic", zap.Any("panic", r))
	}
}

func (l *Logger) Fields() *LoggerFields {
	return &LoggerFields{}
}

type LoggerFields []any

func (f LoggerFields) With(key string, val any) LoggerFields {
	return append(f, key, val)
}
