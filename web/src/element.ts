import { CommentClient } from "./client.js";
import type { Attachment, Comment, Thread, UUID } from "./contracts.js";
import { CommentAPIError } from "./errors.js";

const elementName = "ms-comment-thread";
const imageTypes = new Set(["image/jpeg", "image/png", "image/webp"]);

interface ReplyBranch {
  readonly parentID: UUID;
  readonly details: HTMLDetailsElement;
  readonly summary: HTMLElement;
  readonly list: HTMLOListElement;
  readonly moreButton: HTMLButtonElement;
  loaded: boolean;
  loading: boolean;
  cursor: string | null;
}

interface PendingCreate {
  readonly parentID: UUID | null;
  readonly body: string;
  readonly attachments: Attachment[];
  readonly idempotencyKey: UUID;
}

export class MSCommentThreadElement extends HTMLElement {
  static readonly observedAttributes = [
    "base-url", "space-key", "resource-type", "resource-id", "heading", "page-size",
  ];

  private readonly root: ShadowRoot;
  private readonly section: HTMLElement;
  private readonly titleNode: HTMLHeadingElement;
  private readonly statusNode: HTMLDivElement;
  private readonly listNode: HTMLOListElement;
  private readonly moreButton: HTMLButtonElement;
  private readonly composer: HTMLFormElement;
  private readonly replyContext: HTMLDivElement;
  private readonly textarea: HTMLTextAreaElement;
  private readonly fileInput: HTMLInputElement;
  private readonly composerHint: HTMLDivElement;
  private readonly composerStatus: HTMLDivElement;
  private readonly cancelReplyButton: HTMLButtonElement;
  private readonly submitButton: HTMLButtonElement;
  private readonly replyBranches = new Map<UUID, ReplyBranch>();
  private readonly commentItems = new Map<UUID, HTMLLIElement>();
  private readonly comments = new Map<UUID, Comment>();
  private abort?: AbortController;
  private generation = 0;
  private thread?: Thread;
  private cursor: string | null = null;
  private loading = false;
  private submitting = false;
  private pendingCreate?: PendingCreate;
  private replyToID: UUID | null = null;
  private grant: string | null = null;

  constructor() {
    super();
    this.root = this.attachShadow({ mode: "open" });
    const style = document.createElement("style");
    style.textContent = styles;
    this.section = document.createElement("section");
    this.section.setAttribute("part", "container");
    this.section.setAttribute("aria-labelledby", "heading");
    this.titleNode = document.createElement("h2");
    this.titleNode.id = "heading";
    this.titleNode.setAttribute("part", "heading");
    this.statusNode = document.createElement("div");
    this.statusNode.setAttribute("part", "status");
    this.statusNode.setAttribute("role", "status");
    this.statusNode.setAttribute("aria-live", "polite");
    this.listNode = document.createElement("ol");
    this.listNode.setAttribute("part", "list");
    this.listNode.setAttribute("aria-label", "Comments");
    this.moreButton = actionButton("Load more", "load-more");
    this.moreButton.hidden = true;
    this.moreButton.addEventListener("click", () => { void this.loadMore(); });
    this.composer = document.createElement("form");
    this.composer.setAttribute("part", "composer");
    this.composer.hidden = true;
    this.replyContext = document.createElement("div");
    this.replyContext.setAttribute("part", "reply-context");
    this.replyContext.hidden = true;
    this.cancelReplyButton = actionButton("Cancel reply", "cancel-reply");
    this.cancelReplyButton.hidden = true;
    this.cancelReplyButton.addEventListener("click", () => this.stopReply());
    const label = document.createElement("label");
    label.setAttribute("part", "composer-label");
    label.textContent = "Add a comment";
    this.textarea = document.createElement("textarea");
    this.textarea.name = "body";
    this.textarea.rows = 4;
    this.textarea.setAttribute("part", "composer-input");
    label.append(this.textarea);
    this.fileInput = document.createElement("input");
    this.fileInput.type = "file";
    this.fileInput.name = "images";
    this.fileInput.multiple = true;
    this.fileInput.accept = "image/jpeg,image/png,image/webp";
    this.fileInput.setAttribute("aria-label", "Attach images");
    this.fileInput.setAttribute("part", "composer-files");
    this.composerHint = document.createElement("div");
    this.composerHint.setAttribute("part", "composer-hint");
    this.composerStatus = document.createElement("div");
    this.composerStatus.setAttribute("part", "composer-status");
    this.composerStatus.setAttribute("role", "status");
    this.composerStatus.setAttribute("aria-live", "polite");
    this.submitButton = actionButton("Post comment", "submit");
    this.submitButton.type = "submit";
    this.composer.append(this.replyContext, this.cancelReplyButton, label, this.fileInput, this.composerHint, this.composerStatus, this.submitButton);
    this.composer.addEventListener("submit", (event) => {
      event.preventDefault();
      void this.submitComment();
    });
    this.section.append(this.titleNode, this.statusNode, this.listNode, this.moreButton, this.composer);
    this.root.append(style, this.section);
    this.updateHeading();
  }

