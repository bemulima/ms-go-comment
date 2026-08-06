import type { Attachment, ChangesPage, Comment, Page, RealtimeTicket, SignedURL, Thread, UUID } from "./contracts.js";
import { apiError } from "./errors.js";

export type AccessGrantProvider = string | (() => string | null | undefined);

export interface CommentClientOptions {
  baseURL?: string;
  fetch?: typeof globalThis.fetch;
  accessGrant?: AccessGrantProvider;
}

export class CommentClient {
  readonly baseURL: string;
  private readonly fetcher: typeof globalThis.fetch;
  private readonly grant?: AccessGrantProvider;

  constructor(options: CommentClientOptions = {}) {
    this.baseURL = (options.baseURL ?? "/api/comment/v1").replace(/\/$/, "");
    this.fetcher = options.fetch ?? globalThis.fetch.bind(globalThis);
    this.grant = options.accessGrant;
  }

  ensureThread(spaceKey: string, resourceType: string, resourceID: string, signal?: AbortSignal): Promise<Thread> {
    return this.json("/thread/ensure", { method: "PUT", body: { space_key: spaceKey, resource_type: resourceType, resource_id: resourceID }, signal });
  }
  getThread(threadID: UUID, signal?: AbortSignal): Promise<Thread> {
    return this.json(`/thread/get/${encodeURIComponent(threadID)}`, { signal });
  }
  listComments(threadID: UUID, options: { parentID?: UUID; limit?: number; cursor?: string; signal?: AbortSignal } = {}): Promise<Page<Comment>> {
    const query = new URLSearchParams({ thread_id: threadID });
    if (options.parentID) query.set("parent_id", options.parentID);
    if (options.limit) query.set("limit", String(options.limit));
    if (options.cursor) query.set("cursor", options.cursor);
    return this.json(`/comment/list?${query}`, { signal: options.signal });
  }
  getComment(commentID: UUID, signal?: AbortSignal): Promise<Comment> {
    return this.json(`/comment/get/${encodeURIComponent(commentID)}`, { signal });
  }
  listChanges(threadID: UUID, afterSequence = 0, limit = 100, signal?: AbortSignal): Promise<ChangesPage> {
    const query = new URLSearchParams({ thread_id: threadID, after_sequence: String(afterSequence), limit: String(limit) });
    return this.json(`/comment/changes?${query}`, { signal });
  }
  createComment(input: { threadID: UUID; parentID?: UUID | null; body: string; attachmentIDs?: UUID[]; idempotencyKey: UUID }, signal?: AbortSignal): Promise<Comment> {
    return this.json("/comment/create", { method: "POST", body: { thread_id: input.threadID, parent_id: input.parentID ?? null, body: input.body, attachment_ids: input.attachmentIDs ?? [], idempotency_key: input.idempotencyKey }, signal });
  }
  updateComment(commentID: UUID, body: string, version: number, signal?: AbortSignal): Promise<Comment> {
    return this.json(`/comment/update/${encodeURIComponent(commentID)}`, { method: "PUT", body: { body, version }, signal });
  }
  deleteComment(commentID: UUID, version: number, signal?: AbortSignal): Promise<Comment> {
    return this.json(`/comment/delete/${encodeURIComponent(commentID)}`, { method: "DELETE", body: { version }, signal });
  }
  async uploadAttachment(threadID: UUID, file: Blob, filename?: string, signal?: AbortSignal): Promise<Attachment> {
    const form = new FormData();
    form.set("thread_id", threadID);
    if (filename === undefined) form.set("file", file);
    else form.set("file", file, filename);
    return this.request("/comment-attachment/upload", { method: "POST", body: form, signal });
  }
  attachmentSignedURL(attachmentID: UUID, signal?: AbortSignal): Promise<SignedURL> {
    return this.json(`/comment-attachment/signed-url/${encodeURIComponent(attachmentID)}`, { signal });
  }
  deleteAttachment(attachmentID: UUID, signal?: AbortSignal): Promise<Attachment> {
    return this.json(`/comment-attachment/delete/${encodeURIComponent(attachmentID)}`, { method: "DELETE", signal });
  }
  mintRealtimeTicket(threadID: UUID, lastSequence?: number, signal?: AbortSignal): Promise<RealtimeTicket> {
    return this.json("/realtime/ticket", { method: "POST", body: { thread_id: threadID, ...(lastSequence === undefined ? {} : { last_sequence: lastSequence }) }, signal });
  }

  private json<T>(path: string, options: { method?: string; body?: unknown; signal?: AbortSignal }): Promise<T> {
    const headers = new Headers({ Accept: "application/json", "Content-Type": "application/json" });
    return this.request(path, { method: options.method, headers, body: options.body === undefined ? undefined : JSON.stringify(options.body), signal: options.signal });
  }

  private async request<T>(path: string, init: RequestInit): Promise<T> {
    const headers = new Headers(init.headers);
    const grant = typeof this.grant === "function" ? this.grant() : this.grant;
    if (grant) headers.set("X-Comment-Access-Grant", grant);
    const response = await this.fetcher(`${this.baseURL}${path}`, { ...init, headers, credentials: "include" });
    if (!response.ok) throw await apiError(response);
    return await response.json() as T;
  }
}
