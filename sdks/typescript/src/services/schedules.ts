import { Schedule, ScheduleEntry } from "../models";
import { BaseService } from "./base";

export interface CreateScheduleInput {
  camera_id: string;
  name: string;
  timezone: string;
  entries: ScheduleEntry[];
}

export interface UpdateScheduleInput {
  id: string;
  name?: string;
  timezone?: string;
  entries?: ScheduleEntry[];
}

export interface ListSchedulesInput {
  camera_id?: string;
  page_size?: number;
  cursor?: string;
}

export interface ListSchedulesResponse {
  schedules: Schedule[];
  next_cursor?: string;
}

export class ScheduleService extends BaseService {
  async create(input: CreateScheduleInput): Promise<Schedule> {
    const resp = await this.post<{ schedule: Schedule }>("/v1/schedules", input);
    return resp.schedule;
  }

  async get(id: string): Promise<Schedule> {
    const resp = await super.get<{ schedule: Schedule }>(`/v1/schedules/${id}`);
    return resp.schedule;
  }

  async update(input: UpdateScheduleInput): Promise<Schedule> {
    const updateMask: string[] = [];
    const body: Record<string, unknown> = { id: input.id };
    for (const key of ["name", "timezone", "entries"] as const) {
      if (input[key] !== undefined) {
        body[key] = input[key];
        updateMask.push(key);
      }
    }
    body.update_mask = updateMask;
    const resp = await this.patch<{ schedule: Schedule }>(`/v1/schedules/${input.id}`, body);
    return resp.schedule;
  }

  async delete(id: string): Promise<void> {
    await this.del(`/v1/schedules/${id}`);
  }

  async list(input: ListSchedulesInput = {}): Promise<ListSchedulesResponse> {
    return super.get<ListSchedulesResponse>("/v1/schedules", {
      page_size: input.page_size ?? 50,
      camera_id: input.camera_id,
      cursor: input.cursor,
    });
  }
}
