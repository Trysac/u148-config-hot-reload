package confighotreload_test

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Trysac/u148-config-hot-reload"
)

func ExampleReloader_Watch() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	fs := newMemFS()
	fs.set("app.conf", "port=8080")

	reloader, err := confighotreload.New(confighotreload.Options[Config]{
		Path: "app.conf",
		Open: fs.open,
		Parse: func(data []byte) (Config, error) {
			val, err := strconv.Atoi(strings.TrimPrefix(string(data), "port="))
			return Config{Port: val}, err
		},
	})
	if err != nil {
		panic(err)
	}

	go func() {
		_ = reloader.Watch(ctx, ticker.C)
	}()

	fmt.Printf("Current port: %d\n", reloader.Current().Port)
	// Output:
	// Current port: 8080
}
