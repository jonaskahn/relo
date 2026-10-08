//go:build !darwin && !windows

package codingclients

func applyLiveEnv(Paths, Agent) error { return nil }

func clearLiveEnv(Paths, Agent) error { return nil }

func envPublished(paths Paths, agent Agent) bool {
	return shellPublished(paths, agent.ID)
}
