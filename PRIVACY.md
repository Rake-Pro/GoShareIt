# Privacy Policy

Effective 2026-08-22. Applies to the GoShareIt desktop application for macOS,
Windows, and Linux, and to the GoShareIt project as a whole.

## Summary

GoShareIt does not collect, store, or transmit any personal data to the
project maintainers or to any third party. There is no telemetry, no
analytics, no crash reporting, no account, and no advertising. The
maintainers have no way to see what you capture or where you send it.

## What the app does with your data

- **Captures** (screenshots, recordings, GIFs) are created on your machine
  and stay there unless you configure an upload destination. Local copies,
  the clipboard, and the capture history log are all written under your
  user profile (`~/.goshareit` on macOS/Linux, `%USERPROFILE%\goshareit` on
  Windows) and never leave the machine on their own.
- **Uploads** go only to a server you configure yourself (Nextcloud, WebDAV,
  S3-compatible storage, SFTP, or a custom HTTP endpoint). That transfer is
  between your computer and your chosen provider under that provider's own
  privacy terms. Upload can be turned off entirely ("local-only mode"), in
  which case the app makes no upload connections at all.
- **Credentials** for your upload destination are stored locally in files
  under the app root (or read from environment variables you set) and are
  sent only to that destination. The optional Nextcloud browser sign-in
  talks only to the Nextcloud server you entered.
- **Update checks** query the public GitHub Releases API for this repository
  (`https://api.github.com`) anonymously to learn whether a newer version
  exists. No identifier is sent beyond what any HTTPS request carries (your
  IP address and a user agent), and GitHub's own privacy policy applies to
  that request. Update checks can be disabled in Settings.
- **Logs** are written locally (`goshareit.log` in the app root) for your
  own troubleshooting and are never transmitted.

## What the maintainers receive

Nothing from the app. If you open an issue or discussion on GitHub, GitHub
processes that under its terms; do not paste credentials or private
captures into issues.

## Third parties

The only network endpoints the app contacts are the upload destination you
configure and, if enabled, `api.github.com` for update checks. The app
bundles no third-party SDKs that phone home.

## Children

GoShareIt is a general-purpose utility and is not directed at children; it
collects no data from anyone.

## Changes

Changes to this policy are recorded in the repository history and noted in
[CHANGELOG.md](CHANGELOG.md).

## Contact

Open an issue at <https://github.com/Rake-Pro/GoShareIt/issues>.
