export type UUID = string;

export interface Policy {
  allow_images: boolean;
  allow_links: boolean;
  max_depth: number;
  max_body_length: number;
  max_attachments: number;
  max_image_bytes: number;
  edit_window_seconds: number;
}

export interface Thread {
  id: UUID;
  space_id: UUID;
  resource_type: string;
  resource_id: string;
  status: "open" | "read_only" | "closed" | "hidden";
  policy: Policy;
  comment_count: number;
  root_comment_count: number;
  last_sequence: number;
  last_comment_at?: string;
  created_at: string;
  updated_at: string;
}

export interface Attachment {
  id: UUID;
  status: "pending" | "processing" | "ready" | "failed" | "deleted";
  mime_type: string;
  size_bytes: number;
  width: number;
  height: number;
  original_filename: string;
}

export interface CommentLink { url: string; }

export interface Comment {
  id: UUID;
  thread_id: UUID;
  author_id: UUID;
  parent_id?: UUID;
  root_id: UUID;
  path: UUID[];
  depth: number;
  body: string;
  links: CommentLink[];
  attachments: Attachment[];
  status: "active" | "deleted" | "hidden";
  version: number;
  sequence: number;
  direct_replies_count: number;
  edited_at?: string;
  deleted_at?: string;
  created_at: string;
  updated_at: string;
}

export interface Page<T> { items: T[]; next_cursor: string | null; }
export interface ChangesPage { items: Comment[]; next_after_sequence: number; has_more: boolean; }
export interface SignedURL { url: string; expires_at: string; }
export interface RealtimeTicket { ticket: string; expires_at: string; protocol: "comment.v1"; }

export interface RealtimeEnvelope<T = unknown> {
  v: 1;
  type: string;
  event_id?: UUID;
  thread_id: UUID;
  sequence?: number;
  current_sequence?: number;
  last_sequence?: number;
  occurred_at?: string;
  data?: T;
}

export interface APIErrorBody { error: string; message: string; details: Record<string, unknown>; }
