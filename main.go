package main

import (
	"context"
	"errors"
	"flag"
	"github.com/emiago/sipgo"
	"github.com/emiago/sipgox"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"io"
	"os"
	"os/signal"
	"path"
	"time"
)

func PlaySoundFiles(dialog *sipgox.DialogServerSession, filenames ...string) error {
	// Create an RTP packetizer for PCMU
	// Create an RTP packetizer
	mtu := uint16(1200)                   // Maximum Transmission Unit (MTU)
	payloadType := uint8(0)               // Payload type for PCMU
	ssrc := uint32(123456789)             // Synchronization Source Identifier (SSRC)
	payloader := &codecs.G711Payloader{}  // Payloader for PCMU
	sequencer := rtp.NewRandomSequencer() // Sequencer for generating sequence numbers
	clockRate := uint32(8000)             // Audio clock rate for PCMU

	packetizer := rtp.NewPacketizer(mtu, payloadType, ssrc, payloader, sequencer, clockRate)

	for _, filename := range filenames {
		packets := make([]*rtp.Packet, 0)
		thanks, err := os.ReadFile(path.Clean(filename))
		if err != nil {
			log.Error().Err(err)
			return nil
		}
		// Generate and send RTP packets every 20 milliseconds
		// Generate a dummy audio frame (replace with your actual audio data)
		audioData := thanks

		// Calculate the number of samples
		numSamples := uint32(len(thanks)) / clockRate * 8

		// Packetize the audio data into RTP packets
		packets = append(packets, packetizer.Packetize(audioData, numSamples)...)

		for _, packet := range packets {
			p, err := dialog.ReadRTP()
			if err != nil {
				if errors.Is(err, io.ErrClosedPipe) {
					return nil
				}
				log.Error().Err(err).Msg("Fail to read RTP")
				return err
			}
			p.Payload = append(p.Payload, packet.Payload...)
			// Send the RTP packet
			if err := dialog.WriteRTP(&p); err != nil {
				log.Error().Err(err).Msg("Error sending RTP packet")
				return err
			}

			log.Info().Msgf("Sent RTP packet: SeqNum=%d, Timestamp=%d, Payload=%d bytes\n", packet.SequenceNumber, packet.Timestamp, len(packet.Payload))

			// I'm certain this should be more mathematical, but this seems to be the value that actually plays
			// the full recording properly.
			time.Sleep(160 * time.Millisecond)
		}
	}

	return nil
}

func main() {
	addr := flag.String("l", "0.0.0.0:59232", "My listen ip")
	username := flag.String("u", "alice", "SIP Username")
	echoCount := flag.Int("echo", 1, "How many echos")
	//password := flag.String("p", "alice", "Password")
	flag.Parse()

	lev, err := zerolog.ParseLevel(os.Getenv("LOG_LEVEL"))
	if err != nil || lev == zerolog.NoLevel {
		lev = zerolog.InfoLevel
	}

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMicro
	log.Logger = zerolog.New(zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.StampMicro,
	}).With().Timestamp().Logger().Level(lev)

	// Setup UAC
	ua, err := sipgo.NewUA(
		sipgo.WithUserAgent(*username),
		// sipgo.WithUserAgentIP(net.ParseIP(ip)),
	)
	if err != nil {
		log.Fatal().Err(err).Msg("Fail to setup user agent")
	}

	phoneOpts := []sipgox.PhoneOption{}
	if *addr != "" {
		phoneOpts = append(phoneOpts,
			sipgox.WithPhoneListenAddr(sipgox.ListenAddr{
				// Network: *tran,
				Network: "udp",
				Addr:    *addr,
			}),
		)
	}

	phone := sipgox.NewPhone(ua, phoneOpts...)
	ctx, _ := context.WithCancel(context.Background())
	dialog, err := phone.Answer(ctx, sipgox.AnswerOptions{
		Ringtime: 1 * time.Second,
		// SipHeaders: []sip.Header{sip.NewHeader("X-ECHO-ID", "sipgo")},
	})
	if err != nil {
		log.Fatal().Err(err).Msg("Fail to answer")
	}
	print(*echoCount)
	sig := make(chan os.Signal)
	signal.Notify(sig, os.Interrupt)
	// Start echo
	go func() {
		// Send all incoming calls to the monkeys
		PlaySoundFiles(dialog,
			"sounds/vm-nobodyavail.ulaw",
			"sounds/carried-away-by-monkeys.ulaw",
			"sounds/the-monkeys-twice.ulaw",
			"sounds/lots-o-monkeys.ulaw")

	}()

	select {
	case <-sig:
		ctx, _ := context.WithTimeout(context.Background(), 3*time.Second)
		dialog.Hangup(ctx)
		return

	case <-dialog.Done():
		return
	}
}
