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
	SiyouSysPool         = "syspool"
	UserHomeDir          = "/home"
)

type MediaType string

// Standard media types
const (
	MediaTypeText    MediaType = "text"
	MediaTypeImage   MediaType = "image"
	MediaTypeAudio   MediaType = "audio"
	MediaTypeVideo   MediaType = "video"
	MediaTypeMessage MediaType = "message"
)

// Custom media types
const (
	MediaTypeAll        MediaType = "all"         // all types
	MediaTypeCompress   MediaType = "compress"    // package type
	MediaTypeImageVideo MediaType = "image-video" // image and video types
	MediaTypeDoc        MediaType = "doc"         // doc type
	MediaTypeBt         MediaType = "bt"          // BitTorrent type
	MediaTypeEbook      MediaType = "ebook"       // ebook type
	MediaTypeSoftware   MediaType = "software"    // soft type
	MediaTypeOther      MediaType = "other"       // other type
)

// file event types
const (
	FileEventAdd    = iota + 1 // file add
	FileEventDelete            // file deleted
	FileEventRename            // file rename
)

type ConsumeStatus int

const (
	ConsumeSuccess ConsumeStatus = iota + 1
	ConsumeRetry
	ConsumeFail
)

type TaskLevel int

// task level
const (
	HighLevel TaskLevel = iota + 1
	MidLevel
	LowLevel
)

// sdk app kv
const (
	DefaultAppKeyType = "default"
	AppDataVersionKey = "dataVersion"
)
