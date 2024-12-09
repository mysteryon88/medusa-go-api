// SPDX-License-Identifier: MIT
pragma solidity ^0.8.25;

contract Storage {
    uint256 private storedValue;

    event ValueChanged(uint256 newValue);

    function set(uint256 value) public {
        storedValue = value;
        emit ValueChanged(value);
    }

    function get() public view returns (uint256) {
        return storedValue;
    }

    function property_test() external returns (bool) {
        return storedValue != 0;
    }
}
