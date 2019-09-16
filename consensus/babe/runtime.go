package babe

import (
	scale "github.com/ChainSafe/gossamer/codec"
)

// gets the startup data for Babe from the runtime
func (b *BabeSession) startupData() (*BabeConfiguration, error) {
	ret, err := b.rt.Exec("BabeApi_startup_data", 1, 0)
	if err != nil {
		return nil, err
	}

	bc := new(BabeConfiguration)
	_, err = scale.Decode(ret, bc)
	return bc, err
}

// gets the current epoch data from the runtime
func (b *BabeSession) epoch() (*Epoch, error) {
	ret, err := b.rt.Exec("BabeApi_epoch", 1, 0)
	if err != nil {
		return nil, err
	}

	e := new(Epoch)
	_, err = scale.Decode(ret, e)
	return e, err
}