  connectedCallback(): void { void this.refresh(); }
  disconnectedCallback(): void { this.abort?.abort(); }

  attributeChangedCallback(name: string, oldValue: string | null, newValue: string | null): void {
    if (oldValue === newValue) return;
    if (name === "heading") this.updateHeading();
    if (this.isConnected && name !== "heading") queueMicrotask(() => { void this.refresh(); });
  }

  get baseURL(): string { return this.getAttribute("base-url") ?? "/api/comment/v1"; }
  set baseURL(value: string) { this.setAttribute("base-url", value); }
  get spaceKey(): string { return this.getAttribute("space-key") ?? ""; }
  set spaceKey(value: string) { this.setAttribute("space-key", value); }
  get resourceType(): string { return this.getAttribute("resource-type") ?? ""; }
  set resourceType(value: string) { this.setAttribute("resource-type", value); }
  get resourceID(): string { return this.getAttribute("resource-id") ?? ""; }
  set resourceID(value: string) { this.setAttribute("resource-id", value); }
  get accessGrant(): string | null { return this.grant; }
  set accessGrant(value: string | null) {
    if (value === this.grant) return;
    this.grant = value;
    if (this.isConnected) queueMicrotask(() => { void this.refresh(); });
  }

  async refresh(): Promise<void> {
    const generation = ++this.generation;
    this.abort?.abort();
    const controller = new AbortController();
    this.abort = controller;
    this.thread = undefined;
    this.cursor = null;
    this.replyToID = null;
    this.pendingCreate = undefined;
    this.replyBranches.clear();
    this.commentItems.clear();
    this.comments.clear();
    this.listNode.replaceChildren();
    this.textarea.value = "";
    this.fileInput.value = "";
    this.stopReply();
    this.setComposerState(false, "");
    this.composer.hidden = true;
    if (!this.spaceKey || !this.resourceType || !this.resourceID) {
      this.fail(new Error("space-key, resource-type, and resource-id are required"), "refresh");
      return;
    }
    this.setLoading(true, "Loading comments…");
    try {
      const client = this.client();
      const thread = await client.ensureThread(this.spaceKey, this.resourceType, this.resourceID, controller.signal);
      if (!this.isCurrent(controller, generation)) return;
      this.thread = thread;
      this.configureComposer(thread);
      const page = await client.listComments(thread.id, { limit: this.pageSize(), signal: controller.signal });
      if (!this.isCurrent(controller, generation)) return;
      this.cursor = page.next_cursor;
      this.renderComments(page.items, this.listNode);
      this.setLoading(false, page.items.length === 0 ? "No comments yet." : `${page.items.length} comments loaded.`);
      this.dispatch("ms-comment-ready", { thread });
    } catch (error) {
      if (this.isCurrent(controller, generation)) this.fail(error, "refresh");
    }
  }

