// SPDX-License-Identifier: MIT
pragma solidity ^0.8.25;

// Compile-error fixture: the semicolon is intentionally missing.
contract CompileError {
    uint256 public value

    function set(uint256 v) external {
        value = v;
    }
}
