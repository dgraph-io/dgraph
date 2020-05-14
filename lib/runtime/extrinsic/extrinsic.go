package extrinsic

import (
	"encoding/binary"
	"errors"
	"io"
	"math/big"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/common/optional"
	"github.com/ChainSafe/gossamer/lib/crypto/sr25519"
	"github.com/ChainSafe/gossamer/lib/scale"
)

//nolint
const (
	AuthoritiesChangeType = 0
	TransferType          = 1
	IncludeDataType       = 2
	StorageChangeType     = 3
	// TODO: implement when storage changes trie is completed
	//ChangesTrieConfigUpdateType = 4
)

// Extrinsic represents a runtime Extrinsic
type Extrinsic interface {
	Type() int
	Encode() ([]byte, error)
	Decode(r io.Reader) error
}

// DecodeExtrinsic decodes an Extrinsic from a Reader
func DecodeExtrinsic(r io.Reader) (Extrinsic, error) {
	typ, err := common.ReadByte(r)
	if err != nil {
		return nil, err
	}

	switch typ {
	case AuthoritiesChangeType:
		ext := new(AuthoritiesChangeExt)
		return ext, ext.Decode(r)
	case TransferType:
		ext := new(TransferExt)
		return ext, ext.Decode(r)
	case IncludeDataType:
		ext := new(IncludeDataExt)
		return ext, ext.Decode(r)
	case StorageChangeType:
		ext := new(StorageChangeExt)
		return ext, ext.Decode(r)
	default:
		return nil, errors.New("cannot decode invalid extrinsic type")
	}
}

// AuthoritiesChangeExt represents an Extrinsic::AuthoritiesChange
type AuthoritiesChangeExt struct {
	authorityIDs [][32]byte
}

// NewAuthoritiesChangeExt returns an AuthoritiesChangeExt
func NewAuthoritiesChangeExt(authorityIDs [][32]byte) *AuthoritiesChangeExt {
	return &AuthoritiesChangeExt{
		authorityIDs: authorityIDs,
	}
}

// Type returns AuthoritiesChangeType
func (e *AuthoritiesChangeExt) Type() int {
	return AuthoritiesChangeType
}

// Encode returns the SCALE encoding of the AuthoritiesChangeExt
func (e *AuthoritiesChangeExt) Encode() ([]byte, error) {
	// TODO: scale should work with arrays of [32]byte
	enc, err := scale.Encode(big.NewInt(int64(len(e.authorityIDs))))
	if err != nil {
		return nil, err
	}

	for _, id := range e.authorityIDs {
		enc = append(enc, id[:]...)
	}

	return append([]byte{AuthoritiesChangeType}, enc...), nil
}

// Decode decodes the SCALE encoding into a AuthoritiesChangeExt
func (e *AuthoritiesChangeExt) Decode(r io.Reader) error {
	sd := &scale.Decoder{Reader: r}
	d, err := sd.Decode(e.authorityIDs)
	if err != nil {
		return err
	}

	e.authorityIDs = d.([][32]byte)
	return nil
}

// Transfer represents a runtime Transfer
type Transfer struct {
	from   [32]byte
	to     [32]byte
	amount uint64
	nonce  uint64
}

// NewTransfer returns a Transfer
func NewTransfer(from, to [32]byte, amount, nonce uint64) *Transfer {
	return &Transfer{
		from:   from,
		to:     to,
		amount: amount,
		nonce:  nonce,
	}
}

// Encode returns the SCALE encoding of the Transfer
func (t *Transfer) Encode() ([]byte, error) {
	enc := []byte{}

	buf := make([]byte, 8)

	enc = append(enc, t.from[:]...)
	enc = append(enc, t.to[:]...)

	binary.LittleEndian.PutUint64(buf, t.amount)
	enc = append(enc, buf...)

	binary.LittleEndian.PutUint64(buf, t.nonce)
	enc = append(enc, buf...)

	return enc, nil
}

// Decode decodes the SCALE encoding into a Transfer
func (t *Transfer) Decode(r io.Reader) (err error) {
	t.from, err = common.ReadHash(r)
	if err != nil {
		return err
	}

	t.to, err = common.ReadHash(r)
	if err != nil {
		return err
	}

	t.amount, err = common.ReadUint64(r)
	if err != nil {
		return err
	}

	t.nonce, err = common.ReadUint64(r)
	if err != nil {
		return err
	}

	return nil
}

