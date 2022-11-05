package sdkconst

const (
	Success = "A00200"

	ParamError = "A00400"
	NotFound   = "A00404"
	NotReady   = "A00460"

	ServerError = "A00500"
	RPCError    = "A00600"
)

const (
	UsernameHeader  = "x-username"
	NamespaceHeader = "x-namespace"
)

const (
	MainNamespace        = "main"
	PrivateNamespace     = "private"
	CommonNamespace      = "common"
	SiyouFSMysqlDBPrefix = "siyou"

	FaasMntPrefix = "/siyouyun"
)
