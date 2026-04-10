// Package installer implements the first-boot setup wizard for Kaivue
// Recording Server appliance installations. The wizard walks the operator
// through initial configuration: hardware validation, master key setup,
// admin account creation, storage path selection, network configuration,
// first camera discovery, and optional cloud pairing.
//
// State is persisted to a JSON file after each step so the wizard can
// resume from where it left off after a reboot or power loss.
package installer

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Step identifies a wizard step.
type Step string

const (
	StepHardwareCheck   Step = "hardware_check"
	StepMasterKey       Step = "master_key"
	StepAdminAccount    Step = "admin_account"
	StepStoragePath     Step = "storage_path"
	StepNetwork         Step = "network"
	StepCameraDiscovery Step = "camera_discovery"
	StepNotifications   Step = "notifications"
	StepRemoteAccess    Step = "remote_access"
	StepCloudPairing    Step = "cloud_pairing"
	StepComplete        Step = "complete"
)

// stepOrder defines the fixed order of wizard steps.
var stepOrder = []Step{
	StepHardwareCheck,
	StepMasterKey,
	StepAdminAccount,
	StepStoragePath,
	StepNetwork,
	StepCameraDiscovery,
	StepNotifications,
	StepRemoteAccess,
	StepCloudPairing,
	StepComplete,
}

// StepStatus tracks whether a step passed, was skipped, or failed.
type StepStatus string

const (
	StatusPending   StepStatus = "pending"
	StatusCompleted StepStatus = "completed"
	StatusSkipped   StepStatus = "skipped"
	StatusFailed    StepStatus = "failed"
)

// StepResult records the outcome of a single wizard step.
type StepResult struct {
	Step        Step       `json:"step"`
	Status      StepStatus `json:"status"`
	CompletedAt time.Time  `json:"completed_at,omitempty"`
	Data        any        `json:"data,omitempty"`
	Error       string     `json:"error,omitempty"`
}

// WizardState is the persisted state of a first-boot wizard run.
type WizardState struct {
	ID          string                `json:"id"`
	StartedAt   time.Time             `json:"started_at"`
	CurrentStep Step                  `json:"current_step"`
	Steps       map[Step]*StepResult  `json:"steps"`
	Config      *GeneratedConfig      `json:"config,omitempty"`
}

// GeneratedConfig holds the configuration produced by the wizard.
type GeneratedConfig struct {
	MasterKeyHex    string `json:"master_key_hex,omitempty"`
	AdminUsername   string `json:"admin_username,omitempty"`
	AdminPasswordHash string `json:"admin_password_hash,omitempty"`
	RecordingsPath  string `json:"recordings_path,omitempty"`
	ListenAddress   string `json:"listen_address,omitempty"`
	RuntimeMode     string `json:"runtime_mode,omitempty"`
	CloudEndpoint   string `json:"cloud_endpoint,omitempty"`
	PairingToken    string `json:"pairing_token,omitempty"`
}

// HardwareCheckInput is provided to the hardware check step.
type HardwareCheckInput struct {
	RecordingsPath string `json:"recordings_path"`
}

// AdminAccountInput is provided to the admin account step.
type AdminAccountInput struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
}

// StoragePathInput is provided to the storage path step.
type StoragePathInput struct {
	Path string `json:"path"`
}

// NetworkInput is provided to the network step.
type NetworkInput struct {
	ListenAddress string `json:"listen_address"`
}

// CloudPairingInput is provided to the cloud pairing step.
type CloudPairingInput struct {
	Skip          bool   `json:"skip"`
	CloudEndpoint string `json:"cloud_endpoint,omitempty"`
	PairingToken  string `json:"pairing_token,omitempty"`
}

// Wizard manages the first-boot setup flow.
type Wizard struct {
	stateDir string
	state    *WizardState
	clock    func() time.Time
	randRead func([]byte) (int, error)

	// HardwareChecker is called during the hardware check step.
	// Injected by the caller so the wizard doesn't depend on syscheck directly.
	HardwareChecker func(recordingsPath string) error
}

// NewWizard creates a wizard that persists state to stateDir.
func NewWizard(stateDir string) (*Wizard, error) {
	if stateDir == "" {
		return nil, errors.New("installer: stateDir is required")
	}
	w := &Wizard{
		stateDir: stateDir,
		clock:    time.Now,
		randRead: rand.Read,
	}
	if err := w.loadOrInit(); err != nil {
		return nil, err
	}
	return w, nil
}

// State returns the current wizard state (read-only copy).
func (w *Wizard) State() WizardState {
	return *w.state
}

// CurrentStep returns the step the wizard is currently on.
func (w *Wizard) CurrentStep() Step {
	return w.state.CurrentStep
}

// IsComplete reports whether the wizard has finished all steps.
func (w *Wizard) IsComplete() bool {
	return w.state.CurrentStep == StepComplete
}

// Steps returns the ordered list of steps.
func (w *Wizard) Steps() []Step {
	return stepOrder
}

// CompleteStep marks the current step as completed with the given data
// and advances to the next step. It persists state after each transition.
func (w *Wizard) CompleteStep(step Step, data any) error {
	if step != w.state.CurrentStep {
		return fmt.Errorf("installer: cannot complete step %q, current step is %q", step, w.state.CurrentStep)
	}
	if w.state.CurrentStep == StepComplete {
		return errors.New("installer: wizard is already complete")
	}

	result := w.state.Steps[step]
	result.Status = StatusCompleted
	result.CompletedAt = w.clock()
	result.Data = data

	// Apply step-specific config updates.
	if err := w.applyStepConfig(step, data); err != nil {
		result.Status = StatusFailed
		result.Error = err.Error()
		if saveErr := w.save(); saveErr != nil {
			return fmt.Errorf("installer: save after failure: %w (original: %v)", saveErr, err)
		}
		return err
	}

	// Advance to next step.
	w.state.CurrentStep = w.nextStep(step)
	return w.save()
}

