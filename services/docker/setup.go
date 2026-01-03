package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/containerd/errdefs"
	"github.com/ezenkico/deploy-commander/runner/models"
	"github.com/google/uuid"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

func (p *DockerPlatform) VolumeSetup(
	ctx context.Context,
	job uuid.UUID,
	run uuid.UUID,
	metadata *models.Metadata) error {
	if metadata == nil || metadata.Volumes == nil || len(*metadata.Volumes) == 0 {
		return nil
	}

	for _, volName := range *metadata.Volumes {
		name := DockerVolumeName(job.String(), volName)

		// If it already exists, treat as success.
		_, err := p.client.VolumeInspect(ctx, name, client.VolumeInspectOptions{})
		if err == nil {
			continue
		}
		if !errdefs.IsNotFound(err) {
			return fmt.Errorf("inspect volume %q: %w", name, err)
		}

		_, err = p.client.VolumeCreate(ctx, client.VolumeCreateOptions{
			Name: name,
			Labels: map[string]string{
				"deploy-commander.job":    job.String(),
				"deploy-commander.run":    run.String(),
				"deploy-commander.volume": volName, // original logical name
			},
		})
		if err != nil {
			// If it was created concurrently, Docker will return a conflict; we can just continue.
			// Rather than pattern match error strings, re-check inspect.
			if _, ie := p.client.VolumeInspect(ctx, name, client.VolumeInspectOptions{}); ie == nil {
				continue
			}
			return fmt.Errorf("create volume %q: %w", name, err)
		}
	}

	return nil
}

