import { User } from "../models";
import { BaseService } from "./base";

export interface CreateUserInput {
  username: string;
  email: string;
  password: string;
  display_name?: string;
  groups?: string[];
}

export interface UpdateUserInput {
  id: string;
  email?: string;
  display_name?: string;
  groups?: string[];
  disabled?: boolean;
}

export interface ListUsersInput {
  search?: string;
  page_size?: number;
  cursor?: string;
}

export interface ListUsersResponse {
  users: User[];
  next_cursor?: string;
  total_count?: number;
}

export class UserService extends BaseService {
  async create(input: CreateUserInput): Promise<User> {
    const resp = await this.post<{ user: User }>("/v1/users", input);
    return resp.user;
  }

  async get(id: string): Promise<User> {
    const resp = await super.get<{ user: User }>(`/v1/users/${id}`);
    return resp.user;
  }

  async update(input: UpdateUserInput): Promise<User> {
    const updateMask: string[] = [];
    const body: Record<string, unknown> = { id: input.id };
    for (const key of ["email", "display_name", "groups", "disabled"] as const) {
      if (input[key] !== undefined) {
        body[key] = input[key];
        updateMask.push(key);
      }
    }
    body.update_mask = updateMask;
    const resp = await this.patch<{ user: User }>(`/v1/users/${input.id}`, body);
    return resp.user;
  }

  async delete(id: string): Promise<void> {
    await this.del(`/v1/users/${id}`);
  }

  async list(input: ListUsersInput = {}): Promise<ListUsersResponse> {
    return super.get<ListUsersResponse>("/v1/users", {
      page_size: input.page_size ?? 50,
      search: input.search,
      cursor: input.cursor,
    });
  }
}
