// 本文件负责加载配置、组装服务依赖并启动 HTTP 服务器。
package main

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"devin-gateway/internal/app"
	"devin-gateway/internal/config"
	"devin-gateway/internal/debuglog"
)

// defaultConfigYAML 是内置默认配置，--config 指向的文件不存在时使用它。
//
//go:embed default.yaml
var defaultConfigYAML []byte

func main() {
	configPath := flag.String("config", "config.yaml", "YAML 配置文件路径（不存在时使用内置默认值）")
	check := flag.Bool("check", false, "校验配置与上游连通性后退出")
	syncModels := flag.Bool("sync-models", false, "拉取上游模型目录并写入快照后退出")
	showVersion := flag.Bool("version", false, "打印版本后退出")
	flag.Parse()

	if *showVersion {
		fmt.Println("devin-gateway " + versionString())
		return
	}

	absoluteConfigPath, err := filepath.Abs(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	if _, statErr := os.Stat(*configPath); errors.Is(statErr, fs.ErrNotExist) {
		log.Printf("config %q not found; using built-in defaults (override with environment variables)", *configPath)
	}
	serviceConfig, err := config.LoadWithFallback(absoluteConfigPath, defaultConfigYAML)
	if err != nil {
		log.Fatal(err)
	}
	if strings.HasPrefix(strings.ToLower(serviceConfig.Devin.BaseURL), "http://") {
		log.Printf("warning: devin.base_url uses http; the session token will be sent unencrypted")
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if *check {
		if err := runCheck(&serviceConfig, absoluteConfigPath, os.Stdout, logger); err != nil {
			log.Fatal(err)
		}
		return
	}
	if *syncModels {
		if err := runSyncModels(&serviceConfig, absoluteConfigPath, os.Stdout, logger); err != nil {
			log.Fatal(err)
		}
		return
	}

	snapshotPath := resolveCatalogSnapshotPath(absoluteConfigPath, serviceConfig.Catalog.Snapshot)
	providerAdapter, err := newProviderAdapter(serviceConfig, snapshotPath, logger)
	if err != nil {
		log.Fatal(err)
	}
	var debugManager *debuglog.Manager
	if serviceConfig.Debug.Enabled {
		debugManager = debuglog.NewManager(filepath.Join(filepath.Dir(absoluteConfigPath), "logs"))
	}
	application := app.New(providerAdapter, serviceConfig.Server, debugManager)
	application.SetAPIKey(serviceConfig.Auth.APIKey)
	application.SetLogger(logger)
	server := application.HTTPServer()
	logger.Info("HTTP server listening", "addr", listenURL(server.Addr))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, server); err != nil {
		log.Fatal(err)
	}
}

// listenURL 生成启动日志中的监听描述：配置为通配地址时同时给出 localhost 可访问地址。
func listenURL(listen string) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return listen
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		return listen + " (http://localhost:" + port + ")"
	}
	return "http://" + host + ":" + port
}

func run(ctx context.Context, server interface {
	ListenAndServe() error
	Shutdown(context.Context) error
	Close() error
}) error {
	result := make(chan error, 1)
	go func() {
		result <- server.ListenAndServe()
	}()

	select {
	case err := <-result:
		if errors.Is(err, http.ErrServerClosed) || errors.Is(err, context.Canceled) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		// 优雅关闭超时（可能有活跃 SSE 流），强制关闭不再报错。
		server.Close()
	}
	return nil
}
