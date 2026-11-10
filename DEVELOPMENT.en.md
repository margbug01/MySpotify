# Development guide

[简体中文](DEVELOPMENT.md) · **English**

How MySpotify is built, released and debugged. To just use it, the [README](README.en.md) is enough.

## How it works

- **Sign-in**: a window that usually stays hidden opens open.spotify.com, and `internal/spotify/capture.js` records the token and client-token the web player gets for itself. If you aren't signed in, that window is shown as Spotify's login page. After signing in, the window's cookies are saved encrypted (`internal/app/account_store.go`). When the token is about to expire, those cookies and a one-time TOTP code go straight to `/api/token`, as the web player renews itself, with no window to open. Only when the cookies are no longer valid does it fall back to reloading that window. When not signed in, the same `/api/token` gives a guest identity.
- **Data**: Pathfinder GraphQL persisted queries (their hashes are in `ops` in `internal/spotify/spotify.go`) plus spclient (lyrics, the player queue, playlist changes, radio, recently played, profile). The public Web API answers 429 to this identity across the board, so it isn't used. When a hash goes stale (412 "Invalid query hash"), `internal/spotify/ops.go` rereads the main script and the chunks loaded on demand, retries once with the new hash and saves it to `%APPDATA%\Vinyl\queries.json`, reading no more than once in 10 minutes. Library lists are checked when the window comes back to the front (at most every 5 seconds) and every 30 seconds, keeping the last result when the network fails.
- **Playback**: another hidden WebView page (`internal/player/web/player.html`) plays encrypted MP4. Windows uses hls.js + Widevine. macOS follows the Safari web player: it needs files in the cbcs format and uses WebKit's native HLS + FairPlay (`skd://` plus the file ID, `fairplay-license/v1`). Manifest, segment and license requests all go through a small local service on 127.0.0.1, which stores segments in the cache as they pass, keyed by the CDN path and byte range (the CDN's signature changes daily and stays out of the key). Podcast episodes take this path when they can; for when they can't, see "Podcasts" below. The page is created the first time a song really plays and closed after 5 idle minutes, resuming from the same position next time. It doesn't use the GPU, while the main interface does as usual.
- **Errors**: a 401 on the license asks once more with a new identity. A 429 waits for Retry-After (doubling from 30 seconds when none is given), then restarts from the same position. A 403 stops at that song. Network errors in the page (`startLoad`, or `loadSource` again if the manifest failed) and media errors (`recoverMediaError`) are retried twice per song before skipping to the next. When the end event doesn't arrive, a song counts as finished only if the audio has stopped within the last 0.25 seconds, the tail is fully buffered and nothing has moved for about 4 seconds.
- **Lyrics**: Spotify's synced lyrics → NetEase (`/api/cloudsearch/pc` searching by title and main artist, taking the closest song of the same name within 2 seconds of length; its lengths match Spotify's almost to the millisecond, and the text is Simplified Chinese) → LRCLIB exact lookup (title, main artist, album and length must all match) → LRCLIB search for a timed entry with the same title and artist within 2 seconds → plain text only when nothing has timing. When the artist name from Connect is missing or spelled in a foreign script (Yitai Wang), it's first replaced by the Chinese name Spotify gives (王以太). When a site is busy or rate limiting (LRCLIB 503, NetEase 405/406) it tries again after 3 and 10 seconds. Podcasts aren't looked up. Once a foreign-language song has timed lyrics, NetEase is asked once more (`tv=-1`): if it has a Chinese translation, its original text + translation is used instead (the two match line by line). Chinese lyrics are converted to Simplified using OpenCC's character table (`internal/lyrics/TSCharacters.txt`), unless the system's preferred Chinese is Traditional (zh-Hant/TW/HK).
- **Connect**: the protocol structure follows [Flutify](https://github.com/is-hp-is-mad/Flutify), without the Premium-only Web Playback SDK. There are two long-lived Dealer connections: `connect-state` watches and controls through a hidden observer device, and `track-playback` registers this computer as a device that can play and handles the state machine. The state machine carries only the current and next songs, so the queue shows connect-state's `next_tracks` (hidden looping copies aren't shown, and tracks with only a URI get their names from `decorateContextTracks`); clicking one sends `skip_next` with that track's uid. Local play-next and radio continuation aren't connected to Connect yet.
- **Music videos**: `track-playback/v1/media` is also asked for `manifest_ids_video`, and the `video_version_uri` in the response is this song's video. The video manifest is turned into HLS and played in a visible WebView in the main window (Widevine on Windows, FairPlay on macOS), switching to the video's own audio track. Native controls sit over the video: a transparent layer takes the mouse, and the control bar and subtitles are drawn above the WebView. The data of playlists, albums, artist pages and liked songs already has `associationsV3.videoAssociations`, which is where the list's "MV" tag comes from, with no extra requests.
- **Podcasts**: episodes also use `track-playback/v1/media`, asking for `file_urls_external` as well. Encrypted audio the platform can play comes first. On macOS Spotify gives almost no cbcs for episodes (only cenc, for Widevine), so video podcasts play the audio track from the video manifest instead: a video podcast's video isn't encrypted (H.264 + AAC in fMP4), needs no license on either platform, keeps exactly the same timeline when switching to video, and isn't black in screenshots. Audio-only shows play the original address `file_urls_external` gives (the MP3 in the RSS feed), forwarded by range through the small local service, which follows redirects once and remembers the final address (BBC's takes 3 hops of two or three seconds each). Only shows hosted by Spotify with nothing but Widevine-encrypted audio can't play on macOS: it says so and stops, rather than trying one episode after another. Subtitles are the transcript from `transcript-read-along/v2/episode/<id>` (the same interface the web player uses): each sentence has a start time, and each word a start time and character count. Speakers, chapter titles and "Music playing" don't count as sentences; Chinese transcripts have a space between every two characters, removed for display. An episode's transcript often runs to thousands of sentences, so Now Playing builds only the few dozen rows in view, with the rest taking up their measured height (or one estimated from the text's length if not yet measured).
- **Avatars**: an account's avatar is, in order, the one set on Spotify, the one for the account's email on Gravatar, and the name's initial (on a background of the color Spotify gives). The email comes from asking `www.spotify.com/api/account-settings/v1/profile` with the WebView's cookies.

