package upload

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// defaultClient is used by the HTTP uploaders when no client is passed. It has
// no total timeout, so a large recording on a slow link is never cut off, but
// a server that accepts the upload and then never answers is given up on, and
// redirects follow checkRedirect.
var defaultClient = newHTTPClient()

func newHTTPClient() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = 5 * time.Minute
	return &http.Client{Transport: t, CheckRedirect: checkRedirect}
}

// checkRedirect keeps credentials on the endpoint the user configured. Go
// copies custom headers (API keys) to any redirect target, and Authorization
// whenever the host name matches, whatever the scheme. A redirect from https
// to http is refused (the credentials and the file would go out in
// cleartext); a redirect to another host or port is followed without the
// configured headers.
func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	orig := via[0].URL
	if orig.Scheme == "https" && req.URL.Scheme != "https" {
		return fmt.Errorf("refusing redirect from https to %s://%s", req.URL.Scheme, req.URL.Host)
	}
	// An http->https upgrade on the same host name counts as the same endpoint
	// even though the default port changes.
	sameHost := strings.EqualFold(req.URL.Hostname(), orig.Hostname()) &&
		(req.URL.Scheme != orig.Scheme || portOf(req.URL) == portOf(orig))
	if !sameHost {
		for k := range req.Header {
			switch k {
			case "User-Agent", "Accept", "Accept-Encoding", "Content-Type":
			default:
				req.Header.Del(k)
			}
		}
	}
	return nil
}

func portOf(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	if u.Scheme == "http" {
		return "80"
	}
	return "443"
}

// redactURLError drops everything after the host from the URL inside a
// transport error, so an API key in the query (imgbb, lensdump, chevereto) or
// path never reaches the log or the "Upload failed" notification.
func redactURLError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		if u, perr := url.Parse(ue.URL); perr == nil && u.Host != "" {
			ue.URL = u.Scheme + "://" + u.Host + "/..."
		} else {
			ue.URL = "(redacted)"
		}
	}
	return err
}
