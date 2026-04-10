/**
 * KaiVue TypeScript SDK -- quickstart example.
 *
 * Demonstrates a round-trip: create a camera, list cameras, update, then delete.
 *
 * Usage:
 *   export KAIVUE_URL=https://your-instance.kaivue.io
 *   export KAIVUE_API_KEY=your-api-key
 *   npx ts-node quickstart.ts
 */

import { KaiVueClient, RecordingMode } from "../src";

async function main() {
  const baseUrl = process.env.KAIVUE_URL ?? "https://demo.kaivue.io";
  const apiKey = process.env.KAIVUE_API_KEY ?? "demo-key";

  const client = new KaiVueClient(baseUrl, { apiKey });

  // 1. Create a camera
  console.log("Creating camera...");
  const camera = await client.cameras.create({
    name: "SDK Test Camera",
    ip_address: "192.168.1.100",
    recorder_id: "rec-01",
    recording_mode: RecordingMode.Continuous,
    labels: ["sdk-test", "entrance"],
  });
  console.log(`  Created: ${camera.id} (${camera.name})`);

  // 2. List all cameras
  console.log("\nListing cameras...");
  const { cameras } = await client.cameras.list();
  for (const cam of cameras) {
    console.log(`  ${cam.id}: ${cam.name} [${cam.state}]`);
  }

  // 3. Update the camera
  console.log(`\nUpdating camera ${camera.id}...`);
  const updated = await client.cameras.update({
    id: camera.id,
    name: "SDK Test Camera (updated)",
    motion_sensitivity: 75,
  });
  console.log(`  Updated: ${updated.name}, sensitivity=${updated.motion_sensitivity}`);

  // 4. List events
  console.log(`\nListing events for ${camera.id}...`);
  const { events } = await client.events.list({ camera_id: camera.id });
  console.log(`  Found ${events.length} events`);

  // 5. Delete the camera
  console.log(`\nDeleting camera ${camera.id}...`);
  await client.cameras.delete(camera.id);
  console.log("  Deleted.");

  console.log("\nRound-trip complete!");
}

main().catch(console.error);
