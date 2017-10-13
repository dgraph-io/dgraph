/*
 * Copyright (C) 2017 Dgraph Labs, Inc. and Contributors
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package main

import "net"

func Tokenizer() interface{} { return CIDRTokenizer{} }

type CIDRTokenizer struct{}

func (CIDRTokenizer) Name() string     { return "cidr" }
func (CIDRTokenizer) Type() string     { return "string" }
func (CIDRTokenizer) Identifier() byte { return 0xff }

func (t CIDRTokenizer) Tokens(value interface{}) ([]string, error) {
	_, ipnet, err := net.ParseCIDR(value.(string))
	if err != nil {
		return nil, err
	}
	ones, bits := ipnet.Mask.Size()
	var toks []string
	for i := ones; i >= 1; i-- {
		m := net.CIDRMask(i, bits)
		tok := net.IPNet{
			IP:   ipnet.IP.Mask(m),
			Mask: m,
		}
		toks = append(toks, tok.String())
	}
	return toks, nil
}
