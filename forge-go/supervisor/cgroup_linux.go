//go:build linux

package supervisor

import (
	"fmt"
	"os"

	"github.com/rustic-ai/forge/forge-go/protocol"
)

// applyResourceLimits applies memory limits using cgroups v2 if available on Linux.
func applyResourceLimits(pid int, spec *protocol.AgentSpec) error {
	cgroupBase := "/sys/fs/cgroup"
	if _, err := os.Stat(cgroupBase); os.IsNotExist(err) {
		return fmt.Errorf("cgroups v2 not available")
	}

	agentCgroup := fmt.Sprintf("%s/forge-agent-%d", cgroupBase, pid)
	if err := os.MkdirAll(agentCgroup, 0755); err != nil {
		// If we can't create it due to permissions on a local machine, fail softly but return the error
		return fmt.Errorf("failed to create cgroup directory: %w", err)
	}

	// Write PID to cgroup.procs
	procsFile := fmt.Sprintf("%s/cgroup.procs", agentCgroup)
	if err := os.WriteFile(procsFile, []byte(fmt.Sprintf("%d", pid)), 0644); err != nil {
		return fmt.Errorf("failed to write pid to cgroup.procs: %w", err)
	}

	memLimit := "536870912" // 512 MB default
	pidsLimit := "100"      // default

	if spec != nil {
		if mem, ok := spec.Resources.CustomResources["memory"]; ok {
			memLimit = fmt.Sprintf("%v", mem)
		}

		if spec.Resources.NumCPUs != nil && *spec.Resources.NumCPUs > 0 {
			period := 100000
			max := int((*spec.Resources.NumCPUs) * float64(period))
			_ = os.WriteFile(fmt.Sprintf("%s/cpu.max", agentCgroup), []byte(fmt.Sprintf("%d %d", max, period)), 0644)
		}

		if pids, ok := spec.Resources.CustomResources["pids"]; ok {
			pidsLimit = fmt.Sprintf("%v", pids)
		}
	}

	_ = os.WriteFile(fmt.Sprintf("%s/memory.max", agentCgroup), []byte(memLimit), 0644)
	_ = os.WriteFile(fmt.Sprintf("%s/pids.max", agentCgroup), []byte(pidsLimit), 0644)

	return nil
}
