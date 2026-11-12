package convtask

import "testing"

func TestEncodeDecodeRoundTrips(t *testing.T) {
	thread := "t1"
	in := Payload{
		Verb: VerbInboundMessage, Conversation: "conv-1", CloseKind: CloseAuto,
		Reason: "because", IdleForMS: 1234, Channel: "c1", ThreadID: &thread,
		Text: "hello", Sender: "alice",
	}
	out, err := Decode(in.Encode())
	if err != nil {
		t.Fatal(err)
	}
	if out.Verb != in.Verb || out.Conversation != in.Conversation || out.CloseKind != in.CloseKind ||
		out.Reason != in.Reason || out.IdleForMS != in.IdleForMS || out.Channel != in.Channel ||
		out.ThreadID == nil || *out.ThreadID != *in.ThreadID || out.Text != in.Text || out.Sender != in.Sender {
		t.Fatalf("round trip lost a field: got %+v, want %+v", out, in)
	}
}

func TestDecodeRejectsUndecodableBytes(t *testing.T) {
	if _, err := Decode([]byte("not json")); err == nil {
		t.Fatal("want an error for undecodable bytes")
	}
}

func TestQueueIsTheLiteralConvQueueName(t *testing.T) {
	if Queue != "conv" {
		t.Fatalf(`Queue = %q, want "conv"`, Queue)
	}
}
