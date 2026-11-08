package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/RandomLemon/kei-plugin-persona/internal/memory/memtest"
	"github.com/RandomLemon/kei-plugin-persona/internal/memory/store"
)

type runtimeLogBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *runtimeLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}
func (b *runtimeLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

// runtimeDBProxy 只切断本次宿主的数据库链路，不停止共享 PostgreSQL。
func runtimeDBProxy(t *testing.T, destination string) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	var mu sync.Mutex
	var connections []net.Conn
	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			incoming, err := listener.Accept()
			if err != nil {
				return
			}
			wg.Go(func() {
				defer func() { _ = incoming.Close() }()
				dialCtx, dialCancel := context.WithTimeout(ctx, 5*time.Second)
				upstream, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", destination)
				dialCancel()
				if err != nil {
					return
				}
				defer func() { _ = upstream.Close() }()
				mu.Lock()
				if ctx.Err() != nil {
					mu.Unlock()
					return
				}
				connections = append(connections, incoming, upstream)
				mu.Unlock()
				copied := make(chan struct{})
				go func() {
					defer close(copied)
					_, _ = io.Copy(upstream, incoming)
					_ = upstream.Close()
					_ = incoming.Close()
				}()
				_, _ = io.Copy(incoming, upstream)
				_ = upstream.Close()
				_ = incoming.Close()
				<-copied
			})
		}
	})
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			_ = listener.Close()
			mu.Lock()
			for _, connection := range connections {
				_ = connection.Close()
			}
			mu.Unlock()
			wg.Wait()
		})
	}
	t.Cleanup(stop)
	return listener.Addr().String(), stop
}

// TestRuntimeHostContext 验收真实宿主的摘要注入及数据库故障降级。
// 只向聊天模型发送下面固定的合成聚餐数据，不读取或发送真实会话。
func TestRuntimeHostContext(t *testing.T) {
	binary := os.Getenv("PERSONA_MEMORY_CONTEXT_HOST_BINARY")
	if binary == "" {
		t.Skip("未指定 S5 运行验收宿主")
	}
	dsn := memtest.NewDB(t)
	db := openDB(t, dsn)
	if err := store.Migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	yesterday := store.LocalDate(time.Now(), time.UTC).AddDate(0, 0, -1)
	seed := store.Summary{ConvKey: "group:mock:g1", LocalDate: yesterday, Kind: "daily", Overview: "周末聚餐安排", Version: 1, Topics: []store.Topic{{ConvKey: "group:mock:g1", LocalDate: yesterday, TopicKey: "runtime:dinner", Title: "周末聚餐", Summary: "周六18点在城东餐厅聚餐，预算人均100元。", Importance: 0.9}}}
	if err := db.WithContext(t.Context()).Create(&seed).Error; err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Hostname() == "" {
		t.Fatal("运行验收需要 URL 形式的测试 DSN")
	}
	port := parsed.Port()
	if port == "" {
		port = "5432"
	}
	proxy, cutDB := runtimeDBProxy(t, net.JoinHostPort(parsed.Hostname(), port))
	parsed.Host = proxy
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
	cmd.Env = append(os.Environ(), "KEI_BOTS_QQ_MAIN_ENABLED=false", "KEI_BOTS_MOCK_MAIN_LISTEN_ADDR="+addr, "KEI_AUTH_ADMIN_USERS=mock:u1", "KEI_LOG_LEVEL=debug", "KEI_PLUGINS_PERSONA_DEBUG_PROMPTS=true", "KEI_PLUGINS_PERSONA_GROUP_LIST=[g1]", "KEI_PLUGINS_PERSONA_TRIGGER_KEYWORDS=[bot]", "KEI_PLUGINS_PERSONA_RANDOM_ENABLED=false", "KEI_PLUGINS_PERSONA_RANDOM_TIMEZONE=UTC", "KEI_PLUGINS_PERSONA_MEMORY_DATABASE_DSN="+parsed.String(), "KEI_PLUGINS_PERSONA_BATCH_WINDOW=20ms", "KEI_PLUGINS_PERSONA_BATCH_MAX_WINDOW=30ms", "KEI_PLUGINS_PERSONA_MENTION_MIN_INTERVAL=0s", "KEI_PLUGINS_PERSONA_REPLY_DEDUPE=false", "KEI_PLUGINS_PERSONA_LLM_MAX_TOKENS=2048", "KEI_PLUGINS_PERSONA_MEMORY_CONTEXT_TIMEOUT=200ms")
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
	inject := func(text string) bool {
		data, _ := json.Marshal(map[string]string{"text": text, "user_id": "u1", "user_name": "测试用户", "channel_id": "g1"})
		response, err := client.Post("http://"+addr+"/inject", "application/json", bytes.NewReader(data))
		if err != nil {
			return false
		}
		_ = response.Body.Close()
		return response.StatusCode == http.StatusAccepted
	}
	sent := func() ([]json.RawMessage, string) {
		response, err := client.Get("http://" + addr + "/sent")
		if err != nil {
			return nil, ""
		}
		defer func() { _ = response.Body.Close() }()
		data, _ := io.ReadAll(response.Body)
		var rows []json.RawMessage
		_ = json.Unmarshal(data, &rows)
		return rows, string(data)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		inject("/agent memory status")
		_, text := sent()
		if strings.Contains(text, "数据库=就绪") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("宿主数据库未就绪")
		}
		time.Sleep(100 * time.Millisecond)
	}
	rows, _ := sent()
	before := len(rows)
	if !inject("bot 前两天我们聊了什么") {
		t.Fatal("注入失败")
	}
	deadline = time.Now().Add(45 * time.Second)
	var first string
	for {
		rows, _ = sent()
		for _, row := range rows[min(before, len(rows)):] {
			if !strings.Contains(string(row), "记忆：已启用") {
				first = string(row)
				break
			}
		}
		if first != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("有记忆时未收到回复")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !strings.Contains(first, "聚餐") && !strings.Contains(first, "城东餐厅") {
		t.Fatal("真实模型回复未提及合成摘要话题")
	}
	if !strings.Contains(logs.String(), "# 记忆") || !strings.Contains(logs.String(), "近期重点") {
		t.Fatal("debug 提示词日志缺少记忆块")
	}
	cutDB()
	rows, _ = sent()
	before = len(rows)
	if !inject("bot 现在还能回复吗") {
		t.Fatal("故障后注入失败")
	}
	deadline = time.Now().Add(45 * time.Second)
	for {
		rows, _ = sent()
		if len(rows) > before {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("切断数据库后未正常回复")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !strings.Contains(logs.String(), "memory_context") {
		t.Fatal("数据库故障未记录读取告警")
	}
	t.Log("真实宿主：记忆块出现在 debug 提示词，回复提到聚餐；切断该宿主数据库链路后仍正常回复并记录读取告警")
}
