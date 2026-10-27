package main

import (
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCLICompileArgs(t *testing.T) {
	if os.Getenv("MEDUSA_CLI_TEST") == "1" {
		for i, arg := range os.Args {
			if arg == "--" {
				os.Args = append([]string{"medusa-go-api"}, os.Args[i+1:]...)
				break
			}
		}
		flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
		main()
		return
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "T.sol")
	if err := os.WriteFile(target, []byte("pragma solidity ^0.8.25; contract T {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	workRoot := filepath.Join(dir, "runs")
	cmd := exec.Command(os.Args[0], "-test.run=^TestCLICompileArgs$", "--",
		"-target", target, "-contracts", "T", "-timeout", "1", "-workdir", workRoot,
		"-compile-arg=--solc-args", "-compile-arg=--base-path path with spaces")
	cmd.Env = append(os.Environ(), "MEDUSA_CLI_TEST=1")
	output, _ := cmd.CombinedOutput() // Compilation may fail; the effective config is written first.
	configs, err := filepath.Glob(filepath.Join(workRoot, "*", "config.json"))
	if err != nil || len(configs) != 1 {
		t.Fatalf("config not written: %v; output: %s", err, output)
	}
	raw, err := os.ReadFile(configs[0])
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Compilation struct {
			PlatformConfig struct {
				Args []string `json:"args"`
			} `json:"platformConfig"`
		} `json:"compilation"`
	}
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if want := []string{"--solc-args", "--base-path path with spaces"}; !reflect.DeepEqual(saved.Compilation.PlatformConfig.Args, want) {
		t.Fatalf("compiler args = %q, want %q", saved.Compilation.PlatformConfig.Args, want)
	}
}
