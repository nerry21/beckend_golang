package config

import (
	"os"
	"strings"
)

type Env struct {
	AppAddr string
	GinMode string
}

func LoadEnv() Env {
	appAddr := strings.TrimSpace(os.Getenv("APP_ADDR"))
	if appAddr == "" {
		appAddr = ":8080"
	}

	ginMode := strings.TrimSpace(os.Getenv("GIN_MODE"))

	return Env{
		AppAddr: appAddr,
		GinMode: ginMode,
	}
}
