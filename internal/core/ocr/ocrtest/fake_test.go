package ocrtest

import (
	"context"
	"errors"
	"image"
	"testing"
	"time"

	"github.com/Rake-Pro/GoShareIt/internal/core/ocr"
)

func TestFakeHonorsContext(t *testing.T) {
	f := &Fake{Delay: time.Hour}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := f.Recognize(ctx, image.NewRGBA(image.Rect(0, 0, 1, 1)), ocr.Options{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("Recognize did not return on ctx")
	}
	if f.Calls.Load() != 1 {
		t.Fatalf("Calls = %d, want 1", f.Calls.Load())
	}
}

func TestFakeReturnsScriptedResult(t *testing.T) {
	want := Lines("hi@0,0,10,10")
	f := &Fake{Result: want, Delay: time.Millisecond, Status: ocr.Status{Available: true}}
	if !f.Probe(context.Background()).Available || f.ProbeCalls.Load() != 1 {
		t.Fatal("Probe did not return the scripted status")
	}
	got, err := f.Recognize(context.Background(), nil, ocr.Options{})
	if err != nil || got.Text() != "hi" {
		t.Fatalf("got %q, %v", got.Text(), err)
	}
}

func TestLinesParsesSpecs(t *testing.T) {
	r := Lines("Mail@0,0,30,12 a@b.com@40,0,60,12", "two@0,20,30,12")
	if len(r.Lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(r.Lines))
	}
	w := r.Lines[0].Words[1]
	if w.Text != "a@b.com" || w.Rect != image.Rect(40, 0, 100, 12) {
		t.Fatalf("word = %+v", w)
	}
	if r.Lines[0].Text != "Mail a@b.com" || r.Text() != "Mail a@b.com\ntwo" {
		t.Fatalf("text = %q", r.Text())
	}
	if r.Lines[0].Rect != image.Rect(0, 0, 100, 12) {
		t.Fatalf("line rect = %v", r.Lines[0].Rect)
	}
}
