// Package appconst holds the shared constants a v2 application needs: response codes, identity
// header names, space names, and file-event actions.
//
// The values are unchanged from the old SDK's `pkg/const` (package name sdkconst): response codes
// and header names are platform contracts used by clients, Core, and OS, so switching SDKs must not
// incidentally switch protocols. When migrating, an app only needs to replace the import path and
// can keep the `sdkconst` alias unchanged.
package appconst

// Response codes: kept consistent with the rest of the platform (Core / OS envelopes).
const (
	Success     = "A00200"
	ParamError  = "A00400"
	NotFound    = "A00404"
	NotReady    = "A00460"
	ServerError = "A00500"
	RPCError    = "A00600"
)

// Identity headers. After a request is authenticated by Caddy, the platform injects these, and the
// app uses them to determine the current user and space.
//
// Note: these headers are a trust boundary established by **platform injection**; the app-side mount
// table check (see SpaceFromHeaders and App.Space in the appsdk root package) is a second gate
// against forgery/absence, and you must not rely on the headers alone.
const (
	UsernameHeader  = "x-username"
	GroupNameHeader = "x-group-name"
	NamespaceHeader = "x-namespace"
)

// Space names.
const (
	MainNamespace    = "main"
	PrivateNamespace = "private"
	CommonNamespace  = "common"
)

// File event actions (matching the action values of the protocol event body).
const (
	FileEventAdd    = iota + 1 // add
	FileEventDelete            // delete
	FileEventRename            // rename
)
