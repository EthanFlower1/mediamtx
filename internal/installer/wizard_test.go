package installer

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestWizard(t *testing.T) *Wizard {
	t.Helper()
	dir := t.TempDir()
	w, err := NewWizard(dir)
	if err != nil {
		t.Fatalf("NewWizard: %v", err)
	}
	w.clock = func() time.Time { return time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC) }
	return w
}

func TestNewWizard_RequiresStateDir(t *testing.T) {
	_, err := NewWizard("")
	if err == nil {
		t.Fatal("expected error for empty stateDir")
	}
}

func TestNewWizard_InitializesState(t *testing.T) {
	w := newTestWizard(t)
	if w.CurrentStep() != StepHardwareCheck {
		t.Errorf("expected first step %q, got %q", StepHardwareCheck, w.CurrentStep())
	}
	if w.IsComplete() {
		t.Error("wizard should not be complete on init")
	}
	state := w.State()
	if state.ID == "" {
		t.Error("wizard ID should not be empty")
	}
	if len(state.Steps) != len(stepOrder) {
		t.Errorf("expected %d steps, got %d", len(stepOrder), len(state.Steps))
	}
}

func TestCompleteStep_AdvancesInOrder(t *testing.T) {
	w := newTestWizard(t)

	// Complete hardware check.
	if err := w.CompleteStep(StepHardwareCheck, &HardwareCheckInput{RecordingsPath: "/tmp"}); err != nil {
		t.Fatalf("complete hardware check: %v", err)
	}
	if w.CurrentStep() != StepMasterKey {
		t.Errorf("expected %q after hardware check, got %q", StepMasterKey, w.CurrentStep())
	}

	// Complete master key.
	if err := w.CompleteStep(StepMasterKey, nil); err != nil {
		t.Fatalf("complete master key: %v", err)
	}
	if w.CurrentStep() != StepAdminAccount {
		t.Errorf("expected %q after master key, got %q", StepAdminAccount, w.CurrentStep())
	}

	// Verify master key was generated.
	cfg := w.State().Config
	if cfg.MasterKeyHex == "" {
		t.Error("master key should have been generated")
	}
	if len(cfg.MasterKeyHex) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("master key hex length: got %d, want 64", len(cfg.MasterKeyHex))
	}
}

func TestCompleteStep_WrongStep(t *testing.T) {
	w := newTestWizard(t)
	err := w.CompleteStep(StepMasterKey, nil) // skip ahead
	if err == nil {
		t.Fatal("expected error for wrong step")
	}
}

func TestCompleteStep_AlreadyComplete(t *testing.T) {
	w := newTestWizard(t)
	completeAll(t, w)

	err := w.CompleteStep(StepComplete, nil)
	if err == nil {
		t.Fatal("expected error for completing already-complete wizard")
	}
}

func TestCompleteStep_AdminAccount(t *testing.T) {
	w := newTestWizard(t)
	// Advance to admin account step.
	w.CompleteStep(StepHardwareCheck, &HardwareCheckInput{})
	w.CompleteStep(StepMasterKey, nil)

	input := &AdminAccountInput{
		Username:     "admin",
		PasswordHash: "hashed-password-here",
	}
	if err := w.CompleteStep(StepAdminAccount, input); err != nil {
		t.Fatalf("complete admin account: %v", err)
	}

	cfg := w.State().Config
	if cfg.AdminUsername != "admin" {
		t.Errorf("admin username: got %q, want %q", cfg.AdminUsername, "admin")
	}
}

func TestCompleteStep_AdminAccount_ValidationError(t *testing.T) {
	w := newTestWizard(t)
	w.CompleteStep(StepHardwareCheck, &HardwareCheckInput{})
	w.CompleteStep(StepMasterKey, nil)

	// Empty username.
	err := w.CompleteStep(StepAdminAccount, &AdminAccountInput{PasswordHash: "hash"})
	if err == nil {
		t.Fatal("expected error for empty username")
	}
	// Step should be marked as failed, not advanced.
	if w.CurrentStep() != StepAdminAccount {
		t.Errorf("step should stay at admin_account after failure, got %q", w.CurrentStep())
	}
}

func TestCompleteStep_StoragePath(t *testing.T) {
	w := newTestWizard(t)
	w.CompleteStep(StepHardwareCheck, &HardwareCheckInput{})
	w.CompleteStep(StepMasterKey, nil)
	w.CompleteStep(StepAdminAccount, &AdminAccountInput{Username: "admin", PasswordHash: "h"})

	if err := w.CompleteStep(StepStoragePath, &StoragePathInput{Path: "/data/recordings"}); err != nil {
		t.Fatalf("complete storage path: %v", err)
	}
	if w.State().Config.RecordingsPath != "/data/recordings" {
		t.Errorf("recordings path: got %q", w.State().Config.RecordingsPath)
	}
}

