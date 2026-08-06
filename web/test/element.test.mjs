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

const { defineCommentThreadElement } = await import("../dist/element.js");
defineCommentThreadElement();

function response(body, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

function comment(id, body) {
  return {
    id, thread_id: "thread-1", author_id: "user-1", root_id: id, path: [id], depth: 0,
    body, links: [], attachments: [], status: "active", version: 1, sequence: 1,
    direct_replies_count: 0, created_at: new Date().toISOString(), updated_at: new Date().toISOString(),
  };
}

test("element loads a thread and safely renders root comments", async () => {
  const calls = [];
  globalThis.fetch = async (url, init) => {
    calls.push({ url, init });
    if (url.endsWith("/thread/ensure")) return response({ id: "thread-1", last_sequence: 1 });
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
    if (url.endsWith("/thread/ensure")) return response({ id: "thread-1", last_sequence: 2 });
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
      return response({ id: resourceID === "lesson-1" ? "thread-1" : "thread-2", last_sequence: 2 });
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

test("element reports missing integration attributes", async () => {
  const element = document.createElement("ms-comment-thread");
  const failed = new Promise((resolve) => element.addEventListener("ms-comment-error", resolve, { once: true }));
  document.body.append(element);
  await failed;
  assert.match(element.shadowRoot.querySelector('[part="status"]').textContent, /are required/);
  element.remove();
});
