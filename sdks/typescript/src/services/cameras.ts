import { Camera, CameraState, RecordingMode } from "../models";
import { BaseService } from "./base";

export interface CreateCameraInput {
  name: string;
  ip_address: string;
  recorder_id: string;
  description?: string;
  recording_mode?: RecordingMode;
  labels?: string[];
  username?: string;
  password?: string;
}

export interface UpdateCameraInput {
  id: string;
  name?: string;
  description?: string;
  recording_mode?: RecordingMode;
  labels?: string[];
  audio_enabled?: boolean;
  motion_sensitivity?: number;
}

export interface ListCamerasInput {
  search?: string;
  recorder_id?: string;
  state_filter?: CameraState;
  page_size?: number;
  cursor?: string;
}

export interface ListCamerasResponse {
  cameras: Camera[];
  next_cursor?: string;
  total_count?: number;
}

export class CameraService extends BaseService {
  async create(input: CreateCameraInput): Promise<Camera> {
    const resp = await this.post<{ camera: Camera }>("/v1/cameras", input);
    return resp.camera;
  }

  async get(id: string): Promise<Camera> {
    const resp = await super.get<{ camera: Camera }>(`/v1/cameras/${id}`);
    return resp.camera;
  }

  async update(input: UpdateCameraInput): Promise<Camera> {
    const updateMask: string[] = [];
    const body: Record<string, unknown> = { id: input.id };
    for (const key of ["name", "description", "recording_mode", "labels", "audio_enabled", "motion_sensitivity"] as const) {
      if (input[key] !== undefined) {
        body[key] = input[key];
        updateMask.push(key);
      }
    }
    body.update_mask = updateMask;
    const resp = await this.patch<{ camera: Camera }>(`/v1/cameras/${input.id}`, body);
    return resp.camera;
  }

  async delete(id: string, purgeRecordings = false): Promise<void> {
    await this.del(`/v1/cameras/${id}`, { purge_recordings: purgeRecordings });
  }

  async list(input: ListCamerasInput = {}): Promise<ListCamerasResponse> {
    return this.get<ListCamerasResponse>("/v1/cameras", {
      page_size: input.page_size ?? 50,
      search: input.search,
      recorder_id: input.recorder_id,
      state_filter: input.state_filter,
      cursor: input.cursor,
    });
  }

  async *listAll(input: Omit<ListCamerasInput, "cursor"> = {}): AsyncGenerator<Camera> {
    let cursor: string | undefined;
    while (true) {
      const resp = await this.list({ ...input, cursor });
      for (const cam of resp.cameras) yield cam;
      if (!resp.next_cursor || resp.cameras.length === 0) break;
      cursor = resp.next_cursor;
    }
  }
}
