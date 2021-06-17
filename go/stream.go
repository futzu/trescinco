package threefive

import (
	"bytes"
	"os"
)

//Stream for parsing MPEGTS for SCTE-35
type Stream struct {
	Pkts     int               // packet count.
	pid2prgm map[uint16]uint16 //lookup table for pid to program
	prgm2pcr map[uint16]uint64 //lookup table for program to pcr
	prgm2pts map[uint16]uint64 //lookup table for program to pts
	partial  map[uint16][]byte // partial manages tables spread across multiple packets by pid
	last     map[uint16][]byte // last compares current packet payload to last packet payload by pid
	pids     Pids
}

func (stream *Stream) mkMaps() {
	stream.pid2prgm = make(map[uint16]uint16)
	stream.last = make(map[uint16][]byte)
	stream.partial = make(map[uint16][]byte)
	stream.prgm2pcr = make(map[uint16]uint64)
	stream.prgm2pts = make(map[uint16]uint64)
}

// Decode fname (a file name) for SCTE-35
func (stream *Stream) Decode(fname string) {
	stream.mkMaps()
	stream.Pkts = 0
	file, err := os.Open(fname)
	Chk(err)
	defer file.Close()
	buffer := make([]byte, BufferSize)
	for {
		bytesread, err := file.Read(buffer)
		if err != nil {
			break
		}
		for i := 1; i <= (bytesread / PktSz); i++ {
			end := i * PktSz
			start := end - PktSz
			p := buffer[start:end]
			pkt := &p
			stream.Pkts++
			stream.parse(*pkt)
		}
	}
}

func (stream *Stream) mkPcr(prgm uint16) float64 {
	pcrb := stream.prgm2pcr[prgm]
	return mk90k(pcrb)
}

func (stream *Stream) mkPts(prgm uint16) float64 {
	pts := stream.prgm2pts[prgm]
	return mk90k(pts)
}

func (stream *Stream) parsePusi(pkt []byte) bool {
	if (pkt[1]>>6)&1 == 1 {
		if pkt[6]&1 == 1 {
			return true
		}
	}
	return false
}

func (stream *Stream) parsePts(pkt []byte, pid uint16) {
	if stream.parsePusi(pkt) {
		prgm, ok := stream.pid2prgm[pid]
		if ok {
			pts := (uint64(pkt[13]) >> 1 & 7) << 30
			pts |= uint64(pkt[14]) << 22
			pts |= (uint64(pkt[15]) >> 1) << 15
			pts |= uint64(pkt[16]) << 7
			pts |= uint64(pkt[17]) >> 1
			stream.prgm2pts[prgm] = pts
		}
	}
}

//
func (stream *Stream) parsePcr(pkt []byte, pid uint16) {
	if (pkt[3]>>5)&1 == 1 {
		if (pkt[5]>>4)&1 == 1 {
			pcr := (uint64(pkt[6]) << 25)
			pcr |= (uint64(pkt[7]) << 17)
			pcr |= (uint64(pkt[8]) << 9)
			pcr |= (uint64(pkt[9]) << 1)
			pcr |= uint64(pkt[10]) >> 7
			prgm := stream.pid2prgm[pid]
			stream.prgm2pcr[prgm] = pcr
		}
	}
}

func (stream *Stream) parsePayload(pkt []byte) []byte {
	head := 4
	afc := (pkt[3] >> 5) & 1
	if afc == 1 {
		afl := int(pkt[4])
		head += afl + 1
	}
	if head > PktSz {
		head = PktSz
	}
	return pkt[head:]
}

//chkPartial appends the current packet payload to partial table by pid.
func (stream *Stream) chkPartial(pay []byte, pid uint16, sep []byte) []byte {
	val, ok := stream.partial[pid]
	if ok {
		pay = append(val, pay...)
	}
	return splitByIdx(pay, sep)
}

// sameAsLast compares the current packet to the last packet by pid.
func (stream *Stream) sameAsLast(pay []byte, pid uint16) bool {
	val, ok := stream.last[pid]
	if ok {
		if bytes.Compare(pay, val) == 0 {
			return true
		}
	}
	stream.last[pid] = pay
	return false
}

