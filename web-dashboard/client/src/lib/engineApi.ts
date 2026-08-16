const RAW_ENGINE_API = (import.meta.env.VITE_ENGINE_API_URL as string | undefined)?.trim();
export const ENGINE_API_BASE = (RAW_ENGINE_API && RAW_ENGINE_API.length > 0 ? RAW_ENGINE_API : "/api").replace(/\/$/, "");

export type AgentRole = "Architect" | "Writer" | "Editor" | "Arbiter" | "Translation Agent";

export type LiveAgent = {
  name: AgentRole | string;
  role: AgentRole | string;
  state: string;
  task_id?: string;
  task_kind?: string;
  summary?: string;
  tool?: string;
  turn: number;
  updated_at: string;
  context: {
    tokens: number;
    window: number;
    percent: number;
    scope: string;
    strategy: string;
    active_messages: number;
    summary_messages: number;
    compacted_count: number;
    kept_count: number;
  };
};

export type LiveEvent = {
  seq: number;
  time: string;
  task_id?: string;
  agent?: string;
  category?: string;
  kind?: string;
  summary?: string;
  priority?: string;
  level?: string;
  detail?: string;
  failed?: boolean;
  finished_at?: string;
  retry_at?: string;
};

export type TranslationChapterState = "pending" | "running" | "completed" | "failed" | "stale";

export type TranslationChapter = {
  chapter: number;
  state: TranslationChapterState;
  source_sha256: string;
  translated_sha256?: string;
  job_id?: string;
  glossary_version?: number;
  provider?: string;
  model?: string;
  attempts?: number;
  last_error?: string;
	instruction?: string;
  updated_at: string;
};

export type LiveTranslationProgress = {
	chapter: number;
	stage: "preparing" | "streaming" | "completed" | "failed" | string;
	source_preview?: string;
	preview?: string;
	detail?: string;
	error?: string;
	updated_at: string;
};

export type TranslationJob = {
  id: string;
  state: "queued" | "running" | "completed" | "failed" | "cancelled";
  chapters: number[];
  reason?: string;
  attempts?: number;
  last_error?: string;
  created_at: string;
  updated_at: string;
};

export type TranslationStatus = {
  schema_version: number;
  chapters: Record<string, TranslationChapter>;
  jobs: Record<string, TranslationJob>;
  updated_at: string;
};

export type EngineState = {
  online: boolean;
  snapshot: {
    novel_name: string;
    runtime_state: string;
    status_label: string;
    phase: string;
    flow: string;
    current_chapter: number;
    total_chapters: number;
    completed_count: number;
    total_word_count: number;
    total_cost_usd: number;
    budget_limit_usd: number;
    is_running: boolean;
    current_volume_arc: string;
    next_volume_title: string;
    last_commit_summary: string;
    last_review_summary: string;
    advance_mode?: string;
    advance_permit_chapter?: number;
    has_advance_hold?: boolean;
    advance_hold_reason?: string;
    pending_steer?: string;
    book_dir?: string;
  };
	agents: LiveAgent[];
	translation?: TranslationStatus;
	translation_paused?: boolean;
	updated_at: string;
};

export type AgentDetail = {
  agent: LiveAgent | null;
  role: string;
  history: LiveEvent[];
};

export type EngineSocketMessage = {
	type: "snapshot" | "update" | "translation_progress" | "error";
	state?: EngineState;
	event?: LiveEvent;
	events?: LiveEvent[];
	live_progress?: LiveTranslationProgress;
  error?: string;
  reconnect?: boolean;
};

const CONTROL_TOKEN_STORAGE_KEY = "ainovel-control-token";

export function getControlToken() {
  if (typeof window === "undefined") return "";
  return window.sessionStorage.getItem(CONTROL_TOKEN_STORAGE_KEY) || "";
}

export function setControlToken(token: string) {
  if (typeof window === "undefined") return;
  const value = token.trim();
  if (value) window.sessionStorage.setItem(CONTROL_TOKEN_STORAGE_KEY, value);
  else window.sessionStorage.removeItem(CONTROL_TOKEN_STORAGE_KEY);
}

