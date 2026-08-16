package webapi

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/host"
	"github.com/voocel/ainovel-cli/internal/translation"
)

var dashboardUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	CheckOrigin:     func(*http.Request) bool { return true },
}

type socketMessage struct {
	Type         string                    `json:"type"`
	State        *stateResponse            `json:"state,omitempty"`
	Event        *eventResponse            `json:"event,omitempty"`
	Events       []eventResponse           `json:"events,omitempty"`
	Translation  *translation.Status       `json:"translation,omitempty"`
	LiveProgress *translation.LiveProgress `json:"live_progress,omitempty"`
	Error        string                    `json:"error,omitempty"`
	Reconnect    bool                      `json:"reconnect,omitempty"`
}

func (s *Server) websocket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	conn, err := dashboardUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	updates, unsubscribe := s.runtime.SubscribeEvents()
	defer unsubscribe()
	progressUpdates, unsubscribeProgress := s.subscribeLiveProgress()
	defer unsubscribeProgress()
	out := make(chan socketMessage, 64)
	writerDone := make(chan struct{})
	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	go func() {
		defer close(writerDone)
		ping := time.NewTicker(25 * time.Second)
		defer ping.Stop()
		for {
			select {
			case message, ok := <-out:
				if !ok {
					return
				}
				if err := conn.WriteJSON(message); err != nil {
					_ = conn.Close()
					return
				}
			case <-ping.C:
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					_ = conn.Close()
					return
				}
			}
		}
	}()

	queue := func(message socketMessage) bool {
		select {
		case out <- message:
			return true
		case <-writerDone:
			return false
		}
	}

	state := s.currentState()
	items, replayErr := s.runtime.ReplayQueue(0)
	if replayErr != nil {
		_ = queue(socketMessage{Type: "error", Error: replayErr.Error(), Reconnect: true})
	} else {
		if len(items) > 100 {
			items = items[len(items)-100:]
		}
		replay := make([]eventResponse, 0, len(items))
		for _, item := range items {
			if item.Kind == domain.RuntimeQueueUIEvent {
				replay = append(replay, runtimeEvent(item))
			}
		}
		if !queue(socketMessage{Type: "snapshot", State: &state, Events: replay}) {
			return
		}
		for _, progress := range s.liveProgressSnapshot() {
			if !queue(socketMessage{Type: "translation_progress", LiveProgress: &progress}) {
				return
			}
		}
	}

	for {
		select {
		case event, ok := <-updates:
			if !ok {
				close(out)
				<-writerDone
				return
			}
			converted := liveEvent(event)
			nextState := s.currentState()
			if !queue(socketMessage{Type: "update", State: &nextState, Event: &converted}) {
				return
			}
			state = nextState
		case progress := <-progressUpdates:
			if !queue(socketMessage{Type: "translation_progress", LiveProgress: &progress}) {
				return
			}
		case <-clientDone:
			close(out)
			<-writerDone
			return
		case <-s.runtime.Done():
			close(out)
			<-writerDone
			return
		}
	}
}

func (s *Server) currentState() stateResponse {
	snap := s.runtime.Snapshot()
	agents := make([]agentResponse, 0, len(snap.Agents))
	for _, agent := range snap.Agents {
		agents = append(agents, toAgent(agent))
	}
	out := toSnapshot(snap)
	out.BookDir = s.runtime.ActiveBookDir()
	return stateResponse{Online: true, Snapshot: out, Agents: agents, Translation: s.translationSnapshot(), TranslationPaused: s.runtime.IsTranslationPaused(), UpdatedAt: time.Now()}
}

func runtimeEvent(item domain.RuntimeQueueItem) eventResponse {
	return eventResponse{
		Seq: item.Seq, Time: item.Time, TaskID: item.TaskID, Agent: canonicalAgent(item.Agent),
		Category: item.Category, Kind: payloadString(item.Payload, "kind"), Summary: item.Summary,
		Priority: string(item.Priority), Level: payloadString(item.Payload, "level"),
		Detail: payloadString(item.Payload, "detail"), Failed: payloadBool(item.Payload, "failed"),
		FinishedAt: payloadTime(item.Payload, "finished_at"), RetryAt: payloadTime(item.Payload, "retry_at"),
	}
}

func liveEvent(event host.Event) eventResponse {
	priority := "background"
	if event.Level == "error" || event.Category == "ERROR" {
		priority = "control"
	}
	// Live host.Event has no runtime-queue seq; TaskID carries the call ID so the
	// dashboard can key start/finish pairs without inventing colliding seq=0 rows.
	return eventResponse{
		Seq: 0, Time: event.Time, TaskID: event.ID, Agent: canonicalAgent(event.Agent),
		Category: event.Category, Kind: event.Kind, Summary: event.Summary, Priority: priority,
		Level: event.Level, Detail: event.Detail, Failed: event.Failed,
		FinishedAt: event.FinishedAt, RetryAt: event.RetryAt,
	}
}
