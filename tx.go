package zmqwebproxy

import (
	"bytes"
	"math"

	"github.com/dashpay/dashd-go/chaincfg/chainhash"
	"github.com/dashpay/dashd-go/wire"
)

func ParseTx(rawTx []byte) (*Tx, error) {
	msgTx := &wire.MsgTx{}
	r := bytes.NewReader(rawTx)
	err := msgTx.Deserialize(r)
	if err != nil {
		return nil, err
	}

	tx, err := WireMsgTxToTx(msgTx)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

// Tx maps to the JSON transaction format.
type Tx struct {
	Raw       Base64RFCString `json:"raw,omitempty"`
	Version   int             `json:"version"`
	TxVersion int             `json:"dashVersion"`
	TxType    int             `json:"dashType"`
	Vin       []Vin           `json:"vin"`
	Vout      []Vout          `json:"vout"`
	Locktime  uint32          `json:"locktime"`
}

// Vin represents an input transaction.
type Vin struct {
	TxID      string `json:"txid"`
	Index     int    `json:"index"`
	ScriptSig Script `json:"scriptSig"`
	Sequence  uint32 `json:"sequence"`
}

// Vout represents an output transaction.
type Vout struct {
	Value        float64 `json:"value"`
	N            int     `json:"n"`
	ValueSat     int64   `json:"valueSat"`
	ScriptPubKey Script  `json:"scriptPubKey"`
}

// Script holds the hex representation of a script.
type Script struct {
	Hex HexString `json:"hex"`
}

// WireMsgTxToTx converts a wire.MsgTx to our custom Tx struct.
func WireMsgTxToTx(msgTx *wire.MsgTx) (*Tx, error) {
	tx := &Tx{
		Version:   int(msgTx.Version),
		TxVersion: int(msgTx.Version) & 0xFFFF,
		TxType:    int(msgTx.Version) >> 16 & 0xFFFF, // Assuming extension is always 5 as per your example
		Vin:       make([]Vin, len(msgTx.TxIn)),
		Vout:      make([]Vout, len(msgTx.TxOut)),
		Locktime:  msgTx.LockTime,
	}

	for i, txIn := range msgTx.TxIn {
		txIDHash := chainhash.Hash(txIn.PreviousOutPoint.Hash)
		txID := txIDHash.String()
		scriptSig := Script{Hex: HexString(txIn.SignatureScript)}
		vin := Vin{
			TxID:      txID,
			Index:     int(txIn.PreviousOutPoint.Index),
			ScriptSig: scriptSig,
			Sequence:  txIn.Sequence,
		}
		tx.Vin[i] = vin
	}

	for i, txOut := range msgTx.TxOut {
		valueSat := txOut.Value
		value := float64(valueSat) / math.Pow10(8)
		scriptPubKey := Script{Hex: HexString(txOut.PkScript)}
		vout := Vout{
			Value:        value,
			N:            i,
			ValueSat:     valueSat,
			ScriptPubKey: scriptPubKey,
		}
		tx.Vout[i] = vout
	}

	return tx, nil
}
