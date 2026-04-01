package modelfit

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFinalizeRuntimeProfileMarksHybridNVIDIAAsCPUFallback(t *testing.T) {
	t.Parallel()

	hardware := HardwareProfile{
		GPUs: []GPUDevice{
			{ID: "intel-0", Vendor: "intel", Name: "Intel iGPU", Integrated: true, UnifiedMemory: true},
			{ID: "nvidia-0", Vendor: "nvidia", Name: "RTX 4060", Discrete: true},
		},
	}

	profile := finalizeRuntimeProfile(RuntimeCapabilityProfile{}, hardware)
	require.False(t, profile.RuntimeAvailable)
	require.Equal(t, BackendCPU, profile.SelectedBackend)
	require.Contains(t, profile.ReasonCodes, ReasonNoRuntimeDevices)
	require.Contains(t, profile.ReasonCodes, ReasonNVIDIAPresentRuntimeCPUOnly)
	require.Contains(t, profile.ReasonCodes, ReasonIntelPresentRuntimeCPUOnly)
}

func TestFinalizeRuntimeProfileKeepsProbeAccelerator(t *testing.T) {
	t.Parallel()

	hardware := HardwareProfile{
		GPUs: []GPUDevice{
			{ID: "nvidia-0", Vendor: "nvidia", Name: "RTX 4060", Discrete: true, TotalMemoryBytes: 12 * 1024 * 1024 * 1024},
		},
	}

	profile := finalizeRuntimeProfile(RuntimeCapabilityProfile{
		SelectedBackend: BackendCUDA,
		Confidence:      DetectionConfidenceProbe,
		UsableAccelerators: []UsableAccelerator{
			{
				ID:               "nvidia-0",
				Vendor:           "nvidia",
				Name:             "RTX 4060",
				Backend:          BackendCUDA,
				Discrete:         true,
				TotalMemoryBytes: 12 * 1024 * 1024 * 1024,
			},
		},
	}, hardware)

	require.True(t, profile.RuntimeAvailable)
	require.Equal(t, BackendCUDA, profile.SelectedBackend)
	require.Equal(t, DetectionConfidenceProbe, profile.Confidence)
	require.Len(t, profile.UsableAccelerators, 1)
}

func TestParseLlamaDeviceLinesAndBackendInference(t *testing.T) {
	t.Parallel()

	lines := parseLlamaDeviceLines(`
ggml_vulkan: NVIDIA GeForce RTX 4060
ggml_cuda: NVIDIA GeForce RTX 4060
not a device line
ggml_metal: Apple M4 Pro
`)

	require.Equal(t, []string{
		"ggml_vulkan: NVIDIA GeForce RTX 4060",
		"ggml_cuda: NVIDIA GeForce RTX 4060",
		"ggml_metal: Apple M4 Pro",
	}, lines)

	cudaAccel := buildAcceleratorFromProbeLine(lines[1], 0)
	require.Equal(t, BackendCUDA, cudaAccel.Backend)
	require.Equal(t, "nvidia", cudaAccel.Vendor)

	metalAccel := buildAcceleratorFromProbeLine(lines[2], 1)
	require.Equal(t, BackendMetal, metalAccel.Backend)
	require.Equal(t, "apple", metalAccel.Vendor)
	require.True(t, metalAccel.UnifiedMemory)
}
