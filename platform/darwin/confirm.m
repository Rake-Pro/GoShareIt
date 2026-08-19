//go:build darwin

#import <AppKit/AppKit.h>

// gsi_confirm shows a modal two-button NSAlert on the main thread. The alert
// belongs to this process: app icon, and it dies with the app instead of
// lingering like a detached osascript dialog. Returns 1 (ok button),
// 0 (cancel button / Escape / timed out), -1 (no NSApp run loop to host it,
// or an NSException while showing), -2 (another confirm is already open).
int gsi_confirm(const char *ctitle, const char *cbody, const char *cok,
                const char *ccancel, int giveUpSeconds) {
	if (NSApp == nil) {
		return -1;
	}
	__block int rc = 0;
	void (^show)(void) = ^{
		@try {
			@autoreleasepool {
				// Main-queue blocks DO run during runModal (the main queue is
				// drained in the common modes, which include the modal-panel
				// mode) - so a concurrent gsi_confirm would nest a second
				// modal session inside this one, and the first alert's give-up
				// timer would then abort the wrong session. Serialize instead;
				// main-thread execution makes the flag race-free.
				static BOOL showing = NO;
				if (showing) {
					rc = -2;
					return;
				}
				showing = YES;

				NSAlert *alert = [[[NSAlert alloc] init] autorelease];
				alert.messageText = [NSString stringWithUTF8String:ctitle];
				alert.informativeText = [NSString stringWithUTF8String:cbody];
				[alert addButtonWithTitle:[NSString stringWithUTF8String:cok]]; // first button = default (Return)
				NSButton *cancelBtn = [alert addButtonWithTitle:[NSString stringWithUTF8String:ccancel]];
				cancelBtn.keyEquivalent = @"\x1b"; // Escape answers "no"

				// LSUIElement app: keep the panel above normal windows and take
				// focus, or the alert can open behind whatever is frontmost.
				alert.window.level = NSModalPanelWindowLevel;
				[NSApp activateIgnoringOtherApps:YES];

				// Auto-dismiss. Registered in both modes so it still fires if
				// the run loop is sitting in an event-tracking loop (open menu)
				// when the deadline passes.
				NSTimer *giveUp = [NSTimer timerWithTimeInterval:(NSTimeInterval)giveUpSeconds
				                                         repeats:NO
				                                           block:^(NSTimer *t) { [NSApp abortModal]; }];
				[[NSRunLoop currentRunLoop] addTimer:giveUp forMode:NSModalPanelRunLoopMode];
				[[NSRunLoop currentRunLoop] addTimer:giveUp forMode:NSRunLoopCommonModes];
				NSModalResponse resp = [alert runModal];
				[giveUp invalidate];
				// abortModal can end the session without the button action that
				// orders the window out - make sure no dead alert stays visible.
				[alert.window orderOut:nil];
				showing = NO;
				rc = (resp == NSAlertFirstButtonReturn) ? 1 : 0;
			}
		} @catch (NSException *e) {
			// Never let an NSException unwind into the cgo frame (uncatchable
			// SIGABRT). NB: `showing` intentionally not reset here - after an
			// exception mid-modal the session state is unknown.
			rc = -1;
		}
	};
	if ([NSThread isMainThread]) {
		show();
	} else {
		dispatch_sync(dispatch_get_main_queue(), show);
	}
	return rc;
}
