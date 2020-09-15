package types

import (
	"github.com/ChainSafe/gossamer/lib/scale"
)

// ScheduledChangeType identifies a ScheduledChange consensus digest
var ScheduledChangeType = byte(1)

// ForcedChangeType identifies a ForcedChange consensus digest
var ForcedChangeType = byte(2)

// OnDisabledType identifies a DisabledChange consensus digest
var OnDisabledType = byte(3)

// PauseType identifies a Pause consensus digest
var PauseType = byte(4)

// ResumeType identifies a Resume consensus digest
var ResumeType = byte(5)

// BABEScheduledChange represents a BABE scheduled authority change
type BABEScheduledChange struct {
	Auths []*AuthorityRaw
	Delay uint32
}

// Encode returns a SCALE encoded BABEScheduledChange with first type byte
func (sc *BABEScheduledChange) Encode() ([]byte, error) {
	d, err := scale.Encode(sc)
	if err != nil {
		return nil, err
	}

	return append([]byte{ScheduledChangeType}, d...), nil
}

// BABEForcedChange represents a BABE forced authority change
type BABEForcedChange struct {
	Auths []*AuthorityRaw
	Delay uint32
}

// Encode returns a SCALE encoded BABEForcedChange with first type byte
func (fc *BABEForcedChange) Encode() ([]byte, error) {
	d, err := scale.Encode(fc)
	if err != nil {
		return nil, err
	}

	return append([]byte{ForcedChangeType}, d...), nil
}

// GrandpaScheduledChange represents a GRANDPA scheduled authority change
type GrandpaScheduledChange struct {
	Auths []*GrandpaAuthorityDataRaw
	Delay uint32
}

// Encode returns a SCALE encoded GrandpaScheduledChange with first type byte
func (sc *GrandpaScheduledChange) Encode() ([]byte, error) {
	d, err := scale.Encode(sc)
	if err != nil {
		return nil, err
	}

	return append([]byte{ScheduledChangeType}, d...), nil
}

// GrandpaForcedChange represents a GRANDPA forced authority change
type GrandpaForcedChange struct {
	Auths []*GrandpaAuthorityDataRaw
	Delay uint32
}

// Encode returns a SCALE encoded GrandpaForcedChange with first type byte
func (fc *GrandpaForcedChange) Encode() ([]byte, error) {
	d, err := scale.Encode(fc)
	if err != nil {
		return nil, err
	}

	return append([]byte{ForcedChangeType}, d...), nil
}

// OnDisabled represents a GRANDPA authority being disabled
type OnDisabled struct {
	ID uint64
}

// Encode returns a SCALE encoded OnDisabled with first type byte
func (od *OnDisabled) Encode() ([]byte, error) {
	d, err := scale.Encode(od)
	if err != nil {
		return nil, err
	}

	return append([]byte{OnDisabledType}, d...), nil
}

// Pause represents an authority set pause
type Pause struct {
	Delay uint32
}

// Encode returns a SCALE encoded Pause with first type byte
func (p *Pause) Encode() ([]byte, error) {
	d, err := scale.Encode(p)
	if err != nil {
		return nil, err
	}

	return append([]byte{PauseType}, d...), nil
}

// Resume represents an authority set resume
type Resume struct {
	Delay uint32
}

// Encode returns a SCALE encoded Resume with first type byte
func (r *Resume) Encode() ([]byte, error) {
	d, err := scale.Encode(r)
	if err != nil {
		return nil, err
	}

	return append([]byte{ResumeType}, d...), nil
}
