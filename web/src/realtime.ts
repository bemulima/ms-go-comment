import type { ChangesPage, Comment, RealtimeEnvelope, UUID } from "./contracts.js";
import { CommentClient } from "./client.js";

export interface SocketLike {
  onopen: ((event: Event) => void) | null;
  onmessage: ((event: MessageEvent<string>) => void) | null;
  onclose: ((event: CloseEvent) => void) | null;
  onerror: ((event: Event) => void) | null;
  close(code?: number, reason?: string): void;
  send(data: string): void;
}
export type SocketFactory = (url: string, protocols: string[]) => SocketLike;
export type RealtimeState = "connecting" | "connected" | "reconnecting" | "stopped";

export interface RealtimeClientOptions {
  client: CommentClient;
  threadID: UUID;
  webSocket?: SocketFactory;
  onEvent: (event: RealtimeEnvelope) => void;
  onChanges?: (comments: Comment[]) => void;
  onError?: (error: unknown) => void;
  onState?: (state: RealtimeState) => void;
  reconnectDelayMS?: number;
}

export class CommentRealtimeClient {
  private socket?: SocketLike;
  private stopped = true;
  private ready = false;
  private sequence = 0;
  private reconnectTimer?: ReturnType<typeof setTimeout>;
  private state: RealtimeState = "stopped";

  constructor(private readonly options: RealtimeClientOptions) {}

  async start(lastSequence = 0): Promise<void> {
    this.stop();
    this.stopped = false;
    this.sequence = lastSequence;
    this.setState("connecting");
    await this.connect();
  }

  stop(): void {
    this.stopped = true;
    this.ready = false;
    if (this.reconnectTimer) clearTimeout(this.reconnectTimer);
    this.reconnectTimer = undefined;
    this.socket?.close(1000, "client stopped");
    this.socket = undefined;
    this.setState("stopped");
  }

  typing(active: boolean): void {
    if (this.ready) this.socket?.send(JSON.stringify({ v: 1, type: active ? "typing.start" : "typing.stop" }));
  }

  private async connect(): Promise<void> {
    try {
      const ticket = await this.options.client.mintRealtimeTicket(this.options.threadID, this.sequence);
      if (this.stopped) return;
      const factory = this.options.webSocket ?? ((url, protocols) => new WebSocket(url, protocols));
      const socket = factory(this.socketURL(), [ticket.protocol, `ticket.${ticket.ticket}`]);
      this.socket = socket;
      socket.onopen = () => {
        this.ready = true;
        this.setState("connected");
      };
      socket.onmessage = (event) => { void this.receive(event.data); };
      socket.onerror = (event) => this.options.onError?.(event);
      socket.onclose = (event) => {
        this.ready = false;
        if (!this.stopped && event.code !== 1000) this.scheduleReconnect();
        else if (!this.stopped) {
          this.stopped = true;
          this.setState("stopped");
        }
      };
    } catch (error) {
      this.options.onError?.(error);
      if (!this.stopped) this.scheduleReconnect();
    }
  }

  private async receive(raw: string): Promise<void> {
    try {
      const event = JSON.parse(raw) as RealtimeEnvelope;
      if (event.type === "ping") {
        if (this.ready) this.socket?.send(JSON.stringify({ v: 1, type: "pong" }));
        this.options.onEvent(event);
        return;
      }
      if (event.thread_id !== this.options.threadID) return;
      if (event.type === "resync_required") await this.reconcile();
      if (event.sequence !== undefined) {
        if (event.sequence > this.sequence + 1) await this.reconcile();
        if (event.sequence <= this.sequence) return;
        this.sequence = Math.max(this.sequence, event.sequence);
      }
      this.options.onEvent(event);
    } catch (error) { this.options.onError?.(error); }
  }

  private async reconcile(): Promise<void> {
    const changes: Comment[] = [];
    let page: ChangesPage;
    do {
      page = await this.options.client.listChanges(this.options.threadID, this.sequence, 100);
      changes.push(...page.items);
      this.sequence = page.next_after_sequence;
    } while (page.has_more);
    if (changes.length) this.options.onChanges?.(changes);
  }

  private scheduleReconnect(): void {
    if (this.reconnectTimer) return;
    this.setState("reconnecting");
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = undefined;
      void this.connect();
    }, this.options.reconnectDelayMS ?? 1000);
  }

  private setState(state: RealtimeState): void {
    if (this.state === state) return;
    this.state = state;
    this.options.onState?.(state);
  }

  private socketURL(): string {
    const base = new URL(this.options.client.baseURL, globalThis.location?.href ?? "http://localhost");
    base.protocol = base.protocol === "https:" ? "wss:" : "ws:";
    base.pathname = `${base.pathname.replace(/\/$/, "")}/ws`;
    base.search = "";
    base.hash = "";
    return base.toString();
  }
}
