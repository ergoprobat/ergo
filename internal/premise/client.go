package premise

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

const (
	// UserAgent is the only identity the probe sends.
	UserAgent = "ipso-premise"
	// Timeout is the limit for one probe, including name resolution.
	Timeout = 10 * time.Second
	// BodyLimit is the maximum response body that is hashed and parsed.
	BodyLimit = 1 << 20
	// ObservedLimit is the maximum compact JSON size of an observed value.
	ObservedLimit = 1024
)

const (
	ResultPass  = "pass"
	ResultFail  = "fail"
	ResultError = "error"
)

const (
	ReasonURL      = "url"
	ReasonLookup   = "lookup"
	ReasonPrivate  = "private address"
	ReasonRedirect = "redirect"
	ReasonStatus   = "status"
	ReasonNotJSON  = "body is not json"
	ReasonBody     = "body too large"
	ReasonPointer  = "missing pointer"
	ReasonType     = "wrong type"
	ReasonSemver   = "semver"
	ReasonObserved = "observed too large"
	ReasonTimeout  = "timeout"
	ReasonNetwork  = "network"
)

// Spec is one premise probe. HasEquals is set for an equals comparison, including JSON null.
type Spec struct {
	URL       string
	Pointer   string
	Equals    any
	HasEquals bool
	Semver    string
}

// Result is one probe. Observed is compact JSON when a value was read and it fits the limit.
// BodySHA256 is set when response bytes were read. Reason is set only for an error.
type Result struct {
	Result     string
	Expected   any
	Observed   []byte
	BodySHA256 string
	Reason     string
}

// Client performs the public HTTPS GET from ADR-0053.
// Lookup, Dial, and Roots are test hooks. The zero value uses the system resolver,
// dials the address it already accepted, and uses the system certificate pool.
type Client struct {
	Lookup func(ctx context.Context, host string) ([]net.IP, error)
	Dial   func(ctx context.Context, network, addr string) (net.Conn, error)
	Roots  *x509.CertPool
}

// Check runs one probe. It does not follow redirects and it does not send credentials.
func (c Client) Check(ctx context.Context, spec Spec) Result {
	res := Result{Result: ResultError, Expected: expectedOf(spec)}
	target, err := parseProbeURL(spec.URL)
	if err != nil {
		res.Reason = ReasonURL
		return res
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	ips, err := c.lookup(ctx, target.Hostname())
	if err != nil || len(ips) == 0 {
		res.Reason = ReasonLookup
		return res
	}
	for _, ip := range ips {
		if !allowedAddr(ip) {
			res.Reason = ReasonPrivate
			return res
		}
	}
	fetched, reason := c.get(ctx, target, ips[0])
	if fetched.read {
		body := fetched.body
		if fetched.tooLarge && len(body) > BodyLimit {
			body = body[:BodyLimit]
		}
		res.BodySHA256 = hashBytes(body)
	}
	if reason != "" {
		res.Reason = reason
		return res
	}
	judged := judge(spec, fetched.body)
	judged.BodySHA256 = res.BodySHA256
	return judged
}

func (c Client) lookup(ctx context.Context, host string) ([]net.IP, error) {
	if c.Lookup != nil {
		return c.Lookup(ctx, host)
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, len(addrs))
	for i, addr := range addrs {
		ips[i] = addr.IP
	}
	return ips, nil
}

type fetchResult struct {
	body     []byte
	read     bool
	tooLarge bool
}

func (c Client) get(ctx context.Context, target *url.URL, ip net.IP) (fetchResult, string) {
	pinned := append(net.IP(nil), ip...)
	dial := c.Dial
	if dial == nil {
		dial = (&net.Dialer{}).DialContext
	}
	transport := &http.Transport{
		Proxy: func(*http.Request) (*url.URL, error) { return nil, nil },
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dial(ctx, network, net.JoinHostPort(pinned.String(), "443"))
		},
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: target.Hostname(),
			RootCAs:    c.Roots,
		},
		TLSNextProto:           map[string]func(string, *tls.Conn) http.RoundTripper{},
		DisableKeepAlives:      true,
		DisableCompression:     true,
		MaxResponseHeaderBytes: BodyLimit,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errRedirect
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return fetchResult{}, ReasonURL
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := client.Do(req)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if errors.Is(err, errRedirect) {
			return fetchResult{}, ReasonRedirect
		}
		return fetchResult{}, netReason(err)
	}
	defer func() { _ = resp.Body.Close() }()
	buf, err := io.ReadAll(io.LimitReader(resp.Body, BodyLimit+1))
	got := fetchResult{body: buf, read: true}
	if err != nil {
		return got, netReason(err)
	}
	if int64(len(buf)) > BodyLimit {
		got.tooLarge = true
		return got, ReasonBody
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return got, ReasonRedirect
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return got, ReasonStatus
	}
	return got, ""
}

func netReason(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return ReasonTimeout
	}
	var timeout net.Error
	if errors.As(err, &timeout) && timeout.Timeout() {
		return ReasonTimeout
	}
	return ReasonNetwork
}
