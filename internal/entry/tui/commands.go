package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/host"
	"github.com/voocel/ainovel-cli/internal/i18n"
	"github.com/voocel/ainovel-cli/internal/translation"
)

type slashCommandSpec struct {
	Name        string
	Aliases     []string
	Group       string
	Usage       string
	Description string
	AutoExecute bool
	Hidden      bool
	NeedsIdle   bool
	Run         func(m Model, args []string) (tea.Model, tea.Cmd)
}

type slashCommand struct {
	name string
	args []string
}

func parseSlashCommand(text string) (slashCommand, bool) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return slashCommand{}, false
	}
	fields := strings.Fields(strings.TrimPrefix(text, "/"))
	if len(fields) == 0 {
		return slashCommand{}, false
	}
	return slashCommand{name: strings.ToLower(fields[0]), args: fields[1:]}, true
}

func (s slashCommandSpec) matches(name string) bool {
	if s.Name == name {
		return true
	}
	for _, alias := range s.Aliases {
		if strings.EqualFold(alias, name) {
			return true
		}
	}
	return false
}

func commandRegistryInstance() commandRegistry {
	return newCommandRegistry([]slashCommandSpec{
		{
			Name:        i18n.T("command.help.name"),
			Aliases:     []string{"help"},
			Group:       "Hệ thống",
			Usage:       i18n.T("command.help.usage"),
			Description: i18n.T("command.help.description"),
			AutoExecute: true,
			Run: func(m Model, _ []string) (tea.Model, tea.Cmd) {
				m.help = newHelpState(m.width, m.height)
				m.textarea.Blur()
				return m, nil
			},
		},
		{
			Name:        i18n.T("command.model.name"),
			Aliases:     []string{"model"},
			Group:       "Hệ thống",
			Usage:       i18n.T("command.model.usage"),
			Description: i18n.T("command.model.description"),
			AutoExecute: true,
			Run: func(m Model, args []string) (tea.Model, tea.Cmd) {
				roleHint := ""
				if len(args) > 0 {
					roleHint = args[0]
					if normalizeRoleKey(roleHint) == "" {
						m.applyEvent(host.Event{
							Time: time.Now(), Category: "ERROR", Summary: i18n.T("event.unknown_role", roleHint), Level: "error",
						})
						m.refreshEventViewport()
						return m, nil
					}
				}
				m.modelSwitch = newModelSwitchState(m.runtime, roleHint)
				m.textarea.Blur()
				return m, nil
			},
		},
		{
			Name:        i18n.T("command.config.name"),
			Aliases:     []string{"config"},
			Group:       "Hệ thống",
			Usage:       i18n.T("command.config.usage"),
			Description: i18n.T("command.config.description"),
			AutoExecute: true,
			Run: func(m Model, args []string) (tea.Model, tea.Cmd) {
				if len(args) != 0 {
					m.applyEvent(host.Event{Time: time.Now(), Category: "ERROR", Summary: i18n.T("event.usage", i18n.T("command.config.usage")), Level: "error"})
					m.refreshEventViewport()
					return m, nil
				}
				m.modelConfig = newModelConfigState(m.runtime)
				m.textarea.Blur()
				return m, nil
			},
		},
		{
			Name:        i18n.T("command.diag.name"),
			Aliases:     []string{"diag"},
			Group:       "Phân tích",
			Usage:       i18n.T("command.diag.usage"),
			Description: i18n.T("command.diag.description"),
			AutoExecute: true,
			Run: func(m Model, _ []string) (tea.Model, tea.Cmd) {
				m.reportSeq++
				m.report = newReportState(m.width, m.height, m.reportSeq, time.Now())
				m.textarea.Blur()
				return m, loadReport(m.runtime.Dir(), m.reportSeq)
			},
		},
		{
			Name:        i18n.T("command.review.name"),
			Aliases:     []string{"review"},
			Group:       "Sáng tác",
			Usage:       i18n.T("command.review.usage"),
			Description: i18n.T("command.review.description"),
			Run: func(m Model, args []string) (tea.Model, tea.Cmd) {
				if len(args) != 1 || (args[0] != "bat" && args[0] != "tat" && args[0] != "on" && args[0] != "off") {
					m.applyEvent(host.Event{Time: time.Now(), Category: "ERROR", Summary: i18n.T("event.usage", i18n.T("command.review.usage")), Level: "error"})
					m.refreshEventViewport()
					return m, nil
				}
				mode := domain.ChapterAdvanceReview
				if args[0] == "off" || args[0] == "tat" {
					mode = domain.ChapterAdvanceAuto
				}
				if err := m.runtime.SetAdvanceMode(mode); err != nil {
					m.applyEvent(host.Event{Time: time.Now(), Category: "ERROR", Summary: "Không thể đổi chế độ tiếp tục: " + err.Error(), Level: "error"})
					m.refreshEventViewport()
					return m, nil
				}
				return m, fetchSnapshot(m.runtime)
			},
		},
		{
			Name:        i18n.T("command.next.name"),
			Aliases:     []string{"next"},
			Group:       "Sáng tác",
			Usage:       i18n.T("command.next.usage"),
			Description: i18n.T("command.next.description"),
			AutoExecute: true,
			NeedsIdle:   true,
			Run: func(m Model, args []string) (tea.Model, tea.Cmd) {
				if len(args) != 0 {
					m.applyEvent(host.Event{Time: time.Now(), Category: "ERROR", Summary: i18n.T("event.usage", i18n.T("command.next.usage")), Level: "error"})
					m.refreshEventViewport()
					return m, nil
				}
				if err := m.runtime.AdvanceOneChapter(); err != nil {
					m.applyEvent(host.Event{Time: time.Now(), Category: "ERROR", Summary: "Không thể cho phép viết chương tiếp theo: " + err.Error(), Level: "error"})
					m.refreshEventViewport()
					return m, nil
				}
				return m, tea.Batch(fetchSnapshot(m.runtime), listenDone(m.runtime), m.textarea.Focus())
			},
		},
		{
			Name:        i18n.T("command.import.name"),
			Aliases:     []string{"import"},
			Group:       "Sáng tác",
			Usage:       i18n.T("command.import.usage"),
			Description: i18n.T("command.import.description"),
			NeedsIdle:   true,
			Run: func(m Model, args []string) (tea.Model, tea.Cmd) {
				m.importSeq++
				state, listenCmd, err := startImport(m.runtime, m.importSeq, args, m.width, m.height)
				if err != nil {
					m.applyEvent(host.Event{
						Time: time.Now(), Category: "ERROR", Summary: "导入启动失败：" + err.Error(), Level: "error",
					})
					m.refreshEventViewport()
					return m, nil
				}
				m.importer = state
				m.importHint = "" // 已进入导入流程，欢迎屏的恢复提示完成使命
				m.textarea.Blur()
				return m, listenCmd
			},
		},
		{
			Name:        i18n.T("command.reopen.name"),
			Aliases:     []string{"reopen"},
			Group:       "Sáng tác",
			Usage:       i18n.T("command.reopen.usage"),
			Description: i18n.T("command.reopen.description"),
			NeedsIdle:   true,
			Run: func(m Model, args []string) (tea.Model, tea.Cmd) {
				if err := m.runtime.Reopen(strings.Join(args, " ")); err != nil {
					m.applyEvent(host.Event{
						Time: time.Now(), Category: "ERROR", Summary: "重开失败：" + err.Error(), Level: "error",
					})
					m.refreshEventViewport()
					return m, nil
				}
				return m, tea.Batch(m.textarea.Focus(), resumeBook(m.runtime))
			},
		},
		{
			Name:        i18n.T("command.cocreate.name"),
			Aliases:     []string{"cocreate", "plan"},
			Group:       "Sáng tác",
			Usage:       i18n.T("command.cocreate.usage"),
			Description: i18n.T("command.cocreate.description"),
			AutoExecute: true,
			Run: func(m Model, _ []string) (tea.Model, tea.Cmd) {
				if m.mode != modeRunning {
					m.applyEvent(host.Event{
						Time: time.Now(), Category: "ERROR", Summary: "阶段共创仅在创作中可用", Level: "error",
					})
					m.refreshEventViewport()
					return m, nil
				}
				if !m.runtime.PauseForCoCreate() {
					m.applyEvent(host.Event{
						Time: time.Now(), Category: "ERROR", Summary: "无法进入阶段共创：全书已完成或已在共创中", Level: "error",
					})
					m.refreshEventViewport()
					return m, nil
				}
				m.cocreate = newStageCoCreateState()
				m.resizeTextarea()
				m.textarea.Blur()
				return m, m.sendCoCreate()
			},
		},
		{
			Name:        i18n.T("command.simulate.name"),
			Aliases:     []string{"simulate"},
			Group:       "Sáng tác",
			Usage:       i18n.T("command.simulate.usage"),
			Description: i18n.T("command.simulate.description"),
			NeedsIdle:   true,
			Run: func(m Model, args []string) (tea.Model, tea.Cmd) {
				m.simSeq++
				state, listenCmd, err := startSimulate(m.runtime, m.simSeq, args, m.width, m.height)
				if err != nil {
					m.applyEvent(host.Event{
						Time: time.Now(), Category: "ERROR", Summary: "仿写画像启动失败：" + err.Error(), Level: "error",
					})
					m.refreshEventViewport()
					return m, nil
				}
				m.simulator = state
				m.textarea.Blur()
				return m, listenCmd
			},
		},
		{
			Name:        i18n.T("command.importsim.name"),
			Aliases:     []string{"importsim"},
			Group:       "Sáng tác",
			Usage:       i18n.T("command.importsim.usage"),
			Description: i18n.T("command.importsim.description"),
			NeedsIdle:   true,
			Run: func(m Model, args []string) (tea.Model, tea.Cmd) {
				m.simSeq++
				state, listenCmd, err := startImportSimulation(m.runtime, m.simSeq, args, m.width, m.height)
				if err != nil {
					m.applyEvent(host.Event{
						Time: time.Now(), Category: "ERROR", Summary: "导入仿写画像失败：" + err.Error(), Level: "error",
					})
					m.refreshEventViewport()
					return m, nil
				}
				m.simulator = state
				m.textarea.Blur()
				return m, listenCmd
			},
		},
		{
			Name:        i18n.T("command.translate.name"),
			Aliases:     []string{"translate"},
			Group:       "Sáng tác",
			Usage:       i18n.T("command.translate.usage"),
			Description: i18n.T("command.translate.description"),
			AutoExecute: true,
			Run: func(m Model, args []string) (tea.Model, tea.Cmd) {
				if len(args) > 1 || (len(args) == 1 && args[0] != "trang-thai") {
					m.applyEvent(host.Event{Time: time.Now(), Category: "ERROR", Summary: i18n.T("event.usage", i18n.T("command.translate.usage")), Level: "error"})
					m.refreshEventViewport()
					return m, nil
				}
				if len(args) == 1 {
					status, err := m.runtime.TranslationStatus()
					if err != nil {
						m.applyEvent(host.Event{Time: time.Now(), Category: "ERROR", Summary: "Không thể đọc trạng thái dịch: " + err.Error(), Level: "error"})
						m.refreshEventViewport()
						return m, nil
					}
					completed, active, failed, stale := 0, 0, 0, 0
					for _, record := range status.Chapters {
						switch record.State {
						case translation.ChapterCompleted:
							completed++
						case translation.ChapterFailed:
							failed++
						case translation.ChapterStale:
							stale++
						default:
							active++
						}
					}
					m.applyEvent(host.Event{Time: time.Now(), Category: "TRANSLATION", Summary: i18n.T("event.translation_status", completed, active, failed, stale), Level: "info"})
					m.refreshEventViewport()
					return m, nil
				}
				if err := m.runtime.RequestTranslation(); err != nil {
					m.applyEvent(host.Event{Time: time.Now(), Category: "ERROR", Summary: "Không thể yêu cầu dịch: " + err.Error(), Level: "error"})
					m.refreshEventViewport()
					return m, nil
				}
				m.applyEvent(host.Event{Time: time.Now(), Category: "TRANSLATION", Summary: i18n.T("event.translation_requested"), Level: "info"})
				m.refreshEventViewport()
				return m, nil
			},
		},
		{
			Name:        i18n.T("command.export.name"),
			Aliases:     []string{"export"},
			Group:       "Sáng tác",
			Usage:       i18n.T("command.export.usage"),
			Description: i18n.T("command.export.description"),
			AutoExecute: true,
			Run: func(m Model, args []string) (tea.Model, tea.Cmd) {
				cmd, err := startExport(m.runtime, args)
				if err != nil {
					m.applyEvent(host.Event{
						Time: time.Now(), Category: "ERROR", Summary: "导出启动失败：" + err.Error(), Level: "error",
					})
					m.refreshEventViewport()
					return m, nil
				}
				m.applyEvent(host.Event{
					Time: time.Now(), Category: "SYSTEM", Summary: "正在导出...", Level: "info",
				})
				m.refreshEventViewport()
				return m, cmd
			},
		},
	})
}

func commandSpecs() []slashCommandSpec {
	return commandRegistryInstance().Visible()
}

func (m Model) handleSlashCommand(cmd slashCommand) (tea.Model, tea.Cmd) {
	spec, ok := commandRegistryInstance().Find(cmd.name)
	if !ok {
		m.applyEvent(host.Event{
			Time: time.Now(), Category: "ERROR", Summary: i18n.T("event.command_unknown", cmd.name), Level: "error",
		})
		m.refreshEventViewport()
		return m, nil
	}
	if spec.NeedsIdle && m.snapshot.IsRunning {
		m.applyEvent(host.Event{
			Time: time.Now(), Category: "ERROR", Summary: i18n.T("event.command_needs_idle", spec.Name), Level: "error",
		})
		m.refreshEventViewport()
		return m, nil
	}
	return spec.Run(m, cmd.args)
}
