// Package store 定义记忆子系统的数据库模型、迁移与 gorm 日志适配。
package store

import "time"

// Conversation 是一个会话（群或私聊），conv_key 由 memory 包推导。
type Conversation struct {
	ConvKey    string `gorm:"primaryKey"`
	Type       string `gorm:"not null;default:''"` // group | private
	Platform   string `gorm:"not null;default:''"`
	BotID      string `gorm:"not null;default:''"`
	ChannelID  string `gorm:"not null;default:''"`
	PeerUserID string `gorm:"not null;default:''"`
	Name       string `gorm:"not null;default:''"`
	// ShareEnabled 自 M4 起生效；GORM 插入时跳过零值，写 false 只能用 Update。
	ShareEnabled bool `gorm:"not null;default:true"`
	LastMsgAt    *time.Time
}

// TableName 返回表名。
func (Conversation) TableName() string { return "mem_conversation" }

// Message 是一条原始消息；ID 是切片内的稳定次序。
type Message struct {
	ID          int64     `gorm:"primaryKey"`
	ConvKey     string    `gorm:"not null;default:''"`
	DedupeKey   string    `gorm:"not null;default:''"`
	MsgID       string    `gorm:"not null;default:''"`
	EventID     string    `gorm:"not null;default:''"`
	SenderID    string    `gorm:"not null;default:''"`
	SenderName  string    `gorm:"not null;default:''"`
	SenderIsBot bool      `gorm:"not null"`
	IsSelf      bool      `gorm:"not null"`
	Persona     string    `gorm:"not null;default:''"`
	Text        string    `gorm:"not null;default:''"`
	ReplyTo     string    `gorm:"not null;default:''"`
	Mentions    []string  `gorm:"type:jsonb;serializer:json;not null"`
	SentAt      time.Time `gorm:"not null"`
	LocalDate   time.Time `gorm:"type:date;not null"`
	ReceivedAt  time.Time `gorm:"not null"`
	Recalled    bool      `gorm:"not null"`
}

// TableName 返回表名。
func (Message) TableName() string { return "mem_message" }

// ConvDay 是会话某一本地日的消息计数与各阶段进度。
// 进度列一次建好避免后续改表，在 S4、S6 之前恒为零值。
type ConvDay struct {
	ConvKey          string    `gorm:"primaryKey"`
	LocalDate        time.Time `gorm:"primaryKey;type:date"`
	MsgCount         int       `gorm:"not null"`
	LastMsgID        int64     `gorm:"not null"`
	RolledMsgCount   int       `gorm:"not null"` // S6 使用
	RolledUptoMsgID  int64     `gorm:"not null"` // S6 使用
	RollingRuns      int       `gorm:"not null"` // S6 使用
	SummarizedUptoID *int64    // S4 使用；nil 表示未生成日摘要
}

// TableName 返回表名。
func (ConvDay) TableName() string { return "mem_conv_day" }

// LocalDate 把时刻 t 换算成 loc 下的本地日期，返回该日期的 UTC 零点，供 date 列存取。
func LocalDate(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
