package network

import (
	"fmt"
	"log"
	"net"
	"net/http"

	"sql-db/pkg/executor"
)

func Listen(addr string, exec *executor.Executor) error {
	http.HandleFunc("/query", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			r.ParseForm()
			sql := r.Form.Get("sql")
			result, err := exec.Execute(sql)
			if err != nil {
				fmt.Fprintf(w, "error: %v", err)
				return
			}
			fmt.Fprintf(w, "%v", result)
		}
	})

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	log.Printf("Server listening on %s", addr)
	return http.Serve(ln, nil)
}