//sectionDone aggregates partial tables by pid until the section is complete.
func (stream *Stream) sectionDone(pay []byte, pid uint16, seclen uint16) bool {
	if seclen+3 > uint16(len(pay)) {
		stream.partial[pid] = pay
		return false
	}
	delete(stream.partial, pid)
	return true
}

// Parser for Stream
func (stream *Stream) parse(pkt []byte) {
	p := parsePid(pkt[1], pkt[2])
	pid := &p
	pl := stream.parsePayload(pkt)
	pay := &pl
	if *pid == 0 {
		stream.parsePat(*pay, *pid)
	}
	if stream.pids.isPmt(*pid) {
		stream.parsePmt(*pay, *pid)
	}
	if stream.pids.isPcr(*pid) {
		stream.parsePcr(pkt, *pid)
	} else {
		stream.parsePts(pkt, *pid)
	}
	if stream.pids.isScte35(*pid) {
		stream.parseScte35(*pay, *pid)
	}
}

func (stream *Stream) parsePat(pay []byte, pid uint16) {
	if stream.sameAsLast(pay, pid) {
		return
	}
	pay = stream.chkPartial(pay, pid, []byte("\x00\x00"))
	if len(pay) < 1 {
		return
	}
	seclen := parseLen(pay[2], pay[3])
	if stream.sectionDone(pay, pid, seclen) {
		seclen -= 5 // pay bytes 4,5,6,7,8
		idx := uint16(9)
		end := idx + seclen - 4 //  4 bytes for crc
		chunksize := uint16(4)
		for idx < end {
			prgm := parsePrgm(pay[idx], pay[idx+1])
			if prgm > 0 {
				pmtpid := parsePid(pay[idx+2], pay[idx+3])
				stream.pids.addPmt(pmtpid)
			}
			idx += chunksize
		}
	}
}

func (stream *Stream) parsePmt(pay []byte, pid uint16) {
	if stream.sameAsLast(pay, pid) {
		return
	}
	pay = stream.chkPartial(pay, pid, []byte("\x02"))
	if len(pay) < 1 {
		return
	}
	secinfolen := parseLen(pay[1], pay[2])
	if stream.sectionDone(pay, pid, secinfolen) {
		prgm := parsePrgm(pay[3], pay[4])
		pcrpid := parsePid(pay[8], pay[9])
		stream.pids.addPcr(pcrpid)
		proginfolen := parseLen(pay[10], pay[11])
		idx := uint16(12)
		idx += proginfolen
		silen := secinfolen - 9
		silen -= proginfolen
		stream.parseStreams(silen, pay, idx, prgm)
	}
}

func (stream *Stream) parseStreams(silen uint16, pay []byte, idx uint16, prgm uint16) {
	chunksize := uint16(5)
	endidx := (idx + silen) - chunksize
	for idx < endidx {
		streamtype := pay[idx]
		elpid := parsePid(pay[idx+1], pay[idx+2])
		eilen := parseLen(pay[idx+3], pay[idx+4])
		idx += chunksize
		idx += eilen
		stream.pid2prgm[elpid] = prgm
		stream.vrfyStreamType(elpid, streamtype)
	}
}

func (stream *Stream) vrfyStreamType(pid uint16, streamtype uint8) {
	if streamtype == 6 || streamtype == 134 {
		stream.pids.addScte35(pid)
	}
}

func (stream *Stream) parseScte35(pay []byte, pid uint16) {
	pay = stream.chkPartial(pay, pid, []byte("\xfc0"))
	if len(pay) == 0 {
		return
	}
	seclen := parseLen(pay[1], pay[2])
	if stream.sectionDone(pay, pid, seclen) {
		cue := stream.mkCue(pid)
		if cue.Decode(pay) {
			cue.Show()
		}
	}
}

func (stream *Stream) mkCue(pid uint16) Cue {
	var cue Cue
	cue.Pid = pid
	prgm := stream.pid2prgm[pid]
	cue.Program = prgm
	cue.Pcr = stream.mkPcr(prgm)
	cue.Pts = stream.mkPts(prgm)
	cue.PacketNumber = stream.Pkts
	return cue
}