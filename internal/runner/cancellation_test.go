package runner

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

func TestCampaignCancellation(t *testing.T) {
	for _, when := range []string{"before", "after", "concurrent"} {
		t.Run(when, func(t *testing.T) {
			for range 100 {
				ctx, cancel := context.WithCancel(context.Background())
				var stops, terminations atomic.Int32
				control := campaignCancellation{
					ctx:         ctx,
					onStop:      func() { stops.Add(1) },
					onTerminate: func() { terminations.Add(1) },
				}
				request := func() {
					cancel()
					control.stop()
					control.terminate()
				}
				switch when {
				case "before":
					request()
					if stops.Load() != 0 || terminations.Load() != 0 {
						t.Fatal("cancellation called before initialization")
					}
					control.initialize()
				case "after":
					control.initialize()
					request()
				case "concurrent":
					var wg sync.WaitGroup
					wg.Add(2)
					go func() { defer wg.Done(); control.initialize() }()
					go func() { defer wg.Done(); request() }()
					wg.Wait()
				}
				if stops.Load() == 0 || terminations.Load() == 0 {
					t.Fatal("lost cancellation request")
				}
			}
		})
	}
}