## Build from source

Needs Go 1.27.3 or later, and no cgo.

```
go test ./...
CGO_ENABLED=0 go build -trimpath -ldflags="-H windowsgui -s -w" -o outputs/MySpotify.exe .
```

The macOS app and dmg use MyGo's own packaging command (run it on a Mac; the output is in `build/darwin-arm64/`, and `-skip-dmg` makes only the app):

```
go tool mygo build
```

The icons are all drawn by `tools/icons.py` (on Windows, with Pillow, using headless Edge to render the SVGs). `resources/icon.svg`/`icon.png` is the program's icon; `icon-small.svg` is a simplified version for 16–24 pixels (two grooves, thicker); `icon.ico` combines the two by size. `internal/app/tray.png` is the template icon for the tray and the Mac menu bar (only black and transparent: drawn white automatically on a dark Windows taskbar, tinted by macOS itself). `AppIcon.icon` is the Icon Composer document for the Mac Dock (light and dark backgrounds, `macos.icon` in `mygo.json`; it's compiled in only when `mygo build` runs on a Mac with Xcode 26, otherwise falling back to an `.icns` made from `icon.png`). After changing the icon or version, run `python tools/icons.py` first, then regenerate `rsrc_windows_amd64.syso`, which holds the icon, version information and manifest (also change the version in `mygo.json`):

```
go run github.com/tc-hib/go-winres@v0.3.3 simply --icon resources/icon.ico --manifest gui --product-name MySpotify --file-description MySpotify --product-version 0.4.13 --file-version 0.4.13 --original-filename MySpotify.exe --arch amd64 --out rsrc
```

### Releasing

macOS arm64 / amd64, Windows amd64 and Linux amd64 are packaged at once on a Mac. Intel Mac support is experimental: cross-compilation and package checks pass, but sign-in, music / MV playback and in-app updates still need testing on physical hardware. Linux is also experimental; see [LINUX.md](LINUX.md) for dependencies and verification scope. In-app updates accept only packages signed with the update private key, so a release must use `mygo build` with the private key; a version from `go build` can't update itself.

1. If `go.mod` changed, update `licenses/` (third-party licenses, shipped with the program) and its `README.txt` to match; `go test` reminds you if you forget.
2. Change the version: `version` in `mygo.json`, and the two version numbers in the go-winres command above (regenerating the syso). The version shown under Settings › About is read directly from `mygo.json`.
3. Write this version's section in [`CHANGELOG.md`](CHANGELOG.md). The release notes under Settings › About come from it, and `mygo build` stops if the section is missing. The changelog stays in Chinese, as the app's interface does.
4. Package in a clean working tree:

   ```sh
   MYGO_UPDATER_PRIVATE_KEY="$(cat "$HOME/Library/Application Support/mygo/update-keys/MySpotify/mygo-update.key")" \
     go tool mygo build -platform darwin/arm64,darwin/amd64,windows/amd64,linux/amd64
   ```

