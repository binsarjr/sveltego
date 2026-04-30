package vite

import (
	"testing"

	"github.com/binsarjr/sveltego/test-utils/golden"
)

func TestGenerateConfig_Fixtures(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		opts ConfigOptions
	}{
		{
			name: "none",
			opts: ConfigOptions{
				RouteKeys: []string{"routes/+page"},
			},
		},
		{
			name: "tailwind-v4",
			opts: ConfigOptions{
				RouteKeys: []string{"routes/+page"},
				Addons:    []Addon{AddonTailwindV4},
				CSSEntry:  "src/app.css",
			},
		},
		{
			name: "tailwind-v3",
			opts: ConfigOptions{
				RouteKeys: []string{"routes/+page"},
				Addons:    []Addon{AddonTailwindV3},
				CSSEntry:  "src/app.css",
			},
		},
		{
			name: "service-worker",
			opts: ConfigOptions{
				RouteKeys:          []string{"routes/+page"},
				ServiceWorkerEntry: "src/service-worker.ts",
			},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := GenerateConfig(tc.opts)
			golden.EqualString(t, "vite/"+tc.name+".js", got)
		})
	}
}

func TestHasAddon(t *testing.T) {
	t.Parallel()
	if hasAddon(nil, AddonTailwindV4) {
		t.Errorf("hasAddon(nil) = true; want false")
	}
	if !hasAddon([]Addon{AddonTailwindV4}, AddonTailwindV4) {
		t.Errorf("hasAddon([v4], v4) = false; want true")
	}
	if hasAddon([]Addon{AddonTailwindV4}, AddonTailwindV3) {
		t.Errorf("hasAddon([v4], v3) = true; want false")
	}
}
