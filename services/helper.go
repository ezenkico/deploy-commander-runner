package services

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/ezenkico/deploy-commander/runner/models"
)

func DemuxDockerLogs(dstOut, dstErr io.Writer, src io.Reader) error {
	r := bufio.NewReader(src)

	header := make([]byte, 8)
	for {
		// Read header
		if _, err := io.ReadFull(r, header); err != nil {
			// Clean EOF: stream ends
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return nil
			}
			return err
		}

		streamType := header[0] // 1=stdout, 2=stderr
		size := binary.BigEndian.Uint32(header[4:8])

		if size == 0 {
			continue
		}

		payload := make([]byte, size)
		if _, err := io.ReadFull(r, payload); err != nil {
			return err
		}

		var w io.Writer
		switch streamType {
		case 1:
			w = dstOut
		case 2:
			w = dstErr
		default:
			// Unknown stream, treat as stdout to avoid dropping data
			w = dstOut
		}

		if _, err := w.Write(payload); err != nil {
			return fmt.Errorf("write docker log payload: %w", err)
		}
	}
}

func IsRunnerRole(service *models.MetadataService) bool {
	if service == nil || service.Role == nil {
		return false
	}
	return *service.Role == models.ServiceRoleRunner
}

func DockerServiceName(jobID, serviceKey string) string {
	return fmt.Sprintf("%s-%s", jobID, strings.TrimSpace(serviceKey))
}

func DockerNetworkName(jobID string, name string) string {
	return fmt.Sprintf("%s-%s", jobID, name)
}

func DockerNetworkResourceName(jobID string, name string) string {
	return fmt.Sprintf("%s-%s-resource", jobID, name)
}

func DockerRunnerVolumeName(jobID string) string {
	return fmt.Sprintf("%s-runner", jobID)
}

func DockerVolumeName(jobID, volumeName string) string {
	// Keep names docker-friendly and deterministic.
	safe := func(s string) string {
		s = strings.ToLower(strings.TrimSpace(s))
		s = strings.ReplaceAll(s, " ", "-")
		return s
	}
	return fmt.Sprintf("dc-%s-%s", safe(jobID), safe(volumeName))
}

func CheckDependsOnServicesExist(services map[string]models.MetadataService) error {
	// Stable iteration (nicer error messages)
	keys := make([]string, 0, len(services))
	for k := range services {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, svcKey := range keys {
		svc := services[svcKey]
		if svc.DependsOn == nil || len(*svc.DependsOn) == 0 {
			continue
		}

		for _, depKey := range *svc.DependsOn {
			if _, ok := services[depKey]; !ok {
				return fmt.Errorf("service %q depends_on %q, but %q does not exist", svcKey, depKey, depKey)
			}
		}
	}

	return nil
}

func GetPlatformData(connection models.ResourceConnection) *json.RawMessage {
	if connection.Type != models.ResourceConnectionTypePlatform {
		return nil
	}
	return &connection.Data
}

func CheckCircularDependencies(services map[string]models.MetadataService) error {
	const (
		unvisited = 0
		visiting  = 1
		visited   = 2
	)

	state := make(map[string]uint8, len(services))
	parent := make(map[string]string, len(services))

	var dfs func(string) error
	dfs = func(node string) error {
		switch state[node] {
		case visiting:
			// Found a back-edge; reconstruct cycle path using parent pointers.
			cycle := reconstructCycle(parent, node)
			return fmt.Errorf("circular dependency detected: %s", cycle)
		case visited:
			return nil
		}

		state[node] = visiting

		svc := services[node]
		if svc.DependsOn != nil {
			for _, dep := range *svc.DependsOn {
				// Existence is checked elsewhere; skip unknown just in case.
				if _, ok := services[dep]; !ok {
					continue
				}
				// Track parent for reconstruction (only set if not already set).
				if _, ok := parent[dep]; !ok {
					parent[dep] = node
				}
				if err := dfs(dep); err != nil {
					return err
				}
			}
		}

		state[node] = visited
		return nil
	}

	for node := range services {
		if state[node] == unvisited {
			if err := dfs(node); err != nil {
				return err
			}
		}
	}

	return nil
}

func reconstructCycle(parent map[string]string, start string) string {
	// Walk parent pointers until we repeat a node.
	// Build list in reverse then format.
	seen := map[string]bool{start: true}
	path := []string{start}

	cur := start
	for {
		p, ok := parent[cur]
		if !ok {
			// Fallback; shouldn't happen with a proper parent chain
			break
		}
		path = append(path, p)
		if seen[p] {
			// Close cycle at p
			break
		}
		seen[p] = true
		cur = p
	}

	// path currently like: start, parent(start), parent(...), ..., repeatedNode
	// Reverse to make it read forward, then ensure closure at end.
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}

	// Ensure last equals first for readability
	if len(path) > 0 && path[len(path)-1] != path[0] {
		path = append(path, path[0])
	}

	// Join manually to avoid extra deps
	out := ""
	for i, s := range path {
		if i > 0 {
			out += " -> "
		}
		out += fmt.Sprintf("%q", s)
	}
	return out
}

func DeclaredVolumeSet(vols *[]string) (map[string]struct{}, error) {
	set := map[string]struct{}{}
	if vols == nil {
		return set, nil
	}

	for _, v := range *vols {
		name := strings.TrimSpace(v)
		if name == "" {
			return nil, fmt.Errorf("metadata.volumes contains an empty name")
		}
		if _, exists := set[name]; exists {
			return nil, fmt.Errorf("metadata.volumes contains duplicate volume %q", name)
		}
		set[name] = struct{}{}
	}

	return set, nil
}

func CheckServiceVolumeMounts(services map[string]models.MetadataService, declared map[string]struct{}) (*map[string]struct{}, error) {
	stragglers := make(map[string]struct{})
	for svcKey, svc := range services {
		if svc.Volumes == nil || len(*svc.Volumes) == 0 {
			continue
		}

		// Ensure no duplicate mount paths inside a service
		seenMountPath := map[string]struct{}{}

		for _, m := range *svc.Volumes {
			mountPath := strings.TrimSpace(m.MountPath)
			if mountPath == "" {
				return nil, fmt.Errorf("service %q has a volume with empty mount_path", svcKey)
			}
			if !strings.HasPrefix(mountPath, "/") {
				return nil, fmt.Errorf("service %q volume mount_path %q must be absolute", svcKey, mountPath)
			}
			if _, ok := seenMountPath[mountPath]; ok {
				return nil, fmt.Errorf("service %q has duplicate volume mount_path %q", svcKey, mountPath)
			}
			seenMountPath[mountPath] = struct{}{}

			// Name == nil means runner-provided volume (allowed)
			if m.Name == nil {
				continue
			}

			name := strings.TrimSpace(*m.Name)
			if name == "" {
				return nil, fmt.Errorf("service %q has a volume with empty name", svcKey)
			}

			// Must be declared in metadata.volumes
			if _, ok := declared[name]; !ok {
				stragglers[name] = struct{}{}
			}
		}
	}

	return &stragglers, nil
}
