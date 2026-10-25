package captcha

import (
	"bytes"
	"image/png"
	"strings"
	"testing"
)

func TestNewCodeShape(t *testing.T) {
	code, err := NewCode()
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != codeLen {
		t.Fatalf("code length = %d, want %d", len(code), codeLen)
	}
	for _, c := range code {
		if !strings.ContainsRune(Chars, c) {
			t.Fatalf("code contains unsupported char %q", c)
		}
	}
}

func TestRenderPNG(t *testing.T) {
	code, err := NewCode()
	if err != nil {
		t.Fatal(err)
	}
	pngBytes, err := Render(code)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	if img.Bounds().Dx() != canvasW || img.Bounds().Dy() != canvasH {
		t.Fatalf("unexpected size %v", img.Bounds())
	}
	// 背景不是纯色（有噪点/字形）。
	seen := map[struct{ r, g, b, a uint32 }]bool{}
	for x := 0; x < canvasW; x += 3 {
		for y := 0; y < canvasH; y += 3 {
			r, g, b, a := img.At(x, y).RGBA()
			seen[struct{ r, g, b, a uint32 }{r, g, b, a}] = true
		}
	}
	if len(seen) < 3 {
		t.Fatal("image appears to be blank")
	}
}

func TestRenderRejectsBadLength(t *testing.T) {
	if _, err := Render("ABC"); err == nil {
		t.Fatal("expected error for 3-char code")
	}
}
