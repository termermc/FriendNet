package machine

import (
	"friendnet.org/common"
	pb "friendnet.org/protocol/pb/v1"
)

// ConnMethodSupport contains a list of supported connection methods.
type ConnMethodSupport struct {
	types map[pb.ConnMethodType]struct{}
}

// IsSupported returns true if the specified connection method is supported.
func (s ConnMethodSupport) IsSupported(typ pb.ConnMethodType) bool {
	_, ok := s.types[typ]
	return ok
}

// ProbeConnMethodSupport probes the system for supported connection methods.
// Even if an error is returned, the ConnMethodSupport can still be used.
func ProbeConnMethodSupport() (ConnMethodSupport, error) {
	res := ConnMethodSupport{
		types: make(map[pb.ConnMethodType]struct{}, 2),
	}
	res.types[pb.ConnMethodType_CONN_METHOD_TYPE_IP] = struct{}{}

	// Probe interfaces for an Yggdrasil address.
	probedIps := common.GetUnicastIpsFromInterfaces(false, false)
	for _, ip := range probedIps {
		if common.YggdrasilPrefix.Contains(ip) {
			res.types[pb.ConnMethodType_CONN_METHOD_TYPE_YGGDRASIL] = struct{}{}
		}
	}

	// In the future if any errors occur for probing new methods, add errors to
	// a slice and return them with errors.Join.
	// Do not return if a method probe fails.

	return res, nil
}
