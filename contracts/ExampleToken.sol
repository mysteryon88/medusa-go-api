// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

import "properties/lib/openzeppelin-contracts/contracts/token/ERC20/ERC20.sol";
import "properties/lib/openzeppelin-contracts/contracts/token/ERC20/extensions/ERC20Burnable.sol";
import "properties/lib/openzeppelin-contracts/contracts/security/Pausable.sol";
import "properties/lib/openzeppelin-contracts/contracts/access/Ownable.sol";

contract ExampleToken is ERC20, ERC20Burnable, Pausable, Ownable {
    constructor() ERC20("Example token", "EXT") {}

    function transfer(
        address to,
        uint256 amount
    ) public override returns (bool) {
        uint fee = 1000;
        return super.transfer(to, amount - fee);
    }

    function pause() public onlyOwner {
        _pause();
    }

    function unpause() public onlyOwner {
        _unpause();
    }

    function mint(address to, uint256 amount) public virtual onlyOwner {
        _mint(to, amount);
    }
}
