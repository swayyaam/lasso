#ifndef LASSO_NOTIFY_DARWIN_H
#define LASSO_NOTIFY_DARWIN_H

#include <stdbool.h>

// lassoCanNotify reports whether this process is a real application bundle.
bool lassoCanNotify(void);

// lassoNotify posts a notification attributed to this application, and reports
// whether the system accepted it.
bool lassoNotify(const char *title, const char *body);

// lassoNotificationStatus reports whether this application may post
// notifications: -1 unavailable, 0 not yet asked, 1 refused, 2 allowed.
// It asks nothing and posts nothing.
int lassoNotificationStatus(void);

#endif