  async loadMore(): Promise<void> {
    const thread = this.thread;
    const cursor = this.cursor;
    const controller = this.abort;
    const generation = this.generation;
    if (!thread || !cursor || !controller || this.loading) return;
    this.setLoading(true, "Loading more comments…");
    try {
      const page = await this.client().listComments(thread.id, {
        limit: this.pageSize(), cursor, signal: controller.signal,
      });
      if (!this.isCurrent(controller, generation) || this.thread?.id !== thread.id) return;
      this.cursor = page.next_cursor;
      this.renderComments(page.items, this.listNode);
      this.setLoading(false, `${page.items.length} more comments loaded.`);
    } catch (error) {
      if (this.isCurrent(controller, generation)) this.fail(error, "root-pagination");
    }
  }

  async loadReplies(commentID: UUID): Promise<void> {
    const branch = this.replyBranches.get(commentID);
    if (!branch) return;
    await this.loadReplyPage(branch, branch.loaded);
    branch.details.open = true;
  }

  private async loadReplyPage(branch: ReplyBranch, append: boolean): Promise<void> {
    const thread = this.thread;
    const controller = this.abort;
    const generation = this.generation;
    if (!thread || !controller || branch.loading || (append && !branch.cursor)) return;
    branch.loading = true;
    branch.details.setAttribute("aria-busy", "true");
    branch.summary.textContent = "Loading replies…";
    branch.moreButton.disabled = true;
    try {
      const page = await this.client().listComments(thread.id, {
        parentID: branch.parentID,
        limit: this.pageSize(),
        cursor: append ? branch.cursor ?? undefined : undefined,
        signal: controller.signal,
      });
      if (!this.isCurrent(controller, generation) || this.thread?.id !== thread.id) return;
      branch.cursor = page.next_cursor;
      branch.loaded = true;
      this.renderComments(page.items, branch.list);
      this.updateReplyBranch(branch);
      this.dispatch("ms-comment-replies-loaded", {
        parentID: branch.parentID, count: page.items.length, hasMore: branch.cursor !== null,
      });
    } catch (error) {
      if (this.isCurrent(controller, generation)) {
        branch.summary.textContent = "Unable to load replies";
        this.emitError(error, "reply-pagination");
      }
    } finally {
      branch.loading = false;
      branch.details.setAttribute("aria-busy", "false");
      branch.moreButton.disabled = false;
    }
  }

  private renderComments(comments: Comment[], target: HTMLOListElement): void {
    for (const comment of comments) {
      if (this.commentItems.has(comment.id)) continue;
      this.comments.set(comment.id, comment);
      const item = document.createElement("li");
      item.setAttribute("part", "comment");
      item.dataset.commentId = comment.id;
      item.style.setProperty("--ms-comment-depth", String(comment.depth));
      const article = document.createElement("article");
      article.setAttribute("part", "comment-content");
      const meta = document.createElement("div");
      meta.setAttribute("part", "comment-meta");
      meta.textContent = comment.status === "deleted"
        ? "Deleted comment"
        : comment.status === "hidden" ? "Hidden comment" : `Author ${comment.author_id}`;
      const body = document.createElement("p");
      body.setAttribute("part", "comment-body");
      body.textContent = comment.status === "deleted"
        ? "Comment deleted"
        : comment.status === "hidden" ? "Comment hidden" : comment.body;
      article.append(meta, body);
      if (comment.status === "active") {
        this.renderLinks(comment, article);
        this.renderAttachments(comment, article);
      }
      const actions = document.createElement("div");
      actions.setAttribute("part", "comment-actions");
      const select = actionButton("Select", "comment-select");
      select.setAttribute("aria-label", `Select comment by ${comment.author_id}`);
      select.addEventListener("click", () => this.dispatch("ms-comment-select", { comment }));
      actions.append(select);
      if (this.canReply(comment)) {
        const reply = actionButton("Reply", "reply");
        reply.setAttribute("aria-label", `Reply to comment by ${comment.author_id}`);
        reply.addEventListener("click", () => this.startReply(comment));
        actions.append(reply);
      }
      item.append(article, actions);
      if (comment.direct_replies_count > 0 || this.canReply(comment)) item.append(this.createReplyBranch(comment));
      target.append(item);
      this.commentItems.set(comment.id, item);
    }
    this.moreButton.hidden = this.loading || this.cursor === null;
  }

