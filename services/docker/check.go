package docker

import (
	"context"

	"github.com/ezenkico/deploy-commander/runner/models"
	"github.com/google/uuid"

	"github.com/moby/moby/client"
)

func (p *DockerPlatform) CheckVolumes(ctx context.Context, job string, services map[string]models.MetadataService, volumes *[]string) error {
	declared, err := DeclaredVolumeSet(volumes)
	if err != nil {
		return err
	}

	stragglers, err := CheckServiceVolumeMounts(services, declared)

	if err != nil {
		return err
	}

	// If volumes already exist in Docker, ensure they belong to this job
	if stragglers != nil && len(*stragglers) > 0 {
		if err := p.checkExistingDockerVolumes(ctx, job, *stragglers); err != nil {
			return err
		}
	}

	return nil
}

func (p *DockerPlatform) checkExistingDockerVolumes(
	ctx context.Context,
	jobID string,
	stragglers map[string]struct{},
) error {
	for logicalName, _ := range stragglers {
		volName := DockerVolumeName(jobID, logicalName) // uses "{job}-{volume}" naming

		_, err := p.client.VolumeInspect(ctx, volName, client.VolumeInspectOptions{})
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *DockerPlatform) CheckMetadata(ctx context.Context, job uuid.UUID, metadata *models.Metadata) error {
	if metadata == nil {
		return nil
	}

	if metadata.Services != nil && len(metadata.Services) > 0 {
		err := CheckDependsOnServicesExist(metadata.Services)
		if err != nil {
			return err
		}
		err = CheckCircularDependencies(metadata.Services)
		if err != nil {
			return err
		}
		err = p.CheckVolumes(ctx, job.String(), metadata.Services, metadata.Volumes)
		if err != nil {
			return err
		}
	}

	return nil
}
