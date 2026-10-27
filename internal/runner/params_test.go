package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidate(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "T.sol")
	if err := os.WriteFile(target, []byte("contract T {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	corpus := filepath.Join(dir, "corpus")
	if err := os.Mkdir(corpus, 0o755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		params  RunParams
		wantErr string // expected error substring (empty = valid)
	}{
		{
			name: "valid parameters",
			params: RunParams{
				TargetPath: target, TargetContracts: []string{"T"},
				WorkDir: dir, TimeoutSec: 10,
				AssertionTestingEnabled: true, PropertyTestingEnabled: true,
			},
		},
		{
			name: "missing target",
			params: RunParams{
				TargetContracts: []string{"T"}, WorkDir: dir, TimeoutSec: 10,
				AssertionTestingEnabled: true,
			},
			wantErr: "TargetPath is not set",
		},
		{
			name: "target file does not exist",
			params: RunParams{
				TargetPath: filepath.Join(dir, "missing.sol"), TargetContracts: []string{"T"},
				WorkDir: dir, TimeoutSec: 10, AssertionTestingEnabled: true,
			},
			wantErr: "target file is unavailable",
		},
		{
			name: "empty contract list",
			params: RunParams{
				TargetPath: target, WorkDir: dir, TimeoutSec: 10,
				AssertionTestingEnabled: true,
			},
			wantErr: "TargetContracts is empty",
		},
		{
			name: "no campaign limits",
			params: RunParams{
				TargetPath: target, TargetContracts: []string{"T"}, WorkDir: dir,
				AssertionTestingEnabled: true,
			},
			wantErr: "at least one campaign limit",
		},
		{
			name: "missing WorkDir",
			params: RunParams{
				TargetPath: target, TargetContracts: []string{"T"}, TimeoutSec: 10,
				AssertionTestingEnabled: true,
			},
			wantErr: "WorkDir is not set",
		},
		{
			name: "corpus reuse directory does not exist",
			params: RunParams{
				TargetPath: target, TargetContracts: []string{"T"}, WorkDir: dir,
				TimeoutSec: 10, CorpusReuseDir: filepath.Join(dir, "no-corpus"),
				AssertionTestingEnabled: true,
			},
			wantErr: "CorpusReuseDir",
		},
		{
			name: "all check modes are disabled",
			params: RunParams{
				TargetPath: target, TargetContracts: []string{"T"}, WorkDir: dir,
				TimeoutSec: 10,
			},
			wantErr: "all check modes are disabled",
		},
		{
			name: "too many workers",
			params: RunParams{
				TargetPath: target, TargetContracts: []string{"T"}, WorkDir: dir,
				TimeoutSec: 10, Workers: 1000, AssertionTestingEnabled: true,
			},
			wantErr: "worker count is outside the allowed range",
		},
		{
			name: "corpus reuse is valid",
			params: RunParams{
				TargetPath: target, TargetContracts: []string{"T"}, WorkDir: dir,
				TimeoutSec: 10, CorpusReuseDir: corpus,
				AssertionTestingEnabled: true, PropertyTestingEnabled: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.params.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected valid parameters, got error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

func TestDefaults(t *testing.T) {
	p := RunParams{}
	if got := p.workers(); got != defaultWorkers {
		t.Errorf("workers() = %d, want %d", got, defaultWorkers)
	}
	if got := p.gracePeriod(); got != 30*time.Second {
		t.Errorf("gracePeriod() = %v, want 30s", got)
	}
	p.WorkDir = "wd"
	if got := p.corpusDir(); got != filepath.Join("wd", "corpus") {
		t.Errorf("corpusDir() = %q", got)
	}
	p.CorpusReuseDir = "shared"
	if got := p.corpusDir(); got != "shared" {
		t.Errorf("corpusDir() with reuse = %q", got)
	}
}
