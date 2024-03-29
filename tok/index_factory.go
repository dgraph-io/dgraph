/*
 * Copyright 2016-2023 Dgraph Labs, Inc. and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package tok

import (
	"fmt"

	"github.com/pkg/errors"

	"github.com/dgraph-io/dgraph/tok/index"
	opts "github.com/dgraph-io/dgraph/tok/options"
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
	return fcs.factory.Name()
}

func (fcs *FactoryCreateSpec) CreateIndex(name string) (index.VectorIndex[float32], error) {
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
	return fcs.factory.CreateOrReplace(name, fcs.opts, 32)
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
	floatBits int) (index.VectorIndex[float32], error) {
	return f.delegate.Create(name, o, floatBits)
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
	floatBits int) (index.VectorIndex[float32], error) {
	return f.delegate.CreateOrReplace(name, o, floatBits)
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
	return []string{fmt.Sprint("float")}, nil
}
