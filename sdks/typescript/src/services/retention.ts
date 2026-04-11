import { RetentionPolicy } from "../models";
import { BaseService } from "./base";

export interface CreateRetentionPolicyInput {
  name: string;
  retention_days: number;
  description?: string;
  max_bytes?: number;
  event_retention_days?: number;
}

export interface UpdateRetentionPolicyInput {
  id: string;
  name?: string;
  description?: string;
  retention_days?: number;
  max_bytes?: number;
  event_retention_days?: number;
}

export interface ListRetentionPoliciesInput {
  page_size?: number;
  cursor?: string;
}

export interface ListRetentionPoliciesResponse {
  policies: RetentionPolicy[];
  next_cursor?: string;
}

export class RetentionService extends BaseService {
  async create(input: CreateRetentionPolicyInput): Promise<RetentionPolicy> {
    const resp = await this.post<{ policy: RetentionPolicy }>("/v1/retention-policies", input);
    return resp.policy;
  }

  async get(id: string): Promise<RetentionPolicy> {
    const resp = await super.get<{ policy: RetentionPolicy }>(`/v1/retention-policies/${id}`);
    return resp.policy;
  }

  async update(input: UpdateRetentionPolicyInput): Promise<RetentionPolicy> {
    const updateMask: string[] = [];
    const body: Record<string, unknown> = { id: input.id };
    for (const key of ["name", "description", "retention_days", "max_bytes", "event_retention_days"] as const) {
      if (input[key] !== undefined) {
        body[key] = input[key];
        updateMask.push(key);
      }
    }
    body.update_mask = updateMask;
    const resp = await this.patch<{ policy: RetentionPolicy }>(`/v1/retention-policies/${input.id}`, body);
    return resp.policy;
  }

  async delete(id: string): Promise<void> {
    await this.del(`/v1/retention-policies/${id}`);
  }

  async list(input: ListRetentionPoliciesInput = {}): Promise<ListRetentionPoliciesResponse> {
    return super.get<ListRetentionPoliciesResponse>("/v1/retention-policies", {
      page_size: input.page_size ?? 50,
      cursor: input.cursor,
    });
  }

  async apply(policyId: string, cameraIds: string[]): Promise<RetentionPolicy> {
    const resp = await this.post<{ policy: RetentionPolicy }>(
      `/v1/retention-policies/${policyId}/apply`,
      { policy_id: policyId, camera_ids: cameraIds }
    );
    return resp.policy;
  }
}
