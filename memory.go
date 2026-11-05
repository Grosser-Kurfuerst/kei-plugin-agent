package persona

import (
	"context"
	"strconv"
	"time"

	"github.com/RandomLemon/kei/pkg/bot"

	"github.com/RandomLemon/kei-plugin-persona/internal/memory"
)

// memoryMaxTextRunes 是入站消息写入记忆时的文本上限。
const memoryMaxTextRunes = 2000

// memoryBackend 是插件对记忆子系统的依赖，由 *memory.Service 实现；测试注入假实现。
type memoryBackend interface {
	Start(ctx context.Context)
	Stop(ctx context.Context)
	Enqueue(r memory.Record) bool
	Stats() memory.Stats
}

// memoryRecordFromEvent 由入站事件构造记忆记录；text 为 renderMessage 的扁平文本。
// 调用方保证 ev.Sender 非空（Handler 入口已拦截）。
func (p *Plugin) memoryRecordFromEvent(ev *bot.Event, text string) memory.Record {
	_, platform, botID, channelID := channelKey(ev)
	sender := memory.Sender{ID: ev.Sender.ID, Name: ev.Sender.Name, IsBot: ev.Sender.IsBot}
	conv := memory.Conv{Type: memory.ConvTypeGroup, Platform: platform, BotID: botID, ChannelID: channelID}
	if ev.Channel != nil {
		conv.Name = ev.Channel.Name
	}
	if ev.Message.Kind == bot.MessagePrivate {
		conv.Type = memory.ConvTypePrivate
		conv.PeerUserID = sender.ID
		if conv.Name == "" {
			conv.Name = sender.Name
		}
	}
	at := ev.Time
	if at.IsZero() {
		at = p.now()
	}
	r := memory.Record{
		Conv:    conv,
		EventID: ev.ID,
		MsgID:   ev.Message.ID,
		At:      at.UTC(),
		Sender:  sender,
		Text:    truncateRunes(text, memoryMaxTextRunes),
	}
	for _, seg := range ev.Message.Segments {
		switch seg.Type {
		case bot.SegReply:
			if r.ReplyTo == "" {
				r.ReplyTo = bot.SegmentData(seg, bot.KeyMessageID)
			}
		case bot.SegAt:
			if id := bot.SegmentData(seg, bot.KeyUserID); id != "" && id != atAllSentinel {
				r.Mentions = append(r.Mentions, id)
			}
		}
	}
	return r
}

// memoryRecordFromReply 构造一条自身回复的记忆记录。target 为发送目标（由会话状态构造），
// mention 为回复 @ 的用户（可为空），res 为发送结果（可为 nil），at 为发送成功时刻。
// Conv.Name 留空：群名只由入站消息维护，避免用建状态时的旧名覆盖新名。
func (p *Plugin) memoryRecordFromReply(target bot.Target, personaName, reply, mention string, res *bot.SendResult, at time.Time) memory.Record {
	conv := memory.Conv{Type: memory.ConvTypeGroup, Platform: target.Platform, BotID: target.BotID, ChannelID: target.ChannelID}
	if target.Kind == bot.MessagePrivate {
		conv.Type = memory.ConvTypePrivate
		conv.PeerUserID = target.UserID
	}
	r := memory.Record{
		Conv:    conv,
		At:      at.UTC(),
		Sender:  memory.Sender{ID: target.BotID, Name: p.personaDisplayName(personaName), IsBot: true},
		Self:    true,
		Persona: personaName,
		Text:    reply,
	}
	if res != nil {
		r.MsgID = res.MessageID
	}
	if r.MsgID == "" {
		r.EventID = "local:" + strconv.FormatInt(p.memEpoch, 10) + ":" + strconv.FormatUint(p.memSeq.Add(1), 10)
	}
	if mention != "" {
		r.Mentions = []string{mention}
	}
	return r
}
