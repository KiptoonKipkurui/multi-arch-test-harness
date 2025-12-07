// @title My API
// @version 1.0
// @desc My test harness API
// @server localhost:8080 Local
package main

import (
	"log"
	"os"

	"github.com/kiptoonkipkurui/multi-arch-test-harness/internal/api"
	"github.com/kiptoonkipkurui/multi-arch-test-harness/internal/config"
	"github.com/kiptoonkipkurui/multi-arch-test-harness/internal/logging"
	"github.com/kiptoonkipkurui/multi-arch-test-harness/internal/store"
)

func main() {
	logging.Init()
	config.Load()
	addr := ":8080"
	if v := os.Getenv("MTH_LISTEN_ADDR"); v != "" {
		addr = v
	}
	// storeType := os.Getenv("MTH_STORE")
	builder := store.NewStoreBuilder()
	// switch storeType {
	// case "sqlite":
	// 	builder.WithSQLite("data.db")
	// default:
	// 	builder.WithMemoryStore()
	// }

	st := builder.WithSQLite("./database").Build()

	srv := api.NewServer(addr, st)

	if err := srv.Start(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
