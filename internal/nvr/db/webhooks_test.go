package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebhookConfigCRUD(t *testing.T) {
	d := setupTestDB(t)

	// Insert.
	wh := &WebhookConfig{
		ID:             "wh-1",
		Name:           "Test Webhook",
		URL:            "https://example.com/hook",
		Secret:         "s3cret",
		CameraID:       "cam1",
		EventTypes:     "detection",
		ObjectClasses:  "person,car",
		MinConfidence:  0.5,
		Enabled:        true,
		MaxRetries:     3,
		TimeoutSeconds: 10,
	}
	require.NoError(t, d.InsertWebhookConfig(wh))
	assert.NotEmpty(t, wh.CreatedAt)

	// Get.
	got, err := d.GetWebhookConfig("wh-1")
	require.NoError(t, err)
	assert.Equal(t, "Test Webhook", got.Name)
	assert.Equal(t, "https://example.com/hook", got.URL)
	assert.Equal(t, "person,car", got.ObjectClasses)
	assert.Equal(t, true, got.Enabled)

	// Update.
	got.Name = "Updated Webhook"
	got.ObjectClasses = "person"
	require.NoError(t, d.UpdateWebhookConfig(got))

	got2, err := d.GetWebhookConfig("wh-1")
	require.NoError(t, err)
	assert.Equal(t, "Updated Webhook", got2.Name)
	assert.Equal(t, "person", got2.ObjectClasses)

	// List.
	configs, err := d.ListWebhookConfigs()
	require.NoError(t, err)
	assert.Len(t, configs, 1)

	// ListEnabled.
	enabled, err := d.ListEnabledWebhookConfigs()
	require.NoError(t, err)
	assert.Len(t, enabled, 1)

	// Delete.
	require.NoError(t, d.DeleteWebhookConfig("wh-1"))
	_, err = d.GetWebhookConfig("wh-1")
	assert.ErrorIs(t, err, ErrNotFound)

	// Delete missing returns ErrNotFound.
	assert.ErrorIs(t, d.DeleteWebhookConfig("wh-1"), ErrNotFound)

	// Update missing returns ErrNotFound.
	assert.ErrorIs(t, d.UpdateWebhookConfig(&WebhookConfig{ID: "missing"}), ErrNotFound)
}

func TestWebhookDeliveryCRUD(t *testing.T) {
	d := setupTestDB(t)

	// Create parent webhook.
	wh := &WebhookConfig{
		ID:   "wh-1",
		Name: "Test",
		URL:  "https://example.com",
	}
	require.NoError(t, d.InsertWebhookConfig(wh))

	// Insert delivery.
	del := &WebhookDelivery{
		WebhookID:      "wh-1",
		EventType:      "detection",
		Payload:        `{"class":"person"}`,
		ResponseStatus: 0,
		Attempt:        1,
		Status:         "pending",
	}
	require.NoError(t, d.InsertWebhookDelivery(del))
	assert.NotZero(t, del.ID)
	assert.NotEmpty(t, del.CreatedAt)

	// Update delivery.
	del.Status = "success"
	del.ResponseStatus = 200
	del.CompletedAt = "2026-01-01T00:00:00.000Z"
	require.NoError(t, d.UpdateWebhookDelivery(del))

	// List deliveries.
	deliveries, err := d.ListWebhookDeliveries("wh-1", 10)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	assert.Equal(t, "success", deliveries[0].Status)
	assert.Equal(t, 200, deliveries[0].ResponseStatus)

	// Insert a retrying delivery.
	del2 := &WebhookDelivery{
		WebhookID:   "wh-1",
		EventType:   "detection",
		Payload:     `{"class":"car"}`,
		Attempt:     1,
		Status:      "retrying",
		NextRetryAt: "2020-01-01T00:00:00.000Z", // in the past for testing
	}
	require.NoError(t, d.InsertWebhookDelivery(del2))

	// List pending (retrying with past next_retry_at).
	pending, err := d.ListPendingWebhookDeliveries()
	require.NoError(t, err)
	assert.Len(t, pending, 1)
	assert.Equal(t, del2.ID, pending[0].ID)
}

func TestCleanupOldWebhookDeliveries(t *testing.T) {
	d := setupTestDB(t)

	wh := &WebhookConfig{ID: "wh-1", Name: "Test", URL: "https://example.com"}
	require.NoError(t, d.InsertWebhookConfig(wh))

	// Insert an old successful delivery with explicit old timestamp.
	del := &WebhookDelivery{
		WebhookID: "wh-1",
		EventType: "detection",
		Payload:   `{}`,
		Attempt:   1,
		Status:    "success",
		CreatedAt: "2020-01-01T00:00:00.000Z",
	}
	require.NoError(t, d.InsertWebhookDelivery(del))

	// Cleanup deliveries older than 1 hour.
	count, err := d.CleanupOldWebhookDeliveries(1 * 60 * 60 * 1e9) // 1 hour in nanoseconds via time.Duration
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	deliveries, err := d.ListWebhookDeliveries("wh-1", 10)
	require.NoError(t, err)
	assert.Len(t, deliveries, 0)
}
