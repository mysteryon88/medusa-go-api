# medusa-go-api

- [Previously used, Using v0.1.6](https://github.com/crytic/medusa/issues/429)

Using v0.1.8

Nothing has changed since then. I'm getting an error:

```sh
# github.com/crytic/medusa/chain/config
..\..\..\go\pkg\mod\github.com\crytic\medusa@v0.1.8\chain\config\config.go:35:55: undefined: vm.ConfigExtensions
..\..\..\go\pkg\mod\github.com\crytic\medusa@v0.1.8\chain\config\config.go:43:13: undefined: vm.ConfigExtensions
```

You can fix it by adding this line to `go.mod`

For some reason, using the version package from `go get` doesn't work

```sh
replace github.com/ethereum/go-ethereum => github.com/crytic/medusa-geth v0.0.0-20240708141007-2f7f9258289f
```

And the next command

```sh
go get github.com/ethereum/go-ethereum
```

## References

- [medusa](https://github.com/crytic/medusa)
- [Project Configuration](https://github.com/crytic/medusa/wiki/Project-Configuration)
- [API Overview (WIP)](https://secure-contracts.com/program-analysis/medusa/docs/src/api/api_overview.html)
