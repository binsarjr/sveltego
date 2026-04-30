package adapterlambda_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	adapterlambda "github.com/binsarjr/sveltego/adapter-lambda"
)

func TestBuildEmitsMainAndSAM(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	err := adapterlambda.Build(context.Background(), adapterlambda.BuildContext{
		ProjectRoot:    root,
		ModulePath:     "github.com/example/myapp",
		HandlerName:    "MyApp",
		MemoryMB:       1024,
		TimeoutSeconds: 60,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	mainBody, err := os.ReadFile(filepath.Join(root, ".gen", "lambda", "main.go"))
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	wants := []string{
		"package main",
		"github.com/aws/aws-lambda-go/lambda",
		"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter",
		`gen "github.com/example/myapp/.gen"`,
		"lambda.Start(handler)",
		"httpadapter.New",
	}
	for _, w := range wants {
		if !strings.Contains(string(mainBody), w) {
			t.Errorf("main.go missing %q", w)
		}
	}

	sam, err := os.ReadFile(filepath.Join(root, ".gen", "lambda", "template.yaml"))
	if err != nil {
		t.Fatalf("read template.yaml: %v", err)
	}
	wantSAM := []string{
		"AWS::Serverless::Function",
		"MyAppFunction:",
		"MemorySize: 1024",
		"Timeout: 60",
		"provided.al2023",
		"Path: /{proxy+}",
	}
	for _, w := range wantSAM {
		if !strings.Contains(string(sam), w) {
			t.Errorf("template.yaml missing %q", w)
		}
	}
}

func TestBuildDefaults(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := adapterlambda.Build(context.Background(), adapterlambda.BuildContext{
		ProjectRoot: root,
		ModulePath:  "github.com/example/myapp",
	}); err != nil {
		t.Fatalf("Build: %v", err)
	}
	sam, _ := os.ReadFile(filepath.Join(root, ".gen", "lambda", "template.yaml"))
	wants := []string{"SveltegoFunction:", "MemorySize: 512", "Timeout: 30"}
	for _, w := range wants {
		if !strings.Contains(string(sam), w) {
			t.Errorf("default SAM template missing %q", w)
		}
	}
}

func TestBuildErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		bc      adapterlambda.BuildContext
		wantSub string
	}{
		{"missing project root", adapterlambda.BuildContext{ModulePath: "x"}, "ProjectRoot is required"},
		{"missing module path", adapterlambda.BuildContext{ProjectRoot: t.TempDir()}, "ModulePath is required"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := adapterlambda.Build(context.Background(), tc.bc)
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error %q missing %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestDoc(t *testing.T) {
	t.Parallel()
	if !strings.Contains(adapterlambda.Doc(), "Lambda target") {
		t.Fatalf("Doc missing target heading")
	}
	if !strings.Contains(adapterlambda.Doc(), "aws-lambda-go-api-proxy") {
		t.Fatalf("Doc missing dependency hint")
	}
}

func TestBuildContextCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := adapterlambda.Build(ctx, adapterlambda.BuildContext{
		ProjectRoot: t.TempDir(),
		ModulePath:  "x",
	})
	if err == nil {
		t.Fatalf("expected ctx error")
	}
}
