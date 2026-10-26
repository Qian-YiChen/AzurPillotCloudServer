package gateway

import (
	"encoding/binary"
	"errors"
	"fmt"
	"unicode/utf8"
)

// 游戏协议的消息编号。
const (
	MsgCS10018 = 10018
	MsgSC10019 = 10019
)

// ErrProtocol 表示协议数据损坏（protobuf 帧、wire type、意外消息编号）。
var ErrProtocol = errors.New("malformed protocol data")

type fieldEntry struct {
	wireType int
	varint   uint64
	bytes    []byte
}

// fieldMap 是一次解码得到的字段集合：字段号 -> 多个条目。
type fieldMap map[int][]fieldEntry

func encodeVarint(value uint64) []byte {
	var buf []byte
	for {
		b := byte(value & 0x7F)
		value >>= 7
		if value != 0 {
			b |= 0x80
		}
		buf = append(buf, b)
		if value == 0 {
			break
		}
	}
	return buf
}

func encodeVarintField(fieldNumber int, value uint64) []byte {
	tag := encodeVarint(uint64(fieldNumber) << 3) // wire type 0
	return append(tag, encodeVarint(value)...)
}

// decodeVarint 从 data 的 offset 处解析一个 varint。
func decodeVarint(data []byte, offset int) (uint64, int, error) {
	var result uint64
	shift := uint(0)
	for {
		if offset >= len(data) {
			return 0, offset, ErrProtocol
		}
		b := data[offset]
		result |= uint64(b&0x7F) << shift
		shift += 7
		offset++
		if b&0x80 == 0 {
			break
		}
		if shift >= 64 {
			return 0, offset, ErrProtocol
		}
	}
	return result, offset, nil
}

// decodeProtobufMessage 解析任意 protobuf 消息为字段映射。
func decodeProtobufMessage(data []byte) (fieldMap, error) {
	fields := fieldMap{}
	offset := 0
	for offset < len(data) {
		tag, next, err := decodeVarint(data, offset)
		if err != nil {
			return nil, err
		}
		offset = next
		fieldNumber := int(tag >> 3)
		wireType := int(tag & 0x7)
		switch wireType {
		case 0:
			value, next, err := decodeVarint(data, offset)
			if err != nil {
				return nil, err
			}
			offset = next
			fields[fieldNumber] = append(fields[fieldNumber], fieldEntry{wireType: 0, varint: value})
		case 2:
			length, next, err := decodeVarint(data, offset)
			if err != nil {
				return nil, err
			}
			offset = next
			if length > uint64(len(data)-offset) {
				return nil, ErrProtocol
			}
			value := data[offset : offset+int(length)]
			offset += int(length)
			fields[fieldNumber] = append(fields[fieldNumber], fieldEntry{wireType: 2, bytes: value})
		case 5:
			if offset+4 > len(data) {
				return nil, ErrProtocol
			}
			value := binary.LittleEndian.Uint32(data[offset : offset+4])
			offset += 4
			fields[fieldNumber] = append(fields[fieldNumber], fieldEntry{wireType: 5, varint: uint64(value)})
		default:
			return nil, fmt.Errorf("%w: unsupported wire type %d", ErrProtocol, wireType)
		}
	}
	return fields, nil
}

// buildPacket 构造游戏协议数据包：长度 + 固定字节 + 消息编号 + 序号 + 包体。
func buildPacket(msgID int, body []byte) []byte {
	remaining := 5 + len(body)
	packet := make([]byte, 0, 7+len(body))
	packet = binary.BigEndian.AppendUint16(packet, uint16(remaining))
	packet = append(packet, 0x00)
	packet = binary.BigEndian.AppendUint16(packet, uint16(msgID))
	packet = binary.BigEndian.AppendUint16(packet, 0)
	packet = append(packet, body...)
	return packet
}

// parseResponse 校验响应帧并返回消息编号与包体。
func parseResponse(data []byte) (int, []byte, error) {
	if len(data) < 7 {
		return 0, nil, ErrProtocol
	}
	remaining := binary.BigEndian.Uint16(data[0:2])
	msgID := binary.BigEndian.Uint16(data[3:5])
	end := 7 + int(remaining) - 5
	if int(remaining) < 5 || end > len(data) {
		return 0, nil, ErrProtocol
	}
	return int(msgID), data[7:end], nil
}

// decodeName 解码服务器名称：日/英/韩/台为 UTF-8，国服为 GBK。
func decodeName(value []byte) string {
	if utf8.Valid(value) {
		return string(value)
	}
	return decodeGBK(value)
}

// decodeServerInfo 解析 ServerInfo：1=id, 4=state, 5=name, 6=tag_state, 7=sort。
func decodeServerInfo(data []byte) (RawServer, error) {
	fields, err := decodeProtobufMessage(data)
	if err != nil {
		return RawServer{}, err
	}
	info := RawServer{}
	for fieldNumber, entries := range fields {
		for _, entry := range entries {
			switch fieldNumber {
			case 1:
				if entry.wireType == 0 && entry.varint != 0 {
					info.ID = int(entry.varint)
				} else if entry.wireType == 2 {
					// 兼容把 id 编码成嵌套 varint 的服务器。
					offset := 0
					for offset < len(entry.bytes) {
						v, next, err := decodeVarint(entry.bytes, offset)
						if err != nil {
							return RawServer{}, err
						}
						offset = next
						if v != 0 {
							info.ID = int(v)
						}
					}
				}
			case 4:
				if entry.wireType == 0 {
					info.State = int(entry.varint)
				}
			case 5:
				if entry.wireType == 2 {
					info.Name = decodeName(entry.bytes)
				}
			case 6:
				if entry.wireType == 0 {
					info.TagState = int(entry.varint)
				}
			case 7:
				if entry.wireType == 0 {
					info.Sort = int(entry.varint)
				}
			}
		}
	}
	return info, nil
}

// decodeServerlist 解析 10019 包体中的服务器列表（字段 1 为重复的 ServerInfo）。
func decodeServerlist(data []byte) ([]RawServer, error) {
	servers := []RawServer{}
	fields, err := decodeProtobufMessage(data)
	if err != nil {
		return nil, err
	}
	for fieldNumber, entries := range fields {
		if fieldNumber != 1 {
			continue
		}
		for _, entry := range entries {
			if entry.wireType != 2 {
				continue
			}
			info, err := decodeServerInfo(entry.bytes)
			if err != nil {
				return nil, err
			}
			servers = append(servers, info)
		}
	}
	return servers, nil
}

// encodeServerlistRequest 构造 10018 请求包体（字段 1 = 0）。
func encodeServerlistRequest() []byte {
	return encodeVarintField(1, 0)
}
