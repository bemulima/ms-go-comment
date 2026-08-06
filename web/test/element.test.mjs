import test from "node:test";
import assert from "node:assert/strict";
import { Window } from "happy-dom";

const window = new Window({ url: "https://app.example/course" });
globalThis.window = window;
globalThis.document = window.document;
globalThis.HTMLElement = window.HTMLElement;
globalThis.CustomEvent = window.CustomEvent;
globalThis.customElements = window.customElements;
globalThis.location = window.location;
globalThis.File = window.File;
globalThis.Blob = window.Blob;
globalThis.FormData = window.FormData;

const { defineCommentThreadElement } = await import("../dist/element.js");
defineCommentThreadElement();

function response(body, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

function comment(id, body, overrides = {}) {
  return {
    id, thread_id: "thread-1", author_id: "user-1", root_id: id, path: [id], depth: 0,
    body, links: [], attachments: [], status: "active", version: 1, sequence: 1,
    direct_replies_count: 0, created_at: new Date().toISOString(), updated_at: new Date().toISOString(),
    ...overrides,
  };
}

function thread(id = "thread-1", overrides = {}) {
  const now = new Date().toISOString();
  return {
    id, space_id: "space-1", resource_type: "lesson", resource_id: "lesson-1", status: "open",
    policy: {
      allow_images: true, allow_links: true, max_depth: 5, max_body_length: 2000,
      max_attachments: 3, max_image_bytes: 5 * 1024 * 1024, edit_window_seconds: 900,
    },
    comment_count: 0, root_comment_count: 0, last_sequence: 2,
    created_at: now, updated_at: now, ...overrides,
  };
}

test("element loads a thread and safely renders root comments", async () => {
  const calls = [];
  globalThis.fetch = async (url, init) => {
    calls.push({ url, init });
    if (url.endsWith("/thread/ensure")) return response(thread());
    return response({ items: [comment("comment-1", '<img src=x onerror="boom">')], next_cursor: null });
  };
  const element = document.createElement("ms-comment-thread");
  element.setAttribute("space-key", "course");
  element.setAttribute("resource-type", "lesson");
  element.setAttribute("resource-id", "lesson-1");
  element.accessGrant = "private-grant";
  const ready = new Promise((resolve) => element.addEventListener("ms-comment-ready", resolve, { once: true }));
  document.body.append(element);
  await ready;

  const body = element.shadowRoot.querySelector('[part="comment-body"]');
  assert.equal(body.textContent, '<img src=x onerror="boom">');
  assert.equal(element.shadowRoot.querySelector("img"), null);
  assert.equal(calls[0].init.headers.get("X-Comment-Access-Grant"), "private-grant");
  assert.match(element.shadowRoot.querySelector('[part="status"]').textContent, /1 comments loaded/);
  element.remove();
});

test("element exposes pagination and selection events", async () => {
  let listCall = 0;
  globalThis.fetch = async (url) => {
    if (url.endsWith("/thread/ensure")) return response(thread());
    listCall += 1;
    return listCall === 1
      ? response({ items: [comment("comment-1", "first")], next_cursor: "next" })
      : response({ items: [comment("comment-2", "second")], next_cursor: null });
  };
  const element = document.createElement("ms-comment-thread");
  element.spaceKey = "course";
  element.resourceType = "lesson";
  element.resourceID = "lesson-1";
  const ready = new Promise((resolve) => element.addEventListener("ms-comment-ready", resolve, { once: true }));
  document.body.append(element);
  await ready;
  await element.loadMore();
  assert.equal(element.shadowRoot.querySelectorAll('[part="comment"]').length, 2);
  const selected = new Promise((resolve) => element.addEventListener("ms-comment-select", (event) => resolve(event.detail.comment.id), { once: true }));
  element.shadowRoot.querySelector('[part="comment-select"]').click();
  assert.equal(await selected, "comment-1");
  element.remove();
});

test("element ignores stale pagination after a resource refresh", async () => {
  let resolveStalePage;
  globalThis.fetch = async (url, init) => {
    if (url.endsWith("/thread/ensure")) {
      const resourceID = JSON.parse(init.body).resource_id;
      return response(thread(resourceID === "lesson-1" ? "thread-1" : "thread-2"));
    }
    const requestURL = new URL(url, window.location.href);
    if (requestURL.searchParams.get("cursor") === "next") {
      return await new Promise((resolve) => { resolveStalePage = resolve; });
    }
    return requestURL.searchParams.get("thread_id") === "thread-1"
      ? response({ items: [comment("comment-1", "old root")], next_cursor: "next" })
      : response({ items: [comment("comment-2", "fresh root")], next_cursor: null });
  };
  const element = document.createElement("ms-comment-thread");
  element.spaceKey = "course";
  element.resourceType = "lesson";
  element.resourceID = "lesson-1";
  let ready = new Promise((resolve) => element.addEventListener("ms-comment-ready", resolve, { once: true }));
  document.body.append(element);
  await ready;

  const staleLoad = element.loadMore();
  ready = new Promise((resolve) => element.addEventListener("ms-comment-ready", resolve, { once: true }));
  element.resourceID = "lesson-2";
  await ready;
  resolveStalePage(response({ items: [comment("comment-stale", "stale child")], next_cursor: null }));
  await staleLoad;

  const bodies = [...element.shadowRoot.querySelectorAll('[part="comment-body"]')].map((node) => node.textContent);
  assert.deepEqual(bodies, ["fresh root"]);
  element.remove();
});

test("element lazily renders recursive replies with per-parent pagination", async () => {
  const root = comment("root-1", "root", { direct_replies_count: 2 });
  let replyCalls = 0;
  globalThis.fetch = async (url) => {
    if (url.endsWith("/thread/ensure")) return response(thread());
    const requestURL = new URL(url, window.location.href);
    if (!requestURL.searchParams.has("parent_id")) return response({ items: [root], next_cursor: null });
    replyCalls += 1;
    return requestURL.searchParams.has("cursor")
      ? response({ items: [comment("reply-2", "second reply", { parent_id: "root-1", root_id: "root-1", path: ["root-1", "reply-2"], depth: 1 })], next_cursor: null })
      : response({ items: [comment("reply-1", "first reply", { parent_id: "root-1", root_id: "root-1", path: ["root-1", "reply-1"], depth: 1 })], next_cursor: "reply-next" });
  };
  const element = document.createElement("ms-comment-thread");
  element.spaceKey = "course";
  element.resourceType = "lesson";
  element.resourceID = "lesson-1";
  const ready = new Promise((resolve) => element.addEventListener("ms-comment-ready", resolve, { once: true }));
  document.body.append(element);
  await ready;

  const loaded = new Promise((resolve) => element.addEventListener("ms-comment-replies-loaded", resolve, { once: true }));
  await element.loadReplies("root-1");
  await loaded;
  await element.loadReplies("root-1");

  assert.equal(replyCalls, 2);
  assert.equal(element.shadowRoot.querySelectorAll('[part="comment"]').length, 3);
  assert.deepEqual(
    [...element.shadowRoot.querySelectorAll('[part="comment-body"]')].map((node) => node.textContent),
    ["root", "first reply", "second reply"],
  );
  element.remove();
});

test("composer creates a root comment with a client idempotency key", async () => {
  let createBody;
  globalThis.fetch = async (url, init) => {
    if (url.endsWith("/thread/ensure")) return response(thread());
    if (url.includes("/comment/list")) return response({ items: [], next_cursor: null });
    if (url.endsWith("/comment/create")) {
      createBody = JSON.parse(init.body);
      return response(comment("created-1", createBody.body), 201);
    }
    throw new Error(`unexpected URL ${url}`);
  };
  const element = document.createElement("ms-comment-thread");
  element.spaceKey = "course";
  element.resourceType = "lesson";
  element.resourceID = "lesson-1";
  const ready = new Promise((resolve) => element.addEventListener("ms-comment-ready", resolve, { once: true }));
  document.body.append(element);
  await ready;

  element.shadowRoot.querySelector('[part="composer-input"]').value = "A new comment";
  const created = new Promise((resolve) => element.addEventListener("ms-comment-created", resolve, { once: true }));
  element.shadowRoot.querySelector('[part="submit"]').click();
  await created;

  assert.equal(createBody.parent_id, null);
  assert.equal(createBody.body, "A new comment");
  assert.match(createBody.idempotency_key, /^[0-9a-f-]{36}$/u);
  assert.equal(element.shadowRoot.querySelector('[part="comment-body"]').textContent, "A new comment");
  element.remove();
});

test("composer creates a reply and places it under its parent", async () => {
  const root = comment("parent-1", "parent", { direct_replies_count: 1 });
  let createdReply;
  let createBody;
  globalThis.fetch = async (url, init) => {
    if (url.endsWith("/thread/ensure")) return response(thread());
    const requestURL = new URL(url, window.location.href);
    if (url.endsWith("/comment/create")) {
      createBody = JSON.parse(init.body);
      createdReply = comment("reply-created", createBody.body, {
        parent_id: "parent-1", root_id: "parent-1", path: ["parent-1", "reply-created"], depth: 1,
      });
      return response(createdReply, 201);
    }
    if (requestURL.searchParams.get("parent_id") === "parent-1") {
      return response({ items: [comment("older-reply", "older reply", {
        parent_id: "parent-1", root_id: "parent-1", path: ["parent-1", "older-reply"], depth: 1,
      })], next_cursor: null });
    }
    return response({ items: [root], next_cursor: null });
  };
  const element = document.createElement("ms-comment-thread");
  element.spaceKey = "course";
  element.resourceType = "lesson";
  element.resourceID = "lesson-1";
  const ready = new Promise((resolve) => element.addEventListener("ms-comment-ready", resolve, { once: true }));
  document.body.append(element);
  await ready;

  element.shadowRoot.querySelector('[part="reply"]').click();
  element.shadowRoot.querySelector('[part="composer-input"]').value = "nested reply";
  const created = new Promise((resolve) => element.addEventListener("ms-comment-created", resolve, { once: true }));
  element.shadowRoot.querySelector('[part="submit"]').click();
  await created;

  assert.equal(createBody.parent_id, "parent-1");
  assert.deepEqual(
    [...element.shadowRoot.querySelectorAll('[part="comment-body"]')].map((node) => node.textContent),
    ["parent", "older reply", "nested reply"],
  );
  assert.equal(element.shadowRoot.querySelector('[part="reply-context"]').hidden, true);
  element.remove();
});

test("composer uploads images before create and binds staged IDs", async () => {
  const calls = [];
  let createBody;
  const pending = {
    id: "attachment-1", status: "pending", mime_type: "image/png", size_bytes: 3,
    width: 1, height: 1, original_filename: "photo.png",
  };
  globalThis.fetch = async (url, init) => {
    if (url.endsWith("/thread/ensure")) return response(thread());
    if (url.includes("/comment/list")) return response({ items: [], next_cursor: null });
    if (url.endsWith("/comment-attachment/upload")) {
      calls.push("upload");
      assert.equal(init.body.get("thread_id"), "thread-1");
      return response(pending, 201);
    }
    if (url.endsWith("/comment/create")) {
      calls.push("create");
      createBody = JSON.parse(init.body);
      return response(comment("created-image", "photo", { attachments: [pending] }), 201);
    }
    throw new Error(`unexpected URL ${url}`);
  };
  const element = document.createElement("ms-comment-thread");
  element.spaceKey = "course";
  element.resourceType = "lesson";
  element.resourceID = "lesson-1";
  const ready = new Promise((resolve) => element.addEventListener("ms-comment-ready", resolve, { once: true }));
  document.body.append(element);
  await ready;

  const transfer = new window.DataTransfer();
  transfer.items.add(new window.File(["png"], "photo.png", { type: "image/png" }));
  element.shadowRoot.querySelector('[part="composer-files"]').files = transfer.files;
  element.shadowRoot.querySelector('[part="composer-input"]').value = "photo";
  const created = new Promise((resolve) => element.addEventListener("ms-comment-created", resolve, { once: true }));
  element.shadowRoot.querySelector('[part="submit"]').click();
  await created;

  assert.deepEqual(calls, ["upload", "create"]);
  assert.deepEqual(createBody.attachment_ids, ["attachment-1"]);
  assert.match(element.shadowRoot.querySelector('[part="attachment"]').textContent, /photo.png \(pending\)/u);
  element.remove();
});

test("composer cleans staged uploads when comment creation fails", async () => {
  let deleted = false;
  const pending = {
    id: "attachment-cleanup", status: "pending", mime_type: "image/png", size_bytes: 3,
    width: 1, height: 1, original_filename: "photo.png",
  };
  globalThis.fetch = async (url) => {
    if (url.endsWith("/thread/ensure")) return response(thread());
    if (url.includes("/comment/list")) return response({ items: [], next_cursor: null });
    if (url.endsWith("/comment-attachment/upload")) return response(pending, 201);
    if (url.endsWith("/comment/create")) return response({ error: "invalid_comment_content", message: "Rejected", details: {} }, 422);
    if (url.endsWith("/comment-attachment/delete/attachment-cleanup")) {
      deleted = true;
      return response({ ...pending, status: "deleted" });
    }
    throw new Error(`unexpected URL ${url}`);
  };
  const element = document.createElement("ms-comment-thread");
  element.spaceKey = "course";
  element.resourceType = "lesson";
  element.resourceID = "lesson-1";
  const ready = new Promise((resolve) => element.addEventListener("ms-comment-ready", resolve, { once: true }));
  document.body.append(element);
  await ready;
  const transfer = new window.DataTransfer();
  transfer.items.add(new window.File(["png"], "photo.png", { type: "image/png" }));
  element.shadowRoot.querySelector('[part="composer-files"]').files = transfer.files;
  const failed = new Promise((resolve) => element.addEventListener("ms-comment-error", resolve, { once: true }));
  element.shadowRoot.querySelector('[part="submit"]').click();
  await failed;

  assert.equal(deleted, true);
  assert.match(element.shadowRoot.querySelector('[part="composer-status"]').textContent, /Rejected/u);
  element.remove();
});

test("composer retries an ambiguous create with the same idempotent payload", async () => {
  let uploadCalls = 0;
  let deleteCalls = 0;
  const createBodies = [];
  const pending = {
    id: "attachment-retry", status: "pending", mime_type: "image/png", size_bytes: 3,
    width: 1, height: 1, original_filename: "photo.png",
  };
  globalThis.fetch = async (url, init) => {
    if (url.endsWith("/thread/ensure")) return response(thread());
    if (url.includes("/comment/list")) return response({ items: [], next_cursor: null });
    if (url.endsWith("/comment-attachment/upload")) {
      uploadCalls += 1;
      return response(pending, 201);
    }
    if (url.endsWith("/comment/create")) {
      createBodies.push(JSON.parse(init.body));
      if (createBodies.length === 1) throw new TypeError("network outcome unknown");
      return response(comment("created-retry", "photo", { attachments: [pending] }), 200);
    }
    if (url.endsWith("/comment-attachment/delete/attachment-retry")) {
      deleteCalls += 1;
      return response({ ...pending, status: "deleted" });
    }
    throw new Error(`unexpected URL ${url}`);
  };
  const element = document.createElement("ms-comment-thread");
  element.spaceKey = "course";
  element.resourceType = "lesson";
  element.resourceID = "lesson-1";
  const ready = new Promise((resolve) => element.addEventListener("ms-comment-ready", resolve, { once: true }));
  document.body.append(element);
  await ready;
  const transfer = new window.DataTransfer();
  transfer.items.add(new window.File(["png"], "photo.png", { type: "image/png" }));
  element.shadowRoot.querySelector('[part="composer-files"]').files = transfer.files;
  element.shadowRoot.querySelector('[part="composer-input"]').value = "photo";

  const failed = new Promise((resolve) => element.addEventListener("ms-comment-error", resolve, { once: true }));
  element.shadowRoot.querySelector('[part="submit"]').click();
  await failed;
  assert.equal(element.shadowRoot.querySelector('[part="submit"]').textContent, "Retry post");
  assert.equal(deleteCalls, 0);

  const created = new Promise((resolve) => element.addEventListener("ms-comment-created", resolve, { once: true }));
  element.shadowRoot.querySelector('[part="submit"]').click();
  await created;
  assert.equal(uploadCalls, 1);
  assert.equal(createBodies.length, 2);
  assert.deepEqual(createBodies[1], createBodies[0]);
  assert.equal(deleteCalls, 0);
  element.remove();
});

test("element renders only safe server links and authorized image URLs", async () => {
  const readyAttachment = {
    id: "attachment-ready", status: "ready", mime_type: "image/png", size_bytes: 3,
    width: 1, height: 1, original_filename: "safe.png",
  };
  const projected = comment("comment-safe", "safe body", {
    links: [{ url: "https://example.com/docs" }, { url: "javascript:alert(1)" }],
    attachments: [readyAttachment],
  });
  globalThis.fetch = async (url) => {
    if (url.endsWith("/thread/ensure")) return response(thread());
    if (url.includes("/comment/list")) return response({ items: [projected], next_cursor: null });
    if (url.endsWith("/comment-attachment/signed-url/attachment-ready")) {
      return response({ url: "https://files.example/signed", expires_at: new Date().toISOString() });
    }
    throw new Error(`unexpected URL ${url}`);
  };
  const element = document.createElement("ms-comment-thread");
  element.spaceKey = "course";
  element.resourceType = "lesson";
  element.resourceID = "lesson-1";
  const ready = new Promise((resolve) => element.addEventListener("ms-comment-ready", resolve, { once: true }));
  document.body.append(element);
  await ready;
  await new Promise((resolve) => setTimeout(resolve, 0));

  const anchors = element.shadowRoot.querySelectorAll('[part="comment-links"] a');
  assert.equal(anchors.length, 1);
  assert.equal(anchors[0].href, "https://example.com/docs");
  assert.equal(anchors[0].rel, "noopener noreferrer nofollow ugc");
  assert.equal(element.shadowRoot.querySelector('[part="attachment-image"]').src, "https://files.example/signed");
  assert.equal(element.shadowRoot.querySelector('[part="attachment-image"]').referrerPolicy, "no-referrer");
  element.remove();
});

test("composer reflects disabled link and image policy", async () => {
  let createCalled = false;
  const restricted = thread("thread-1", {
    policy: { ...thread().policy, allow_images: false, allow_links: false, max_attachments: 0 },
  });
  globalThis.fetch = async (url) => {
    if (url.endsWith("/thread/ensure")) return response(restricted);
    if (url.includes("/comment/list")) return response({ items: [], next_cursor: null });
    createCalled = true;
    return response(comment("unexpected", "unexpected"), 201);
  };
  const element = document.createElement("ms-comment-thread");
  element.spaceKey = "course";
  element.resourceType = "lesson";
  element.resourceID = "lesson-1";
  const ready = new Promise((resolve) => element.addEventListener("ms-comment-ready", resolve, { once: true }));
  document.body.append(element);
  await ready;
  assert.equal(element.shadowRoot.querySelector('[part="composer-files"]').hidden, true);
  element.shadowRoot.querySelector('[part="composer-input"]').value = "https://example.com";
  element.shadowRoot.querySelector('[part="submit"]').click();
  await Promise.resolve();

  assert.equal(createCalled, false);
  assert.match(element.shadowRoot.querySelector('[part="composer-status"]').textContent, /Links are disabled/u);
  element.remove();
});

test("element reports missing integration attributes", async () => {
  const element = document.createElement("ms-comment-thread");
  const failed = new Promise((resolve) => element.addEventListener("ms-comment-error", resolve, { once: true }));
  document.body.append(element);
  await failed;
  assert.match(element.shadowRoot.querySelector('[part="status"]').textContent, /are required/);
  element.remove();
});
