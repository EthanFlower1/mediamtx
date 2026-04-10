// KaiVue Go SDK -- quickstart example.
//
// Demonstrates a round-trip: create a camera, list cameras, update, then delete.
//
// Usage:
//
//	export KAIVUE_URL=https://your-instance.kaivue.io
//	export KAIVUE_API_KEY=your-api-key
//	go run main.go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/kaivue/sdk-go/kaivue"
)

func main() {
	baseURL := os.Getenv("KAIVUE_URL")
	if baseURL == "" {
		baseURL = "https://demo.kaivue.io"
	}
	apiKey := os.Getenv("KAIVUE_API_KEY")
	if apiKey == "" {
		apiKey = "demo-key"
	}

	client := kaivue.NewClient(baseURL, kaivue.WithAPIKey(apiKey))
	ctx := context.Background()

	// 1. Create a camera
	fmt.Println("Creating camera...")
	cam, err := client.Cameras.Create(ctx, &kaivue.CreateCameraRequest{
		Name:          "SDK Test Camera",
		IPAddress:     "192.168.1.100",
		RecorderID:    "rec-01",
		RecordingMode: kaivue.RecordingModeContinuous,
		Labels:        []string{"sdk-test", "entrance"},
	})
	if err != nil {
		log.Fatalf("Create camera: %v", err)
	}
	fmt.Printf("  Created: %s (%s)\n", cam.ID, cam.Name)

	// 2. List all cameras
	fmt.Println("\nListing cameras...")
	listResp, err := client.Cameras.List(ctx, &kaivue.ListCamerasRequest{})
	if err != nil {
		log.Fatalf("List cameras: %v", err)
	}
	for _, c := range listResp.Cameras {
		fmt.Printf("  %s: %s [%s]\n", c.ID, c.Name, c.State)
	}

	// 3. Update the camera
	fmt.Printf("\nUpdating camera %s...\n", cam.ID)
	sensitivity := 75
	updated, err := client.Cameras.Update(ctx, &kaivue.UpdateCameraRequest{
		ID:                cam.ID,
		Name:              "SDK Test Camera (updated)",
		MotionSensitivity: &sensitivity,
		UpdateMask:        []string{"name", "motion_sensitivity"},
	})
	if err != nil {
		log.Fatalf("Update camera: %v", err)
	}
	fmt.Printf("  Updated: %s, sensitivity=%d\n", updated.Name, updated.MotionSensitivity)

	// 4. List events
	fmt.Printf("\nListing events for %s...\n", cam.ID)
	eventsResp, err := client.Events.List(ctx, &kaivue.ListEventsRequest{
		CameraID: cam.ID,
	})
	if err != nil {
		log.Fatalf("List events: %v", err)
	}
	fmt.Printf("  Found %d events\n", len(eventsResp.Events))

	// 5. Delete the camera
	fmt.Printf("\nDeleting camera %s...\n", cam.ID)
	if err := client.Cameras.Delete(ctx, cam.ID, false); err != nil {
		log.Fatalf("Delete camera: %v", err)
	}
	fmt.Println("  Deleted.")

	fmt.Println("\nRound-trip complete!")
}
