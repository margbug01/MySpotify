#!/usr/bin/env bash
# Clears what the app keeps about the listener, so it opens as on first use:
# its settings, where playback was left, its window and its login. The
# songs it cached stay unless --songs is given.
#
#   scripts/reset.sh              the development build (mygo dev)
#   scripts/reset.sh --release    the installed build (mygo build)
#   scripts/reset.sh --keep-login keeps the login, to skip scanning again
#   scripts/reset.sh --songs      also clears the cached songs
#
# The app saves its settings and session as it quits, so a running one is
# quit first.
set -euo pipefail

name="MyGo 云音乐"
id="dev.egoist.cloudmusic"
release=0 keep_login=0 songs=0
for arg in "$@"; do
	case "$arg" in
	--release) release=1 ;;
	--keep-login) keep_login=1 ;;
	--songs) songs=1 ;;
	-h | --help)
		sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'
		exit 0
		;;
	*)
		echo "unknown option: $arg" >&2
		exit 2
		;;
	esac
done
if [[ $release == 0 ]]; then
	name+=" Dev"
	id+=".dev"
fi

if [[ $(uname) != Darwin ]]; then
	echo "only macOS for now" >&2
	exit 1
fi
data="$HOME/Library/Application Support/$name"
cache="$HOME/Library/Caches/$name"

if pgrep -f "/$name.app/Contents/MacOS/" >/dev/null; then
	echo "quitting $name…"
	osascript -e "tell application id \"$id\" to quit" >/dev/null 2>&1 || true
	for _ in $(seq 50); do
		pgrep -f "/$name.app/Contents/MacOS/" >/dev/null || break
		sleep 0.1
	done
	if pgrep -f "/$name.app/Contents/MacOS/" >/dev/null; then
		echo "$name is still running: quit it and try again" >&2
		exit 1
	fi
fi

if [[ -d $data ]]; then
	for f in "$data"/* "$data"/.[!.]*; do
		[[ -e $f ]] || continue
		# The login: the session, and the device it was made on.
		if [[ $keep_login == 1 ]]; then
			case "$(basename "$f")" in session | device-id) continue ;; esac
		fi
		rm -rf "$f"
		echo "removed ${f/#$HOME/~}"
	done
fi

# The installed build keeps the login in the Keychain, the development one
# in the session file above.
if [[ $release == 1 && $keep_login == 0 ]]; then
	if security delete-generic-password -s "$id" -a netease-session >/dev/null 2>&1; then
		echo "removed the login from the Keychain"
	fi
fi

if [[ $songs == 1 && -d $cache ]]; then
	rm -rf "$cache"
	echo "removed ${cache/#$HOME/~}"
fi
echo "$name will open as on first use"
