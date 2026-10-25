// Package integration produces self-contained, runnable SDK starter projects.
package integration

import (
	"archive/zip"
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

//go:embed templates/*
var templates embed.FS

type Config struct {
	ProductID   string         `json:"product_id"`
	BaseURL     string         `json:"base_url"`
	AppKey      string         `json:"app_key"`
	ProductCode string         `json:"product_code"`
	BindMode    string         `json:"bind_mode"`
	BindValue   string         `json:"bind_value"`
	PublicKeys  map[int]string `json:"public_keys"`
}

func Validate(c Config, language string) error {
	u, err := url.Parse(c.BaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.ForceQuery || u.RawQuery != "" || u.Fragment != "" || u.EscapedPath() != "" && u.EscapedPath() != "/" {
		return errors.New("base_url must be the HTTP(S) origin of the license platform")
	}
	if language != "node" && language != "go" && language != "java" {
		return errors.New("unsupported integration language")
	}
	if len(c.BindValue) > 500 || strings.ContainsAny(c.BindValue, "\x00\r\n") {
		return errors.New("invalid binding value")
	}
	if c.BindMode != "none" && strings.TrimSpace(c.BindValue) == "" {
		return errors.New("binding value is required")
	}
	return nil
}

func Build(sdkDir, language string, c Config) ([]byte, error) {
	if err := Validate(c, language); err != nil {
		return nil, err
	}
	origin, _ := url.Parse(c.BaseURL)
	c.BaseURL = origin.Scheme + "://" + origin.Host
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	add := func(name string, content []byte) error {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.SetMode(0600)
		entry, err := w.CreateHeader(h)
		if err != nil {
			return err
		}
		_, err = entry.Write(content)
		return err
	}
	copyFile := func(source, name string) error {
		content, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		return add(name, content)
	}
	config, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, err
	}
	if err = add("config.json", append(config, '\n')); err != nil {
		return nil, err
	}
	if err = add(".gitignore", []byte(".license*\nactivate.json\nverify.json\nupdate.zip\ntarget/\n")); err != nil {
		return nil, err
	}
	if err = copyFile(filepath.Join(sdkDir, "..", "LICENSE"), "LICENSE"); err != nil {
		return nil, err
	}
	if err = copyFile(filepath.Join(sdkDir, language, "README.md"), "SDK-README.md"); err != nil {
		return nil, err
	}
	root, extension, destination := filepath.Join(sdkDir, language), ".go", "sdk/"
	mainSource, mainTarget := "go.txt", "main.go"
	switch language {
	case "node":
		root, extension = filepath.Join(root, "src"), ".js"
		destination = "sdk/src/"
		mainSource, mainTarget = "node.txt", "app.mjs"
		if err = copyFile(filepath.Join(sdkDir, "node", "package.json"), "sdk/package.json"); err != nil {
			return nil, err
		}
	case "go":
		if err = copyFile(filepath.Join(root, "go.mod"), "sdk/go.mod"); err != nil {
			return nil, err
		}
		mod := "module yunnuo-integration-demo\n\ngo 1.22\n\nrequire github.com/yunnuo88520/yunnuo-license/sdk/go v0.0.0\n\nreplace github.com/yunnuo88520/yunnuo-license/sdk/go => ./sdk\n"
		if err = add("go.mod", []byte(mod)); err != nil {
			return nil, err
		}
	case "java":
		root, extension, destination = filepath.Join(root, "src", "main", "java"), ".java", "src/main/java/"
		mainSource, mainTarget = "java.txt", "src/main/java/demo/Main.java"
		pom, e := os.ReadFile(filepath.Join(sdkDir, "java", "pom.xml"))
		if e != nil {
			return nil, e
		}
		plugin := `<plugins><plugin><groupId>org.codehaus.mojo</groupId><artifactId>exec-maven-plugin</artifactId><version>3.5.1</version><configuration><mainClass>demo.Main</mainClass></configuration></plugin>`
		if err = add("pom.xml", []byte(strings.Replace(string(pom), "<plugins>", plugin, 1))); err != nil {
			return nil, err
		}
	}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("SDK source must not be a symlink")
		}
		if (!strings.HasSuffix(path, extension) && !(language == "node" && strings.HasSuffix(path, ".d.ts"))) || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		return copyFile(path, destination+filepath.ToSlash(rel))
	})
	if err != nil {
		return nil, err
	}
	main, err := templates.ReadFile("templates/" + mainSource)
	if err != nil {
		return nil, err
	}
	if err = add(mainTarget, main); err != nil {
		return nil, err
	}
	readme, err := templates.ReadFile("templates/README.md")
	if err != nil {
		return nil, err
	}
	for _, args := range []string{"activate CARD_CODE", "verify", "heartbeat", "serve", "attach LICENSE_NO", "renew RENEW_CARD_CODE"} {
		command := map[string]string{"node": "node app.mjs " + args, "go": "go run . " + args, "java": "mvn -q compile exec:java -Dexec.args=\"" + args + "\""}[language]
		readme = []byte(strings.ReplaceAll(string(readme), "{{COMMAND}} "+args, command))
	}
	if err = add("README.md", readme); err != nil {
		return nil, err
	}
	if err = w.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
