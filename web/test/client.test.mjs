import test from "node:test";
import assert from "node:assert/strict";
import { CommentAPIError, CommentClient, CommentRealtimeClient } from "../dist/index.js";

test("REST client uses gateway credentials and only the comment grant header", async () => {
  const calls = [];
  const client = new CommentClient({
    baseURL: "https://app.example/api/comment/v1/",
    accessGrant: () => "private-grant",
    fetch: async (url, init) => {
      calls.push({ url, init });
      return new Response(JSON.stringify({ id: "thread-1" }), { status: 200, headers: { "Content-Type": "application/json" } });
    },
  });
  await client.ensureThread("course", "lesson", "lesson-1");
  assert.equal(calls[0].url, "https://app.example/api/comment/v1/thread/ensure");
  assert.equal(calls[0].init.credentials, "include");
  assert.equal(calls[0].init.headers.get("X-Comment-Access-Grant"), "private-grant");
  assert.equal(calls[0].init.headers.has("X-User-ID"), false);
  assert.equal(calls[0].init.headers.has("X-User-Role"), false);
});

test("REST client exposes stable API errors and request IDs", async () => {
  const client = new CommentClient({
    fetch: async () => new Response(JSON.stringify({ error: "rate_limited", message: "slow down", details: {} }), {
      status: 429, headers: { "Content-Type": "application/json", "X-Request-ID": "request-1" },
    }),
  });
  await assert.rejects(client.getThread("thread-1"), (error) => {
    assert.ok(error instanceof CommentAPIError);
    assert.equal(error.code, "rate_limited");
    assert.equal(error.requestID, "request-1");
    return true;
  });
});

test("realtime client keeps tickets out of URLs and reconciles sequence gaps", async () => {
  const calls = [];
  const client = new CommentClient({
    baseURL: "https://app.example/api/comment/v1",
    fetch: async (url) => {
      calls.push(url);
      if (url.endsWith("/realtime/ticket")) {
        return Response.json({ ticket: "opaque-ticket", expires_at: new Date().toISOString(), protocol: "comment.v1" }, { status: 201 });
      }
      return Response.json({ items: [{ id: "comment-1" }], next_after_sequence: 2, has_more: false });
    },
  });
  let socket;
  let socketURL;
  let protocols;
  const states = [];
  const sent = [];
  const changes = [];
  const realtime = new CommentRealtimeClient({
    client, threadID: "thread-1", onEvent: () => {}, onChanges: (items) => changes.push(...items),
    onState: (state) => states.push(state), reconnectDelayMS: 1,
    webSocket: (url, selectedProtocols) => {
      socketURL = url;
      protocols = selectedProtocols;
      socket = { onopen: null, onmessage: null, onclose: null, onerror: null, close() {}, send(data) { sent.push(data); } };
      return socket;
    },
  });
  await realtime.start(0);
  assert.equal(socketURL, "wss://app.example/api/comment/v1/ws");
  assert.deepEqual(protocols, ["comment.v1", "ticket.opaque-ticket"]);
  assert.equal(socketURL.includes("opaque-ticket"), false);
  socket.onopen({});
  socket.onmessage({ data: JSON.stringify({ v: 1, type: "ping" }) });
  assert.equal(sent[0], JSON.stringify({ v: 1, type: "pong" }));
  socket.onmessage({ data: JSON.stringify({ v: 1, type: "comment.created", thread_id: "thread-1", sequence: 2, data: {} }) });
  await new Promise((resolve) => setTimeout(resolve, 10));
  assert.equal(changes.length, 1);
  assert.ok(calls.some((url) => url.includes("/comment/changes?")));
  socket.onclose({ code: 1006 });
  await new Promise((resolve) => setTimeout(resolve, 10));
  assert.equal(calls.filter((url) => url.endsWith("/realtime/ticket")).length, 2);
  realtime.stop();
  assert.ok(states.includes("connecting"));
  assert.ok(states.includes("connected"));
  assert.ok(states.includes("reconnecting"));
  assert.equal(states.at(-1), "stopped");
});
