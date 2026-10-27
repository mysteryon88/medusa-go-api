// medusa-go-api runs bounded Medusa fuzzing campaigns through the Go API
// with an isolated work directory and a structured report.
package main

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/mysteryon88/medusa-go-api/runner"
)

func main() {
	var (
		target       = flag.String("target", "", "path to the .sol contract file (required)")
		contracts    = flag.String("contracts", "", "comma-separated target contract names")
		allContracts = flag.Bool("all-contracts", false, "test all compiled contracts (TestAllContracts)")
		workers      = flag.Int("workers", 2, "number of Medusa workers (1-2 is enough for a demo)")
		timeoutSec   = flag.Uint64("timeout", 20, "campaign limit in seconds (0 = no limit; then -test-limit is required)")
		testLimit    = flag.Uint64("test-limit", 0, "limit on tested calls/transactions (0 = no limit)")
		workRoot     = flag.String("workdir", "workdir", "run root directory; each run gets a directory inside it")
		corpusReuse  = flag.String("corpus-reuse", "", "existing corpus directory to reuse")
		solc         = flag.String("solc", "", "solc version for crytic-compile (empty = solc from PATH)")
		graceSec     = flag.Int("grace", 30, "seconds to wait for completion after Stop before Terminate")
		noProperty   = flag.Bool("no-property", false, "disable property tests")
		noAssertion  = flag.Bool("no-assertion", false, "disable assertion tests")
		optimization = flag.Bool("optimization", false, "enable optimization tests")
		printVersion = flag.Bool("version", false, "Medusa module version from build info")
	)
	var compileArgs []string
	flag.Func("compile-arg", "additional crytic-compile argument (may be repeated)", func(value string) error {
		compileArgs = append(compileArgs, value)
		return nil
	})
	flag.Parse()
	if *printVersion {
		fmt.Println(medusaVersion())
		return
	}
	if *target == "" {
		fmt.Fprintln(os.Stderr, "error: required flag -target is not set")
		flag.Usage()
		os.Exit(2)
	}

	// Ctrl+C/SIGTERM are translated into context cancellation (the same mechanism
	// as programmatic cancellation through context.CancelFunc in Go code).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runID := fmt.Sprintf("run-%s-%s", time.Now().UTC().Format("20060102T150405Z"), randSuffix())
	workDir := filepath.Join(*workRoot, runID)

	params := runner.RunParams{
		TargetPath:                 *target,
		TargetContracts:            splitNonEmpty(*contracts),
		TestAllContracts:           *allContracts,
		Workers:                    *workers,
		TimeoutSec:                 *timeoutSec,
		TestLimit:                  *testLimit,
		WorkDir:                    workDir,
		CorpusReuseDir:             *corpusReuse,
		CompileArgs:                compileArgs,
		SolcVersion:                *solc,
		AssertionTestingEnabled:    !*noAssertion,
		PropertyTestingEnabled:     !*noProperty,
		OptimizationTestingEnabled: *optimization,
		GracePeriod:                time.Duration(*graceSec) * time.Second,
		OnEvent: func(ev runner.ProgressEvent) {
			fmt.Printf("[event] %s %s\n", ev.Time.Format(time.RFC3339), ev.Type)
		},
		PropertiesSHA: propertiesSHA(),
	}

	report, err := runner.Run(ctx, params)

	fmt.Println("────────── RUN SUMMARY ──────────")
	if report != nil {
		fmt.Printf("run_id:            %s\n", report.RunID)
		fmt.Printf("technical status: %s\n", report.Status.Technical)
		fmt.Printf("reason:           %s\n", report.Status.Reason)
		if report.Status.Error != "" {
			fmt.Printf("error:            %s\n", report.Status.Error)
		}
		fmt.Printf("test cases:       total %d, passed %d, failed %d, not started %d\n",
			report.TestResults.Summary.Total, report.TestResults.Summary.Passed,
			report.TestResults.Summary.Failed, report.TestResults.Summary.NotStarted)
		for _, tc := range report.TestResults.Items {
			if tc.Status == "FAILED" {
				fmt.Printf("  FAILED: %s\n", tc.Name)
			}
		}
		for _, a := range report.Artifacts {
			fmt.Printf("artifact %-14s %s\n", a.Type, a.Path)
		}
	}
	exitCode := 0
	switch {
	case err == nil:
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		fmt.Println("run cancelled")
		exitCode = 2
	default:
		fmt.Printf("technical error: %v\n", err)
		exitCode = 1
	}
	os.Exit(exitCode)
}

// randSuffix returns 4 random bytes as hex so runs started in the same second
// do not collide on directory names.
func randSuffix() string {
	var b [4]byte
	if _, err := crand.Read(b[:]); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(b[:])
}

func splitNonEmpty(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// propertiesSHA reads the crytic/properties submodule SHA for the report.
func propertiesSHA() string {
	out, err := exec.Command("git", "-C", "properties", "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// medusaVersion returns the github.com/crytic/medusa version from build info.
func medusaVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range bi.Deps {
			if dep.Path == "github.com/crytic/medusa" {
				return dep.Version
			}
		}
	}
	return "(unknown)"
}
