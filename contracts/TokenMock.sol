import "./ExampleToken.sol";

contract TokenMock is ExampleToken {
    // Address originating transactions in Echidna (must be equal to the `sender` configuration parameter)
    address constant USER1 = address(0x10000);
    address constant USER2 = address(0x20000);
    address constant USER3 = address(0x30000);

    // Initial balance for users' accounts
    uint256 constant INITIAL_BALANCE = 1000e18;

    bool public isMintableOrBurnable;
    uint256 public initialSupply;

    constructor() {
        _mint(USER1, INITIAL_BALANCE);
        _mint(USER2, INITIAL_BALANCE);
        _mint(USER3, INITIAL_BALANCE);
        _mint(msg.sender, INITIAL_BALANCE);

        initialSupply = totalSupply();
        isMintableOrBurnable = true;
    }
}
