// Package tesseract is the Linux OCR engine: it runs the optional tesseract
// command as a local subprocess over stdin/stdout (no temp files, no network)
// after a pure-Go preprocessing pass. It has no build tag, so it compiles and
// is unit-tested everywhere, but only the Linux factory uses it.
package tesseract

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Rake-Pro/GoShareIt/internal/core/ocr"
)

// probeCacheTTL bounds how long a full probe result is reused while the
// command still resolves to the same path.
const probeCacheTTL = 60 * time.Second

// Install hint shown when the command is missing.
const installHint = "Install it with your package manager: sudo apt install tesseract-ocr tesseract-ocr-eng (Debian/Ubuntu) or sudo dnf install tesseract tesseract-langpack-eng (Fedora)."

// Engine runs the tesseract CLI. The zero value searches PATH for
// "tesseract" and uses the default language choice.
type Engine struct {
	Path  string   // command name or path; "" = "tesseract" on PATH
	Langs []string // BCP-47 tags to prefer; empty = eng, else the first installed

	mu       sync.Mutex
	cached   ocr.Status
	cachedAt time.Time
	exe      string   // resolved path the cache belongs to
	codes    []string // installed tesseract language codes (no osd)
}

// Probe reports whether a usable tesseract (4.0 or newer, with at least one
// language) is installed. Every call resolves the command with LookPath; the
// --version/--list-langs subprocesses run again only when the resolved path
// changed or the cached result is older than 60 s, so a user who just
// installed tesseract gets the tool on the next probe.
func (e *Engine) Probe(ctx context.Context) ocr.Status {
	name := e.Path
	if name == "" {
		name = "tesseract"
	}
	exe, err := exec.LookPath(name)
	e.mu.Lock()
	defer e.mu.Unlock()
	if err != nil {
		e.exe, e.codes = "", nil
		e.cached = ocr.Status{Reason: "Tesseract is not installed.", Hint: installHint}
		e.cachedAt = time.Now()
		return e.cached
	}
	if exe == e.exe && time.Since(e.cachedAt) < probeCacheTTL {
		return e.cached
	}
	st, codes := probe(ctx, exe)
	if ctx.Err() != nil {
		// Cut short by the caller's deadline (a cold first start can take
		// seconds): report it, but do not remember it, so the next probe
		// tries again instead of repeating a timeout for 60 s.
		return ocr.Status{Reason: "Tesseract did not answer in time.", Hint: "Try again; if it keeps happening, check that tesseract --version runs."}
	}
	e.exe, e.codes, e.cached, e.cachedAt = exe, codes, st, time.Now()
	return st
}

func probe(ctx context.Context, exe string) (ocr.Status, []string) {
	out, err := probeCmd(ctx, exe, "--version").CombinedOutput()
	if err != nil && len(out) == 0 {
		return ocr.Status{Reason: "Tesseract did not start: " + firstLine(err.Error()) + ".", Hint: installHint}, nil
	}
	ver, major, ok := parseVersion(string(out))
	if !ok {
		return ocr.Status{Reason: "Tesseract did not report its version.", Hint: installHint}, nil
	}
	if major < 4 {
		return ocr.Status{Reason: "Tesseract " + ver + " is too old; version 4.0 or newer is required.", Hint: installHint}, nil
	}
	out, err = probeCmd(ctx, exe, "--list-langs").CombinedOutput()
	if err != nil && len(out) == 0 {
		return ocr.Status{Reason: "Tesseract could not list its languages.", Hint: installHint}, nil
	}
	codes := parseLangs(string(out))
	if len(codes) == 0 {
		return ocr.Status{Reason: "Tesseract has no language data.", Hint: "Install a language pack, for example tesseract-ocr-eng."}, nil
	}
	langs := make([]string, len(codes))
	for i, c := range codes {
		langs[i] = toBCP47(c)
	}
	return ocr.Status{Available: true, Engine: ocr.EngineTesseract, Version: ver, Langs: langs}, codes
}

// probeCmd builds a probe subprocess that is killed with ctx and reaped
// within 2 s even if a child keeps its output pipe open.
func probeCmd(ctx context.Context, exe, arg string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, exe, arg)
	cmd.WaitDelay = 2 * time.Second
	return cmd
}

var reVersion = regexp.MustCompile(`(?i)tesseract\s+v?(\d+)\.(\d+)(?:\.(\d+))?`)

// parseVersion finds "tesseract 5.3.4" in --version output.
func parseVersion(out string) (ver string, major int, ok bool) {
	m := reVersion.FindStringSubmatch(out)
	if m == nil {
		return "", 0, false
	}
	major, _ = strconv.Atoi(m[1])
	ver = m[1] + "." + m[2]
	if m[3] != "" {
		ver += "." + m[3]
	}
	return ver, major, true
}

