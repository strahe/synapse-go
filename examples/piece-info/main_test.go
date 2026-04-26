package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunPieceInfoPrintsCIDv2ForNormalPayload(t *testing.T) {
	var stdout bytes.Buffer
	err := runPieceInfo(bytes.NewReader(bytes.Repeat([]byte("pi"), 128)), &stdout)
	if err != nil {
		t.Fatalf("runPieceInfo: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		"cidV1=",
		"cidV2=",
		"rawSize=256",
		"paddedSize=512",
		"pieceRoot=",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q\ngot:\n%s", want, out)
		}
	}
	if strings.Contains(out, "undefined") {
		t.Fatalf("cidV2 unexpectedly undefined:\n%s", out)
	}
}

func TestRunPieceInfoExplainsSmallPayloadCIDv2(t *testing.T) {
	var stdout bytes.Buffer
	err := runPieceInfo(strings.NewReader("small payload"), &stdout)
	if err != nil {
		t.Fatalf("runPieceInfo: %v", err)
	}
	if !strings.Contains(stdout.String(), "cidV2=<undefined: raw payload smaller than 127 bytes>") {
		t.Fatalf("output=%s", stdout.String())
	}
}

func TestParseConfigAcceptsPositionalFile(t *testing.T) {
	cfg, err := parseConfig([]string{"./payload.bin"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.FilePath != "./payload.bin" {
		t.Fatalf("FilePath=%q", cfg.FilePath)
	}
}

func TestParseConfigRejectsFileFlagWithExtraPath(t *testing.T) {
	_, err := parseConfig([]string{"--file", "good.bin", "ignored.bin"})
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("err=%v want usage error", err)
	}
}
