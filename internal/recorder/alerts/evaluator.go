package alerts

import (
	"context"
	"fmt"
	"log"
	"syscall"
	"time"

	db "github.com/bluenviron/mediamtx/internal/shared/legacydb"
)

// Evaluator periodically checks alert rules and creates alerts when
// thresholds are exceeded. It also sends email notifications when configured.
type Evaluator struct {
	DB             *db.DB
	RecordingsPath string
	EmailSender    *EmailSender
	Interval       time.Duration // evaluation interval (default 60s)

	ctx    context.Context
	cancel context.CancelFunc
}

// Start begins the periodic evaluation loop.
func (e *Evaluator) Start(ctx context.Context) {
	e.ctx, e.cancel = context.WithCancel(ctx)

	interval := e.Interval
	if interval == 0 {
		interval = 60 * time.Second
	}

	go func() {
		// Run an initial evaluation shortly after startup.
		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()

		select {
		case <-timer.C:
			e.evaluate()
		case <-e.ctx.Done():
			return
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				e.evaluate()
			case <-e.ctx.Done():
				return
			}
		}
	}()

	log.Printf("[NVR] [INFO] [alerts] evaluator started (interval=%s)", interval)
}

// Stop halts the evaluation loop.
func (e *Evaluator) Stop() {
	if e.cancel != nil {
		e.cancel()
	}
}

// evaluate checks all enabled rules and fires alerts as needed.
func (e *Evaluator) evaluate() {
	rules, err := e.DB.ListEnabledAlertRules()
	if err != nil {
		log.Printf("[NVR] [ERROR] [alerts] failed to list rules: %v", err)
		return
	}

	for _, rule := range rules {
		if err := e.evaluateRule(rule); err != nil {
			log.Printf("[NVR] [WARN] [alerts] rule %s (%s) evaluation failed: %v", rule.ID, rule.Name, err)
		}
	}
}

// evaluateRule checks a single rule and creates an alert if the condition is met.
func (e *Evaluator) evaluateRule(rule *db.AlertRule) error {
	// Check cooldown: skip if last alert for this rule is within cooldown period.
	latest, err := e.DB.GetLatestAlertForRule(rule.ID)
	if err != nil {
		return fmt.Errorf("check latest alert: %w", err)
	}
	if latest != nil {
		lastTime, err := time.Parse(time.RFC3339, latest.CreatedAt)
		if err == nil {
			cooldown := time.Duration(rule.CooldownMinutes) * time.Minute
			if time.Since(lastTime) < cooldown {
				return nil // still within cooldown
			}
		}
	}

	var triggered bool
	var severity, message, details string

	switch rule.RuleType {
	case "disk_usage":
		triggered, severity, message, details = e.checkDiskUsage(rule)
	case "camera_offline":
		triggered, severity, message, details = e.checkCameraOffline(rule)
	case "recording_gap":
		triggered, severity, message, details = e.checkRecordingGap(rule)
	default:
		return fmt.Errorf("unknown rule type: %s", rule.RuleType)
	}

	if !triggered {
		return nil
	}

	alert := &db.Alert{
		RuleID:   rule.ID,
		RuleType: rule.RuleType,
		Severity: severity,
		Message:  message,
		Details:  details,
	}

	if err := e.DB.CreateAlert(alert); err != nil {
		return fmt.Errorf("create alert: %w", err)
	}

	log.Printf("[NVR] [INFO] [alerts] alert fired: %s (rule=%s)", message, rule.Name)

	// Send email notification if configured.
	if rule.NotifyEmail && e.EmailSender != nil {
		go e.sendNotification(alert)
	}

	return nil
}

// checkDiskUsage evaluates disk usage against the threshold percentage.
func (e *Evaluator) checkDiskUsage(rule *db.AlertRule) (triggered bool, severity, message, details string) {
	path := e.RecordingsPath
	if path == "" {
		path = "./recordings/"
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return false, "", "", ""
	}

	totalBytes := stat.Blocks * uint64(stat.Bsize)
	freeBytes := stat.Bavail * uint64(stat.Bsize)
	if totalBytes == 0 {
		return false, "", "", ""
	}

	usedPercent := float64(totalBytes-freeBytes) / float64(totalBytes) * 100

	if usedPercent < rule.ThresholdValue {
		return false, "", "", ""
	}

	severity = "warning"
	if usedPercent > 95 {
		severity = "critical"
	}

	freeMB := float64(freeBytes) / (1024 * 1024)
	totalMB := float64(totalBytes) / (1024 * 1024)

	message = fmt.Sprintf("Disk usage at %.1f%% (threshold: %.1f%%)", usedPercent, rule.ThresholdValue)
	details = fmt.Sprintf("Path: %s\nTotal: %.0f MB\nFree: %.0f MB\nUsed: %.1f%%", path, totalMB, freeMB, usedPercent)

	return true, severity, message, details
}

