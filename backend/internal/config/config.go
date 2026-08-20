package config

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	Addr               string
	DatabaseDriver     string
	Database           string
	PublicStaticDir    string
	AdminStaticDir     string
	AgentStaticDir     string
	AdminUsername      string
	AdminPassword      string
	AdminName          string
	CardPepper         []byte
	DataKey            []byte
	TrustProxyHeaders  bool
	DatabaseConfigFile string
	AdminExplicit      bool
}

func Load() (Config, error) {
	cfg := Config{
		Addr:               env("YN_ADDR", ":8080"),
		DatabaseDriver:     env("YN_DB_DRIVER", "sqlite"),
		Database:           env("YN_DB", "file:yn-license-dev.db?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"),
		PublicStaticDir:    env("YN_PUBLIC_STATIC_DIR", "../frontend/dist"),
		AdminStaticDir:     env("YN_ADMIN_STATIC_DIR", "../frontend/dist/admin-console"),
		AgentStaticDir:     env("YN_AGENT_STATIC_DIR", "../frontend/dist/agent-console"),
		AdminUsername:      env("YN_ADMIN_USERNAME", "admin"),
		AdminPassword:      os.Getenv("YN_ADMIN_PASSWORD"),
		AdminName:          env("YN_ADMIN_NAME", "系统管理员"),
		CardPepper:         []byte(env("YN_CARD_PEPPER", "dev-card-pepper-change-me")),
		TrustProxyHeaders:  envBool("YN_TRUST_PROXY_HEADERS", false),
		DatabaseConfigFile: env("YN_DB_CONFIG_FILE", "yn-license-config.json"),
		AdminExplicit:      os.Getenv("YN_ADMIN_USERNAME") != "" && os.Getenv("YN_ADMIN_PASSWORD") != "",
	}
	if fileCfg, err := readDatabaseConfig(cfg.DatabaseConfigFile); err != nil {
		return Config{}, err
	} else if fileCfg.Driver != "" && os.Getenv("YN_DB_DRIVER") == "" && os.Getenv("YN_DB") == "" {
		cfg.DatabaseDriver, cfg.Database = fileCfg.Driver, fileCfg.DSN
	}

	keyText := env("YN_DATA_KEY", "dev-data-key-change-me")
	key, err := parseKey(keyText)
	if err != nil {
		return Config{}, err
	}
	cfg.DataKey = key
	return cfg, nil
}

type DatabaseConfig struct {
	Driver string `json:"driver"`
	DSN    string `json:"dsn"`
}

func readDatabaseConfig(path string) (DatabaseConfig, error) {
	if path == "" {
		return DatabaseConfig{}, nil
	}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return DatabaseConfig{}, nil
	}
	if err != nil {
		return DatabaseConfig{}, err
	}
	var cfg DatabaseConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return DatabaseConfig{}, errors.New("invalid database config file")
	}
	return cfg, nil
}

func SaveDatabaseConfig(path string, cfg DatabaseConfig) error {
	if path == "" {
		return nil
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
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
