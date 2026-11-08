package summary

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/RandomLemon/kei-plugin-persona/internal/memory/llm"
	"github.com/RandomLemon/kei-plugin-persona/internal/memory/store"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Pipeline 是日摘要 Handler 的依赖；启动后只读。
type Pipeline struct {
	DB           *gorm.DB
	Client       *llm.Client
	Small, Large string
	Location     *time.Location
	Now          func() time.Time
	Log          *slog.Logger
}

// Daily 按日期升序处理本会话最近七天未摘要的数据。
func (p Pipeline) Daily(ctx context.Context, job store.Job) (store.JobResult, error) {
	result := store.JobResult{}
	today := store.LocalDate(p.Now(), p.Location)
	var days []store.ConvDay
	err := p.DB.WithContext(ctx).Where("conv_key = ? AND local_date >= ? AND local_date < ? AND msg_count > 0 AND summarized_upto_id IS NULL", job.Payload.ConvKey, today.AddDate(0, 0, -7), today).Order("local_date").Find(&days).Error
	if err != nil {
		return result, err
	}
	client := *p.Client
	client.Log = p.Log.With("job_id", job.ID, "conv_key", job.Payload.ConvKey)
	for _, day := range days {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if err = p.day(ctx, &client, day, &result); err != nil {
			result.FailedDays++
			p.Log.Warn("persona memory: 日摘要失败", "operation", "memory_summary", "job_id", job.ID, "conv_key", day.ConvKey, "date", day.LocalDate.Format("2006-01-02"), "err", err)
		} else {
			result.Days++
		}
	}
	if result.FailedDays > 0 {
		return result, fmt.Errorf("%d 个日期摘要失败", result.FailedDays)
	}
	return result, nil
}

func (p Pipeline) day(ctx context.Context, client *llm.Client, day store.ConvDay, result *store.JobResult) error {
	var messages []store.Message
	if err := p.DB.WithContext(ctx).Where("conv_key = ? AND local_date = ? AND recalled = false", day.ConvKey, day.LocalDate).Order("sent_at, id").Find(&messages).Error; err != nil {
		return err
	}
	var conv store.Conversation
	if err := p.DB.WithContext(ctx).Where("conv_key = ?", day.ConvKey).First(&conv).Error; err != nil {
		return err
	}
	input := NewInput(messages, conv, day.LocalDate, p.Location)
	slices, dropped := Slice(messages)
	result.Slices += len(slices)
	result.DroppedSlices += dropped
	if dropped > 0 {
		p.Log.Warn("persona memory: 日摘要切片超限", "operation", "memory_summary", "conv_key", day.ConvKey, "dropped_slices", dropped)
	}
	type sliceResult struct {
		topics []store.Topic
		usage  llm.Usage
		err    error
		active bool
	}
	outputs := make([]sliceResult, len(slices))
	var wg sync.WaitGroup
	tasks := make(chan int)
	for range min(sliceConcurrency, len(slices)) {
		wg.Go(func() {
			for n := range tasks {
				raw := slices[n]
				filtered := Prefilter(raw)
				if len(filtered) == 0 {
					continue
				}
				out := SliceOutput{}
				usage, err := client.ChatJSON(ctx, llm.Request{Model: p.Small, System: SliceSystem, User: input.RenderSlice(filtered), Temperature: llmTemperature}, &out)
				outputs[n] = sliceResult{usage: usage, err: err, active: true}
				if err == nil {
					outputs[n].topics = input.Normalize(out, raw, filtered)
				}
			}
		})
	}
dispatch:
	for n := range slices {
		select {
		case tasks <- n:
		case <-ctx.Done():
			break dispatch
		}
	}
	close(tasks)
	wg.Wait()
	active, failed := 0, 0
	var topics []store.Topic
	var callErr error
	for _, out := range outputs {
		result.PromptTokens += out.usage.PromptTokens
		result.CompletionTokens += out.usage.CompletionTokens
		if out.active {
			active++
		}
		if out.err != nil {
			failed++
			result.FailedSlices++
			if callErr == nil && !errors.Is(out.err, llm.ErrInvalidOutput) {
				callErr = out.err
			}
		}
		topics = append(topics, out.topics...)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// 网络、超时与 HTTP 错误可能是临时故障：整天失败重试，避免永久丢失该切片。
	if callErr != nil {
		return callErr
	}
	if active > 0 && failed == active {
		return fmt.Errorf("全部切片小结失败")
	}
	model := p.Small
	overview := ""
	if len(topics) > 0 {
		var known []store.Topic
		if err := p.DB.WithContext(ctx).Table("mem_topic AS t").Select("t.*").Joins("JOIN mem_summary AS s ON s.id=t.summary_id").Where("t.conv_key = ? AND t.local_date >= ? AND t.local_date < ? AND s.kind='daily'", day.ConvKey, day.LocalDate.AddDate(0, 0, -WindowDays), day.LocalDate).Order("t.local_date DESC, t.importance DESC, t.id").Limit(30).Scan(&known).Error; err != nil {
			return err
		}
		output := MergeOutput{}
		fallback := false
		if len(topics) == 1 && len(known) == 0 {
			output.Overview = topics[0].Title
			fallback = true
		} else {
			usage, err := client.ChatJSON(ctx, llm.Request{Model: p.Large, System: MergeSystem, User: renderMerge(topics, known), Temperature: llmTemperature}, &output)
			result.PromptTokens += usage.PromptTokens
			result.CompletionTokens += usage.CompletionTokens
			if err != nil {
				if !errors.Is(err, llm.ErrInvalidOutput) {
					return err
				}
				fallback = true
				result.MergeFallbacks++
			} else {
				model = p.Large
			}
		}
		topics, overview = Merge(output, topics, known, day.ConvKey, day.LocalDate, fallback)
	}
	for n := range topics {
		topics[n].Title = Redact(topics[n].Title)
		topics[n].Summary = Redact(topics[n].Summary)
	}
	overview = Redact(overview)
	return p.write(ctx, day, messages, topics, overview, conv.Type, model)
}

func (p Pipeline) write(ctx context.Context, day store.ConvDay, messages []store.Message, topics []store.Topic, overview, scope, model string) error {
	if scope == "private" {
		scope = "user_private"
	}
	return p.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked store.ConvDay
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("conv_key=? AND local_date=?", day.ConvKey, day.LocalDate).First(&locked).Error; err != nil {
			return err
		}
		var old store.Summary
		err := tx.Where("conv_key=? AND local_date=? AND kind='daily'", day.ConvKey, day.LocalDate).First(&old).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if old.ID != 0 {
			if err := tx.Delete(&old).Error; err != nil {
				return err
			}
		}
		if len(topics) > 0 {
			summary := store.Summary{ConvKey: day.ConvKey, ScopeType: scope, LocalDate: day.LocalDate, Kind: "daily", Version: old.Version + 1, Overview: overview, MsgCount: len(messages), Model: model, Topics: topics}
			if len(messages) > 0 {
				summary.FromMsgID = messages[0].ID
				summary.ToMsgID = messages[len(messages)-1].ID
			}
			if err := tx.Create(&summary).Error; err != nil {
				return err
			}
		}
		return tx.Model(&locked).Update("summarized_upto_id", day.LastMsgID).Error
	})
}
