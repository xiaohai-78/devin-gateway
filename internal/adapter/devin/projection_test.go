// 本文件验证 /v1/models 的 raw/openai/both 投影。
package devin

import (
	"testing"

	"devin-gateway/internal/adapter"
)

// TestProjectModelsModes 验证 openai 模式隐藏档位 id 并合成基础模型。
func TestProjectModelsModes(t *testing.T) {
	actual := []adapter.ModelInfo{
		{ID: "swe-2-medium", Family: "SWE-2", Effort: "medium", OwnedBy: "windsurf", SupportsImages: true, MaxTokens: 100, Created: 10},
		{ID: "swe-2-high", Family: "SWE-2", Effort: "high", OwnedBy: "windsurf", SupportsImages: true, MaxTokens: 200, Created: 11},
		{ID: "swe-2-medium-lightning", Family: "SWE-2", Effort: "medium", FastMode: true, OwnedBy: "windsurf"},
		{ID: "glm-5-2", OwnedBy: "zai"},
	}
	tiers := map[string]map[string]string{"swe-2": {"medium": "swe-2-medium", "high": "swe-2-high"}}

	visible := projectModels(actual, tiers, "openai")
	if len(visible) != 3 {
		t.Fatalf("openai visible = %d, want 3: %#v", len(visible), visible)
	}
	base := visible[0]
	if base.ID != "swe-2" || !base.RequiresReasoningEffort {
		t.Fatalf("base = %#v", base)
	}
	if len(base.ReasoningEfforts) != 2 || base.ReasoningEfforts[0] != "high" || base.ReasoningEfforts[1] != "medium" {
		t.Fatalf("efforts = %#v", base.ReasoningEfforts)
	}
	if !base.SupportsImages || base.MaxTokens != 200 || base.OwnedBy != "windsurf" || base.Label != "SWE-2" {
		t.Fatalf("base metadata = %#v", base)
	}
	if visible[1].ID != "swe-2-medium-lightning" || visible[2].ID != "glm-5-2" {
		t.Fatalf("visible = %#v", visible)
	}

	both := projectModels(actual, tiers, "both")
	if len(both) != 5 {
		t.Fatalf("both visible = %d, want 5", len(both))
	}
	raw := projectModels(actual, tiers, "raw")
	if len(raw) != 4 || raw[0].ID != "swe-2-medium" {
		t.Fatalf("raw = %#v", raw)
	}
}

// TestProjectModelsRealBase 验证基础 id 本身是真实模型时：保留真实条目与档位条目，
// 挂上可选 reasoning_efforts，但不标 requires_reasoning_effort（不传 effort 也能调用）。
func TestProjectModelsRealBase(t *testing.T) {
	actual := []adapter.ModelInfo{
		{ID: "swe-1-7", Family: "SWE-1.7"},
		{ID: "swe-1-7-medium", Family: "SWE-1.7", Effort: "medium"},
		{ID: "kimi-k2-7"},
	}
	tiers := map[string]map[string]string{
		"swe-1-7": {"medium": "swe-1-7-medium", "max": "swe-1-7-max"}, // max 不在目录里
	}

	visible := projectModels(actual, tiers, "openai")
	byID := make(map[string]adapter.ModelInfo, len(visible))
	for _, model := range visible {
		byID[model.ID] = model
	}
	base, ok := byID["swe-1-7"]
	if !ok {
		t.Fatalf("real base missing: %#v", visible)
	}
	if base.RequiresReasoningEffort {
		t.Fatalf("real base must not require effort: %#v", base)
	}
	if len(base.ReasoningEfforts) != 1 || base.ReasoningEfforts[0] != "medium" {
		t.Fatalf("real base efforts = %#v, want [medium]", base.ReasoningEfforts)
	}
	if _, ok := byID["swe-1-7-medium"]; !ok {
		t.Fatalf("tier entry of a real base must stay visible: %#v", visible)
	}
	if kimi := byID["kimi-k2-7"]; kimi.RequiresReasoningEffort || len(kimi.ReasoningEfforts) != 0 {
		t.Fatalf("unrelated model changed: %#v", kimi)
	}
	// 投影不能改到调用方的切片。
	if len(actual[0].ReasoningEfforts) != 0 {
		t.Fatalf("projectModels mutated input: %#v", actual[0])
	}
}

// TestProjectModelsPlaceholderBase 验证为 devin.model 补的占位条目和基础 id 同名时，
// 仍按合成基础模型展示（requires_reasoning_effort=true），且只出现一次。
func TestProjectModelsPlaceholderBase(t *testing.T) {
	actual := []adapter.ModelInfo{
		{ID: "swe-2-high", Family: "SWE-2", Effort: "high"},
		{ID: "swe-2", Placeholder: true},
	}
	tiers := map[string]map[string]string{"swe-2": {"high": "swe-2-high"}}

	visible := projectModels(actual, tiers, "openai")
	count := 0
	for _, model := range visible {
		if model.ID != "swe-2" {
			continue
		}
		count++
		if !model.RequiresReasoningEffort {
			t.Fatalf("placeholder base must be shown as synthetic: %#v", model)
		}
	}
	if count != 1 {
		t.Fatalf("swe-2 appears %d times: %#v", count, visible)
	}
}
