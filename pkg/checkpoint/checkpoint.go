package checkpoint

import (
	"encoding/json"

	"k8s.io/kubernetes/pkg/kubelet/checkpointmanager/checksum"

	"github.com/SchSeba/dra-driver-sriov/pkg/types"
)

type Checkpoint struct {
	Checksum checksum.Checksum `json:"checksum"`
	V1       *CheckpointV1     `json:"v1,omitempty"`
}

type CheckpointV1 struct {
	PreparedClaims types.PreparedClaims `json:"preparedClaims,omitempty"`
}

func NewCheckpoint() *Checkpoint {
	pc := &Checkpoint{
		Checksum: 0,
		V1: &CheckpointV1{
			PreparedClaims: make(types.PreparedClaims),
		},
	}
	return pc
}

func (cp *Checkpoint) MarshalCheckpoint() ([]byte, error) {
	cp.Checksum = 0
	out, err := json.Marshal(*cp)
	if err != nil {
		return nil, err
	}
	cp.Checksum = checksum.New(out)
	return json.Marshal(*cp)
}

func (cp *Checkpoint) UnmarshalCheckpoint(data []byte) error {
	return json.Unmarshal(data, cp)
}

func (cp *Checkpoint) VerifyChecksum() error {
	ck := cp.Checksum
	cp.Checksum = 0
	defer func() {
		cp.Checksum = ck
	}()
	out, err := json.Marshal(*cp)
	if err != nil {
		return err
	}
	return ck.Verify(out)
}
