import { Integration, IntegrationKind, EventKind } from "../models";
import { BaseService } from "./base";

export interface CreateIntegrationInput {
  name: string;
  kind: IntegrationKind;
  config?: Record<string, string>;
  subscribed_events?: EventKind[];
  camera_ids?: string[];
}

export interface UpdateIntegrationInput {
  id: string;
  name?: string;
  enabled?: boolean;
  config?: Record<string, string>;
  subscribed_events?: EventKind[];
  camera_ids?: string[];
}

export interface ListIntegrationsInput {
  kind_filter?: IntegrationKind;
  page_size?: number;
  cursor?: string;
}

export interface ListIntegrationsResponse {
  integrations: Integration[];
  next_cursor?: string;
}

export interface TestIntegrationResponse {
  success: boolean;
  message: string;
  latency_ms: number;
}

export class IntegrationService extends BaseService {
  async create(input: CreateIntegrationInput): Promise<Integration> {
    const resp = await this.post<{ integration: Integration }>("/v1/integrations", input);
    return resp.integration;
  }

  async get(id: string): Promise<Integration> {
    const resp = await super.get<{ integration: Integration }>(`/v1/integrations/${id}`);
    return resp.integration;
  }

  async update(input: UpdateIntegrationInput): Promise<Integration> {
    const updateMask: string[] = [];
    const body: Record<string, unknown> = { id: input.id };
    for (const key of ["name", "enabled", "config", "subscribed_events", "camera_ids"] as const) {
      if (input[key] !== undefined) {
        body[key] = input[key];
        updateMask.push(key);
      }
    }
    body.update_mask = updateMask;
    const resp = await this.patch<{ integration: Integration }>(`/v1/integrations/${input.id}`, body);
    return resp.integration;
  }

  async delete(id: string): Promise<void> {
    await this.del(`/v1/integrations/${id}`);
  }

  async list(input: ListIntegrationsInput = {}): Promise<ListIntegrationsResponse> {
    return super.get<ListIntegrationsResponse>("/v1/integrations", {
      page_size: input.page_size ?? 50,
      kind_filter: input.kind_filter,
      cursor: input.cursor,
    });
  }

  async test(id: string): Promise<TestIntegrationResponse> {
    return this.post<TestIntegrationResponse>(`/v1/integrations/${id}/test`, {});
  }
}
