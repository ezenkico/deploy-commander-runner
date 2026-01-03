package docker

import (
	"context"

	"github.com/ezenkico/deploy-commander/runner/models"
	"github.com/ezenkico/deploy-commander/runner/services/agent"

	"github.com/moby/moby/client"
)

// DockerPlatform implements interfaces.Platform for plain Docker (Engine API).
type DockerPlatform struct {
	client *client.Client
	comm   *agent.AgentCommunication
}

// NewDockerPlatform initializes the Docker platform using environment variables
// (e.g. DOCKER_HOST) and API version negotiation.
func NewDockerPlatform(comm *agent.AgentCommunication) (*DockerPlatform, error) {
	c, err := client.New(
		client.FromEnv,
	)
	if err != nil {
		return nil, err
	}

	return &DockerPlatform{
		client: c,
		comm:   comm,
	}, nil
}

// Run executes the requested action (run/teardown/update) for the given configuration.
// Implementation will be filled in later.
func (p *DockerPlatform) Run(ctx context.Context, config models.Configuration) error {
	if config.Action == "teardown" {
		return p.Teardown(ctx, config.Job)
	}
	metadata := config.Metadata
	if metadata != nil {
		err := p.CheckMetadata(ctx, config.Job, metadata)
		if err != nil {
			return err
		}

		err = p.VolumeSetup(ctx, config.Job, config.Run, metadata)
		if err != nil {
			return err
		}
		err = p.ServiceSetup(ctx, config.Job, config.Run, metadata)
		if err != nil {
			return err
		}
		err = p.RemoveServices(ctx, config.Job, metadata.RemoveServices)
		if err != nil {
			return err
		}
		err = p.RemoveVolumes(ctx, config.Job, metadata.RemoveVolumes)
		if err != nil {
			return err
		}
		err = p.SetupConnections(ctx, metadata.Connections)
		if err != nil {
			return err
		}
	}

	return nil
}
