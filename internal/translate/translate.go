package translate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/voocel/agentcore"
	"github.com/voocel/ainovel-cli/internal/bootstrap"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/store"
)

// Run translates the completed chapters.
func Run(ctx context.Context, s *store.Store, models *bootstrap.ModelSet, translatorPrompt string, onEvent func(summary string)) error {
	runMeta, err := s.RunMeta.Load()
	if err != nil {
		return fmt.Errorf("read run meta: %w", err)
	}
	progress, err := s.Progress.Load()
	if err != nil {
		return fmt.Errorf("read progress: %w", err)
	}

	if runMeta == nil || progress == nil || progress.Phase != domain.PhaseComplete {
		return fmt.Errorf("translation can only run when phase is Complete")
	}

	model := models.ForRole("translator")

	// Try load characters
	chars, err := s.Characters.Load()
	if err != nil {
		chars = []domain.Character{}
	}
	
	// Create vi output dir
	outDir := filepath.Join(s.Dir(), "vi", "chapters")
	
	systemPrompt := translatorPrompt
	if len(chars) > 0 {
		systemPrompt += "\n\nNhân vật:\n"
		for _, c := range chars {
			systemPrompt += fmt.Sprintf("- %s: %s\n", c.Name, c.Description)
		}
	}
	
	agent := agentcore.NewAgent(agentcore.WithModel(model), agentcore.WithSystemPrompt(systemPrompt))

	chapters := progress.CompletedChapters

	for _, ch := range chapters {
		destFile := filepath.Join(outDir, fmt.Sprintf("%02d.md", ch))
		if exists(destFile) {
			if onEvent != nil {
				onEvent(fmt.Sprintf("Skip %02d", ch))
			}
			continue
		}

		if onEvent != nil {
			onEvent(fmt.Sprintf("Translating %02d", ch))
		}

		content, err := s.Drafts.LoadChapterText(ch)
		if err != nil || content == "" {
			continue
		}

		if err := agent.PromptMessages(ctx, agentcore.UserMsg(content)); err != nil {
			return fmt.Errorf("translate %02d: %w", ch, err)
		}

		msgs := agent.ExportMessages()
		if len(msgs) == 0 {
			return fmt.Errorf("translate %02d: empty response", ch)
		}
		
		lastMsg := msgs[len(msgs)-1]
		if lastMsg.Role != agentcore.RoleAssistant || len(lastMsg.Content) == 0 {
			return fmt.Errorf("translate %02d: unexpected response", ch)
		}
		
		respContent := lastMsg.Content[0].Text

		if err := os.MkdirAll(outDir, 0755); err != nil {
			return fmt.Errorf("ensure dir %s: %w", outDir, err)
		}

		err = os.WriteFile(destFile, []byte(respContent), 0644)
		if err != nil {
			return fmt.Errorf("write %02d: %w", ch, err)
		}
	}

	if onEvent != nil {
		onEvent("Translation complete")
	}

	return nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