func (p *DockerPlatform) SetupService(
	ctx context.Context,
	job uuid.UUID,
	run uuid.UUID,
	createdNetworks map[string]struct{},
	serviceName string, // <-- pass the map key in (strongly recommended)
	service *models.MetadataService,
) (map[string]struct{}, error) {

	if service == nil {
		return createdNetworks, nil
	}

	isRunner := IsRunnerRole(service)

	// 1) Create or verify networks exist or create or verify the job network exists (simple start)
	networks := make(map[string]struct{})
	if service.NetworkGroups != nil {
		for _, group := range *service.NetworkGroups {
			netName := DockerNetworkName(job.String(), group) // {job}-{group}

			if _, ok := createdNetworks[netName]; !ok {
				_, err := p.client.NetworkInspect(ctx, netName, client.NetworkInspectOptions{})
				if err != nil {
					_, err = p.client.NetworkCreate(ctx, netName, client.NetworkCreateOptions{
						Labels: map[string]string{
							"deploy-commander.job":  job.String(),
							"deploy-commander.run":  run.String(),
							"deploy-commander.net":  group, // logical group name
							"deploy-commander.kind": "group",
						},
					})
					if err != nil {
						if _, ie := p.client.NetworkInspect(ctx, netName, client.NetworkInspectOptions{}); ie != nil {
							return createdNetworks, fmt.Errorf("create network %q: %w", netName, err)
						}
					}
				}
				createdNetworks[netName] = struct{}{}
			}

			networks[netName] = struct{}{}
		}
	}
	if service.Connections != nil {
		for _, conn := range *service.Connections {
			data := GetPlatformData(conn)
			if data == nil {
				continue
			}

			var pc models.DockerPlatformConnection
			if err := json.Unmarshal(*data, &pc); err != nil {
				return createdNetworks, fmt.Errorf("invalid platform connection data: %w", err)
			}
			if pc.Network == "" {
				return createdNetworks, fmt.Errorf("platform connection network is required")
			}

			// IMPORTANT: connection networks are created by other jobs.
			// Use the network name exactly as provided (no DockerNetworkName wrapping).
			netName := pc.Network

			// Verify network exists. If it doesn't, that's a metadata/config error.
			if _, err := p.client.NetworkInspect(ctx, netName, client.NetworkInspectOptions{}); err != nil {
				return createdNetworks, fmt.Errorf("platform connection network %q not found: %w", netName, err)
			}

			networks[netName] = struct{}{}
		}
	}
	resources := []models.CreateResource{}
	resourceNames := make(map[string]struct{})
	if service.Resources != nil {
		for _, spec := range *service.Resources {
			if isRunner {
				resources = append(resources, models.CreateResource{
					ResourceType:       spec.ResourceType,
					Name:               spec.Name,
					PlatformConnection: nil,
					PublicConnection:   spec.PublicConnection,
					Metadata:           spec.Metadata,
				})
				continue
			}
			netName := DockerNetworkResourceName(job.String(), spec.Name)

			// Check if the network exists. If not, create it (race-safe).
			if _, ok := createdNetworks[netName]; !ok {
				_, err := p.client.NetworkInspect(ctx, netName, client.NetworkInspectOptions{})
				if err != nil {
					_, err = p.client.NetworkCreate(ctx, netName, client.NetworkCreateOptions{
						Labels: map[string]string{
							"deploy-commander.job":  job.String(),
							"deploy-commander.run":  run.String(),
							"deploy-commander.net":  spec.Name, // resource name (useful for debugging)
							"deploy-commander.kind": "resource",
						},
					})
					if err != nil {
						// Race-safe: re-inspect
						if _, ie := p.client.NetworkInspect(ctx, netName, client.NetworkInspectOptions{}); ie != nil {
							return createdNetworks, fmt.Errorf("create resource network %q: %w", netName, err)
						}
					}
				}
				createdNetworks[netName] = struct{}{}
			}

			// Build the platform connection payload for this resource (Network-only).
			pc := models.DockerPlatformConnection{Network: netName}

			b, err := json.Marshal(pc)
			if err != nil {
				return createdNetworks, fmt.Errorf("marshal platform connection for resource %q: %w", spec.Name, err)
			}

			rm := json.RawMessage(b) // convert []byte -> json.RawMessage

			// Convert spec -> CreateResource
			resources = append(resources, models.CreateResource{
				ResourceType:       spec.ResourceType,
				Name:               spec.Name,
				PlatformConnection: &rm, // correct type: *json.RawMessage
				PublicConnection:   spec.PublicConnection,
				Metadata:           spec.Metadata,
			})

			resourceNames[spec.Name] = struct{}{}

			// The service must join this resource network so it can talk to the resource container.
			networks[netName] = struct{}{}
		}
	}
	if len(networks) < 1 {
		jobNet := job.String()
		if _, ok := createdNetworks[jobNet]; !ok {
			// Create network if needed
			_, err := p.client.NetworkInspect(ctx, jobNet, client.NetworkInspectOptions{})
			if err != nil {
				_, err = p.client.NetworkCreate(ctx, jobNet, client.NetworkCreateOptions{
					Labels: map[string]string{
						"deploy-commander.job": job.String(),
						"deploy-commander.run": run.String(),
					},
				})
				if err != nil {
					// Race-safe: re-inspect
					if _, ie := p.client.NetworkInspect(ctx, jobNet, client.NetworkInspectOptions{}); ie != nil {
						return createdNetworks, fmt.Errorf("create network %q: %w", jobNet, err)
					}
				}
			}
			createdNetworks[jobNet] = struct{}{}
		}
		networks[jobNet] = struct{}{}
	}

	// 2) Container name (job-scoped)
	containerName := DockerServiceName(job.String(), serviceName)

	// 3) Env
	env := []string{}
	if service.Environment != nil {
		for k, v := range service.Environment {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	// 4) Volume mounts (named volumes only; no host paths)
	mounts := []mount.Mount{}
	if service.Volumes != nil {
		for _, vm := range *service.Volumes {
			if strings.TrimSpace(vm.MountPath) == "" {
				return createdNetworks, fmt.Errorf("service %q volume mount_path is empty", serviceName)
			}
			target := vm.MountPath

			// Name == nil means runner-provided volume.
			// For docker, you can choose a deterministic named volume for it or skip for now.
			// Here: we create/use a deterministic runner volume per job.
			var source string
			if vm.Name == nil {
				source = DockerRunnerVolumeName(job.String())
			} else {
				source = DockerVolumeName(job.String(), *vm.Name)
			}

			mounts = append(mounts, mount.Mount{
				Type:   mount.TypeVolume,
				Source: source,
				Target: target,
			})
		}
	}

	// 5) Port bindings (minimal TCP only for now)
	exposed := network.PortSet{}
	portMap := network.PortMap{}

	portType := []network.IPProtocol{"tcp", "udp"}

	if service.Bindings != nil {
		for _, b := range *service.Bindings {
			// Need at least container port to expose in container config
			if b.ContainerPort == nil {
				continue
			}

			containerPort := *b.ContainerPort

			for _, t := range portType {
				port, _ := network.PortFrom(uint16(containerPort), t)

				exposed[port] = struct{}{}

				// host publish optional
				if b.HostPort != nil {
					hostPort := strconv.Itoa(*b.HostPort)
					hostIP := "0.0.0.0"
					if b.HostIP != nil {
						hostIP = *b.HostIP
					}

					addr, err := netip.ParseAddr(hostIP)
					if err != nil {
						return createdNetworks, fmt.Errorf("service %q has invalid host_ip %q: %w", serviceName, hostIP, err)
					}

					portMap[port] = append(portMap[port], network.PortBinding{
						HostIP:   addr,
						HostPort: hostPort,
					})
				}
			}
		}
	}

	// 6) Remove container if it exists
	inspect, err := p.client.ContainerInspect(ctx, containerName, client.ContainerInspectOptions{})
	if err == nil {
		// Extract prior resources (if labeled) so update/recreate doesn't lose them.
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
			return createdNetworks, fmt.Errorf("remove existing container %q: %w", containerName, err)
		}
	}

	// 7) Labels
	labels := map[string]string{
		"deploy-commander.job":     job.String(),
		"deploy-commander.run":     run.String(),
		"deploy-commander.service": serviceName,
	}

	namesLength := len(resourceNames)

	if namesLength > 0 {
		names := make([]string, 0, namesLength)
		for name := range resourceNames {
			names = append(names, name)
		}

		b, err := json.Marshal(names)
		if err != nil {
			return nil, fmt.Errorf("marshal resource names label: %w", err)
		}

		labels["deploy-commander.resources"] = string(b)
	}

	// 8) Container configs
	cCfg := &container.Config{
		Image:        service.Image,
		Env:          env,
		Labels:       labels,
		ExposedPorts: exposed,
	}

	hCfg := &container.HostConfig{
		Mounts:       mounts,
		PortBindings: portMap,
		RestartPolicy: container.RestartPolicy{
			Name: container.RestartPolicyAlways,
		},
	}

	if isRunner {
		hCfg.RestartPolicy = container.RestartPolicy{
			Name: container.RestartPolicyDisabled,
		}
	}

	endpointConfigs := make(map[string]*network.EndpointSettings)
	for net := range networks {
		es := &network.EndpointSettings{}
		if service.Aliases != nil && len(*service.Aliases) > 0 {
			es.Aliases = *service.Aliases
		}
		endpointConfigs[net] = es
	}

	nCfg := &network.NetworkingConfig{
		EndpointsConfig: endpointConfigs,
	}

	containerID := ""

	// 9) Create container

	// Now create a fresh container
	created, err := p.client.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:           cCfg,
		HostConfig:       hCfg,
		NetworkingConfig: nCfg,
		Name:             containerName,
		Image:            service.Image,
	})
	if err != nil {
		// Race-safe: if something else created it, inspect and proceed
		inspected, ie := p.client.ContainerInspect(ctx, containerName, client.ContainerInspectOptions{})
		if ie != nil {
			return createdNetworks, fmt.Errorf("create container %q: %w", containerName, err)
		}
		containerID = inspected.Container.ID
	} else {
		containerID = created.ID
	}

	// Start the container
	if _, err := p.client.ContainerStart(ctx, containerID, client.ContainerStartOptions{}); err != nil {
		return createdNetworks, fmt.Errorf("start container %q: %w", containerName, err)
	}

	// 10) If runner
	if isRunner {
		// Stream logs while it runs
		rc, err := p.client.ContainerLogs(ctx, containerID, client.ContainerLogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true,
			Timestamps: false,
			Since:      "0",
		})
		if err != nil {
			return createdNetworks, fmt.Errorf("logs container %q: %w", containerName, err)
		}
		defer rc.Close()

		logDone := make(chan error, 1)
		go func() {
			logDone <- DemuxDockerLogs(os.Stdout, os.Stderr, rc)
		}()

		// Wait for completion
		waitBodyC := p.client.ContainerWait(ctx, containerID, client.ContainerWaitOptions{})
		var statusCode int64

		select {
		case err := <-waitBodyC.Error:
			if err != nil {
				return createdNetworks, fmt.Errorf("wait container %q: %w", containerName, err)
			}
		case res := <-waitBodyC.Result:
			statusCode = res.StatusCode
		}

		// Ensure log stream finishes (usually ends when container exits)
		if err := <-logDone; err != nil {
			// If the container exited, sometimes the log stream ends with EOF — that's fine.
			// io.Copy returns nil on clean EOF; anything else is worth surfacing.
			return createdNetworks, fmt.Errorf("stream logs for %q: %w", containerName, err)
		}

		// Remove container after completion
		if _, err := p.client.ContainerRemove(ctx, containerID, client.ContainerRemoveOptions{
			Force:         true,
			RemoveVolumes: false,
		}); err != nil {
			return createdNetworks, fmt.Errorf("remove container %q: %w", containerName, err)
		}

		// If it failed, surface that as an error after logs are printed
		if statusCode != 0 {
			return createdNetworks, fmt.Errorf("runner container %q exited with status %d", containerName, statusCode)
		}

	}

	// 11) Setup the resources
	if p.comm != nil {
		for _, resource := range resources {
			_, err := p.comm.CreateResource(ctx, resource)
			if err != nil {
				return createdNetworks, fmt.Errorf("Failed to send resource %s", resource.Name)
			}
		}
	}

	return createdNetworks, nil
}

