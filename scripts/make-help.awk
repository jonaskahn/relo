# Renders the list `make` prints with no arguments. Groups come from the
# `# === group: purpose ===` dividers in the Makefile, a target's one-liner
# from the `## text` line above it, so the list can never drift from the file.
# Targets always print in name-and-description list form.

function render(   g, t, name_column) {
	name_column = widest + 3

	for (g = 1; g <= groups; g++) {
		if (g > 1) printf "\n"
		printf "  %s%s%s  %s%s%s\n", strong[g], label[g], reset, dim, purpose[g], reset
		printf "  %s%s%s\n", dim, rule, reset

		for (t = 1; t <= member[g]; t++)
			printf "    %s%-*s%s  %s%s%s\n", hue[g], name_column, target[g, t], reset, dim, note[g, t], reset
	}
}

BEGIN {
	width = 72
	rule = ""
	for (i = 0; i < width - 4; i++) rule = rule "─"

	if (c) {
		bold = "\033[1m"; dim = "\033[2m"; reset = "\033[0m"
		tones = split("36 32 33 35 34 96", palette, " ")
	} else {
		bold = dim = reset = ""
	}

	printf "\n  %srelo%s %s·%s  make <target>\n", bold, reset, dim, reset
}

/^# === / {
	header = $0
	sub(/^# === /, "", header)
	sub(/ ===$/, "", header)
	colon = index(header, ":")

	groups++
	label[groups] = toupper(substr(header, 1, colon - 1))
	purpose[groups] = substr(header, colon + 2)
	member[groups] = 0

	if (c) {
		tone = palette[(groups - 1) % tones + 1]
		hue[groups] = "\033[" tone "m"
		strong[groups] = "\033[1;" tone "m"
	} else {
		hue[groups] = strong[groups] = ""
	}
}

/^## / {
	pending = $0
	sub(/^## /, "", pending)
}

/^[a-z][a-zA-Z0-9-]*:/ {
	name = $1
	sub(/:$/, "", name)

	member[groups]++
	target[groups, member[groups]] = name
	note[groups, member[groups]] = pending
	pending = ""

	if (length(name) > widest) widest = length(name)
}

END { render() }
