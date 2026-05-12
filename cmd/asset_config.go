package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/NotNull92/hera-agent/internal/assetconfig"
	"github.com/NotNull92/hera-agent/internal/tui"
	"github.com/charmbracelet/bubbletea"
)

func assetConfigCmd(args []string) error {
	// Load enabled assets into env for AI agent consumption
	loadEnabledAssetsEnv()

	// Check for --json flag early
	for _, arg := range args {
		if arg == "--json" {
			data, err := jsonOutputForAI()
			if err != nil {
				return fmt.Errorf("error: %v", err)
			}
			fmt.Println(string(data))
			return nil
		}
	}

	if len(args) == 0 {
		// No subcommand — launch interactive TUI
		return runAssetConfigTUI()
	}

	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "list", "ls":
		return assetConfigList()
	case "enable":
		if len(subArgs) == 0 {
			return fmt.Errorf("usage: asset-config enable <id>")
		}
		return assetConfigToggle(subArgs[0], true)
	case "disable":
		if len(subArgs) == 0 {
			return fmt.Errorf("usage: asset-config disable <id>")
		}
		return assetConfigToggle(subArgs[0], false)
	case "toggle":
		if len(subArgs) == 0 {
			return fmt.Errorf("usage: asset-config toggle <id>")
		}
		return assetConfigToggleAction(subArgs[0])
	case "detect":
		return assetConfigDetect()
	case "get":
		if len(subArgs) == 0 {
			return fmt.Errorf("usage: asset-config get <id>")
		}
		return assetConfigGet(subArgs[0])
	case "path":
		fmt.Println(assetconfig.ConfigFilePath())
		return nil
	case "--help", "-h":
		printAssetConfigHelp()
		return nil
	default:
		return fmt.Errorf("unknown subcommand: %s\n\nUse \"asset-config --help\" for available commands", sub)
	}
}

func runAssetConfigTUI() error {
	p := tea.NewProgram(tui.NewAssetConfigModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func assetConfigList() error {
	cfg, err := assetconfig.Load()
	if err != nil {
		return err
	}

	fmt.Printf("Asset Config v%s — %s\n\n", cfg.Version, assetconfig.ConfigFilePath())

	// Group by category
	categoryNames := map[string]string{
		"inspector":     "인스펙터",
		"validation":    "검증",
		"serialization": "직렬화",
		"animation":     "애니메이션/연출",
	}

	categorized := make(map[string][]assetconfig.AssetEntry)
	for _, a := range cfg.Assets {
		cat := a.Category
		categorized[cat] = append(categorized[cat], a)
	}

	catOrder := []string{"inspector", "validation", "serialization", "animation"}
	for _, cat := range catOrder {
		items, ok := categorized[cat]
		if !ok {
			continue
		}
		fmt.Printf("  %s\n", categoryNames[cat])
		for _, a := range items {
			status := "OFF"
			if a.Enabled {
				status = "ON "
			}
			installed := "  "
			if a.Installed {
				installed = "✓"
			}
			fmt.Printf("    [%s] %s  %s %s\n", status, a.ID, installed, a.Name)
		}
		fmt.Println()
	}

	return nil
}

func assetConfigToggle(id string, enabled bool) error {
	cfg, err := assetconfig.SetAssetEnabled(id, enabled)
	if err != nil {
		return err
	}
	if cfg == nil {
		return fmt.Errorf("asset not found: %s", id)
	}

	state := "disabled"
	if enabled {
		state = "enabled"
	}
	fmt.Printf("✓ %s %s\n", id, state)
	return nil
}

func assetConfigToggleAction(id string) error {
	cfg, err := assetconfig.ToggleAsset(id)
	if err != nil {
		return err
	}
	if cfg == nil {
		return fmt.Errorf("asset not found: %s", id)
	}

	for _, a := range cfg.Assets {
		if a.ID == id {
			state := "disabled"
			if a.Enabled {
				state = "enabled"
			}
			fmt.Printf("✓ %s %s\n", id, state)
			return nil
		}
	}
	return nil
}

func assetConfigDetect() error {
	// This command shows instructions for asset detection.
	// The actual detection requires Unity running with the Connector package.
	fmt.Println("에셋 감지를 실행하려면 Unity가 실행 중이어야 합니다.")
	fmt.Println("Unity 실행 후 아래 명령으로 감지:")
	fmt.Println("  hera-agent asset-config detect")
	fmt.Println()
	fmt.Printf("Config path: %s\n", assetconfig.ConfigFilePath())
	return nil
}

func assetConfigGet(id string) error {
	enabled, err := assetconfig.IsEnabled(id)
	if err != nil {
		return err
	}

	fmt.Printf("%s: %v\n", id, enabled)
	return nil
}

func printAssetConfigHelp() {
	fmt.Print(`Usage: hera-agent asset-config [subcommand]

Interactive TUI:
  asset-config                  대화형 체크박스 UI 실행 (Space로 토글)

Subcommands:
  list, ls                      전체 에셋 목록 + 상태 출력
  enable <id>                   에셋 활성화
  disable <id>                  에셋 비활성화
  toggle <id>                   에셋 토글 (ON/OFF 반전)
  detect                        설치된 에셋 자동 감지 (Unity 필요)
  get <id>                      특정 에셋 상태 확인
  path                          설정 파일 경로 출력

Available Assets:
  odin_inspector                Odin Inspector
  odin_validator                Odin Validator
  odin_serializer               Odin Serializer
  dotween                       DOTween
  dotween_pro                   DOTween Pro

Examples:
  hera-agent asset-config
  hera-agent asset-config enable dotween
  hera-agent asset-config list
  hera-agent asset-config toggle odin_inspector

TUI Controls:
  ↑/k       위로 이동
  ↓/j       아래로 이동
  Space/Enter 토글 (ON/OFF)
  q/Esc     나가기 (변경사항 자동 저장)
`)
}

// jsonOutputForAI returns the enabled assets as JSON for AI agent consumption.
// This is used when the AI needs to know which assets to prioritize.
func jsonOutputForAI() ([]byte, error) {
	enabled, err := assetconfig.GetEnabledAssets()
	if err != nil {
		return nil, err
	}

	type aiAsset struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Category string `json:"category"`
	}

	var assets []aiAsset
	for _, a := range enabled {
		assets = append(assets, aiAsset{
			ID:       a.ID,
			Name:     a.Name,
			Category: a.Category,
		})
	}

	return json.MarshalIndent(map[string]interface{}{
		"enabled_assets": assets,
		"total":          len(assets),
	}, "", "  ")
}

// loadEnabledAssetsEnv loads enabled asset IDs into UNITY_AGENT_ENABLED_ASSETS env var.
// Only called when needed, not via init().
func loadEnabledAssetsEnv() {
	cfg, err := assetconfig.Load()
	if err != nil {
		return
	}
	var enabled []string
	for _, a := range cfg.Assets {
		if a.Enabled {
			enabled = append(enabled, a.ID)
		}
	}
	if len(enabled) > 0 {
		_ = os.Setenv("UNITY_AGENT_ENABLED_ASSETS", strings.Join(enabled, ","))
	}
}
