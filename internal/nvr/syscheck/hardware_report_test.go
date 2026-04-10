package syscheck

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClassifyTier_Enterprise(t *testing.T) {
	r := &HardwareReport{
		CPUCores: 16,
		TotalRAM: 32 * 1024 * 1024 * 1024,
		FreeDisk: 4 * 1024 * 1024 * 1024 * 1024,
	}
	require.Equal(t, TierEnterprise, ClassifyTier(r))
}

func TestClassifyTier_Mid(t *testing.T) {
	r := &HardwareReport{
		CPUCores: 4,
		TotalRAM: 8 * 1024 * 1024 * 1024,
		FreeDisk: 500 * 1024 * 1024 * 1024,
	}
	require.Equal(t, TierMid, ClassifyTier(r))
}

func TestClassifyTier_Mini(t *testing.T) {
	r := &HardwareReport{
		CPUCores: 2,
		TotalRAM: 4 * 1024 * 1024 * 1024,
		FreeDisk: 100 * 1024 * 1024 * 1024,
	}
	require.Equal(t, TierMini, ClassifyTier(r))
}

func TestClassifyTier_Insufficient(t *testing.T) {
	r := &HardwareReport{
		CPUCores: 1,
		TotalRAM: 1 * 1024 * 1024 * 1024,
		FreeDisk: 5 * 1024 * 1024 * 1024,
	}
	require.Equal(t, TierInsufficient, ClassifyTier(r))
}

func TestClassifyTier_BoundaryMini(t *testing.T) {
	// Exactly at mini thresholds
	r := &HardwareReport{
		CPUCores: MiniCPUCores,
		TotalRAM: MiniRAMBytes,
		FreeDisk: MiniDiskBytes,
	}
	require.Equal(t, TierMini, ClassifyTier(r))
}

func TestClassifyTier_BoundaryMid(t *testing.T) {
	r := &HardwareReport{
		CPUCores: MidCPUCores,
		TotalRAM: MidRAMBytes,
		FreeDisk: MidDiskBytes,
	}
	require.Equal(t, TierMid, ClassifyTier(r))
}

func TestClassifyTier_BoundaryEnterprise(t *testing.T) {
	r := &HardwareReport{
		CPUCores: EntCPUCores,
		TotalRAM: EntRAMBytes,
		FreeDisk: EntDiskBytes,
	}
	require.Equal(t, TierEnterprise, ClassifyTier(r))
}

func TestClassifyTier_HighCPULowRAM(t *testing.T) {
	// Enterprise CPU but mini RAM — should be mini
	r := &HardwareReport{
		CPUCores: 16,
		TotalRAM: 4 * 1024 * 1024 * 1024,
		FreeDisk: 100 * 1024 * 1024 * 1024,
	}
	require.Equal(t, TierMini, ClassifyTier(r))
}

func TestGenerateReport_WithMocks(t *testing.T) {
	report, err := generateReport(".", func() (uint64, error) {
		return 16 * 1024 * 1024 * 1024, nil // 16 GB
	}, func(_ string) (uint64, error) {
		return 1024 * 1024 * 1024 * 1024, nil // 1 TB
	})
	require.NoError(t, err)
	require.NotNil(t, report)
	require.True(t, report.CPUCores > 0)
	require.Equal(t, uint64(16*1024*1024*1024), report.TotalRAM)
	require.Equal(t, uint64(1024*1024*1024*1024), report.FreeDisk)
	require.NotEmpty(t, report.CPUArch)
	require.NotEmpty(t, report.GOOS)
	// Tier depends on CPU cores of the test runner, but should be at least mini
	require.NotEqual(t, TierInsufficient, report.Tier)
}

func TestGenerateReport_Real(t *testing.T) {
	report, err := GenerateReport(".")
	require.NoError(t, err)
	require.NotNil(t, report)
	require.True(t, report.CPUCores > 0)
	require.True(t, report.TotalRAM > 0)
	require.True(t, report.FreeDisk > 0)
}
