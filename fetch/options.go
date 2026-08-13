package fetch

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

type fetchConfig struct {
	base       *resty.Client
	timeout    time.Duration
	proxy      string
	httpClient *http.Client
	transport  http.RoundTripper
}

// FetchOption configures the Go-side resty executor. It is not exposed to JS.
type FetchOption func(*fetchConfig)

func newClient(opts ...FetchOption) *resty.Client {
	cfg := &fetchConfig{}
	for _, o := range opts {
		o(cfg)
	}
	// resty v2.16.5 has no SetHTTPClient setter; the only way to supply a
	// custom *http.Client is at construction via NewWithClient. When a base
	// resty client is provided (WithRestyClient) it takes precedence and the
	// httpClient option is ignored, matching the WithRestyClient doc.
	var c *resty.Client
	if cfg.base != nil {
		c = cfg.base
	} else if cfg.httpClient != nil {
		c = resty.NewWithClient(cfg.httpClient)
	} else {
		c = resty.New()
		c.SetRedirectPolicy(defaultFetchRedirectPolicy(20))
	}
	if cfg.timeout > 0 {
		c.SetTimeout(cfg.timeout)
	}
	if cfg.proxy != "" {
		c.SetProxy(cfg.proxy)
	}
	if cfg.transport != nil {
		c.SetTransport(cfg.transport)
	}
	return c
}

func defaultFetchRedirectPolicy(limit int) resty.RedirectPolicy {
	return resty.RedirectPolicyFunc(func(request *http.Request, via []*http.Request) error {
		if len(via) >= limit {
			return fmt.Errorf("stopped after %d redirects", limit)
		}
		if len(via) > 0 && !sameHTTPOrigin(via[len(via)-1].URL, request.URL) {
			request.Header.Del("Authorization")
		}
		return nil
	})
}

func sameHTTPOrigin(left, right *url.URL) bool {
	if left == nil || right == nil ||
		!strings.EqualFold(left.Scheme, right.Scheme) ||
		!strings.EqualFold(left.Hostname(), right.Hostname()) {
		return false
	}
	return effectivePort(left) == effectivePort(right)
}

func effectivePort(value *url.URL) string {
	if port := value.Port(); port != "" {
		return port
	}
	switch strings.ToLower(value.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

// WithTimeout sets the total request timeout, including response body reads.
func WithTimeout(d time.Duration) FetchOption {
	return func(f *fetchConfig) { f.timeout = d }
}

// WithProxy sets an HTTP/HTTPS/SOCKS proxy URL.
func WithProxy(proxyURL string) FetchOption {
	return func(f *fetchConfig) { f.proxy = proxyURL }
}

// WithHTTPClient supplies a custom *http.Client.
func WithHTTPClient(client *http.Client) FetchOption {
	return func(f *fetchConfig) { f.httpClient = client }
}

// WithTransport sets the underlying http.RoundTripper.
func WithTransport(rt http.RoundTripper) FetchOption {
	return func(f *fetchConfig) { f.transport = rt }
}

// WithRestyClient is an advanced escape hatch; the provided client takes
// precedence and other applicable options are still applied on top of it.
func WithRestyClient(client *resty.Client) FetchOption {
	return func(f *fetchConfig) { f.base = client }
}
