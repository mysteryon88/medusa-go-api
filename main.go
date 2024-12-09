package main

import (
	"encoding/json"
	"fmt"

	"github.com/crytic/medusa/fuzzing"
	"github.com/crytic/medusa/fuzzing/config"
)

func main() {
	// Install the compiler and default config
	projectConfig, err := config.GetDefaultProjectConfig("crytic-compile")
	if err != nil {
		return
	}
	// Set the compilation target
	err = projectConfig.Compilation.SetTarget("contracts/CryticERC20ExternalHarness.sol")
	if err != nil {
		return
	}
	// Edit the config
	projectConfig.Fuzzing.Workers = 10
	projectConfig.Fuzzing.CorpusDirectory = "corpus"
	projectConfig.Fuzzing.Testing.TestAllContracts = true
	projectConfig.Fuzzing.TargetContracts = []string{"CryticERC20ExternalHarness"}

	// Disable other test modes according to the example from the repository
	projectConfig.Fuzzing.Testing.PropertyTesting.Enabled = false
	projectConfig.Fuzzing.Testing.OptimizationTesting.Enabled = false
	projectConfig.Fuzzing.Testing.StopOnFailedTest = false

	// Set compilation arguments
	compile_config := setCompileArgs(projectConfig)
	projectConfig.Compilation.PlatformConfig.UnmarshalJSON(compile_config)

	// Saving the config to a file
	projectConfig.WriteToFile("config.json")

	fuzzer, err := fuzzing.NewFuzzer(*projectConfig)
	if err != nil {
		return
	}

	// Start the fuzzer
	err = fuzzer.Start()
	if err != nil {
		return
	}

	// Fetch test cases results
	testCases := fuzzer.TestCases()

	fmt.Println(testCases)
}

func setCompileArgs(projectConfig *config.ProjectConfig) []byte {

	var data map[string]interface{}

	bytes, _ := projectConfig.Compilation.PlatformConfig.MarshalJSON()

	json.Unmarshal(bytes, &data)
	data["args"] = []string{"--foundry-compile-all"}
	updatedJSON, _ := json.MarshalIndent(data, "", "  ")

	return updatedJSON

}
