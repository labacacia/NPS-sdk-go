// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package core

import "fmt"

type ErrFrame struct{ Msg string }
type ErrCodec struct{ Msg string }
type ErrAnchorNotFound struct{ ID string }
type ErrAnchorPoison struct{ ID string }
type ErrIdentity struct{ Msg string }

func (e *ErrFrame) Error() string          { return "NPS frame error: " + e.Msg }
func (e *ErrCodec) Error() string          { return "NPS codec error: " + e.Msg }
func (e *ErrAnchorNotFound) Error() string { return fmt.Sprintf("NPS anchor not found: %s", e.ID) }
func (e *ErrAnchorPoison) Error() string   { return fmt.Sprintf("NPS anchor poison: %s", e.ID) }
func (e *ErrIdentity) Error() string       { return "NPS identity error: " + e.Msg }
