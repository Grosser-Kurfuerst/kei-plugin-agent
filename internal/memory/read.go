package memory

import (
	"context"
	"fmt"

	"github.com/RandomLemon/kei-plugin-persona/internal/memory/store"
	"github.com/RandomLemon/kei-plugin-persona/internal/memory/summary"
)

// Context 只读取可信 Scene 指定的会话，返回预算内的近期日摘要。
func (s *Service) Context(ctx context.Context, req ContextRequest) (ContextResult, error) {
	s.mu.RLock()
	db, ready := s.db, s.state == StateReady
	s.mu.RUnlock()
	if !ready || db == nil {
		return ContextResult{}, ErrNotReady
	}
	key, ok := convKey(req.Scene.Conv)
	if !ok {
		return ContextResult{}, fmt.Errorf("memory: 会话身份不完整")
	}
	today := store.LocalDate(s.now(), s.cfg.Location)
	var candidates []summary.Candidate
	err := db.WithContext(ctx).Table("mem_topic AS t").Select("t.*, s.overview").Joins("JOIN mem_summary AS s ON s.id = t.summary_id").Where("t.conv_key = ? AND s.conv_key = ? AND t.local_date >= ? AND t.local_date < ? AND s.kind = 'daily'", key, key, today.AddDate(0, 0, -summary.WindowDays), today).Order("t.local_date DESC, t.id").Scan(&candidates).Error
	if err != nil {
		s.mu.RLock()
		stopped := s.stopped
		s.mu.RUnlock()
		if stopped {
			return ContextResult{}, ErrNotReady
		}
		return ContextResult{}, err
	}
	lines := summary.SelectLines(candidates, req.Queries, today, req.BudgetChars)
	if len(lines) == 0 {
		return ContextResult{}, nil
	}
	return ContextResult{Blocks: []Block{{Title: "近期重点", Lines: lines}}}, nil
}
