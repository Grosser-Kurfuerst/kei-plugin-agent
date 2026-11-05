package persona

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/RandomLemon/kei/pkg/bot"

	"github.com/RandomLemon/kei-plugin-persona/internal/memory"
)

// memoryTestPlugin 返回只装配了记录构造所需字段的插件。
func memoryTestPlugin(now time.Time) *Plugin {
	return &Plugin{
		cfg: &config{personas: map[string]personaConfig{
			"tsundere": {DisplayName: "小傲娇"},
		}},
		now: func() time.Time { return now },
	}
}

func TestMemoryRecordFromGroupEvent(t *testing.T) {
	p := memoryTestPlugin(time.Now())
	shanghai := time.FixedZone("CST", 8*3600)
	ev := groupEvent("g1", "u1", "张三", "hi")
	ev.ID = "ev-1"
	ev.Time = time.Date(2026, 11, 5, 20, 0, 0, 0, shanghai)
	ev.Message.ID = "msg-1"
	ev.Message.Segments = append(ev.Message.Segments,
		bot.Segment{Type: bot.SegReply, Data: map[string]any{bot.KeyMessageID: 9001}},
		bot.Segment{Type: bot.SegReply, Data: map[string]any{bot.KeyMessageID: "second"}},
		bot.Segment{Type: bot.SegAt, Data: map[string]any{bot.KeyUserID: "u2"}},
		bot.Segment{Type: bot.SegAt, Data: map[string]any{bot.KeyUserID: "all"}},
		bot.Segment{Type: bot.SegAt, Data: map[string]any{bot.KeyUserID: "u3"}},
	)
	ev.Sender.IsBot = true

	r := p.memoryRecordFromEvent(ev, "hi")
	wantConv := memory.Conv{Type: memory.ConvTypeGroup, Platform: "mock", BotID: "bot1", ChannelID: "g1", Name: "群g1"}
	if r.Conv != wantConv {
		t.Errorf("Conv = %+v, want %+v", r.Conv, wantConv)
	}
	if r.EventID != "ev-1" || r.MsgID != "msg-1" {
		t.Errorf("EventID/MsgID = %q/%q", r.EventID, r.MsgID)
	}
	if r.At.Location() != time.UTC || !r.At.Equal(ev.Time) {
		t.Errorf("At = %v, want UTC of %v", r.At, ev.Time)
	}
	if r.Sender != (memory.Sender{ID: "u1", Name: "张三", IsBot: true}) {
		t.Errorf("Sender = %+v", r.Sender)
	}
	if r.Self || r.Persona != "" || r.Text != "hi" {
		t.Errorf("Self/Persona/Text = %v/%q/%q", r.Self, r.Persona, r.Text)
	}
	if r.ReplyTo != "9001" {
		t.Errorf("ReplyTo = %q, want 9001", r.ReplyTo)
	}
	if !slices.Equal(r.Mentions, []string{"u2", "u3"}) {
		t.Errorf("Mentions = %v, want [u2 u3]", r.Mentions)
	}
}

func TestMemoryRecordFromEventZeroTimeAndTruncate(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.FixedZone("X", 3600))
	p := memoryTestPlugin(now)
	ev := groupEvent("g1", "u1", "张三", "")
	ev.Time = time.Time{}

	r := p.memoryRecordFromEvent(ev, strings.Repeat("字", memoryMaxTextRunes+10))
	if r.At.Location() != time.UTC || !r.At.Equal(now) {
		t.Errorf("At = %v, want UTC of %v", r.At, now)
	}
	if n := runeCount(r.Text); n != memoryMaxTextRunes {
		t.Errorf("Text 长度 = %d, want %d", n, memoryMaxTextRunes)
	}
}

func TestMemoryRecordFromPrivateEvent(t *testing.T) {
	p := memoryTestPlugin(time.Now())
	r := p.memoryRecordFromEvent(privateEvent("u9", "李四", "在吗"), "在吗")
	want := memory.Conv{Type: memory.ConvTypePrivate, Platform: "mock", BotID: "bot1", PeerUserID: "u9", Name: "李四"}
	if r.Conv != want {
		t.Errorf("Conv = %+v, want %+v", r.Conv, want)
	}
}

func TestMemoryRecordFromReply(t *testing.T) {
	now := time.Date(2026, 11, 5, 20, 0, 0, 0, time.FixedZone("CST", 8*3600))
	p := memoryTestPlugin(now)
	p.memEpoch = 42
	group := bot.Target{Platform: "mock", BotID: "bot1", ChannelID: "g1", Kind: bot.MessageGroup}

	r := p.memoryRecordFromReply(group, "tsundere", "哼", "u1", &bot.SendResult{MessageID: "m-1"}, now)
	wantConv := memory.Conv{Type: memory.ConvTypeGroup, Platform: "mock", BotID: "bot1", ChannelID: "g1"}
	if r.Conv != wantConv {
		t.Errorf("Conv = %+v, want %+v（Name 必须为空）", r.Conv, wantConv)
	}
	if r.MsgID != "m-1" || r.EventID != "" {
		t.Errorf("MsgID/EventID = %q/%q", r.MsgID, r.EventID)
	}
	if r.Sender != (memory.Sender{ID: "bot1", Name: "小傲娇", IsBot: true}) {
		t.Errorf("Sender = %+v", r.Sender)
	}
	if !r.Self || r.Persona != "tsundere" || r.Text != "哼" || r.ReplyTo != "" {
		t.Errorf("Self/Persona/Text/ReplyTo = %v/%q/%q/%q", r.Self, r.Persona, r.Text, r.ReplyTo)
	}
	if r.At.Location() != time.UTC || !r.At.Equal(now) {
		t.Errorf("At = %v", r.At)
	}
	if !slices.Equal(r.Mentions, []string{"u1"}) {
		t.Errorf("Mentions = %v, want [u1]", r.Mentions)
	}

	private := bot.Target{Platform: "mock", BotID: "bot1", UserID: "u9", Kind: bot.MessagePrivate}
	r1 := p.memoryRecordFromReply(private, "default", "嗯", "", nil, now)
	r2 := p.memoryRecordFromReply(private, "default", "嗯", "", &bot.SendResult{}, now)
	if want := (memory.Conv{Type: memory.ConvTypePrivate, Platform: "mock", BotID: "bot1", PeerUserID: "u9"}); r1.Conv != want {
		t.Errorf("私聊 Conv = %+v, want %+v", r1.Conv, want)
	}
	if r1.EventID != "local:42:1" || r2.EventID != "local:42:2" {
		t.Errorf("本地 EventID = %q, %q", r1.EventID, r2.EventID)
	}
	if r1.Mentions != nil {
		t.Errorf("无 @ 时 Mentions 应为空, got %v", r1.Mentions)
	}
}
