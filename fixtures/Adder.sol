// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

// A valid teaching contract for positive wrapper smoke checks.
// property_sumCoversCount is always true: each add call increases sum
// by at least 1 and count by exactly 1 (uint256 overflow is unreachable:
// x <= 255, requiring roughly 2^248 calls).
contract Adder {
    uint256 public sum;
    uint256 public count;

    event Added(uint256 x, uint256 newSum, uint256 newCount);

    function add(uint8 seed) external {
        uint256 x = (uint256(seed) % 255) + 1;
        sum += x;
        count++;
        emit Added(x, sum, count);
    }

    function property_sumCoversCount() external returns (bool) {
        return sum >= count;
    }
}
