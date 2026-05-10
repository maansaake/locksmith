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

// lockApp holds all runtime state for the interactive CLI session.
type lockApp struct {
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

func newLockApp(host string, port uint16, tlsConfig *tls.Config) *lockApp {
	return &lockApp{host: host, port: port, tlsConfig: tlsConfig}
}

func (a *lockApp) addr() string {
	return net.JoinHostPort(a.host, strconv.Itoa(int(a.port)))
}

// run connects to Locksmith and starts the interactive REPL.
func (a *lockApp) run() error {
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

// reprintPrompt re-draws the prompt after an async notification.
// It is safe to call from any goroutine; it only prints if liner is active.
func (a *lockApp) reprintPrompt() {
	a.mu.Lock()
	l := a.line
	a.mu.Unlock()
	if l != nil {
		fmt.Print(promptStr())
	}
}

// runREPL is the main input loop.  It uses liner for readline-style editing,
// command history, and tab completion.  Each typed line is dispatched through
// a cobra command tree so every command benefits from cobra's argument handling.
func (a *lockApp) runREPL() {
	// liner.NewLiner() reads from os.Stdin.  When stdin is redirected (e.g.
	// inside a Docker container without -t, through a pipe, or in some IDEs),
	// liner's terminal-support detection fails and Prompt() returns io.EOF
	// immediately.  Opening /dev/tty directly gives liner access to the actual
	// controlling terminal regardless of how stdin is connected.
	if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0600); err == nil {
		orig := os.Stdin
		os.Stdin = tty
		defer func() {
			os.Stdin = orig
			tty.Close()
		}()
	}

	l := liner.NewLiner()
	l.SetCtrlCAborts(true)

	a.mu.Lock()
	a.line = l
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		a.line = nil
		a.mu.Unlock()
		l.Close()
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
func (a *lockApp) setupCompletion(l *liner.State) {
	topLevel := []string{"acquire ", "release ", "list", "reconnect", "help", "exit", "quit"}

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
		if strings.HasPrefix(line, "release ") {
			prefix := strings.TrimPrefix(line, "release ")
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
func (a *lockApp) buildCobra() *cobra.Command {
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

func (a *lockApp) acquireCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "acquire [lock]",
		Short: "Acquire a lock",
		RunE: func(_ *cobra.Command, args []string) error {
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
				if err := form.Run(); err != nil || tag == "" {
					return nil
				}
			}

			if !a.isConnected() {
				fmt.Println(
					errorStyle.Render("✗ Not connected.") + "  " +
						mutedStyle.Render("Use 'reconnect' to reconnect."),
				)
				return nil
			}

			if err := a.c.Acquire(tag); err != nil {
				fmt.Println(errorStyle.Render("✗ Acquire error: " + err.Error()))
			} else {
				fmt.Println(
					infoStyle.Render("→ Acquire request sent for ") +
						lockTagStyle.Render(tag) + "  " +
						mutedStyle.Render("(waiting for server acknowledgement…)"),
				)
			}
			return nil
		},
	}
}

func (a *lockApp) releaseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "release [lock]",
		Short: "Release a lock",
		RunE: func(_ *cobra.Command, args []string) error {
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
				if err := form.Run(); err != nil || tag == "" {
					return nil
				}
			}

			if !a.isConnected() {
				fmt.Println(
					errorStyle.Render("✗ Not connected.") + "  " +
						mutedStyle.Render("Use 'reconnect' to reconnect."),
				)
				return nil
			}

			if err := a.c.Release(tag); err != nil {
				fmt.Println(errorStyle.Render("✗ Release error: " + err.Error()))
				return nil
			}
			a.removeLock(tag)
			fmt.Println(successStyle.Render("✓ Released: ") + lockTagStyle.Render(tag))
			return nil
		},
	}
}

func (a *lockApp) listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
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

func (a *lockApp) reconnectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reconnect",
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

func (a *lockApp) exitCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "exit",
		Aliases: []string{"quit"},
		Short:   "Exit locksmithctl",
		Run: func(_ *cobra.Command, _ []string) {
			a.shouldExit = true
		},
	}
}

// ---------------------------------------------------------------------------
// Thread-safe state helpers
// ---------------------------------------------------------------------------

func (a *lockApp) getLocks() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	result := make([]string, len(a.locks))
	copy(result, a.locks)
	return result
}

func (a *lockApp) addLock(lockTag string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.locks = append(a.locks, lockTag)
}

func (a *lockApp) removeLock(lockTag string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, l := range a.locks {
		if l == lockTag {
			a.locks = append(a.locks[:i], a.locks[i+1:]...)
			return
		}
	}
}

func (a *lockApp) isConnected() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.connected
}

func (a *lockApp) setConnected(v bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.connected = v
}
