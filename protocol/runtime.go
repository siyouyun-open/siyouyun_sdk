package protocol

// ContainerRoot 是容器内 AppStore 运行时的统一父级。
//
// 应用可见的私有数据、用户文件、RuntimeSpec、secrets、bus 和插件
// 都收口在 /lacx 下；/lacx 之外的 /tmp 只作为临时缓存。
const (
	ContainerRoot = "/lacx"

	// ContainerDataDir 是应用私有持久化目录（RuntimeSpec paths.data）。
	ContainerDataDir = ContainerRoot + "/data"
	// ContainerUserFSDir 是用户空间授权的逻辑根（FileProvider/FUSE 导出）。
	ContainerUserFSDir = ContainerRoot + "/userfs"
	// ContainerRuntimeDir 是只读平台运行资源目录。
	ContainerRuntimeDir = ContainerRoot + "/run"
	// ContainerSecretsDir 是平台秘密预留目录（只读）。
	ContainerSecretsDir = ContainerRoot + "/secrets"
	// ContainerBusDir 是平台 bus UDS 目录。
	ContainerBusDir = ContainerRoot + "/bus"
	// ContainerPluginsDir 是动态插件池。
	ContainerPluginsDir = ContainerRoot + "/plugins"
	// ContainerResourceDir 兼容旧名；值已统一到 /lacx/run。
	ContainerResourceDir = ContainerRuntimeDir
	// UFIPoolPrefix 是 UFI pool 前缀：/ufi/pool-raw/<pool>/<rel>。
	UFIPoolPrefix = "/ufi/pool-raw/"

	// RuntimeSpecVersion 是 RuntimeSpec v1 的唯一版本号。
	//
	// 与 docs/specs/runtime-spec-v1.schema.json 的 const、SDK v2 runtime.Version
	// 保持一致。多空间/UFI 映射字段是 additive 扩展（不升版本）：
	//
	//	spaces          []string   被本实例服务的部署空间列表（仅有 core active scope grants 的空间）
	//	mounts[].space  string     该 mount 所属部署空间（native 多空间；容器可空）
	//	mounts[].pool   string     存储池名（如 syspool；非池 scope 为空）
	//	mounts[].root   string     scope 在池内的相对根（直接来自 core grant UFI 的池内
	//	                           前缀，如 Music/Photos；整空间为 ""/"；不是按 scope key 猜）
	//	grants[].space  string     grant 所属部署空间（多空间时用于和 scope 一起定位）
	//
	// 注意：mounts[].expiresAt / grants[].expiresAt 是 Unix **毫秒**（与 core grant 同单位），
	// 0 表示不过期（容器侧 scheduleGrantExpiry 与 native 都按毫秒消费）。
	//
	// 老 writer 不写这些字段；reader 必须容忍缺失（回退 app.namespace / 无池信息）。
	RuntimeSpecVersion = 1
)
