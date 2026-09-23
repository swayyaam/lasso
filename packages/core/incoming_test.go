package core

import "testing"

func TestIncomingLink(t *testing.T) {
	yt := "https://www.youtube.com/watch?v=jNQXAC9IVRw"
	accepted := map[string]string{
		"lasso://open?url=https%3A%2F%2Fwww.youtube.com%2Fwatch%3Fv%3DjNQXAC9IVRw": yt,
		"lasso:open?url=https%3A%2F%2Fwww.youtube.com%2Fwatch%3Fv%3DjNQXAC9IVRw":   yt,
		"LASSO://open?url=https%3A%2F%2Fvimeo.com%2F1":                             "https://vimeo.com/1",
		// A link dropped on the Dock arrives as itself.
		yt:                       yt,
		"  https://vimeo.com/1 ": "https://vimeo.com/1",
	}
	for in, want := range accepted {
		got, err := IncomingLink(in)
		if err != nil || got != want {
			t.Errorf("IncomingLink(%q) = %q, %v; want %q", in, got, err, want)
		}
	}

	refused := []string{
		"",
		"lasso://open",
		"lasso://open?url=",
		"lasso://download?url=https%3A%2F%2Fvimeo.com%2F1",
		"lasso://open?url=file%3A%2F%2F%2Fetc%2Fpasswd",
		"lasso://open?url=javascript%3Aalert(1)",
		"file:///etc/passwd",
		"ftp://example.com/x",
		// This Mac and the local network: a page handing these over would be
		// reaching past the browser.
		"lasso://open?url=http%3A%2F%2F127.0.0.1%3A8080%2F",
		"lasso://open?url=http%3A%2F%2Flocalhost%2Fadmin",
		"lasso://open?url=http%3A%2F%2F192.168.1.1%2F",
		"lasso://open?url=http%3A%2F%2F100.100.100.100%2F",
		"lasso://open?url=http%3A%2F%2F%5B%3A%3A1%5D%2F",
		"lasso://open?url=http%3A%2F%2Fprinter.local%2F",
		"http://169.254.169.254/latest/meta-data/",
		"https:///no-host",
	}
	for _, in := range refused {
		if got, err := IncomingLink(in); err == nil {
			t.Errorf("IncomingLink(%q) = %q, want it refused", in, got)
		}
	}
}
