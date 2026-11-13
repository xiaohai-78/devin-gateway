// 本文件验证服务入口能向调用方返回服务器启动失败。
package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"devin-gateway/internal/config"
)

// TestBuiltInDefaultListensOnLoopback 锁定内置默认配置只绑回环：直接运行二进制且未设
// AUTH_API_KEY 时，局域网不应能访问。容器场景由 Dockerfile 的 SERVER_LISTEN 覆盖。
func TestBuiltInDefaultListensOnLoopback(t *testing.T) {
	// 先 Setenv 让测试结束时恢复原值，再真正清掉，避免空串被当成「已设置」。
	t.Setenv("SERVER_LISTEN", "")
	os.Unsetenv("SERVER_LISTEN")
	missing := filepath.Join(t.TempDir(), "absent.yaml")
	loaded, err := config.LoadWithFallback(missing, defaultConfigYAML)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Server.Listen != "127.0.0.1:18089" {
		t.Fatalf("built-in listen = %q, want 127.0.0.1:18089", loaded.Server.Listen)
	}
}

type fakeServer struct {
	shutdown bool
}

func (server *fakeServer) ListenAndServe() error {
	return errors.New("listen stopped")
}

func (server *fakeServer) Shutdown(context.Context) error {
	server.shutdown = true
	return nil
}

func (server *fakeServer) Close() error { return nil }

// TestListenURL verifies listen address descriptions used in the startup log.
func TestListenURL(t *testing.T) {
	cases := map[string]string{
		":18089":         ":18089 (http://localhost:18089)",
		"0.0.0.0:18089":  "0.0.0.0:18089 (http://localhost:18089)",
		"127.0.0.1:9090": "http://127.0.0.1:9090",
		"[::]:18089":     "[::]:18089 (http://localhost:18089)",
		"invalid":        "invalid",
	}
	for listen, want := range cases {
		if got := listenURL(listen); got != want {
			t.Errorf("listenURL(%q) = %q, want %q", listen, got, want)
		}
	}
}

// TestRunReturnsServeError verifies unexpected server failures are returned to main.
func TestRunReturnsServeError(t *testing.T) {
	server := &fakeServer{}
	if err := run(context.Background(), server); err == nil {
		t.Fatal("run() error = nil, want serve error")
	}
	if server.shutdown {
		t.Fatal("server was shut down after an unexpected serve failure")
	}
}
