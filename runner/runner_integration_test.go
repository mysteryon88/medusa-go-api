package runner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Integration tests require solc and crytic-compile in PATH.
// They are skipped in -short mode.

// mkWorkDir creates the work directory outside t.TempDir(): Medusa's global
// logger keeps the log file descriptor open (the public API cannot close it),
// so removing TempDir fails on Windows. Cleanup errors are intentionally ignored.
func mkWorkDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "medusa-runner-it-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func repoPath(t *testing.T, parts ...string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller is unavailable")
	}
	// runner/runner_integration_test.go -> repository root
	root := filepath.Dir(filepath.Dir(thisFile))
	// CWD = repository root: the target is compiled with a relative path without
	// ".." (solc canonicalizes ".." to an absolute path, which breaks
	// crytic-compile target parsing on Windows).
	t.Chdir(root)
	return filepath.Join(parts...)
}

func baseParams(t *testing.T, target string, workDir string) RunParams {
	t.Helper()
	return RunParams{
		TargetPath:              target,
		TargetContracts:         []string{contractNameFor(target)},
		WorkDir:                 workDir,
		Workers:                 2,
		TimeoutSec:              15,
		AssertionTestingEnabled: true,
		PropertyTestingEnabled:  true,
	}
}

func contractNameFor(target string) string {
	base := filepath.Base(target)
	return strings.TrimSuffix(base, ".sol")
}

func TestIntegrationTargetContractsBalances(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires solc/crytic-compile")
	}
	params := baseParams(t, repoPath(t, "fixtures", "Funded.sol"), mkWorkDir(t))
	params.TargetContractsBalances = []string{"1000000000000000000000"}
	params.Workers = 1
	params.TestLimit = 100
	report, err := Run(context.Background(), params)
	if err != nil {
		t.Fatalf("funded deployment failed: %v", err)
	}
	if report.Status.Technical != StatusCompleted || report.TestResults.Summary.Failed != 0 {
		t.Fatalf("funded campaign: status=%+v tests=%+v", report.Status, report.TestResults.Summary)
	}
	for _, tc := range report.TestResults.Items {
		if strings.Contains(tc.Name, "property_initialETH") && tc.Status == "PASSED" {
			return
		}
	}
	t.Fatal("property_initialETH did not pass")
}

// Scenario: input compilation error.
func TestIntegrationCompileError(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires solc/crytic-compile")
	}
	params := baseParams(t, repoPath(t, "fixtures", "CompileError.sol"), mkWorkDir(t))
	report, err := Run(context.Background(), params)
	if err == nil {
		t.Fatalf("expected a technical error, got nil (report: %+v)", report.Status)
	}
	if report == nil || report.Status.Technical != StatusError || report.Status.Reason != ReasonCompileError {
		t.Fatalf("status: %+v", report.Status)
	}
	if !strings.Contains(report.Status.Error, "Error") {
		t.Errorf("error text does not look like a compilation error: %s", report.Status.Error)
	}
	if report.TestResults.Summary.Total != 0 {
		t.Errorf("tests must not run after a compilation error: %+v", report.TestResults.Summary)
	}
}

// Scenario: target with no testable functions (StopOnNoTests).
func TestIntegrationNoTests(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires solc/crytic-compile")
	}
	params := baseParams(t, repoPath(t, "fixtures", "Empty.sol"), mkWorkDir(t))
	report, err := Run(context.Background(), params)
	if err == nil {
		t.Fatal("expected a no-tests error")
	}
	if report.Status.Technical != StatusError || report.Status.Reason != ReasonNoTests {
		t.Fatalf("status: %+v", report.Status)
	}
	if report.TestResults.Summary.Total != 0 {
		t.Errorf("there must be no test cases: %+v", report.TestResults.Summary)
	}
}

