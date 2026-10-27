package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SchemaVersion is the report schema version.
const SchemaVersion = 1

// Technical run statuses (separate from check results: a run that completed
// successfully may contain FAILED tests).
const (
	StatusCompleted = "completed" // campaign completed by limit or normally
	StatusCancelled = "cancelled" // cancelled by the caller (context)
	StatusError     = "error"     // technical error: configuration, compilation, or run
)

// Completion reasons.
const (
	ReasonTimeout           = "timeout"    // Fuzzing.Timeout elapsed
	ReasonTestLimit         = "test-limit" // Fuzzing.TestLimit reached
	ReasonFailedTestStop    = "stop-on-failed-test"
	ReasonCallerCancel      = "caller-cancel"
	ReasonNoTests           = "no-tests"      // StopOnNoTests: no checks found
	ReasonCompileError      = "compile-error" // target compilation error
	ReasonConfigError       = "config-error"  // parameter/configuration error
	ReasonInternalError     = "internal-error"
	ReasonStoppedAbnormally = "stopped-abnormally" // Start did not return after Stop/Terminate
)

// TestCaseResult is a copy of Medusa test-case data (Medusa's internal objects
// are mutable and are not exposed).
type TestCaseResult struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"` // assertion | property | optimization | custom (by ID prefix)
	Name         string `json:"name"`
	Status       string `json:"status"` // NOT STARTED | RUNNING | PASSED | FAILED
	Message      string `json:"message,omitempty"`
	CallSequence string `json:"call_sequence,omitempty"` // trace of the violating sequence
}

// TestSummary summarizes test-case statuses.
type TestSummary struct {
	Total      int `json:"total"`
	Passed     int `json:"passed"`
	Failed     int `json:"failed"`
	NotStarted int `json:"not_started"`
	Running    int `json:"running"`
}

// Metrics contains only metrics that are actually available. Unavailable
// metrics are represented by nil (not zero) so zero is not mistaken for a measurement.
type Metrics struct {
	// TestCaseCounts is available through the public API (fuzzing.Fuzzer.TestCases).
	TestCaseCounts *TestSummary `json:"test_case_counts"`
	// CorpusFilesCallSequences/TestResults count corpus files on disk after the
	// run (the call_sequences/ and test_results/ directories).
	CorpusFilesCallSequences *int `json:"corpus_files_call_sequences,omitempty"`
	CorpusFilesTestResults   *int `json:"corpus_files_test_results,omitempty"`
	// WorkersCreated is derived from WorkerCreated events.
	WorkersCreated *int `json:"workers_created,omitempty"`
	// Fuzzer does not expose CallsTested, SequencesTested, GasUsed, or BranchesHit
	// (the metrics field is private); this limitation is recorded explicitly.
	CallsTested      *uint64 `json:"calls_tested"`
	SequencesTested  *uint64 `json:"sequences_tested"`
	GasUsed          *uint64 `json:"gas_used"`
	BranchesHit      *uint64 `json:"branches_hit"`
	MetricsAvailable bool    `json:"metrics_available"`
	MetricsNote      string  `json:"metrics_note,omitempty"`
}

// Artifact is an item in the run artifact list.
type Artifact struct {
	Type string `json:"type"` // config | corpus | logs | coverage | compile-export | report
	Path string `json:"path"`
}

// PhaseTiming contains the durations of the main phases in milliseconds; null means the phase did not run.
type PhaseTiming struct {
	PreparationMs *int64 `json:"preparation_ms"`
	CampaignMs    *int64 `json:"campaign_ms"`
	TeardownMs    *int64 `json:"teardown_ms"`
}

// Report is a versioned structured run report (schema v1).
type Report struct {
	SchemaVersion int       `json:"schema_version"`
	RunID         string    `json:"run_id"`
	GeneratedAt   time.Time `json:"generated_at"`

	Tool struct {
		MedusaModule  string `json:"medusa_module"` // github.com/crytic/medusa version
		GoVersion     string `json:"go_version"`
		PropertiesSHA string `json:"properties_sha,omitempty"`
	} `json:"tool"`

	Project struct {
		TargetPath      string   `json:"target_path"`
		TargetSHA256    string   `json:"target_sha256"`
		TargetContracts []string `json:"target_contracts"`
	} `json:"project"`

	// EffectiveConfig is the Medusa configuration that was actually used
	// (the config.ProjectConfig structure serialized as is).
	EffectiveConfig json.RawMessage `json:"effective_config"`

	Timing struct {
		StartedAt  time.Time   `json:"started_at"`
		FinishedAt time.Time   `json:"finished_at"`
		Phases     PhaseTiming `json:"phases"`
	} `json:"timing"`

	Status struct {
		Technical string `json:"technical"` // completed | cancelled | error
		Reason    string `json:"reason"`
		Error     string `json:"error,omitempty"`
	} `json:"status"`

	TestResults struct {
		Summary TestSummary      `json:"summary"`
		Items   []TestCaseResult `json:"items"`
	} `json:"test_results"`

	Metrics   Metrics    `json:"metrics"`
	Warnings  []string   `json:"warnings"`
	Artifacts []Artifact `json:"artifacts"`
}

// WriteReport serializes the report to <workdir>/report.json.
func (r *Report) WriteReport(workDir string) (string, error) {
	path := filepath.Join(workDir, "report.json")
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", fmt.Errorf("serialize report: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create report directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write report %s: %w", path, err)
	}
	return path, nil
}

// ParseReport reads a report from a file (for tools, agents, and tests).
func ParseReport(path string) (*Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read report %s: %w", path, err)
	}
	var r Report
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse report %s: %w", path, err)
	}
	return &r, nil
}