async function getJSON<T>(path: string): Promise<T> {
  const response = await fetch(`${ENGINE_API_BASE}${path}`, { headers: { Accept: "application/json" } });
  if (!response.ok) {
    let detail = "";
    try {
      const payload = await response.json() as { error?: string };
      detail = payload.error || "";
    } catch {
      detail = response.statusText;
    }
    throw new Error(`Engine API ${response.status}${detail ? `: ${detail}` : ""}`);
  }
  return response.json() as Promise<T>;
}

async function postJSON<T>(path: string, body: unknown = {}): Promise<T> {
  const token = getControlToken();
  const response = await fetch(`${ENGINE_API_BASE}${path}`, {
    method: "POST",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify(body),
  });
  if (!response.ok) {
    let detail = "";
    try {
      const payload = await response.json() as { error?: string };
      detail = payload.error || "";
    } catch {
      detail = response.statusText;
    }
    throw new Error(`Engine API ${response.status}${detail ? `: ${detail}` : ""}`);
  }
  return response.json() as Promise<T>;
}

export function fetchEngineState() { return getJSON<EngineState>("/state"); }

export function fetchEngineEvents(filters: { agent?: string; category?: string; kind?: string; level?: string } = {}) {
  const query = new URLSearchParams();
  Object.entries(filters).forEach(([key, value]) => { if (value) query.set(key, value); });
  return getJSON<{ items: LiveEvent[]; next: number }>(`/events${query.toString() ? `?${query}` : ""}`);
}

export function fetchAgentDetail(role: string) {
  return getJSON<AgentDetail>(`/agents/${encodeURIComponent(role.toLowerCase().replaceAll(" ", "-"))}`);
}

export function getTranslationReportUrl(format: "md" | "txt" = "md", jobId?: string) {
  const query = new URLSearchParams({ format });
  if (jobId) query.set("job_id", jobId);
  return `${ENGINE_API_BASE}/translation/report?${query.toString()}`;
}

export function retryTranslation(chapters: number[]) {
  return postJSON<{ ok: boolean; chapters: number[]; message: string }>("/translation/retry", { chapters });
}

export function requestTranslation() {
	return postJSON<{ ok: boolean; message: string }>("/translation/request");
}

export function pauseTranslation() {
	return postJSON<{ ok: boolean; paused: boolean; message: string }>("/translation/pause");
}

export function resumeTranslation() {
	return postJSON<{ ok: boolean; paused: boolean; message: string }>("/translation/resume");
}

export function stopTranslationChapter(chapter: number) {
	return postJSON<{ ok: boolean; message: string }>(`/translation/chapter/${chapter}/stop`);
}

export function setTranslationInstruction(chapter: number, instruction: string) {
	return postJSON<TranslationChapter>(`/translation/chapter/${chapter}/instruction`, { instruction });
}

export function fetchGlossary() {
  return getJSON<{ version: number; terms: Record<string, { source: string; vietnamese: string; first_chapter: number; last_chapter: number; updated_at: string }> }>("/translation/glossary");
}

export function updateGlossary(entries: Record<string, string>) {
  return postJSON<{ version: number; terms: Record<string, { source: string; vietnamese: string; first_chapter: number; last_chapter: number; updated_at: string }> }>("/translation/glossary", { entries });
}

export function fetchSnapshots() {
  return getJSON<{ items: Array<{ id: string; title: string; created_at: string; size_kb: number; files: number }> }>("/snapshots");
}

export function createSnapshot(title: string) {
  return postJSON<{ id: string; title: string; created_at: string; size_kb: number; files: number }>("/snapshots", { title });
}

export function restoreSnapshot(snapshotId: string) {
  return postJSON<{ ok: boolean; message: string }>("/snapshots/restore", { id: snapshotId, confirm: true });
}

export function fetchModelSettings() {
  return getJSON<{
    providers: Array<{
      name: string;
      type: string;
      api: string;
      base_url: string;
      models: Array<{ name: string; context_window?: number }>;
      has_api_key: boolean;
      api_key_hint: string;
      requires_api_key: boolean;
    }>;
    default_provider: string;
    default_model: string;
    config_path: string;
    references: Record<string, string[]>;
  }>("/settings/model");
}

export function updateModelSettings(settings: {
  provider: string;
  base_url: string;
  api_key: string;
  model: string;
  role?: string;
  test_only?: boolean;
  context_window?: number;
  add_model_if_missing?: boolean;
}) {
  return postJSON<{ ok: boolean; message: string; model?: string; added?: boolean }>("/settings/model", settings);
}

