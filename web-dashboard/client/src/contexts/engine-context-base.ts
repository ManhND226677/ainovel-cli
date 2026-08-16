/** Stable context object — keep this file tiny so HMR rarely remints the identity. */
import { createContext } from "react";
import type { EngineState, LiveEvent, LiveTranslationProgress } from "@/lib/engineApi";
import type { nowPlaying } from "@/lib/statusCopy";

export type EngineContextValue = {
  engine: EngineState | null;
  events: LiveEvent[];
  liveProgress: LiveTranslationProgress[];
  liveProgressHistory: Record<number, LiveTranslationProgress[]>;
  connected: boolean;
  loading: boolean;
  error: string | null;
  transport: "connecting" | "live" | "polling" | "offline";
  retryIn: number;
  refresh: () => Promise<void>;
  workingLabel?: string;
  playing: ReturnType<typeof nowPlaying>;
};

export const EngineContext = createContext<EngineContextValue | null>(null);

export const offlineEngineValue: EngineContextValue = {
  engine: null,
  events: [],
  liveProgress: [],
  liveProgressHistory: {},
  connected: false,
  loading: true,
  error: null,
  transport: "connecting",
  retryIn: 0,
  refresh: async () => {},
  workingLabel: "Đang kết nối…",
  playing: {
    title: "Đang kết nối engine",
    detail: "Chờ EngineProvider sẵn sàng.",
    tone: "off",
  },
};
