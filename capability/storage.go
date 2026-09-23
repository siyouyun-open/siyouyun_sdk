package capability

import (
	"context"

	"github.com/siyouyun-open/siyouyun_sdk/v2/bus"
	"github.com/siyouyun-open/siyouyun_sdk/v2/protocol"
)

// Storage is the **control-plane** capability for storage spaces.
//
// The data plane (reading/writing/listing files) goes through local POSIX + the mount table and is
// not here; here there are only platform semantics that POSIX cannot express: space enumeration and
// quota, and the trash.
type Storage struct{ client *bus.Client }

// NewStorage creates a Storage wrapper.
func NewStorage(client *bus.Client) *Storage { return &Storage{client: client} }

// GroupSpace is a user/group space and its quota.
type GroupSpace struct {
	Namespace      string `json:"namespace"`
	NamespaceAlias string `json:"namespaceAlias"`
	PoolName       string `json:"poolName"`
	Quota          uint64 `json:"quota"`
}

// UserSpaces lists the spaces available to the current user (personal and group) and their quotas.
func (s *Storage) UserSpaces(ctx context.Context, scope Scope) ([]GroupSpace, error) {
	var spaces []GroupSpace
	if err := callEnvelope(ctx, s.client, &bus.Request{
		Template: protocol.TemplateStorageUserSpaces,
		Payload:  scope.payload(),
	}, &spaces); err != nil {
		return nil, err
	}
	return spaces, nil
}

// Trash moves a file to the trash.
//
// This is the only operation of the FS control plane that stays on the bus: the trash is a platform
// semantic (tied to Core's metadata and restore flow) that local POSIX unlink/rename cannot express.
func (s *Storage) Trash(ctx context.Context, scope Scope, ufi string) error {
	payload := scope.payload()
	payload["ufi"] = ufi
	return callEnvelope(ctx, s.client, &bus.Request{
		Template: protocol.TemplateFSTrash,
		Payload:  payload,
	}, nil)
}
