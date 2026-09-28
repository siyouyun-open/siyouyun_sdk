package protocol

// ContractVersion is the version of this contract between the runtime and applications.
//
// This package defines the things both sides must agree on verbatim: scope value syntax
// and containment rules, mount table fields, bus frames and message types, capability
// template names, and response codes.
//
// It has a **byte-identical** copy on each side, host (runtime: minictl / pkg-types) and
// SDK (applications):
//
//   - byte-level consistency is guarded by `scripts/check-protocol-sync.sh` (run in CI; a
//     non-empty diff fails the build);
//   - semantic consistency is guarded by `testdata/contract_vectors.json` — both copies run
//     the same vectors, so whoever changes behavior goes red first;
//   - this constant turns "changing semantics" into an explicit action: any semantic change
//     must bump it, update the vector file, and confirm that old applications do not behave
//     strangely against a new runtime.
//
// Why it is not placed in a shared module: the SDK must be able to exist standalone with 0
// external dependencies (it can be moved wholesale / copied into any application
// repository), and the host should not depend on the application-side SDK just for one
// contract. Keeping a copy on each side and using the three gates above to prevent
// divergence is a better deal than introducing a cross-repository module dependency.
// Version 2 (2026-09): the `infra.qdrant.*` templates left the built-in catalog. They are
// now ordinary App-provided capabilities (declared by the Qdrant App's manifest and called
// with a signature), so a runtime that still hardcodes them and one that does not would
// disagree on signatures and on the call surface.
const ContractVersion = 2
