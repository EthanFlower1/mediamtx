import { Recording } from "../models";
import { BaseService } from "./base";

export interface ListRecordingsInput {
  camera_id?: string;
  start_time?: string;
  end_time?: string;
  event_clips_only?: boolean;
  page_size?: number;
  cursor?: string;
}

export interface ListRecordingsResponse {
  recordings: Recording[];
  next_cursor?: string;
  total_count?: number;
}

export interface ExportRecordingInput {
  camera_id: string;
  start_time: string;
  end_time: string;
  format?: string;
}

export interface ExportRecordingResponse {
  download_url: string;
  expires_at?: string;
}

export class RecordingService extends BaseService {
  async get(id: string): Promise<Recording> {
    const resp = await super.get<{ recording: Recording }>(`/v1/recordings/${id}`);
    return resp.recording;
  }

  async list(input: ListRecordingsInput = {}): Promise<ListRecordingsResponse> {
    return super.get<ListRecordingsResponse>("/v1/recordings", {
      page_size: input.page_size ?? 50,
      camera_id: input.camera_id,
      start_time: input.start_time,
      end_time: input.end_time,
      event_clips_only: input.event_clips_only,
      cursor: input.cursor,
    });
  }

  async delete(id: string): Promise<void> {
    await this.del(`/v1/recordings/${id}`);
  }

  async export(input: ExportRecordingInput): Promise<ExportRecordingResponse> {
    return this.post<ExportRecordingResponse>("/v1/recordings/export", {
      ...input,
      format: input.format ?? "mp4",
    });
  }

  async *listAll(input: Omit<ListRecordingsInput, "cursor"> = {}): AsyncGenerator<Recording> {
    let cursor: string | undefined;
    while (true) {
      const resp = await this.list({ ...input, cursor });
      for (const rec of resp.recordings) yield rec;
      if (!resp.next_cursor || resp.recordings.length === 0) break;
      cursor = resp.next_cursor;
    }
  }
}
