package runner

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"

	"github.com/crytic/medusa/chain"
	"github.com/crytic/medusa/fuzzing"
	"github.com/crytic/medusa/fuzzing/executiontracer"
)

// hardKillAfterTerminate is how long to wait for Start to return after Terminate()
// (a safeguard; Terminate cancels the emergency context and must end the loop).
const hardKillAfterTerminate = 60 * time.Second

// noTestsErrFragment is a fragment of the StopOnNoTests error text (fuzzer.go,
// medusa v1.5.1); matching by string intentionally couples this to that version.
const noTestsErrFragment = "no assertion, property, optimization, or custom tests were found to fuzz"

// Run executes a bounded Medusa fuzzing campaign through the public Go API
// and returns a structured report. Contract:
//   - err == nil                       — campaign completed (status=completed);
//   - errors.Is(err, context.Canceled) — cancelled by the caller (status=cancelled);
//   - any other err                   — technical error (status=error).
//
// The report is also written to <WorkDir>/report.json (if WorkDir is available).
// Parallel runs within one process are NOT supported: NewFuzzer replaces the
// global logging.GlobalLogger (see the project documentation).
func Run(ctx context.Context, params RunParams) (*Report, error) {
	report := newReport(&params)

	// Validate parameters and prepare the workspace.
	if err := params.Validate(); err != nil {
		report.fail(StatusError, ReasonConfigError, err)
		return finalize(ctx, report, params, err)
	}
	if err := os.MkdirAll(params.WorkDir, 0o755); err != nil {
		err = fmt.Errorf("create WorkDir %s: %w", params.WorkDir, err)
		report.fail(StatusError, ReasonInternalError, err)
		return finalize(ctx, report, params, err)
	}

	runStart := time.Now().UTC()
	report.Timing.StartedAt = runStart
	prepStart := runStart

	// Target file: hash used to identify the input project.
	targetHash, err := hashFile(params.TargetPath)
	if err != nil {
		report.fail(StatusError, ReasonConfigError, err)
		return finalize(ctx, report, params, err)
	}
	report.Project.TargetSHA256 = targetHash

	// Effective Medusa configuration and its config.json artifact.
	projectConfig, err := buildProjectConfig(params)
	if err != nil {
		report.fail(StatusError, ReasonConfigError, err)
		return finalize(ctx, report, params, err)
	}
	if raw, mErr := marshalConfig(projectConfig); mErr == nil {
		report.EffectiveConfig = raw
	} else {
		report.Warnings = append(report.Warnings, fmt.Sprintf("serialize effective configuration: %v", mErr))
	}
	if wErr := projectConfig.WriteToFile(filepath.Join(params.WorkDir, "config.json")); wErr != nil {
		wErr = fmt.Errorf("write config.json: %w", wErr)
		report.fail(StatusError, ReasonInternalError, wErr)
		return finalize(ctx, report, params, wErr)
	}

	// Preparation phase: NewFuzzer compiles the target (blocking).
	// The Medusa Go API cannot interrupt Compile(); if ctx is cancelled during
	// preparation, return cancelled and let the goroutine finish on its own (its
	// Fuzzer is inactive until Start). Side effects are limited to logs/ and crytic-export/.
	prepCh := make(chan *fuzzing.Fuzzer, 1)
	prepErrCh := make(chan error, 1)
	go func() {
		f, err := fuzzing.NewFuzzer(projectConfig)
		if err != nil {
			prepErrCh <- err
			return
		}
		prepCh <- f
	}()

	var fuzzer *fuzzing.Fuzzer
	select {
	case fuzzer = <-prepCh:
	case err := <-prepErrCh:
		err = fmt.Errorf("prepare/compile (NewFuzzer): %w", err)
		report.Timing.Phases.PreparationMs = msSince(prepStart)
		report.fail(StatusError, ReasonCompileError, err)
		return finalize(ctx, report, params, err)
	case <-ctx.Done():
		report.Timing.Phases.PreparationMs = msSince(prepStart)
		report.fail(StatusCancelled, ReasonCallerCancel, ctx.Err())
		report.Warnings = append(report.Warnings,
			"cancelled during preparation: the Medusa Go API cannot interrupt compilation; the background NewFuzzer goroutine will finish on its own")
		return finalize(ctx, report, params, ctx.Err())
	}
	report.Timing.Phases.PreparationMs = msSince(prepStart)

	// Progress events (before Start).
	var workersCreated atomic.Int32
	subscribeEvents(fuzzer, &params, &workersCreated, report.RunID)

	// Campaign phase: Start blocks until completion (limit/error/Stop).
	campaignStart := time.Now().UTC()
	startCh := make(chan error, 1)
	if ctx.Err() != nil {
		// The context is already cancelled; do not start the campaign.
		report.fail(StatusCancelled, ReasonCallerCancel, ctx.Err())
		return finalize(ctx, report, params, ctx.Err())
	}
	control := campaignCancellation{ctx: ctx, onStop: fuzzer.Stop, onTerminate: fuzzer.Terminate}
	setup := fuzzer.Hooks.ChainSetupFunc
	fuzzer.Hooks.ChainSetupFunc = func(f *fuzzing.Fuzzer, testChain *chain.TestChain) (*executiontracer.ExecutionTrace, error) {
		control.initialize()
		return setup(f, testChain)
	}
	go func() { startCh <- fuzzer.Start() }()

	var startErr error
	abnormal := false
	cancelled := false
	select {
	case startErr = <-startCh:
	case <-ctx.Done():
		cancelled = true
		control.stop() // graceful cancellation; Terminate below if this does not help
		select {
		case startErr = <-startCh:
		case <-time.After(params.gracePeriod()):
			control.terminate()
			select {
			case startErr = <-startCh:
			case <-time.After(hardKillAfterTerminate):
				abnormal = true
				startErr = errors.New("fuzzer did not return after Stop and Terminate")
			}
		}
	}
	report.Timing.Phases.CampaignMs = msSince(campaignStart)

	if abnormal {
		// Reading TestCases() while Start is running is unsafe (the method has no lock).
		report.fail(StatusError, ReasonStoppedAbnormally, startErr)
		report.Warnings = append(report.Warnings, "results were not read: the campaign did not complete")
		return finalize(ctx, report, params, startErr)
	}

	// Collect results: all statuses are final after Start returns.
	collectTestCases(report, fuzzer.TestCases())
	wc := int(workersCreated.Load())
	report.Metrics.WorkersCreated = &wc
	countCorpusFiles(report, params.corpusDir())
	findCoverage(report, params.corpusDir())

	switch {
	case cancelled:
		report.Status.Technical = StatusCancelled
		report.Status.Reason = ReasonCallerCancel
		report.Status.Error = ctx.Err().Error()
		if startErr != nil {
			// Stop/Terminate can make Start return an error; preserve it.
			report.Warnings = append(report.Warnings, fmt.Sprintf("Start returned an error during cancellation: %v", startErr))
		}
	case startErr != nil:
		if strings.Contains(startErr.Error(), noTestsErrFragment) {
			report.fail(StatusError, ReasonNoTests, startErr)
		} else {
			report.fail(StatusError, ReasonInternalError, startErr)
		}
	default:
		report.Status.Technical = StatusCompleted
		report.Status.Reason = inferStopReason(report, params, campaignStart)
	}

	var retErr error
	if report.Status.Technical == StatusError {
		retErr = errors.New(report.Status.Error)
	} else if cancelled {
		retErr = ctx.Err()
	}
	return finalize(ctx, report, params, retErr)
}

