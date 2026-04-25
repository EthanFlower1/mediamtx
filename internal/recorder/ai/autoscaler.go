// internal/nvr/ai/autoscaler.go
package ai

import (
	"context"
	"log"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// AutoscaleConfig holds thresholds and limits for the detection pipeline
// autoscaler. All fields have sensible defaults if left at zero value.
type AutoscaleConfig struct {
	// Enabled turns autoscaling on. When false the pipeline runs at a fixed
	// frame interval and never scales.
	Enabled bool

	// CPUHighThreshold is the percentage (0-100) of CPU usage above which the
	// autoscaler begins reducing the frame sampling rate. Default: 80.
	CPUHighThreshold float64

	// CPULowThreshold is the percentage below which the autoscaler restores
	// the sampling rate toward the base interval. Default: 50.
	CPULowThreshold float64

	// MemHighThreshold is the percentage of used memory above which the
	// autoscaler begins reducing the frame sampling rate. Default: 85.
	MemHighThreshold float64

	// MemLowThreshold is the percentage below which the autoscaler restores
	// the sampling rate toward the base interval. Default: 60.
	MemLowThreshold float64

	// BaseInterval is the default frame sampling interval when the system is
	// not under load. Default: 500ms.
	BaseInterval time.Duration

	// MaxInterval is the slowest sampling interval the autoscaler will use.
	// Default: 5s.
	MaxInterval time.Duration

	// MinInterval is a floor -- the autoscaler will never sample faster than
	// this even when restoring. Default: same as BaseInterval.
	MinInterval time.Duration

	// PollPeriod is how often the load monitor samples CPU and memory usage.
	// Default: 3s.
	PollPeriod time.Duration

	// ScaleUpFactor is the multiplier applied to the current interval when load
	// exceeds the high threshold. Default: 2.0 (double the interval).
	ScaleUpFactor float64

	// ScaleDownFactor is the divisor applied to the current interval when load
	// drops below the low threshold. Default: 2.0 (halve the interval).
	ScaleDownFactor float64
}

func (c *AutoscaleConfig) withDefaults() AutoscaleConfig {
	out := *c
	if out.CPUHighThreshold == 0 {
		out.CPUHighThreshold = 80
	}
	if out.CPULowThreshold == 0 {
		out.CPULowThreshold = 50
	}
	if out.MemHighThreshold == 0 {
		out.MemHighThreshold = 85
	}
	if out.MemLowThreshold == 0 {
		out.MemLowThreshold = 60
	}
	if out.BaseInterval == 0 {
		out.BaseInterval = 500 * time.Millisecond
	}
	if out.MaxInterval == 0 {
		out.MaxInterval = 5 * time.Second
	}
	if out.MinInterval == 0 {
		out.MinInterval = out.BaseInterval
	}
	if out.PollPeriod == 0 {
		out.PollPeriod = 3 * time.Second
	}
	if out.ScaleUpFactor == 0 {
		out.ScaleUpFactor = 2.0
	}
	if out.ScaleDownFactor == 0 {
		out.ScaleDownFactor = 2.0
	}
	return out
}

// LoadSnapshot captures a point-in-time view of system resource usage.
type LoadSnapshot struct {
	CPUPercent float64
	MemPercent float64
	Timestamp  time.Time
}

// loadReader is an interface so tests can inject fake system metrics.
type loadReader interface {
	Read() (LoadSnapshot, error)
}

// sysLoadReader reads real CPU/memory from the OS.
type sysLoadReader struct{}

func (s *sysLoadReader) Read() (LoadSnapshot, error) {
	cpuPcts, err := cpu.Percent(0, false)
	if err != nil {
		return LoadSnapshot{}, err
	}
	cpuPct := 0.0
	if len(cpuPcts) > 0 {
		cpuPct = cpuPcts[0]
	}

	vmStat, err := mem.VirtualMemory()
	if err != nil {
		return LoadSnapshot{}, err
	}
	_ = runtime.NumCPU() // keep import used

	return LoadSnapshot{
		CPUPercent: cpuPct,
		MemPercent: vmStat.UsedPercent,
		Timestamp:  time.Now(),
	}, nil
}

// Autoscaler monitors system load and dynamically adjusts the frame sampling
// interval for a detection pipeline. It exposes the current interval via
// Interval() for the frame sampler goroutine to read.
type Autoscaler struct {
	config AutoscaleConfig
	reader loadReader

	mu       sync.RWMutex
	current  time.Duration
	lastSnap LoadSnapshot

	camera string // for log messages
}

// NewAutoscaler creates an autoscaler with real system metrics.
func NewAutoscaler(camera string, cfg AutoscaleConfig) *Autoscaler {
	cfg = cfg.withDefaults()
	return &Autoscaler{
		config:  cfg,
		reader:  &sysLoadReader{},
		current: cfg.BaseInterval,
		camera:  camera,
	}
}

// newAutoscalerWithReader is used by tests to inject a fake load reader.
func newAutoscalerWithReader(camera string, cfg AutoscaleConfig, r loadReader) *Autoscaler {
	cfg = cfg.withDefaults()
	return &Autoscaler{
		config:  cfg,
		reader:  r,
		current: cfg.BaseInterval,
		camera:  camera,
	}
}

// Interval returns the current frame sampling interval. It is safe to call
// from any goroutine.
func (a *Autoscaler) Interval() time.Duration {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.current
}

// LastSnapshot returns the most recent load reading.
func (a *Autoscaler) LastSnapshot() LoadSnapshot {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.lastSnap
}

// Run polls system load on each PollPeriod tick and adjusts the interval.
// It blocks until ctx is cancelled.
func (a *Autoscaler) Run(ctx context.Context) {
	ticker := time.NewTicker(a.config.PollPeriod)
	defer ticker.Stop()

	log.Printf("[ai][%s] autoscaler started (base=%s, max=%s, cpu_high=%.0f%%, cpu_low=%.0f%%, mem_high=%.0f%%, mem_low=%.0f%%)",
		a.camera, a.config.BaseInterval, a.config.MaxInterval,
		a.config.CPUHighThreshold, a.config.CPULowThreshold,
		a.config.MemHighThreshold, a.config.MemLowThreshold)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[ai][%s] autoscaler stopped", a.camera)
			return
		case <-ticker.C:
			a.tick()
		}
	}
}

// tick reads load once and adjusts interval accordingly.
func (a *Autoscaler) tick() {
	snap, err := a.reader.Read()
	if err != nil {
		log.Printf("[ai][%s] autoscaler: load read error: %v", a.camera, err)
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	a.lastSnap = snap
	prev := a.current

	highLoad := snap.CPUPercent >= a.config.CPUHighThreshold ||
		snap.MemPercent >= a.config.MemHighThreshold
	lowLoad := snap.CPUPercent <= a.config.CPULowThreshold &&
		snap.MemPercent <= a.config.MemLowThreshold

	if highLoad {
		// Scale up interval (reduce sampling rate).
		next := time.Duration(float64(a.current) * a.config.ScaleUpFactor)
		if next > a.config.MaxInterval {
			next = a.config.MaxInterval
		}
		a.current = next
	} else if lowLoad {
		// Scale down interval (increase sampling rate).
		next := time.Duration(float64(a.current) / a.config.ScaleDownFactor)
		if next < a.config.MinInterval {
			next = a.config.MinInterval
		}
		a.current = next
	}
	// Between thresholds: keep current interval.

	if a.current != prev {
		log.Printf("[ai][%s] autoscaler: interval %s -> %s (cpu=%.1f%%, mem=%.1f%%)",
			a.camera, prev, a.current, snap.CPUPercent, snap.MemPercent)
	}
}
