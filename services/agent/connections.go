package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/ezenkico/deploy-commander/runner/models"
	"github.com/google/uuid"
)

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
