package docker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/containerd/errdefs"
	"github.com/google/uuid"

	"github.com/moby/moby/client"
)

func (p *DockerPlatform) RemoveServices(ctx context.Context, job uuid.UUID, removeServices *[]string) error {
	if removeServices == nil {
		return nil
	}

	resourceNames := make(map[string]struct{})

	for _, service := range *removeServices {
		containerName := DockerServiceName(job.String(), service)
		inspect, err := p.client.ContainerInspect(ctx, containerName, client.ContainerInspectOptions{})
		if err == nil {
			// Extract prior resources
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
					// If JSON is malformed, ignore silently (or log if you have logger available).
				}
			}

			// Stop (best-effort) then remove
			_, _ = p.client.ContainerStop(ctx, containerName, client.ContainerStopOptions{})
			_, err := p.client.ContainerRemove(ctx, containerName, client.ContainerRemoveOptions{
				Force:         true,
				RemoveVolumes: false,
			})
			if err != nil {
				return fmt.Errorf("remove existing container %q: %w", containerName, err)
			}
		}
	}

	if p.comm != nil {
		for resource, _ := range resourceNames {
			p.comm.DeleteResourceByName(ctx, resource)
		}
	}

	return nil
}

func (p *DockerPlatform) RemoveVolumes(ctx context.Context, job uuid.UUID, removeVolumes *[]string) error {
	if removeVolumes == nil {
		return nil
	}

	for _, volume := range *removeVolumes {
		if volume == "" {
			continue
		}

		volumeName := DockerVolumeName(job.String(), volume)

		// Idempotent remove:
		// - if it doesn't exist, ignore
		// - otherwise remove it
		if _, err := p.client.VolumeRemove(ctx, volumeName, client.VolumeRemoveOptions{}); err != nil {
			// If it was already gone, that's fine.
			if errdefs.IsNotFound(err) {
				continue
			}
			return fmt.Errorf("remove volume %q: %w", volumeName, err)
		}
	}

	return nil
}
