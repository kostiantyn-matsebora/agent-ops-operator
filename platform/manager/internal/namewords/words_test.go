package namewords

import "testing"

func TestWords(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{
			name: "camelCase and punctuation both split",
			text: "NodeDown — ns-prod",
			want: "node-down-ns",
		},
		{
			name: "a stopword is dropped",
			text: "Nightly backup of prod-db",
			want: "nightly-backup-prod",
		},
		{
			name: "only the first three significant words are kept",
			text: "one two three four five",
			want: "one-two-three",
		},
		{
			name: "no significant word yields an empty chain",
			text: "... 👍 ---",
			want: "",
		},
		{
			name: "empty text yields an empty chain",
			text: "",
			want: "",
		},
		{
			name: "over-length truncation drops whole trailing words",
			text: "incident response automation extra",
			want: "incident-response",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Words(tc.text)
			if got != tc.want {
				t.Errorf("Words(%q) = %q, want %q", tc.text, got, tc.want)
			}
			if len(got) > MaxWordChainLength {
				t.Errorf("Words(%q) = %q, exceeds MaxWordChainLength %d", tc.text, got, MaxWordChainLength)
			}
		})
	}
}