// Medusa initializes cancel functions inside Start, before ChainSetupFunc.
type campaignCancellation struct {
	ctx                context.Context
	ready              atomic.Bool
	terminateRequested atomic.Bool
	onStop             func()
	onTerminate        func()
}

func (c *campaignCancellation) initialize() {
	c.ready.Store(true)
	if c.ctx.Err() != nil {
		c.onStop()
	}
	if c.terminateRequested.Load() {
		c.onTerminate()
	}
}

func (c *campaignCancellation) stop() {
	if c.ready.Load() {
		c.onStop()
	}
}

func (c *campaignCancellation) terminate() {
	c.terminateRequested.Store(true)
	if c.ready.Load() {
		c.onTerminate()
	}
}

// inferStopReason infers why a successful campaign ended: Medusa does not
// expose this through its API (the wrapper infers it from time/results).
func inferStopReason(report *Report, params RunParams, campaignStart time.Time) string {
	dur := time.Since(campaignStart)
	failed := report.TestResults.Summary.Failed
	if params.TimeoutSec > 0 && dur >= time.Duration(float64(params.TimeoutSec)*0.9*float64(time.Second)) {
		return ReasonTimeout
	}
	if failed > 0 {
		return ReasonFailedTestStop // StopOnFailedTest=true (default)
	}
	if params.TestLimit > 0 {
		return ReasonTestLimit
	}
	return "unknown"
}

