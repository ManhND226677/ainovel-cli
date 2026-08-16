import { useContext, useMemo, type ReactNode } from "react";
import { useEngineDashboard } from "@/hooks/useEngineDashboard";
import { nowPlaying } from "@/lib/statusCopy";
import {
  EngineContext,
  offlineEngineValue,
  type EngineContextValue,
} from "./engine-context-base";

export type { EngineContextValue } from "./engine-context-base";

export function EngineProvider({ children }: { children: ReactNode }) {
  const {
    engine,
    events,
    liveProgress,
    liveProgressHistory,
    connected,
    loading,
    error,
    transport,
    retryIn,
    refresh,
  } = useEngineDashboard({});

  const playing = useMemo(() => nowPlaying(engine, events), [engine, events]);
  const workingLabel = playing.agent ? `${playing.agent}: ${playing.detail}` : playing.title;

  const value = useMemo<EngineContextValue>(() => ({
    engine,
    events,
    liveProgress,
    liveProgressHistory,
    connected,
    loading,
    error,
    transport,
    retryIn,
    refresh,
    playing,
    workingLabel,
  }), [
    engine,
    events,
    liveProgress,
    liveProgressHistory,
    connected,
    loading,
    error,
    transport,
    retryIn,
    refresh,
    playing,
    workingLabel,
  ]);

  return <EngineContext.Provider value={value}>{children}</EngineContext.Provider>;
}

/**
 * Read engine telemetry. During Vite HMR the provider module can briefly
 * desync from consumers; fall back to a quiet offline stub instead of
 * crashing the whole dashboard with a white error screen.
 */
export function useEngine(): EngineContextValue {
  const ctx = useContext(EngineContext);
  if (!ctx) {
    if (import.meta.env.DEV) {
      console.warn("[ainovel] useEngine outside EngineProvider (often HMR) — using offline stub");
    }
    return offlineEngineValue;
  }
  return ctx;
}
