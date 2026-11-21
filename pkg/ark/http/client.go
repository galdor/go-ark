package http

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"go.n16f.net/ark/pkg/ark/json"
	"go.n16f.net/ark/pkg/ark/log"
)

const (
	DefaultClientConnectionTimeout = 30 // seconds
	DefaultClientRequestTimeout    = 30 // seconds
)

type ClientBodyEncoder func(any) ([]byte, error)
type ClientBodyDecoder func([]byte, any) error
type ClientErrorHandler func(*http.Response, []byte) error

type ClientCfg struct {
	LogRequests         bool              `json:"log_requests"`
	DisableRedirections bool              `json:"disable_redirections"`
	TLS                 *TLSClientCfg     `json:"tls"`
	ConnectionTimeout   *int              `json:"connection_timeout"` // seconds
	RequestTimeout      *int              `json:"request_timeout"`    // seconds
	BaseURI             string            `json:"base_uri"`
	Header              map[string]string `json:"header"`

	Log                 *log.Logger        `json:"-"`
	RequestBodyEncoder  ClientBodyEncoder  `json:"-"`
	ResponseBodyDecoder ClientBodyDecoder  `json:"-"`
	ErrorHandler        ClientErrorHandler `json:"-"`
}

func (cfg *ClientCfg) ValidateJSON(v *json.Validator) {
	v.CheckOptionalObject("tls", cfg.TLS)

	if cfg.BaseURI != "" {
		v.CheckURI("base_uri", cfg.BaseURI)
	}

	if cfg.ConnectionTimeout != nil {
		v.CheckIntMin("connection_timeout", *cfg.ConnectionTimeout, 0)
	}

	if cfg.RequestTimeout != nil {
		v.CheckIntMin("request_timeout", *cfg.RequestTimeout, 0)
	}
}

type TLSClientCfg struct {
	CACertificates []string `json:"ca_certificates"`
}

func (cfg *TLSClientCfg) ValidateJSON(v *json.Validator) {
	v.WithChild("ca_certificates", func() {
		for i, path := range cfg.CACertificates {
			v.CheckStringNotEmpty(i, path)
		}
	})
}

type Client struct {
	Cfg *ClientCfg
	Log *log.Logger

	client  *http.Client
	tlsCfg  *tls.Config
	baseURI *url.URL
}

func NewClient(cfg *ClientCfg) (*Client, error) {
	if cfg.ConnectionTimeout == nil {
		cfg.ConnectionTimeout = new(DefaultClientConnectionTimeout)
	}

	if cfg.RequestTimeout == nil {
		cfg.RequestTimeout = new(DefaultClientRequestTimeout)
	}

	if cfg.Log == nil {
		cfg.Log = log.DefaultLogger()
	}

	dialer := net.Dialer{
		Timeout:   time.Duration(*cfg.ConnectionTimeout) * time.Second,
		KeepAlive: 30 * time.Second,
	}

	transport := http.Transport{
		DialContext: dialer.DialContext,

		MaxIdleConns: 100,

		IdleConnTimeout:       60 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	tlsCfg := tls.Config{}

	if cfg.TLS != nil {
		caCertificatePool, err := LoadCertificates(cfg.TLS.CACertificates)
		if err != nil {
			return nil, err
		}

		tlsCfg.RootCAs = caCertificatePool
	}

	client := http.Client{
		Timeout:   time.Duration(*cfg.RequestTimeout) * time.Second,
		Transport: NewRoundTripper(&transport, cfg),

		CheckRedirect: checkClientRedirect,
	}

	if cfg.DisableRedirections {
		client.CheckRedirect =
			func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			}
	}

	c := Client{
		Cfg: cfg,

		client: &client,
		tlsCfg: &tlsCfg,
	}

	if cfg.BaseURI != "" {
		baseURI, err := url.Parse(cfg.BaseURI)
		if err != nil {
			return nil, fmt.Errorf("cannot parse base URI: %w", err)
		}
		c.baseURI = baseURI
	}

	transport.DialTLSContext = c.dialTLSContext

	return &c, nil
}

func (c *Client) CloseConnections() {
	c.client.CloseIdleConnections()
}

func (c *Client) Do(req *http.Request) (*http.Response, error) {
	return c.client.Do(req)
}

func (c *Client) SendRequest(
	ctx context.Context, method, uriRef string, header http.Header,
	reqBody, resBody any,
) (*http.Response, error) {
	uri, err := c.resolveURI(uriRef)
	if err != nil {
		return nil, err
	}

	var bodyReader io.Reader

	if reqBody != nil {
		if data, ok := reqBody.([]byte); ok {
			bodyReader = bytes.NewReader(data)
		} else {
			if encoder := c.Cfg.RequestBodyEncoder; encoder != nil {
				data, err := encoder(reqBody)
				if err != nil {
					return nil,
						fmt.Errorf("cannot encode request body: %w", err)
				}

				bodyReader = bytes.NewReader(data)
			}
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, uri, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("cannot create request: %w", err)
	}

	for name, values := range header {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}

	res, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return res, fmt.Errorf("cannot read response body: %w", err)
	}

	// We want the caller to be able to decode the response body again if
	// necessary.
	res.Body = io.NopCloser(bytes.NewBuffer(data))

	if resBody != nil {
		if pData, ok := resBody.(*[]byte); ok {
			*pData = data
		} else {
			if decoder := c.Cfg.ResponseBodyDecoder; decoder != nil {
				if err := decoder(data, resBody); err != nil {
					return res,
						fmt.Errorf("cannot decode response body: %w", err)
				}
			}
		}
	}

	if handler := c.Cfg.ErrorHandler; handler != nil {
		if err := handler(res, data); err != nil {
			return res, err
		}
	}

	return res, nil
}

func (c *Client) resolveURI(uri string) (string, error) {
	if c.baseURI == nil {
		return uri, nil
	}

	ref, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("cannot parse URI: %w", err)
	}

	return c.baseURI.ResolveReference(ref).String(), nil
}

func (c *Client) dialTLSContext(
	ctx context.Context, network, address string,
) (net.Conn, error) {
	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{
			Timeout:   time.Duration(*c.Cfg.ConnectionTimeout) * time.Second,
			KeepAlive: 30 * time.Second,
		},

		Config: c.tlsCfg,
	}

	conn, err := dialer.DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func LoadCertificates(certificates []string) (*x509.CertPool, error) {
	pool := x509.NewCertPool()

	for _, certificate := range certificates {
		data, err := os.ReadFile(certificate)
		if err != nil {
			return nil, fmt.Errorf("cannot read %q: %w", certificate, err)
		}

		if pool.AppendCertsFromPEM(data) == false {
			return nil, fmt.Errorf("cannot load certificates from %q",
				certificate)
		}
	}

	return pool, nil
}

func checkClientRedirect(req *http.Request, via []*http.Request) error {
	// Strip authorization data when redirecting to a different host.
	if len(via) > 0 && req.URL.Host != via[0].URL.Host {
		req.Header.Del("Authorization")
	}

	return nil
}
