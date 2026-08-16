import { beforeEach, describe, expect, it, vi } from "vitest";
import { createSnapshot, pauseTranslation, requestTranslation, restoreSnapshot, resumeTranslation, retryTranslation, setControlToken, setTranslationInstruction, stopTranslationChapter, updateGlossary, updateModelSettings } from "./engineApi";

const values = new Map<string, string>();

Object.defineProperty(globalThis, "window", {
  configurable: true,
  value: {
    sessionStorage: {
      getItem: (key: string) => values.get(key) || null,
      setItem: (key: string, value: string) => values.set(key, value),
      removeItem: (key: string) => values.delete(key),
    },
  },
});

describe("engineApi control token", () => {
  beforeEach(() => {
    values.clear();
    vi.stubGlobal("fetch", vi.fn().mockImplementation(() => Promise.resolve(new Response(JSON.stringify({ ok: true, chapters: [8], message: "queued" }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }))));
  });

  it("gắn Bearer token cho POST điều khiển", async () => {
    setControlToken(" local-control-token ");

    await retryTranslation([8]);

    expect(fetch).toHaveBeenCalledWith(expect.stringContaining("/translation/retry"), expect.objectContaining({
      method: "POST",
      headers: expect.objectContaining({ Authorization: "Bearer local-control-token" }),
    }));
  });

  it("xóa Authorization khi token được để trống", async () => {
    setControlToken("token-to-clear");
    setControlToken("");

    await retryTranslation([8]);

    const request = vi.mocked(fetch).mock.calls[0]?.[1] as RequestInit;
    expect(request.headers).not.toHaveProperty("Authorization");
  });

  it("gắn Bearer token cho glossary, snapshot và Model AI", async () => {
    setControlToken("workspace-token");

	await requestTranslation();
    await updateGlossary({ "灵脉": "linh mạch" });
    await createSnapshot("Trước khi sửa chương 12");
    await restoreSnapshot("snap-2026-08-12");
    await updateModelSettings({ provider: "proxy", model: "writer-model", base_url: "https://api.example/v1", api_key: "", test_only: true });

    const requests = vi.mocked(fetch).mock.calls.map(([url, options]) => ({ url: String(url), options: options as RequestInit }));
    expect(requests.map((item) => item.url)).toEqual(expect.arrayContaining([
		expect.stringContaining("/translation/request"),
      expect.stringContaining("/translation/glossary"),
      expect.stringContaining("/snapshots"),
      expect.stringContaining("/snapshots/restore"),
      expect.stringContaining("/settings/model"),
    ]));
    for (const { options } of requests) {
      expect(options.headers).toMatchObject({ Authorization: "Bearer workspace-token" });
    }
  });

  it("gắn Bearer token và payload đúng cho control telemetry theo chapter", async () => {
    setControlToken("telemetry-token");

    await pauseTranslation();
    await resumeTranslation();
    await stopTranslationChapter(17);
    await setTranslationInstruction(17, "Giữ xưng hô ta – ngươi.");

    const requests = vi.mocked(fetch).mock.calls.map(([url, options]) => ({ url: String(url), options: options as RequestInit }));
    expect(requests.map((item) => item.url)).toEqual(expect.arrayContaining([
      expect.stringContaining("/translation/pause"),
      expect.stringContaining("/translation/resume"),
      expect.stringContaining("/translation/chapter/17/stop"),
      expect.stringContaining("/translation/chapter/17/instruction"),
    ]));
    for (const { options } of requests) {
      expect(options.headers).toMatchObject({ Authorization: "Bearer telemetry-token" });
    }
    const instructionRequest = requests.find((item) => item.url.endsWith("/translation/chapter/17/instruction"));
    expect(instructionRequest?.options.body).toBe(JSON.stringify({ instruction: "Giữ xưng hô ta – ngươi." }));
  });
});
