// Package runner is a small reusable layer for running Medusa fuzzing
// campaigns through its public Go API: an isolated workspace, duration limits,
// programmatic cancellation, and a structured report.
package runner

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"
)

// ProgressEvent is a lightweight campaign progress event (for callbacks, without queues).
type ProgressEvent struct {
	RunID string    `json:"run_id"`
	Type  string    `json:"type"` // fuzzer-starting | fuzzer-stopping | worker-created | worker-destroyed
	Time  time.Time `json:"time"`
}

// RunParams contains the parameters for one campaign run.
type RunParams struct {
	// TargetPath is the contract file path (relative to the process CWD) to
	// compile through crytic-compile (standalone solc mode).
	TargetPath string
	// TargetContracts lists the target contract names (contracts deployed to
	// the test chain and checked). The list must not be empty: medusa can infer
	// the target only when exactly one contract is compiled; the wrapper requires
	// an explicit list for determinism (TestAllContracts only expands the set of
	// contracts to CHECK).
	TargetContracts []string
	// TargetContractsBalances specifies initial ETH in wei as decimal uint256
	// strings in TargetContracts order. Omitted trailing balances default to 0.
	// ETH is sent during deployment; the constructor must be payable.
	TargetContractsBalances []string
	// TestAllContracts enables tests for all compiled contracts.
	TestAllContracts bool

	// Workers is the number of workers; 0 means the default value (2).
	Workers int
	// TimeoutSec is the campaign duration limit in seconds (counted from Start,
	// after compilation). 0 means no limit, but then TestLimit must be set.
	TimeoutSec uint64
	// TestLimit is the limit on the number of tested CALLS/transactions (not
	// sequences; see medusa FuzzingConfig.TestLimit). 0 means no limit.
	TestLimit uint64

	// WorkDir is the required run directory, where config.json, corpus/, logs/,
	// crytic-export/, and report.json are created.
	WorkDir string
	// CorpusReuseDir is an optional existing corpus directory to reuse (instead
	// of WorkDir/corpus).
	CorpusReuseDir string

	// CompileArgs contains additional crytic-compile arguments.
	CompileArgs []string
	// SolcVersion is the solc version for crytic-compile (empty = solc from PATH).
	SolcVersion string

	// AssertionTestingEnabled enables assertion checks (enabled by default in
	// medusa; fail-on-assert mode).
	AssertionTestingEnabled bool
	// PropertyTestingEnabled enables property_* checks.
	PropertyTestingEnabled bool
	// OptimizationTestingEnabled enables optimize_* checks.
	OptimizationTestingEnabled bool

	// GracePeriod is how long to wait for completion after Stop() before Terminate().
	// 0 means the default value (30 seconds).
	GracePeriod time.Duration

	// OnEvent is an optional progress callback. It should be lightweight because
	// it is called synchronously by Medusa goroutines.
	OnEvent func(ProgressEvent)

	// PropertiesSHA is version information for the crytic/properties submodule
	// (set by the caller and included in the report).
	PropertiesSHA string
}

const defaultWorkers = 2

func (p *RunParams) workers() int {
	if p.Workers <= 0 {
		return defaultWorkers
	}
	return p.Workers
}

func (p *RunParams) gracePeriod() time.Duration {
	if p.GracePeriod <= 0 {
		return 30 * time.Second
	}
	return p.GracePeriod
}

func (p *RunParams) corpusDir() string {
	if p.CorpusReuseDir != "" {
		return p.CorpusReuseDir
	}
	return filepath.Join(p.WorkDir, "corpus")
}

// Validate checks parameters before any calls to Medusa.
// It returns a specific error message (unlike the old main.go, which silently
// discarded errors with a return).
func (p *RunParams) Validate() error {
	var errs []error
	if p.TargetPath == "" {
		errs = append(errs, errors.New("TargetPath is not set"))
	} else if _, err := os.Stat(p.TargetPath); err != nil {
		errs = append(errs, fmt.Errorf("target file is unavailable: %q: %w", p.TargetPath, err))
	}
	if len(p.TargetContracts) == 0 {
		errs = append(errs, errors.New("TargetContracts is empty: medusa requires at least one contract to deploy to the test chain (TestAllContracts only expands the set being checked)"))
	}
	if p.Workers < 0 || p.Workers > 256 {
		errs = append(errs, fmt.Errorf("worker count is outside the allowed range 0..256: %d", p.Workers))
	}
	if p.TimeoutSec == 0 && p.TestLimit == 0 {
		errs = append(errs, errors.New("neither TimeoutSec nor TestLimit is set: at least one campaign limit is required"))
	}
	if p.TimeoutSec > math.MaxInt32 {
		errs = append(errs, fmt.Errorf("TimeoutSec is too large (>%d seconds)", int64(math.MaxInt32)))
	}
	if p.WorkDir == "" {
		errs = append(errs, errors.New("WorkDir is not set"))
	}
	if p.CorpusReuseDir != "" {
		if info, err := os.Stat(p.CorpusReuseDir); err != nil || !info.IsDir() {
			errs = append(errs, fmt.Errorf("CorpusReuseDir does not exist or is not a directory: %q", p.CorpusReuseDir))
		}
	}
	if !p.AssertionTestingEnabled && !p.PropertyTestingEnabled && !p.OptimizationTestingEnabled {
		errs = append(errs, errors.New("all check modes are disabled: the campaign will find no tests"))
	}
	return errors.Join(errs...)
}
