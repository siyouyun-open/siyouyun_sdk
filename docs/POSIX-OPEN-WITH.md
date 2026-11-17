# POSIX Open-With File Grants

Status: runtime contract for the POSIX open-with mode.
Applies to: AppStore-managed V3 container applications.
SDK impact: no data-plane API change is required.

## 1. Why this mode exists

An open-with request grants one application read access to one file. The
permission decision belongs to the Core permission service. The application
must not receive a bearer grant URL and must not call a grant file-read API.

The runtime turns the grant into a local POSIX path inside the application's
container:

```text
open-with
  -> Core issues a file grant
  -> AppStore receives grantPrefix
  -> daemon stages the file in the stable FUSE grants root
  -> AppStore opens the application with ?path=...
  -> application reads the path with ordinary POSIX calls
```

The grant HTTP read endpoints that existed in earlier releases have been
removed. The remaining authority is the Core grant record; the FUSE path is a
short-lived runtime projection.

## 2. URL and filesystem contract

The application entry is opened with a single `path` query parameter:

```text
/web/<l7-hash>/?path=%2Flacx%2Fuserfs%2Fgrants%2Fgrant_xxx%2Fphoto.jpg
```

Decoded path:

```text
/lacx/userfs/grants/<grantPrefix>/<basename>
```

Rules:

- `path` is an absolute path and must start with `/lacx/userfs/`.
- The path is opaque to the application except for the `/lacx/userfs/` prefix.
- The application must not derive a host path, UFI, storage pool path, or
  another user's directory from it.
- The path is only valid for the current grant. It must not be cached,
  persisted, logged as a durable location, or reused after a restart.
- The file is exposed through a read-only FUSE mount. Range reads are
  supported by the filesystem.
- The stable grants root is:

```text
/lacx/userfs/grants
```

The application does not need to know this root; it should use the received
`path` as-is, with URL encoding preserved.

## 3. Session grants versus forever grants

Session and forever are properties of the Core grant, not of the FUSE path.
Both use exactly the same path shape.

| Grant duration | Core representation | Runtime projection |
| --- | --- | --- |
| `session` | `expiresAt = now + session TTL` | file is staged and the expiry event removes it |
| `forever` | `expiresAt = 0` | file remains until revoked or the app is uninstalled |
| `once` | consumed by the first authorized use | not used for normal POSIX open-with |

The FUSE layer distinguishes the two cases with `expiresAt`:

```text
expiresAt == 0  -> forever grant
expiresAt > 0   -> session grant
```

The path itself contains no duration marker. The application must not branch
on session versus forever; it should simply use the path and handle `ENOENT`
or an expired grant like any other unavailable file.

Lifecycle:

- `session`: the Core grant expires; the Core cleanup emits
  `grant.expired`; the daemon removes the staging file; new opens fail.
- `forever`: the Core grant has no expiry; it remains until an explicit
  revoke, app uninstall, or storage loss.
- Explicit revoke: `grant.revoked` removes the file for either duration.
- Daemon restart: the persistent grant store reloads active grants. Active
  session grants are restaged if they are still active; expired session grants
  are not. Forever grants are restaged until revoked.
- A running application may still hold an already-open file descriptor after
  revoke. New opens fail immediately; callers that require abort-on-revoke
  must implement application-level cancellation.

## 4. Manifest requirements

An application that reads open-with files must opt in explicitly:

```yaml
version: v3
app_info:
  id: com.example.viewer
  version: 1.0.0

permissions: []
scopes: []          # only if the app has no other storage scopes

contract:
  open_with:
    access: posix
    kinds: [image]
    ext: [jpg, jpeg, png]
    entry: /
    priority: 100
    verbs: [open]
```

Rules:

- `contract.open_with.access: posix` is mandatory.
- If `access` is absent, AppStore keeps the legacy grant-URL mode. The legacy
  grant HTTP API no longer exists, so a non-migrated application is not
  usable for open-with.
- Directory permissions/scopes used only for open-with must be removed.
  Keep unrelated storage scopes only if the app actually uses them.
- The app version must be bumped and the package redeployed. An installed old
  manifest will keep the old behavior.
- The entry page must preserve the `path` query parameter. A redirect or SPA
  route that drops it breaks the file open.

## 5. Application implementation

### 5.1 Browser/JavaScript

```js
const rawPath = new URLSearchParams(location.search).get('path') || '';
if (!rawPath.startsWith('/lacx/userfs/')) {
  showError('invalid open-with path');
  return;
}

// The application's HTTP server exposes /media/ as an alias of
// /lacx/userfs/. Keep the URL relative so it resolves under the L7 prefix.
const rel = rawPath.slice('/lacx/userfs/'.length);
const mediaURL = 'media/' + rel
  .split('/')
  .filter(Boolean)
  .map(encodeURIComponent)
  .join('/');

video.src = mediaURL;
```

Do not do this:

```js
// removed API
fetch('/syy/v2/app/fs/grant/object?grant=...');

// wrong mount-table lookup
const local = sdk.fs.Load().LocalPath(rawPath);

// absolute URL can resolve against the AppStore origin, not this app
video.src = '/media/photo.jpg';
```

### 5.2 Native process inside the container

```go
rawPath := r.URL.Query().Get("path")
if !strings.HasPrefix(rawPath, "/lacx/userfs/") {
    http.Error(w, "invalid path", http.StatusBadRequest)
    return
}
// Open the path directly. FUSE does the authorization.
f, err := os.Open(rawPath)
if err != nil {
    http.Error(w, "file unavailable", http.StatusNotFound)
    return
}
defer f.Close()
```

Do not translate the path through the SDK mount table. The mount table
contains storage-scope mounts, not open-with grants.

### 5.3 nginx

```nginx
location /media/ {
    alias /lacx/userfs/;
    autoindex off;
    add_header Accept-Ranges bytes always;
}
```

## 6. SDKv2 position

SDKv2 is a bus/capability client plus mount-table helper. It is not part of
the open-with data path.

- No SDKv2 API is required to receive or read a POSIX open-with file.
- Do not add `/lacx/userfs/grants` to the mount table. The grant root is a
  runtime projection, not a storage scope.
- Do not use `fs.Load()` to resolve an open-with path to a UFI.
- SDKv2 applications that need `MountTable.Spaces` must use SDK v2.2.0 or
  later. This is independent of POSIX open-with.
- Native host applications are not covered by the V3 stable-FUSE projection.
  Supporting native open-with would require a separate runtime/SDK capability.

## 7. Migration checklist

- [ ] Set `contract.open_with.access: posix`.
- [ ] Remove open-with directory permissions/scopes.
- [ ] Remove grant URL parsing.
- [ ] Remove `/syy/v2/app/fs/grant/*` calls.
- [ ] Parse `path` and validate the `/lacx/userfs/` prefix.
- [ ] Serve the file through a relative `/media/` alias or direct POSIX read.
- [ ] Keep Range support for media files.
- [ ] Preserve `path` through redirects and SPA routing.
- [ ] Bump the app version and rebuild the `.lacapp` package.
- [ ] Upgrade SDKv2 to v2.2.0+ if the app uses `MountTable.Spaces`.
- [ ] Test add, read, Range, revoke, session expiry, and daemon restart.
