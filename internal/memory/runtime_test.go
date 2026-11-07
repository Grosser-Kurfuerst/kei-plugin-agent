package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RandomLemon/kei-plugin-persona/internal/memory/memtest"
	"github.com/RandomLemon/kei-plugin-persona/internal/memory/store"
)

// TestRuntimeHostCleanup 用真实宿主、真实时钟与 mock 控制面验收 S3。
// PERSONA_MEMORY_HOST_BINARY 指向事先构建的 devhost；普通质量门不重复等待分钟调度。
func TestRuntimeHostCleanup(t *testing.T) {
	binary := os.Getenv("PERSONA_MEMORY_HOST_BINARY")
	if binary == "" {
		t.Skip("未指定运行验收宿主")
	}
	dsn := memtest.NewDB(t)
	db := openDB(t, dsn)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()
	cfg, err := filepath.Abs("../../cmd/devhost/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Add(time.Minute).Format("15:04")
	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "-config", cfg)
	cmd.Env = append(os.Environ(), "KEI_BOTS_QQ_MAIN_ENABLED=false", "KEI_BOTS_MOCK_MAIN_LISTEN_ADDR="+addr, "KEI_AUTH_ADMIN_USERS=mock:u1", "KEI_PLUGINS_PERSONA_GROUP_LIST=[g1]", "KEI_PLUGINS_PERSONA_RANDOM_TIMEZONE=UTC", "KEI_PLUGINS_PERSONA_MEMORY_DAILY_AT="+at, "KEI_PLUGINS_PERSONA_MEMORY_DATABASE_DSN="+dsn)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Signal(os.Interrupt)
		if err := cmd.Wait(); err != nil && ctx.Err() == nil {
			t.Errorf("宿主退出：%v", err)
		}
	}()
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(130 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("宿主定时清理未完成")
		}
		if db.Migrator().HasTable(&store.Job{}) {
			var n int64
			if err = db.WithContext(ctx).Model(&store.Job{}).Where("type='cleanup' AND status='done'").Count(&n).Error; err == nil && n == 1 {
				break
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Second):
		}
	}
	body := []byte(`{"text":"/agent memory status","user_id":"u1","channel_id":"g1"}`)
	resp, err := client.Post("http://"+addr+"/inject", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	for range 20 {
		resp, err = client.Get("http://" + addr + "/sent")
		if err != nil {
			t.Fatal(err)
		}
		data, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		var parsed any
		if err = json.Unmarshal(data, &parsed); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "任务：待执行") && strings.Contains(string(data), "数据库=就绪") {
			t.Logf("宿主 daily_at=%s：cleanup=done，status 输出任务行", at)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("宿主 status 未出现任务行，HTTP=%d", resp.StatusCode)
}