func subscribeEvents(fuzzer *fuzzing.Fuzzer, params *RunParams, workersCreated *atomic.Int32, runID string) {
	emit := func(eventType string) {
		if params.OnEvent == nil {
			return
		}
		defer func() { _ = recover() }() // The caller's callback must not crash Medusa goroutines.
		params.OnEvent(ProgressEvent{RunID: runID, Type: eventType, Time: time.Now().UTC()})
	}
	fuzzer.Events.FuzzerStarting.Subscribe(func(fuzzing.FuzzerStartingEvent) error {
		emit("fuzzer-starting")
		return nil
	})
	fuzzer.Events.FuzzerStopping.Subscribe(func(fuzzing.FuzzerStoppingEvent) error {
		emit("fuzzer-stopping")
		return nil
	})
	fuzzer.Events.WorkerCreated.Subscribe(func(fuzzing.FuzzerWorkerCreatedEvent) error {
		workersCreated.Add(1)
		emit("worker-created")
		return nil
	})
	fuzzer.Events.WorkerDestroyed.Subscribe(func(fuzzing.FuzzerWorkerDestroyedEvent) error {
		emit("worker-destroyed")
		return nil
	})
}

func collectTestCases(report *Report, testCases []fuzzing.TestCase) {
	items := make([]TestCaseResult, 0, len(testCases))
	for _, tc := range testCases {
		item := TestCaseResult{
			ID:      tc.ID(),
			Kind:    kindByID(tc.ID()),
			Name:    tc.Name(),
			Status:  string(tc.Status()),
			Message: tc.Message(),
		}
		if tc.CallSequence() != nil {
			item.CallSequence = tc.CallSequence().Log().String()
		}
		items = append(items, item)
		switch tc.Status() {
		case fuzzing.TestCaseStatusPassed:
			report.TestResults.Summary.Passed++
		case fuzzing.TestCaseStatusFailed:
			report.TestResults.Summary.Failed++
		case fuzzing.TestCaseStatusRunning:
			report.TestResults.Summary.Running++
		default:
			report.TestResults.Summary.NotStarted++
		}
	}
	report.TestResults.Summary.Total = len(items)
	report.TestResults.Items = items
	report.Metrics.TestCaseCounts = &report.TestResults.Summary
}

func kindByID(id string) string {
	switch {
	case strings.HasPrefix(id, "ASSERTION-"):
		return "assertion"
	case strings.HasPrefix(id, "PROPERTY-"):
		return "property"
	case strings.HasPrefix(id, "OPTIMIZATION-"):
		return "optimization"
	default:
		return "custom"
	}
}

func countCorpusFiles(report *Report, corpusDir string) {
	seqs := countFiles(filepath.Join(corpusDir, "call_sequences"))
	res := countFiles(filepath.Join(corpusDir, "test_results"))
	if seqs != nil {
		report.Metrics.CorpusFilesCallSequences = seqs
	}
	if res != nil {
		report.Metrics.CorpusFilesTestResults = res
	}
}

func findCoverage(report *Report, corpusDir string) {
	coverageDir := filepath.Join(corpusDir, "coverage")
	matches, _ := filepath.Glob(filepath.Join(coverageDir, "*"))
	for _, m := range matches {
		report.Artifacts = append(report.Artifacts, Artifact{Type: "coverage", Path: m})
	}
}