5. Create a `v<version>` release on GitHub. Save it as a draft first, upload all the attachments, and after checking them publish it and set it as Latest. The app reads the manifest in the latest release and can't see drafts, so it never finds a manifest whose archive hasn't finished uploading.
   - `build/darwin-arm64/`: the dmg (renamed to `MySpotify-<version>-macos-arm64.dmg`), `vinyl-<version>-darwin-arm64.tar.gz`, `*.delta`, `update-darwin-arm64.json`
   - `build/darwin-amd64/`: the dmg (renamed to `MySpotify-<version>-macos-amd64.dmg`), `vinyl-<version>-darwin-amd64.tar.gz`, `*.delta` (when earlier versions exist), `update-darwin-amd64.json`
   - `build/windows-amd64/`: put the program files in this folder and a `README.txt` into `MySpotify-<version>-windows-amd64/` and zip it under the same name; add `vinyl-<version>-windows-amd64.tar.gz`, `*.delta`, `update-windows-amd64.json`

Linux attachments include the `.deb`, `.tar.gz`, `update-linux-amd64.json` and `install-linux-webview.sh`; the install script also ships in the app's resources. Desktop users install the dependencies separately.

`*.delta` files are incremental packages from the previous few versions to this one. `mygo build` reads the published versions and makes them automatically, and the app prefers them when updating.

The public key is in `updates` in `mygo.json`. The private key stays only on the Mac used for releases, with permissions 600, and never goes in the repository. If it's lost, installed apps can't receive further updates; if someone else gets it, they can push any code to every user. Keep another copy in a password manager.

Logs (including stack traces from crashes) go to `vinyl.log` in the settings folder. Past 1 MB it's moved to `vinyl.log.1` and a new one starts.

### Debugging

- `NM_SHOTS=<dir>`: walks through the home page, charts, a playlist, Now Playing, the queue and search, taking a screenshot at each step (muted).
- `NM_SHOTS=<dir> NM_SCENE=once`: silently plays a song from the charts for 10 seconds (`NM_PLAY=<playlist/album/podcast URI>` plays the first of that list instead), asking for only one license, and prints the playback state and cache size. Use this first to verify real playback. Add `NM_VIDEO=1` to play the video of a song that has one (music videos, video podcasts), stopping on Now Playing.
- `NM_SHOTS=<dir> NM_SCENE=play`: verifies playback in full (starting, seeking to the end into the next song, pause and resume, next). It takes several licenses, so don't run it repeatedly.
- `NM_SHOTS=<dir> NM_SCENE=readme`: takes the README's screenshots with real data, the account's name shown as Alex (in the home page's titles too), without playing. Then run `python tools/readme_shots.py <dir> docs/screenshots` for the WebPs with rounded corners and a shadow. This scene writes settings and the playback session on exit; to keep yours untouched, point `APPDATA` elsewhere when running it.
- `NM_SHOTS=<dir> NM_SCENE=connect`: a probe for Connect registration that plays no audio and unregisters the test device at the end.
- `NM_SHOTS=<dir> NM_SCENE=identity`: exports the current signed-in identity to `identity.json`, for the LIVE tests below.
- `LIVE=<identity.json> go test -run TestLive ./internal/spotify`: hits every interface with a real identity, read-only.
- `LIVE=<identity.json> LIVE_EDIT=1 go test -run TestLiveEdit ./internal/spotify`: on this account, creates a playlist, adds and removes songs, renames and deletes it, saves albums, playlists and podcasts, likes a song and follows an artist, reading back after each step and finally restoring everything. It really changes the account.
- `LIVE=1 go test -run TestLiveScriptHashes ./internal/spotify`: checks that the query hashes in use can all still be found in the live scripts.

A version built without `-H windowsgui` shows these scenes' output in the console.

## Website

The website is published from `docs/` on `main`, with no build step. `docs/index.html` is Chinese and `docs/en.html` is English; the two have the same structure and share `docs/style.css` and `docs/site.js`. When changing product details, change both pages and both READMEs (`README.md`, `README.en.md`).
