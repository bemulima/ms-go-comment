import type { APIErrorBody } from "./contracts.js";

export class CommentAPIError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
    public readonly details: Record<string, unknown> = {},
    public readonly requestID: string | null = null,
  ) {
    super(message);
    this.name = "CommentAPIError";
  }
}

export async function apiError(response: Response): Promise<CommentAPIError> {
  let body: Partial<APIErrorBody> = {};
  try { body = await response.json() as Partial<APIErrorBody>; } catch { /* stable fallback */ }
  return new CommentAPIError(
    response.status,
    body.error ?? "http_error",
    body.message ?? `Comment request failed with status ${response.status}`,
    body.details ?? {},
    response.headers.get("X-Request-ID"),
  );
}
