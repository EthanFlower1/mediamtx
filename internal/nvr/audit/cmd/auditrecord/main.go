// Binary auditrecord is a standalone recording process used by lifecycle tests.
// It creates a Stream with H264 media, starts a Recorder writing fMP4, and
// prints "READY" to stdout once recording has begun. It then writes IDR frames
// at 30fps in an infinite loop until killed.
//
// Usage: auditrecord <record-dir>
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/recorder"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
)

type silentLogger struct{}

func (silentLogger) Log(_ logger.Level, _ string, _ ...any) {}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: auditrecord <record-dir>\n")
		os.Exit(1)
	}
	recordDir := os.Args[1]

	desc := &description.Session{
		Medias: []*description.Media{
			test.UniqueMediaH264(),
			test.UniqueMediaMPEG4Audio(),
		},
	}

	strm := &stream.Stream{
		Desc:              desc,
		WriteQueueSize:    512,
		RTPMaxPayloadSize: 1450,
		Parent:            silentLogger{},
	}
	if err := strm.Initialize(); err != nil {
		fmt.Fprintf(os.Stderr, "stream init: %v\n", err)
		os.Exit(1)
	}
	defer strm.Close()

	sub := &stream.SubStream{
		Stream:        strm,
		UseRTPPackets: false,
	}
	if err := sub.Initialize(); err != nil {
		fmt.Fprintf(os.Stderr, "substream init: %v\n", err)
		os.Exit(1)
	}

	recordPath := filepath.Join(recordDir, "%path/%Y-%m-%d_%H-%M-%S-%f")

	rec := &recorder.Recorder{
		PathFormat:      recordPath,
		Format:          conf.RecordFormatFMP4,
		PartDuration:    100 * time.Millisecond,
		MaxPartSize:     50 * 1024 * 1024,
		SegmentDuration: 1 * time.Second,
		PathName:        "testpath",
		Stream:          strm,
		Parent:          silentLogger{},
	}
	rec.Initialize()
	defer rec.Close()

	// Signal readiness to the parent process.
	fmt.Println("READY")

	// Write H264 IDR frames at 30fps forever.
	const fps = 30
	frameDur := 90000 / fps
	ticker := time.NewTicker(time.Second / fps)
	defer ticker.Stop()

	startNTP := time.Now()
	i := 0
	for range ticker.C {
		pts := int64(i * frameDur)
		ntp := startNTP.Add(time.Duration(i) * time.Second / fps)

		sub.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
			PTS: pts,
			NTP: ntp,
			Payload: unit.PayloadH264{
				test.FormatH264.SPS,
				test.FormatH264.PPS,
				{5}, // IDR
			},
		})
		i++
	}
}
