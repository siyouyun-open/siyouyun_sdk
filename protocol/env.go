package protocol

// Names of the environment variables injected into application processes.
//
// native is a host process, and the runtime announces everything about it — "where I am,
// who I am, where the bus is" — through environment variables. The names live here so that
// the daemon (injection side) and the SDK (read side) share them, avoiding a change on one
// side that forgets the other.
//
// Container applications get a **different set** (for historical reasons, see the
// EnvContainer* block below): it has no MINI_APP_* prefix, and no instance ID / install
// directory / caller token — those can be derived from the container's own bundle and from
// "the source address is the identity". The SDK is responsible for normalizing both sets
// into a single Runtime.
const (
	// EnvAppCode is the application code (the install directory name).
	EnvAppCode = "APPCODE"
	// EnvAppID is the application id (file event topics are scoped by it).
	EnvAppID = "MINI_APP_ID"
	// EnvCode is an explicit alias for the application code (same value as EnvAppCode).
	EnvCode = "MINI_APP_CODE"
	// EnvNamespace is the space the instance belongs to; for native it is always default.
	EnvNamespace = "MINI_APP_NAMESPACE"
	// EnvRuntime is the runtime kind (native).
	EnvRuntime = "MINI_APP_RUNTIME"
	// EnvInstanceID is the stable instance ID.
	EnvInstanceID = "MINI_APP_INSTANCE_ID"

	// EnvBusSocket is the absolute path of the bus UDS (the preferred channel for RPC +
	// subscriptions).
	EnvBusSocket = "MINI_APP_BUS_SOCKET"
	// EnvBusHTTP is the daemon bus HTTP base address (file/streaming capabilities and
	// fallback RPC).
	EnvBusHTTP = "MINI_APP_BUS_HTTP"
	// EnvBusWS is the daemon bus WebSocket address (optional channel for event subscriptions).
	EnvBusWS = "MINI_APP_BUS_WS"
	// EnvCallerToken is the caller token issued by the daemon; it restores the caller
	// identity over HTTP/WS.
	// It only identifies the caller, and is not an authorization credential.
	EnvCallerToken = "MINI_APP_CALLER_TOKEN"

	// EnvEndpoint is this instance's listen address (http://127.0.0.1:<port>).
	EnvEndpoint = "MINI_APP_ENDPOINT"
	// EnvPort is this instance's listen port.
	EnvPort = "MINI_APP_PORT"
	// EnvInstallRoot is the install directory (versioned, not a persistent data location).
	EnvInstallRoot = "MINI_APP_INSTALL_ROOT"
	// EnvStaticRoot is the static asset directory.
	EnvStaticRoot = "MINI_APP_STATIC_ROOT"
	// EnvInstallSpace is the storage space chosen by the user (may be empty).
	EnvInstallSpace = "MINI_APP_INSTALL_SPACE"
	// EnvDataDir is the platform-managed application data directory; injected only when
	// storage.app.file is declared.
	EnvDataDir = "MINI_APP_DATA_DIR"
	// EnvLegacyNamespace is the space variable kept for compatibility with old SDKs.
	EnvLegacyNamespace = "MINI_NAMESPACE"

	// EnvStoragePoolPrefix is the prefix of storage pool path variables:
	// NATIVE_STORAGE_POOL_<NAME>=<abs path>, injected only when storage.user.file is declared.
	EnvStoragePoolPrefix = "NATIVE_STORAGE_POOL_"

	// RuntimeNative is the EnvRuntime value for native processes.
	RuntimeNative = "native"
	// DefaultNamespace is the space of native instances.
	DefaultNamespace = "default"
)

// Runtime environment variables for container (v3/runc) applications.
//
// The difference from native is not "a few fewer variables" but a **different identity
// model**:
//
//   - a container naturally belongs to one user space, and the daemon can derive the
//     namespace from its network namespace/IP, so there is no MINI_APP_NAMESPACE, only
//     MINI_NAMESPACE (the same name as the old SDK);
//   - a container is that one app, and there is no "one process serving multiple spaces",
//     so there is no instance ID;
//   - its identity towards the bus comes from the socket and the source address, so no
//     caller token is needed;
//   - the install directory is fixed inside the container (/ or the working directory), so
//     there is no need to inject it.
//
// These variables are injected by the container runtime (runtime.GetDefaultConfig); the SDK
// read side shares the same constants.
const (
	// EnvContainerRuntime is the container runtime kind variable; its value is RUNC (see
	// RuntimeContainer).
	EnvContainerRuntime = "APP_RUNTIME"
	// RuntimeContainer is the value of EnvContainerRuntime.
	RuntimeContainer = "RUNC"

	// EnvContainerBusSocket is the fixed path of the bus socket inside the container.
	//
	// It is provided by a deploy-time bind-mount (host <base>/bus/<namespace>/<containerID> →
	// container /bus), so it does **not** need an environment variable to announce it; the
	// path is part of the contract.
	EnvContainerBusSocket = "/bus/app.sock"

	// EnvContainerUsername is the username the container belongs to (the result of
	// namespaceToUsername).
	EnvContainerUsername = "MINI_USERNAME"
)

// ContainerRuntimeKind is the value of Runtime.RuntimeKind for containers (lowercase,
// symmetric with native).
const ContainerRuntimeKind = "runc"

// Bus-related HTTP headers.
const (
	// HeaderCallerToken declares the native caller identity (its value is EnvCallerToken).
	HeaderCallerToken = "X-Mini-Caller-Token"
	// HeaderLocalToken is the daemon's local shared token (used by AppStore/CLI); it is not
	// an application identity.
	HeaderLocalToken = "X-Minictl-Token"
	// HeaderFunctionTemplate declares the template in file/streaming HTTP calls.
	HeaderFunctionTemplate = "X-Function-Template"
	// HeaderFunctionSignature declares a custom signature in file/streaming HTTP calls.
	HeaderFunctionSignature = "X-Function-Signature"
	// HeaderProviderAppID pins the call to a specific provider App.
	HeaderProviderAppID = "X-Provider-App-ID"
	// HeaderStreamMode takes the streaming call path when it is "true".
	HeaderStreamMode = "X-Stream-Mode"
	// HeaderNamespace is the routing scope (not an identity).
	HeaderNamespace = "X-Namespace"
)

// HTTP paths.
const (
	// PathCapabilityCall is the capability call endpoint (shared by JSON / multipart / streaming).
	PathCapabilityCall = "/api/v1/capability/call"
	// PathCapabilityList is the capability list endpoint.
	PathCapabilityList = "/api/v1/capability/list"
	// PathCapabilityDiscover is the capability discovery endpoint.
	PathCapabilityDiscover = "/api/v1/capability/discover"
	// PathBusWS is the bus WebSocket endpoint.
	PathBusWS = "/api/bus/ws"
)
