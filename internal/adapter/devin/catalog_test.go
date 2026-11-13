// 本文件验证模型目录元数据、family 建议、快照与回退。
package devin

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"devin-gateway/internal/adapter"
	"devin-gateway/internal/connect"
)

// TestDecodeClientModelConfigMetadata 验证 label、family metadata 和默认标记。
func TestDecodeClientModelConfigMetadata(t *testing.T) {
	var encoded buffer
	encoded.str(1, "SWE-2 High")
	encoded.str(22, "swe-2-high")
	encoded.uint(18, 262000)
	encoded.str(27, "description")
	encoded.message(30, func(family *buffer) {
		family.str(1, "SWE-2")
		family.message(2, func(entry *buffer) {
			entry.str(1, "Reasoning Effort")
			entry.message(2, func(value *buffer) {
				value.uint(1, 1)
				value.str(2, "High")
			})
		})
		family.boolean(3, true)
	})
	encoded.boolean(31, true)

	config, err := decodeClientModelConfig(encoded.data)
	if err != nil {
		t.Fatal(err)
	}
	if config.label != "SWE-2 High" || config.modelUID != "swe-2-high" {
		t.Fatalf("identity = %q/%q", config.label, config.modelUID)
	}
	if config.maxTokens != 262000 || config.description != "description" {
		t.Fatalf("metadata = %d/%q", config.maxTokens, config.description)
	}
	if config.family == nil || config.family.label != "SWE-2" || !config.family.defaultInFamily {
		t.Fatalf("family = %#v", config.family)
	}
	if got := config.effort(); got != "high" {
		t.Fatalf("effort = %q", got)
	}
	if !config.defaultInFamily {
		t.Fatal("defaultInFamily = false")
	}
}

// TestSuggestFamilies 验证 family 建议排除 fast 变体并保留档位。
func TestSuggestFamilies(t *testing.T) {
	models := []adapter.ModelInfo{
		{ID: "swe-2-medium", Family: "SWE-2", Effort: "medium"},
		{ID: "swe-2-high", Family: "SWE-2", Effort: "high"},
		{ID: "swe-2-medium-lightning", Family: "SWE-2", Effort: "medium", FastMode: true},
		{ID: "glm-5-3-low", Family: "GLM-5.3", Effort: "low"},
	}
	families := suggestFamilies(models)
	if got := families["swe-2"]["medium"]; got != "swe-2-medium" {
		t.Fatalf("swe-2 medium = %q", got)
	}
	if got := families["swe-2"]["high"]; got != "swe-2-high" {
		t.Fatalf("swe-2 high = %q", got)
	}
	if got := families["glm-5-3"]["low"]; got != "glm-5-3-low" {
		t.Fatalf("glm-5-3 low = %q", got)
	}
}

// TestCatalogSnapshotRoundTrip 验证快照原子写入、权限与读取。
func TestCatalogSnapshotRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "models.json")
	snapshot := catalogSnapshot{
		SchemaVersion: 1,
		GeneratedAt:   time.Now().UTC(),
		Source:        "https://example.com",
		CatalogHash:   "abc",
		Models: []adapter.ModelInfo{
			{ID: "swe-2-high", SupportsImages: true, Family: "SWE-2", Effort: "high"},
		},
		Families: map[string]map[string]string{"swe-2": {"high": "swe-2-high"}},
	}
	if err := writeCatalogSnapshot(path, snapshot); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	loaded, err := loadCatalogSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Models) != 1 || loaded.Models[0].ID != "swe-2-high" {
		t.Fatalf("loaded = %#v", loaded)
	}
	if loaded.Families["swe-2"]["high"] != "swe-2-high" {
		t.Fatalf("families = %#v", loaded.Families)
	}
}

