package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

const codexInspectionRunnerTick = 30 * time.Second

type CodexInspectionRunner struct {
	svc  *CodexInspectionService
	stop chan struct{}
	done chan struct{}
	once sync.Once

	mu              sync.Mutex
	lastIntervalRun time.Time
	firedTimePoints map[string]struct{}
}

func NewCodexInspectionRunner(svc *CodexInspectionService) *CodexInspectionRunner {
	return &CodexInspectionRunner{
		svc:             svc,
		stop:            make(chan struct{}),
		done:            make(chan struct{}),
		firedTimePoints: map[string]struct{}{},
	}
}

func ProvideCodexInspectionRunner(svc *CodexInspectionService) *CodexInspectionRunner {
	r := NewCodexInspectionRunner(svc)
	r.Start()
	return r
}

func (r *CodexInspectionRunner) Start() {
	if r == nil || r.svc == nil {
		return
	}
	go r.loop()
}

func (r *CodexInspectionRunner) Stop() {
	if r == nil {
		return
	}
	r.once.Do(func() { close(r.stop) })
	select {
	case <-r.done:
	case <-time.After(5 * time.Second):
	}
}

func (r *CodexInspectionRunner) loop() {
	defer close(r.done)
	ticker := time.NewTicker(codexInspectionRunnerTick)
	defer ticker.Stop()
	for {
		select {
		case <-r.stop:
			return
		case <-ticker.C:
			r.tick()
		}
	}
}

func (r *CodexInspectionRunner) tick() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	settings, err := r.svc.GetSettings(ctx)
	if err != nil {
		slog.Warn("codex_inspection.runner_get_settings_failed", "error", err)
		return
	}
	settings = normalizeCodexInspectionSettings(settings)
	if !settings.Enabled {
		return
	}
	if !r.shouldRun(settings, time.Now()) {
		return
	}
	_, err = r.svc.StartRun(ctx, CodexInspectionRunRequest{
		TriggerType:  CodexInspectionTriggerScheduled,
		TriggerKey:   settings.Schedule.Mode,
		ApplyActions: settings.Actions.AutoApply,
	})
	if err != nil {
		slog.Warn("codex_inspection.runner_start_failed", "error", err)
	}
}

func (r *CodexInspectionRunner) shouldRun(settings CodexInspectionSettings, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch settings.Schedule.Mode {
	case CodexInspectionScheduleModeTimePoints:
		loc, err := time.LoadLocation(settings.Schedule.TimeZone)
		if err != nil {
			return false
		}
		local := now.In(loc)
		current := local.Format("15:04")
		for _, point := range settings.Schedule.TimePoints {
			if strings.TrimSpace(point) != current {
				continue
			}
			key := fmt.Sprintf("%s %s", local.Format("2006-01-02"), current)
			if _, ok := r.firedTimePoints[key]; ok {
				return false
			}
			r.firedTimePoints[key] = struct{}{}
			return true
		}
		return false
	default:
		interval := time.Duration(settings.Schedule.IntervalMinutes) * time.Minute
		if interval <= 0 {
			interval = time.Hour
		}
		if r.lastIntervalRun.IsZero() || now.Sub(r.lastIntervalRun) >= interval {
			r.lastIntervalRun = now
			return true
		}
		return false
	}
}
