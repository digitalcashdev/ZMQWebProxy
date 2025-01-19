package zmqwebproxy

import (
	"bytes"
	"fmt"

	"github.com/dashpay/dashd-go/wire"
)

// Block represents the RPC/JSON-style of a wire.MsgBlock
type Block struct {
	Raw               Base64RFCString `json:"raw,omitempty"`
	Hash              string          `json:"hash"`
	Version           uint32          `json:"version"`
	VersionHex        string          `json:"versionHex"`
	MerkleRoot        string          `json:"merkleroot"`
	Time              int             `json:"time"`
	MedianTime        int             `json:"mediantime"`
	Nonce             uint            `json:"nonce"`
	Bits              string          `json:"bits"`
	Difficulty        float64         `json:"difficulty"`
	ChainWork         string          `json:"chainwork"`
	Ntx               int             `json:"nTx"`
	PreviousBlockHash string          `json:"previousblockhash"`
	ChainLock         bool            `json:"chainlock"`
	Size              int             `json:"size"`
	Tx                []string        `json:"tx"`
	// CbTx              CbTx            `json:"cbTx"`
}

// // Block represents the RPC/JSON-style of a wire.MsgBlock
// // CbTx represents the coinbase transaction in the desired JSON format.
// type CbTx struct {
// 	Version           uint32  `json:"version"`
// 	Height            int32   `json:"height"`
// 	MerkleRootMNList  string  `json:"merkleRootMNList"`
// 	MerkleRootQuorums string  `json:"merkleRootQuorums"`
// 	BestCLHeightDiff  int32   `json:"bestCLHeightDiff"`
// 	BestCLSignature   string  `json:"bestCLSignature"`
// 	CreditPoolBalance float64 `json:"creditPoolBalance"`
// }

func ParseBlock(rawBlock []byte) (*Block, error) {
	msgBlock := &wire.MsgBlock{}
	r := bytes.NewReader(rawBlock)
	if err := msgBlock.Deserialize(r); err != nil {
		return nil, err
	}

	block, err := WireMsgBlockToBlock(msgBlock)
	if err != nil {
		return nil, err
	}

	return block, nil
}

func WireMsgBlockToBlock(msgBlock *wire.MsgBlock) (*Block, error) {
	block := &Block{
		Version:    uint32(msgBlock.Header.Version),
		VersionHex: fmt.Sprintf("%08x", msgBlock.Header.Version),
		MerkleRoot: msgBlock.Header.MerkleRoot.String(),
		Time:       int(msgBlock.Header.Timestamp.Unix()),
		Nonce:      uint(msgBlock.Header.Nonce),
		Bits:       fmt.Sprintf("%08x", msgBlock.Header.Bits),
		Ntx:        len(msgBlock.Transactions),
		Size:       msgBlock.SerializeSizeStripped(),
		Tx:         make([]string, 0, len(msgBlock.Transactions)),
	}

	// block.ChainWork = chainWork.String()

	// Calculate difficulty
	// targetDifficulty := blockchain.CompactToBig(msgBlock.Header.Bits)
	// difficulty := float64(chaincfg.MainNetParams.PowLimitBits) / targetDifficulty.Float64()
	// block.Difficulty = difficulty

	return block, nil
}
