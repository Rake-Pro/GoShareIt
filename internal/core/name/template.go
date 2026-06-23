// Package name renders ShareX-style filename templates.
package name

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"
)

// DefaultTemplate is used when no template is configured.
const DefaultTemplate = "goshareit_{datetime}_{rand}.{ext}"

// nowFunc and randFunc are overridable in tests.
var (
	nowFunc  = time.Now
	randFunc = randomToken
)

// Render expands the supported tokens in tmpl:
//
//	{date}     -> 2006-01-02
//	{time}     -> 15-04-05
//	{datetime} -> 2006-01-02_15-04-05
//	{rand}     -> 6 random hex chars
//	{ext}      -> the provided extension (without a leading dot)
//
// ext may be passed with or without a leading dot.
func Render(tmpl, ext string) string {
	if tmpl == "" {
		tmpl = DefaultTemplate
	}
	now := nowFunc()
	ext = strings.TrimPrefix(ext, ".")

	r := strings.NewReplacer(
		"{datetime}", now.Format("2006-01-02_15-04-05"),
		"{date}", now.Format("2006-01-02"),
		"{time}", now.Format("15-04-05"),
		"{rand}", randFunc(),
		"{ext}", ext,
	)
	return r.Replace(tmpl)
}

func randomToken() string {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		// rand.Read on a healthy system does not fail; fall back to a stamp.
		return nowFunc().Format("150405")
	}
	return hex.EncodeToString(b)
}
