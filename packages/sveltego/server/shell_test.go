package server

import (
	"strings"
	"testing"
)

func TestParseShell(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		src     string
		head    string
		mid     string
		tail    string
		wantErr string
	}{
		{
			name: "minimal_well_formed",
			src:  "<head>%sveltego.head%</head><body>%sveltego.body%</body>",
			head: "<head>",
			mid:  "</head><body>",
			tail: "</body>",
		},
		{
			name: "extra_whitespace_preserved",
			src:  "A\n%sveltego.head%\nB\n%sveltego.body%\nC",
			head: "A\n",
			mid:  "\nB\n",
			tail: "\nC",
		},
		{
			name:    "empty",
			src:     "",
			wantErr: "empty",
		},
		{
			name:    "missing_head",
			src:     "<body>%sveltego.body%</body>",
			wantErr: "missing %sveltego.head%",
		},
		{
			name:    "missing_body",
			src:     "<head>%sveltego.head%</head>",
			wantErr: "missing %sveltego.body%",
		},
		{
			name:    "duplicate_head",
			src:     "%sveltego.head%%sveltego.head%%sveltego.body%",
			wantErr: "duplicate %sveltego.head%",
		},
		{
			name:    "duplicate_body",
			src:     "%sveltego.head%%sveltego.body%%sveltego.body%",
			wantErr: "duplicate %sveltego.body%",
		},
		{
			name:    "body_before_head",
			src:     "%sveltego.body%X%sveltego.head%",
			wantErr: "%sveltego.body% before %sveltego.head%",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			head, mid, tail, err := parseShell(tc.src)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if head != tc.head || mid != tc.mid || tail != tc.tail {
				t.Fatalf("split mismatch: head=%q mid=%q tail=%q", head, mid, tail)
			}
		})
	}
}
