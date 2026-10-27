package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestBuildProjectConfigTargetContractsBalances(t *testing.T) {
	for _, tt := range []struct {
		name     string
		balances []string
		wantErr  bool
	}{
		{"omitted", nil, false},
		{"empty", []string{}, false},
		{"zero", []string{"0"}, false},
		{"partial", []string{"1000000000000000000000"}, false},
		{"ordered", []string{"1000000000000000000000", "7"}, false},
		{"max uint256", []string{"115792089237316195423570985008687907853269984665640564039457584007913129639935"}, false},
		{"too many", []string{"1", "2", "3"}, true},
		{"negative", []string{"-1"}, true},
		{"invalid", []string{"oops"}, true},
		{"blank", []string{""}, true},
		{"fraction", []string{"1.5"}, true},
		{"overflow", []string{"115792089237316195423570985008687907853269984665640564039457584007913129639936"}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := buildProjectConfig(RunParams{
				TargetPath: "T.sol", TargetContracts: []string{"T", "U"},
				TargetContractsBalances: tt.balances, WorkDir: t.TempDir(),
				TimeoutSec: 5, PropertyTestingEnabled: true,
			})
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected invalid balance configuration to fail")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(cfg)
			if err != nil {
				t.Fatal(err)
			}
			var saved struct {
				Fuzzing struct {
					Balances []string `json:"targetContractsBalances"`
				} `json:"fuzzing"`
			}
			if err := json.Unmarshal(raw, &saved); err != nil {
				t.Fatal(err)
			}
			if len(saved.Fuzzing.Balances) != 0 || len(tt.balances) != 0 {
				if !reflect.DeepEqual(saved.Fuzzing.Balances, tt.balances) {
					t.Fatalf("serialized balances = %v, want %v", saved.Fuzzing.Balances, tt.balances)
				}
			}
		})
	}
}

// buildProjectConfig checks the effective Medusa configuration.
func TestBuildProjectConfig(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "T.sol")
	if err := os.WriteFile(target, []byte("contract T {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	params := RunParams{
		TargetPath:              target,
		TargetContracts:         []string{"T"},
		WorkDir:                 filepath.Join(dir, "wd"),
		Workers:                 2,
		TimeoutSec:              15,
		TestLimit:               100,
		CompileArgs:             []string{"--fake-flag"},
		SolcVersion:             "0.8.25",
		AssertionTestingEnabled: true,
		PropertyTestingEnabled:  true,
	}

	cfg, err := buildProjectConfig(params)
	if err != nil {
		t.Fatalf("buildProjectConfig: %v", err)
	}

	if cfg.Fuzzing.Workers != 2 {
		t.Errorf("Workers = %d, want 2", cfg.Fuzzing.Workers)
	}
	if cfg.Fuzzing.Timeout != 15 {
		t.Errorf("Timeout = %d, want 15", cfg.Fuzzing.Timeout)
	}
	if cfg.Fuzzing.TestLimit != 100 {
		t.Errorf("TestLimit = %d, want 100", cfg.Fuzzing.TestLimit)
	}
	if cfg.Fuzzing.CorpusDirectory != params.corpusDir() {
		t.Errorf("CorpusDirectory = %q, want %q", cfg.Fuzzing.CorpusDirectory, params.corpusDir())
	}
	if !cfg.Fuzzing.CoverageEnabled {
		t.Error("CoverageEnabled must be enabled")
	}
	if !cfg.Fuzzing.Testing.AssertionTesting.Enabled {
		t.Error("AssertionTesting must be enabled")
	}
	if !cfg.Fuzzing.Testing.PropertyTesting.Enabled {
		t.Error("PropertyTesting must be enabled")
	}
	if cfg.Fuzzing.Testing.OptimizationTesting.Enabled {
		t.Error("OptimizationTesting must be disabled")
	}
	if len(cfg.Fuzzing.TargetContracts) != 1 || cfg.Fuzzing.TargetContracts[0] != "T" {
		t.Errorf("TargetContracts = %v", cfg.Fuzzing.TargetContracts)
	}
	if cfg.Slither == nil || cfg.Slither.UseSlither {
		t.Error("Slither must be disabled (UseSlither=false, not nil: nil panics in NewFuzzer)")
	}
	if cfg.Logging.LogDirectory == "" {
		t.Error("LogDirectory must point to the work directory")
	}

	// Platform configuration: exportDirectory, solcVersion, args.
	var platform map[string]any
	if err := json.Unmarshal(*cfg.Compilation.PlatformConfig, &platform); err != nil {
		t.Fatalf("parse PlatformConfig: %v", err)
	}
	if platform["exportDirectory"] != filepath.Join(params.WorkDir, "crytic-export") {
		t.Errorf("exportDirectory = %v", platform["exportDirectory"])
	}
	if platform["solcVersion"] != "0.8.25" {
		t.Errorf("solcVersion = %v", platform["solcVersion"])
	}
	args, _ := platform["args"].([]any)
	if len(args) != 1 || args[0] != "--fake-flag" {
		t.Errorf("args = %v", platform["args"])
	}
	if len(args) == 1 && args[0] == "--foundry-compile-all" {
		t.Error("args must not contain --foundry-compile-all (this is not a Foundry project)")
	}
}

// TestAllContracts without an explicit contract list must not break config construction.
func TestBuildProjectConfigAllContracts(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "T.sol")
	if err := os.WriteFile(target, []byte("contract T {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	params := RunParams{
		TargetPath:              target,
		TestAllContracts:        true,
		WorkDir:                 dir,
		TimeoutSec:              5,
		AssertionTestingEnabled: true,
	}
	cfg, err := buildProjectConfig(params)
	if err != nil {
		t.Fatalf("buildProjectConfig: %v", err)
	}
	if !cfg.Fuzzing.Testing.TestAllContracts {
		t.Error("TestAllContracts must be enabled")
	}
	if len(cfg.Fuzzing.TargetContracts) != 0 {
		t.Errorf("TargetContracts must be empty: %v", cfg.Fuzzing.TargetContracts)
	}
}

func TestInferStopReason(t *testing.T) {
	tests := []struct {
		name      string
		timeout   uint64
		testLimit uint64
		failed    int
		elapsedMs int64
		want      string
	}{
		{"timeout", 10, 0, 0, 9500, ReasonTimeout},
		{"test failure before timeout", 60, 0, 1, 5000, ReasonFailedTestStop},
		{"test limit", 0, 100, 0, 5000, ReasonTestLimit},
		{"failure with a test limit", 60, 100, 1, 1000, ReasonFailedTestStop},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Report{}
			r.TestResults.Summary.Failed = tt.failed
			params := RunParams{TimeoutSec: tt.timeout, TestLimit: tt.testLimit}
			campaignStart := time.Now().Add(-time.Duration(tt.elapsedMs) * time.Millisecond)
			got := inferStopReason(r, params, campaignStart)
			if got != tt.want {
				t.Errorf("inferStopReason = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestKindByID(t *testing.T) {
	cases := map[string]string{
		"ASSERTION-T-f":    "assertion",
		"PROPERTY-T-f":     "property",
		"OPTIMIZATION-T-f": "optimization",
		"CUSTOM":           "custom",
	}
	for id, want := range cases {
		if got := kindByID(id); got != want {
			t.Errorf("kindByID(%q) = %q, want %q", id, got, want)
		}
	}
}
