package memory

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/RandomLemon/kei-plugin-persona/internal/memory/memtest"
	"github.com/RandomLemon/kei-plugin-persona/internal/memory/store"
)

func TestContextScopeWindowAndLifecycle(t *testing.T) {
	dsn := memtest.NewDB(t)
	cfg := testConfig(t, dsn)
	now := time.Date(2026, 11, 8, 17, 0, 0, 0, time.UTC) // 上海11月9日01:00，未过04:00日界，仍属11月8日摘要日。
	today := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
	s := startService(t, cfg, func(s *Service) { s.now = func() time.Time { return now } })
	db := openDB(t, dsn)
	for _, spec := range []struct {
		conv, title, kind string
		days              int
	}{
		{"group:qq:g1", "昨日", "daily", 1}, {"group:qq:g1", "大前天", "daily", 3},
		{"group:qq:g1", "超出窗口", "daily", 4}, {"group:qq:g1", "今天", "daily", 0},
		{"group:qq:g1", "滚动", "rolling", 1}, {"group:qq:g2", "其他群", "daily", 1},
		{"private:qq:b1:u1", "私聊", "daily", 1}, {"private:qq:b2:u1", "其他机器人私聊", "daily", 1},
	} {
		date := today.AddDate(0, 0, -spec.days)
		summary := store.Summary{ConvKey: spec.conv, LocalDate: date, Kind: spec.kind, Overview: "日概况", Topics: []store.Topic{{ConvKey: spec.conv, LocalDate: date, TopicKey: spec.title, Title: spec.title, Summary: "对应摘要", Importance: 1, KeyPoints: []store.KeyPoint{{Type: "todo", Text: "确认安排", Speaker: "u1", MsgIDs: []string{"m1"}}}, Participants: []string{"u1"}}}}
		if err := db.WithContext(t.Context()).Create(&summary).Error; err != nil {
			t.Fatal(err)
		}
	}
	req := ContextRequest{Scene: Scene{Conv: Conv{Type: ConvTypeGroup, Platform: "qq", ChannelID: "g1"}}, BudgetChars: 1200}
	result, err := s.Context(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Blocks) != 1 {
		t.Fatal(result)
	}
	joined := strings.Join(result.Blocks[0].Lines, " ")
	for _, word := range []string{"昨日", "大前天", "待办：确认安排"} {
		if !strings.Contains(joined, word) {
			t.Fatalf("漏掉 %s: %s", word, joined)
		}
	}
	for _, word := range []string{"超出窗口", "今天", "滚动", "其他群", "私聊"} {
		if strings.Contains(joined, word) {
			t.Fatalf("越界 %s: %s", word, joined)
		}
	}
	req.Scene.Conv = Conv{Type: ConvTypePrivate, Platform: "qq", BotID: "b1", PeerUserID: "u1"}
	result, err = s.Context(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	joined = strings.Join(result.Blocks[0].Lines, " ")
	if !strings.Contains(joined, "私聊") || strings.Contains(joined, "昨日") || strings.Contains(joined, "其他机器人") {
		t.Fatal(joined)
	}
	req.Scene.Conv = Conv{Type: ConvTypeGroup, Platform: "qq", ChannelID: "empty"}
	result, err = s.Context(t.Context(), req)
	if err != nil || len(result.Blocks) != 0 {
		t.Fatalf("空会话 %+v %v", result, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err = s.Context(ctx, req); !errors.Is(err, context.Canceled) {
		t.Fatalf("ctx错误 %v", err)
	}
	stopNow(t, s)
	if _, err = s.Context(t.Context(), req); !errors.Is(err, ErrNotReady) {
		t.Fatal(err)
	}
	fresh := New(cfg, discardLogger(), nil)
	if _, err = fresh.Context(t.Context(), req); !errors.Is(err, ErrNotReady) {
		t.Fatal(err)
	}
}

func TestContextStopDuringQuery(t *testing.T) {
	dsn := memtest.NewDB(t)
	s := startService(t, testConfig(t, dsn), nil)
	db := openDB(t, dsn)
	tx := db.WithContext(t.Context()).Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	if err := tx.Exec("LOCK TABLE mem_topic IN ACCESS EXCLUSIVE MODE").Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := s.Context(ctx, ContextRequest{Scene: Scene{Conv: Conv{Type: ConvTypeGroup, Platform: "qq", ChannelID: "g1"}}, BudgetChars: 1200})
		done <- err
	}()
	// 等服务连接在数据库锁上等待后触发 Stop，读取不能延长其生命周期。
	waitFor(t, func() bool {
		var n int64
		db.WithContext(t.Context()).Raw("SELECT count(*) FROM pg_stat_activity WHERE wait_event_type='Lock' AND query LIKE '%mem_topic AS t%'").Scan(&n)
		return n > 0
	}, "读取等待数据库锁")
	stopNow(t, s)
	if err := <-done; !errors.Is(err, ErrNotReady) {
		t.Fatalf("Stop并发查询应返回ErrNotReady，得到 %v", err)
	}
}