func TestSkipStep_Optional(t *testing.T) {
	w := newTestWizard(t)
	// Advance to camera discovery (optional).
	w.CompleteStep(StepHardwareCheck, &HardwareCheckInput{})
	w.CompleteStep(StepMasterKey, nil)
	w.CompleteStep(StepAdminAccount, &AdminAccountInput{Username: "admin", PasswordHash: "h"})
	w.CompleteStep(StepStoragePath, &StoragePathInput{Path: "/data"})
	w.CompleteStep(StepNetwork, &NetworkInput{ListenAddress: ":9997"})

	if w.CurrentStep() != StepCameraDiscovery {
		t.Fatalf("expected camera_discovery, got %q", w.CurrentStep())
	}

	if err := w.SkipStep(StepCameraDiscovery); err != nil {
		t.Fatalf("skip camera discovery: %v", err)
	}
	if w.CurrentStep() != StepNotifications {
		t.Errorf("expected notifications after skip, got %q", w.CurrentStep())
	}

	result := w.State().Steps[StepCameraDiscovery]
	if result.Status != StatusSkipped {
		t.Errorf("camera discovery status: got %q, want %q", result.Status, StatusSkipped)
	}
}

func TestSkipStep_Required(t *testing.T) {
	w := newTestWizard(t)
	err := w.SkipStep(StepHardwareCheck)
	if err == nil {
		t.Fatal("expected error for skipping required step")
	}
}

func TestFailStep(t *testing.T) {
	w := newTestWizard(t)
	testErr := errors.New("disk too small")
	if err := w.FailStep(StepHardwareCheck, testErr); err != nil {
		t.Fatalf("fail step: %v", err)
	}

	result := w.State().Steps[StepHardwareCheck]
	if result.Status != StatusFailed {
		t.Errorf("status: got %q, want %q", result.Status, StatusFailed)
	}
	if result.Error != "disk too small" {
		t.Errorf("error: got %q", result.Error)
	}
	// Should NOT advance.
	if w.CurrentStep() != StepHardwareCheck {
		t.Errorf("current step should stay at hardware_check, got %q", w.CurrentStep())
	}
}

func TestResume_AfterReboot(t *testing.T) {
	dir := t.TempDir()
	w1, _ := NewWizard(dir)
	w1.clock = func() time.Time { return time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC) }
	w1.CompleteStep(StepHardwareCheck, &HardwareCheckInput{})
	w1.CompleteStep(StepMasterKey, nil)

	// Simulate reboot: create new wizard from same dir.
	w2, err := NewWizard(dir)
	if err != nil {
		t.Fatalf("resume wizard: %v", err)
	}
	if w2.CurrentStep() != StepAdminAccount {
		t.Errorf("resumed wizard at %q, want %q", w2.CurrentStep(), StepAdminAccount)
	}
	if w2.State().Config.MasterKeyHex == "" {
		t.Error("master key should persist across resume")
	}
}

func TestReset(t *testing.T) {
	w := newTestWizard(t)
	w.CompleteStep(StepHardwareCheck, &HardwareCheckInput{})
	w.CompleteStep(StepMasterKey, nil)

	if err := w.Reset(); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if w.CurrentStep() != StepHardwareCheck {
		t.Errorf("after reset: got %q, want %q", w.CurrentStep(), StepHardwareCheck)
	}
}

func TestHardwareChecker_Integration(t *testing.T) {
	w := newTestWizard(t)
	checkerCalled := false
	w.HardwareChecker = func(path string) error {
		checkerCalled = true
		if path != "/opt/recordings" {
			t.Errorf("hardware checker path: got %q, want %q", path, "/opt/recordings")
		}
		return nil
	}

	if err := w.CompleteStep(StepHardwareCheck, &HardwareCheckInput{RecordingsPath: "/opt/recordings"}); err != nil {
		t.Fatalf("hardware check: %v", err)
	}
	if !checkerCalled {
		t.Error("hardware checker should have been called")
	}
}

func TestHardwareChecker_Failure(t *testing.T) {
	w := newTestWizard(t)
	w.HardwareChecker = func(string) error {
		return errors.New("insufficient RAM")
	}

	err := w.CompleteStep(StepHardwareCheck, &HardwareCheckInput{})
	if err == nil {
		t.Fatal("expected error from hardware checker")
	}
	if w.CurrentStep() != StepHardwareCheck {
		t.Errorf("should stay on hardware_check after failure, got %q", w.CurrentStep())
	}
}

