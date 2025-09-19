package abi

import _ "embed"

// ERC token standard ABIs for decoder tests and runtime helpers.

//go:embed erc20.json
var ERC20 []byte

//go:embed erc721.json
var ERC721 []byte

//go:embed erc1155.json
var ERC1155 []byte
