#!/bin/bash
# Visualize Dgraph cluster topology from docker-compose.yml

FILE="${1:-docker-compose.yml}"

if [[ ! -f $FILE ]]; then
	echo "Usage: $0 [docker-compose.yml]"
	exit 1
fi

# Flatten the file (join continuation lines) for easier parsing
FLAT=$(tr '\n' ' ' <"$FILE" | sed 's/  */ /g')

# Extract zero nodes (find --my=zeroN:port patterns near "zero")
zeros=$(echo "$FLAT" | grep -oE '\-\-my=zero[a-zA-Z0-9_-]*:[0-9]+' | sed -E 's/--my=([^:]+):.*/\1/' | sort -u)

# Extract alpha nodes with their group assignments
TMPFILE=$(mktemp)

# Extract all alphas by their --my flag. Group assignment is optional in flags.
# If no group= is found near the alpha's command, put it under UNASSIGNED.
alphas=$(echo "$FLAT" | grep -oE '\-\-my=alpha[a-zA-Z0-9_-]*:[0-9]+' | sed -E 's/--my=([^:]+):.*/\1/' | sort -u)
for alpha in $alphas; do
	# Look for group= near this alpha occurrence in the flattened compose.
	# Many compose files won't specify it; in that case group is assigned dynamically by Zero.
	# Use awk (portable on macOS) instead of BSD sed regex intervals.
	group=$(awk -v s="$FLAT" -v a="--my=${alpha}:" '
		BEGIN { pos = index(s, a); if (pos == 0) exit }
		END {
			win = substr(s, pos, 600)
			if (match(win, /group=[0-9]+/)) {
				g = substr(win, RSTART, RLENGTH)
				split(g, parts, "=")
				print parts[2]
			}
		}
	' </dev/null)
	if [[ -z $group ]]; then
		group="UNASSIGNED"
	fi
	echo "$group $alpha"
done >"$TMPFILE"

# Get unique groups (numeric groups first, UNASSIGNED last, never duplicated)
numeric_groups=$(grep -E '^[0-9]+ ' "$TMPFILE" | cut -d' ' -f1 | sort -n | uniq)
unique_groups="$numeric_groups"
if grep -q '^UNASSIGNED ' "$TMPFILE"; then
	unique_groups="$unique_groups UNASSIGNED"
fi

# Extract ratel if present
has_ratel=$(grep -q 'ratel' "$FILE" && echo "yes" || echo "no")

# Get replica count
replicas=$(grep -oE '\-\-replicas=[0-9]+' "$FILE" | head -1 | cut -d= -f2)
replicas=${replicas:-1}

# Calculate max alphas per group for width calculation
max_per_group=0
for group in $unique_groups; do
	cnt=$(grep -c "^$group " "$TMPFILE" 2>/dev/null || echo 0)
	if [[ $cnt -gt $max_per_group ]]; then
		max_per_group=$cnt
	fi
done

# Count zeros
zero_count=$(echo "$zeros" | wc -w | tr -d ' ')

# Box width: 14 chars per node (┌──────────┐ + 2 spaces) + margins
# Use the larger of zeros or max alphas per group
max_nodes=$zero_count
if [[ $max_per_group -gt $max_nodes ]]; then
	max_nodes=$max_per_group
fi
[[ $max_nodes -lt 3 ]] && max_nodes=3

inner_width=$((max_nodes * 14 + 4))
[[ $inner_width -lt 60 ]] && inner_width=60
outer_width=$((inner_width + 4))

# Total line width (display columns between outer left and right border)
W=$((outer_width))

# Print N space characters
sp() {
	local i
	for ((i = 0; i < $1; i++)); do printf " "; done
}

# Print N copies of a character
rp() {
	local i
	for ((i = 0; i < $1; i++)); do printf "%s" "$2"; done
}

# Finish outer row: pad from current column to W, then print |
fin() {
	sp "$((W - $1))"
	printf "|\n"
}

# Finish inner row (inside group box): pad then print | |
# Total should be W+1 cols: content($1) + spaces + 3(| |) = W+1
fini() {
	sp "$((W - $1 - 2))"
	printf "| |\n"
}

# Separator using full width
sep() {
	printf "%s" "$1"
	rp "$W" "$2"
	printf "%s\n" "$3"
}

# Print a row that starts with '|' and does NOT yet include the final right border.
# Pads with spaces so the final '|' is always aligned.
outln() {
	local s="$1"
	local l=${#s}
	# Total line length must be W + 2 (left border + interior + right border).
	# If s already includes the left border '|', it must be padded to length W+1,
	# then we print the final right border.
	printf "%s" "$s"
	local pad=$((W + 1 - l))
	if [[ $pad -lt 0 ]]; then
		pad=0
	fi
	sp "$pad"
	printf "|\n"
}

# Render a row inside the group box.
# The group box top uses: "|  +" + GW*"-" + "+".
# For interior rows we use: "|  |" + <payload> + <pad-to-GW> + "|".
out_group() {
	local payload="$1"
	local plen=${#payload}
	local pad=$((GW - plen))
	if [[ $pad -lt 0 ]]; then
		pad=0
	fi
	local s="|  |${payload}"
	# pad inside the group box up to width GW
	for ((i = 0; i < pad; i++)); do
		s+=" "
	done
	s+="|"
	outln "$s"
}

echo ""

# === HEADER ===
sep "+" "=" "+"
outln "|  DGRAPH CLUSTER TOPOLOGY"
sep "+" "=" "+"
echo ""

# === ZERO LAYER ===
sep "+" "-" "+"
outln "|  ZERO LAYER (Cluster Coordination)"
sep "+" "-" "+"

# Zero boxes: each is 14 cols (box 12 + 2 spaces)
line="|  "
for z in $zeros; do line+="+----------+  "; done
outln "$line"
line="|  "
for z in $zeros; do line+=$(printf "| %-8s |  " "$z"); done
outln "$line"
line="|  "
for z in $zeros; do line+="|  (zero)  |  "; done
outln "$line"
line="|  "
for z in $zeros; do line+="+----------+  "; done
outln "$line"

sep "+" "-" "+"
echo ""

# === ALPHA LAYER ===
sep "+" "-" "+"
outln "|  ALPHA LAYER (Data Storage) - Replicas: $replicas"
sep "+" "-" "+"

# Group box inner width
GW=$((inner_width - 2))

for group in $unique_groups; do
	alphas=$(grep "^$group " "$TMPFILE" | cut -d' ' -f2)
	count=$(echo "$alphas" | wc -l | tr -d ' ')
	alpha_count=$(echo "$alphas" | wc -w | tr -d ' ')

	# Empty row
	outln "|"

	# Group box top
	line="|  +"
	for ((i = 0; i < GW; i++)); do line+="-"; done
	line+="+"
	outln "$line"

	# Group title row
	if [[ $group == "UNASSIGNED" ]]; then
		gtitle="  GROUP UNASSIGNED (Raft Replicas: $count)"
	else
		gtitle="  GROUP $group  (Raft Replicas: $count)"
	fi
	out_group "$gtitle"

	# Alpha boxes
	line="  "
	for a in $alphas; do line+="+----------+  "; done
	out_group "$line"
	line="  "
	for a in $alphas; do line+=$(printf "| %-8s |  " "$a"); done
	out_group "$line"
	line="  "
	for a in $alphas; do line+="| (alpha)  |  "; done
	out_group "$line"
	line="  "
	for a in $alphas; do line+="+----------+  "; done
	out_group "$line"

	# Group box bottom
	line="|  +"
	for ((i = 0; i < GW; i++)); do line+="-"; done
	line+="+"
	outln "$line"
done

outln "|"
sep "+" "-" "+"

# === RATEL ===
if [[ $has_ratel == "yes" ]]; then
	echo ""
	sep "+" "-" "+"
	outln "|  UI LAYER"
	sep "+" "-" "+"
	outln "|  +----------+"
	outln "|  |  ratel   |  (Web UI on :8000)"
	outln "|  +----------+"
	sep "+" "-" "+"
fi

rm -f "$TMPFILE"

echo ""
echo "Legend: Alphas in same GROUP replicate data via Raft consensus"
echo ""
