// Command flaresolverr is the HolyArrs FlareSolverr plugin. When a Source's
// site puts a bot check (Cloudflare, DDoS-Guard) in front of its feed,
// HolyArrs hands that one page to this plugin through the botcheck hook; the
// plugin asks the owner's FlareSolverr to open it and hands back the page,
// the cookies and the browser's User-Agent. All the rules about which pages
// may be handed over, and what is kept, are HolyArrs's own.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
)

// Environment HolyArrs passes in. The token is read once and dropped.
const (
	envTarget  = "HOLYARRS_PLUGIN_TARGET" // HolyArrs's local address, such as http://127.0.0.1:6262
	envToken   = "HOLYARRS_PLUGIN_TOKEN"
	envAddress = "HOLYARRS_PLUGIN_SETTING_ADDRESS" // the "address" setting
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	target, token := os.Getenv(envTarget), os.Getenv(envToken)
	os.Unsetenv(envToken)
	if target == "" || token == "" {
		// Nothing to ask without them; wait to be stopped rather than exit
		// and be started again.
		log.Printf("%s or %s is not set; this HolyArrs is too old for this plugin", envTarget, envToken)
		<-ctx.Done()
		return
	}
	newPlugin(target, token, os.Getenv(envAddress), log.Default()).run(ctx)
}
