import { CommentClient } from "./client.js";
import type { Comment, Thread } from "./contracts.js";

const elementName = "ms-comment-thread";

export class MSCommentThreadElement extends HTMLElement {
  static readonly observedAttributes = [
    "base-url", "space-key", "resource-type", "resource-id", "heading", "page-size",
  ];

  private readonly root: ShadowRoot;
  private readonly titleNode: HTMLHeadingElement;
  private readonly statusNode: HTMLDivElement;
  private readonly listNode: HTMLOListElement;
  private readonly moreButton: HTMLButtonElement;
  private abort?: AbortController;
  private generation = 0;
  private thread?: Thread;
  private cursor: string | null = null;
  private loading = false;
  private grant: string | null = null;

  constructor() {
    super();
    this.root = this.attachShadow({ mode: "open" });
    const style = document.createElement("style");
    style.textContent = styles;
    const section = document.createElement("section");
    section.setAttribute("part", "container");
    section.setAttribute("aria-labelledby", "heading");
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
    this.moreButton = document.createElement("button");
    this.moreButton.type = "button";
    this.moreButton.textContent = "Load more";
    this.moreButton.setAttribute("part", "load-more");
    this.moreButton.hidden = true;
    this.moreButton.addEventListener("click", () => { void this.loadMore(); });
    section.append(this.titleNode, this.statusNode, this.listNode, this.moreButton);
    this.root.append(style, section);
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
    this.abort = new AbortController();
    this.thread = undefined;
    this.cursor = null;
    this.listNode.replaceChildren();
    if (!this.spaceKey || !this.resourceType || !this.resourceID) {
      this.fail(new Error("space-key, resource-type, and resource-id are required"));
      return;
    }
    this.setLoading(true, "Loading comments…");
    try {
      const client = this.client();
      const thread = await client.ensureThread(this.spaceKey, this.resourceType, this.resourceID, this.abort.signal);
      if (generation !== this.generation) return;
      this.thread = thread;
      const page = await client.listComments(thread.id, { limit: this.pageSize(), signal: this.abort.signal });
      if (generation !== this.generation) return;
      this.cursor = page.next_cursor;
      this.renderComments(page.items, false);
      this.setLoading(false, page.items.length === 0 ? "No comments yet." : `${page.items.length} comments loaded.`);
      this.dispatchEvent(new CustomEvent("ms-comment-ready", { detail: { thread }, bubbles: true, composed: true }));
    } catch (error) {
      if (!this.abort.signal.aborted && generation === this.generation) this.fail(error);
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
      if (controller.signal.aborted || generation !== this.generation || this.thread?.id !== thread.id) return;
      this.cursor = page.next_cursor;
      this.renderComments(page.items, true);
      this.setLoading(false, `${page.items.length} more comments loaded.`);
    } catch (error) {
      if (!controller.signal.aborted && generation === this.generation) this.fail(error);
    }
  }

  private client(): CommentClient {
    return new CommentClient({ baseURL: this.baseURL, accessGrant: () => this.grant });
  }

  private renderComments(comments: Comment[], append: boolean): void {
    if (!append) this.listNode.replaceChildren();
    for (const comment of comments) {
      const item = document.createElement("li");
      item.setAttribute("part", "comment");
      item.dataset.commentId = comment.id;
      const select = document.createElement("button");
      select.type = "button";
      select.setAttribute("part", "comment-select");
      const meta = document.createElement("div");
      meta.setAttribute("part", "comment-meta");
      meta.textContent = comment.status === "deleted" ? "Deleted comment" : `Author ${comment.author_id}`;
      const body = document.createElement("p");
      body.setAttribute("part", "comment-body");
      body.textContent = comment.status === "deleted" ? "Comment deleted" : comment.body;
      select.append(meta, body);
      select.addEventListener("click", () => this.dispatchEvent(new CustomEvent("ms-comment-select", {
        detail: { comment }, bubbles: true, composed: true,
      })));
      item.append(select);
      this.listNode.append(item);
    }
    this.moreButton.hidden = this.cursor === null;
  }

  private setLoading(loading: boolean, message: string): void {
    this.loading = loading;
    this.statusNode.textContent = message;
    this.statusNode.setAttribute("aria-busy", String(loading));
    this.moreButton.disabled = loading;
    this.moreButton.hidden = loading || this.cursor === null;
  }

  private fail(error: unknown): void {
    this.loading = false;
    const normalized = error instanceof Error ? error : new Error("Unable to load comments");
    this.statusNode.textContent = normalized.message;
    this.statusNode.setAttribute("aria-busy", "false");
    this.moreButton.hidden = true;
    this.dispatchEvent(new CustomEvent("ms-comment-error", { detail: { error: normalized }, bubbles: true, composed: true }));
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

const styles = `
:host { color: var(--ms-comment-color, #18212f); display: block; font: var(--ms-comment-font, 400 1rem/1.5 system-ui, sans-serif); }
[part="container"] { background: var(--ms-comment-background, #fff); border: 1px solid var(--ms-comment-border, #d9e0e8); border-radius: var(--ms-comment-radius, .75rem); padding: var(--ms-comment-space, 1rem); }
[part="heading"] { font-size: 1.25rem; margin: 0 0 .75rem; }
[part="status"] { color: var(--ms-comment-muted, #5d6878); min-height: 1.5em; }
[part="list"] { list-style: none; margin: .75rem 0 0; padding: 0; }
[part="comment"] { border-top: 1px solid var(--ms-comment-border, #d9e0e8); padding: .75rem 0; }
[part="comment-select"] { background: none; border: 0; color: inherit; cursor: pointer; display: block; font: inherit; padding: 0; text-align: start; width: 100%; }
[part="comment-select"]:focus-visible { outline: 2px solid var(--ms-comment-action, #2457d6); outline-offset: .25rem; }
[part="comment-meta"] { color: var(--ms-comment-muted, #5d6878); font-size: .8125rem; }
[part="comment-body"] { margin: .25rem 0 0; overflow-wrap: anywhere; white-space: pre-wrap; }
[part="load-more"] { background: var(--ms-comment-action, #2457d6); border: 0; border-radius: .5rem; color: #fff; cursor: pointer; padding: .625rem 1rem; }
[part="load-more"]:disabled { cursor: wait; opacity: .6; }
`;
