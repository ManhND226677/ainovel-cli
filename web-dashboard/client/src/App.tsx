// Mission Control SPA — provider + shell always mount as one tree (HMR-safe).
import { Route, Switch } from "wouter";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import ErrorBoundary from "./components/ErrorBoundary";
import AppShell from "./components/AppShell";
import StatusBar from "./components/StatusBar";
import CommandPalette from "./components/CommandPalette";
import EventToasts from "./components/EventToasts";
import { ThemeProvider } from "./contexts/ThemeContext";
import { EngineProvider, useEngine } from "./contexts/EngineContext";
import Home from "./pages/Home";
import AgentDetail from "./pages/AgentDetail";
import TranslationBatch from "./pages/TranslationBatch";
import ModelSettings from "./pages/ModelSettings";
import SnapshotHistory from "./pages/SnapshotHistory";
import Manuscript from "./pages/Manuscript";
import Library from "./pages/Library";
import CoCreate from "./pages/CoCreate";
import NotFound from "./pages/NotFound";

function ShellRoutes() {
  return (
    <Switch>
      <Route path="/" component={Home} />
      <Route path="/agents/:role" component={AgentDetail} />
      <Route path="/translation" component={TranslationBatch} />
      <Route path="/manuscript" component={Manuscript} />
      <Route path="/library" component={Library} />
      <Route path="/cocreate" component={CoCreate} />
      <Route path="/snapshots" component={SnapshotHistory} />
      <Route path="/settings" component={ModelSettings} />
      <Route component={NotFound} />
    </Switch>
  );
}

function ShellFrame() {
  const { engine, connected, transport, workingLabel } = useEngine();
  return (
    <AppShell
      engine={engine}
      connected={connected}
      transport={transport}
      footer={
        <StatusBar
          engine={engine}
          connected={connected}
          transport={transport}
          workingLabel={workingLabel}
        />
      }
    >
      <CommandPalette />
      <EventToasts />
      <ShellRoutes />
    </AppShell>
  );
}

/** Single subtree: Provider wraps every useEngine consumer. */
function EngineApp() {
  return (
    <EngineProvider>
      <ShellFrame />
    </EngineProvider>
  );
}

function App() {
  return (
    <ErrorBoundary>
      <ThemeProvider defaultTheme="light">
        <TooltipProvider>
          <Toaster position="bottom-right" />
          <EngineApp />
        </TooltipProvider>
      </ThemeProvider>
    </ErrorBoundary>
  );
}

export default App;
