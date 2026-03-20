package db

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func newTestCamera(t *testing.T, d *DB) *Camera {
	t.Helper()
	cam := &Camera{Name: "Test Camera", RTSPURL: "rtsp://192.168.1.10/stream"}
	require.NoError(t, d.CreateCamera(cam))
	return cam
}

func newTestRule(cameraID string) *RecordingRule {
	return &RecordingRule{
		CameraID:         cameraID,
		Name:             "Weekday Rule",
		Mode:             "always",
		Days:             "[1,2,3,4,5]",
		StartTime:        "08:00",
		EndTime:          "18:00",
		PostEventSeconds: 30,
		Enabled:          true,
	}
}

func TestRecordingRuleCreate(t *testing.T) {
	d := newTestDB(t)
	cam := newTestCamera(t, d)

	rule := newTestRule(cam.ID)
	err := d.CreateRecordingRule(rule)
	require.NoError(t, err)
	require.NotEmpty(t, rule.ID)
	require.NotEmpty(t, rule.CreatedAt)
	require.NotEmpty(t, rule.UpdatedAt)
}

func TestRecordingRuleGet(t *testing.T) {
	d := newTestDB(t)
	cam := newTestCamera(t, d)

	rule := newTestRule(cam.ID)
	require.NoError(t, d.CreateRecordingRule(rule))

	got, err := d.GetRecordingRule(rule.ID)
	require.NoError(t, err)
	require.Equal(t, rule.ID, got.ID)
	require.Equal(t, cam.ID, got.CameraID)
	require.Equal(t, "Weekday Rule", got.Name)
	require.Equal(t, "always", got.Mode)
	require.Equal(t, "[1,2,3,4,5]", got.Days)
	require.Equal(t, "08:00", got.StartTime)
	require.Equal(t, "18:00", got.EndTime)
	require.Equal(t, 30, got.PostEventSeconds)
	require.True(t, got.Enabled)
}

func TestRecordingRuleGetNotFound(t *testing.T) {
	d := newTestDB(t)

	_, err := d.GetRecordingRule("nonexistent-id")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestRecordingRuleList(t *testing.T) {
	d := newTestDB(t)
	cam := newTestCamera(t, d)

	rule1 := newTestRule(cam.ID)
	rule1.Name = "Rule A"
	require.NoError(t, d.CreateRecordingRule(rule1))

	rule2 := newTestRule(cam.ID)
	rule2.Name = "Rule B"
	require.NoError(t, d.CreateRecordingRule(rule2))

	rules, err := d.ListRecordingRules(cam.ID)
	require.NoError(t, err)
	require.Len(t, rules, 2)
	// Ordered by created_at.
	require.Equal(t, "Rule A", rules[0].Name)
	require.Equal(t, "Rule B", rules[1].Name)
}

func TestRecordingRuleListEmpty(t *testing.T) {
	d := newTestDB(t)
	cam := newTestCamera(t, d)

	rules, err := d.ListRecordingRules(cam.ID)
	require.NoError(t, err)
	require.Nil(t, rules)
}

func TestRecordingRuleUpdate(t *testing.T) {
	d := newTestDB(t)
	cam := newTestCamera(t, d)

	rule := newTestRule(cam.ID)
	require.NoError(t, d.CreateRecordingRule(rule))

	rule.Name = "Updated Rule"
	rule.Mode = "events"
	rule.PostEventSeconds = 60
	rule.Enabled = false
	require.NoError(t, d.UpdateRecordingRule(rule))

	got, err := d.GetRecordingRule(rule.ID)
	require.NoError(t, err)
	require.Equal(t, "Updated Rule", got.Name)
	require.Equal(t, "events", got.Mode)
	require.Equal(t, 60, got.PostEventSeconds)
	require.False(t, got.Enabled)
}

func TestRecordingRuleUpdateNotFound(t *testing.T) {
	d := newTestDB(t)

	rule := &RecordingRule{ID: "nonexistent-id", Mode: "always"}
	err := d.UpdateRecordingRule(rule)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestRecordingRuleDelete(t *testing.T) {
	d := newTestDB(t)
	cam := newTestCamera(t, d)

	rule := newTestRule(cam.ID)
	require.NoError(t, d.CreateRecordingRule(rule))

	require.NoError(t, d.DeleteRecordingRule(rule.ID))

	_, err := d.GetRecordingRule(rule.ID)
	require.ErrorIs(t, err, ErrNotFound)

	// Deleting again should return ErrNotFound.
	require.ErrorIs(t, d.DeleteRecordingRule(rule.ID), ErrNotFound)
}

func TestRecordingRuleDeleteNotFound(t *testing.T) {
	d := newTestDB(t)

	err := d.DeleteRecordingRule("nonexistent-id")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestRecordingRuleCascadeDeleteCamera(t *testing.T) {
	d := newTestDB(t)
	cam := newTestCamera(t, d)

	rule := newTestRule(cam.ID)
	require.NoError(t, d.CreateRecordingRule(rule))

	// Verify the rule exists.
	_, err := d.GetRecordingRule(rule.ID)
	require.NoError(t, err)

	// Delete the camera; the rule should be cascade-deleted.
	require.NoError(t, d.DeleteCamera(cam.ID))

	_, err = d.GetRecordingRule(rule.ID)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestRecordingRuleForeignKeyConstraint(t *testing.T) {
	d := newTestDB(t)

	// Attempt to insert a rule with a non-existent camera_id.
	rule := newTestRule("nonexistent-camera-id")
	err := d.CreateRecordingRule(rule)
	require.Error(t, err, "expected foreign key constraint violation")
}

func TestRecordingRuleListAllEnabledEmpty(t *testing.T) {
	d := newTestDB(t)

	cam := newTestCamera(t, d)

	// Create only a disabled rule.
	rule := newTestRule(cam.ID)
	rule.Enabled = false
	require.NoError(t, d.CreateRecordingRule(rule))

	rules, err := d.ListAllEnabledRecordingRules()
	require.NoError(t, err)
	require.Nil(t, rules)
}

func TestRecordingRuleListAllEnabled(t *testing.T) {
	d := newTestDB(t)

	cam1 := &Camera{Name: "Camera 1", RTSPURL: "rtsp://192.168.1.10/stream", MediaMTXPath: "cameras/cam1"}
	require.NoError(t, d.CreateCamera(cam1))

	cam2 := &Camera{Name: "Camera 2", RTSPURL: "rtsp://192.168.1.11/stream", MediaMTXPath: "cameras/cam2"}
	require.NoError(t, d.CreateCamera(cam2))

	// Enabled rule on camera 1.
	rule1 := newTestRule(cam1.ID)
	rule1.Name = "Enabled Rule 1"
	rule1.Enabled = true
	require.NoError(t, d.CreateRecordingRule(rule1))

	// Disabled rule on camera 1.
	rule2 := newTestRule(cam1.ID)
	rule2.Name = "Disabled Rule"
	rule2.Enabled = false
	require.NoError(t, d.CreateRecordingRule(rule2))

	// Enabled rule on camera 2.
	rule3 := newTestRule(cam2.ID)
	rule3.Name = "Enabled Rule 2"
	rule3.Enabled = true
	require.NoError(t, d.CreateRecordingRule(rule3))

	rules, err := d.ListAllEnabledRecordingRules()
	require.NoError(t, err)
	require.Len(t, rules, 2)
	require.Equal(t, "Enabled Rule 1", rules[0].Name)
	require.Equal(t, "Enabled Rule 2", rules[1].Name)
}