// Scenario: valid contract with an invariant, completed by timeout.
func TestIntegrationAdderTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires solc/crytic-compile")
	}
	workDir := mkWorkDir(t)
	params := baseParams(t, repoPath(t, "fixtures", "Adder.sol"), workDir)
	params.TimeoutSec = 8 // short campaign

	report, err := Run(context.Background(), params)
	if err != nil {
		t.Fatalf("expected successful completion, got error: %v", err)
	}
	if report.Status.Technical != StatusCompleted || report.Status.Reason != ReasonTimeout {
		t.Fatalf("status: %+v", report.Status)
	}
	if report.TestResults.Summary.Total == 0 {
		t.Fatal("no test cases were found")
	}
	var propertyFound, assertionFound bool
	for _, tc := range report.TestResults.Items {
		if tc.Kind == "property" && strings.Contains(tc.Name, "property_sumCoversCount") {
			propertyFound = true
			if tc.Status != "PASSED" {
				t.Errorf("property_sumCoversCount should pass: %s", tc.Status)
			}
		}
		if tc.Kind == "assertion" && strings.Contains(tc.Name, "add(") {
			assertionFound = true
		}
	}
	if !propertyFound {
		t.Error("property test property_sumCoversCount was not found")
	}
	if !assertionFound {
		t.Error("assertion test for method add was not found")
	}
	if report.TestResults.Summary.Failed != 0 {
		t.Errorf("the valid fixture must have no FAILED tests: %+v", report.TestResults.Summary)
	}
	if report.Timing.Phases.CampaignMs == nil || *report.Timing.Phases.CampaignMs < 7000 {
		t.Errorf("campaign should run for about 8 seconds: %+v", report.Timing.Phases)
	}
	// The report and artifacts exist.
	if _, err := ParseReport(filepath.Join(workDir, "report.json")); err != nil {
		t.Errorf("report.json: %v", err)
	}
	if report.Metrics.CorpusFilesCallSequences == nil {
		t.Log("warning: corpus is empty (possibly because the campaign was short)")
	}
}

// Scenario: programmatic cancellation during a campaign.
func TestIntegrationCancel(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires solc/crytic-compile")
	}
	params := baseParams(t, repoPath(t, "fixtures", "Adder.sol"), mkWorkDir(t))
	params.TimeoutSec = 60 // high limit; cancel earlier programmatically
	params.GracePeriod = 20 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(3 * time.Second)
		cancel()
	}()
	start := time.Now()
	report, err := Run(ctx, params)
	elapsed := time.Since(start)
	if !errors.Is(err, context.Canceled) {
		if err == nil {
			t.Fatalf("expected cancellation, err=nil, status %+v", report.Status)
		} else {
			t.Logf("warning: err=%v (status %+v)", err, report.Status)
		}
	}
	if report.Status.Technical != StatusCancelled || report.Status.Reason != ReasonCallerCancel {
		t.Fatalf("status: %+v", report.Status)
	}
	if elapsed > 45*time.Second {
		t.Errorf("cancellation should finish within the grace period: %v", elapsed)
	}
	if len(report.Warnings) == 0 {
		t.Error("expected warnings")
	}
}

func TestIntegrationCancelOnStart(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires solc/crytic-compile")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	params := baseParams(t, repoPath(t, "fixtures", "Adder.sol"), mkWorkDir(t))
	params.TimeoutSec = 60
	params.OnEvent = func(event ProgressEvent) {
		if event.Type == "fuzzer-starting" {
			cancel()
		}
	}
	report, err := Run(ctx, params)
	if !errors.Is(err, context.Canceled) || report.Status.Technical != StatusCancelled {
		t.Fatalf("startup cancellation: err=%v status=%+v", err, report.Status)
	}
}

// Scenario: invariant violation — a failed check in a technically successful
// run (status=completed, reason=stop-on-failed-test).
func TestIntegrationBrokenInvariant(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires solc/crytic-compile")
	}
	params := baseParams(t, repoPath(t, "fixtures", "PricedStorage.sol"), mkWorkDir(t))
	params.TimeoutSec = 30

	report, err := Run(context.Background(), params)
	if err != nil {
		t.Fatalf("there should be no technical error: %v", err)
	}
	if report.Status.Technical != StatusCompleted {
		t.Fatalf("status: %+v", report.Status)
	}
	if report.TestResults.Summary.Failed == 0 {
		t.Fatal("expected at least one FAILED test case")
	}
	found := false
	for _, tc := range report.TestResults.Items {
		if tc.Status == "FAILED" && tc.Kind == "property" {
			found = true
			if tc.CallSequence == "" {
				t.Error("a FAILED property test must have a call sequence")
			}
		}
	}
	if !found {
		t.Error("FAILED property test was not found")
	}
	if report.Status.Reason != ReasonFailedTestStop {
		t.Errorf("reason: %q, want stop-on-failed-test", report.Status.Reason)
	}
}
