package docker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/containerd/errdefs"
	"github.com/google/uuid"

	"github.com/moby/moby/client"
)

func (p *DockerPlatform) TearDownServices(ctx context.Context, job uuid.UUID) error {
	resourceNames := make(map[string]struct{})

	// Get services from job (containers with the job in the label "deploy-commander.job")
	f := make(client.Filters).
		Add("label", "deploy-commander.job="+job.String())

	containers, err := p.client.ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: f,
	})
	if err != nil {
		return fmt.Errorf("list job containers (job=%s): %w", job.String(), err)
	}

	// For each service:
	// - extract resources
	// - stop + remove container
	for _, c := range containers.Items {
		inspect, err := p.client.ContainerInspect(ctx, c.ID, client.ContainerInspectOptions{})
		if err != nil {
			// If it vanished between list and inspect, ignore.
			if errdefs.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("inspect container %q: %w", c.ID, err)
		}

		// Extract resource names from labels (Option A JSON label).
		if inspect.Container.Config != nil && inspect.Container.Config.Labels != nil {
			if v, ok := inspect.Container.Config.Labels["deploy-commander.resources"]; ok && v != "" {
				var names []string
				if je := json.Unmarshal([]byte(v), &names); je == nil {
					for _, n := range names {
						if n != "" {
							resourceNames[n] = struct{}{}
						}
					}
				}
			}
		}

		// Stop (best-effort) then remove
		_, _ = p.client.ContainerStop(ctx, c.ID, client.ContainerStopOptions{})
		_, err = p.client.ContainerRemove(ctx, c.ID, client.ContainerRemoveOptions{
			Force:         true,
			RemoveVolumes: false,
		})
		if err != nil && !errdefs.IsNotFound(err) {
			return fmt.Errorf("remove container %q: %w", c.ID, err)
		}
	}

	if p.comm != nil {
		for resource, _ := range resourceNames {
			p.comm.DeleteResourceByName(ctx, resource)
		}
	}

	return nil
}

func (p *DockerPlatform) TearDownVolumes(ctx context.Context, job uuid.UUID) error {
	// Get volumes for the job (volumes with the job in the label "deploy-commander.job")
	f := make(client.Filters).
		Add("label", "deploy-commander.job="+job.String())

	vols, err := p.client.VolumeList(ctx, client.VolumeListOptions{
		Filters: f,
	})
	if err != nil {
		return fmt.Errorf("list job volumes (job=%s): %w", job.String(), err)
	}

	// Remove each volume
	for _, v := range vols.Items {
		if v.Name == "" {
			continue
		}

		if _, err := p.client.VolumeRemove(ctx, v.Name, client.VolumeRemoveOptions{}); err != nil {
			// Idempotent: if it vanished, ignore.
			if errdefs.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("remove volume %q: %w", v.Name, err)
		}
	}

	return nil
}

func (p *DockerPlatform) TearDownNetworks(ctx context.Context, job uuid.UUID) error {
	// Get networks for the job (networks with the job in the label "deploy-commander.job")
	f := make(client.Filters).
		Add("label", "deploy-commander.job="+job.String())

	nets, err := p.client.NetworkList(ctx, client.NetworkListOptions{
		Filters: f,
	})
	if err != nil {
		return fmt.Errorf("list job networks (job=%s): %w", job.String(), err)
	}

	// Remove each network
	for _, n := range nets.Items {
		if n.Name == "" || n.ID == "" {
			continue
		}

		// Prefer removing by ID to avoid name collisions.
		if _, err := p.client.NetworkRemove(ctx, n.ID, client.NetworkRemoveOptions{}); err != nil {
			// Idempotent: if it vanished, ignore.
			if errdefs.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("remove network %q (%s): %w", n.Name, n.ID, err)
		}
	}

	return nil
}

func (p *DockerPlatform) Teardown(ctx context.Context, job uuid.UUID) error {

	err := p.TearDownServices(ctx, job)
	if err != nil {
		return err
	}
	err = p.TearDownVolumes(ctx, job)
	if err != nil {
		return err
	}
	err = p.TearDownNetworks(ctx, job)
	if err != nil {
		return err
	}

	return nil
}
