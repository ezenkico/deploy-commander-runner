package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type AgentCommunication struct {
	Endpoint string
	Type     string // tcp or unix

	SocketPath string
	HostPort   string
	BaseURL    string

	Token string // bearer token
}

// NewAgentCommunicationFromEnv loads and parses AGENT_ENDPOINT.
func NewAgentCommunicationFromEnv() (*AgentCommunication, error) {
	endpoint := strings.TrimSpace(os.Getenv("AGENT_ENDPOINT"))
	if endpoint == "" {
		return nil, errors.New("AGENT_ENDPOINT is not set")
	}

	token := strings.TrimSpace(os.Getenv("TOKEN"))
	if token == "" {
		return nil, errors.New("TOKEN is not set")
	}

	ac, err := NewAgentCommunication(endpoint)
	if err != nil {
		return nil, err
	}

	ac.Token = token
	return ac, nil
}

// NewAgentCommunication parses an endpoint like:
//
//	unix:///var/run/agent.sock
//	tcp://example.com:8080
func NewAgentCommunication(endpoint string) (*AgentCommunication, error) {
	u, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil {
		return nil, fmt.Errorf("invalid AGENT_ENDPOINT %q: %w", endpoint, err)
	}

	ac := &AgentCommunication{Endpoint: endpoint}

	switch strings.ToLower(u.Scheme) {
	case "unix":
		// url.Parse treats unix:///path as Path="/path"
		if u.Path == "" {
			return nil, fmt.Errorf("unix endpoint missing socket path: %q", endpoint)
		}
		ac.Type = "unix"
		ac.SocketPath = u.Path

		// For HTTP requests over a unix socket, the URL host is ignored by the transport,
		// but net/http requires a valid URL. Use a stable dummy host.
		ac.BaseURL = "http://agent"

	case "tcp":
		// tcp://host:port
		if u.Host == "" {
			return nil, fmt.Errorf("tcp endpoint missing host:port: %q", endpoint)
		}
		ac.Type = "tcp"
		ac.HostPort = u.Host
		ac.BaseURL = "http://" + u.Host

	default:
		return nil, fmt.Errorf("unsupported AGENT_ENDPOINT scheme %q (use unix:// or tcp://)", u.Scheme)
	}

	return ac, nil
}

// Client returns an *http.Client configured to talk to the agent over tcp or unix,
// plus the BaseURL to use for requests.
func (a *AgentCommunication) Client() (*http.Client, string, error) {
	switch a.Type {
	case "tcp":
		// Plain HTTP over TCP. (If you later want TLS, you can switch BaseURL to https://
		// and configure TLS settings on the Transport.)
		return &http.Client{
			Timeout: 60 * time.Second,
		}, a.BaseURL, nil

	case "unix":
		// HTTP over Unix domain socket via custom DialContext.
		dialer := &net.Dialer{Timeout: 10 * time.Second}

		tr := &http.Transport{
			// IMPORTANT: ignore the addr and always dial the unix socket path
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.DialContext(ctx, "unix", a.SocketPath)
			},
		}

		return &http.Client{
			Transport: tr,
			Timeout:   60 * time.Second,
		}, a.BaseURL, nil

	default:
		return nil, "", fmt.Errorf("invalid agent communication type %q", a.Type)
	}
}

func (a *AgentCommunication) NewRequest(
	ctx context.Context,
	method string,
	path string,
	body io.Reader,
) (*http.Request, error) {

	req, err := http.NewRequestWithContext(
		ctx,
		method,
		a.BaseURL+path,
		body,
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+a.Token)
	req.Header.Set("Content-Type", "application/json")

	return req, nil
}
