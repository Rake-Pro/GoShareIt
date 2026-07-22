//go:build darwin

#include <stdbool.h>
#import <ApplicationServices/ApplicationServices.h>
#import <CoreGraphics/CoreGraphics.h>
// IOHIDRequestAccess/kIOHIDRequestTypeListenEvent live in hidsystem/, not hid/
// (hid/IOHIDLib.h merely happened to compile before Xcode made implicit
// declarations a hard error).
#import <IOKit/hidsystem/IOHIDLib.h>

// gsi_request_accessibility checks Accessibility trust and, if not yet
// determined, prompts the user (kAXTrustedCheckOptionPrompt). Returns whether the
// process is currently trusted.
bool gsi_request_accessibility(void) {
	const void *keys[] = {kAXTrustedCheckOptionPrompt};
	const void *vals[] = {kCFBooleanTrue};
	CFDictionaryRef opts = CFDictionaryCreate(NULL, keys, vals, 1,
	                                          &kCFTypeDictionaryKeyCallBacks,
	                                          &kCFTypeDictionaryValueCallBacks);
	bool trusted = AXIsProcessTrustedWithOptions(opts);
	CFRelease(opts);
	return trusted;
}

// gsi_request_screen_capture returns true if Screen Recording is already granted;
// otherwise it requests it (which shows the system prompt the first time).
bool gsi_request_screen_capture(void) {
	if (CGPreflightScreenCaptureAccess()) {
		return true;
	}
	return CGRequestScreenCaptureAccess();
}

// gsi_request_input_monitoring requests Input Monitoring (the listen-events TCC
// service the CGEventTap hotkey backend needs) and returns whether it is granted.
bool gsi_request_input_monitoring(void) {
	return IOHIDRequestAccess(kIOHIDRequestTypeListenEvent);
}
