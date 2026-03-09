package supervisor

import (
	"os"
	"testing"

	"github.com/rustic-ai/forge/forge-go/registry"
)

func TestBuildBwrapArgs(t *testing.T) {

	homeDir, _ := os.UserHomeDir()
	uvToolDir := homeDir + "/.local/share/uv"
	uvCacheDir := homeDir + "/.cache/uv"
	forgeDir := homeDir + "/.forge"

	baseArgs := []string{
		"--unshare-all",
		"--ro-bind", "/", "/",
		"--dev", "/dev",
		"--proc", "/proc",
		"--tmpfs", "/tmp",
		"--die-with-parent",
	}

	tests := []struct {
		name     string
		entry    *registry.AgentRegistryEntry
		cmd      []string
		expected []string
	}{
		{
			name: "Airgapped Network (Empty)",
			entry: &registry.AgentRegistryEntry{
				Network: []string{},
			},
			cmd: []string{"echo", "hello"},
			expected: append(append([]string{}, baseArgs...),
				"--bind", uvToolDir, uvToolDir,
				"--bind", uvCacheDir, uvCacheDir,
				"--bind", forgeDir, forgeDir,
				"--", "echo", "hello",
			),
		},
		{
			name: "Airgapped Network (Explicit None)",
			entry: &registry.AgentRegistryEntry{
				Network: []string{"none"},
			},
			cmd: []string{"echo", "hello"},
			expected: append(append([]string{}, baseArgs...),
				"--bind", uvToolDir, uvToolDir,
				"--bind", uvCacheDir, uvCacheDir,
				"--bind", forgeDir, forgeDir,
				"--", "echo", "hello",
			),
		},
		{
			name: "Shared Network (Host)",
			entry: &registry.AgentRegistryEntry{
				Network: []string{"api.openai.com"},
			},
			cmd: []string{"python", "-m", "agent"},
			expected: append(append(append([]string{}, baseArgs...), "--share-net"),
				"--bind", uvToolDir, uvToolDir,
				"--bind", uvCacheDir, uvCacheDir,
				"--bind", forgeDir, forgeDir,
				"--", "python", "-m", "agent",
			),
		},
		{
			name: "Custom Filesystem Binds",
			entry: &registry.AgentRegistryEntry{
				Network: []string{},
				Filesystem: []registry.FilesystemPermission{
					{Path: "/app/data", Mode: "rw"},
					{Path: "/app/config", Mode: "ro"},
				},
			},
			cmd: []string{"python"},
			expected: append(append([]string{}, baseArgs...),
				"--bind", "/app/data", "/app/data",
				"--ro-bind", "/app/config", "/app/config",
				"--bind", uvToolDir, uvToolDir,
				"--bind", uvCacheDir, uvCacheDir,
				"--bind", forgeDir, forgeDir,
				"--", "python",
			),
		},
	}

	sup := NewBubblewrapSupervisor(nil)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sup.buildBwrapArgs(tc.entry, tc.cmd)

			if len(got) != len(tc.expected) {
				t.Fatalf("buildBwrapArgs() len = %d, want %d\n  got:  %v\n  want: %v", len(got), len(tc.expected), got, tc.expected)
			}
			for i := range got {
				if got[i] != tc.expected[i] {
					t.Errorf("buildBwrapArgs()[%d] = %q, want %q", i, got[i], tc.expected[i])
				}
			}
		})
	}
}
