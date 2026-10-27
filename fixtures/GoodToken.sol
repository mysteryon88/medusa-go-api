// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

import {ITokenMock} from "properties/contracts/ERC20/external/util/ITokenMock.sol";

// Valid ERC20 fixture for external crytic/properties checks:
// no transfer fee (unlike the demo ExampleToken), and no mint/burn after
// deployment. The USER1..USER3 and deployer balances match the properties
// constants (PropertiesConstants).
contract GoodToken {
    string public constant name = "Good Token";
    string public constant symbol = "GDT";
    uint8 public constant decimals = 18;

    uint256 public immutable initialSupply;
    bool public constant isMintableOrBurnable = false;

    mapping(address => uint256) private _balances;
    mapping(address => mapping(address => uint256)) private _allowances;

    uint256 private _totalSupply;

    event Transfer(address indexed from, address indexed to, uint256 value);
    event Approval(address indexed owner, address indexed spender, uint256 value);

    constructor() {
        address user1 = address(0x10000);
        address user2 = address(0x20000);
        address user3 = address(0x30000);
        uint256 initial = 1000e18;

        _mint(user1, initial);
        _mint(user2, initial);
        _mint(user3, initial);
        _mint(msg.sender, initial);

        initialSupply = _totalSupply;
    }

    function totalSupply() external view returns (uint256) {
        return _totalSupply;
    }

    function balanceOf(address account) external view returns (uint256) {
        return _balances[account];
    }

    function allowance(address owner, address spender) external view returns (uint256) {
        return _allowances[owner][spender];
    }

    function approve(address spender, uint256 amount) external returns (bool) {
        _allowances[msg.sender][spender] = amount;
        emit Approval(msg.sender, spender, amount);
        return true;
    }

    function transfer(address to, uint256 amount) external returns (bool) {
        _transfer(msg.sender, to, amount);
        return true;
    }

    function transferFrom(address from, address to, uint256 amount) external returns (bool) {
        uint256 allowed = _allowances[from][msg.sender];
        if (allowed != type(uint256).max) {
            require(allowed >= amount, "GoodToken: insufficient allowance");
            _allowances[from][msg.sender] = allowed - amount;
        }
        _transfer(from, to, amount);
        return true;
    }

    function _transfer(address from, address to, uint256 amount) internal {
        require(from != address(0), "GoodToken: transfer from zero address");
        require(to != address(0), "GoodToken: transfer to zero address");
        uint256 balance = _balances[from];
        require(balance >= amount, "GoodToken: insufficient balance");
        unchecked {
            _balances[from] = balance - amount;
            _balances[to] += amount;
        }
        emit Transfer(from, to, amount);
    }

    function _mint(address to, uint256 amount) internal {
        _totalSupply += amount;
        _balances[to] += amount;
        emit Transfer(address(0), to, amount);
    }
}
