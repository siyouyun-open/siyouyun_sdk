package sdkconst

const (
	Success     = "A00200"
	ParamError  = "A00400"
	NotFound    = "A00404"
	NotReady    = "A00460"
	ServerError = "A00500"
	RPCError    = "A00600"
)

const (
	UsernameHeader  = "x-username"
	GroupNameHeader = "x-group-name"
	NamespaceHeader = "x-namespace"
)

const (
	MainNamespace        = "main"
	PrivateNamespace     = "private"
	CommonNamespace      = "common"
	SiyouFSMysqlDBPrefix = "siyou"

	SiyouFSMountPrefix = "/siyouyun/mnt"
	UserSpacePrefix    = "user_"
)

const (
	CoreServiceURL   = "http://10.62.0.1:40100/syy"
	OSURL            = "http://10.62.0.1:40000/os"
	UnixSocketFile   = "/siyouyun/unix-socket/syy_os_file.socket"
	AIServiceURL     = "10.62.0.1:40051"
	MilvusServiceURL = "10.62.0.1:19530"
)
