package onvif

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"net/url"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtp"
)

// MetadataFrameCallback is called when an analytics frame is received
// from the RTSP metadata stream.
type MetadataFrameCallback func(frame *MetadataFrame)

// MetadataStreamSubscriber manages an RTSP connection to a camera's
// metadata stream, receiving RTP packets containing XML analytics/event
// data and routing parsed results through callbacks.
type MetadataStreamSubscriber struct {
	streamURI string
	username  string
	password  string
	eventCb   EventCallback
	frameCb   MetadataFrameCallback
	cancel    context.CancelFunc
}

// NewMetadataStreamSubscriber validates inputs and returns a subscriber
// ready to be started. At least one of eventCb or frameCb must be non-nil.
// The streamURI should be an RTSP URL for the camera profile that has a
// metadata configuration attached. Credentials are injected into the RTSP
// connection for digest authentication.
func NewMetadataStreamSubscriber(streamURI, username, password string, eventCb EventCallback, frameCb MetadataFrameCallback) (*MetadataStreamSubscriber, error) {
	if streamURI == "" {
		return nil, fmt.Errorf("onvif metadata stream: stream URI is required")
	}
	if eventCb == nil && frameCb == nil {
		return nil, fmt.Errorf("onvif metadata stream: at least one callback (event or frame) is required")
	}
	return &MetadataStreamSubscriber{
		streamURI: streamURI,
		username:  username,
		password:  password,
		eventCb:   eventCb,
		frameCb:   frameCb,
	}, nil
}

// Start connects to the RTSP metadata stream and reads packets with
// exponential backoff retry on errors. It blocks until the context is
// cancelled.
func (ms *MetadataStreamSubscriber) Start(ctx context.Context) {
	ctx, ms.cancel = context.WithCancel(ctx)

	backoff := 5 * time.Second
	maxBackoff := 5 * time.Minute

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := ms.connectAndRead(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Printf("onvif metadata stream [%s]: %v, retrying in %v", ms.streamURI, err, backoff)
		} else {
			backoff = 5 * time.Second
		}

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		}

		backoff = backoff * 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// Stop cancels the context, terminating the RTSP connection and retry loop.
func (ms *MetadataStreamSubscriber) Stop() {
	if ms.cancel != nil {
		ms.cancel()
	}
}

// connectAndRead performs a single RTSP session: DESCRIBE, find metadata
// track, SETUP, PLAY, and read RTP packets until error or context cancel.
func (ms *MetadataStreamSubscriber) connectAndRead(ctx context.Context) error {
	// Inject credentials into the URI for RTSP digest auth.
	streamURI := ms.streamURI
	if ms.username != "" {
		if parsed, err := url.Parse(streamURI); err == nil {
			parsed.User = url.UserPassword(ms.username, ms.password)
			streamURI = parsed.String()
		}
	}

	u, err := base.ParseURL(streamURI)
	if err != nil {
		return fmt.Errorf("parse stream URI: %w", err)
	}

	c := &gortsplib.Client{
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	c.Scheme = u.Scheme
	c.Host = u.Host

	err = c.Start()
	if err != nil {
		return fmt.Errorf("RTSP start: %w", err)
	}
	defer c.Close()

	desc, _, err := c.Describe(u)
	if err != nil {
		return fmt.Errorf("RTSP describe: %w", err)
	}

	// Find the metadata media track.
	var metadataMedia *description.Media
	for _, m := range desc.Medias {
		if m.IsBackChannel {
			continue
		}

		// Check media type.
		mediaType := strings.ToLower(string(m.Type))
		if mediaType == "application" || mediaType == "metadata" {
			metadataMedia = m
			break
		}

		// Check format codecs for metadata-related strings.
		for _, f := range m.Formats {
			codec := strings.ToLower(f.Codec())
			if strings.Contains(codec, "metadata") ||
				strings.Contains(codec, "application") ||
				strings.Contains(codec, "vnd.onvif") {
				metadataMedia = m
				break
			}
		}
		if metadataMedia != nil {
			break
		}
	}

	if metadataMedia == nil {
		return fmt.Errorf("no metadata track found in RTSP stream")
	}

	_, err = c.Setup(desc.BaseURL, metadataMedia, 0, 0)
	if err != nil {
		return fmt.Errorf("RTSP setup metadata track: %w", err)
	}

	const maxBufSize = 256 * 1024
	const trimSize = 128 * 1024
	var xmlBuf []byte

	c.OnPacketRTPAny(func(_ *description.Media, _ format.Format, pkt *rtp.Packet) {
		xmlBuf = append(xmlBuf, pkt.Payload...)

		// Cap buffer size.
		if len(xmlBuf) > maxBufSize {
			xmlBuf = xmlBuf[len(xmlBuf)-trimSize:]
		}

		// Try to parse the accumulated XML.
		frame, events, err := ParseMetadataStreamFull(xmlBuf)
		if err != nil {
			// Not a complete XML document yet; keep accumulating.
			return
		}

		// Successful parse — reset buffer.
		xmlBuf = xmlBuf[:0]

		// Dispatch events through callback.
		if ms.eventCb != nil {
			for _, evt := range events {
				eventType, ok := classifyTopic(evt.Topic)
				if ok {
					ms.eventCb(eventType, evt.Active)
				}
			}
		}

		// Dispatch frame through callback.
		if ms.frameCb != nil && frame != nil {
			ms.frameCb(frame)
		}
	})

	_, err = c.Play(nil)
	if err != nil {
		return fmt.Errorf("RTSP play: %w", err)
	}

	// Block until error or context cancellation.
	done := make(chan error, 1)
	go func() {
		done <- c.Wait()
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		c.Close()
		<-done
		return nil
	}
}

// GetMetadataStreamURI retrieves the RTSP metadata stream URI for a
// camera profile, trying Media2 first then falling back to Media1.
func GetMetadataStreamURI(xaddr, username, password, profileToken string) (string, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return "", fmt.Errorf("connect to ONVIF device: %w", err)
	}

	// Try Media2 first.
	if client.HasService("media2") {
		uri, err := GetStreamUri2(client, profileToken)
		if err == nil && uri != "" {
			return uri, nil
		}
		log.Printf("onvif metadata stream [%s]: Media2 GetStreamUri failed (%v), falling back to Media1", xaddr, err)
	}

	// Fall back to Media1.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	streamResp, err := client.Dev.GetStreamURI(ctx, profileToken)
	if err != nil {
		return "", fmt.Errorf("media1 GetStreamURI: %w", err)
	}
	if streamResp == nil || streamResp.URI == "" {
		return "", fmt.Errorf("media1 GetStreamURI: empty URI")
	}

	return streamResp.URI, nil
}
