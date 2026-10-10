//go:build linux

package engines

import (
	"testing"

	"github.com/Rake-Pro/GoShareIt/internal/core/ocr/tesseract"
)

func TestLinuxUsesTesseract(t *testing.T) {
	e, ok := New(Config{TesseractPath: "/opt/tesseract", Langs: []string{"de"}}).(*tesseract.Engine)
	if !ok || e.Path != "/opt/tesseract" || len(e.Langs) != 1 {
		t.Fatalf("New = %#v", e)
	}
}
