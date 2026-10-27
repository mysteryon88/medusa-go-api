// SPDX-License-Identifier: MIT
pragma solidity ^0.8.25;

// Demonstrates a failing check with a technically successful run:
// property_nonZero is violated by a call to set(0).
contract PricedStorage {
    uint256 private storedValue;

    event ValueChanged(uint256 newValue);

    function set(uint256 value) public {
        storedValue = value;
        emit ValueChanged(value);
    }

    function get() public view returns (uint256) {
        return storedValue;
    }

    function property_nonZero() external returns (bool) {
        return storedValue != 0;
    }
}
