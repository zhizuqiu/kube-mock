package provider

import (
	"context"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	"github.com/virtual-kubelet/virtual-kubelet/trace"
	"time"
)

const (
	statusChangeInterval = 1 * time.Second
)

type MockPodsTrackerHandler interface {
	UpdateMockPodsStatus(ctx context.Context)
}

type MockPodsTracker struct {
	handler MockPodsTrackerHandler
}

func (pt *MockPodsTracker) StartTracking(ctx context.Context) {
	ctx, span := trace.StartSpan(ctx, "MockTracker.StartTracking")
	defer span.End()

	statusChangeTimer := time.NewTimer(statusChangeInterval)
	defer statusChangeTimer.Stop()

	for {
		log.G(ctx).Debug("Pod status change loop start")

		select {
		case <-ctx.Done():
			log.G(ctx).WithError(ctx.Err()).Debug("Pod status change loop exiting")
			return
		case <-statusChangeTimer.C:
			pt.changeMockPodsLoop(ctx)
			statusChangeTimer.Reset(statusChangeInterval)
		}
	}
}

func (pt *MockPodsTracker) changeMockPodsLoop(ctx context.Context) {
	ctx, span := trace.StartSpan(ctx, "MockTracker.changeMockPods")
	defer span.End()
	pt.handler.UpdateMockPodsStatus(ctx)
}
