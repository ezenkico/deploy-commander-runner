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