// parseLangs reads --list-langs output: a header line, then one code per
// line. osd (orientation detection) is not a recognition language.
func parseLangs(out string) []string {
	var codes []string
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || l == "osd" || strings.Contains(l, " ") || strings.HasSuffix(l, ":") {
			continue
		}
		codes = append(codes, l)
	}
	return codes
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// Recognize preprocesses img, runs tesseract over stdin/stdout and parses its
// TSV. ctx cancellation kills the process.
func (e *Engine) Recognize(ctx context.Context, img image.Image, opts ocr.Options) (ocr.Result, error) {
	st := e.Probe(ctx)
	if !st.Available {
		return ocr.Result{}, ocr.ErrUnavailable
	}
	e.mu.Lock()
	exe, installed := e.exe, e.codes
	e.mu.Unlock()

	want := opts.Langs
	if len(want) == 0 {
		want = e.Langs
	}
	codes := pickLangs(want, installed)

	pngBytes, tf, err := preprocess(img)
	if err != nil {
		return ocr.Result{}, fmt.Errorf("tesseract: preprocess: %w", err)
	}
	args := []string{"stdin", "stdout", "-l", strings.Join(codes, "+"), "--psm", "3", "--dpi", "300", "-c", "preserve_interword_spaces=1", "tsv"}
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Stdin = bytes.NewReader(pngBytes)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ocr.Result{}, ctx.Err()
		}
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return ocr.Result{}, fmt.Errorf("tesseract: %w: %s", err, firstLine(msg))
		}
		return ocr.Result{}, fmt.Errorf("tesseract: %w", err)
	}
	lines, err := parseTSV(stdout.Bytes(), tf, img.Bounds())
	if err != nil {
		return ocr.Result{}, err
	}
	langs := make([]string, len(codes))
	for i, c := range codes {
		langs[i] = toBCP47(c)
	}
	return ocr.Result{Engine: ocr.EngineTesseract, Langs: langs, Lines: lines}, nil
}

// pickLangs maps the wanted BCP-47 tags to installed tesseract codes. With
// none wanted (or none installed) it uses eng when installed, else the first
// installed code.
func pickLangs(want, installed []string) []string {
	has := make(map[string]bool, len(installed))
	for _, c := range installed {
		has[c] = true
	}
	var out []string
	for _, tag := range want {
		if c := toTesseract(tag); has[c] && !contains(out, c) {
			out = append(out, c)
		}
	}
	if len(out) > 0 {
		return out
	}
	if has["eng"] || len(installed) == 0 {
		return []string{"eng"}
	}
	return []string{installed[0]}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// langTable maps tesseract codes to BCP-47 tags. Unknown codes pass through.
var langTable = map[string]string{
	"eng": "en", "deu": "de", "fra": "fr", "spa": "es", "ita": "it", "por": "pt",
	"nld": "nl", "pol": "pl", "rus": "ru", "ukr": "uk", "ces": "cs", "dan": "da",
	"fin": "fi", "nor": "no", "swe": "sv", "tur": "tr", "ell": "el", "hun": "hu",
	"ron": "ro", "jpn": "ja", "kor": "ko", "chi_sim": "zh-Hans", "chi_tra": "zh-Hant",
	"ara": "ar", "heb": "he", "hin": "hi", "tha": "th", "vie": "vi", "ind": "id",
}

func toBCP47(code string) string {
	if t, ok := langTable[code]; ok {
		return t
	}
	return code
}

// toTesseract maps a BCP-47 tag to a tesseract code: an exact table match
// first, then the primary subtag ("en-US" -> "eng"), then the tag itself.
func toTesseract(tag string) string {
	tag = strings.TrimSpace(tag)
	for code, t := range langTable {
		if strings.EqualFold(t, tag) {
			return code
		}
	}
	primary, _, _ := strings.Cut(tag, "-")
	if strings.EqualFold(primary, "zh") {
		if strings.Contains(strings.ToLower(tag), "hant") || strings.HasSuffix(strings.ToUpper(tag), "-TW") || strings.HasSuffix(strings.ToUpper(tag), "-HK") {
			return "chi_tra"
		}
		return "chi_sim"
	}
	for code, t := range langTable {
		if strings.EqualFold(t, primary) {
			return code
		}
	}
	return tag
}

// errTSV marks output that is not tesseract TSV.
var errTSV = errors.New("tesseract: unexpected TSV output")
