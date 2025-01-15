package main

import (
	"bytes"

	"github.com/DigitalCashDev/zmqproxy"
	"github.com/dashpay/dashd-go/wire"
)

func parseTx(rawTx []byte) (*zmqproxy.Tx, error) {
	msgTx := &wire.MsgTx{}
	r := bytes.NewReader(rawTx)
	err := msgTx.Deserialize(r)
	if err != nil {
		return nil, err
	}

	tx, err := zmqproxy.MsgTxToTx(msgTx)
	if err != nil {
		return nil, err
	}

	return tx, nil
}
