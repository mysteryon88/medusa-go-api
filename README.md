# medusa-go-api

An example of running limited [Medusa](https://github.com/crytic/medusa) fuzzing campaigns programmatically through its **Go API**.

[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/mysteryon88/medusa-go-api)

Current versions: `github.com/crytic/medusa` **v1.5.1**, [crytic/properties](https://github.com/crytic/properties) submodule **v1.1.0** (`9b91ad9`).

## Features

- reusable `runner` package: parameters → Medusa configuration → bounded campaign → structured JSON report (schema v1);
- an isolated working directory for each run (`config.json`, `corpus/`, `logs/`, `crytic-export/`, `report.json`);
- campaign limits (timeout / number of calls) and programmatic cancellation through `context.Context` (Stop → grace period → Terminate);
- the run's technical status is separate from check results; unavailable metrics are `null`;
- progress events (`run_id`, type, time) through a callback;
- fixtures: a valid invariant (`fixtures/Adder.sol`), a valid ERC20 through properties (`fixtures/GoodTokenHarness.sol`), intentional violations (`contracts/`, `fixtures/PricedStorage.sol`), and errors (`fixtures/CompileError.sol`, `fixtures/Empty.sol`).

## Installation

Requirements:

| Tool | Version | Purpose |
|---|---|---|
| Go | ≥ 1.24 | build |
| solc | 0.8.x (tested with 0.8.25) | compile targets |
| crytic-compile | ≥ 0.3.x (`pip install crytic-compile`) | compilation wrapper used by Medusa |
| Python | for crytic-compile | — |
| Foundry | not required | compilation uses standalone solc mode |

```bash
git clone --recursive https://github.com/mysteryon88/medusa-go-api.git
cd medusa-go-api
go build ./...
go test -short ./...        # unit tests without solc/crytic-compile
go test ./...               # all tests (solc and crytic-compile must be in PATH)
```

## Quick start

Run a campaign from the repository root. On Windows, pass a relative target path without `..`: absolute paths and paths containing `..` break target parsing in crytic-compile.

```bash
go run . -target fixtures/Adder.sol -contracts Adder -workers 2 -timeout 15
```

After the run, the summary is printed to stdout and a structured report is written to `workdir/run-<timestamp>/report.json`. Exit codes: `0` — the campaign completed; `1` — a technical error (configuration/compilation/run); `2` — cancelled.

Other examples:

```bash
# Valid ERC20 fixture through crytic/properties (external property checks)
go run . -target fixtures/GoodTokenHarness.sol -contracts GoodTokenHarness -all-contracts -workers 2 -timeout 20

# Existing demo example with a fee bug: expect 1 FAILED, exit 0
go run . -target contracts/CryticERC20ExternalHarness.sol -contracts CryticERC20ExternalHarness -all-contracts -workers 2 -timeout 25

# Stop at the call limit instead of a timeout (TestLimit counts calls/transactions)
go run . -target fixtures/Adder.sol -contracts Adder -workers 1 -timeout 0 -test-limit 100000

# Reuse the corpus from a previous run
go run . -target fixtures/GoodTokenHarness.sol -contracts GoodTokenHarness -all-contracts \
  -timeout 20 -corpus-reuse "workdir/<previous-run>/corpus"

# Invalid inputs
go run . -target fixtures/CompileError.sol -contracts CompileError -timeout 10   # compile-error
go run . -target fixtures/Empty.sol -contracts Empty -timeout 10                # no-tests
```

Flags: `-target` (required), `-contracts`, `-all-contracts`, `-workers` (default 2), `-timeout` (seconds, default 20), `-test-limit` (call/transaction limit), `-workdir` (run root), `-corpus-reuse`, `-solc` (solc version), `-compile-arg` (crytic-compile arguments), `-grace` (seconds to wait after Stop), `-no-assertion`/`-no-property`/`-optimization`, `-version`.

## Use as a library

```go
report, err := runner.Run(ctx, runner.RunParams{
    TargetPath: "fixtures/Adder.sol", TargetContracts: []string{"Adder"},
    WorkDir: workDir, Workers: 2, TimeoutSec: 15,
    AssertionTestingEnabled: true, PropertyTestingEnabled: true,
    OnEvent: func(ev runner.ProgressEvent) { /* ... */ },
})
// err == nil means the campaign completed; context.Canceled means cancellation; any other error is a technical error.
// report.Status.Technical: completed | cancelled | error; report.Status.Reason: timeout | ...
```

Within one process, parallel campaigns are not supported (Medusa's global `logging.GlobalLogger`); compilation cannot be interrupted through `ctx`. On Windows, log files may remain locked until the process exits.

`govulncheck ./...` reports [GO-2026-4479](https://pkg.go.dev/vuln/GO-2026-4479)
in the transitive dependency `github.com/pion/dtls/v2`. This example runs an in-memory
test chain without starting geth's P2P/NAT or DTLS clients, so `runner.Run` does not
use the affected transport. The v2 dependency has no known fixed version; reassess
this finding if network transport support is added.

To fund the initial deployment with ETH, set `TargetContractsBalances: []string{"1000000000000000000000"}`
in `RunParams` — this is 1000 ETH in wei for the first contract. Balances are specified
as decimal uint256 strings in `TargetContracts` order; omitted trailing values mean 0. ETH is
sent during deployment, so the constructor must be `payable`. The example target is
`fixtures/Funded.sol` (contract `Funded`). This field is available through the Go API; the CLI
uses zero balances.

The `runner` package is a public importable API:
`import "github.com/mysteryon88/medusa-go-api/runner"`.

## License

Unless otherwise stated, this project's original code and documentation are licensed
under the [GNU Affero General Public License v3.0](LICENSE) (`AGPL-3.0-only`).

Solidity example files marked `SPDX-License-Identifier: MIT` are licensed under MIT.

Third-party code, dependencies, and submodules retain their own licenses. In particular:

- [Medusa v1.5.1](https://github.com/crytic/medusa/blob/v1.5.1/LICENSE) is licensed under AGPLv3.
- [crytic/properties v1.1.0](https://github.com/crytic/properties/blob/v1.1.0/LICENSE) is licensed under AGPLv3; its nested dependencies retain their respective licenses.

## Links

- [Medusa](https://github.com/crytic/medusa) · [crytic/properties](https://github.com/crytic/properties)
- [Project Configuration](https://secure-contracts.com/program-analysis/medusa/docs/src/project_configuration/overview.html) · [API Overview](https://secure-contracts.com/program-analysis/medusa/docs/src/api/api_overview.html)
