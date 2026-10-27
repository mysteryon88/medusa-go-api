package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunValidationWritesReport(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "T.sol")
	if err := os.WriteFile(target, []byte("contract T {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	workDir := filepath.Join(dir, "runs", "invalid")
	report, err := Run(context.Background(), RunParams{
		TargetPath: target, TargetContracts: []string{"T"},
		WorkDir: workDir, PropertyTestingEnabled: true,
	})
	if err == nil || !strings.Contains(err.Error(), "at least one campaign limit") {
		t.Fatalf("expected a campaign limit validation error, got %v", err)
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		t.Fatalf("validation error contains a filesystem error: %v", err)
	}
	saved, readErr := ParseReport(filepath.Join(workDir, "report.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if saved.Status.Technical != StatusError || saved.Status.Reason != ReasonConfigError || saved.Status.Error != err.Error() || saved.RunID != report.RunID {
		t.Fatalf("unexpected validation report: %+v", saved)
	}
}

func TestFinalizePersistsReturnedReport(t *testing.T) {
	dir := t.TempDir()
	report := newReport(&RunParams{})
	report.Status.Technical = StatusCompleted
	report.Status.Reason = ReasonTimeout
	got, err := finalize(context.Background(), report, RunParams{WorkDir: dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := ParseReport(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	savedJSON, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(savedJSON, wantJSON) {
		t.Fatal("saved report differs from returned report")
	}
}

func TestFinalizeWriteFailure(t *testing.T) {
	for _, tt := range []struct {
		name   string
		prior  error
		status string
		reason string
	}{
		{"completed", nil, StatusError, ReasonInternalError},
		{"cancelled", context.Canceled, StatusCancelled, ReasonCallerCancel},
		{"config error", errors.New("invalid target"), StatusError, ReasonConfigError},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, "report.json"), 0o755); err != nil {
				t.Fatal(err)
			}
			report := newReport(&RunParams{})
			report.Status.Technical = StatusCompleted
			if tt.prior != nil {
				report.fail(tt.status, tt.reason, tt.prior)
			}
			got, err := finalize(context.Background(), report, RunParams{WorkDir: dir}, tt.prior)
			var pathErr *os.PathError
			if !errors.As(err, &pathErr) {
				t.Fatalf("missing report write error: %v", err)
			}
			if tt.prior != nil && !errors.Is(err, tt.prior) {
				t.Fatalf("lost previous error: %v", err)
			}
			if got.Status.Technical != tt.status || got.Status.Reason != tt.reason || got.Status.Error == "" {
				t.Fatalf("write failure status: %+v", got.Status)
			}
			for _, artifact := range got.Artifacts {
				if artifact.Type == "report" {
					t.Fatal("failed report advertised as an artifact")
				}
			}
		})
	}
}

func TestReportRoundTrip(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC().Round(0)

	// Prepare metric counters: available values and unavailable ones (null).
	seqs, res := 5, 2
	wc := 3
	passed, failed := 4, 1
	rep := &Report{
		SchemaVersion: SchemaVersion,
		RunID:         "run-test",
		GeneratedAt:   now,
	}
	rep.Tool.MedusaModule = "v1.5.1"
	rep.Tool.GoVersion = "1.26.4"
	rep.Tool.PropertiesSHA = "9b91ad9569cb3064b7e4b252a266c2ee1d8e9c7f"
	rep.Project.TargetPath = "fixtures/Adder.sol"
	rep.Project.TargetSHA256 = strings.Repeat("ab", 32)
	rep.Project.TargetContracts = []string{"Adder"}
	rep.EffectiveConfig = json.RawMessage(`{"fuzzing":{"workers":2}}`)
	rep.Timing.StartedAt = now
	rep.Timing.FinishedAt = now.Add(30 * time.Second)
	prep, camp, tear := int64(1500), int64(20000), int64(20)
	rep.Timing.Phases = PhaseTiming{PreparationMs: &prep, CampaignMs: &camp, TeardownMs: &tear}
	rep.Status.Technical = StatusCompleted
	rep.Status.Reason = ReasonTimeout
	rep.TestResults.Summary = TestSummary{Total: 5, Passed: passed, Failed: failed, NotStarted: 0}
	rep.TestResults.Items = []TestCaseResult{{
		ID: "PROPERTY-Adder-property_sumCoversCount()", Kind: "property",
		Name: "Property Test: Adder.property_sumCoversCount()", Status: "PASSED",
	}}
	rep.Metrics.TestCaseCounts = &rep.TestResults.Summary
	rep.Metrics.CorpusFilesCallSequences = &seqs
	rep.Metrics.CorpusFilesTestResults = &res
	rep.Metrics.WorkersCreated = &wc
	rep.Metrics.MetricsAvailable = false
	rep.Warnings = []string{"w1"}
	rep.Artifacts = []Artifact{{Type: "report", Path: "x/report.json"}}

	path, err := rep.WriteReport(dir)
	if err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	if !strings.HasSuffix(path, "report.json") {
		t.Errorf("report path: %s", path)
	}

	got, err := ParseReport(path)
	if err != nil {
		t.Fatalf("ParseReport: %v", err)
	}
	if got.RunID != rep.RunID || got.SchemaVersion != SchemaVersion {
		t.Errorf("RunID/SchemaVersion: %q/%d", got.RunID, got.SchemaVersion)
	}
	if got.Status.Technical != StatusCompleted || got.Status.Reason != ReasonTimeout {
		t.Errorf("status: %+v", got.Status)
	}
	if got.TestResults.Summary.Total != 5 || got.TestResults.Summary.Failed != 1 {
		t.Errorf("summary: %+v", got.TestResults.Summary)
	}
	// Unavailable metrics must serialize as null, not 0.
	if got.Metrics.CallsTested != nil || got.Metrics.GasUsed != nil {
		t.Errorf("unavailable metrics must be null: %+v", got.Metrics)
	}
	if got.Metrics.CorpusFilesCallSequences == nil || *got.Metrics.CorpusFilesCallSequences != 5 {
		t.Error("corpus_files_call_sequences must be 5")
	}
	if got.Timing.Phases.CampaignMs == nil || *got.Timing.Phases.CampaignMs != 20000 {
		t.Error("campaign_ms must be 20000")
	}
}

func TestReportNullMetricsJson(t *testing.T) {
	// Synthetic check of wrapper statuses: technical status and check results
	// remain separate in the JSON representation.
	rep := newReport(&RunParams{})
	rep.fail(StatusError, ReasonNoTests, errTest("no assertion, property, optimization, or custom tests were found to fuzz"))
	rep.TestResults.Summary = TestSummary{Total: 0}

	data, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	status := raw["status"].(map[string]any)
	if status["technical"] != StatusError || status["reason"] != ReasonNoTests {
		t.Errorf("status: %v", status)
	}
	metrics := raw["metrics"].(map[string]any)
	for _, k := range []string{"calls_tested", "sequences_tested", "gas_used", "branches_hit"} {
		if v, ok := metrics[k]; !ok || v != nil {
			t.Errorf("metrics.%s must be null, got %v", k, v)
		}
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }

// A RunID from params takes priority over autogeneration (worker identifier
// chain: report.run_id == run directory name == protocol run_id).
func TestNewReportRunIDFromParams(t *testing.T) {
	withID := newReport(&RunParams{RunID: "task-abc123"})
	if withID.RunID != "task-abc123" {
		t.Errorf("RunID: %q, want task-abc123", withID.RunID)
	}
	withoutID := newReport(&RunParams{})
	if !strings.HasPrefix(withoutID.RunID, "run-") {
		t.Errorf("autogenerated RunID: %q", withoutID.RunID)
	}
}

func TestParseReportMissing(t *testing.T) {
	if _, err := ParseReport(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Error("expected an error when reading a nonexistent report")
	}
}
