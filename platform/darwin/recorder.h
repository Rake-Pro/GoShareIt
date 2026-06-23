// Native macOS screen recorder C shim (no ffmpeg). Implemented in recorder.m
// using AVFoundation; called from recorder.go via cgo. Plain C declarations
// only so the cgo preamble (compiled as C) can include this header safely.
#ifndef GSI_RECORDER_H
#define GSI_RECORDER_H

// gsi_recorder_start begins recording the FULL main display to out_path (an
// .mp4). Equivalent to gsi_recorder_start_region with a non-positive w/h.
// Returns 0 on success, non-zero on failure (see gsi_recorder_last_error).
int gsi_recorder_start(const char *out_path);

// gsi_recorder_start_region begins recording the main display to out_path,
// optionally cropped to a sub-rectangle. (x, y, w, h) are in screen PIXELS with
// a TOP-LEFT origin. If w <= 0 || h <= 0 the full display is recorded (no crop).
// Otherwise the shim converts the pixel rect to AVCaptureScreenInput.cropRect,
// which is in display POINTS with a BOTTOM-LEFT origin: it divides by the main
// display's backing scale factor and flips Y against the display's point height.
// Returns 0 on success, non-zero on failure (see gsi_recorder_last_error).
int gsi_recorder_start_region(const char *out_path, int x, int y, int w, int h);

// gsi_recorder_stop stops the active recording and BLOCKS until the movie file
// output's completion delegate has fired, i.e. the .mp4 is fully flushed to
// disk. Returns 0 on success, non-zero on failure.
int gsi_recorder_stop(void);

// gsi_recorder_last_error returns the message for the most recent failure, or
// an empty string. The returned pointer is owned by the shim; copy it out.
const char *gsi_recorder_last_error(void);

#endif // GSI_RECORDER_H
