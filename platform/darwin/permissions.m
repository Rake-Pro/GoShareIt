//go:build darwin

#include <stdbool.h>
#import <ApplicationServices/ApplicationServices.h>
#import <CoreGraphics/CoreGraphics.h>

// gsi_request_screen_capture returns true if Screen Recording is already granted;
// otherwise it requests it (which shows the system prompt the first time).
bool gsi_request_screen_capture(void) {
	if (CGPreflightScreenCaptureAccess()) {
		return true;
	}
	return CGRequestScreenCaptureAccess();
}
