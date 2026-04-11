import { Event, EventKind } from "../models";
import { BaseService } from "./base";

export interface ListEventsInput {
  camera_id?: string;
  kinds?: EventKind[];
  start_time?: string;
  end_time?: string;
  min_confidence?: number;
  query?: string;
  page_size?: number;
  cursor?: string;
}

export interface ListEventsResponse {
  events: Event[];
  next_cursor?: string;
  total_count?: number;
}

export class EventService extends BaseService {
  async get(id: string): Promise<Event> {
    const resp = await super.get<{ event: Event }>(`/v1/events/${id}`);
    return resp.event;
  }

  async list(input: ListEventsInput = {}): Promise<ListEventsResponse> {
    return super.get<ListEventsResponse>("/v1/events", {
      page_size: input.page_size ?? 50,
      camera_id: input.camera_id,
      kinds: input.kinds?.join(","),
      start_time: input.start_time,
      end_time: input.end_time,
      min_confidence: input.min_confidence,
      query: input.query,
      cursor: input.cursor,
    });
  }

  async acknowledge(id: string, note?: string): Promise<Event> {
    const resp = await this.post<{ event: Event }>(`/v1/events/${id}/acknowledge`, {
      id,
      note: note ?? "",
    });
    return resp.event;
  }

  async *listAll(input: Omit<ListEventsInput, "cursor"> = {}): AsyncGenerator<Event> {
    let cursor: string | undefined;
    while (true) {
      const resp = await this.list({ ...input, cursor });
      for (const evt of resp.events) yield evt;
      if (!resp.next_cursor || resp.events.length === 0) break;
      cursor = resp.next_cursor;
    }
  }
}