func TestFullWizardRun(t *testing.T) {
	w := newTestWizard(t)
	completeAll(t, w)

	if !w.IsComplete() {
		t.Error("wizard should be complete")
	}

	cfg := w.State().Config
	if cfg.MasterKeyHex == "" {
		t.Error("master key missing")
	}
	if cfg.AdminUsername != "admin" {
		t.Errorf("admin username: %q", cfg.AdminUsername)
	}
	if cfg.RecordingsPath != "/data/recordings" {
		t.Errorf("recordings path: %q", cfg.RecordingsPath)
	}
}

func TestCorruptState_Reinitializes(t *testing.T) {
	dir := t.TempDir()
	// Write corrupt JSON.
	os.WriteFile(filepath.Join(dir, "wizard-state.json"), []byte("{corrupt"), 0600)

	w, err := NewWizard(dir)
	if err != nil {
		t.Fatalf("should handle corrupt state: %v", err)
	}
	if w.CurrentStep() != StepHardwareCheck {
		t.Errorf("should reinitialize to first step, got %q", w.CurrentStep())
	}
}

func TestNetworkStep_DefaultAddress(t *testing.T) {
	w := newTestWizard(t)
	w.CompleteStep(StepHardwareCheck, &HardwareCheckInput{})
	w.CompleteStep(StepMasterKey, nil)
	w.CompleteStep(StepAdminAccount, &AdminAccountInput{Username: "admin", PasswordHash: "h"})
	w.CompleteStep(StepStoragePath, &StoragePathInput{Path: "/data"})

	// Empty listen address should default.
	if err := w.CompleteStep(StepNetwork, &NetworkInput{}); err != nil {
		t.Fatalf("network step: %v", err)
	}
	if w.State().Config.ListenAddress != ":9997" {
		t.Errorf("listen address: got %q, want %q", w.State().Config.ListenAddress, ":9997")
	}
}

func TestCloudPairing_WithCredentials(t *testing.T) {
	w := newTestWizard(t)
	// Advance to cloud pairing.
	w.CompleteStep(StepHardwareCheck, &HardwareCheckInput{})
	w.CompleteStep(StepMasterKey, nil)
	w.CompleteStep(StepAdminAccount, &AdminAccountInput{Username: "admin", PasswordHash: "h"})
	w.CompleteStep(StepStoragePath, &StoragePathInput{Path: "/data"})
	w.CompleteStep(StepNetwork, &NetworkInput{})
	w.SkipStep(StepCameraDiscovery)
	w.SkipStep(StepNotifications)
	w.SkipStep(StepRemoteAccess)

	input := &CloudPairingInput{
		CloudEndpoint: "https://api.kaivue.io",
		PairingToken:  "tok-abc123",
	}
	if err := w.CompleteStep(StepCloudPairing, input); err != nil {
		t.Fatalf("cloud pairing: %v", err)
	}
	cfg := w.State().Config
	if cfg.CloudEndpoint != "https://api.kaivue.io" {
		t.Errorf("cloud endpoint: %q", cfg.CloudEndpoint)
	}
	if cfg.PairingToken != "tok-abc123" {
		t.Errorf("pairing token: %q", cfg.PairingToken)
	}
}

// completeAll runs through every step of the wizard.
func completeAll(t *testing.T, w *Wizard) {
	t.Helper()
	if err := w.CompleteStep(StepHardwareCheck, &HardwareCheckInput{}); err != nil {
		t.Fatalf("hardware check: %v", err)
	}
	if err := w.CompleteStep(StepMasterKey, nil); err != nil {
		t.Fatalf("master key: %v", err)
	}
	if err := w.CompleteStep(StepAdminAccount, &AdminAccountInput{
		Username:     "admin",
		PasswordHash: "argon2id$...",
	}); err != nil {
		t.Fatalf("admin account: %v", err)
	}
	if err := w.CompleteStep(StepStoragePath, &StoragePathInput{Path: "/data/recordings"}); err != nil {
		t.Fatalf("storage path: %v", err)
	}
	if err := w.CompleteStep(StepNetwork, &NetworkInput{ListenAddress: ":9997"}); err != nil {
		t.Fatalf("network: %v", err)
	}
	if err := w.SkipStep(StepCameraDiscovery); err != nil {
		t.Fatalf("skip camera discovery: %v", err)
	}
	if err := w.SkipStep(StepNotifications); err != nil {
		t.Fatalf("skip notifications: %v", err)
	}
	if err := w.SkipStep(StepRemoteAccess); err != nil {
		t.Fatalf("skip remote access: %v", err)
	}
	if err := w.CompleteStep(StepCloudPairing, &CloudPairingInput{Skip: true}); err != nil {
		t.Fatalf("cloud pairing: %v", err)
	}
}
