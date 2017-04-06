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

package testing

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSpielberg(t *testing.T) {
	q := `
    {
      me(id: m.06pj8) {
        name@en
        director.film (first: 4)  {
            name@en
        }
      }
    }`

	res := decodeResponse(q)
	expectedRes := `{"me":[{"director.film":[{"name@en":"Indiana Jones and the Temple of Doom"},{"name@en":"Jaws"},{"name@en":"Saving Private Ryan"},{"name@en":"Close Encounters of the Third Kind"}],"name@en":"Steven Spielberg"}]}`
	require.JSONEq(t, expectedRes, res)
}
