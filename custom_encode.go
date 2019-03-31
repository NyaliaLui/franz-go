package kgo

import (
	"hash/crc32"

	"github.com/twmb/kgo/kbin"
	"github.com/twmb/kgo/kmsg"
)

var crc32c = crc32.MakeTable(crc32.Castagnoli) // record crc's use Castagnoli table

// messageBufferedProduceRequest is a kmsg.Request that is used when we want to
// flush our buffered records.
//
// It is the same as kmsg.ProduceRequest, but with a custom AppendTo.
type messageBufferedProduceRequest struct {
	version int16

	acks    int16
	timeout int32
	data    map[string]map[int32]*recordBatch

	compression []CompressionCodec
}

func (*messageBufferedProduceRequest) Key() int16           { return 0 }
func (*messageBufferedProduceRequest) MaxVersion() int16    { return 7 }
func (*messageBufferedProduceRequest) MinVersion() int16    { return 3 }
func (m *messageBufferedProduceRequest) SetVersion(v int16) { m.version = v }
func (m *messageBufferedProduceRequest) GetVersion() int16  { return m.version }
func (m *messageBufferedProduceRequest) AppendTo(dst []byte) []byte {
	if m.version >= 3 {
		dst = kbin.AppendNullableString(dst, nil) // TODO transactional ID
	}

	compressor := loadProduceCompressor(m.compression, m.version)

	dst = kbin.AppendInt16(dst, m.acks)
	dst = kbin.AppendInt32(dst, m.timeout)
	dst = kbin.AppendArrayLen(dst, len(m.data))
	for topic, partitions := range m.data {
		dst = kbin.AppendString(dst, topic)
		dst = kbin.AppendArrayLen(dst, len(partitions))
		for partition, batch := range partitions {
			dst = kbin.AppendInt32(dst, partition)
			dst = batch.appendTo(dst, compressor)
		}
	}
	return dst
}

func (m *messageBufferedProduceRequest) ResponseKind() kmsg.Response {
	return &kmsg.ProduceResponse{Version: m.version}
}

func (r *recordBatch) appendTo(dst []byte, compressor *compressor) []byte {
	nullableBytesLen := r.wireLength - 4 // NULLABLE_BYTES leading length, minus itself
	nullableBytesLenAt := len(dst)       // in case compression adjusting
	dst = kbin.AppendInt32(dst, nullableBytesLen)

	dst = kbin.AppendInt64(dst, 0) // firstOffset, defined as zero for producing

	batchLen := nullableBytesLen - 8 - 4 // minus baseOffset, minus self
	batchLenAt := len(dst)               // in case compression adjusting
	dst = kbin.AppendInt32(dst, batchLen)

	dst = kbin.AppendInt32(dst, -1) // partitionLeaderEpoch, unused in clients
	dst = kbin.AppendInt8(dst, 2)   // magic, defined as 2 for records v0.11.0.0+

	crcStart := len(dst)           // fill at end
	dst = kbin.AppendInt32(dst, 0) // reserved crc

	attrsAt := len(dst) // in case compression adjusting
	attrs := r.attrs
	dst = kbin.AppendInt16(dst, attrs)
	dst = kbin.AppendInt32(dst, int32(len(r.records)-1)) // lastOffsetDelta
	dst = kbin.AppendInt64(dst, r.firstTimestamp)

	// maxTimestamp is the timestamp of the last record in a batch.
	lastRecord := r.records[len(r.records)-1]
	dst = kbin.AppendInt64(dst, r.firstTimestamp+int64(lastRecord.n.timestampDelta))

	dst = kbin.AppendInt64(dst, -1) // producerId
	dst = kbin.AppendInt16(dst, -1) // producerEpoch
	dst = kbin.AppendInt32(dst, -1) // baseSequence

	dst = kbin.AppendArrayLen(dst, len(r.records))
	recordsAt := len(dst)
	for _, pnr := range r.records {
		dst = pnr.appendTo(dst)
	}

	if compressor != nil {
		toCompress := dst[recordsAt:]
		zipr := compressor.getZipr()
		defer compressor.putZipr(zipr)

		compressed := zipr.compress(toCompress)
		if compressed != nil && // nil would be from an error
			len(compressed) < len(toCompress) {

			// our compressed was shorter: copy over
			copy(dst[recordsAt:], compressed)
			dst = dst[:recordsAt+len(compressed)]

			// update the few record batch fields we already wrote
			savings := int32(len(toCompress) - len(compressed))
			nullableBytesLen -= savings
			batchLen -= savings
			attrs |= int16(compressor.attrs)
			kbin.AppendInt32(dst[:nullableBytesLenAt], nullableBytesLen)
			kbin.AppendInt32(dst[:batchLenAt], batchLen)
			kbin.AppendInt16(dst[:attrsAt], attrs)
		}
	}

	kbin.AppendInt32(dst[:crcStart], int32(crc32.Checksum(dst[crcStart+4:], crc32c)))

	return dst
}

func (pnr promisedNumberedRecord) appendTo(dst []byte) []byte {
	dst = kbin.AppendVarint(dst, pnr.n.lengthField)
	dst = kbin.AppendInt8(dst, 0) // attributes, currently unused
	dst = kbin.AppendVarint(dst, pnr.n.timestampDelta)
	dst = kbin.AppendVarint(dst, pnr.n.offsetDelta)
	dst = kbin.AppendVarintBytes(dst, pnr.pr.r.Key)
	dst = kbin.AppendVarintBytes(dst, pnr.pr.r.Value)
	dst = kbin.AppendVarint(dst, int32(len(pnr.pr.r.Headers)))
	for _, h := range pnr.pr.r.Headers {
		dst = kbin.AppendVarintString(dst, h.Key)
		dst = kbin.AppendVarintBytes(dst, h.Value)
	}
	return dst
}