  private createReplyBranch(comment: Comment): HTMLDetailsElement {
    const details = document.createElement("details");
    details.setAttribute("part", "replies");
    const summary = document.createElement("summary");
    summary.setAttribute("part", "replies-toggle");
    const list = document.createElement("ol");
    list.setAttribute("part", "list replies-list");
    list.setAttribute("aria-label", `Replies to comment ${comment.id}`);
    const moreButton = actionButton("Load more replies", "replies-load-more");
    moreButton.hidden = true;
    const branch: ReplyBranch = {
      parentID: comment.id, details, summary, list, moreButton,
      loaded: false, loading: false, cursor: null,
    };
    this.replyBranches.set(comment.id, branch);
    this.updateReplyBranch(branch);
    details.addEventListener("toggle", () => {
      if (details.open && !branch.loaded) void this.loadReplyPage(branch, false);
    });
    moreButton.addEventListener("click", () => { void this.loadReplyPage(branch, true); });
    details.append(summary, list, moreButton);
    return details;
  }

  private updateReplyBranch(branch: ReplyBranch): void {
    const count = this.comments.get(branch.parentID)?.direct_replies_count ?? branch.list.children.length;
    branch.details.hidden = count === 0;
    branch.summary.textContent = count === 1 ? "1 reply" : `${count} replies`;
    branch.moreButton.hidden = branch.cursor === null;
  }

  private renderLinks(comment: Comment, article: HTMLElement): void {
    const list = document.createElement("ul");
    list.setAttribute("part", "comment-links");
    for (const link of comment.links) {
      const url = safeHTTPURL(link.url);
      if (!url) continue;
      const item = document.createElement("li");
      const anchor = document.createElement("a");
      anchor.href = url;
      anchor.textContent = url;
      anchor.target = "_blank";
      anchor.rel = "noopener noreferrer nofollow ugc";
      item.append(anchor);
      list.append(item);
    }
    if (list.childElementCount > 0) article.append(list);
  }

  private renderAttachments(comment: Comment, article: HTMLElement): void {
    if (comment.attachments.length === 0) return;
    const list = document.createElement("ul");
    list.setAttribute("part", "comment-attachments");
    for (const attachment of comment.attachments) {
      const item = document.createElement("li");
      item.setAttribute("part", "attachment");
      item.textContent = `${attachment.original_filename} (${attachment.status})`;
      list.append(item);
      if (attachment.status === "ready") void this.loadAttachmentPreview(attachment, item);
    }
    article.append(list);
  }

  private async loadAttachmentPreview(attachment: Attachment, target: HTMLLIElement): Promise<void> {
    const controller = this.abort;
    const generation = this.generation;
    if (!controller) return;
    try {
      const signed = await this.client().attachmentSignedURL(attachment.id, controller.signal);
      const url = safeHTTPURL(signed.url);
      if (!url || !this.isCurrent(controller, generation) || !target.isConnected) return;
      const image = document.createElement("img");
      image.setAttribute("part", "attachment-image");
      image.src = url;
      image.alt = attachment.original_filename;
      image.loading = "lazy";
      image.referrerPolicy = "no-referrer";
      target.append(image);
    } catch {
      if (this.isCurrent(controller, generation)) target.dataset.preview = "unavailable";
    }
  }

