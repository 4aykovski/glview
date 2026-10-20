#!/usr/bin/env bash
# Guard against confidential data reaching the public repository.
#
# Usage:
#   confidential.sh staged         staged changes (added lines, paths, author)
#   confidential.sh msg <file>     a commit message
#   confidential.sh range <revs>   commits being pushed (messages, diffs, authors)
#   confidential.sh tree           every tracked file
#
# Two kinds of checks:
#   1. the local denylist .confidential-patterns (gitignored: it names the
#      things that must not leak, so it never leaves the machine), one entry
#      per line, '#' starts a comment: a case-insensitive extended regex, or
#      "=word" for a fixed string matched as a whole word (fast; use it for
#      logins and project names);
#   2. built-in generic checks: tokens and keys, e-mail addresses outside
#      the allowlist, URLs of internal hosts.
#
# There is no bypass: fix the content, or extend the local denylist.
set -euo pipefail

root=$(git rev-parse --show-toplevel)
denylist="$root/.confidential-patterns"

if [[ ! -f "$denylist" ]]; then
	cat >&2 <<EOF
confidential: $denylist is missing.
Create it (it is gitignored) with one extended regex per line naming
everything that must never reach GitHub: work hosts, groups, projects,
task keys, colleagues' logins, internal domains. An empty file is allowed
only if there is nothing to protect.
EOF
	exit 1
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
patterns="$tmp/patterns"
words="$tmp/words"
hits="$tmp/hits" # scan runs in pipeline subshells, so findings go to a file
grep -v -E '^[[:space:]]*(#|$|=)' "$denylist" >"$patterns" || true
{ grep -E '^=' "$denylist" || true; } | cut -c2- | { grep -v -E '^[[:space:]]*$' || true; } >"$words"

# Built-in secret shapes. Obvious test fakes (glpat-xxxx…, glpat-test-…) pass.
secret_re='(gl(pat|ptt|dt|rt|cbt|imt|agent|oas|ft|soat)-[A-Za-z0-9_-]{20,}|gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{30,}|AKIA[0-9A-Z]{16}|xox[abposr]-[A-Za-z0-9-]{10,}|-----BEGIN [A-Z ]*PRIVATE KEY-----)'
fake_re='(x{8,}|fake|test|example|dummy|sample)'
email_re='[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}'
email_ok='[A-Za-z0-9._%+-]+@(([A-Za-z0-9-]+\.)*example\.(com|org|net)|users\.noreply\.github\.com)|noreply@anthropic\.com|[A-Za-z0-9._%+-]+@(github|gitlab)\.com'
host_re='https?://[A-Za-z0-9.-]*(corp|intra|internal|local|lan)([.:/]|$)'

# emit reads line numbers of the scanned input and reports those lines.
emit() { # input file, reason
	sort -un | awk -F'\t' -v r="$2" -v h="$hits" '
		NR == FNR { want[$1]; next }
		FNR in want {
			printf "confidential: %s: %s\n    %s\n", $1, r, substr($0, length($1) + 2, 200) > "/dev/stderr"
			print "x" >> h
		}' - "$1"
}

# word_hits prints numbers of lines containing a denylisted "=word": tokens
# of [a-z0-9._/-] are looked up whole and by their "/"- and "-"-separated
# runs ("team/app/billing-api-v2" finds "team/app" and "billing-api"). BSD
# grep -F -w is too slow for hundreds of words, awk hashes are instant.
word_hits() {
	awk -v W="$words" '
		function has(s,   p, n, a, b, x) {
			if (s in w) return 1
			n = split(s, p, "-")
			for (a = 1; a <= n; a++) {
				x = p[a]
				if (x in w) return 1
				for (b = a + 1; b <= n; b++) { x = x "-" p[b]; if (x in w) return 1 }
			}
			return 0
		}
		BEGIN { while ((getline l < W) > 0) w[tolower(l)] = 1 }
		{
			n = split(tolower($0), t, /[^a-z0-9._\/-]+/)
			for (i = 1; i <= n; i++) {
				tok = t[i]
				gsub(/^[._\/-]+|[._\/-]+$/, "", tok)
				if (tok == "") continue
				m = split(tok, seg, "/")
				for (a = 1; a <= m; a++) {
					x = seg[a]
					if (has(x)) { print NR; next }
					for (b = a + 1; b <= m; b++) { x = x "/" seg[b]; if (has(x)) { print NR; next } }
				}
			}
		}' "$1"
}

# scan reads "location<TAB>text" lines; "authors" checks the denylist only
# (an author's own e-mail is not a leak).
scan() {
	local in text
	in=$(mktemp "$tmp/in.XXXXXX")
	cat >"$in"
	text="$in.text"
	cut -f2- "$in" >"$text"
	if [[ -s "$patterns" ]]; then
		{ grep -n -i -E -f "$patterns" "$text" || true; } | cut -d: -f1 | emit "$in" "matches .confidential-patterns"
	fi
	if [[ -s "$words" ]]; then
		word_hits "$text" | emit "$in" "matches .confidential-patterns"
	fi
	[[ "${1:-}" == authors ]] && return 0
	{ grep -n -o -E "$secret_re" "$text" || true; } | { grep -v -i -E "$fake_re" || true; } | cut -d: -f1 | emit "$in" "looks like a secret"
	{ grep -n -o -E "$email_re" "$text" || true; } | { grep -v -i -E "^[0-9]+:($email_ok)$" || true; } | cut -d: -f1 | emit "$in" "e-mail outside the allowlist"
	{ grep -n -i -E "$host_re" "$text" || true; } | cut -d: -f1 | emit "$in" "internal host URL"
}

# diff_lines turns a unified diff into "path:line<TAB>text" for added lines.
diff_lines() {
	awk '
		/^\+\+\+ / { path = substr($0, 7); next }
		/^@@/ { split($3, a, ","); line = substr(a[1], 2) + 0; next }
		/^\+/ { printf "%s:%d\t%s\n", path, line, substr($0, 2); line++ }
	'
}

case "${1:-}" in
staged)
	git diff --cached --name-only --diff-filter=ACMR | awk '{ printf "path\t%s\n", $0 }' | scan
	git diff --cached --no-color --no-ext-diff -U0 --diff-filter=ACMR | diff_lines | scan
	printf 'author\t%s\n' "$(git var GIT_AUTHOR_IDENT)" | scan authors
	;;
msg)
	{ grep -v '^#' "$2" || true; } | awk '{ printf "commit message:%d\t%s\n", NR, $0 }' | scan
	;;
range)
	shift
	git log --no-color --format='%h%x09%an <%ae> / %cn <%ce>' "$@" | awk -F'\t' '{ printf "author of %s\t%s\n", $1, $2 }' | scan authors
	git log --no-color --format='==commit== %h%n%B' "$@" | awk '/^==commit== / { c = $2; next } { printf "message of %s\t%s\n", c, $0 }' | scan
	git log --no-color --format='' -p "$@" | diff_lines | scan
	;;
tree)
	git ls-files | awk '{ printf "path\t%s\n", $0 }' | scan
	git grep -I -n -e '' | awk '{ i = index($0, ":"); j = index(substr($0, i + 1), ":"); printf "%s\t%s\n", substr($0, 1, i + j - 1), substr($0, i + j + 1) }' | scan
	;;
*)
	echo "usage: $0 staged | msg <file> | range <revs> | tree" >&2
	exit 2
	;;
esac

if [[ -s "$hits" ]]; then
	echo "confidential: blocked. Remove the data above; never bypass with --no-verify." >&2
	exit 1
fi