export function getEngineWebSocketURL() {
  const origin = typeof window !== "undefined" ? window.location.origin : "http://127.0.0.1:10001";
  const base = (ENGINE_API_BASE.startsWith("http://") || ENGINE_API_BASE.startsWith("https://"))
    ? ENGINE_API_BASE
    : `${origin}${ENGINE_API_BASE.startsWith("/") ? "" : "/"}${ENGINE_API_BASE}`;
  const url = new URL(base);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  url.pathname = `${url.pathname.replace(/\/$/, "")}/ws`;
  url.search = "";
  return url.toString();
}


export function resumeEngine() { return postJSON<{ ok: boolean }>("/engine/resume"); }
export function abortEngine() { return postJSON<{ ok: boolean }>("/engine/abort"); }
export function continueEngine(prompt: string) { return postJSON<{ ok: boolean }>("/engine/continue", { prompt }); }
export function steerEngine(prompt: string) { return postJSON<{ ok: boolean }>("/engine/steer", { prompt }); }
export function setAdvanceMode(mode: "auto" | "review" | "on" | "off" | "bat" | "tat") {
  return postJSON<{ ok: boolean; mode?: string }>("/engine/advance", { mode });
}
export function advanceNextChapter() {
  return postJSON<{ ok: boolean }>("/engine/advance/next");
}

export type ManuscriptChapterMeta = {
  chapter: number;
  title?: string;
  core_event?: string;
  completed: boolean;
  word_count?: number;
  translation_state?: string;
};

export type ChapterManuscript = {
  chapter: number;
  title?: string;
  core_event?: string;
  chinese?: string;
  vietnamese?: string;
  chinese_chars: number;
  vietnamese_chars: number;
  completed: boolean;
  translation_state?: string;
  translation_error?: string;
  source_sha256?: string;
};

export function fetchManuscriptOutline() {
  return getJSON<{ items: ManuscriptChapterMeta[]; updated_at: string }>("/manuscript/outline");
}

export function fetchManuscriptChapter(chapter: number) {
  return getJSON<ChapterManuscript>(`/manuscript/chapters/${chapter}`);
}

export function isAgentRole(value: string): value is AgentRole {
  return ["Architect", "Writer", "Editor", "Arbiter", "Translation Agent"].includes(value);
}


// ── Library + Export ─────────────────────────────────────────

export type LibraryBook = {
  id: string;
  title: string;
  title_vi?: string;
  author?: string;
  description?: string;
  path: string;
  slug: string;
  created_at?: string;
  updated_at?: string;
  phase?: string;
  completed_zh?: number;
  total_chapters?: number;
  completed_vi?: number;
  active?: boolean;
  exists?: boolean;
};

export type LibraryResponse = {
  root: string;
  active_id?: string;
  books: LibraryBook[];
};

