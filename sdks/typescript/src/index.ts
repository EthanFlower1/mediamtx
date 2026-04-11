/**
 * KaiVue VMS TypeScript SDK.
 *
 * Manage cameras, users, recordings, events, schedules, retention policies,
 * and integrations through the KaiVue public API.
 *
 * @example
 * ```typescript
 * import { KaiVueClient } from "@kaivue/sdk";
 *
 * const client = new KaiVueClient("https://your-instance.kaivue.io", {
 *   apiKey: "your-key",
 * });
 * const cameras = await client.cameras.list();
 * ```
 *
 * @packageDocumentation
 */

export { KaiVueClient, KaiVueClientOptions } from "./client";
export { APIKeyAuth, OAuthAuth, AuthProvider } from "./auth";
export {
  KaiVueError,
  AuthenticationError,
  NotFoundError,
  ValidationError,
  RateLimitError,
  ServerError,
  FieldError,
} from "./errors";
export {
  Camera,
  CameraState,
  RecordingMode,
  StreamProfile,
  User,
  Recording,
  Event,
  EventKind,
  BoundingBox,
  Schedule,
  ScheduleEntry,
  RetentionPolicy,
  Integration,
  IntegrationKind,
  PaginatedResponse,
} from "./models";