// SkipStep marks an optional step as skipped and advances.
func (w *Wizard) SkipStep(step Step) error {
	if step != w.state.CurrentStep {
		return fmt.Errorf("installer: cannot skip step %q, current step is %q", step, w.state.CurrentStep)
	}
	if !w.isSkippable(step) {
		return fmt.Errorf("installer: step %q is required and cannot be skipped", step)
	}

	result := w.state.Steps[step]
	result.Status = StatusSkipped
	result.CompletedAt = w.clock()

	w.state.CurrentStep = w.nextStep(step)
	return w.save()
}

// FailStep records a failure on the current step without advancing.
func (w *Wizard) FailStep(step Step, err error) error {
	if step != w.state.CurrentStep {
		return fmt.Errorf("installer: cannot fail step %q, current step is %q", step, w.state.CurrentStep)
	}

	result := w.state.Steps[step]
	result.Status = StatusFailed
	result.Error = err.Error()

	return w.save()
}

// Reset clears all state and starts the wizard over.
func (w *Wizard) Reset() error {
	w.initState()
	return w.save()
}

func (w *Wizard) applyStepConfig(step Step, data any) error {
	if w.state.Config == nil {
		w.state.Config = &GeneratedConfig{}
	}

	switch step {
	case StepMasterKey:
		key := make([]byte, 32)
		if _, err := w.randRead(key); err != nil {
			return fmt.Errorf("generate master key: %w", err)
		}
		w.state.Config.MasterKeyHex = hex.EncodeToString(key)

	case StepAdminAccount:
		input, ok := data.(*AdminAccountInput)
		if !ok {
			return errors.New("admin_account step requires *AdminAccountInput")
		}
		if input.Username == "" {
			return errors.New("admin username is required")
		}
		if input.PasswordHash == "" {
			return errors.New("admin password hash is required")
		}
		w.state.Config.AdminUsername = input.Username
		w.state.Config.AdminPasswordHash = input.PasswordHash

	case StepStoragePath:
		input, ok := data.(*StoragePathInput)
		if !ok {
			return errors.New("storage_path step requires *StoragePathInput")
		}
		if input.Path == "" {
			return errors.New("storage path is required")
		}
		w.state.Config.RecordingsPath = input.Path

	case StepNetwork:
		input, ok := data.(*NetworkInput)
		if !ok {
			return errors.New("network step requires *NetworkInput")
		}
		if input.ListenAddress == "" {
			w.state.Config.ListenAddress = ":9997"
		} else {
			w.state.Config.ListenAddress = input.ListenAddress
		}

	case StepCloudPairing:
		input, ok := data.(*CloudPairingInput)
		if !ok {
			return errors.New("cloud_pairing step requires *CloudPairingInput")
		}
		if !input.Skip {
			w.state.Config.CloudEndpoint = input.CloudEndpoint
			w.state.Config.PairingToken = input.PairingToken
		}

	case StepHardwareCheck:
		if w.HardwareChecker != nil {
			input, ok := data.(*HardwareCheckInput)
			path := "."
			if ok && input.RecordingsPath != "" {
				path = input.RecordingsPath
			}
			if err := w.HardwareChecker(path); err != nil {
				return fmt.Errorf("hardware check failed: %w", err)
			}
		}
	}

	return nil
}

func (w *Wizard) isSkippable(step Step) bool {
	switch step {
	case StepCameraDiscovery, StepNotifications, StepRemoteAccess, StepCloudPairing:
		return true
	default:
		return false
	}
}

func (w *Wizard) nextStep(current Step) Step {
	for i, s := range stepOrder {
		if s == current && i+1 < len(stepOrder) {
			return stepOrder[i+1]
		}
	}
	return StepComplete
}

func (w *Wizard) statePath() string {
	return filepath.Join(w.stateDir, "wizard-state.json")
}

func (w *Wizard) loadOrInit() error {
	data, err := os.ReadFile(w.statePath())
	if errors.Is(err, os.ErrNotExist) {
		w.initState()
		return nil
	}
	if err != nil {
		return fmt.Errorf("installer: read state: %w", err)
	}

	var state WizardState
	if err := json.Unmarshal(data, &state); err != nil {
		// Corrupt state — re-initialize.
		w.initState()
		return nil
	}

	w.state = &state
	return nil
}

func (w *Wizard) initState() {
	id := make([]byte, 8)
	w.randRead(id) //nolint:errcheck // best-effort ID
	w.state = &WizardState{
		ID:          hex.EncodeToString(id),
		StartedAt:   w.clock(),
		CurrentStep: stepOrder[0],
		Steps:       make(map[Step]*StepResult, len(stepOrder)),
		Config:      &GeneratedConfig{},
	}
	for _, s := range stepOrder {
		w.state.Steps[s] = &StepResult{
			Step:   s,
			Status: StatusPending,
		}
	}
}

func (w *Wizard) save() error {
	if err := os.MkdirAll(w.stateDir, 0700); err != nil {
		return fmt.Errorf("installer: mkdir state dir: %w", err)
	}
	data, err := json.MarshalIndent(w.state, "", "  ")
	if err != nil {
		return fmt.Errorf("installer: marshal state: %w", err)
	}
	return os.WriteFile(w.statePath(), data, 0600)
}
