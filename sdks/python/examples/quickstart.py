#!/usr/bin/env python3
"""KaiVue Python SDK — quickstart example.

Demonstrates a round-trip: create a camera, list cameras, update it, then delete.

Usage:
    export KAIVUE_URL=https://your-instance.kaivue.io
    export KAIVUE_API_KEY=your-api-key
    python quickstart.py
"""

import os

from kaivue import KaiVueClient, RecordingMode


def main() -> None:
    base_url = os.environ.get("KAIVUE_URL", "https://demo.kaivue.io")
    api_key = os.environ.get("KAIVUE_API_KEY", "demo-key")

    with KaiVueClient(base_url, api_key=api_key) as client:
        # 1. Create a camera
        print("Creating camera...")
        camera = client.cameras.create(
            name="SDK Test Camera",
            ip_address="192.168.1.100",
            recorder_id="rec-01",
            recording_mode=RecordingMode.CONTINUOUS,
            labels=["sdk-test", "entrance"],
        )
        print(f"  Created: {camera.id} ({camera.name})")

        # 2. List all cameras
        print("\nListing cameras...")
        cameras = client.cameras.list()
        for cam in cameras:
            print(f"  {cam.id}: {cam.name} [{cam.state.value}]")

        # 3. Update the camera
        print(f"\nUpdating camera {camera.id}...")
        updated = client.cameras.update(
            camera.id,
            name="SDK Test Camera (updated)",
            motion_sensitivity=75,
        )
        print(f"  Updated: {updated.name}, sensitivity={updated.motion_sensitivity}")

        # 4. List events for the camera
        print(f"\nListing events for {camera.id}...")
        events = client.events.list(camera_id=camera.id)
        print(f"  Found {len(events)} events")

        # 5. Delete the camera
        print(f"\nDeleting camera {camera.id}...")
        client.cameras.delete(camera.id)
        print("  Deleted.")

        print("\nRound-trip complete!")


if __name__ == "__main__":
    main()
