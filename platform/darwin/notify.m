//go:build darwin

#include <stdbool.h>
#import <Foundation/Foundation.h>
#import <UserNotifications/UserNotifications.h>

// Always present banners, even while the app is frontmost (e.g. right after an
// update dialog activated us) - the default suppresses them for the active app.
@interface GSINotifyDelegate : NSObject <UNUserNotificationCenterDelegate>
@end

@implementation GSINotifyDelegate
- (void)userNotificationCenter:(UNUserNotificationCenter *)center
       willPresentNotification:(UNNotification *)notification
         withCompletionHandler:(void (^)(UNNotificationPresentationOptions))completionHandler {
	completionHandler(UNNotificationPresentationOptionBanner | UNNotificationPresentationOptionList);
}
@end

// UNUserNotificationCenter throws NSInternalInconsistencyException when the
// process has no bundle identifier (a bare `go build` binary outside the .app),
// so callers must gate on this. It is a cheap pre-filter, not a guarantee -
// gsi_notify still catches exceptions itself (an Info.plist alone does not
// prove a LaunchServices-registered bundle).
bool gsi_notify_available(void) {
	return [[NSBundle mainBundle] bundleIdentifier] != nil;
}

// gsi_notify posts one notification attributed to this app bundle (so it
// carries the GoShareIt icon). Returns:
//   0 = accepted by the notification center
//   1 = authorization denied by the user
//   2 = center rejected the request
//   3 = NSException (center unusable in this launch context - caller should
//       fall back to osascript)
//   4 = authorization prompt still unanswered / status query timed out; this
//       one notification is dropped, later calls follow the user's answer
//
// Every semaphore wait branches on its result: on timeout the completion
// handler has not run, so its __block flag must not be read (no happens-before
// edge - reading it would be a data race, not just a stale value).
int gsi_notify(const char *ctitle, const char *cbody) {
	@try {
		@autoreleasepool {
			UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];

			static GSINotifyDelegate *delegate;
			static dispatch_once_t once;
			dispatch_once(&once, ^{
				delegate = [[GSINotifyDelegate alloc] init]; // lives for the process (.delegate is weak)
				center.delegate = delegate;
			});

			__block UNAuthorizationStatus status = UNAuthorizationStatusNotDetermined;
			dispatch_semaphore_t statusSem = dispatch_semaphore_create(0);
			[center getNotificationSettingsWithCompletionHandler:^(UNNotificationSettings *s) {
				status = s.authorizationStatus;
				dispatch_semaphore_signal(statusSem);
			}];
			long statusTimedOut = dispatch_semaphore_wait(
			    statusSem, dispatch_time(DISPATCH_TIME_NOW, 5 * NSEC_PER_SEC));
			dispatch_release(statusSem);
			if (statusTimedOut) {
				return 4;
			}

			if (status == UNAuthorizationStatusNotDetermined) {
				// First run only: show the system permission prompt. The wait
				// is bounded because the user may leave the prompt open for
				// minutes; once they answer, the grant is standing and the
				// settings query above short-circuits every later call.
				__block BOOL granted = NO;
				dispatch_semaphore_t authSem = dispatch_semaphore_create(0);
				[center requestAuthorizationWithOptions:(UNAuthorizationOptionAlert | UNAuthorizationOptionSound)
				                      completionHandler:^(BOOL ok, NSError *err) {
					granted = ok;
					dispatch_semaphore_signal(authSem);
				}];
				long authTimedOut = dispatch_semaphore_wait(
				    authSem, dispatch_time(DISPATCH_TIME_NOW, 10 * NSEC_PER_SEC));
				dispatch_release(authSem);
				if (authTimedOut) {
					return 4;
				}
				if (!granted) {
					return 1;
				}
			} else if (status == UNAuthorizationStatusDenied) {
				return 1;
			}

			UNMutableNotificationContent *content = [[[UNMutableNotificationContent alloc] init] autorelease];
			content.title = [NSString stringWithUTF8String:ctitle];
			content.body = [NSString stringWithUTF8String:cbody];
			UNNotificationRequest *req =
			    [UNNotificationRequest requestWithIdentifier:[[NSUUID UUID] UUIDString]
			                                         content:content
			                                         trigger:nil];
			__block BOOL added = NO;
			dispatch_semaphore_t addSem = dispatch_semaphore_create(0);
			[center addNotificationRequest:req withCompletionHandler:^(NSError *err) {
				added = (err == nil);
				dispatch_semaphore_signal(addSem);
			}];
			long addTimedOut = dispatch_semaphore_wait(
			    addSem, dispatch_time(DISPATCH_TIME_NOW, 5 * NSEC_PER_SEC));
			dispatch_release(addSem);
			if (addTimedOut) {
				return 4;
			}
			return added ? 0 : 2;
		}
	} @catch (NSException *e) {
		// Never let an NSException unwind into the cgo frame - that is an
		// uncatchable SIGABRT for the whole app.
		return 3;
	}
}