async function apiJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const method = (init?.method || "GET").toUpperCase();
  const token = method === "GET" || method === "HEAD" ? "" : getControlToken();
  const res = await fetch(`${ENGINE_API_BASE}${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(init?.headers || {}),
    },
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error((data as { error?: string }).error || res.statusText || "request failed");
  }
  return data as T;
}

export function fetchLibrary() {
  return apiJSON<LibraryResponse>("/library");
}

export function createLibraryBook(body: {
  title: string;
  title_vi?: string;
  author?: string;
  description?: string;
  slug?: string;
  open?: boolean;
}) {
  return apiJSON<{ book: LibraryBook; open?: boolean; error?: string }>("/library", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function openLibraryBook(body: { id?: string; path?: string }) {
  return apiJSON<{ ok: boolean; id?: string; path?: string; dir?: string }>("/library/open", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

/** Cất truyện vào archive (chỉ cho phép với truyện không đang mở). */
export function archiveLibraryBook(body: { id: string }) {
  return apiJSON<{ ok: boolean }>("/library/archive", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export type ActiveBookSummary = {
  dir?: string;
  title?: string;
  phase?: string;
  completed?: number;
  has_chapter_files?: boolean;
  blocks_new_start?: boolean;
  message?: string;
};

export function fetchActiveBook() {
  return apiJSON<{ active: ActiveBookSummary; dir?: string }>("/library/active");
}

/** Safe new-novel path: create library folder, switch, then StartPrepared. Never wipes the previous book. */
export function createLibraryBookAndStart(body: {
  title?: string;
  title_vi?: string;
  author?: string;
  description?: string;
  slug?: string;
  prompt: string;
}) {
  return postJSON<{ ok: boolean; book: LibraryBook; dir?: string }>("/library/create-and-start", body);
}

export type CoCreateSession = {
  active: boolean;
  mode?: "startup" | "stage" | string;
  title?: string;
  draft?: string;
  ready?: boolean;
  can_commit?: boolean;
  suggestions?: string[];
  busy?: boolean;
  history?: Array<{ role: string; content: string }>;
  updated_at?: string;
  stage_host?: boolean;
};

export function fetchCoCreate() {
  return getJSON<CoCreateSession>("/cocreate");
}
export function startCoCreate(body: { mode?: "startup" | "stage"; message: string; title?: string }) {
  return postJSON<{ ok: boolean; error?: string; session?: CoCreateSession }>("/cocreate/start", body);
}
export function messageCoCreate(message: string) {
  return postJSON<{ ok: boolean; error?: string; session?: CoCreateSession }>("/cocreate/message", { message });
}
export function commitCoCreate(body: { title?: string; draft?: string } = {}) {
  return postJSON<{ ok: boolean; mode?: string; message?: string; title?: string; dir?: string }>("/cocreate/commit", body);
}
export function cancelCoCreate() {
  return postJSON<{ ok: boolean }>("/cocreate/cancel", {});
}

export type ExportRequest = {
  format?: "txt" | "epub";
  language?: "zh" | "vi";
  from?: number;
  to?: number;
  author?: string;
  title?: string;
  download?: boolean;
};

/** Parse download filename from Content-Disposition (prefer RFC 5987 filename*). */
export function parseContentDispositionFilename(cd: string, fallback = "ban-thao.epub"): string {
  if (!cd) return fallback;
  // filename*=UTF-8''percent-encoded  (or filename*=utf-8''…)
  const star = /filename\*\s*=\s*(?:UTF-8''|utf-8'')([^;]+)/i.exec(cd);
  if (star?.[1]) {
    try {
      const decoded = decodeURIComponent(star[1].trim().replace(/^["']|["']$/g, ""));
      if (decoded) return decoded;
    } catch {
      /* fall through */
    }
  }
  // filename="..." or filename=...
  const plain = /filename\s*=\s*("(?:\\.|[^"])*"|[^;]+)/i.exec(cd);
  if (plain?.[1]) {
    let v = plain[1].trim();
    if (v.startsWith('"') && v.endsWith('"')) v = v.slice(1, -1).replace(/\\"/g, '"');
    v = v.trim();
    // Ignore generic stubs the old backend used to emit.
    if (v && !/^export\.(epub|txt)$/i.test(v)) return v;
  }
  return fallback;
}

export async function requestExport(body: ExportRequest) {
  if (body.download) {
    const token = getControlToken();
    const res = await fetch(`${ENGINE_API_BASE}/export`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: JSON.stringify({ ...body, download: true }),
    });
    if (!res.ok) {
      const data = await res.json().catch(() => ({}));
      throw new Error((data as { error?: string }).error || res.statusText);
    }
    const blob = await res.blob();
const ext = body.format || "epub";
    const lang = body.language || "zh";
    const fallback =
      lang === "vi" ? `ban-dich-viet.${ext}` : `ban-thao-trung.${ext}`;
    // Prefer X-Export-Filename (percent-encoded UTF-8) then filename*=UTF-8''…
    let headerName = res.headers.get("X-Export-Filename")?.trim() || "";
    if (headerName) {
      try {
        headerName = decodeURIComponent(headerName);
      } catch {
        /* keep raw */
      }
    }
    const cd = res.headers.get("Content-Disposition") || "";
    const name = headerName || parseContentDispositionFilename(cd, fallback);
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = name;
    a.click();
    URL.revokeObjectURL(url);
    return { downloaded: true as const, name };
  }
  return apiJSON<{
    path: string;
    chapters: number;
    bytes: number;
    skipped?: number[];
    format: string;
    language: string;
  }>("/export", { method: "POST", body: JSON.stringify(body) });
}
