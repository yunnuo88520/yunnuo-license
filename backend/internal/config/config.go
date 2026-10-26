package config

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type Config struct {
	Addr                 string
	DatabaseDriver       string
	Database             string
	ServeFrontend        bool
	PublicStaticDir      string
	AdminStaticDir       string
	AgentStaticDir       string
	CardPepper           []byte
	DataKey              []byte
	TrustProxyHeaders    bool
	TrustedProxyNetworks []netip.Prefix
	AdminNetworks        []netip.Prefix
	DatabaseConfigFile   string
	AdminPath            string
	ReleaseDir           string
}

func Load() (Config, error) {
	cfg := Config{
		Addr:               env("YN_ADDR", ":8080"),
		DatabaseDriver:     env("YN_DB_DRIVER", "sqlite"),
		Database:           env("YN_DB", "file:yn-license-dev.db?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"),
		ServeFrontend:      envBool("YN_SERVE_FRONTEND", true),
		PublicStaticDir:    env("YN_PUBLIC_STATIC_DIR", "../frontend/dist"),
		AdminStaticDir:     env("YN_ADMIN_STATIC_DIR", "../frontend/dist/admin-console"),
		AgentStaticDir:     env("YN_AGENT_STATIC_DIR", "../frontend/dist/agent-console"),
		CardPepper:         []byte(env("YN_CARD_PEPPER", "dev-card-pepper-change-me")),
		TrustProxyHeaders:  envBool("YN_TRUST_PROXY_HEADERS", false),
		DatabaseConfigFile: env("YN_DB_CONFIG_FILE", "yn-license-config.json"),
		ReleaseDir:         env("YN_RELEASE_DIR", "yn-releases"),
	}
	fileCfg, err := readDatabaseConfig(cfg.DatabaseConfigFile)
	if err != nil {
		return Config{}, err
	}
	cfg.TrustedProxyNetworks, err = ParseNetworks(env("YN_TRUSTED_PROXY_CIDRS", "127.0.0.1/32,::1/128"))
	if err != nil {
		return Config{}, err
	}
	cfg.AdminNetworks, err = ParseNetworks(env("YN_ADMIN_ALLOWED_CIDRS", ""))
	if err != nil {
		return Config{}, err
	}
	if fileCfg.Driver != "" {
		cfg.DatabaseDriver, cfg.Database = fileCfg.Driver, fileCfg.DSN
	}
	cfg.AdminPath, err = NormalizeAdminPath(fileCfg.AdminPath)
	if err != nil {
		return Config{}, err
	}
	if os.Getenv("YN_CARD_PEPPER") == "" && fileCfg.CardPepper != "" {
		cfg.CardPepper = []byte(fileCfg.CardPepper)
	}
	keyText := env("YN_DATA_KEY", fileCfg.DataKey)
	if keyText == "" {
		keyText = "dev-data-key-change-me"
	}
	key, err := parseKey(keyText)
	if err != nil {
		return Config{}, err
	}
	cfg.DataKey = key
	return cfg, nil
}

func ParseNetworks(value string) ([]netip.Prefix, error) {
	out := []netip.Prefix{}
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(part)
		if err != nil {
			return nil, errors.New("network allowlist must contain valid CIDR networks")
		}
		out = append(out, prefix.Masked())
	}
	return out, nil
}

type DatabaseConfig struct {
	Driver     string `json:"driver"`
	DSN        string `json:"dsn"`
	AdminPath  string `json:"admin_path,omitempty"`
	CardPepper string `json:"card_pepper,omitempty"`
	DataKey    string `json:"data_key,omitempty"`
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
	previous, err := readDatabaseConfig(path)
	if err != nil {
		return err
	}
	if cfg.AdminPath == "" {
		cfg.AdminPath = previous.AdminPath
	}
	if cfg.AdminPath == "" {
		cfg.AdminPath = "/admin"
	}
	if cfg.CardPepper == "" {
		cfg.CardPepper = previous.CardPepper
	}
	if cfg.DataKey == "" {
		cfg.DataKey = previous.DataKey
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
	tmp, err := os.CreateTemp(filepath.Dir(path), ".yunnuo-config-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
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

func randomKey() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func NormalizeAdminPath(value string) (string, error) {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if value == "" {
		return "/admin", nil
	}
	if !regexp.MustCompile(`^/[A-Za-z0-9_-]+(/[A-Za-z0-9_-]+)*$`).MatchString(value) {
		return "", errors.New("invalid admin_path in configuration")
	}
	for _, prefix := range []string{"/install", "/recover", "/assets", "/v1", "/agent", "/user", "/developer", "/agent-console", "/user-console", "/developer-console", "/admin-console", "/healthz"} {
		if value == prefix || strings.HasPrefix(value, prefix+"/") {
			return "", errors.New("admin_path conflicts with a reserved route")
		}
	}
	if strings.HasPrefix(value, "/admin/") {
		return "", errors.New("admin_path conflicts with the management API")
	}
	return value, nil
}

// Preserve legacy keys for installed databases; new installations get random keys.
func (cfg *Config) PrepareInstallationKeys(initialized bool) error {
	f, err := readDatabaseConfig(cfg.DatabaseConfigFile)
	if err != nil {
		return err
	}
	changed := false
	if os.Getenv("YN_CARD_PEPPER") == "" && f.CardPepper == "" {
		f.CardPepper = string(cfg.CardPepper)
		if !initialized {
			f.CardPepper = randomKey()
		}
		cfg.CardPepper = []byte(f.CardPepper)
		changed = true
	}
	if os.Getenv("YN_DATA_KEY") == "" && f.DataKey == "" {
		f.DataKey = hex.EncodeToString(cfg.DataKey)
		if !initialized {
			f.DataKey = randomKey()
		}
		cfg.DataKey, err = parseKey(f.DataKey)
		if err != nil {
			return err
		}
		changed = true
	}
	if changed {
		return SaveDatabaseConfig(cfg.DatabaseConfigFile, f)
	}
	return nil
}
