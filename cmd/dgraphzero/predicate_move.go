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

/*
Steps to move predicate p from g1 to g2.
1. Update group zero config to block mutations on predicate p.

2. Propose RefreshMembership on g1 so all nodes refresh their config from group zero.
Useful if some nodes get paritioned away from group zero.
All the nodes would get update from group zero and they can start rejecting mutations, in
case they don't get update from group zero the mutation would reach g1 and it would reject it.

3. Initiate transfer from g1 to g2.
4. In case of error, unblock mutations, propose refreshmembership on g1.
5. Else, update config that predicate p would be served by g2.
Depending on whether the nodes get the new configuration from group zero, requests might come to
g1 or g2. Writes would be rejected, reads can be done from any group and they should give same results.

6. Propose refreshmembership on g1, then g1 would know that it doesn't serve that predicate anymore.
The above is necessary to block reads on g1 followers which are paritioned away from group zero,
now due to linearizability they would get refreshmembership and would throw error if they would crash
if they can't reach group zero, or else refresh and reject the read request.

7. Unblock mutations on predicate p.
*/
