# Browser-grade TLS with `WithTransport`

Google fingerprints the TLS handshake. Go's `crypto/tls` produces a ClientHello
no browser would ever send, so the live endpoint answers the stock transport
with a bot wall — a `*gflight.BlockedError` — far more often than it answers a
browser.

`WithTransport` is the seam for fixing that. This library bundles no TLS stack:
the dependency list stays at one module, and a uTLS fingerprint that is current
today is stale in six months. The recipe lives here as code you copy into your
own program, where you own the upgrade.

There is no `main.go` in this directory on purpose — compiling it would drag
[uTLS](https://github.com/refraction-networking/utls) into `go.mod` for every
consumer of the library.

## The recipe

In your own module:

```sh
go get github.com/refraction-networking/utls
```

```go
package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/khoi-truong/gflight"
	utls "github.com/refraction-networking/utls"
)

// utlsTransport dials TLS with a Chrome fingerprint instead of Go's.
func utlsTransport() http.RoundTripper {
	return &http.Transport{
		// HTTP/2 negotiation is handled by the uTLS handshake's ALPN below;
		// leave ForceAttemptHTTP2 off so net/http does not re-negotiate.
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			raw, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			conn := utls.UClient(raw, &utls.Config{ServerName: host}, utls.HelloChrome_Auto)
			if err := conn.HandshakeContext(ctx); err != nil {
				_ = raw.Close()
				return nil, err
			}
			return conn, nil
		},
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     90 * time.Second,
	}
}

func main() {
	client := gflight.New(
		gflight.WithTransport(utlsTransport()),
		// A browser-shaped handshake still deserves browser-shaped pacing.
		gflight.WithRateLimit(2, 1),
		gflight.WithRetry(gflight.RetryPolicy{MaxAttempts: 3}),
		gflight.WithUserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) "+
			"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36"),
	)

	its, err := client.Search(context.Background(), gflight.SearchRequest{
		Origin:      "SGN",
		Destination: "HAN",
		DepartDate:  time.Now().AddDate(0, 1, 0),
		Adults:      1,
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(len(its), "itineraries")
}
```

## What matters

- **Match the User-Agent to the fingerprint.** A Chrome ClientHello carrying a
  Go User-Agent is a louder signal than either alone. `HelloChrome_Auto` tracks
  a recent Chrome; keep the header's version in the same neighbourhood.
- **Layering.** `WithTransport` is the *base*; `WithRetry` wraps it and
  `WithRateLimit` sits between the two. Retrying a bot wall harder is not the
  fix — slow down, or change the network path.
- **`WithHTTPClient` wins.** If you pass a client that already has a non-nil
  `Transport`, that transport is the base and this one is ignored. Use one or
  the other.
- **Watch it.** `WithObserver` reports each attempt's status; a rising 429/403
  share is the signal to back off before the address is blocked outright.
- **It is not a guarantee.** Fingerprinting is an arms race with no SLA on
  either side. Keep the `*BlockedError.DeepLink` browser fallback wired up.
