# tmpfox

*tmpfox* is a *Firefox* wrapper that:

1. Creates a temporary *Firefox* profile
2. Installs `user.js` configuration file from [Arkenfox](https://github.com/arkenfox/user.js) for increased privacy and security
3. Installs extensions [uBlock Origin](https://addons.mozilla.org/en-US/firefox/addon/ublock-origin/), [ClearURLs](https://addons.mozilla.org/en-US/firefox/addon/clearurls/), [Simple Temporary Containers](https://addons.mozilla.org/en-US/firefox/addon/simple-temporary-containers/), [Bypass Twitter login wall](https://addons.mozilla.org/en-US/firefox/addon/bypass-twitter-login-wall/), and any other extension you specify
4. Launches *Firefox*

Installed extensions are not enabled. *tmpfox* sets the homepage to `about:addons` so you can easily enable them manually once *Firefox* starts.

The temporary profile is deleted on exit (unless you use the flag `-keep`).

## Usage

The above describes the default behavior without any command line options.

If you want to install more extensions, you should use the `-ext` option with the extension's slug as argument.
The slug is the last part of the URL of the extension, e.g. for `https://addons.mozilla.org/en-US/firefox/addon/privacy-badger17/` the slug is `privacy-badger17`:

If you want to install the extensions *Privacy Badger* and *Firefox Multi-Account Containers* (in addition to the recommended extensions):
```sh
tmpfox -ext privacy-badger17 -ext multi-account-containers
```

If you don't want to install the recommended extensions, but only *uBlock Origin* and *Firefox Multi-Account Containers*:
```sh
tmpfox -ext-no-rec -ext ublock-origin -ext multi-account-containers
```

If you don't want to download a `user.js` file:
```sh
tmpfox -userjs ""
```

Type `tmpfox -h` for a description of all the options.

## Supported platforms

*Linux*, *Windows 8+*, *macOS 10.13+*

## Installation

### Option 1: Download release binary (recommended)

Download the latest [release](https://github.com/cmitsakis/tmpfox/releases) and run it. No installation is required.

#### macOS

On *macOS* you have to remove the application from *quarantine* by following the instructions [here](https://support.apple.com/guide/mac-help/welcome/mac), or by running the following command:

```sh
xattr -d com.apple.quarantine /path/to/tmpfox
```

### Option 2: Build from source

If you have installed [Go](https://golang.org/), you can install *tmpfox* by running the following command:

```sh
go install go.mitsakis.org/tmpfox@latest
```

## License

Copyright (C) 2021 Charalampos Mitsakis (go.mitsakis.org/tmpfox)

*tmpfox* is licensed under the [EUPL-1.2-or-later](LICENSE).
