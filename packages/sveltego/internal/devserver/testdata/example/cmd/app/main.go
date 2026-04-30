package main

import (
	"log"
	"os"

	gen "devexample/.gen"

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
	port := os.Getenv("PORT")
	if port == "" {
		port = "5174"
	}
	addr := "127.0.0.1:" + port
	log.Printf("devexample: listening on %s", addr)
	log.Fatal(s.ListenAndServe(addr))
}
