//  Copyright (c) 2014 Couchbase, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// 		http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package in

import (
	"unicode"

	"github.com/blevesearch/bleve/analysis"
	"github.com/willf/bitset"
)

type ScriptData struct {
	flag       rune
	base       rune
	decompMask *bitset.BitSet
}

var scripts = map[*unicode.RangeTable]*ScriptData{
	unicode.Devanagari: {
		flag: 1,
		base: 0x0900,
	},
	unicode.Bengali: {
		flag: 2,
		base: 0x0980,
	},
	unicode.Gurmukhi: {
		flag: 4,
		base: 0x0A00,
	},
	unicode.Gujarati: {
		flag: 8,
		base: 0x0A80,
	},
	unicode.Oriya: {
		flag: 16,
		base: 0x0B00,
	},
	unicode.Tamil: {
		flag: 32,
		base: 0x0B80,
	},
	unicode.Telugu: {
		flag: 64,
		base: 0x0C00,
	},
	unicode.Kannada: {
		flag: 128,
		base: 0x0C80,
	},
	unicode.Malayalam: {
		flag: 256,
		base: 0x0D00,
	},
}

func flag(ub *unicode.RangeTable) rune {
	return scripts[ub].flag
}

var decompositions = [][]rune{
	/* devanagari, gujarati vowel candra O */
	{0x05, 0x3E, 0x45, 0x11, flag(unicode.Devanagari) | flag(unicode.Gujarati)},
	/* devanagari short O */
	{0x05, 0x3E, 0x46, 0x12, flag(unicode.Devanagari)},
	/* devanagari, gujarati letter O */
	{0x05, 0x3E, 0x47, 0x13, flag(unicode.Devanagari) | flag(unicode.Gujarati)},
	/* devanagari letter AI, gujarati letter AU */
	{0x05, 0x3E, 0x48, 0x14, flag(unicode.Devanagari) | flag(unicode.Gujarati)},
	/* devanagari, bengali, gurmukhi, gujarati, oriya AA */
	{0x05, 0x3E, -1, 0x06, flag(unicode.Devanagari) | flag(unicode.Bengali) | flag(unicode.Gurmukhi) | flag(unicode.Gujarati) | flag(unicode.Oriya)},
	/* devanagari letter candra A */
	{0x05, 0x45, -1, 0x72, flag(unicode.Devanagari)},
	/* gujarati vowel candra E */
	{0x05, 0x45, -1, 0x0D, flag(unicode.Gujarati)},
	/* devanagari letter short A */
	{0x05, 0x46, -1, 0x04, flag(unicode.Devanagari)},
	/* gujarati letter E */
	{0x05, 0x47, -1, 0x0F, flag(unicode.Gujarati)},
	/* gurmukhi, gujarati letter AI */
	{0x05, 0x48, -1, 0x10, flag(unicode.Gurmukhi) | flag(unicode.Gujarati)},
	/* devanagari, gujarati vowel candra O */
	{0x05, 0x49, -1, 0x11, flag(unicode.Devanagari) | flag(unicode.Gujarati)},
	/* devanagari short O */
	{0x05, 0x4A, -1, 0x12, flag(unicode.Devanagari)},
	/* devanagari, gujarati letter O */
	{0x05, 0x4B, -1, 0x13, flag(unicode.Devanagari) | flag(unicode.Gujarati)},
	/* devanagari letter AI, gurmukhi letter AU, gujarati letter AU */
	{0x05, 0x4C, -1, 0x14, flag(unicode.Devanagari) | flag(unicode.Gurmukhi) | flag(unicode.Gujarati)},
	/* devanagari, gujarati vowel candra O */
	{0x06, 0x45, -1, 0x11, flag(unicode.Devanagari) | flag(unicode.Gujarati)},
	/* devanagari short O */
	{0x06, 0x46, -1, 0x12, flag(unicode.Devanagari)},
	/* devanagari, gujarati letter O */
	{0x06, 0x47, -1, 0x13, flag(unicode.Devanagari) | flag(unicode.Gujarati)},
	/* devanagari letter AI, gujarati letter AU */
	{0x06, 0x48, -1, 0x14, flag(unicode.Devanagari) | flag(unicode.Gujarati)},
	/* malayalam letter II */
	{0x07, 0x57, -1, 0x08, flag(unicode.Malayalam)},
	/* devanagari letter UU */
	{0x09, 0x41, -1, 0x0A, flag(unicode.Devanagari)},
	/* tamil, malayalam letter UU (some styles) */
	{0x09, 0x57, -1, 0x0A, flag(unicode.Tamil) | flag(unicode.Malayalam)},
	/* malayalam letter AI */
	{0x0E, 0x46, -1, 0x10, flag(unicode.Malayalam)},
	/* devanagari candra E */
	{0x0F, 0x45, -1, 0x0D, flag(unicode.Devanagari)},
	/* devanagari short E */
	{0x0F, 0x46, -1, 0x0E, flag(unicode.Devanagari)},
	/* devanagari AI */
	{0x0F, 0x47, -1, 0x10, flag(unicode.Devanagari)},
	/* oriya AI */
	{0x0F, 0x57, -1, 0x10, flag(unicode.Oriya)},
	/* malayalam letter OO */
	{0x12, 0x3E, -1, 0x13, flag(unicode.Malayalam)},
	/* telugu, kannada letter AU */
	{0x12, 0x4C, -1, 0x14, flag(unicode.Telugu) | flag(unicode.Kannada)},
	/* telugu letter OO */
	{0x12, 0x55, -1, 0x13, flag(unicode.Telugu)},
	/* tamil, malayalam letter AU */
	{0x12, 0x57, -1, 0x14, flag(unicode.Tamil) | flag(unicode.Malayalam)},
	/* oriya letter AU */
	{0x13, 0x57, -1, 0x14, flag(unicode.Oriya)},
	/* devanagari qa */
	{0x15, 0x3C, -1, 0x58, flag(unicode.Devanagari)},
	/* devanagari, gurmukhi khha */
	{0x16, 0x3C, -1, 0x59, flag(unicode.Devanagari) | flag(unicode.Gurmukhi)},
	/* devanagari, gurmukhi ghha */
	{0x17, 0x3C, -1, 0x5A, flag(unicode.Devanagari) | flag(unicode.Gurmukhi)},
	/* devanagari, gurmukhi za */
	{0x1C, 0x3C, -1, 0x5B, flag(unicode.Devanagari) | flag(unicode.Gurmukhi)},
	/* devanagari dddha, bengali, oriya rra */
	{0x21, 0x3C, -1, 0x5C, flag(unicode.Devanagari) | flag(unicode.Bengali) | flag(unicode.Oriya)},
	/* devanagari, bengali, oriya rha */
	{0x22, 0x3C, -1, 0x5D, flag(unicode.Devanagari) | flag(unicode.Bengali) | flag(unicode.Oriya)},
	/* malayalam chillu nn */
	{0x23, 0x4D, 0xFF, 0x7A, flag(unicode.Malayalam)},
	/* bengali khanda ta */
	{0x24, 0x4D, 0xFF, 0x4E, flag(unicode.Bengali)},
	/* devanagari nnna */
	{0x28, 0x3C, -1, 0x29, flag(unicode.Devanagari)},
	/* malayalam chillu n */
	{0x28, 0x4D, 0xFF, 0x7B, flag(unicode.Malayalam)},
	/* devanagari, gurmukhi fa */
	{0x2B, 0x3C, -1, 0x5E, flag(unicode.Devanagari) | flag(unicode.Gurmukhi)},
	/* devanagari, bengali yya */
	{0x2F, 0x3C, -1, 0x5F, flag(unicode.Devanagari) | flag(unicode.Bengali)},
	/* telugu letter vocalic R */
	{0x2C, 0x41, 0x41, 0x0B, flag(unicode.Telugu)},
	/* devanagari rra */
	{0x30, 0x3C, -1, 0x31, flag(unicode.Devanagari)},
	/* malayalam chillu rr */
	{0x30, 0x4D, 0xFF, 0x7C, flag(unicode.Malayalam)},
	/* malayalam chillu l */
	{0x32, 0x4D, 0xFF, 0x7D, flag(unicode.Malayalam)},
	/* devanagari llla */
	{0x33, 0x3C, -1, 0x34, flag(unicode.Devanagari)},
	/* malayalam chillu ll */
	{0x33, 0x4D, 0xFF, 0x7E, flag(unicode.Malayalam)},
	/* telugu letter MA */
	{0x35, 0x41, -1, 0x2E, flag(unicode.Telugu)},
	/* devanagari, gujarati vowel sign candra O */
	{0x3E, 0x45, -1, 0x49, flag(unicode.Devanagari) | flag(unicode.Gujarati)},
	/* devanagari vowel sign short O */
	{0x3E, 0x46, -1, 0x4A, flag(unicode.Devanagari)},
	/* devanagari, gujarati vowel sign O */
	{0x3E, 0x47, -1, 0x4B, flag(unicode.Devanagari) | flag(unicode.Gujarati)},
	/* devanagari, gujarati vowel sign AU */
	{0x3E, 0x48, -1, 0x4C, flag(unicode.Devanagari) | flag(unicode.Gujarati)},
	/* kannada vowel sign II */
	{0x3F, 0x55, -1, 0x40, flag(unicode.Kannada)},
	/* gurmukhi vowel sign UU (when stacking) */
	{0x41, 0x41, -1, 0x42, flag(unicode.Gurmukhi)},
	/* tamil, malayalam vowel sign O */
	{0x46, 0x3E, -1, 0x4A, flag(unicode.Tamil) | flag(unicode.Malayalam)},
	/* kannada vowel sign OO */
	{0x46, 0x42, 0x55, 0x4B, flag(unicode.Kannada)},
	/* kannada vowel sign O */
	{0x46, 0x42, -1, 0x4A, flag(unicode.Kannada)},
	/* malayalam vowel sign AI (if reordered twice) */
	{0x46, 0x46, -1, 0x48, flag(unicode.Malayalam)},
	/* telugu, kannada vowel sign EE */
	{0x46, 0x55, -1, 0x47, flag(unicode.Telugu) | flag(unicode.Kannada)},
	/* telugu, kannada vowel sign AI */
	{0x46, 0x56, -1, 0x48, flag(unicode.Telugu) | flag(unicode.Kannada)},
	/* tamil, malayalam vowel sign AU */
	{0x46, 0x57, -1, 0x4C, flag(unicode.Tamil) | flag(unicode.Malayalam)},
	/* bengali, oriya vowel sign O, tamil, malayalam vowel sign OO */
	{0x47, 0x3E, -1, 0x4B, flag(unicode.Bengali) | flag(unicode.Oriya) | flag(unicode.Tamil) | flag(unicode.Malayalam)},
	/* bengali, oriya vowel sign AU */
	{0x47, 0x57, -1, 0x4C, flag(unicode.Bengali) | flag(unicode.Oriya)},
	/* kannada vowel sign OO */
	{0x4A, 0x55, -1, 0x4B, flag(unicode.Kannada)},
	/* gurmukhi letter I */
	{0x72, 0x3F, -1, 0x07, flag(unicode.Gurmukhi)},
	/* gurmukhi letter II */
	{0x72, 0x40, -1, 0x08, flag(unicode.Gurmukhi)},
	/* gurmukhi letter EE */
	{0x72, 0x47, -1, 0x0F, flag(unicode.Gurmukhi)},
	/* gurmukhi letter U */
	{0x73, 0x41, -1, 0x09, flag(unicode.Gurmukhi)},
	/* gurmukhi letter UU */
	{0x73, 0x42, -1, 0x0A, flag(unicode.Gurmukhi)},
	/* gurmukhi letter OO */
	{0x73, 0x4B, -1, 0x13, flag(unicode.Gurmukhi)},
}