// finalize adds timing/artifacts to the report and writes report.json.
func finalize(ctx context.Context, report *Report, params RunParams, retErr error) (*Report, error) {
	teardownStart := time.Now().UTC()
	report.Timing.FinishedAt = teardownStart
	report.GeneratedAt = teardownStart

	report.Warnings = append(report.Warnings,
		"completing within a time limit without finding violations does NOT prove the contract is correct")

	abs := func(p string) string { a, _ := filepath.Abs(p); return a }
	if params.WorkDir != "" {
		for _, art := range []struct{ typ, path string }{
			{"config", filepath.Join(params.WorkDir, "config.json")},
			{"corpus", params.corpusDir()},
			{"logs", filepath.Join(params.WorkDir, "logs")},
		} {
			if _, err := os.Stat(art.path); err == nil {
				report.Artifacts = append(report.Artifacts, Artifact{Type: art.typ, Path: abs(art.path)})
			}
		}
		if _, err := os.Stat(filepath.Join(params.WorkDir, "crytic-export")); err == nil {
			report.Artifacts = append(report.Artifacts, Artifact{Type: "compile-export", Path: abs(filepath.Join(params.WorkDir, "crytic-export"))})
		}
		report.Artifacts = append(report.Artifacts, Artifact{Type: "report", Path: filepath.Join(params.WorkDir, "report.json")})
	}
	// Set fields before writing so the file matches the returned report.
	report.Timing.Phases.TeardownMs = msSince(teardownStart)
	if params.WorkDir != "" {
		if _, err := report.WriteReport(params.WorkDir); err != nil {
			report.Artifacts = report.Artifacts[:len(report.Artifacts)-1]
			if retErr == nil {
				report.fail(StatusError, ReasonInternalError, err)
			}
			retErr = errors.Join(retErr, err)
			report.Status.Error = retErr.Error()
		}
	}
	return report, retErr
}

func newReport(params *RunParams) *Report {
	r := &Report{
		SchemaVersion: SchemaVersion,
		RunID:         newRunID(),
		Metrics: Metrics{
			CallsTested:      nil,
			SequencesTested:  nil,
			GasUsed:          nil,
			BranchesHit:      nil,
			MetricsAvailable: false,
			MetricsNote:      "Fuzzer does not expose calls_tested, sequences_tested, gas_used, or branches_hit (metrics field is private, medusa v1.5.1 fuzzer.go); null means unavailable, not zero",
		},
		TestResults: struct {
			Summary TestSummary      `json:"summary"`
			Items   []TestCaseResult `json:"items"`
		}{Items: []TestCaseResult{}},
		Warnings:  []string{},
		Artifacts: []Artifact{},
	}
	r.Project.TargetPath = params.TargetPath
	r.Project.TargetContracts = params.TargetContracts
	r.Tool.MedusaModule = medusaModuleVersion()
	r.Tool.GoVersion = goVersion()
	r.Tool.PropertiesSHA = params.PropertiesSHA
	return r
}

func (r *Report) fail(status, reason string, err error) {
	r.Status.Technical = status
	r.Status.Reason = reason
	if err != nil {
		r.Status.Error = err.Error()
	}
}

func newRunID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("run-%s-%s", time.Now().UTC().Format("20060102T150405Z"), hex.EncodeToString(b[:]))
}

func medusaModuleVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range bi.Deps {
			if dep.Path == "github.com/crytic/medusa" {
				return dep.Version
			}
		}
	}
	return "(unknown; run with go run/go build for this module)"
}

func goVersion() string { return strings.TrimPrefix(runtime.Version(), "go") }

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read target file %s: %w", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func countFiles(dir string) *int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() {
			n++
		}
	}
	return &n
}

func msSince(t time.Time) *int64 {
	ms := time.Since(t).Milliseconds()
	return &ms
}

func marshalConfig(projectConfig any) (json.RawMessage, error) {
	data, err := json.Marshal(projectConfig)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}
