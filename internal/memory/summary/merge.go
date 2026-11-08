package summary

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/RandomLemon/kei-plugin-persona/internal/memory/store"
)

// MergeOutput 是合并模型输出。
type MergeOutput struct {
	Overview string        `json:"overview"`
	Topics   []MergedTopic `json:"topics"`
}

// MergedTopic 只通过编号选择来源话题与要点。
type MergedTopic struct {
	From       []string `json:"from"`
	Title      string   `json:"title"`
	Summary    string   `json:"summary"`
	KeyPoints  []string `json:"key_points"`
	Importance *float64 `json:"importance"`
	Continues  string   `json:"continues"`
}

// Validate 检查顶层结构。
func (o *MergeOutput) Validate() error {
	if o.Topics == nil {
		return fmt.Errorf("缺少 topics 数组")
	}
	return nil
}

func index(value, prefix string, limit int) int {
	if !strings.HasPrefix(value, prefix) {
		return -1
	}
	n, err := strconv.Atoi(strings.TrimPrefix(value, prefix))
	if err != nil || n < 1 || n > limit {
		return -1
	}
	return n - 1
}

// Merge 规范化来源与延续关系；fallback 时完全由已校验切片话题生成结果。
func Merge(output MergeOutput, source, known []store.Topic, conv string, date time.Time, fallback bool) ([]store.Topic, string) {
	var topics []store.Topic
	used := map[int]bool{}
	continued := map[string]bool{}
	if !fallback {
		for _, candidate := range output.Topics {
			var selected []int
			for _, ref := range candidate.From {
				n := index(ref, "s", len(source))
				if n >= 0 && !used[n] {
					selected = append(selected, n)
					used[n] = true
				}
			}
			if len(selected) == 0 {
				continue
			}
			best := source[selected[0]]
			topic := store.Topic{}
			for j, n := range selected {
				s := source[n]
				if s.Importance > best.Importance {
					best = s
				}
				for _, v := range s.Participants {
					topic.Participants = appendUnique(topic.Participants, v)
				}
				for _, v := range s.Entities {
					topic.Entities = appendUnique(topic.Entities, v)
				}
				if j == 0 || s.FirstMsgID < topic.FirstMsgID {
					topic.FirstMsgID = s.FirstMsgID
				}
				if s.LastMsgID > topic.LastMsgID {
					topic.LastMsgID = s.LastMsgID
				}
				if j == 0 || s.FirstAt.Before(topic.FirstAt) {
					topic.FirstAt = s.FirstAt
				}
				if s.LastAt.After(topic.LastAt) {
					topic.LastAt = s.LastAt
				}
			}
			topic.Title = truncate(Redact(candidate.Title), 20)
			if topic.Title == "" {
				topic.Title = best.Title
			}
			topic.Summary = truncate(Redact(candidate.Summary), 120)
			if topic.Summary == "" {
				topic.Summary = best.Summary
			}
			topic.Importance = best.Importance
			if candidate.Importance != nil {
				topic.Importance = importance(*candidate.Importance)
			}
			seen := map[string]bool{}
			for _, ref := range candidate.KeyPoints {
				parts := strings.Split(ref, ".")
				if len(parts) != 2 || seen[ref] {
					continue
				}
				n := index(parts[0], "s", len(source))
				p, err := strconv.Atoi(parts[1])
				if n < 0 || !slices.Contains(selected, n) || err != nil || p < 0 || p >= len(source[n].KeyPoints) {
					continue
				}
				seen[ref] = true
				topic.KeyPoints = append(topic.KeyPoints, source[n].KeyPoints[p])
			}
			if n := index(candidate.Continues, "k", len(known)); n >= 0 && !continued[known[n].TopicKey] {
				topic.TopicKey = known[n].TopicKey
				topic.ContinuesFrom = known[n].TopicKey
				continued[topic.TopicKey] = true
			}
			topics = append(topics, topic)
		}
	}
	for n, t := range source {
		if !used[n] && (fallback || t.Importance >= 0.3) {
			t.TopicKey = ""
			t.ContinuesFrom = ""
			topics = append(topics, t)
		}
	}
	for n := range topics {
		t := &topics[n]
		if t.TopicKey == "" {
			t.TopicKey = fmt.Sprintf("topic:%s:%s:%d", conv, date.Format("20060102"), n+1)
		}
		t.ConvKey = conv
		t.LocalDate = date
		for _, p := range t.KeyPoints {
			t.HasOpen = t.HasOpen || p.Type == "todo" || p.Type == "open_question"
		}
	}
	overview := truncate(Redact(output.Overview), 60)
	if fallback || overview == "" {
		ranked := slices.Clone(topics)
		slices.SortStableFunc(ranked, func(a, b store.Topic) int {
			if a.Importance > b.Importance {
				return -1
			}
			if a.Importance < b.Importance {
				return 1
			}
			return 0
		})
		titles := []string{}
		for _, t := range ranked[:min(3, len(ranked))] {
			titles = append(titles, t.Title)
		}
		overview = truncate(strings.Join(titles, "；"), 60)
	}
	return topics, overview
}
