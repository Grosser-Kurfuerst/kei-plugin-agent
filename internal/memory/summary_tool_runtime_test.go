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

// TestRuntimeHostSummaryTool 用真实宿主和已配置聊天模型验收显式查询，只发送固定合成摘要。
func TestRuntimeHostSummaryTool(t *testing.T) {
	binary := os.Getenv("PERSONA_MEMORY_SUMMARY_HOST_BINARY")
	if binary == "" {
		t.Skip("未指定 S6 运行验收宿主")
	}
	dsn := memtest.NewDB(t)
	db := openDB(t, dsn)
	if err := store.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	today := store.LocalDate(time.Now(), time.UTC, 4*time.Hour)
	// 上周三，在自动注入的三日窗口之外，必须经工具查询。
	weekday := (int(today.Weekday()) + 6) % 7
	date := today.AddDate(0, 0, -weekday-7+2).Format("2006-01-02")
	day, err := time.Parse("2006-01-02", date)
	if err != nil {
		t.Fatal(err)
	}
	row := store.Summary{ConvKey: "group:mock:g1", LocalDate: day, Kind: "daily", Version: 1, Topics: []store.Topic{{ConvKey: "group:mock:g1", LocalDate: day, TopicKey: "runtime:summary", Title: "聚餐安排", Summary: "上周三讨论聚餐，决定周六18点在松果餐厅聚餐，人均预算100元。", Importance: 0.9}}}
	if err = db.WithContext(t.Context()).Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()
	configPath, err := filepath.Abs("../../cmd/devhost/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "-config", configPath)
	cmd.Env = append(os.Environ(), "KEI_BOTS_QQ_MAIN_ENABLED=false", "KEI_BOTS_MOCK_MAIN_LISTEN_ADDR="+addr, "KEI_AUTH_ADMIN_USERS=mock:u1", "KEI_LOG_LEVEL=debug", "KEI_PLUGINS_PERSONA_DEBUG_PROMPTS=true", "KEI_PLUGINS_PERSONA_GROUP_LIST=[g1]", "KEI_PLUGINS_PERSONA_TRIGGER_KEYWORDS=[bot]", "KEI_PLUGINS_PERSONA_RANDOM_ENABLED=false", "KEI_PLUGINS_PERSONA_RANDOM_TIMEZONE=UTC", "KEI_PLUGINS_PERSONA_MEMORY_DAILY_AT=04:00", "KEI_PLUGINS_PERSONA_MEMORY_DATABASE_DSN="+dsn, "KEI_PLUGINS_PERSONA_MEMORY_TOOLS_ENABLED=true", "KEI_PLUGINS_PERSONA_MCP_URL=", "KEI_PLUGINS_PERSONA_BATCH_WINDOW=20ms", "KEI_PLUGINS_PERSONA_BATCH_MAX_WINDOW=30ms", "KEI_PLUGINS_PERSONA_MENTION_MIN_INTERVAL=0s", "KEI_PLUGINS_PERSONA_REPLY_DEDUPE=false", "KEI_PLUGINS_PERSONA_LLM_MAX_TOKENS=2048", "KEI_PLUGINS_PERSONA_PERSONAS={\"default\":{\"prompt\":\"用户询问过去聊天内容时，先用 memory__summary 按日期查询，再据工具结果简短回答。\"}}")
	logs := &runtimeLogBuffer{}
	cmd.Stdout = logs
	cmd.Stderr = logs
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
	inject := func(text string) {
		data, _ := json.Marshal(map[string]string{"text": text, "user_id": "u1", "channel_id": "g1"})
		res, err := client.Post("http://"+addr+"/inject", "application/json", bytes.NewReader(data))
		if err == nil {
			_ = res.Body.Close()
		}
	}
	sent := func() string {
		res, err := client.Get("http://" + addr + "/sent")
		if err != nil {
			return ""
		}
		defer func() { _ = res.Body.Close() }()
		data, _ := io.ReadAll(res.Body)
		return string(data)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		inject("/agent memory status")
		if strings.Contains(sent(), "数据库=就绪") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("宿主数据库未就绪")
		}
		time.Sleep(100 * time.Millisecond)
	}
	inject("bot 上周三（" + date + "）聊了什么？请按该日期查聊天摘要。")
	deadline = time.Now().Add(60 * time.Second)
	for !strings.Contains(sent(), "松果餐厅") || !strings.Contains(logs.String(), "tool=memory__summary") || !strings.Contains(logs.String(), "status=ok") {
		if time.Now().After(deadline) {
			t.Fatal("未观察到成功的 memory__summary 调用及包含合成摘要的回复")
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Log("真实宿主与聊天模型：询问上周三，成功调用 memory__summary，回复包含工具返回的松果餐厅安排；仅使用固定合成数据")
}
