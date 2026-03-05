package provider

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// debugTransport is an http.RoundTripper that logs request and response details.
// It redacts the Authorization and x-api-key headers to protect credentials.
type debugTransport struct {
	base   http.RoundTripper
	output io.Writer
}

func (d *debugTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Read and log request body.
	var reqBody []byte
	if req.Body != nil {
		reqBody, _ = io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewReader(reqBody))
	}

	fmt.Fprintf(d.output, "piper: debug: > %s %s\n", req.Method, req.URL)
	if len(reqBody) > 0 {
		fmt.Fprintf(d.output, "piper: debug: > body: %s\n", string(reqBody))
	}

	resp, err := d.base.RoundTrip(req)
	if err != nil {
		fmt.Fprintf(d.output, "piper: debug: < error: %v\n", err)
		return nil, err
	}

	fmt.Fprintf(d.output, "piper: debug: < %s\n", resp.Status)

	// For non-streaming responses, log the body (tee so the caller can still read it).
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") && resp.Body != nil {
		var buf bytes.Buffer
		if _, err := io.Copy(&buf, resp.Body); err == nil {
			resp.Body.Close()
			body := buf.String()
			fmt.Fprintf(d.output, "piper: debug: < body: %s\n", body)
			resp.Body = io.NopCloser(strings.NewReader(body))
		}
	}

	return resp, nil
}

// wrapDebug replaces the HTTP client's transport with a logging wrapper.
// Pass nil to unwrap (restore the default transport).
func wrapDebug(client *http.Client, w io.Writer) {
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	if w == nil {
		client.Transport = http.DefaultTransport
		return
	}
	client.Transport = &debugTransport{base: base, output: w}
}
