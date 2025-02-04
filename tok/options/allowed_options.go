/*
 * SPDX-FileCopyrightText: © Hypermode Inc. <hello@hypermode.com>
 * SPDX-License-Identifier: Apache-2.0
 */

package options

import (
	"fmt"
)

// AllowedOptions specifies the collection of allowed option kinds,
// and specifies how to interpret the values.
// The assumption is that at some point, options are being specified
// by some kind of key-value pair in a string, such as: "dataWidth:8".
// In this case, for a given index, if we want to form the desired
// options, we need to know that:
//  1. "dataWidth" is an allowed option
//  2. how to interpret the string "8".
//
// In this case, to an AllowedOptions type, we would add the key "dataWidth"
// and an IntParser as the value (or whichever data type is appropriate for
// the dataWidth option).
type AllowedOptions map[string]OptionParser

func NewAllowedOptions() AllowedOptions {
	return AllowedOptions(make(map[string]OptionParser))
}

// OptionValue pair represents the use of a particular option with a
// string representation of its intended value. It is expected that one
// might originally encounter a string representation of the value, but
// then might need to interpret the intended value for the option.
type OptionValuePair struct {
	Option string
	Value  string
}

// AddIntOption(optName) will specify that optName is an allowed option,
// and that the values will be interepreted by the standard IntOptionParser.
func (ao AllowedOptions) AddIntOption(optName string) AllowedOptions {
	ao[optName] = IntOptParser
	return ao
}

// AddStringOption(optName) will specify that optName is an allowed option,
// and that the values will be interepreted by the standard StringOptionParser.
func (ao AllowedOptions) AddStringOption(optName string) AllowedOptions {
	ao[optName] = StringOptParser
	return ao
}

// AddFloat64Option(optName) will specify that optName is an allowed option,
// and that the values will be interepreted by the standard Float64OptionParser.
func (ao AllowedOptions) AddFloat64Option(optName string) AllowedOptions {
	ao[optName] = Float64OptParser
	return ao
}

// AddCustomOption(optName) will specify that optName is an allowed option,
// and that the values will be interpreted by the provided parser.
func (ao AllowedOptions) AddCustomOption(optName string, parser OptionParser) AllowedOptions {
	ao[optName] = parser
	return ao
}

// ao.GetOptionList() provides the list of named options for ao.
func (ao AllowedOptions) GetOptionList() []string {
	keys := make([]string, len(ao))
	i := 0
	for k := range ao {
		keys[i] = k
	}
	return keys
}

// ao.GetParsedOption(optName, optValue) will parse optValue based on the
// parser associated with optName in ao. Note that if ao does not specify
// a parser for optName, this will panic!
func (ao AllowedOptions) GetParsedOption(optName, optValue string) (any, error) {
	parser, ok := ao[optName]
	if !ok {
		return nil, fmt.Errorf("option %s is not allowed!", optName)
	}
	return parser(optValue)
}

// ao.ParsePair(pair) is shorthand for:
//
//	ao.GetParsedOption(pair.Option, pair.Value)
func (ao AllowedOptions) ParsePair(pair OptionValuePair) (any, error) {
	return ao.GetParsedOption(pair.Option, pair.Value)
}

// ao.PopulateOptions(pairs, opts) will use ao to interpret pairs and
// use the results to populate opts. This will look at each OptionValuePair in
// pairs, and if the value can be properly parsed, it will add the result to
// opts for the given option and result. If one or more of the  options cannot
// be properly interpreted, this results in an error.
func (ao AllowedOptions) PopulateOptions(pairs []OptionValuePair, opts Options) error {
	for _, p := range pairs {
		val, err := ao.ParsePair(p)
		if err != nil {
			// TODO: Perhaps wrap the error with a bit more context??
			return err
		}
		opts.SetOpt(p.Option, val)
	}
	return nil
}
