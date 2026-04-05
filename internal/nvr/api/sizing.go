package api

import (
	"math"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// sizingProfile holds typical bitrate and resolution dimensions for a given
// resolution label.
type sizingProfile struct {
	Label      string  // e.g. "1080p"
	Width      int     // pixels
	Height     int     // pixels
	BitrateMbps float64 // typical H.264 CBR bitrate in Mbps
}

// resolutionProfiles maps resolution labels to typical bitrate profiles.
// Bitrates are based on industry-standard H.264 CBR encoding at the given
// resolution and assume a single stream per camera.
var resolutionProfiles = map[string]sizingProfile{
	"360p":  {Label: "360p", Width: 640, Height: 360, BitrateMbps: 1.0},
	"480p":  {Label: "480p", Width: 854, Height: 480, BitrateMbps: 1.5},
	"720p":  {Label: "720p", Width: 1280, Height: 720, BitrateMbps: 2.5},
	"1080p": {Label: "1080p", Width: 1920, Height: 1080, BitrateMbps: 4.0},
	"2k":    {Label: "2K", Width: 2560, Height: 1440, BitrateMbps: 8.0},
	"1440p": {Label: "1440p", Width: 2560, Height: 1440, BitrateMbps: 8.0},
	"4k":    {Label: "4K", Width: 3840, Height: 2160, BitrateMbps: 16.0},
	"2160p": {Label: "2160p", Width: 3840, Height: 2160, BitrateMbps: 16.0},
}

// sizingResponse is the JSON response for the deployment sizing calculator.
type sizingResponse struct {
	Input       sizingInput       `json:"input"`
	CPU         sizingCPU         `json:"cpu"`
	RAM         sizingRAM         `json:"ram"`
	Storage     sizingStorage     `json:"storage"`
	Bandwidth   sizingBandwidth   `json:"bandwidth"`
	Tier        string            `json:"tier"`
	Notes       []string          `json:"notes"`
}

type sizingInput struct {
	Cameras       int    `json:"cameras"`
	Resolution    string `json:"resolution"`
	FPS           int    `json:"fps"`
	RetentionDays int    `json:"retention_days"`
	AI            bool   `json:"ai_enabled"`
}

type sizingCPU struct {
	Cores       int    `json:"cores"`
	Description string `json:"description"`
}

type sizingRAM struct {
	GB          int    `json:"gb"`
	Description string `json:"description"`
}

type sizingStorage struct {
	TotalTB     float64 `json:"total_tb"`
	TotalGB     float64 `json:"total_gb"`
	PerCameraGB float64 `json:"per_camera_gb"`
	Description string  `json:"description"`
}

type sizingBandwidth struct {
	IngressMbps float64 `json:"ingress_mbps"`
	Description string  `json:"description"`
}

// Sizing calculates recommended deployment resources based on the number
// of cameras, resolution, frame rate, retention period, and whether AI
// analytics are enabled. It uses typical H.264 bitrates to estimate
// storage and bandwidth, and rule-of-thumb CPU/RAM sizing.
//
//	GET /api/nvr/system/sizing?cameras=16&resolution=1080p&fps=15&retention_days=30&ai=true
func (h *SystemHandler) Sizing(c *gin.Context) {
	// Parse query parameters with sensible defaults.
	cameras, _ := strconv.Atoi(c.DefaultQuery("cameras", "1"))
	if cameras < 1 {
		cameras = 1
	}
	if cameras > 1000 {
		cameras = 1000
	}

	resolution := c.DefaultQuery("resolution", "1080p")
	profile, ok := resolutionProfiles[resolution]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":                "unsupported resolution",
			"supported_resolutions": []string{"360p", "480p", "720p", "1080p", "1440p", "2k", "4k", "2160p"},
		})
		return
	}

	fps, _ := strconv.Atoi(c.DefaultQuery("fps", "15"))
	if fps < 1 {
		fps = 1
	}
	if fps > 60 {
		fps = 60
	}

	retentionDays, _ := strconv.Atoi(c.DefaultQuery("retention_days", "30"))
	if retentionDays < 1 {
		retentionDays = 1
	}
	if retentionDays > 365 {
		retentionDays = 365
	}

	aiEnabled := c.DefaultQuery("ai", "false") == "true"

	// Scale bitrate linearly by FPS ratio relative to reference FPS of 30.
	// The profile bitrates assume 30 FPS; scale proportionally.
	referenceFPS := 30.0
	fpsScale := float64(fps) / referenceFPS
	scaledBitrateMbps := profile.BitrateMbps * fpsScale

	// ---- Bandwidth ----
	ingressMbps := scaledBitrateMbps * float64(cameras)

	// ---- Storage ----
	// bytes/sec per camera = bitrate in bits/sec / 8
	bytesPerSecPerCamera := (scaledBitrateMbps * 1_000_000) / 8
	bytesPerDayPerCamera := bytesPerSecPerCamera * 86400 // 24*60*60
	perCameraGB := (bytesPerDayPerCamera * float64(retentionDays)) / (1024 * 1024 * 1024)
	totalGB := perCameraGB * float64(cameras)
	totalTB := totalGB / 1024

	// Round to two decimal places.
	perCameraGB = math.Round(perCameraGB*100) / 100
	totalGB = math.Round(totalGB*100) / 100
	totalTB = math.Round(totalTB*100) / 100

	// ---- CPU ----
	// Rule of thumb: 0.5 core per camera for transcoding/recording,
	// plus 1 core base overhead for the NVR process.
	// AI adds 0.5 cores per camera for inference.
	cpuPerCamera := 0.5
	if aiEnabled {
		cpuPerCamera += 0.5
	}
	// Higher resolutions need more CPU.
	if profile.Width > 1920 {
		cpuPerCamera += 0.25
	}
	if profile.Width > 2560 {
		cpuPerCamera += 0.25
	}
	cpuCores := int(math.Ceil(cpuPerCamera*float64(cameras) + 1))
	// Minimum 2 cores.
	if cpuCores < 2 {
		cpuCores = 2
	}

	// ---- RAM ----
	// Base: 1 GB for NVR process + 256 MB per camera for buffers.
	// AI adds 512 MB per camera for model inference.
	ramMBPerCamera := 256.0
	if aiEnabled {
		ramMBPerCamera += 512.0
	}
	ramMB := 1024.0 + ramMBPerCamera*float64(cameras)
	ramGB := int(math.Ceil(ramMB / 1024))
	// Minimum 2 GB.
	if ramGB < 2 {
		ramGB = 2
	}

	// ---- Tier ----
	tier := "small"
	switch {
	case cameras > 64:
		tier = "enterprise"
	case cameras > 32:
		tier = "large"
	case cameras > 8:
		tier = "medium"
	}

	// ---- Notes ----
	var notes []string
	notes = append(notes, "Estimates assume H.264 CBR encoding. H.265 may reduce storage by 30-50%.")
	notes = append(notes, "Actual bitrates vary by scene complexity and camera settings.")
	if aiEnabled {
		notes = append(notes, "AI inference adds significant CPU/RAM overhead. GPU acceleration recommended for >8 cameras.")
	}
	if totalTB > 10 {
		notes = append(notes, "Consider RAID or distributed storage for reliability at this scale.")
	}
	if cameras > 32 {
		notes = append(notes, "At this camera count, consider dedicated network switches with IGMP snooping.")
	}
	notes = append(notes, "Add 20% storage overhead for filesystem and database overhead.")

	resp := sizingResponse{
		Input: sizingInput{
			Cameras:       cameras,
			Resolution:    profile.Label,
			FPS:           fps,
			RetentionDays: retentionDays,
			AI:            aiEnabled,
		},
		CPU: sizingCPU{
			Cores:       cpuCores,
			Description: cpuDescription(cpuCores),
		},
		RAM: sizingRAM{
			GB:          ramGB,
			Description: ramDescription(ramGB),
		},
		Storage: sizingStorage{
			TotalTB:     totalTB,
			TotalGB:     totalGB,
			PerCameraGB: perCameraGB,
			Description: storageDescription(totalTB),
		},
		Bandwidth: sizingBandwidth{
			IngressMbps: math.Round(ingressMbps*100) / 100,
			Description: bandwidthDescription(ingressMbps),
		},
		Tier:  tier,
		Notes: notes,
	}

	c.JSON(http.StatusOK, resp)
}