// AsSignedExtrinsic returns a TransferExt that includes the transfer and a signature.
func (t *Transfer) AsSignedExtrinsic(key *sr25519.PrivateKey) (*TransferExt, error) {
	enc, err := t.Encode()
	if err != nil {
		return nil, err
	}

	sig, err := key.Sign(enc)
	if err != nil {
		return nil, err
	}

	sigb := [64]byte{}
	copy(sigb[:], sig)

	return NewTransferExt(t, sigb), nil
}

// TransferExt represents an Extrinsic::Transfer
type TransferExt struct {
	transfer  *Transfer
	signature [sr25519.SignatureLength]byte
}

// NewTransferExt returns a TransferExt
func NewTransferExt(transfer *Transfer, signature [sr25519.SignatureLength]byte) *TransferExt {
	return &TransferExt{
		transfer:  transfer,
		signature: signature,
	}
}

// Type returns TransferType
func (e *TransferExt) Type() int {
	return TransferType
}

// Encode returns the SCALE encoding of the TransferExt
func (e *TransferExt) Encode() ([]byte, error) {
	enc := []byte{TransferType}

	tenc, err := e.transfer.Encode()
	if err != nil {
		return nil, err
	}

	enc = append(enc, tenc...)
	enc = append(enc, e.signature[:]...)

	return enc, nil
}

// Decode decodes the SCALE encoding into a TransferExt
func (e *TransferExt) Decode(r io.Reader) error {
	e.transfer = new(Transfer)
	err := e.transfer.Decode(r)
	if err != nil {
		return err
	}

	_, err = r.Read(e.signature[:])
	if err != nil {
		return err
	}

	return nil
}

// IncludeDataExt represents an Extrinsic::IncludeData
type IncludeDataExt struct {
	data []byte
}

// NewIncludeDataExt returns a IncludeDataExt
func NewIncludeDataExt(data []byte) *IncludeDataExt {
	return &IncludeDataExt{
		data: data,
	}
}

// Type returns IncludeDataType
func (e *IncludeDataExt) Type() int {
	return IncludeDataType
}

// Encode returns the SCALE encoding of the IncludeDataExt
func (e *IncludeDataExt) Encode() ([]byte, error) {
	enc, err := scale.Encode(e.data)
	if err != nil {
		return nil, err
	}

	return append([]byte{IncludeDataType}, enc...), nil
}

// Decode decodes the SCALE encoding into a IncludeDataExt
func (e *IncludeDataExt) Decode(r io.Reader) error {
	sd := &scale.Decoder{Reader: r}
	d, err := sd.Decode(e.data)
	if err != nil {
		return err
	}

	e.data = d.([]byte)
	return nil
}

// StorageChangeExt represents an Extrinsic::StorageChange
type StorageChangeExt struct {
	key   []byte
	value *optional.Bytes
}

// NewStorageChangeExt returns a StorageChangesExt
func NewStorageChangeExt(key []byte, value *optional.Bytes) *StorageChangeExt {
	return &StorageChangeExt{
		key:   key,
		value: value,
	}
}

// Type returns StorageChangeType
func (e *StorageChangeExt) Type() int {
	return StorageChangeType
}

// Encode returns the SCALE encoding of the StorageChangeExt
func (e *StorageChangeExt) Encode() ([]byte, error) {
	enc := []byte{StorageChangeType}

	d, err := scale.Encode(e.key)
	if err != nil {
		return nil, err
	}

	enc = append(enc, d...)

	if e.value.Exists() {
		enc = append(enc, 1)
		d, err = scale.Encode(e.value.Value())
		if err != nil {
			return nil, err
		}

		enc = append(enc, d...)
	} else {
		enc = append(enc, 0)
	}

	return enc, nil
}

// Decode decodes the SCALE encoding into a StorageChangeExt
func (e *StorageChangeExt) Decode(r io.Reader) error {
	sd := &scale.Decoder{Reader: r}
	d, err := sd.Decode([]byte{})
	if err != nil {
		return err
	}

	e.key = d.([]byte)

	exists, err := common.ReadByte(r)
	if err != nil {
		return err
	}

	if exists == 1 {
		d, err = sd.Decode([]byte{})
		if err != nil {
			return err
		}

		e.value = optional.NewBytes(true, d.([]byte))
	} else {
		e.value = optional.NewBytes(false, nil)
	}

	return nil
}

// Key returns the extrinsic's key
func (e *StorageChangeExt) Key() []byte {
	return e.key
}

// Value returns the extrinsic's value
func (e *StorageChangeExt) Value() *optional.Bytes {
	return e.value
}
