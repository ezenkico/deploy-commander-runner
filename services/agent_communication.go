package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ezenkico/deploy-commander/runner/models"
	"github.com/google/uuid"
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

// Resource interactions
const agentResourcesPath = "/v1/resources"

func (a *AgentCommunication) CreateResource(
	ctx context.Context,
	resource models.CreateResource,
) (uuid.UUID, error) {

	client, _, err := a.Client()
	if err != nil {
		return uuid.Nil, err
	}

	body, err := json.Marshal(resource)
	if err != nil {
		return uuid.Nil, err
	}

	req, err := a.NewRequest(
		ctx,
		http.MethodPost,
		agentResourcesPath,
		bytes.NewReader(body),
	)
	if err != nil {
		return uuid.Nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return uuid.Nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return uuid.Nil, fmt.Errorf("create resource failed (%d): %s", resp.StatusCode, string(b))
	}

	var out struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return uuid.Nil, err
	}

	return out.ID, nil
}

func (a *AgentCommunication) ListResources(
	ctx context.Context,
	resourceType *string,
	limit *uint32,
	offset *uint32,
) ([]uuid.UUID, error) {

	client, baseURL, err := a.Client()
	if err != nil {
		return nil, err
	}

	u, err := url.Parse(baseURL + agentResourcesPath)
	if err != nil {
		return nil, err
	}

	q := u.Query()
	if resourceType != nil {
		q.Set("resource_type", *resourceType)
	}
	if limit != nil {
		q.Set("limit", fmt.Sprintf("%d", *limit))
	}
	if offset != nil {
		q.Set("offset", fmt.Sprintf("%d", *offset))
	}
	u.RawQuery = q.Encode()

	req, err := a.NewRequest(ctx, http.MethodGet, u.RequestURI(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list resources failed (%d): %s", resp.StatusCode, string(b))
	}

	var ids []uuid.UUID
	if err := json.NewDecoder(resp.Body).Decode(&ids); err != nil {
		return nil, err
	}

	return ids, nil
}

func (a *AgentCommunication) GetResource(
	ctx context.Context,
	id uuid.UUID,
) (*models.Resource, error) {

	client, _, err := a.Client()
	if err != nil {
		return nil, err
	}

	req, err := a.NewRequest(
		ctx,
		http.MethodGet,
		fmt.Sprintf("%s/%s", agentResourcesPath, id.String()),
		nil,
	)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get resource failed (%d): %s", resp.StatusCode, string(b))
	}

	var resource models.Resource
	if err := json.NewDecoder(resp.Body).Decode(&resource); err != nil {
		return nil, err
	}

	return &resource, nil
}

func (a *AgentCommunication) DeleteResource(
	ctx context.Context,
	id uuid.UUID,
) error {

	client, _, err := a.Client()
	if err != nil {
		return err
	}

	req, err := a.NewRequest(
		ctx,
		http.MethodDelete,
		fmt.Sprintf("%s/%s", agentResourcesPath, id.String()),
		nil,
	)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete resource failed (%d): %s", resp.StatusCode, string(b))
	}

	return nil
}

func (a *AgentCommunication) DeleteResourceByName(
	ctx context.Context,
	name string,
) error {

	client, _, err := a.Client()
	if err != nil {
		return err
	}

	req, err := a.NewRequest(
		ctx,
		http.MethodDelete,
		fmt.Sprintf("%s/name/%s", agentResourcesPath, name),
		nil,
	)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete resource failed (%d): %s", resp.StatusCode, string(b))
	}

	return nil
}

// Connection Interactions
const agentConnectionsPath = "/v1/connections"

func (a *AgentCommunication) CreateConnection(
	ctx context.Context,
	body models.CreateConnectionRequest,
) (uuid.UUID, error) {

	client, _, err := a.Client()
	if err != nil {
		return uuid.Nil, err
	}

	b, err := json.Marshal(body)
	if err != nil {
		return uuid.Nil, err
	}

	req, err := a.NewRequest(ctx, http.MethodPost, agentConnectionsPath, bytes.NewReader(b))
	if err != nil {
		return uuid.Nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return uuid.Nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		rb, _ := io.ReadAll(resp.Body)
		return uuid.Nil, fmt.Errorf("create connection failed (%d): %s", resp.StatusCode, string(rb))
	}

	var out models.CreateConnectionResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return uuid.Nil, err
	}

	return out.ID, nil
}

func (a *AgentCommunication) ListConnections(
	ctx context.Context,
	job *uuid.UUID,
	resource *uuid.UUID,
	limit *uint32,
	offset *uint32,
) ([]uuid.UUID, error) {

	client, baseURL, err := a.Client()
	if err != nil {
		return nil, err
	}

	u, err := url.Parse(baseURL + agentConnectionsPath)
	if err != nil {
		return nil, err
	}

	q := u.Query()
	if job != nil {
		q.Set("job", job.String())
	}
	if resource != nil {
		q.Set("resource", resource.String())
	}
	if limit != nil {
		q.Set("limit", fmt.Sprintf("%d", *limit))
	}
	if offset != nil {
		q.Set("offset", fmt.Sprintf("%d", *offset))
	}
	u.RawQuery = q.Encode()

	req, err := a.NewRequest(ctx, http.MethodGet, u.RequestURI(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		rb, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list connections failed (%d): %s", resp.StatusCode, string(rb))
	}

	var ids []uuid.UUID
	if err := json.NewDecoder(resp.Body).Decode(&ids); err != nil {
		return nil, err
	}

	return ids, nil
}

func (a *AgentCommunication) GetConnection(
	ctx context.Context,
	resourceID uuid.UUID,
	id uuid.UUID,
) (*models.Connection, error) {

	client, _, err := a.Client()
	if err != nil {
		return nil, err
	}

	path := fmt.Sprintf("%s/%s/%s", agentConnectionsPath, resourceID.String(), id.String())
	req, err := a.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		rb, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get connection failed (%d): %s", resp.StatusCode, string(rb))
	}

	var conn models.Connection
	if err := json.NewDecoder(resp.Body).Decode(&conn); err != nil {
		return nil, err
	}

	return &conn, nil
}

func (a *AgentCommunication) DeleteConnection(
	ctx context.Context,
	resourceID uuid.UUID,
	id uuid.UUID,
) error {

	client, _, err := a.Client()
	if err != nil {
		return err
	}

	path := fmt.Sprintf("%s/%s/%s", agentConnectionsPath, resourceID.String(), id.String())
	req, err := a.NewRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		rb, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete connection failed (%d): %s", resp.StatusCode, string(rb))
	}

	return nil
}