func (p *DockerPlatform) ServiceSetup(ctx context.Context,
	job uuid.UUID,
	run uuid.UUID,
	metadata *models.Metadata) error {

	services := metadata.Services

	if services == nil {
		return nil
	}

	ranServices := []string{}
	createdNetworks := make(map[string]struct{})
	var err error = nil

	for len(services) > 0 {
		notRun := make(map[string]models.MetadataService)

		for name, service := range services {
			if service.DependsOn != nil {
				cantRun := false
				for _, dependency := range *service.DependsOn {
					if !slices.Contains(ranServices, dependency) {
						cantRun = true
						break
					}
				}
				if cantRun {
					notRun[name] = service
					continue
				}
			}

			createdNetworks, err = p.SetupService(ctx, job, run, createdNetworks, name, &service)
			if err != nil {
				return err
			}
			ranServices = append(ranServices, name)
		}
		services = notRun
	}

	return nil
}

func (p *DockerPlatform) SetupConnections(ctx context.Context, connectionPlan *models.ConnectionPlan) error {
	if connectionPlan == nil {
		return nil
	}
	comm := p.comm
	if comm == nil {
		return nil
	}

	// Helper: resolve ResourceRef -> resource UUID
	resolveResourceID := func(ref models.ResourceRef) (uuid.UUID, error) {
		if ref.ID != nil {
			return *ref.ID, nil
		}
		// At the runner layer, we currently have no lookup mechanism for (Service, Name) -> UUID.
		// If you later add one (e.g. comm.ResolveResource(service,name)), plug it in here.
		if ref.Service != nil || ref.Name != nil {
			return uuid.Nil, fmt.Errorf("cannot resolve resource by service/name in runner; ResourceRef.id is required (service=%v name=%v)", ref.Service, ref.Name)
		}
		return uuid.Nil, fmt.Errorf("resource ref is empty; ResourceRef.id is required")
	}

	// 1) Post new connections
	if connectionPlan.Create != nil {
		for _, spec := range *connectionPlan.Create {
			resourceID, err := resolveResourceID(spec.Resource)
			if err != nil {
				return fmt.Errorf("create connection: %w", err)
			}

			_, err = comm.CreateConnection(ctx, models.CreateConnectionRequest{
				Resource: resourceID,
				Job:      spec.Job,
				Metadata: spec.Metadata,
			})
			if err != nil {
				return fmt.Errorf("create connection (resource=%s job=%s): %w", resourceID, spec.Job, err)
			}
		}
	}

	// 2) Remove connections
	if connectionPlan.Remove != nil {
		for _, spec := range *connectionPlan.Remove {
			// With the current comm API, DeleteConnection needs BOTH the resource UUID and the connection UUID.
			// So we only support remove when spec includes BOTH:
			// - spec.ID (connection id)
			// - spec.Resource (to resolve resource id)
			if spec.ID != nil {
				if spec.Resource == nil {
					return fmt.Errorf("remove connection %s: resource ref is required (DeleteConnection needs resourceID + connectionID)", spec.ID.String())
				}
				resourceID, err := resolveResourceID(*spec.Resource)
				if err != nil {
					return fmt.Errorf("remove connection %s: %w", spec.ID.String(), err)
				}

				if err := comm.DeleteConnection(ctx, resourceID, *spec.ID); err != nil {
					return fmt.Errorf("delete connection (resource=%s id=%s): %w", resourceID, spec.ID.String(), err)
				}
				continue
			}

			// Resource-only removal ("remove all connections for resource") is not possible with the current comm API
			// because we have no "list connections for resource" endpoint here.
			if spec.Resource != nil {
				return fmt.Errorf("remove connections for resource: unsupported with current API (need list-connections or delete-by-resource endpoint)")
			}
		}
	}

	return nil
}
