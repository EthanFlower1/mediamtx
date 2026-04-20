package metrics

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Sample holds a single point-in-time snapshot of system metrics.
type Sample struct {
	Timestamp  int64   `json:"t"`     // Unix seconds
	CPUPercent float64 `json:"cpu"`   // system CPU 0-100
	MemPercent float64 `json:"mem"`   // system RAM 0-100
	MemAllocMB float64 `json:"alloc"` // Go heap MB
	MemSysMB   float64 `json:"sys"`   // Go process MB
	Goroutines int     `json:"gr"`    // goroutine count
}

// Collector samples system metrics on a fixed interval and stores them in a
// fixed-size ring buffer.
type Collector struct {
	mu        sync.RWMutex
	samples   []Sample
	maxSize   int
	pos       int
	count     int
	interval  time.Duration
	stopCh    chan struct{}
	wg        sync.WaitGroup
	prevTotal uint64 // previous /proc/stat total jiffies
	prevIdle  uint64 // previous /proc/stat idle jiffies
}

// NewCollector creates a Collector that retains up to maxSize samples, taken
// every interval. Call Start to begin collection.
func NewCollector(maxSize int, interval time.Duration) *Collector {
	return &Collector{
		samples:  make([]Sample, maxSize),
		maxSize:  maxSize,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start launches the background goroutine. It takes an initial sample
// immediately so callers see data right away.
func (c *Collector) Start() {
	c.collect()
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				c.collect()
			case <-c.stopCh:
				return
			}
		}
	}()
}

// Stop signals the background goroutine to exit and waits for it to finish.
func (c *Collector) Stop() {
	close(c.stopCh)
	c.wg.Wait()
}

// History returns all stored samples in chronological order (oldest first).
func (c *Collector) History() []Sample {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.count == 0 {
		return []Sample{}
	}

	out := make([]Sample, c.count)
	if c.count < c.maxSize {
		// Buffer not yet full; samples live at indices [0, count).
		copy(out, c.samples[:c.count])
	} else {
		// Buffer has wrapped; oldest entry is at pos.
		n := copy(out, c.samples[c.pos:])
		copy(out[n:], c.samples[:c.pos])
	}
	return out
}

// Current returns the most recently collected sample, or a zero Sample if no
// data has been collected yet.
func (c *Collector) Current() Sample {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.count == 0 {
		return Sample{}
	}
	// pos points to the slot that will be written next; the last written slot
	// is one position behind.
	last := (c.pos - 1 + c.maxSize) % c.maxSize
	return c.samples[last]
}

// collect gathers a single metrics snapshot and appends it to the ring buffer.
func (c *Collector) collect() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	s := Sample{
		Timestamp:  time.Now().Unix(),
		CPUPercent: c.readCPUPercent(),
		MemPercent: readMemPercent(),
		MemAllocMB: float64(ms.HeapAlloc) / (1024 * 1024),
		MemSysMB:   float64(ms.Sys) / (1024 * 1024),
		Goroutines: runtime.NumGoroutine(),
	}

	c.mu.Lock()
	c.samples[c.pos] = s
	c.pos = (c.pos + 1) % c.maxSize
	if c.count < c.maxSize {
		c.count++
	}
	c.mu.Unlock()
}

// readCPUPercent reads /proc/stat and returns the CPU utilisation percentage
// since the previous call. Returns 0 on non-Linux platforms or on the first
// call (no delta available yet).
func (c *Collector) readCPUPercent() float64 {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		// fields[0] == "cpu", fields[1..] are jiffies values
		if len(fields) < 5 {
			return 0
		}
		var total uint64
		for _, f := range fields[1:] {
			v, err := strconv.ParseUint(f, 10, 64)
			if err != nil {
				return 0
			}
			total += v
		}
		idle, err := strconv.ParseUint(fields[4], 10, 64)
		if err != nil {
			return 0
		}

		prevTotal := c.prevTotal
		prevIdle := c.prevIdle
		c.prevTotal = total
		c.prevIdle = idle

		if prevTotal == 0 {
			// First read — no delta available.
			return 0
		}

		totalDelta := total - prevTotal
		idleDelta := idle - prevIdle
		if totalDelta == 0 {
			return 0
		}
		return float64(totalDelta-idleDelta) / float64(totalDelta) * 100
	}
	return 0
}

// readMemPercent reads /proc/meminfo and returns the percentage of RAM in use.
// Returns 0 on non-Linux platforms or on parse error.
func readMemPercent() float64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()

	var memTotal, memAvailable uint64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		v, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			memTotal = v
		case "MemAvailable:":
			memAvailable = v
		}
		if memTotal > 0 && memAvailable > 0 {
			break
		}
	}

	if memTotal == 0 {
		return 0
	}
	used := memTotal - memAvailable
	return float64(used) / float64(memTotal) * 100
}
