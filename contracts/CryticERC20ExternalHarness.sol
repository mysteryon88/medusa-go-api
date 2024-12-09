// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

import "./TokenMock.sol";
import {ITokenMock} from "properties/contracts/ERC20/external/util/ITokenMock.sol";
import {CryticERC20ExternalBasicProperties} from "properties/contracts/ERC20/external/properties/ERC20ExternalBasicProperties.sol";

contract CryticERC20ExternalHarness is CryticERC20ExternalBasicProperties {
    constructor() {
        // Deploy ERC20
        token = ITokenMock(address(new TokenMock()));
    }
}
