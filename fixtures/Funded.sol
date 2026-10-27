// SPDX-License-Identifier: MIT
pragma solidity ^0.8.25;

contract Funded {
    constructor() payable {
        require(msg.value == 1_000 ether, "expected initial ETH");
    }

    function ping() external pure {}

    function property_initialETH() external view returns (bool) {
        return address(this).balance >= 1_000 ether;
    }
}
