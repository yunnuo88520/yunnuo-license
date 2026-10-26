package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func (cfg Config) SetupTokenPath() string { return cfg.DatabaseConfigFile + ".install-token" }
func (cfg Config) PrepareSetupToken(initialized bool) (string, error) {
	if initialized {
		return "", nil
	}
	if token := strings.TrimSpace(os.Getenv("YN_SETUP_TOKEN")); token != "" {
		if len(token) < 32 || len(token) > 256 {
			return "", errors.New("YN_SETUP_TOKEN must contain 32 to 256 characters")
		}
		return token, nil
	}
	path := cfg.SetupTokenPath()
	if cfg.DatabaseConfigFile == "" {
		return "", errors.New("a private configuration path is required for the installation token")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err == nil {
		token := randomKey()
		_, err = f.WriteString(token + "\n")
		closeErr := f.Close()
		if err != nil {
			return "", err
		}
		return token, closeErr
	}
	if !os.IsExist(err) {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.Mode().Perm()&0077 != 0 {
		return "", errors.New("installation token file permissions must be 0600")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(raw))
	if len(token) < 32 || len(token) > 256 {
		return "", errors.New("installation token file is invalid")
	}
	return token, nil
}
