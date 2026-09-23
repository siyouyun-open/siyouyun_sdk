# Siyouyun FaaS SDK (v2)

The SDK for writing native applications for the Siyouyun NAS platform. It is the client side of
the runtime↔application contract: the bus client, the typed capability wrappers, the mount-table /
space view, and the wire protocol definitions.

**This branch (`v2`) is a rewrite, not an evolution of v1.** The two lines coexist in this
repository: `main` carries v1 (`github.com/siyouyun-open/siyouyun_sdk`, used by older apps) and
`v2` carries the SDK described here. The module path of this line is
`github.com/siyouyun-open/siyouyun_sdk/v2`, so both can be imported side by side.

## Install

```bash
go get github.com/siyouyun-open/siyouyun_sdk/v2@v2.1.0
```

Optional add-ons are separate modules, so the core stays dependency-free:

```bash
go get github.com/siyouyun-open/siyouyun_sdk/contrib/restjson@v0.1.0    # {code,data,msg} envelope for iris handlers
go get github.com/siyouyun-open/siyouyun_sdk/contrib/watchdirs@v0.1.0   # expose the file-event watch scope to a UI
```

## Layout

| Path | What it is |
| --- | --- |
| `appsdk.go` | runtime environment resolution and client assembly (package root) |
| `space.go` | space validation and the space view (`App.Space`, identity headers) |
| `bus/` | bus client: unix socket + websocket transport, session, reconnect, subscriptions |
| `capability/` | typed wrappers over the capability templates (KV, events, platform, media, storage) |
| `fs/` | mount-table parsing and the local POSIX data plane (scopes, path resolution) |
| `protocol/` | the contract: scope grammar, mount table, bus frames, capability catalog, codes |
| `appconst/`, `apperr/`, `paging/` | shared constants, i18n errors, minimal pagination |
| `bustest/` | an in-process bus test server for application tests |
| `contrib/restjson/` | HTTP response envelope (`{code,data,msg}`), separate module (depends on iris) |
| `contrib/watchdirs/` | routes that read/write an app's file-event watch scope, separate module |

The SDK depends on exactly one third-party module in its core (`gorilla/websocket`, pure Go,
no cgo) so that an application can pull it in without dragging a dependency tree along.

## The contract is duplicated on purpose

`protocol/` defines what the runtime and the application must agree on byte-for-byte: the scope
value grammar, the mount-table fields, the bus message and frame types, the capability template
names, and the response codes. The platform's runtime repository keeps a **byte-identical copy**
of this package, and the two copies are held together by three guards:

1. `ContractVersion` (in `protocol/contract.go`) must be bumped on any semantic change;
2. `protocol/testdata/contract_vectors.json` is the shared, executable description of the
   contract — both implementations run the same vectors, so whichever one changes behaviour
   turns red first;
3. a sync script (`scripts/check-protocol-sync.sh` in the runtime repository) diffs the copies
   and fails on any byte difference.

Why duplicate instead of sharing a module: the SDK must stay independently usable (no
dependency on the runtime repository), and the runtime must not depend on the SDK. Both copies
are small, dependency-free, and covered by the same vectors.

If you change the contract: edit the runtime's copy first (it is the authority for what the
runtime accepts), bump `ContractVersion`, update the vectors, run the sync script, and only then
publish a new SDK version.

## Versioning

- This line is v2+: the module path ends in `/v2` and releases are tagged `v2.x.y` on the `v2`
  branch.
- Add-on modules are versioned independently: tag `contrib/restjson/vX.Y.Z` and
  `contrib/watchdirs/vX.Y.Z`.
- A change that alters the contract (scope grammar, mount-table fields, frame or template
  vocabulary, response codes) is a **breaking** change: bump `ContractVersion` and, in the SDK,
  the major version.
- `docs/APPSDK.md` in this repository is the long-form manual (bus, spaces, capabilities,
  events, error handling, the mount table).

## License

Apache License 2.0 — see [LICENSE](LICENSE).
