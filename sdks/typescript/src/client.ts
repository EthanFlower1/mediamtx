/**
 * KaiVue SDK main client.
 *
 * Provides access to all 7 API services: cameras, users, recordings,
 * events, schedules, retention, and integrations.
 */

import { APIKeyAuth, AuthProvider } from "./auth";
import { CameraService } from "./services/cameras";
import { UserService } from "./services/users";
import { RecordingService } from "./services/recordings";
import { EventService } from "./services/events";
import { ScheduleService } from "./services/schedules";
import { RetentionService } from "./services/retention";
import { IntegrationService } from "./services/integrations";

export interface KaiVueClientOptions {
  /** API key for authentication. */
  apiKey?: string;
  /** Custom auth provider (alternative to apiKey). */
  auth?: AuthProvider;
  /** Request timeout in milliseconds. Default: 30000. */
  timeout?: number;
}

export class KaiVueClient {
  readonly cameras: CameraService;
  readonly users: UserService;
  readonly recordings: RecordingService;
  readonly events: EventService;
  readonly schedules: ScheduleService;
  readonly retention: RetentionService;
  readonly integrations: IntegrationService;

  constructor(baseUrl: string, opts: KaiVueClientOptions = {}) {
    if (opts.apiKey && opts.auth) {
      throw new Error("Provide either apiKey or auth, not both");
    }

    const auth = opts.apiKey ? new APIKeyAuth(opts.apiKey) : opts.auth;
    const httpOpts = { baseUrl, auth, timeout: opts.timeout };

    this.cameras = new CameraService(httpOpts);
    this.users = new UserService(httpOpts);
    this.recordings = new RecordingService(httpOpts);
    this.events = new EventService(httpOpts);
    this.schedules = new ScheduleService(httpOpts);
    this.retention = new RetentionService(httpOpts);
    this.integrations = new IntegrationService(httpOpts);
  }
}
