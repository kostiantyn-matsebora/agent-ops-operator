package activity

import (
	"fmt"
	"strings"
	"testing"
)

func TestEmitBoundsTheDataMap(t *testing.T) {
	l := New(4)
	data := map[string]string{}
	for i := 0; i < MaxDataKeys+5; i++ {
		data[fmt.Sprintf("k%02d", i)] = strings.Repeat("v", MaxDataValue*3)
	}
	l.Emit(Event{Kind: KindModelCall, Data: data})
	got, _ := l.Since("", 0)
	if len(got) != 1 {
		t.Fatalf("want one event, got %d", len(got))
	}
	if n := len(got[0].Data); n != MaxDataKeys {
		t.Fatalf("data kept %d keys, want the bound %d", n, MaxDataKeys)
	}
	for k, v := range got[0].Data {
		if len(v) > MaxDataValue {
			t.Fatalf("value of %s is %d bytes, over the bound %d", k, len(v), MaxDataValue)
		}
	}
	if _, kept := got[0].Data[fmt.Sprintf("k%02d", MaxDataKeys+4)]; kept {
		t.Fatal("the keys beyond the bound must be the ones dropped, in sorted order")
	}
	if len(data) != MaxDataKeys+5 {
		t.Fatal("bounding must not modify the emitter's map")
	}
}

func TestEmptyDataIsOmitted(t *testing.T) {
	l := New(4)
	l.Emit(Event{Kind: KindToolCall, Data: map[string]string{}})
	got, _ := l.Since("", 0)
	if got[0].Data != nil {
		t.Fatalf("an empty data map must be omitted, got %+v", got[0].Data)
	}
}

func TestDataDoesNotDisturbCursorsOrEviction(t *testing.T) {
	l := New(3)
	for i := 0; i < 7; i++ {
		l.Emit(Event{Kind: KindModelCall, Data: map[string]string{"model": "m", "tokensIn": fmt.Sprint(i)}})
	}
	got, _ := l.Since("", 0)
	if len(got) != 3 {
		t.Fatalf("ring grew past its capacity: %d", len(got))
	}
	if got[0].Data["tokensIn"] != "4" || got[2].Data["tokensIn"] != "6" {
		t.Fatalf("oldest must be evicted first, data travelling with its event: %+v", got)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Cursor >= got[i].Cursor {
			t.Fatalf("cursors not increasing: %q then %q", got[i-1].Cursor, got[i].Cursor)
		}
	}
}

func TestTruncateKeepsRunesWhole(t *testing.T) {
	if got := Truncate("aé", 2); got != "a" {
		t.Fatalf("a cut inside a rune must back off to its start, got %q", got)
	}
	if got := Truncate("abc", 5); got != "abc" {
		t.Fatalf("a short string is untouched, got %q", got)
	}
}
