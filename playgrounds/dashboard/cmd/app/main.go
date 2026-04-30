// Command app boots the dashboard playground server.
package main

import (
	"log"
	"os"

	gen "github.com/binsarjr/sveltego/playgrounds/dashboard/.gen"

	"github.com/binsarjr/sveltego/exports/kit/params"
	"github.com/binsarjr/sveltego/server"
)

func main() {
	shell, err := os.ReadFile("app.html")
	if err != nil {
		log.Fatalf("read app.html: %v", err)
	}
	s, err := server.New(server.Config{
		Routes:   gen.Routes(),
		Matchers: params.DefaultMatchers(),
		Shell:    string(shell),
		Hooks:    gen.Hooks(),
	})
	if err != nil {
		log.Fatalf("server.New: %v", err)
	}
	addr := ":3000"
	log.Printf("dashboard listening on %s", addr)
	log.Fatal(s.ListenAndServe(addr))
}
