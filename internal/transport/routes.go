package transport

import "strings"

// RestPathToTemplate maps a REST method+path pair to a bus capability template.
func RestPathToTemplate(method, path string) string {
	path = strings.TrimPrefix(path, "/syy")

	mappings := map[string]string{
		// FS (Core :40100)
		"POST /v2/app/file/open":         "system.fs.open",
		"POST /v2/app/file/close":        "system.fs.close",
		"POST /v2/app/file/read":         "system.fs.read",
		"POST /v2/app/file/read/at":      "system.fs.read",
		"POST /v2/app/file/write":        "system.fs.write",
		"POST /v2/app/file/write/at":     "system.fs.write",
		"POST /v2/app/file/write/string": "system.fs.write",
		"POST /v2/app/file/seek":         "system.fs.seek",
		"POST /v2/app/file/name":         "system.fs.name",
		"POST /v2/app/file/readdir":      "system.fs.readdir",
		"GET  /v2/app/file/readdirnames": "system.fs.readdir",
		"POST /v2/app/file/stat":         "system.fs.stat",
		"POST /v2/app/file/sync":         "system.fs.sync",
		"POST /v2/app/file/truncate":     "system.fs.truncate",
		"GET  /v2/app/fs/object/list":    "system.fs.list",
		"POST /v2/app/fs/mkdir/all":      "system.fs.mkdir",
		"POST /v2/app/fs/remove":         "system.fs.remove",
		"POST /v2/app/fs/remove/all":     "system.fs.removeall",
		"POST /v2/app/fs/rename":         "system.fs.rename",
		"POST /v2/app/fs/copy":           "system.fs.copy",
		"POST /v2/app/fs/exists":         "system.fs.exists",
		"POST /v2/app/fs/object/detail":  "system.fs.detail",
		"POST /v2/app/fs/chtimes":        "system.fs.chtimes",
		"POST /v2/app/fs/trash/move":     "system.fs.remove",

		// KV (Core :40100)
		"POST /v2/app/kv/put":        "system.kv.put",
		"GET  /v2/app/kv/value":      "system.kv.get",
		"GET  /v2/app/kv/values":     "system.kv.list",
		"POST /v2/app/kv/delete":     "system.kv.delete",
		"POST /v2/app/kv/sys/put":    "system.kv.sys.put",
		"GET  /v2/app/kv/sys/value":  "system.kv.sys.get",
		"GET  /v2/app/kv/sys/values": "system.kv.sys.list",
		"POST /v2/app/kv/sys/delete": "system.kv.sys.delete",

		// Schedule (Core :40100)
		"POST /v2/app/schedule/once/create": "system.schedule.once.create",
		"POST /v2/app/schedule/once/update": "system.schedule.once.update",
		"POST /v2/app/schedule/cron/create": "system.schedule.cron.create",
		"POST /v2/app/schedule/cron/update": "system.schedule.cron.update",

		// Message (Core :40100)
		"POST /v2/app/msg/session/send": "system.msg.send",

		// Core health
		"HEAD /status/ready": "system.core.ready",
		"GET  /status/ready": "system.core.ready",

		// FFmpeg (OS :40000)
		"GET /os/codec/ffmpeg/info": "system.ffmpeg.info",

		// FS UGNs (OS :40000)
		"GET /os/fs/group/space/list": "system.fs.ugns",

		// OS
		"POST /os/app/event/register":       "system.os.event.register",
		"GET  /os/app/event/config":         "system.os.event.config",
		"GET  /os/app/event/file/exist":     "system.os.event.file.exist",
		"GET  /os/app/info":                 "system.os.info",
		"GET  /os/status/ready":             "system.os.ready",
		"GET  /os/device/service-endpoints": "system.os.endpoints",
		"POST /os/app/robot/register":       "system.os.event.register",
		"POST /os/storage/restore/meta":     "system.storage.restore",
	}

	return mappings[method+" "+path]
}
