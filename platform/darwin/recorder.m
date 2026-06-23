// Native macOS screen recorder using AVFoundation (NO ffmpeg).
//
// API choice: AVFoundation's AVCaptureScreenInput + AVCaptureMovieFileOutput,
// rather than ScreenCaptureKit (SCStream -> AVAssetWriter). Rationale: this code
// is written without a macOS toolchain to compile against, so robustness of the
// un-compiled path matters most. AVCaptureMovieFileOutput owns H.264 encoding
// and mp4 container finalization internally and exposes a single, well-defined
// completion delegate callback to synchronize on. ScreenCaptureKit would require
// hand-rolling the SCStreamOutput -> CMSampleBuffer -> AVAssetWriterInput pump,
// pixel-format/timing plumbing, and manual finishWriting bookkeeping - far more
// surface to get subtly wrong sight-unseen. Both are native and ffmpeg-free.
//
// AVCaptureScreenInput is deprecated as of macOS 13 (Apple steers new code to
// ScreenCaptureKit) but remains fully functional. A future pass may migrate to
// ScreenCaptureKit once this can be compiled and tested on a real Mac.
//
// Permissions: screen capture requires the Screen Recording TCC grant. The
// signed .app must hold it; Info.plist already carries NSScreenCaptureUsageDescription.

#import "recorder.h"
#import <Foundation/Foundation.h>
#import <AVFoundation/AVFoundation.h>
#import <CoreMedia/CoreMedia.h>
#import <CoreGraphics/CoreGraphics.h>

#include <string.h>

static char g_last_error[512];

static void gsi_set_error(NSString *msg) {
    if (msg == nil) {
        g_last_error[0] = '\0';
        return;
    }
    const char *c = msg.UTF8String;
    if (c == NULL) {
        g_last_error[0] = '\0';
        return;
    }
    strncpy(g_last_error, c, sizeof(g_last_error) - 1);
    g_last_error[sizeof(g_last_error) - 1] = '\0';
}

const char *gsi_recorder_last_error(void) {
    return g_last_error;
}

@interface GSIRecorderDelegate : NSObject <AVCaptureFileOutputRecordingDelegate>
@property (nonatomic, strong) dispatch_semaphore_t done;
@property (nonatomic, strong) NSError *finishError;
@property (nonatomic, assign) BOOL success;
@end

@implementation GSIRecorderDelegate
- (void)captureOutput:(AVCaptureFileOutput *)output
        didFinishRecordingToOutputFileAtURL:(NSURL *)outputFileURL
        fromConnections:(NSArray<AVCaptureConnection *> *)connections
        error:(NSError *)error {
    // AVCaptureMovieFileOutput often reports a non-nil error even on a clean
    // stop (e.g. AVErrorMaximumDurationReached). The userInfo flag below is the
    // authoritative success signal; trust it before treating error as fatal.
    BOOL ok = YES;
    if (error != nil) {
        id v = error.userInfo[AVErrorRecordingSuccessfullyFinishedKey];
        ok = (v != nil) ? [v boolValue] : NO;
        if (!ok) {
            self.finishError = error;
        }
    }
    self.success = ok;
    dispatch_semaphore_signal(self.done);
}
@end

static AVCaptureSession *g_session = nil;
static AVCaptureMovieFileOutput *g_output = nil;
static GSIRecorderDelegate *g_delegate = nil;

int gsi_recorder_start(const char *out_path) {
    @autoreleasepool {
        gsi_set_error(nil);
        if (g_session != nil) {
            gsi_set_error(@"recorder already started");
            return 1;
        }
        if (out_path == NULL) {
            gsi_set_error(@"nil output path");
            return 2;
        }

        CGDirectDisplayID displayID = CGMainDisplayID();
        AVCaptureScreenInput *input =
            [[AVCaptureScreenInput alloc] initWithDisplayID:displayID];
        if (input == nil) {
            gsi_set_error(@"failed to create screen input");
            return 3;
        }
        input.minFrameDuration = CMTimeMake(1, 30); // ~30 fps cap
        input.capturesCursor = YES;
        // NOTE: VideoRegion cropping would set input.cropRect here. Start()
        // receives no rect (the Recorder interface passes only a Mode), so we
        // record the full display for both VideoFull and VideoRegion. See the
        // TODO in recorder.go; Capabilities advertises VideoFull only.

        AVCaptureSession *session = [[AVCaptureSession alloc] init];
        if ([session canAddInput:input]) {
            [session addInput:input];
        } else {
            gsi_set_error(@"cannot add screen input to session");
            return 4;
        }

        AVCaptureMovieFileOutput *output = [[AVCaptureMovieFileOutput alloc] init];
        if ([session canAddOutput:output]) {
            [session addOutput:output];
        } else {
            gsi_set_error(@"cannot add movie file output to session");
            return 5;
        }

        GSIRecorderDelegate *delegate = [[GSIRecorderDelegate alloc] init];
        delegate.done = dispatch_semaphore_create(0);

        [session startRunning];

        NSString *path = [NSString stringWithUTF8String:out_path];
        NSURL *url = [NSURL fileURLWithPath:path];
        [output startRecordingToOutputFileURL:url recordingDelegate:delegate];

        g_session = session;
        g_output = output;
        g_delegate = delegate;
        return 0;
    }
}

int gsi_recorder_stop(void) {
    @autoreleasepool {
        gsi_set_error(nil);
        if (g_session == nil || g_output == nil || g_delegate == nil) {
            gsi_set_error(@"recorder not started");
            return 1;
        }

        [g_output stopRecording];

        // Block until the delegate's didFinishRecording callback fires, meaning
        // the mp4 is fully written. AVCaptureFileOutput delivers this on its own
        // internal queue (not the main queue), so blocking the calling thread
        // here does not deadlock. The Go side must not call this on the main
        // thread regardless (documented in recorder.go).
        dispatch_semaphore_wait(g_delegate.done, DISPATCH_TIME_FOREVER);

        [g_session stopRunning];

        int rc = 0;
        if (!g_delegate.success) {
            NSError *err = g_delegate.finishError;
            gsi_set_error(err != nil ? err.localizedDescription
                                     : @"recording did not finish successfully");
            rc = 2;
        }

        g_session = nil;
        g_output = nil;
        g_delegate = nil;
        return rc;
    }
}
