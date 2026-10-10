// Package ocrtest provides a scripted ocr.Engine for unit tests.
package ocrtest

import (
	"context"
	"image"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Rake-Pro/GoShareIt/internal/core/ocr"
)

// Fake is a scripted engine. Probe returns Status; Recognize returns Result
// and Err after Delay (or ctx.Err() when ctx ends first). The counters let
// tests assert how often each method ran.
type Fake struct {
	Status ocr.Status
	Result ocr.Result
	Err    error
	Delay  time.Duration

	Calls      atomic.Int32 // Recognize calls
	ProbeCalls atomic.Int32 // Probe calls
}

// Probe returns f.Status.
func (f *Fake) Probe(context.Context) ocr.Status {
	f.ProbeCalls.Add(1)
	return f.Status
}

// Recognize returns f.Result and f.Err after f.Delay, honoring ctx.
func (f *Fake) Recognize(ctx context.Context, _ image.Image, _ ocr.Options) (ocr.Result, error) {
	f.Calls.Add(1)
	if f.Delay > 0 {
		select {
		case <-ctx.Done():
			return ocr.Result{}, ctx.Err()
		case <-time.After(f.Delay):
		}
	}
	return f.Result, f.Err
}

// Lines builds a Result from compact specs, one argument per line. A line
// spec is space-separated words written "text@x,y,w,h"; the text runs up to
// the last '@', so emails work ("a@b.com@10,0,60,12").
func Lines(specs ...string) ocr.Result {
	var r ocr.Result
	for _, spec := range specs {
		var l ocr.Line
		for _, f := range strings.Fields(spec) {
			at := strings.LastIndexByte(f, '@')
			if at < 0 {
				continue
			}
			var v [4]int
			for i, p := range strings.SplitN(f[at+1:], ",", 4) {
				v[i], _ = strconv.Atoi(p)
			}
			l.Words = append(l.Words, ocr.Word{
				Text: f[:at],
				Rect: image.Rect(v[0], v[1], v[0]+v[2], v[1]+v[3]),
				Conf: 1,
			})
		}
		l.Rect = ocr.UnionRect(l.Words)
		l.Text = ocr.JoinWords(l.Words)
		r.Lines = append(r.Lines, l)
	}
	return r
}
