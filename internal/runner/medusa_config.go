package runner

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"

	"github.com/crytic/medusa/compilation/types"
	"github.com/crytic/medusa/fuzzing/config"
)

// relativeTarget converts the target path to a form that crytic-compile handles
// on Windows: absolute paths containing "C:" break target parsing
// ("Unknown file: Users:\..."). The path is relative to the process CWD,
// as in the original project example ("contracts/....sol").
//
// Windows limitation: if the path contains "..", solc canonicalizes it to an
// absolute path and crytic-compile target parsing fails. Run the process from
// a directory where the target can be reached without ".." (for example, the repo root:
// "fixtures/Adder.sol").
func relativeTarget(target string) string {
	abs, err := filepath.Abs(target)
	if err != nil {
		return filepath.ToSlash(target)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return filepath.ToSlash(target)
	}
	rel, err := filepath.Rel(cwd, abs)
	if err != nil {
		// Another drive, etc.; leave it as is.
		return filepath.ToSlash(target)
	}
	return filepath.ToSlash(rel)
}

// buildProjectConfig builds config.ProjectConfig from RunParams.
// Serialization and write errors are all checked (the old main.go ignored them).
func buildProjectConfig(params RunParams) (config.ProjectConfig, error) {
	projectConfig, err := config.GetDefaultProjectConfig("crytic-compile")
	if err != nil {
		return config.ProjectConfig{}, fmt.Errorf("get default configuration: %w", err)
	}

	// Compilation target (standalone solc mode through crytic-compile).
	if err := projectConfig.Compilation.SetTarget(relativeTarget(params.TargetPath)); err != nil {
		return config.ProjectConfig{}, fmt.Errorf("set compilation target %q: %w", params.TargetPath, err)
	}

	// crytic-compile platform arguments: exportDirectory isolates compilation
	// artifacts in the work directory (otherwise crytic-export is created in CWD);
	// args are passed through unchanged. --foundry-compile-all is intentionally
	// not used because this is not a Foundry project (there is no foundry.toml).
	platformCfg := map[string]any{}
	if projectConfig.Compilation.PlatformConfig != nil {
		if err := json.Unmarshal(*projectConfig.Compilation.PlatformConfig, &platformCfg); err != nil {
			return config.ProjectConfig{}, fmt.Errorf("parse compilation platform configuration: %w", err)
		}
	}
	platformCfg["exportDirectory"] = filepath.Join(params.WorkDir, "crytic-export")
	if params.SolcVersion != "" {
		platformCfg["solcVersion"] = params.SolcVersion
	}
	platformCfg["args"] = params.CompileArgs
	rawPlatform, err := json.Marshal(platformCfg)
	if err != nil {
		return config.ProjectConfig{}, fmt.Errorf("serialize compilation platform configuration: %w", err)
	}
	if err := projectConfig.Compilation.PlatformConfig.UnmarshalJSON(rawPlatform); err != nil {
		return config.ProjectConfig{}, fmt.Errorf("set compilation platform configuration: %w", err)
	}

	// Fuzzing.
	projectConfig.Fuzzing.Workers = params.workers()
	projectConfig.Fuzzing.Timeout = int(params.TimeoutSec)
	projectConfig.Fuzzing.TestLimit = params.TestLimit
	projectConfig.Fuzzing.CorpusDirectory = params.corpusDir()
	projectConfig.Fuzzing.CoverageEnabled = true
	projectConfig.Fuzzing.CoverageFormats = []string{"lcov"}
	// The default PruneFrequency (5 minutes) does not matter for short runs.

	// Check modes.
	projectConfig.Fuzzing.Testing.AssertionTesting.Enabled = params.AssertionTestingEnabled
	projectConfig.Fuzzing.Testing.PropertyTesting.Enabled = params.PropertyTestingEnabled
	projectConfig.Fuzzing.Testing.OptimizationTesting.Enabled = params.OptimizationTestingEnabled
	projectConfig.Fuzzing.Testing.TestAllContracts = params.TestAllContracts
	if len(params.TargetContracts) > 0 {
		projectConfig.Fuzzing.TargetContracts = params.TargetContracts
	}
	if len(params.TargetContractsBalances) > len(params.TargetContracts) {
		return config.ProjectConfig{}, fmt.Errorf("TargetContractsBalances: more balances than TargetContracts")
	}
	for i, value := range params.TargetContractsBalances {
		balance, ok := new(big.Int).SetString(value, 10)
		if !ok || balance.Sign() < 0 || balance.BitLen() > 256 {
			return config.ProjectConfig{}, fmt.Errorf("TargetContractsBalances[%d]: %q — expected a decimal uint256 amount in wei", i, value)
		}
		projectConfig.Fuzzing.TargetContractsBalances = append(projectConfig.Fuzzing.TargetContractsBalances, &config.ContractBalance{Int: *balance})
	}
	// StopOnFailedTest remains true (the medusa default): the first FAILED test
	// stops the campaign, which is useful for demo fixtures with violations.

	// Slither is not used (it adds an unnecessary stage and artifacts in CWD).
	// IMPORTANT: nil is not allowed here: NewFuzzer/AddCompilationTargets calls
	// Slither.RunSlither on a nil receiver and panics (medusa v1.5.1,
	// compilation/types/slither.go:87), so disable it with an explicit flag.
	projectConfig.Slither = &types.SlitherConfig{UseSlither: false}

	// Medusa logs go in the work directory (log-<ts>.log inside logs/).
	// The global logging.GlobalLogger holds the file handle: on Windows,
	// the file remains locked until the process exits (an upstream limitation).
	projectConfig.Logging.LogDirectory = filepath.Join(params.WorkDir, "logs")

	// Validate early so configuration errors are reported before compilation.
	if err := projectConfig.Validate(); err != nil {
		return config.ProjectConfig{}, fmt.Errorf("invalid configuration: %w", err)
	}
	return *projectConfig, nil
}
