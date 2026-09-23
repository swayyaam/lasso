package main

import (
	"fmt"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// documents are the pages About links to, by name.
//
// The page asks for a name and never hands over an address, for the same
// reason OpenFile takes an ID rather than a path: nothing that ran script in
// the page can use this to send someone anywhere else.
var documents = map[string]string{
	"privacy": "https://github.com/swayyaam/lasso/blob/main/PRIVACY.md",
	"terms":   "https://github.com/swayyaam/lasso/blob/main/TERMS.md",
	"licence": "https://github.com/swayyaam/lasso/blob/main/LICENSE",
	"notices": "https://github.com/swayyaam/lasso/blob/main/THIRD_PARTY_NOTICES.md",
}

// OpenDocument opens the privacy policy, the terms, the licence or the
// third-party notices in the person's browser. Each also ships inside the app,
// in Contents/Resources.
func (a *App) OpenDocument(name string) error {
	url, ok := documents[name]
	if !ok {
		return fmt.Errorf("there is no document called %q", name)
	}
	if a.ctx == nil {
		return fmt.Errorf("Lasso is still starting up")
	}
	runtime.BrowserOpenURL(a.ctx, url)
	return nil
}