  private configureComposer(thread: Thread): void {
    const writable = thread.status === "open";
    this.composer.hidden = !writable;
    this.textarea.maxLength = thread.policy.max_body_length;
    const images = thread.policy.allow_images && thread.policy.max_attachments > 0;
    this.fileInput.hidden = !images;
    this.setComposerState(false, "");
    this.composerHint.textContent = [
      `Maximum ${thread.policy.max_body_length} characters.`,
      thread.policy.allow_links ? "HTTP(S) links are allowed." : "Links are disabled.",
      images
        ? `Up to ${thread.policy.max_attachments} JPEG, PNG, or WebP images; ${formatBytes(thread.policy.max_image_bytes)} each.`
        : "Images are disabled.",
    ].join(" ");
    if (!writable) this.statusNode.textContent = `Discussion is ${thread.status}.`;
  }

  private startReply(comment: Comment): void {
    this.replyToID = comment.id;
    this.replyContext.textContent = `Replying to ${comment.author_id}`;
    this.replyContext.hidden = false;
    this.cancelReplyButton.hidden = false;
    this.textarea.focus();
    this.dispatch("ms-comment-reply-start", { comment });
  }

  private stopReply(): void {
    this.replyToID = null;
    this.replyContext.hidden = true;
    this.cancelReplyButton.hidden = true;
  }

  private async submitComment(): Promise<void> {
    const thread = this.thread;
    const controller = this.abort;
    const generation = this.generation;
    if (!thread || !controller || this.submitting || thread.status !== "open") return;
    if (this.pendingCreate) {
      await this.postPendingComment(thread, controller, generation, this.pendingCreate);
      return;
    }
    const body = this.textarea.value.trim();
    const files = Array.from(this.fileInput.files ?? []);
    const validation = this.validateDraft(thread, body, files);
    if (validation) {
      this.setComposerState(false, validation);
      return;
    }
    this.setComposerState(true, files.length > 0 ? "Uploading images…" : "Posting comment…");
    const client = this.client();
    const uploaded: Attachment[] = [];
    try {
      for (const file of files) {
        const attachment = await client.uploadAttachment(thread.id, file, file.name, controller.signal);
        uploaded.push(attachment);
        if (!this.isCurrent(controller, generation)) {
          await Promise.allSettled(uploaded.map((item) => client.deleteAttachment(item.id)));
          return;
        }
        this.dispatch("ms-comment-attachment-uploaded", { attachment });
      }
    } catch (error) {
      await Promise.allSettled(uploaded.map((attachment) => client.deleteAttachment(attachment.id)));
      if (this.isCurrent(controller, generation)) {
        const normalized = normalizeError(error);
        this.setComposerState(false, normalized.message);
        this.emitError(normalized, "upload-attachment");
      }
      return;
    }
    const pending: PendingCreate = {
      parentID: this.replyToID,
      body,
      attachments: uploaded,
      idempotencyKey: crypto.randomUUID(),
    };
    this.pendingCreate = pending;
    await this.postPendingComment(thread, controller, generation, pending);
  }

  private async postPendingComment(
    thread: Thread,
    controller: AbortController,
    generation: number,
    pending: PendingCreate,
  ): Promise<void> {
    this.setComposerState(true, "Posting comment…");
    const client = this.client();
    let comment: Comment;
    try {
      comment = await client.createComment({
        threadID: thread.id,
        parentID: pending.parentID,
        body: pending.body,
        attachmentIDs: pending.attachments.map((attachment) => attachment.id),
        idempotencyKey: pending.idempotencyKey,
      }, controller.signal);
    } catch (error) {
      if (!this.isCurrent(controller, generation)) return;
      const normalized = normalizeError(error);
      if (error instanceof CommentAPIError && error.status >= 400 && error.status < 500) {
        await Promise.allSettled(pending.attachments.map((attachment) => client.deleteAttachment(attachment.id)));
        this.pendingCreate = undefined;
        this.setComposerState(false, normalized.message);
      } else {
        this.lockComposerForRetry();
      }
      this.emitError(normalized, "create-comment");
      return;
    }
    if (!this.isCurrent(controller, generation)) return;
    this.pendingCreate = undefined;
    await this.integrateCreated(comment, pending.parentID);
    this.textarea.value = "";
    this.fileInput.value = "";
    this.stopReply();
    this.setComposerState(false, "Comment posted.");
    this.dispatch("ms-comment-created", { comment, parentID: pending.parentID });
  }

