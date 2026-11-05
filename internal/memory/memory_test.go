package memory

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestStartUnreachableDSN(t *testing.T) {
	s := New(Config{
		DatabaseDSN:    "postgres://u:p@127.0.0.1:1/db?sslmode=disable",
		Location:       time.UTC,
		QueueSize:      10,
		IngestBatch:    5,
		IngestInterval: time.Second,
	}, discardLogger())
	s.connectBackoffMin = 10 * time.Millisecond

	begin := time.Now()
	s.Start(t.Context())
	if elapsed := time.Since(begin); elapsed > 100*time.Millisecond {
		t.Fatalf("Start 耗时 %v，应立即返回", elapsed)
	}
	time.Sleep(50 * time.Millisecond) // 让连库协程至少失败、退避一轮
	if got := s.Stats().State; got != StateConnecting {
		t.Fatalf("State = %q, want %q", got, StateConnecting)
	}

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	begin = time.Now()
	s.Stop(ctx)
	if elapsed := time.Since(begin); elapsed > 500*time.Millisecond {
		t.Fatalf("Stop 耗时 %v", elapsed)
	}
	select {
	case <-s.done:
	default:
		t.Fatal("Stop 返回后连库协程仍未退出")
	}
	if got := s.Stats().State; got != StateStopped {
		t.Fatalf("State = %q, want %q", got, StateStopped)
	}
	if s.Enqueue(Record{}) {
		t.Fatal("停止后 Enqueue 应返回 false")
	}
	s.Stop(ctx) // 幂等
}

func TestStopWithoutStart(t *testing.T) {
	s := New(Config{QueueSize: 1, IngestBatch: 1, IngestInterval: time.Second}, discardLogger())
	s.Stop(t.Context())
	if got := s.Stats().State; got != StateStopped {
		t.Fatalf("State = %q, want %q", got, StateStopped)
	}
	s.Start(t.Context())
	if s.done != nil {
		t.Fatal("Stop 之后 Start 不应启动后台协程")
	}
}
