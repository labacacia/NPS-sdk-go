// Copyright 2026 INNO LOTUS PTY LTD
// SPDX-License-Identifier: Apache-2.0
package core

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/vmihailenco/msgpack/v5"
)

const (
	binaryVectorVersion = byte(1)
	binaryVectorPrefix  = 16
	binaryVectorMarker  = "$nps_binary_vector"
)

var binaryVectorMagic = []byte{'N', 'P', 'B', 'V'}

func encodeBinaryVector(dict FrameDict) ([]byte, error) {
	metadata, ok := cloneValue(dict).(FrameDict)
	if !ok {
		return nil, &ErrCodec{Msg: "binary vector metadata root must be an object"}
	}

	vectors := make([][]float32, 0, 1)
	extractVectorSearchVector(metadata, &vectors)
	if len(vectors) > math.MaxUint16 {
		return nil, &ErrCodec{Msg: "binary vector supports at most 65535 vectors per frame"}
	}

	metadataBytes, err := msgpack.Marshal(metadata)
	if err != nil {
		return nil, &ErrCodec{Msg: err.Error()}
	}

	segmentBytes := 0
	for _, vector := range vectors {
		segmentBytes += 4 + len(vector)*4
	}

	payload := make([]byte, binaryVectorPrefix+len(metadataBytes)+segmentBytes)
	copy(payload[0:4], binaryVectorMagic)
	payload[4] = binaryVectorVersion
	payload[5] = 0
	binary.BigEndian.PutUint16(payload[6:8], uint16(len(vectors)))
	binary.BigEndian.PutUint32(payload[8:12], uint32(len(metadataBytes)))
	binary.BigEndian.PutUint32(payload[12:16], 0)
	copy(payload[binaryVectorPrefix:], metadataBytes)

	offset := binaryVectorPrefix + len(metadataBytes)
	for _, vector := range vectors {
		binary.BigEndian.PutUint32(payload[offset:offset+4], uint32(len(vector)))
		offset += 4
		for _, value := range vector {
			binary.LittleEndian.PutUint32(payload[offset:offset+4], math.Float32bits(value))
			offset += 4
		}
	}

	return payload, nil
}

func decodeBinaryVector(payload []byte) (FrameDict, error) {
	if len(payload) < binaryVectorPrefix {
		return nil, &ErrCodec{Msg: fmt.Sprintf("binary vector payload too short: %d bytes", len(payload))}
	}
	if string(payload[0:4]) != string(binaryVectorMagic) {
		return nil, &ErrCodec{Msg: "binary vector payload magic mismatch"}
	}
	if payload[4] != binaryVectorVersion {
		return nil, &ErrCodec{Msg: fmt.Sprintf("unsupported binary vector version: %d", payload[4])}
	}
	if payload[5] != 0 || binary.BigEndian.Uint32(payload[12:16]) != 0 {
		return nil, &ErrCodec{Msg: "binary vector reserved fields must be zero"}
	}

	vectorCount := int(binary.BigEndian.Uint16(payload[6:8]))
	metadataLen := int(binary.BigEndian.Uint32(payload[8:12]))
	if metadataLen > len(payload)-binaryVectorPrefix {
		return nil, &ErrCodec{Msg: "binary vector metadata length exceeds payload length"}
	}

	offset := binaryVectorPrefix
	var metadata FrameDict
	if err := msgpack.Unmarshal(payload[offset:offset+metadataLen], &metadata); err != nil {
		return nil, &ErrCodec{Msg: err.Error()}
	}
	offset += metadataLen

	vectors := make([][]float32, 0, vectorCount)
	for i := 0; i < vectorCount; i++ {
		if len(payload)-offset < 4 {
			return nil, &ErrCodec{Msg: "binary vector segment missing dimension"}
		}
		dim32 := binary.BigEndian.Uint32(payload[offset : offset+4])
		maxInt := int(^uint(0) >> 1)
		if uint64(dim32) > uint64(maxInt)/4 {
			return nil, &ErrCodec{Msg: "binary vector dimension exceeds addressable payload size"}
		}
		dim := int(dim32)
		offset += 4
		byteLen := dim * 4
		if len(payload)-offset < byteLen {
			return nil, &ErrCodec{Msg: "binary vector segment is truncated"}
		}

		vector := make([]float32, dim)
		for j := range vector {
			value := math.Float32frombits(binary.LittleEndian.Uint32(payload[offset : offset+4]))
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return nil, &ErrCodec{Msg: "binary vector values must be finite float32"}
			}
			vector[j] = value
			offset += 4
		}
		vectors = append(vectors, vector)
	}

	if offset != len(payload) {
		return nil, &ErrCodec{Msg: "binary vector payload has trailing bytes"}
	}

	if err := restoreVectorSearchVector(metadata, vectors); err != nil {
		return nil, err
	}
	return metadata, nil
}

