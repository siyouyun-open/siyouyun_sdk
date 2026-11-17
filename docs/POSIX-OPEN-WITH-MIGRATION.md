# Open-With POSIX Migration Specification

## 1. Scope

This document specifies how an existing AppStore application moves from the
removed grant-URL/grant-HTTP open-with mode to the POSIX FUSE open-with mode.

It is intentionally app-focused. The Core grant service, the AppStore launcher,
and the minictl stable FUSE grants root are already implemented; the remaining
work for each app is manifest, URL handling, and file serving.

## 2. Required runtime contract

```text
AppStore open-with entry
  query parameter: path
  path value: /lacx/userfs/grants/<grantPrefix>/<basename>
  file content: read through POSIX/FUSE
  authorization: Core grant, not the URL
  revoked/expired: file removed from the grants root
```

The application must not receive or use:

```text
grant=...
/syy/v2/app/fs/grant/meta
/syy/v2/app/fs/grant/object
/syy/v2/app/fs/grant/list
/syy/v2/app/fs/grant/stat
```

## 3. Manifest migration

Before:

```yaml
permissions: [storage.file]
scopes:
  space.photos: read
contract:
  open_with:
    kinds: [image]
```

After:

```yaml
permissions: []
scopes: []
contract:
  open_with:
    access: posix
    kinds: [image]
```

If the app also uses non-open-with storage, keep those scopes and permissions;
only remove the directory grants that existed solely to read the open-with
file.

Mandatory:

```yaml
contract:
  open_with:
    access: posix
```

Bump `app_info.version`, repack, and reinstall. The installed manifest is what
AppStore reads; changing source without upgrading the installed package does
not migrate existing devices.

## 4. Application URL handling

### 4.1 Required behavior

- Read the `path` query parameter.
- Reject missing or non-`/lacx/userfs/` paths.
- Use the path for the current request only.
- Preserve `path` across internal navigation and redirects.
- Do not expose the path in analytics or durable logs.

### 4.2 Browser mapping

```js
const path = new URLSearchParams(location.search).get('path') || '';
const rel = path.startsWith('/lacx/userfs/')
  ? path.slice('/lacx/userfs/'.length)
  : '';
const media = 'media/' + rel.split('/').filter(Boolean)
  .map(encodeURIComponent).join('/');
```

Use a relative `media/` URL. An absolute `/media/` URL resolves against the
AppStore origin rather than the app's L7 prefix.

### 4.3 Server mapping

For a Go/Python/Node server inside the V3 container, open the received path
directly:

```text
os.Open("/lacx/userfs/grants/...")
```

The FUSE mount provides authorization and read-only semantics. Do not copy the
file into app-private storage and do not try to resolve it through the mount
table.

## 5. File serving

The recommended pattern is an app-owned HTTP route:

```nginx
location /media/ {
    alias /lacx/userfs/;
    autoindex off;
    add_header Accept-Ranges bytes always;
}
```

Requirements:

- `Content-Type` is derived from the file extension or file content.
- `Range` requests return `206` and `Content-Range`.
- No directory listing.
- No direct static directory fallback outside `/media/`.
- No grant HTTP proxy.

## 6. Session versus forever semantics

The app does not need to distinguish session from forever in the URL. The
runtime does:

```text
session grant: expiresAt > 0; Core emits grant.expired; file removed
forever grant: expiresAt == 0; remains until revoke/uninstall
```

Both grants use the same path shape and the same app code.

Session expiry is eventually consistent with Core cleanup. Applications
should handle `ENOENT`/404 as an expired-but-not-yet-cleaned or revoked file.
For strict per-open expiry checks, a future runtime capability is required;
this migration does not add one.

## 7. AppStore embedded applications

The AppStore-managed V3 media applications are the reference implementation:

```text
apps/lacx-media/video
apps/lacx-media/image
```

They have:

```yaml
permissions: []
contract.open_with.access: posix
```

and use:

```text
?path=...
media/<relative path>
location /media/ { alias /lacx/userfs/; }
```

No AppStore preinstall app is used for open-with media; the preinstall list does
not contain the video/image viewers. The AppStore launcher's built-in media
preview is not an open-with handler and does not use grant paths.

## 8. Acceptance tests

For each migrated app:

```text
1. open-with returns ?path=... and no grant parameter
2. no request goes to /syy/v2/app/fs/grant/*
3. media returns 200
4. Range returns 206
5. revoke returns 404 within the event window
6. session grant expires and returns 404
7. forever grant survives app restart and daemon restart
8. grant add does not restart the app
9. app restart does not expose a stale path
10. manifest in the installed app has access: posix and no open-with-only scopes
```

## 9. Rollback policy

There is no runtime fallback to grant HTTP. Rolling back an app package to a
grant-URL build is not supported against this runtime. Rollback means restoring
the previous app build only if it already uses POSIX open-with.