func cpuDescription(cores int) string {
	switch {
	case cores <= 4:
		return "Entry-level multi-core processor (e.g., Intel i3/i5, ARM Cortex-A76)"
	case cores <= 8:
		return "Mid-range processor (e.g., Intel i5/i7, AMD Ryzen 5/7)"
	case cores <= 16:
		return "High-performance processor (e.g., Intel i7/i9, AMD Ryzen 9)"
	default:
		return "Server-grade processor (e.g., Intel Xeon, AMD EPYC)"
	}
}

func ramDescription(gb int) string {
	switch {
	case gb <= 4:
		return "Standard desktop memory"
	case gb <= 16:
		return "Mid-range workstation memory"
	case gb <= 64:
		return "High-capacity server memory"
	default:
		return "Enterprise server memory; consider ECC RAM"
	}
}

func storageDescription(tb float64) string {
	switch {
	case tb < 1:
		return "Standard SSD or HDD"
	case tb < 10:
		return "Enterprise HDD or SSD array recommended"
	case tb < 50:
		return "RAID array or NAS recommended for reliability"
	default:
		return "Distributed storage or SAN recommended"
	}
}

func bandwidthDescription(mbps float64) string {
	switch {
	case mbps < 100:
		return "Standard 1 Gbps network sufficient"
	case mbps < 1000:
		return "1 Gbps network with dedicated VLAN recommended"
	default:
		return "10 Gbps network infrastructure recommended"
	}
}