// checkCameraOffline evaluates whether any cameras have been offline longer
// than the threshold (in minutes).
func (e *Evaluator) checkCameraOffline(rule *db.AlertRule) (triggered bool, severity, message, details string) {
	cameras, err := e.DB.ListCameras()
	if err != nil {
		return false, "", "", ""
	}

	thresholdMinutes := rule.ThresholdValue
	var offlineCameras []string

	for _, cam := range cameras {
		// If a specific camera is set, only check that one.
		if rule.CameraID != "" && cam.ID != rule.CameraID {
			continue
		}

		if cam.Status == "disconnected" || cam.Status == "offline" || cam.Status == "" {
			// Check how long the camera has been offline using connection events.
			events, err := e.DB.ListConnectionEvents(cam.ID, 1)
			if err != nil || len(events) == 0 {
				// No connection event history; consider it offline since last update.
				updatedAt, parseErr := time.Parse(time.RFC3339, cam.UpdatedAt)
				if parseErr != nil {
					continue
				}
				offlineDuration := time.Since(updatedAt).Minutes()
				if offlineDuration >= thresholdMinutes {
					offlineCameras = append(offlineCameras, fmt.Sprintf("%s (offline %.0fm)", cam.Name, offlineDuration))
				}
				continue
			}

			// Use the latest connection event timestamp.
			eventTime, parseErr := time.Parse(time.RFC3339, events[0].CreatedAt)
			if parseErr != nil {
				continue
			}
			offlineDuration := time.Since(eventTime).Minutes()
			if offlineDuration >= thresholdMinutes {
				offlineCameras = append(offlineCameras, fmt.Sprintf("%s (offline %.0fm)", cam.Name, offlineDuration))
			}
		}
	}

	if len(offlineCameras) == 0 {
		return false, "", "", ""
	}

	severity = "warning"
	if len(offlineCameras) > 2 {
		severity = "critical"
	}

	message = fmt.Sprintf("%d camera(s) offline for >%.0f minutes", len(offlineCameras), thresholdMinutes)
	details = fmt.Sprintf("Offline cameras:\n%s", joinLines(offlineCameras))

	return true, severity, message, details
}

// checkRecordingGap evaluates whether there are recording gaps longer than
// the threshold (in minutes) for cameras with active recording rules.
func (e *Evaluator) checkRecordingGap(rule *db.AlertRule) (triggered bool, severity, message, details string) {
	cameras, err := e.DB.ListCameras()
	if err != nil {
		return false, "", "", ""
	}

	thresholdMinutes := rule.ThresholdValue
	thresholdMs := int64(thresholdMinutes * 60 * 1000)
	var gapCameras []string

	for _, cam := range cameras {
		if rule.CameraID != "" && cam.ID != rule.CameraID {
			continue
		}

		// Check if camera has any enabled recording rules.
		rules, err := e.DB.ListRecordingRules(cam.ID)
		if err != nil || len(rules) == 0 {
			continue
		}
		hasEnabled := false
		for _, r := range rules {
			if r.Enabled {
				hasEnabled = true
				break
			}
		}
		if !hasEnabled {
			continue
		}

		// Check recording gaps exceeding threshold.
		gaps, err := e.DB.GetRecordingGaps(cam.ID, thresholdMs)
		if err != nil {
			continue
		}

		for _, gap := range gaps {
			gapMinutes := float64(gap.DurationMs) / 60000
			gapCameras = append(gapCameras, fmt.Sprintf("%s (gap: %.0fm at %s)", cam.Name, gapMinutes, gap.Start))
		}
	}

	if len(gapCameras) == 0 {
		return false, "", "", ""
	}

	severity = "warning"
	message = fmt.Sprintf("Recording gaps detected (>%.0f min threshold)", thresholdMinutes)
	details = fmt.Sprintf("Cameras with gaps:\n%s", joinLines(gapCameras))

	return true, severity, message, details
}

// sendNotification sends an email notification for an alert.
func (e *Evaluator) sendNotification(alert *db.Alert) {
	cfg, err := e.DB.GetSMTPConfig()
	if err != nil || cfg.Host == "" {
		return
	}

	// Send to the from address as the default recipient.
	to := cfg.FromAddr
	if to == "" {
		return
	}

	if err := e.EmailSender.SendAlertEmail(cfg, to, alert); err != nil {
		log.Printf("[NVR] [ERROR] [alerts] failed to send alert email: %v", err)
		_ = e.DB.UpdateAlertEmailStatus(alert.ID, false, err.Error())
		return
	}

	_ = e.DB.UpdateAlertEmailStatus(alert.ID, true, "")
	log.Printf("[NVR] [INFO] [alerts] alert email sent for alert %s", alert.ID)
}

func joinLines(items []string) string {
	result := ""
	for _, item := range items {
		result += "  - " + item + "\n"
	}
	return result
}
