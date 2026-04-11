/**
 * Error types for the KaiVue SDK.
 *
 * Error codes align with the API's error model:
 *   401/403 -> AuthenticationError
 *   404     -> NotFoundError
 *   400/422 -> ValidationError
 *   429     -> RateLimitError
 *   500+    -> ServerError
 */

export interface FieldError {
  field: string;
  message: string;
}

export class KaiVueError extends Error {
  readonly statusCode?: number;
  readonly requestId?: string;
  readonly fieldErrors: FieldError[];

  constructor(
    message: string,
    statusCode?: number,
    requestId?: string,
    fieldErrors: FieldError[] = []
  ) {
    super(message);
    this.name = "KaiVueError";
    this.statusCode = statusCode;
    this.requestId = requestId;
    this.fieldErrors = fieldErrors;
  }
}

export class AuthenticationError extends KaiVueError {
  constructor(message: string, statusCode?: number, requestId?: string) {
    super(message, statusCode, requestId);
    this.name = "AuthenticationError";
  }
}

export class NotFoundError extends KaiVueError {
  constructor(message: string, requestId?: string) {
    super(message, 404, requestId);
    this.name = "NotFoundError";
  }
}

export class ValidationError extends KaiVueError {
  constructor(
    message: string,
    fieldErrors: FieldError[] = [],
    requestId?: string
  ) {
    super(message, 422, requestId, fieldErrors);
    this.name = "ValidationError";
  }
}

export class RateLimitError extends KaiVueError {
  readonly retryAfter?: number;

  constructor(message: string, retryAfter?: number, requestId?: string) {
    super(message, 429, requestId);
    this.name = "RateLimitError";
    this.retryAfter = retryAfter;
  }
}

export class ServerError extends KaiVueError {
  constructor(message: string, statusCode: number, requestId?: string) {
    super(message, statusCode, requestId);
    this.name = "ServerError";
  }
}

/** Map an HTTP error response to the appropriate SDK error. */
export function throwForStatus(
  statusCode: number,
  body: Record<string, unknown>,
  requestId?: string
): never {
  const msg = (body.message as string) ?? (body.error as string) ?? "Unknown error";
  const fieldErrors: FieldError[] = Array.isArray(body.field_errors)
    ? body.field_errors.map((fe: Record<string, string>) => ({
        field: fe.field,
        message: fe.message,
      }))
    : [];
  const rid = requestId ?? (body.request_id as string);

  if (statusCode === 401 || statusCode === 403) {
    throw new AuthenticationError(msg, statusCode, rid);
  } else if (statusCode === 404) {
    throw new NotFoundError(msg, rid);
  } else if (statusCode === 400 || statusCode === 422) {
    throw new ValidationError(msg, fieldErrors, rid);
  } else if (statusCode === 429) {
    throw new RateLimitError(msg, undefined, rid);
  } else {
    throw new ServerError(msg, statusCode, rid);
  }
}
