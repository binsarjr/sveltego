package adapterserver_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	adapterserver "github.com/binsarjr/sveltego/adapter-server"
)

func TestBuildCopiesBinaryAndAssets(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	binSrc := filepath.Join(tmp, "input-bin")
	if err := os.WriteFile(binSrc, []byte("\x7fELFstub"), 0o755); err != nil {
		t.Fatalf("seed binary: %v", err)
	}

	assetsSrc := filepath.Join(tmp, "assets")
	if err := os.MkdirAll(filepath.Join(assetsSrc, "static"), 0o755); err != nil {
		t.Fatalf("seed assets dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(assetsSrc, "static", "logo.svg"), []byte("<svg/>"), 0o644); err != nil {
		t.Fatalf("seed asset: %v", err)
	}

	out := filepath.Join(tmp, "dist")
	err := adapterserver.Build(context.Background(), adapterserver.BuildContext{
		ProjectRoot: tmp,
		BinaryPath:  binSrc,
		AssetsDir:   assetsSrc,
		OutputDir:   out,
		BinaryName:  "myapp",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(out, "myapp"))
	if err != nil {
		t.Fatalf("read output binary: %v", err)
	}
	if string(got) != "\x7fELFstub" {
		t.Fatalf("output binary content mismatch: %q", got)
	}

	asset, err := os.ReadFile(filepath.Join(out, "assets", "static", "logo.svg"))
	if err != nil {
		t.Fatalf("read copied asset: %v", err)
	}
	if string(asset) != "<svg/>" {
		t.Fatalf("asset content mismatch: %q", asset)
	}
}

func TestBuildDefaultBinaryName(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	binSrc := filepath.Join(tmp, "input-bin")
	if err := os.WriteFile(binSrc, []byte("bin"), 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}
	out := filepath.Join(tmp, "dist")

	if err := adapterserver.Build(context.Background(), adapterserver.BuildContext{
		BinaryPath: binSrc,
		OutputDir:  out,
	}); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "sveltego")); err != nil {
		t.Fatalf("default binary name not used: %v", err)
	}
}

func TestBuildErrors(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	cases := []struct {
		name    string
		bc      adapterserver.BuildContext
		wantSub string
	}{
		{
			name:    "missing binary path",
			bc:      adapterserver.BuildContext{OutputDir: tmp},
			wantSub: "BinaryPath is required",
		},
		{
			name:    "missing output dir",
			bc:      adapterserver.BuildContext{BinaryPath: filepath.Join(tmp, "bin")},
			wantSub: "OutputDir is required",
		},
		{
			name: "binary not found",
			bc: adapterserver.BuildContext{
				BinaryPath: filepath.Join(tmp, "does-not-exist"),
				OutputDir:  filepath.Join(tmp, "dist"),
			},
			wantSub: "binary not found",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := adapterserver.Build(context.Background(), tc.bc)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error %q missing %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestDoc(t *testing.T) {
	t.Parallel()
	if !strings.Contains(adapterserver.Doc(), "Server target") {
		t.Fatalf("Doc missing target heading")
	}
}

func TestBuildContextCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := adapterserver.Build(ctx, adapterserver.BuildContext{
		BinaryPath: "ignored",
		OutputDir:  "ignored",
	})
	if err == nil {
		t.Fatalf("expected context error")
	}
}
