// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package core

// FrameRegistry tracks which frame types are registered for decode.
type FrameRegistry struct {
	registered map[FrameType]bool
}

func NewFrameRegistry() *FrameRegistry {
	return &FrameRegistry{registered: make(map[FrameType]bool)}
}

func (r *FrameRegistry) Register(ft FrameType) { r.registered[ft] = true }

func (r *FrameRegistry) IsRegistered(ft FrameType) bool { return r.registered[ft] }

// CreateDefaultRegistry registers NCP frames only.
func CreateDefaultRegistry() *FrameRegistry {
	r := NewFrameRegistry()
	for _, ft := range []FrameType{
		FrameTypeAnchor, FrameTypeDiff, FrameTypeStream, FrameTypeCaps, FrameTypeHello, FrameTypeError,
	} {
		r.Register(ft)
	}
	return r
}

// CreateFullRegistry registers all five protocol frame types.
func CreateFullRegistry() *FrameRegistry {
	r := CreateDefaultRegistry()
	for _, ft := range []FrameType{
		// NWP
		FrameTypeQuery, FrameTypeAction,
		// NIP
		FrameTypeIdent, FrameTypeTrust, FrameTypeRevoke,
		// NDP
		FrameTypeAnnounce, FrameTypeResolve, FrameTypeGraph,
		// NOP
		FrameTypeTask, FrameTypeDelegate, FrameTypeSync, FrameTypeAlignStream,
	} {
		r.Register(ft)
	}
	return r
}
