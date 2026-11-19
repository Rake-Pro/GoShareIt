# Privacy Policy

Effective 2026-11-18. Applies to the GoShareIt desktop application for macOS,
Windows, and Linux, and to the GoShareIt project as a whole.

## Summary

GoShareIt does not collect, store, or transmit any personal data to the
project maintainers or to any third party. There is no telemetry, no
analytics, no crash reporting, no account, and no advertising. The
maintainers have no way to see what you capture or where you send it.

## What the app does with your data

- **Captures** (screenshots, recordings, GIFs) are created on your machine
  and stay there unless you configure an upload destination. Local copies
  (when saving is turned on) go to the folder you choose, by default
  `Pictures/GoShareIt` in your home folder. The capture history log is
  written under the app root (`~/.goshareit` on macOS/Linux,
  `%USERPROFILE%\goshareit` on Windows). None of these leave the machine on
  their own.
- **Uploads** go only to a server you configure yourself (Nextcloud, WebDAV,
  S3-compatible storage, SFTP, or a custom HTTP endpoint). That transfer is
  between your computer and your chosen provider under that provider's own
  privacy terms. Upload can be turned off entirely ("local-only mode"), in
  which case the app makes no upload connections at all.
- **Credentials** for your upload destination are stored locally in files
  under the app root (or read from environment variables you set) and are
  sent only to that destination. If the destination redirects an upload to
  another host, the credentials and custom headers are not sent along, and a
  redirect from https:// to http:// is refused. The optional Nextcloud
  browser sign-in talks only to the Nextcloud server you entered; the sign-in
  page that server names opens in your default browser.
- **Update checks** query the public GitHub Releases API
  (`https://api.github.com`) anonymously for the configured repository
  (`update.repo`, this repository by default) to learn whether a newer
  version exists. The opt-in dev channel also reads a small manifest from
  `https://github.com`. Downloading an update fetches the release files
  through the same API, which redirects to GitHub's release-asset storage
  (`*.githubusercontent.com`). No identifier is sent beyond what any HTTPS
  request carries (your IP address and a user agent), and GitHub's own
  privacy policy applies to those requests. Update checks can be disabled
  in Settings.
- **Text recognition** (text selection, Copy text, Quick redact in the editor,
  and Capture Text from the tray or its hotkey) runs entirely on your
  machine: Apple Vision on macOS, Windows OCR on Windows, and the
  `tesseract` command you installed on Linux (as a local subprocess fed over
  stdin/stdout). Recognized text goes only to your
  clipboard when you copy it (Capture Text copies it right away; the region
  it captured is not saved). While the editor is open, the last text you
  copied is also held in a private temporary file (readable only by you) so
  the app can keep it on the clipboard after the editor closes; that file is
  deleted when the editor closes. Recognized text is not kept anywhere else,
  not added to the capture history, and never sent anywhere.
- **Logs** are written locally (`goshareit.log` in the app root) for your
  own troubleshooting and are never transmitted.

## What the maintainers receive

Nothing from the app. If you open an issue or discussion on GitHub, GitHub
processes that under its terms; do not paste credentials or private
captures into issues.

## Third parties

The only network endpoints the app contacts are the upload destination you
configure (and, for the Nextcloud browser sign-in, the server you entered)
and, if update checks are enabled, GitHub: `api.github.com`, `github.com`
(dev channel manifest) and GitHub's release-asset storage
(`*.githubusercontent.com`) for downloads. The app bundles no third-party
SDKs that phone home.

## Children

GoShareIt is a general-purpose utility and is not directed at children; it
collects no data from anyone.

## Changes

Changes to this policy are recorded in the repository history and noted in
[CHANGELOG.md](CHANGELOG.md).

## Contact

Open an issue at <https://github.com/Rake-Pro/GoShareIt/issues>.
