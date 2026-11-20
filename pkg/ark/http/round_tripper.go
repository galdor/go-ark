package http

import (
	"math"
	nethttp "net/http"
	"strconv"
	"time"

	"go.n16f.net/ark/pkg/ark/log"
	"go.n16f.net/ark/pkg/ark/utils"
)

type RoundTripper struct {
	Cfg *ClientCfg
	Log *log.Logger

	nethttp.RoundTripper
}

func NewRoundTripper(rt nethttp.RoundTripper, cfg *ClientCfg) *RoundTripper {
	return &RoundTripper{
		Cfg: cfg,
		Log: cfg.Log,

		RoundTripper: rt,
	}
}

func (rt *RoundTripper) RoundTrip(
	req *nethttp.Request,
) (*nethttp.Response, error) {
	start := time.Now()

	rt.finalizeReq(req)

	res, err := rt.RoundTripper.RoundTrip(req)

	if err == nil && rt.Cfg.LogRequests {
		rt.logRequest(req, res, time.Since(start).Seconds())
	}

	return res, err
}

func (rt *RoundTripper) finalizeReq(req *nethttp.Request) {
	for name, value := range rt.Cfg.Header {
		req.Header.Add(name, value)
	}
}

func (rt *RoundTripper) logRequest(
	req *nethttp.Request, res *nethttp.Response, seconds float64,
) {
	attrs := []any{
		"event", "http_client.request",
		"http.request_time", int64(math.Round(seconds * 1e6)),
	}

	statusString := "-"
	if res != nil {
		statusString = strconv.Itoa(res.StatusCode)

		attrs = append(attrs, "http.status")
		attrs = append(attrs, res.StatusCode)
	}

	rt.Log.InfoData(attrs, "%s %s %s %s",
		req.Method, utils.EscapeNonPrintable(req.URL.Path), statusString,
		utils.FormatSeconds(seconds, 1))
}