func extractVectorSearchVector(metadata FrameDict, vectors *[][]float32) {
	vectorSearch, ok := asFrameDict(metadata["vector_search"])
	if !ok {
		return
	}

	vector, ok := readFloatVector(vectorSearch["vector"])
	if !ok {
		return
	}

	index := len(*vectors)
	*vectors = append(*vectors, vector)
	vectorSearch["vector"] = FrameDict{
		binaryVectorMarker: index,
		"dtype":            "float32",
		"dim":              len(vector),
	}
}

func restoreVectorSearchVector(metadata FrameDict, vectors [][]float32) error {
	vectorSearch, ok := asFrameDict(metadata["vector_search"])
	if !ok {
		return nil
	}
	markerValue, ok := vectorSearch["vector"]
	if !ok {
		return nil
	}
	marker, ok := asFrameDict(markerValue)
	if !ok {
		return &ErrCodec{Msg: "binary vector marker must be an object"}
	}

	index, ok := intValue(marker[binaryVectorMarker])
	if !ok {
		return &ErrCodec{Msg: "binary vector marker missing vector index"}
	}
	if index < 0 || index >= len(vectors) {
		return &ErrCodec{Msg: fmt.Sprintf("binary vector marker references vector %d, but only %d vectors are present", index, len(vectors))}
	}
	if dtype, _ := marker["dtype"].(string); dtype != "float32" {
		return &ErrCodec{Msg: "binary vector v1 only supports dtype=float32"}
	}
	dim, ok := intValue(marker["dim"])
	if !ok || dim != len(vectors[index]) {
		return &ErrCodec{Msg: "binary vector marker dimension does not match vector segment"}
	}

	vectorSearch["vector"] = vectors[index]
	return nil
}

func cloneValue(value any) any {
	switch v := value.(type) {
	case FrameDict:
		out := make(FrameDict, len(v))
		for key, item := range v {
			out[key] = cloneValue(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = cloneValue(item)
		}
		return out
	default:
		return value
	}
}

func asFrameDict(value any) (FrameDict, bool) {
	switch v := value.(type) {
	case FrameDict:
		return v, true
	default:
		return nil, false
	}
}

func readFloatVector(value any) ([]float32, bool) {
	switch v := value.(type) {
	case []float32:
		return append([]float32(nil), v...), true
	case []float64:
		out := make([]float32, len(v))
		for i, item := range v {
			out[i] = float32(item)
		}
		return out, true
	case []any:
		out := make([]float32, len(v))
		for i, item := range v {
			number, ok := floatValue(item)
			if !ok || math.IsNaN(number) || math.IsInf(number, 0) {
				return nil, false
			}
			out[i] = float32(number)
		}
		return out, true
	default:
		return nil, false
	}
}

func floatValue(value any) (float64, bool) {
	switch v := value.(type) {
	case float32:
		return float64(v), true
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	default:
		return 0, false
	}
}

func intValue(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int8:
		return int(v), true
	case int16:
		return int(v), true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case uint:
		return int(v), true
	case uint8:
		return int(v), true
	case uint16:
		return int(v), true
	case uint32:
		return int(v), true
	case uint64:
		return int(v), true
	default:
		return 0, false
	}
}
