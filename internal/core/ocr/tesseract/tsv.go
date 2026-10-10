package tesseract

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"math"
	"strconv"
	"strings"

	"github.com/Rake-Pro/GoShareIt/internal/core/ocr"
)

// TSV columns: level page_num block_num par_num line_num word_num left top
// width height conf text.
const tsvCols = 12

// parseTSV turns tesseract TSV into lines of words. Rows with level 5 are
// words; conf -1 rows and empty text are skipped. Words are grouped into
// lines by (block, par, line) in output order, and every box is mapped back
// through tf into the source image (bounds) and rounded outward.
func parseTSV(data []byte, tf transform, bounds image.Rectangle) ([]ocr.Line, error) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	type key struct{ block, par, line int }
	index := map[key]int{}
	var lines []ocr.Line
	header := true
	for sc.Scan() {
		row := sc.Text()
		if header {
			header = false
			if !strings.HasPrefix(row, "level") {
				return nil, errTSV
			}
			continue
		}
		f := strings.SplitN(row, "\t", tsvCols)
		if len(f) < tsvCols-1 || f[0] != "5" {
			continue
		}
		n := make([]int, 10)
		for i := 0; i < 10; i++ {
			v, err := strconv.Atoi(strings.TrimSpace(f[i]))
			if err != nil {
				return nil, fmt.Errorf("%w: column %d %q", errTSV, i, f[i])
			}
			n[i] = v
		}
		conf, err := strconv.ParseFloat(strings.TrimSpace(f[10]), 64)
		if err != nil || conf < 0 {
			continue
		}
		text := ""
		if len(f) == tsvCols {
			text = strings.TrimSpace(f[11])
		}
		if text == "" {
			continue
		}
		w := ocr.Word{
			Text: text,
			Rect: tf.back(n[6], n[7], n[8], n[9]).Add(bounds.Min).Intersect(bounds),
			Conf: float32(conf / 100),
		}
		k := key{n[2], n[3], n[4]}
		i, ok := index[k]
		if !ok {
			i = len(lines)
			index[k] = i
			lines = append(lines, ocr.Line{})
		}
		lines[i].Words = append(lines[i].Words, w)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("tesseract: read TSV: %w", err)
	}
	if header {
		return nil, errTSV // no output at all
	}
	for i := range lines {
		lines[i].Rect = ocr.UnionRect(lines[i].Words)
		lines[i].Text = ocr.JoinWords(lines[i].Words)
	}
	return lines, nil
}

// back maps a box in preprocessed-image pixels to source-image pixels
// (origin 0,0), rounding outward.
func (tf transform) back(left, top, width, height int) image.Rectangle {
	s := tf.scale
	if s <= 0 {
		s = 1
	}
	x0 := math.Floor(float64(left-tf.pad) / s)
	y0 := math.Floor(float64(top-tf.pad) / s)
	x1 := math.Ceil(float64(left+width-tf.pad) / s)
	y1 := math.Ceil(float64(top+height-tf.pad) / s)
	return image.Rect(int(x0), int(y0), int(x1), int(y1))
}