func init() {
	for _, scriptData := range scripts {
		scriptData.decompMask = bitset.New(0x7d)
		for _, decomposition := range decompositions {
			ch := decomposition[0]
			flags := decomposition[4]
			if (flags & scriptData.flag) != 0 {
				scriptData.decompMask.Set(uint(ch))
			}
		}
	}
}

func lookupScript(r rune) *unicode.RangeTable {
	for script := range scripts {
		if unicode.Is(script, r) {
			return script
		}
	}
	return nil
}

func normalize(input []rune) []rune {
	inputLen := len(input)
	for i := 0; i < inputLen; i++ {
		r := input[i]
		script := lookupScript(r)
		if script != nil {
			scriptData := scripts[script]
			ch := r - scriptData.base
			if scriptData.decompMask.Test(uint(ch)) {
				input = compose(ch, script, scriptData, input, i, inputLen)
				inputLen = len(input)
			}
		}
	}
	return input[0:inputLen]
}

func compose(ch0 rune, script0 *unicode.RangeTable, scriptData *ScriptData, input []rune, pos int, inputLen int) []rune {
	if pos+1 >= inputLen {
		return input // need at least 2 characters
	}

	ch1 := input[pos+1] - scriptData.base
	script1 := lookupScript(input[pos+1])
	if script0 != script1 {
		return input // need to be same script
	}

	ch2 := rune(-1)
	if pos+2 < inputLen {
		ch2 = input[pos+2] - scriptData.base
		script2 := lookupScript(input[pos+2])
		if input[pos+2] == '\u200D' {
			ch2 = 0xff // zero width joiner
		} else if script2 != script1 {
			ch2 = -1 // still allow 2 character match
		}
	}

	for _, decomposition := range decompositions {
		if decomposition[0] == ch0 &&
			(decomposition[4]&scriptData.flag) != 0 {
			if decomposition[1] == ch1 &&
				(decomposition[2] < 0 || decomposition[2] == ch2) {
				input[pos] = scriptData.base + decomposition[3]
				input = analysis.DeleteRune(input, pos+1)
				if decomposition[2] >= 0 {
					input = analysis.DeleteRune(input, pos+1)
				}
				return input
			}
		}
	}
	return input
}
