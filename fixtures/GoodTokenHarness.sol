// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

import "./GoodToken.sol";
import {ITokenMock} from "properties/contracts/ERC20/external/util/ITokenMock.sol";
import {CryticERC20ExternalBasicProperties} from "properties/contracts/ERC20/external/properties/ERC20ExternalBasicProperties.sol";

// Harness for the valid ERC20 fixture used by external crytic/properties checks
// (similar to contracts/CryticERC20ExternalHarness.sol, but with a contract
// that has no intentional violations).
contract GoodTokenHarness is CryticERC20ExternalBasicProperties {
    constructor() {
        token = ITokenMock(address(new GoodToken()));
    }
}
