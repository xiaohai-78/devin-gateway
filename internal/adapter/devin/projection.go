// 本文件实现 /v1/models 的展示投影和自动档位解析。
package devin

import (
	"context"
	"sort"
	"strings"

	"devin-gateway/internal/adapter"
)

// effectiveTiers 返回显式配置 + 自动 family 建议的合并结果，显式配置优先。
func (a *Adapter) effectiveTiers() map[string]map[string]string {
	tiers := cloneTiers(a.config.ReasoningTiers)
	if !a.config.AutoTiers {
		return tiers
	}
	a.modelsMu.RLock()
	families := cloneFamilies(a.catalogFamilies)
	a.modelsMu.RUnlock()
	return mergeTiers(tiers, families)
}

// resolveReasoning 在需要时先加载目录，再把「基础模型 + 档位」解析成实际 uid。
func (a *Adapter) resolveReasoning(ctx context.Context, model, effort string) (string, error) {
	tiers := a.effectiveTiers()
	// 两种情况要先拉目录：自动档位还没有 families；或者请求的 id 命中档位表、
	// 又没传 effort——此时要靠目录判断它是不是上游真实模型。families 可能在启动时
	// 已从快照恢复，但目录本身（a.models）只在首次 ListModels 时加载。
	_, mapped := tiers[strings.TrimSpace(model)]
	needFamilies := a.config.AutoTiers && a.familyCount() == 0
	needCatalog := mapped && strings.TrimSpace(effort) == "" && !a.catalogLoaded()
	if needFamilies || needCatalog {
		// 失败时继续用已有信息；上游错误会在真正调用时体现。
		_, _ = a.ListModels(ctx)
		tiers = a.effectiveTiers()
	}
	return resolveReasoningModel(model, effort, tiers, a.hasActualModel)
}

// catalogLoaded 报告目录是否已加载（来自上游或快照）。
func (a *Adapter) catalogLoaded() bool {
	a.modelsMu.RLock()
	defer a.modelsMu.RUnlock()
	return a.models != nil
}

// hasActualModel 报告 id 是否是上游目录里真实存在的 uid。
// 合成的基础模型不在 a.models 里；为 devin.model 补的占位条目（Placeholder）也不算。
func (a *Adapter) hasActualModel(id string) bool {
	a.modelsMu.RLock()
	defer a.modelsMu.RUnlock()
	for _, model := range a.models {
		if model.ID == id {
			return !model.Placeholder
		}
	}
	return false
}

func (a *Adapter) familyCount() int {
	a.modelsMu.RLock()
	defer a.modelsMu.RUnlock()
	return len(a.catalogFamilies)
}

// visibleModels 返回当前配置下对外的模型投影。
func (a *Adapter) visibleModels(models []adapter.ModelInfo) []adapter.ModelInfo {
	return projectModels(models, a.effectiveTiers(), a.config.ModelMode)
}

// projectModels 按 mode 把实际目录投影成对外可见模型。
// raw: 原样；openai: 基础模型 + 非档位模型；both: 两者都保留。
func projectModels(actual []adapter.ModelInfo, tiers map[string]map[string]string, mode string) []adapter.ModelInfo {
	if mode == "" || mode == "raw" || len(tiers) == 0 {
		return cloneModelInfo(actual)
	}
	byID := make(map[string]adapter.ModelInfo, len(actual))
	for _, model := range actual {
		byID[model.ID] = model
	}

	bases := make([]adapter.ModelInfo, 0, len(tiers))
	hidden := make(map[string]struct{})
	// realBaseEfforts 记录「基础 id 本身是真实模型」时可选的档位：
	// 不传 effort 也能直接调用，传了就改写到对应档位。
	realBaseEfforts := make(map[string][]string)
	for base, efforts := range tiers {
		if len(efforts) == 0 {
			continue
		}
		if existing, exists := byID[base]; exists && !existing.Placeholder {
			// 基础 id 本身是真实模型，不再合成，也不藏真实条目；
			// 只把可选档位挂到它上面，requires_reasoning_effort 保持 false。
			available := make(map[string]string, len(efforts))
			for effort, target := range efforts {
				if _, ok := byID[target]; ok {
					available[effort] = target
				}
			}
			if len(available) > 0 {
				realBaseEfforts[base] = sortedTiers(available)
			}
			continue
		}
		targets := make([]adapter.ModelInfo, 0, len(efforts))
		available := make(map[string]string, len(efforts))
		for effort, target := range efforts {
			if model, ok := byID[target]; ok {
				targets = append(targets, model)
				available[effort] = target
			}
		}
		if len(targets) == 0 {
			continue
		}
		if mode == "openai" {
			for _, target := range targets {
				hidden[target.ID] = struct{}{}
			}
		}
		bases = append(bases, baseModel(base, targets, available))
	}
	sort.Slice(bases, func(left, right int) bool { return bases[left].ID < bases[right].ID })

	synthesized := make(map[string]struct{}, len(bases))
	for _, base := range bases {
		synthesized[base.ID] = struct{}{}
	}
	visible := make([]adapter.ModelInfo, 0, len(bases)+len(actual))
	visible = append(visible, bases...)
	for _, model := range actual {
		if _, ok := hidden[model.ID]; ok {
			continue
		}
		if _, ok := synthesized[model.ID]; ok && model.Placeholder {
			// 占位条目和合成的基础模型同名：只保留后者（带 requires_reasoning_effort）。
			continue
		}
		if efforts, ok := realBaseEfforts[model.ID]; ok && len(model.ReasoningEfforts) == 0 {
			model.ReasoningEfforts = efforts
		}
		visible = append(visible, model)
	}
	return visible
}

// baseModel 根据映射目标合成一个虚拟基础模型。
func baseModel(id string, targets []adapter.ModelInfo, efforts map[string]string) adapter.ModelInfo {
	base := adapter.ModelInfo{
		ID:                      id,
		RequiresReasoningEffort: true,
		ReasoningEfforts:        sortedTiers(efforts),
	}
	for index, target := range targets {
		if index == 0 || (target.Created > 0 && (base.Created == 0 || target.Created < base.Created)) {
			base.Created = target.Created
		}
		if target.SupportsImages {
			base.SupportsImages = true
		}
		if target.MaxTokens > base.MaxTokens {
			base.MaxTokens = target.MaxTokens
		}
		if base.Label == "" {
			base.Label = target.Family
		}
		if base.OwnedBy == "" {
			base.OwnedBy = target.OwnedBy
		} else if base.OwnedBy != target.OwnedBy {
			base.OwnedBy = "devin"
		}
	}
	if base.Label == "" {
		base.Label = id
	}
	if base.OwnedBy == "" {
		base.OwnedBy = "devin"
	}
	return base
}

// sortedTiers 返回排序后的档位键。
func sortedTiers(tiers map[string]string) []string {
	values := make([]string, 0, len(tiers))
	for effort := range tiers {
		values = append(values, effort)
	}
	sort.Strings(values)
	return values
}

// cloneTiers 复制档位映射。
func cloneTiers(tiers map[string]map[string]string) map[string]map[string]string {
	return cloneFamilies(tiers)
}

// mergeTiers 合并自动建议与显式配置；显式配置优先。
func mergeTiers(explicit, automatic map[string]map[string]string) map[string]map[string]string {
	merged := cloneFamilies(explicit)
	if merged == nil {
		merged = make(map[string]map[string]string)
	}
	for base, efforts := range automatic {
		if _, exists := merged[base]; exists {
			continue
		}
		merged[base] = efforts
	}
	return merged
}