// TestListModelsFallsBackToSnapshot 验证上游失败时读取本地快照并标记 stale。
func TestListModelsFallsBackToSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "models.json")
	snapshot := catalogSnapshot{
		SchemaVersion: 1,
		GeneratedAt:   time.Now().UTC(),
		Source:        "https://example.com",
		CatalogHash:   "abc",
		Models:        []adapter.ModelInfo{{ID: "swe-2-high", SupportsImages: true}},
		Families:      map[string]map[string]string{"swe-2": {"high": "swe-2-high"}},
	}
	if err := writeCatalogSnapshot(path, snapshot); err != nil {
		t.Fatal(err)
	}

	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "boom", http.StatusInternalServerError)
	}))
	defer upstream.Close()

	provider := &Adapter{
		config:         Config{BaseURL: upstream.URL, Token: "test", Model: "swe-2"},
		unaryClient:    connect.NewClient(upstream.URL, upstream.Client()),
		snapshotPath:   path,
		modelsCacheTTL: time.Minute,
		imageSupport:   map[string]bool{},
		logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	models, err := provider.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "swe-2-high" {
		t.Fatalf("models = %#v", models)
	}
	manifest := provider.ModelManifest()
	if !manifest.Stale {
		t.Fatal("manifest should be stale")
	}
	if manifest.Families["swe-2"]["high"] != "swe-2-high" {
		t.Fatalf("families = %#v", manifest.Families)
	}
}

// newSnapshotOnlyAdapter 构造一个上游必定失败、只能靠快照工作的 adapter。
func newSnapshotOnlyAdapter(t *testing.T, snapshot catalogSnapshot, model string) *Adapter {
	t.Helper()
	path := filepath.Join(t.TempDir(), "models.json")
	if err := writeCatalogSnapshot(path, snapshot); err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(upstream.Close)
	provider, err := New(Config{
		BaseURL: upstream.URL, Token: "test", Model: model,
		AutoTiers: true, SnapshotPath: path,
	})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

// TestResolveReasoningIgnoresPlaceholderModel 验证仅为「可发现」而补进目录的 devin.model
// 不会被当成上游真实模型：它若同时是合成基础 id，不传 effort 仍须本地拒绝。
func TestResolveReasoningIgnoresPlaceholderModel(t *testing.T) {
	provider := newSnapshotOnlyAdapter(t, catalogSnapshot{
		SchemaVersion: 1,
		GeneratedAt:   time.Now().UTC(),
		Models: []adapter.ModelInfo{
			{ID: "swe-2-high", Family: "SWE-2", Effort: "high"},
			{ID: "swe-2", Placeholder: true}, // fetchCatalog 为 devin.model 补的占位
		},
		Families: map[string]map[string]string{"swe-2": {"high": "swe-2-high"}},
	}, "swe-2")
	// 模拟目录已加载（例如客户端先调了 /v1/models）。
	if _, err := provider.ListModels(context.Background()); err != nil {
		t.Fatal(err)
	}

	if _, err := provider.resolveReasoning(context.Background(), "swe-2", ""); err == nil {
		t.Fatal("placeholder base id was treated as a real upstream model")
	}
	got, err := provider.resolveReasoning(context.Background(), "swe-2", "high")
	if err != nil || got != "swe-2-high" {
		t.Fatalf("with effort = %q, %v", got, err)
	}
}

// TestResolveReasoningLoadsCatalogOnFirstRequest 验证首请求时即使 families 已从快照恢复，
// 也会先加载目录再判断基础 id 是否是真实模型，而不是因为目录为空把真实模型拒掉。
func TestResolveReasoningLoadsCatalogOnFirstRequest(t *testing.T) {
	provider := newSnapshotOnlyAdapter(t, catalogSnapshot{
		SchemaVersion: 1,
		GeneratedAt:   time.Now().UTC(),
		Models: []adapter.ModelInfo{
			{ID: "swe-1-7", Family: "SWE-1.7"},
			{ID: "swe-1-7-medium", Family: "SWE-1.7", Effort: "medium"},
		},
		Families: map[string]map[string]string{"swe-1-7": {"medium": "swe-1-7-medium"}},
	}, "swe-1-7")

	got, err := provider.resolveReasoning(context.Background(), "swe-1-7", "")
	if err != nil || got != "swe-1-7" {
		t.Fatalf("first request for real base = %q, %v; want passthrough", got, err)
	}
}
