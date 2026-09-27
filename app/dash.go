package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
)

// Dash runs the session with the dashboard as its only ui: the board and the dashboard on
// ui.board.port, asks and approvals from the browser, the terminal only says where to look.
// ctrl+c ends the session.
func (a *App) Dash(ctx context.Context, open bool) int {
	errw := a.Opt.Err
	if a.Workspace == nil {
		fmt.Fprintf(errw, "@S FAIL\n@? no %s here — `%s init` founds one\n", worldWord(a.Cfg.Dist.Name), a.Cfg.Dist.Name)
		return 2
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)
	go func() {
		select {
		case <-sig:
			cancel()
		case <-ctx.Done():
		}
	}()
	a.serveBoard(ctx)
	a.mu.Lock()
	addr := a.boardAddr
	a.mu.Unlock()
	if addr == "" {
		fmt.Fprintf(errw, "@S FAIL\n@? the dashboard could not listen: %v\n", a.Holes())
		return 2
	}
	w := a.webBus()
	w.mu.Lock()
	w.answerable = true
	w.mu.Unlock()
	h, err := a.tuiHost(ctx)
	if err != nil {
		fmt.Fprintf(errw, "@S FAIL\n@? %v\n", err)
		return 2
	}
	h.attach(w)
	w.attach(h)
	defer w.close()
	url := "http://" + addr + "/dash"
	fmt.Fprintf(errw, "%s dashboard: %s\n  the session runs here; ask and approve in the browser · ctrl+c ends it\n", a.Cfg.Dist.Name, url)
	if open {
		openBrowser(url)
	}
	return h.run(func() error {
		<-ctx.Done()
		return nil
	})
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
