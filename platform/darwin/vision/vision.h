// Apple Vision text recognition C shim. Implemented in vision.m; called from
// vision.go via cgo. Plain C declarations only so the cgo preamble (compiled
// as C) can include this header safely.
#ifndef GSI_VISION_H
#define GSI_VISION_H

typedef struct {
    char *text;       // UTF-8, malloc'd
    float conf;       // 0..1
    float x, y, w, h; // pixels, top-left origin
} gsi_vision_word;

typedef struct {
    char *text;       // top candidate for the whole line, malloc'd
    float conf;
    float x, y, w, h;
    int word_count;
    gsi_vision_word *words;
} gsi_vision_line;

typedef struct {
    int line_count;
    gsi_vision_line *lines;
} gsi_vision_result;

// gsi_vision_languages fills *out with a malloc'd, NUL-separated, double-NUL
// terminated list of supported BCP-47 tags for revision 3 and returns the
// count (0 = unavailable, see gsi_vision_last_error). Caller frees *out.
int gsi_vision_languages(char **out);

// gsi_vision_recognize runs a VNRecognizeTextRequest over a premultiplied
// RGBA8 buffer (stride bytes per row). langs is a NUL-separated list of
// nlangs tags (may be NULL with nlangs 0 for automatic detection). fast != 0
// selects the fast level, else accurate. Returns 0 on success and sets *out
// (free it with gsi_vision_free); non-zero on failure (see
// gsi_vision_last_error).
int gsi_vision_recognize(const unsigned char *rgba, int width, int height, int stride,
                         const char *langs, int nlangs, int fast, gsi_vision_result **out);

// gsi_vision_free releases a result from gsi_vision_recognize.
void gsi_vision_free(gsi_vision_result *r);

// gsi_vision_last_error returns the message for the most recent failure, or
// an empty string. The pointer is owned by the shim; copy it out.
const char *gsi_vision_last_error(void);

#endif // GSI_VISION_H
