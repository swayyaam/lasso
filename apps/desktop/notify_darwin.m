#import <Foundation/Foundation.h>
#import <UserNotifications/UserNotifications.h>

#include "notify_darwin.h"

// lassoCanNotify reports whether this process is a real application bundle.
//
// UNUserNotificationCenter refuses to exist outside one: asking for
// currentNotificationCenter from a bare binary raises an exception that takes
// the process with it. `go test` and `go run` are bare binaries, so this check
// is what keeps a notification from crashing them.
bool lassoCanNotify(void) {
  return [[NSBundle mainBundle] bundleIdentifier] != nil;
}

// authorizationTimeout bounds the wait for the user to answer the permission
// prompt. Only the first notification ever waits; afterwards the system answers
// from its own record immediately.
static const int64_t authorizationTimeoutSeconds = 5;

// lassoNotify posts a notification attributed to this application.
//
// The point of doing it here rather than through osascript is attribution: a
// notification posted by osascript comes from Script Editor, with Script
// Editor's name and icon, because that is the process that asked for it. Posted
// from inside the app it carries this bundle's identifier, so it shows Lasso's
// name and icon and appears under Lasso in Notification Settings.
//
// It returns false when the system will not take it — most likely because this
// build is only ad-hoc signed, which macOS may refuse to register for
// notifications. The caller falls back rather than going silent, because a
// notification from the wrong name beats no notification at all.
bool lassoNotify(const char *title, const char *body) {
  @autoreleasepool {
    if (!lassoCanNotify()) {
      return false;
    }

    UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];

    // Asked for on the first notification rather than at launch, which is the
    // moment the permission makes sense to the person answering it.
    __block BOOL granted = NO;
    dispatch_semaphore_t answered = dispatch_semaphore_create(0);
    [center requestAuthorizationWithOptions:(UNAuthorizationOptionAlert)
                          completionHandler:^(BOOL allowed, NSError *_Nullable error) {
                            granted = allowed && error == nil;
                            dispatch_semaphore_signal(answered);
                          }];

    // The completion handler is delivered on an internal queue, so this waits
    // without blocking whatever would deliver it. Go calls this from a
    // goroutine, never from the thread running the UI.
    dispatch_semaphore_wait(
        answered, dispatch_time(DISPATCH_TIME_NOW, authorizationTimeoutSeconds * NSEC_PER_SEC));
    if (!granted) {
      return false;
    }

    UNMutableNotificationContent *content = [[UNMutableNotificationContent alloc] init];
    content.title = title ? @(title) : @"";
    content.body = body ? @(body) : @"";

    // A nil trigger means deliver immediately.
    UNNotificationRequest *request = [UNNotificationRequest requestWithIdentifier:[[NSUUID UUID] UUIDString]
                                                                         content:content
                                                                         trigger:nil];
    [center addNotificationRequest:request withCompletionHandler:nil];
    return true;
  }
}

// lassoNotificationStatus reports the current authorization, without asking for
// it and without posting anything.
//
// The doctor uses it: a build that macOS will not let post under its own name
// is worth saying out loud, because the notifications still arrive — from
// Script Editor, which is the process the fallback runs in — and a user seeing
// that has no way to know why.
int lassoNotificationStatus(void) {
  @autoreleasepool {
    if (!lassoCanNotify()) {
      return -1;
    }

    __block int status = -1;
    dispatch_semaphore_t answered = dispatch_semaphore_create(0);
    [[UNUserNotificationCenter currentNotificationCenter]
        getNotificationSettingsWithCompletionHandler:^(UNNotificationSettings *settings) {
          switch (settings.authorizationStatus) {
            case UNAuthorizationStatusNotDetermined:
              status = 0;
              break;
            case UNAuthorizationStatusDenied:
              status = 1;
              break;
            case UNAuthorizationStatusAuthorized:
            case UNAuthorizationStatusProvisional:
              status = 2;
              break;
            default:
              status = -1;
              break;
          }
          dispatch_semaphore_signal(answered);
        }];
    dispatch_semaphore_wait(answered, dispatch_time(DISPATCH_TIME_NOW, 3 * NSEC_PER_SEC));
    return status;
  }
}
