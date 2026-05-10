package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/charmbracelet/huh"
	"github.com/maansaake/locksmith/pkg/client"
	"github.com/peterh/liner"
	"github.com/spf13/cobra"
)

const (
	cmdHelp      = "help"
	cmdList      = "list"
	cmdAcquire   = "acquire"
	cmdRelease   = "release"
	cmdReconnect = "reconnect"
	cmdExit      = "exit"
	cmdQuit      = "quit"
)

// ctlSession holds all runtime state for the interactive CLI session.
type ctlSession struct {
	mu         sync.Mutex
	locks      []string
	c          client.Client
	connected  bool
	shouldExit bool

	host      string
	port      uint16
	tlsConfig *tls.Config

	// liner is the readline-style input handler (set only while the REPL is running).
	line *liner.State
}

func newCTLSession(host string, port uint16, tlsConfig *tls.Config) *ctlSession {
	return &ctlSession{host: host, port: port, tlsConfig: tlsConfig}
}

func (a *ctlSession) addr() string {
	return net.JoinHostPort(a.host, strconv.Itoa(int(a.port)))
}

// run connects to Locksmith and starts the interactive REPL.
func (a *ctlSession) run() error {
	fmt.Println(renderBanner(a.addr()))

	a.c = client.New(&client.Opts{
		Host:      a.host,
		Port:      a.port,
		TLSConfig: a.tlsConfig,

		// OnAcquired fires in a goroutine; we clear the current input line,
		// print the notification, then reprint the prompt so the trace remains visible.
		OnAcquired: func(lockTag string) {
			a.addLock(lockTag)
			fmt.Printf("\r\033[K%s %s\n",
				successStyle.Render("✓ Acquired:"),
				lockTagStyle.Render(lockTag),
			)
			a.reprintPrompt()
		},

		// OnDisconnected fires when the connection drops unexpectedly.
		OnDisconnected: func() {
			a.clearLocks()
			a.setConnected(false)
			fmt.Printf("\r\033[K%s\n", renderDisconnectWarning())
			a.reprintPrompt()
		},
	})

	if err := a.c.Connect(); err != nil {
		// Connection failure is non-fatal: show an error and drop into the REPL
		// so the user can type 'reconnect' once the server becomes available.
		fmt.Println(warnBoxStyle.Render(
			warnStyle.Render("⚠  Could not connect to "+a.addr()) + "\n" +
				mutedStyle.Render(err.Error()) + "\n" +
				mutedStyle.Render("Type 'reconnect' to try again"),
		))
		fmt.Println()
	} else {
		a.setConnected(true)
		fmt.Println(successStyle.Render("✓ Connected to ") + infoStyle.Render(a.addr()))
		fmt.Println()
	}

	printHelp()
	fmt.Println()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		a.c.Close()
		fmt.Println()
		os.Exit(0)
	}()

	a.runREPL()
	a.c.Close()
	return nil
}

// reprintPrompt re-draws the prompt, for example after an async notification.
func (a *ctlSession) reprintPrompt() {
	fmt.Print(promptStr())
}

// runREPL is the main input loop. It uses liner for readline-style editing,
// command history, and tab completion. Each typed line is dispatched through
// a cobra command tree so every command benefits from cobra's argument handling.
func (a *ctlSession) runREPL() {
	l := liner.NewLiner()
	l.SetCtrlCAborts(true)

	a.mu.Lock()
	a.line = l
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		a.line = nil
		a.mu.Unlock()
		_ = l.Close()
	}()

	a.setupCompletion(l)
	root := a.buildCobra()

	for {
		input, err := l.Prompt(promptStr())
		if err != nil {
			// liner.ErrPromptAborted = Ctrl-C; io.EOF = Ctrl-D / pipe closed.
			fmt.Println()
			break
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		// Append to liner's internal history so ↑/↓ work.
		l.AppendHistory(input)

		root.SetArgs(strings.Fields(input))
		//nolint:govet // shad
		if err := root.Execute(); err != nil {
			// Cobra returns an error for unknown commands (SilenceErrors suppresses
			// its own printing so we render a styled message here).
			args := strings.Fields(input)
			if len(args) > 0 {
				fmt.Println(
					errorStyle.Render("✗ Unknown command: "+args[0]) + "  " +
						mutedStyle.Render("Type 'help' for available commands."),
				)
			}
		}

		if a.shouldExit {
			break
		}
	}
}

// setupCompletion registers liner's tab-completion function.
// Completing at the start of the line offers command names; completing after
// "release " offers the names of currently-held locks.
func (a *ctlSession) setupCompletion(l *liner.State) {
	topLevel := []string{cmdAcquire + " ", cmdRelease + " ", cmdList, cmdReconnect, cmdHelp, cmdExit, cmdQuit}

	l.SetCompleter(func(line string) []string {
		var completions []string

		if !strings.Contains(line, " ") {
			// Complete command names.
			for _, c := range topLevel {
				if strings.HasPrefix(c, line) {
					completions = append(completions, c)
				}
			}
			return completions
		}

		// Complete "release <lock>" with currently-held lock tags.
		if after, ok := strings.CutPrefix(line, "release "); ok {
			prefix := after
			for _, lock := range a.getLocks() {
				if strings.HasPrefix(lock, prefix) {
					completions = append(completions, "release "+lock)
				}
			}
		}

		return completions
	})
}

