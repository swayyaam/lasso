package core

import (
	"errors"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

// LinkScheme is Lasso's own URL scheme. A bookmarklet or a Shortcut hands
// Lasso a page with lasso://open?url=<the page, escaped>.
const LinkScheme = "lasso"

// ErrNotALink is something handed to Lasso that is not a link it will open.
var ErrNotALink = errors.New("that is not a link Lasso can open")

// IncomingLink is the web link inside something another app handed Lasso: a
// lasso:// link, or a link dropped on the Dock icon.
//
// Opening one resolves it at once, the same as pasting, and a page can hand
// one over with nothing more than a click and the browser's "Open Lasso?"
// prompt. So it is held to more than a paste is: http or https only, and never
// an address on this Mac or the local network, where a resolve would be the
// page reaching past the browser's own protections. The check reads the
// literal host; a public name that resolves to a private address is a risk
// the same as any link, and no worse for having come this way.
func IncomingLink(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", ErrNotALink
	}

	if strings.EqualFold(parsed.Scheme, LinkScheme) {
		// lasso://open?url=… parses with Host "open"; lasso:open?url=… with
		// Opaque "open". Both are what a hand-written bookmarklet produces.
		if parsed.Host != "open" && parsed.Opaque != "open" {
			return "", ErrNotALink
		}
		return publicWebLink(parsed.Query().Get("url"))
	}
	return publicWebLink(raw)
}

func publicWebLink(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "", ErrNotALink
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", ErrNotALink
	}

	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return "", ErrNotALink
	}
	if addr, err := netip.ParseAddr(host); err == nil && !isPublicIP(net.IP(addr.AsSlice())) {
		return "", ErrNotALink
	}
	return parsed.String(), nil
}
