package jobs

import (
	"context"
	"github.com/RandomLemon/kei-plugin-persona/internal/memory/store"
	"time"
)

// Scheduler 每日按本地时间建立去重任务。
type Scheduler struct {
	Queue    Queue
	Location *time.Location
	DailyAt  string
}

// Tick 刷新计数，并在每日指定时间后创建任务。
func (s Scheduler) Tick(ctx context.Context, now time.Time) (Counts, error) {
	counts, err := s.Queue.Count(ctx)
	if err != nil {
		return counts, err
	}
	local := now.In(s.Location)
	if local.Format("15:04") < s.DailyAt {
		return counts, nil
	}
	date := local.Format("2006-01-02")
	err = s.Queue.Create(ctx, store.Job{Type: "cleanup", DedupeKey: "cleanup:" + date, Payload: store.JobPayload{Date: date}, Status: "pending", RunAfter: now.UTC()})
	return counts, err
}
