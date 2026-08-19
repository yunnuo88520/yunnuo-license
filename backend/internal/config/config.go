package config

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"strconv"
)

type Config struct {
	Addr              string
	DatabaseDriver    string
	Database          string
	PublicStaticDir   string
	AdminStaticDir    string
	AgentStaticDir    string
	AdminUsername     string
	AdminPassword     string
	AdminName         string
	CardPepper        []byte
	DataKey           []byte
	TrustProxyHeaders bool
}

func Load() (Config, error) {
	cfg := Config{
		Addr:              env("YN_ADDR", ":8080"),
		DatabaseDriver:    env("YN_DB_DRIVER", "sqlite"),
		Database:          env("YN_DB", "file:yn-license-dev.db?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"),
		PublicStaticDir:   env("YN_PUBLIC_STATIC_DIR", "../frontend/public"),
		AdminStaticDir:    env("YN_ADMIN_STATIC_DIR", "../frontend/admin"),
		AgentStaticDir:    env("YN_AGENT_STATIC_DIR", "../frontend/agent"),
		AdminUsername:     env("YN_ADMIN_USERNAME", "admin"),
		AdminPassword:     env("YN_ADMIN_PASSWORD", "admin123"),
		AdminName:         env("YN_ADMIN_NAME", "系统管理员"),
		CardPepper:        []byte(env("YN_CARD_PEPPER", "dev-card-pepper-change-me")),
		TrustProxyHeaders: envBool("YN_TRUST_PROXY_HEADERS", false),
	}

	keyText := env("YN_DATA_KEY", "dev-data-key-change-me")
	key, err := parseKey(keyText)
	if err != nil {
		return Config{}, err
	}
	cfg.DataKey = key
	return cfg, nil
}

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func parseKey(value string) ([]byte, error) {
	if len(value) == 64 {
		if raw, err := hex.DecodeString(value); err == nil && len(raw) == 32 {
			return raw, nil
		}
	}
	if raw, err := base64.StdEncoding.DecodeString(value); err == nil && len(raw) == 32 {
		return raw, nil
	}
	if raw, err := base64.RawStdEncoding.DecodeString(value); err == nil && len(raw) == 32 {
		return raw, nil
	}
	if value == "" {
		return nil, errors.New("empty data key")
	}
	sum := sha256.Sum256([]byte(value))
	return sum[:], nil
}

func EnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
