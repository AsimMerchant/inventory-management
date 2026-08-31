// Command storeregister is the store desk's register. Double-click it: it opens
// a browser on this machine and nowhere else, and keeps its data in one plain
// file beside itself.
package main

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"storeregister/internal/store"
	"storeregister/internal/web"
)

// The port hunt. 8765 first, then upward. Twenty-one tries is more than a desk
// laptop will ever need and stops the program looping if something is very wrong.
const (
	firstPort = 8765
	lastPort  = 8785
)

func main() {
	path, err := store.DataPath()
	if err != nil {
		stop("The register could not work out where to keep its file: " + err.Error())
	}

	st, loaded, err := store.Open(path)
	if err != nil {
		stop(err.Error())
	}

	ln, err := listen()
	if err != nil {
		stop("The Store Register may already be open. Look at your browser tabs before starting it again.")
	}

	addr := ln.Addr().String()
	fmt.Print(consoleBanner(addr))

	srv := &http.Server{
		Handler:           web.NewServer(st, loaded.Warning, time.Now),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		if err := srv.Serve(ln); err != nil {
			fmt.Println(err)
		}
	}()

	url := "http://" + addr + "/"
	if err := openBrowser(url); err != nil {
		fmt.Println("Open your browser and type: " + url)
	}

	// Stopped by closing the window. There is nothing to shut down gracefully:
	// every entry is already on disk before its page is drawn.
	select {}
}

// listen takes the first free port in the range, on the loopback address only.
// Binding anything else raises the Windows Defender Firewall dialog, and a
// non-technical person facing a security prompt is a failed handover.
func listen() (net.Listener, error) {
	for port := firstPort; port <= lastPort; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			return ln, nil
		}
	}
	return nil, fmt.Errorf("every port from %d to %d is busy", firstPort, lastPort)
}

// consoleBanner is what the black window says. The address comes first on
// purpose: a reader who sees "Store Register is running" looks away satisfied
// and never reaches the warning below it.
func consoleBanner(addr string) string {
	return "  Open this address in your browser:\n" +
		"  http://" + addr + "\n" +
		"\n" +
		"Leave this window open. If you close it, the register stops.\n"
}

// openBrowser opens the default browser. A failure is one printed line and
// never stops the server: the address is already on the screen.
func openBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

// stop prints a message a person can read and waits, because a double-clicked
// window disappears the moment the program ends.
func stop(message string) {
	fmt.Println(message)
	fmt.Println("Press Enter to close.")
	bufio.NewReader(os.Stdin).ReadString('\n')
	os.Exit(1)
}
