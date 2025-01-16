package zmqwebproxy

import (
	"bytes"

	"github.com/dashpay/dashd-go/wire"
)

func parseTx(rawTx []byte) (*Tx, error) {
	msgTx := &wire.MsgTx{}
	r := bytes.NewReader(rawTx)
	err := msgTx.Deserialize(r)
	if err != nil {
		return nil, err
	}

	tx, err := MsgTxToTx(msgTx)
	if err != nil {
		return nil, err
	}

	return tx, nil
}
