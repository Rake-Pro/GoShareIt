// Package engines picks the platform OCR engine. One file per GOOS; this is
// the only place that names a platform engine. The editor, the settings
// service and the host all build their engine here.
package engines

// Config is the subset of config.OCRConfig an engine needs.
type Config struct {
	Langs         []string // BCP-47 tags; empty = engine default
	TesseractPath string   // Linux: "" = PATH lookup
}
