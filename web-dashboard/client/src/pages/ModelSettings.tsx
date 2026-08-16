import { useEffect, useMemo, useState } from "react";
import { Link } from "wouter";
import { ArrowLeft, KeyRound, Loader2, Plus, Save, ServerCog, Sparkles, TestTube2 } from "lucide-react";
import { toast } from "sonner";
import ConnectionBanner from "@/components/ConnectionBanner";
import { fetchModelSettings, getControlToken, setControlToken, updateModelSettings } from "@/lib/engineApi";

type SettingsSnapshot = Awaited<ReturnType<typeof fetchModelSettings>>;
type ProviderSnap = NonNullable<SettingsSnapshot["providers"]>[number];
type ModelSnap = NonNullable<ProviderSnap["models"]>[number];

const ROLE_OPTIONS = [
  { value: "", label: "Mặc định (default)" },
  { value: "writer", label: "Writer" },
  { value: "editor", label: "Editor" },
  { value: "architect_long", label: "Architect (dài)" },
  { value: "architect_short", label: "Architect (ngắn)" },
  { value: "arbiter", label: "Arbiter" },
  { value: "translator", label: "Translator (dịch ZH→VI)" },
  { value: "translation_coordinator", label: "Translation Coordinator" },
];

export default function ModelSettings() {
  const [settings, setSettings] = useState<SettingsSnapshot | null>(null);
  const [provider, setProvider] = useState("");
  const [model, setModel] = useState("");
  const [baseURL, setBaseURL] = useState("");
  const [apiKey, setAPIKey] = useState("");
  const [role, setRole] = useState("");
  const [contextWindow, setContextWindow] = useState("");
  const [controlToken, setControlTokenValue] = useState(() => getControlToken());
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const currentProvider = useMemo(
    () => settings?.providers.find((item) => item.name === provider) || null,
    [settings, provider],
  );
  const availableModels = currentProvider?.models || [];
  const knownModelNames = useMemo(
    () => new Set(availableModels.map((item) => item.name)),
    [availableModels],
  );
  const modelIsCustom = Boolean(model.trim()) && !knownModelNames.has(model.trim());
  const references = useMemo(() => {
    if (!settings?.references) return [] as string[];
    // references map keys are opaque provider\0model; surface all role tags for current selection if present in values
    const hits = new Set<string>();
    Object.values(settings.references).forEach((roles) => roles.forEach((roleName) => hits.add(roleName)));
    return Array.from(hits);
  }, [settings]);

  const applyProvider = (nextProvider: string, snapshot = settings, preferredModel = "") => {
    const item = snapshot?.providers.find((candidate) => candidate.name === nextProvider);
    setProvider(nextProvider);
    setBaseURL(item?.base_url || "");
    const nextModel = preferredModel
      || (snapshot?.default_provider === nextProvider ? snapshot.default_model : "")
      || item?.models[0]?.name
      || "";
    setModel(nextModel);
    const matched = item?.models.find((entry) => entry.name === nextModel);
    setContextWindow(matched?.context_window ? String(matched.context_window) : "");
    setAPIKey("");
  };

  const pickModel = (name: string) => {
    setModel(name);
    const matched = availableModels.find((entry) => entry.name === name);
    setContextWindow(matched?.context_window ? String(matched.context_window) : "");
  };

  const load = async () => {
    setLoading(true);
    try {
      const next = await fetchModelSettings();
      setSettings(next);
      const selected = next.default_provider || next.providers[0]?.name || "";
      applyProvider(selected, next, next.default_model || "");
      setError(null);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Không thể tải cấu hình Model AI.");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void load();
  }, []);

  const submit = async (testOnly: boolean) => {
    const modelName = model.trim();
    if (!provider || !modelName) {
      toast.error("Chọn provider và nhập model trước khi tiếp tục.");
      return;
    }
    setSaving(true);
    try {
      const windowValue = Number(contextWindow);
      const result = await updateModelSettings({
        provider,
        model: modelName,
        base_url: baseURL,
        api_key: apiKey,
        role: role || undefined,
        test_only: testOnly,
        context_window: Number.isFinite(windowValue) && windowValue > 0 ? windowValue : undefined,
        add_model_if_missing: true,
      });
      toast.success(testOnly ? "Kết nối model đã sẵn sàng" : "Đã lưu và áp dụng Model AI", {
        description: result.message,
      });
      if (!testOnly) await load();
    } catch (cause) {
      toast.error(testOnly ? "Không thể thử kết nối model" : "Không thể lưu cấu hình model", {
        description: cause instanceof Error ? cause.message : "Engine trả về lỗi không xác định.",
      });
    } finally {
      setSaving(false);
    }
  };

  const updateControlToken = (value: string) => {
    setControlTokenValue(value);
    setControlToken(value);
  };

  return (
    <div className="content-wrap settings-nested">
      <div className="mc-page-nav">
        <Link href="/" className="back-link"><ArrowLeft size={16} /> Điều khiển</Link>
        <div className="translation-brand">
          <span className="translation-brand-mark"><ServerCog size={16} /></span>
          <span>Cài đặt Model AI</span>
        </div>
      </div>

      <div className="translation-heading">
        <div>
          <p className="eyebrow">CẤU HÌNH ENGINE</p>
          <h1>Model & credential</h1>
          <p className="lede">
            Chọn provider + model, rồi chọn đúng <strong>role</strong> cần áp dụng.
            Role <strong>Translator</strong> điều khiển dịch ZH→VI — đổi default alone không đổi translator.
            Tránh slug dạng <code>zyloo/…</code> nếu proxy không có credential provider <code>zyloo</code>.
          </p>
        </div>
        <div className="translation-book">
          <span>AN TOÀN</span>
          <strong>Key không về UI</strong>
          <small>
            {currentProvider?.has_api_key
              ? `Đang dùng ${currentProvider.api_key_hint || "credential đã lưu"}`
              : "Chưa có credential"}
          </small>
        </div>
      </div>

      <ConnectionBanner
        connected={!error}
        loading={loading}
        transport={error ? "offline" : "live"}
        error={error}
        retryIn={0}
        onRetry={() => void load()}
      />

      <section className="translation-panel" style={{ maxWidth: 920, margin: "28px auto" }}>
        <div className="batch-toolbar">
          <div>
            <p className="panel-kicker">MODEL WORKBENCH</p>
            <h2>Provider · model tự do · role</h2>
            <span>
              {settings?.config_path
                ? `Tệp cấu hình: ${settings.config_path}`
                : "Đang chờ engine cung cấp cấu hình"}
            </span>
          </div>
        </div>

        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(260px, 1fr))", gap: 18, padding: 24 }}>
          <label className="settings-field">
            <span>Provider</span>
            <select
              value={provider}
              onChange={(event) => applyProvider(event.target.value)}
              disabled={loading || saving}
            >
              {(settings?.providers || []).map((item) => (
                <option key={item.name} value={item.name}>
                  {item.name} · {item.type || item.api || "custom"}
                </option>
              ))}
            </select>
          </label>

          <label className="settings-field">
            <span>Gán cho role</span>
            <select value={role} onChange={(event) => setRole(event.target.value)} disabled={loading || saving}>
              {ROLE_OPTIONS.map((item) => (
                <option key={item.value || "default"} value={item.value}>{item.label}</option>
              ))}
            </select>
          </label>

          <label className="settings-field settings-wide">
            <span>
              Model id <em>(gõ tự do — không bị khóa list)</em>
            </span>
            <input
              list="ainovel-model-suggestions"
              value={model}
              onChange={(event) => setModel(event.target.value)}
              placeholder="vd. google/gemini-2.5-flash hoặc zyloo/gemini-3.1-pro-preview"
              disabled={loading || saving}
              autoComplete="off"
              spellCheck={false}
            />
            <datalist id="ainovel-model-suggestions">
              {availableModels.map((item) => (
                <option key={item.name} value={item.name} />
              ))}
            </datalist>
            <small>
              {modelIsCustom
                ? "Model mới — sẽ được thêm vào thư viện provider khi lưu."
                : availableModels.length > 0
                  ? `${availableModels.length} model đã đăng ký cho provider này (gợi ý bên dưới).`
                  : "Provider chưa có model đăng ký — gõ id rồi lưu."}
            </small>
          </label>

          {availableModels.length > 0 && (
            <div className="settings-field settings-wide">
              <span>Gợi ý nhanh từ config</span>
              <div className="model-chip-row">
                {availableModels.map((item: ModelSnap) => {
                  const active = item.name === model;
                  return (
                    <button
                      key={item.name}
                      type="button"
                      className={`model-chip ${active ? "is-active" : ""}`}
                      onClick={() => pickModel(item.name)}
                      disabled={loading || saving}
                    >
                      {item.name}
                      {item.context_window ? <em>{Math.round(item.context_window / 1000)}k</em> : null}
                    </button>
                  );
                })}
              </div>
            </div>
          )}

          <label className="settings-field">
            <span>Context window <em>(tuỳ chọn)</em></span>
            <input
              value={contextWindow}
              onChange={(event) => setContextWindow(event.target.value.replace(/[^\d]/g, ""))}
              placeholder="vd. 200000"
              disabled={loading || saving}
              inputMode="numeric"
            />
            <small>Để trống = giữ mặc định engine. Hữu ích khi model mới chưa có trong catalog.</small>
          </label>

          <label className="settings-field settings-wide">
            <span>Base URL</span>
            <input
              value={baseURL}
              onChange={(event) => setBaseURL(event.target.value)}
              placeholder="https://api.example.com/v1"
              disabled={loading || saving}
            />
          </label>

          <label className="settings-field settings-wide">
            <span>
              <KeyRound size={13} /> API Key mới <em>(để trống để giữ key hiện tại)</em>
            </span>
            <input
              value={apiKey}
              onChange={(event) => setAPIKey(event.target.value)}
              type="password"
              placeholder={currentProvider?.has_api_key ? currentProvider.api_key_hint : "Nhập key khi provider yêu cầu"}
              disabled={loading || saving}
            />
          </label>

          <label className="settings-field settings-wide">
            <span>
              <KeyRound size={13} /> Control token local <em>(chỉ khi API Go bật --token)</em>
            </span>
            <input
              value={controlToken}
              onChange={(event) => updateControlToken(event.target.value)}
              type="password"
              autoComplete="off"
              placeholder="Token xác thực endpoint điều khiển"
              disabled={saving}
            />
            <small>Chỉ lưu trong session trình duyệt, gửi qua Authorization cho POST điều khiển.</small>
          </label>
        </div>

        <div className="settings-actions">
          <button type="button" className="subtle-button" onClick={() => void submit(true)} disabled={saving || loading}>
            <TestTube2 size={15} /> {saving ? <Loader2 className="spin" size={15} /> : "Thử kết nối"}
          </button>
          <button type="button" className="primary-button" onClick={() => void submit(false)} disabled={saving || loading}>
            {modelIsCustom ? <Plus size={15} /> : <Save size={15} />}
            {modelIsCustom ? "Thêm model & áp dụng" : "Lưu và áp dụng"}
          </button>
        </div>
      </section>

      <section className="translation-footnote" style={{ maxWidth: 920, margin: "0 auto" }}>
        <Sparkles size={14} />
        <span>
          <strong>Thử kết nối</strong> gọi model thật nhưng chưa ghi file.
          <strong> Lưu</strong> sẽ đăng ký model vào provider (nếu chưa có), cập nhật config trên đĩa và hot-swap role đã chọn.
          {references.length > 0 ? ` Roles đang tham chiếu model trong config: ${references.join(", ")}.` : ""}
        </span>
      </section>
    </div>
  );
}