  private async integrateCreated(comment: Comment, parentID: UUID | null): Promise<void> {
    if (!parentID) {
      this.renderComments([comment], this.listNode);
      return;
    }
    const parent = this.comments.get(parentID);
    if (parent) parent.direct_replies_count += 1;
    const branch = this.replyBranches.get(parentID);
    if (!branch) return;
    if (branch.loaded) {
      this.renderComments([comment], branch.list);
      this.updateReplyBranch(branch);
      branch.details.open = true;
      return;
    }
    await this.loadReplyPage(branch, false);
    this.renderComments([comment], branch.list);
    this.updateReplyBranch(branch);
    branch.details.open = true;
  }

  private validateDraft(thread: Thread, body: string, files: File[]): string | null {
    if (!body && files.length === 0) return "Write a comment or select an image.";
    if ([...body].length > thread.policy.max_body_length) return `Comment exceeds ${thread.policy.max_body_length} characters.`;
    if (!thread.policy.allow_links && /https?:\/\//iu.test(body)) return "Links are disabled for this discussion.";
    if (files.length === 0) return null;
    if (!thread.policy.allow_images || thread.policy.max_attachments === 0) return "Images are disabled for this discussion.";
    if (files.length > thread.policy.max_attachments) return `Select at most ${thread.policy.max_attachments} images.`;
    for (const file of files) {
      if (!imageTypes.has(file.type)) return "Only JPEG, PNG, and WebP images are supported.";
      if (file.size > thread.policy.max_image_bytes) return `${file.name} exceeds the image size limit.`;
    }
    return null;
  }

  private canReply(comment: Comment): boolean {
    return comment.status === "active" && this.thread?.status === "open" && comment.depth < this.thread.policy.max_depth;
  }

  private client(): CommentClient {
    return new CommentClient({ baseURL: this.baseURL, accessGrant: this.grant ?? undefined });
  }

  private setLoading(loading: boolean, message: string): void {
    this.loading = loading;
    this.statusNode.textContent = message;
    this.section.setAttribute("aria-busy", String(loading));
    this.moreButton.disabled = loading;
    this.moreButton.hidden = loading || this.cursor === null;
  }

  private setComposerState(submitting: boolean, message: string): void {
    this.submitting = submitting;
    this.textarea.disabled = submitting;
    this.fileInput.disabled = submitting || this.fileInput.hidden;
    this.submitButton.disabled = submitting;
    this.cancelReplyButton.disabled = submitting;
    this.composer.setAttribute("aria-busy", String(submitting));
    this.composerStatus.textContent = message;
    if (!submitting) this.submitButton.textContent = "Post comment";
  }

  private lockComposerForRetry(): void {
    this.submitting = false;
    this.textarea.disabled = true;
    this.fileInput.disabled = true;
    this.cancelReplyButton.disabled = true;
    this.submitButton.disabled = false;
    this.submitButton.textContent = "Retry post";
    this.composer.setAttribute("aria-busy", "false");
    this.composerStatus.textContent = "Posting outcome is unknown. Retry sends the same idempotent request.";
  }

  private fail(error: unknown, operation: string): void {
    this.loading = false;
    const normalized = normalizeError(error);
    this.statusNode.textContent = normalized.message;
    this.section.setAttribute("aria-busy", "false");
    this.moreButton.hidden = true;
    this.emitError(normalized, operation);
  }

  private emitError(error: unknown, operation: string): void {
    this.dispatch("ms-comment-error", { error: normalizeError(error), operation });
  }

  private dispatch(name: string, detail: Record<string, unknown>): void {
    this.dispatchEvent(new CustomEvent(name, { detail, bubbles: true, composed: true }));
  }

  private isCurrent(controller: AbortController, generation: number): boolean {
    return !controller.signal.aborted && controller === this.abort && generation === this.generation;
  }

  private updateHeading(): void { this.titleNode.textContent = this.getAttribute("heading") ?? "Comments"; }
  private pageSize(): number {
    const value = Number(this.getAttribute("page-size") ?? 20);
    return Number.isInteger(value) && value >= 1 && value <= 100 ? value : 20;
  }
}

export function defineCommentThreadElement(registry: CustomElementRegistry = customElements): void {
  if (!registry.get(elementName)) registry.define(elementName, MSCommentThreadElement);
}

function actionButton(label: string, part: string): HTMLButtonElement {
  const button = document.createElement("button");
  button.type = "button";
  button.textContent = label;
  button.setAttribute("part", part);
  return button;
}

function normalizeError(error: unknown): Error {
  return error instanceof Error ? error : new Error("Unable to complete comment operation");
}

function safeHTTPURL(value: string): string | null {
  try {
    const url = new URL(value);
    return url.protocol === "http:" || url.protocol === "https:" ? url.href : null;
  } catch {
    return null;
  }
}

function formatBytes(value: number): string {
  return value >= 1024 * 1024 ? `${Math.floor(value / (1024 * 1024))} MiB` : `${Math.floor(value / 1024)} KiB`;
}

const styles = `
:host { color: var(--ms-comment-color, #18212f); display: block; font: var(--ms-comment-font, 400 1rem/1.5 system-ui, sans-serif); }
[hidden] { display: none !important; }
[part="container"] { background: var(--ms-comment-background, #fff); border: 1px solid var(--ms-comment-border, #d9e0e8); border-radius: var(--ms-comment-radius, .75rem); padding: var(--ms-comment-space, 1rem); }
[part="heading"] { font-size: 1.25rem; margin: 0 0 .75rem; }
[part="status"], [part="composer-hint"], [part="composer-status"] { color: var(--ms-comment-muted, #5d6878); min-height: 1.5em; }
[part~="list"] { list-style: none; margin: .75rem 0 0; padding: 0; }
[part="comment"] { border-top: 1px solid var(--ms-comment-border, #d9e0e8); padding: .75rem 0; }
[part="comment-meta"] { color: var(--ms-comment-muted, #5d6878); font-size: .8125rem; }
[part="comment-body"] { margin: .25rem 0 0; overflow-wrap: anywhere; white-space: pre-wrap; }
[part="comment-links"], [part="comment-attachments"] { margin: .5rem 0 0; padding-inline-start: 1.25rem; }
[part="attachment-image"] { display: block; height: auto; margin-block-start: .5rem; max-height: var(--ms-comment-image-max-height, 24rem); max-width: 100%; }
[part="comment-actions"] { display: flex; gap: .5rem; margin-block-start: .5rem; }
[part="replies"] { border-inline-start: 2px solid var(--ms-comment-border, #d9e0e8); margin: .5rem 0 0 .5rem; padding-inline-start: 1rem; }
[part="replies-toggle"] { cursor: pointer; }
[part="composer"] { border-top: 1px solid var(--ms-comment-border, #d9e0e8); display: grid; gap: .5rem; margin-block-start: 1rem; padding-block-start: 1rem; }
[part="composer-label"], [part="composer-input"] { display: block; width: 100%; }
[part="composer-input"] { box-sizing: border-box; font: inherit; resize: vertical; }
[part="reply-context"] { font-weight: 600; }
[part="load-more"], [part="replies-load-more"], [part="submit"] { background: var(--ms-comment-action, #2457d6); border: 0; border-radius: .5rem; color: #fff; cursor: pointer; padding: .625rem 1rem; }
[part="comment-select"], [part="reply"], [part="cancel-reply"] { background: none; border: 0; color: var(--ms-comment-action, #2457d6); cursor: pointer; font: inherit; padding: .25rem; }
button:focus-visible, summary:focus-visible, textarea:focus-visible, input:focus-visible { outline: 2px solid var(--ms-comment-action, #2457d6); outline-offset: .2rem; }
button:disabled { cursor: wait; opacity: .6; }
`;
