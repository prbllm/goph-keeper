package main

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func newLogger(level string) (*zap.Logger, error) {
	var lvl zapcore.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		return nil, fmt.Errorf("parse log level: %w", err)
	}
	zcfg := zap.NewProductionConfig()
	zcfg.Level = zap.NewAtomicLevelAt(lvl)
	return zcfg.Build()
}
