package integration

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

func TestKitsContainLocalSDKAndMatchingConfiguration(t *testing.T) {
	cfg := Config{ProductID: "prod_demo", BaseURL: "https://license.example.com/", AppKey: "app_demo", ProductCode: "EBOX", BindMode: "domain", BindValue: "ebox.example.com", PublicKeys: map[int]string{1: "public-key"}}
	for _, language := range []string{"node", "go", "java"} {
		t.Run(language, func(t *testing.T) {
			content, err := Build(filepath.Join("..", "..", "..", "sdk"), language, cfg)
			if err != nil {
				t.Fatal(err)
			}
			archive, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
			if err != nil {
				t.Fatal(err)
			}
			files := map[string][]byte{}
			for _, entry := range archive.File {
				if path.IsAbs(entry.Name) || strings.Contains(entry.Name, "../") {
					t.Fatal("unsafe path", entry.Name)
				}
				stream, e := entry.Open()
				if e != nil {
					t.Fatal(e)
				}
				data, e := io.ReadAll(stream)
				stream.Close()
				if e != nil {
					t.Fatal(e)
				}
				files[entry.Name] = data
			}
			var got Config
			if err = json.Unmarshal(files["config.json"], &got); err != nil {
				t.Fatal(err)
			}
			if got.AppKey != cfg.AppKey || got.ProductID != cfg.ProductID || got.BaseURL != "https://license.example.com" || got.PublicKeys[1] != cfg.PublicKeys[1] {
				t.Fatal("configuration does not match product")
			}
			if strings.Contains(string(files["README.md"]), "{{COMMAND}}") || len(files["LICENSE"]) == 0 {
				t.Fatal("instructions or license missing")
			}
			required := map[string][]string{"node": {"app.mjs", "sdk/src/index.js", "sdk/src/index.d.ts", "sdk/package.json"}, "go": {"main.go", "go.mod", "sdk/go.mod", "sdk/client.go"}, "java": {"pom.xml", "src/main/java/demo/Main.java", "src/main/java/com/yunnuo/license/LicenseClient.java"}}
			for _, name := range required[language] {
				if len(files[name]) == 0 {
					t.Fatal("missing project file", name)
				}
			}
		})
	}
}

func TestConfigurationRejectsUnsafeOrIncompleteOrigins(t *testing.T) {
	cfg := Config{BaseURL: "https://license.example.com", BindMode: "domain", BindValue: "ebox.example.com"}
	for _, value := range []string{"file:///etc/passwd", "javascript:alert(1)", "https://user:password@license.example.com", "https://license.example.com/admin", "https://license.example.com?secret=value", "https://license.example.com?", "https://license.example.com/%2F", "https://license.example.com/#fragment"} {
		c := cfg
		c.BaseURL = value
		if Validate(c, "node") == nil {
			t.Fatal("accepted invalid origin", value)
		}
	}
	if Validate(cfg, "../../secret") == nil {
		t.Fatal("accepted invalid language")
	}
	cfg.BindValue = ""
	if err := Validate(cfg, "go"); err != nil {
		t.Fatal("standard product kits must allow runtime activation input", err)
	}
	cfg.BindMode = "none"
	if err := Validate(cfg, "go"); err != nil {
		t.Fatal(err)
	}
}