// buildCobra constructs the cobra command tree used inside the REPL.
// A fresh tree is built once and reused for every REPL iteration.
func (a *ctlSession) buildCobra() *cobra.Command {
	root := &cobra.Command{
		Use:           "locksmithctl",
		SilenceErrors: true,
		SilenceUsage:  true,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}

	// Replace cobra's built-in help display with our styled version.
	root.SetHelpFunc(func(_ *cobra.Command, _ []string) { printHelp() })

	root.AddCommand(
		a.acquireCmd(),
		a.releaseCmd(),
		a.listCmd(),
		a.reconnectCmd(),
		a.exitCmd(),
	)

	return root
}

// ---------------------------------------------------------------------------
// Command definitions
// ---------------------------------------------------------------------------

func (a *ctlSession) acquireCmd() *cobra.Command {
	return &cobra.Command{
		Use:   cmdAcquire + " [lock]",
		Short: "Acquire a lock",
		RunE: func(_ *cobra.Command, args []string) error {
			if !a.isConnected() {
				fmt.Println(
					errorStyle.Render("✗ Not connected.") + "  " +
						mutedStyle.Render("Use 'reconnect' to reconnect."),
				)
				return nil
			}

			tag := ""
			if len(args) >= 1 {
				tag = args[0]
			} else {
				// Huh form: interactive lock-tag input when no arg was provided.
				form := huh.NewForm(
					huh.NewGroup(
						huh.NewInput().
							Title("Lock Tag").
							Description("Enter the lock tag to acquire").
							Placeholder("my-lock").
							Value(&tag),
					),
				)

				if err := form.Run(); err != nil {
					fmt.Println(errorStyle.Render("✗ Acquire failed: " + err.Error()))
					return nil //nolint:nilerr // error already printed
				}

				if tag == "" {
					fmt.Println(mutedStyle.Render("No lock tag entered."))
					return nil
				}
			}

			if err := a.c.Acquire(tag); err != nil {
				fmt.Println(errorStyle.Render("✗ Acquire failed: " + err.Error()))
				return nil //nolint:nilerr // error already printed
			}

			fmt.Println(
				infoStyle.Render("→ Acquire request sent for ") +
					lockTagStyle.Render(tag) + "  " +
					mutedStyle.Render("(waiting for server acknowledgement…)"),
			)

			return nil
		},
	}
}

func (a *ctlSession) releaseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   cmdRelease + " [lock]",
		Short: "Release a lock",
		RunE: func(_ *cobra.Command, args []string) error {
			if !a.isConnected() {
				fmt.Println(
					errorStyle.Render("✗ Not connected.") + "  " +
						mutedStyle.Render("Use 'reconnect' to reconnect."),
				)
				return nil
			}

			tag := ""
			if len(args) >= 1 {
				tag = args[0]
			} else {
				locks := a.getLocks()
				if len(locks) == 0 {
					fmt.Println(mutedStyle.Render("No locks acquired."))
					return nil
				}
				// Huh form: select from currently-held locks.
				form := huh.NewForm(
					huh.NewGroup(
						huh.NewSelect[string]().
							Title("Select lock to release").
							Options(huh.NewOptions(locks...)...).
							Value(&tag),
					),
				)

				if err := form.Run(); err != nil {
					fmt.Println(errorStyle.Render("✗ Release failed: " + err.Error()))
					return nil //nolint:nilerr // error already printed
				}

				if tag == "" {
					fmt.Println(mutedStyle.Render("No lock selected."))
					return nil
				}
			}

			if err := a.c.Release(tag); err != nil {
				fmt.Println(errorStyle.Render("✗ Release failed: " + err.Error()))
				return nil //nolint:nilerr // error already printed
			}

			a.removeLock(tag)
			fmt.Println(successStyle.Render("✓ Released: ") + lockTagStyle.Render(tag))
			return nil
		},
	}
}

func (a *ctlSession) listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   cmdList,
		Short: "List all acquired locks",
		Run: func(_ *cobra.Command, _ []string) {
			locks := a.getLocks()
			if len(locks) == 0 {
				fmt.Println(mutedStyle.Render("No locks acquired."))
				return
			}
			fmt.Println(renderLockList(locks))
		},
	}
}

func (a *ctlSession) reconnectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   cmdReconnect,
		Short: "Reconnect to the Locksmith server",
		Run: func(_ *cobra.Command, _ []string) {
			if a.isConnected() {
				fmt.Println(infoStyle.Render("Already connected to " + a.addr()))
				return
			}
			fmt.Println(infoStyle.Render("→ Reconnecting to " + a.addr() + "…"))
			if err := a.c.Connect(); err != nil {
				fmt.Println(errorStyle.Render("✗ Reconnect failed: " + err.Error()))
				return
			}
			a.setConnected(true)
			fmt.Println(successStyle.Render("✓ Reconnected to ") + infoStyle.Render(a.addr()))
		},
	}
}

func (a *ctlSession) exitCmd() *cobra.Command {
	return &cobra.Command{
		Use:     cmdExit,
		Aliases: []string{cmdQuit},
		Short:   "Exit locksmithctl",
		Run: func(_ *cobra.Command, _ []string) {
			a.shouldExit = true
		},
	}
}

// ---------------------------------------------------------------------------
// Thread-safe state helpers
// ---------------------------------------------------------------------------

func (a *ctlSession) getLocks() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	result := make([]string, len(a.locks))
	copy(result, a.locks)
	return result
}

func (a *ctlSession) addLock(lockTag string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.locks = append(a.locks, lockTag)
}

func (a *ctlSession) removeLock(lockTag string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, l := range a.locks {
		if l == lockTag {
			a.locks = append(a.locks[:i], a.locks[i+1:]...)
			return
		}
	}
}

func (a *ctlSession) clearLocks() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.locks = nil
}

func (a *ctlSession) isConnected() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.connected
}

func (a *ctlSession) setConnected(v bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.connected = v
}
