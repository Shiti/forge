package agent

import (
	"github.com/rustic-ai/forge/forge-go/control"
	"github.com/rustic-ai/forge/forge-go/supervisor"
)

func buildOrgSupervisorFactory(statusStore supervisor.AgentStatusStore, defaultSupervisor, dataDir string, attachProcessTree bool) control.SupervisorFactory {
	return func(orgID string) supervisor.AgentSupervisor {
		opts := []supervisor.ProcessSupervisorOption{
			supervisor.WithOrganizationID(orgID),
			supervisor.WithWorkDirBase(dataDir),
		}
		if attachProcessTree {
			opts = append(opts, supervisor.WithAttachedProcessTree())
		}
		processSup := supervisor.NewProcessSupervisor(statusStore, opts...)

		var dockerSup *supervisor.DockerSupervisor
		if ds, err := supervisor.NewDockerSupervisor(statusStore); err == nil && ds.Available() {
			dockerSup = ds
		}

		var bwrapSup *supervisor.BubblewrapSupervisor
		bs := supervisor.NewBubblewrapSupervisor(statusStore)
		if bs.Available() {
			bwrapSup = bs
		}

		return supervisor.NewDispatchingSupervisor(defaultSupervisor, processSup, dockerSup, bwrapSup)
	}
}
