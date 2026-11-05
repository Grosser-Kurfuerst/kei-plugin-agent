package memory

import "time"

// 会话类型，取值见 Conv.Type。
const (
	ConvTypeGroup   = "group"
	ConvTypePrivate = "private"
)

// 连库状态，取值见 Stats.State。
const (
	StateConnecting    = "连接中"
	StateReady         = "就绪"
	StateMigrateFailed = "迁移失败"
	StateStopped       = "已停止"
)

// Record 是一条原始消息。
type Record struct {
	Conv     Conv
	EventID  string
	MsgID    string
	At       time.Time // UTC
	Sender   Sender
	Self     bool
	Persona  string // 仅 Self=true 时填写
	Text     string
	ReplyTo  string
	Mentions []string
}

// Conv 描述消息所在会话；conv_key 由 memory 包推导。
type Conv struct {
	Type       string // ConvTypeGroup | ConvTypePrivate
	Platform   string
	BotID      string
	ChannelID  string
	PeerUserID string
	Name       string
}

// Sender 是消息发送者。
type Sender struct {
	ID    string
	Name  string
	IsBot bool
}

// Stats 是 /agent memory status 使用的计数快照。
type Stats struct {
	State                         string // StateConnecting | StateReady | StateMigrateFailed | StateStopped
	Written, Duplicates, Rejected int64
	Dropped, FailedBatches        int64
	QueueLen, QueueCap            int
}
