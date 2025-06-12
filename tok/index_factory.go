/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package tok

import (
	"github.com/pkg/errors"

	"github.com/hypermodeinc/dgraph/v25/tok/index"
	opts "github.com/hypermodeinc/dgraph/v25/tok/options"
)

// registerIndexFactory(f) will register f as both a Tokenizer and specifically
// as an IndexFactory.
func registerIndexFactory(f IndexFactory) {
	// Note: All necessary checks for duplication, etc. is done in
	//       registerTokenizers. Since we add IndexFactory instances
	//       to both tokenizers map and indexFactories map, it suffices
	//       to just check the tokenizers map for uniqueness.
	registerTokenizer(f)
	indexFactories[f.Name()] = f
}

// IndexFactory combines the notion of a Tokenizer with
// index.IndexFactory. We register IndexFactory instances just
// like we register Tokenizers.
type IndexFactory interface {
	Tokenizer
	// TODO: Distinguish between float64 and float32, allowing either.
	//       Default should be float32.
	index.IndexFactory[float32]
}

// FactoryCreateSpec includes an IndexFactory and the options required
// to instantiate a VectorIndex of the given type.
// In short, everything that is needed in order to create a VectorIndex!
type FactoryCreateSpec struct {
	factory IndexFactory
	opts    opts.Options
}

func (fcs *FactoryCreateSpec) Name() string {
	return fcs.factory.Name() + fcs.factory.GetOptions(fcs.opts)
}

func (fcs *FactoryCreateSpec) CreateIndex(name string, split int) (index.VectorIndex[float32], error) {
	if fcs == nil || fcs.factory == nil {
		return nil,
			errors.Errorf(
				"cannot create Index for '%s' with nil factory",
				name)
	}

	// TODO: What we should really be doing here is a "Find it, and if found,
	//       replace it *only if* the options have changed!" However, there
	//       is currently no way to introspect the options.
	//       We cheat for the moment and simply do a CreateOrReplace.
	//       This avoids us getting into duplicate create conflicts, but
	//       has the downside of not allowing us to reuse the pre-existing
	//       index.
	// nil VectorSource at the moment.
	return fcs.factory.CreateOrReplace(name, fcs.opts, 32, split)
}

func createIndexFactory(f index.IndexFactory[float32]) IndexFactory {
	return &indexFactory{delegate: f}
}

type indexFactory struct {
	delegate index.IndexFactory[float32]
}

func (f *indexFactory) Name() string { return f.delegate.Name() }
func (f *indexFactory) AllowedOptions() opts.AllowedOptions {
	return f.delegate.AllowedOptions()
}
func (f *indexFactory) Create(
	name string,
	o opts.Options,
	floatBits int,
	split int) (index.VectorIndex[float32], error) {
	return f.delegate.Create(name, o, floatBits, split)
}
func (f *indexFactory) Find(name string) (index.VectorIndex[float32], error) {
	return f.delegate.Find(name)
}
func (f *indexFactory) Remove(name string) error {
	return f.delegate.Remove(name)
}
func (f *indexFactory) CreateOrReplace(
	name string,
	o opts.Options,
	floatBits int,
	split int) (index.VectorIndex[float32], error) {
	return f.delegate.CreateOrReplace(name, o, floatBits, split)
}

func (f *indexFactory) GetOptions(o opts.Options) string {
	return f.delegate.GetOptions(o)
}

func (f *indexFactory) Type() string {
	return "float32vector"
}
func (f *indexFactory) Tokens(v interface{}) ([]string, error) {
	return tokensForExpectedVFloat(v)
}
func (f *indexFactory) Identifier() byte { return IdentVFloat }
func (f *indexFactory) IsSortable() bool { return false }
func (f *indexFactory) IsLossy() bool    { return true }

func tokensForExpectedVFloat(v interface{}) ([]string, error) {
	// If there is a vfloat, we can only allow one mutation at a time
	return []string{"float"}, nil
}
